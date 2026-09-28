package apiServices

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	"github.com/wecredit/communication-sdk/pkg/cache"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/gorm"
)

const stageConfigurationLockWaitSeconds = 10

var ErrConfigurationBusy = errors.New("stage configuration is temporarily busy")

func stageConfigurationLockIdentity(lenderName, commType string, stage int) string {
	return fmt.Sprintf("%s|%s|%d", strings.ToLower(strings.TrimSpace(lenderName)), strings.ToUpper(strings.TrimSpace(commType)), stage)
}

func stageConfigurationLockName(identity string) string {
	digest := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("comm-stage:%x", digest[:24])
}

func templateStageLockIdentity(template apiModels.Templatedetails) (string, error) {
	if template.Stage == nil {
		return "", nil
	}
	canonical, err := cache.CanonicalTemplateStage(*template.Stage)
	if err != nil {
		return "", err
	}
	whole, err := strconv.Atoi(strings.SplitN(canonical, ".", 2)[0])
	if err != nil {
		return "", err
	}
	return stageConfigurationLockIdentity(template.Process, template.Channel, whole), nil
}

// acquireStageConfigurationLocks sorts canonical identities before hashing.
// Every call must use the pinned GORM connection supplied by DB.Connection.
func acquireStageConfigurationLocks(conn *gorm.DB, identities ...string) ([]string, error) {
	ordered := OrderedStageConfigurationLockIdentities(identities...)

	locks := make([]string, 0, len(ordered))
	for _, identity := range ordered {
		name := stageConfigurationLockName(identity)
		var acquired sql.NullInt64
		if err := conn.Raw("SELECT GET_LOCK(?, ?)", name, stageConfigurationLockWaitSeconds).Scan(&acquired).Error; err != nil {
			releaseStageConfigurationLocks(conn, locks)
			return nil, fmt.Errorf("acquire stage configuration lock: %w", err)
		}
		if !acquired.Valid || acquired.Int64 != 1 {
			releaseStageConfigurationLocks(conn, locks)
			return nil, fmt.Errorf("%w: timed out after %d seconds", ErrConfigurationBusy, stageConfigurationLockWaitSeconds)
		}
		locks = append(locks, name)
	}
	return locks, nil
}

func OrderedStageConfigurationLockIdentities(identities ...string) []string {
	unique := make(map[string]struct{}, len(identities))
	ordered := make([]string, 0, len(identities))
	for _, identity := range identities {
		if identity == "" {
			continue
		}
		if _, exists := unique[identity]; exists {
			continue
		}
		unique[identity] = struct{}{}
		ordered = append(ordered, identity)
	}
	sort.Strings(ordered)
	return ordered
}

// Release is deliberately fail-open: it never changes the durable mutation result.
func releaseStageConfigurationLocks(conn *gorm.DB, locks []string) {
	for i := len(locks) - 1; i >= 0; i-- {
		var released sql.NullInt64
		err := conn.Raw("SELECT RELEASE_LOCK(?)", locks[i]).Scan(&released).Error
		if err != nil || !released.Valid || released.Int64 != 1 {
			utils.Error(fmt.Errorf("stage configuration lock release failed (lock=%s released=%v value=%d): %v", locks[i], released.Valid, released.Int64, err))
		}
	}
}
