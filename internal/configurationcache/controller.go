package configurationcache

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	redisclient "github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/models"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/gorm"
)

type ControllerOptions struct {
	Environment           string
	VersionTable          string
	TemplateTable         string
	PollInterval          time.Duration
	MinimumReloadInterval time.Duration
}

type reloadRequest struct {
	templateVersion int64
	triggerSource   string
}

// Cache reload monitoring is intentionally log-only while communication-sdk
// runs as a single ECS instance. If the service is scaled out, keep the same
// version/pub-sub protocol and add the ECS task ID plus requested/installed
// versions to these structured logs. CloudWatch Logs Insights and log-derived
// alarms can then show lag or failures per task. Introduce durable per-instance
// propagation storage only if a future release-status API needs queryable
// history; it is not needed for cache correctness.
type Controller struct {
	readDB  *gorm.DB
	redis   *redisclient.Client
	options ControllerOptions

	mu               sync.Mutex
	pending          reloadRequest
	installedVersion int64
	lastAttempt      time.Time
	wake             chan struct{}
}

// OptionsFromConfig converts the configuration into controller options
func OptionsFromConfig(config models.Config) (ControllerOptions, error) {
	pollInterval, err := boundedSeconds(config.CacheVersionPollIntervalSeconds, 90, 60, 120)
	if err != nil {
		return ControllerOptions{}, fmt.Errorf("cache version poll interval: %w", err)
	}

	minimumReloadInterval, err := boundedSeconds(config.CacheReloadMinIntervalSeconds, 60, 1, 3600)
	if err != nil {
		return ControllerOptions{}, fmt.Errorf("cache reload minimum interval: %w", err)
	}

	options := ControllerOptions{
		Environment:           strings.TrimSpace(config.Environment),
		VersionTable:          strings.TrimSpace(config.ConfigurationVersionTable),
		TemplateTable:         strings.TrimSpace(config.TemplateDetailsTable),
		PollInterval:          pollInterval,
		MinimumReloadInterval: minimumReloadInterval,
	}

	if options.Environment == "" || options.VersionTable == "" || options.TemplateTable == "" {
		return ControllerOptions{}, errors.New("cache propagation environment and table names are required")
	}

	return options, nil
}

// StartController starts the controller
func StartController(ctx context.Context, readDB *gorm.DB, redis *redisclient.Client, options ControllerOptions) (*Controller, error) {
	if readDB == nil || redis == nil {
		return nil, errors.New("cache controller requires read DB and Redis")
	}

	controller := &Controller{
		readDB:  readDB,
		redis:   redis,
		options: options,
		wake:    make(chan struct{}, 1),
	}

	if err := controller.reload(ctx, reloadRequest{triggerSource: "startup"}); err != nil {
		return nil, fmt.Errorf("initial template cache load: %w", err)
	}

	channel, err := invalidationChannel(options.Environment)
	if err != nil {
		return nil, err
	}

	subscription := redis.Subscribe(ctx, channel)
	if _, err := subscription.Receive(ctx); err != nil {
		_ = subscription.Close()
		return nil, fmt.Errorf("establish cache invalidation subscription: %w", err)
	}
	utils.Info(fmt.Sprintf(
		"template cache invalidation subscription established (environment=%s channel=%s)",
		options.Environment, channel,
	))

	go controller.runWorker(ctx)
	go controller.runSubscriber(ctx, subscription)
	go controller.runPoller(ctx)

	return controller, nil
}

// requestReload requests a reload of the template cache
func (c *Controller) requestReload(request reloadRequest) {
	c.mu.Lock()
	if request.templateVersion <= c.installedVersion {
		c.mu.Unlock()
		return
	}

	if request.templateVersion >= c.pending.templateVersion {
		c.pending = request
	}

	c.mu.Unlock()

	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// runWorker runs the worker
func (c *Controller) runWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.wake:
		}

		c.mu.Lock()
		wait := c.options.MinimumReloadInterval - time.Since(c.lastAttempt)
		c.mu.Unlock()
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}

		c.mu.Lock()
		request := c.pending
		c.pending = reloadRequest{}
		installedVersion := c.installedVersion
		c.mu.Unlock()

		if request.templateVersion == 0 || request.templateVersion <= installedVersion {
			continue
		}

		if err := c.reload(ctx, request); err != nil {
			c.requestReload(request)
		}
	}
}

// runSubscriber runs the subscriber
func (c *Controller) runSubscriber(ctx context.Context, subscription *redisclient.PubSub) {
	channel, err := invalidationChannel(c.options.Environment)
	if err != nil {
		utils.Error(err)
		return
	}

	for ctx.Err() == nil {
		if subscription == nil {
			subscription = c.redis.Subscribe(ctx, channel)
			if _, err := subscription.Receive(ctx); err != nil {
				_ = subscription.Close()
				subscription = nil
				if ctx.Err() != nil {
					return
				}

				utils.Error(fmt.Errorf("establish cache invalidation subscription: %w", err))
			} else {
				utils.Info(fmt.Sprintf(
					"template cache invalidation subscription re-established (environment=%s channel=%s)",
					c.options.Environment, channel,
				))
			}
		}

		if subscription == nil {
			if !waitForRetry(ctx, 5*time.Second) {
				return
			}
			continue
		}

		for {
			message, receiveErr := subscription.ReceiveMessage(ctx)
			if receiveErr != nil {
				_ = subscription.Close()
				subscription = nil
				if ctx.Err() != nil {
					return
				}

				utils.Error(fmt.Errorf("cache invalidation subscription: %w", receiveErr))
				break
			}

			var event templateInvalidationEvent
			if unmarshalErr := json.Unmarshal([]byte(message.Payload), &event); unmarshalErr != nil {
				utils.Error(fmt.Errorf("decode cache invalidation: %w", unmarshalErr))
				continue
			}
			utils.Info(fmt.Sprintf(
				"template cache invalidation received (environment=%s channel=%s templateVersion=%d)",
				c.options.Environment, message.Channel, event.TemplateVersion,
			))

			// request a reload of the template cache
			c.requestReload(reloadRequest{
				templateVersion: event.TemplateVersion,
				triggerSource:   "pubsub",
			})
		}

		if !waitForRetry(ctx, 5*time.Second) {
			return
		}
	}
}

// runPoller runs the poller
func (c *Controller) runPoller(ctx context.Context) {
	// Reconcile immediately so an update committed between startup load and
	// subscription establishment cannot be missed.
	c.poll(ctx)
	for ctx.Err() == nil {
		jitter := time.Duration(rand.Int63n(int64(c.options.PollInterval/5)+1)) - c.options.PollInterval/10
		timer := time.NewTimer(c.options.PollInterval + jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			c.poll(ctx)
		}
	}
}

// poll polls the template version
func (c *Controller) poll(ctx context.Context) {
	templateVersion, err := ReadTemplateVersion(ctx, c.readDB, c.options.VersionTable)
	if err != nil {
		utils.Error(fmt.Errorf("poll configuration version: %w", err))
		return
	}

	c.requestReload(reloadRequest{
		templateVersion: templateVersion,
		triggerSource:   "poll",
	})
}

// reload reloads the template cache
func (c *Controller) reload(ctx context.Context, request reloadRequest) error {
	started := time.Now()
	c.mu.Lock()
	c.lastAttempt = started
	installedBefore := c.installedVersion // get the installed version before the reload
	c.mu.Unlock()

	// start a transaction
	tx := c.readDB.WithContext(ctx).Begin(&sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if tx.Error != nil {
		return c.recordFailure(request, installedBefore, started, fmt.Errorf("start template snapshot transaction: %w", tx.Error))
	}

	defer tx.Rollback() // rollback the transaction if an error occurs

	// get the template version from the database
	templateVersion, err := ReadTemplateVersion(ctx, tx, c.options.VersionTable)
	if err != nil {
		return c.recordFailure(request, installedBefore, started, err)
	}

	// check if the template version is behind the requested version
	if templateVersion < request.templateVersion {
		return c.recordFailure(request, installedBefore, started, fmt.Errorf("read replica template version %d is behind requested version %d", templateVersion, request.templateVersion))
	}

	// get the template data from the database
	var rows []apiModels.Templatedetails
	if err := tx.Table(c.options.TemplateTable).Find(&rows).Error; err != nil {
		return c.recordFailure(request, installedBefore, started, err)
	}

	// build the template snapshot
	snapshot, err := cache.BuildTemplateSnapshot(rows)
	if err != nil {
		return c.recordFailure(request, installedBefore, started, err)
	}

	// commit the transaction
	if err := tx.Commit().Error; err != nil {
		return c.recordFailure(request, installedBefore, started, fmt.Errorf("commit template snapshot read: %w", err))
	}

	// install the template snapshot
	if err := cache.InstallTemplateSnapshot(snapshot); err != nil {
		return c.recordFailure(request, installedBefore, started, err)
	}

	c.mu.Lock()
	c.installedVersion = templateVersion
	c.mu.Unlock()

	// log the unresolvable templates
	for _, finding := range snapshot.Findings {
		findingLog, marshalErr := json.Marshal(map[string]interface{}{
			"event": "unresolvable_active_template", "environment": c.options.Environment,
			"templateId": finding.TemplateID, "channel": finding.Channel, "reason": finding.Reason,
		})

		// log the unresolvable templates
		if marshalErr == nil {
			utils.Warn(string(findingLog))
		}
	}

	utils.Info(fmt.Sprintf(
		"template cache reload succeeded (trigger=%s requestedVersion=%d installedVersion=%d duration=%s)",
		request.triggerSource, request.templateVersion, templateVersion, time.Since(started),
	))

	return nil
}

// recordFailure records a failure
func (c *Controller) recordFailure(request reloadRequest, installedVersion int64, started time.Time, reloadErr error) error {
	utils.Error(fmt.Errorf(
		"template cache reload failed (trigger=%s requestedVersion=%d installedVersion=%d duration=%s): %w",
		request.triggerSource, request.templateVersion, installedVersion, time.Since(started), reloadErr,
	))

	return fmt.Errorf("reload template cache: %w", reloadErr)
}

// boundedSeconds converts a string to a duration
func boundedSeconds(raw string, defaultValue, minimum, maximum int) (time.Duration, error) {
	value := defaultValue
	if strings.TrimSpace(raw) != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return 0, err
		}
		value = parsed
	}

	if value < minimum || value > maximum {
		return 0, fmt.Errorf("must be between %d and %d seconds", minimum, maximum)
	}

	return time.Duration(value) * time.Second, nil
}

// waitForRetry waits for a retry
func waitForRetry(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
