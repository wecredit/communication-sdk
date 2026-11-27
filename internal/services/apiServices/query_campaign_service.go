package apiServices

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	sdkHelper "github.com/wecredit/communication-sdk/sdk/helper"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrQueryCampaignNotFound   = errors.New("query campaign not found")
	ErrQueryCampaignValidation = errors.New("query campaign validation failed")
	ErrQueryCampaignConflict   = errors.New("query campaign conflict")
)

type previewJobEntry struct {
	cancel     context.CancelFunc
	generation uint64
	startedAt  time.Time
}

// QueryCampaignService owns admin CRUD/preview/approve/enable for query campaigns.
type QueryCampaignService struct {
	ReadDB  *gorm.DB
	WriteDB *gorm.DB
	// ROPool returns an existing service *sql.DB for the given DbTargetId.
	// Callers must not Close the returned handle — it is a shared pool.
	ROPool func(dbTargetID string) (*sql.DB, error)

	previewMu   sync.Mutex
	previewJobs map[int64]previewJobEntry
	previewGen  uint64
}

func NewQueryCampaignService(readDB, writeDB *gorm.DB) *QueryCampaignService {
	s := &QueryCampaignService{
		ReadDB:      readDB,
		WriteDB:     writeDB,
		previewJobs: make(map[int64]previewJobEntry),
	}
	s.ROPool = s.defaultROPool
	return s
}

// defaultROPool reuses the service's existing GORM pools (no separate RO DSN ENV).
// Prefer tech read for communication-mysql; Core / Marketing use their product pools.
func (s *QueryCampaignService) defaultROPool(dbTargetID string) (*sql.DB, error) {
	var gormDB *gorm.DB
	switch dbTargetID {
	case sdkHelper.DbTargetMarketingSQLServer:
		gormDB = database.DBMarketing
	case sdkHelper.DbTargetZapCashMySQLV1:
		gormDB = database.DBZapCashV1
	case sdkHelper.DbTargetCommunicationMySQL:
		gormDB = database.DBtechRead
		if gormDB == nil {
			gormDB = database.DBtechWrite
		}
	default:
		return nil, fmt.Errorf("%w: unknown dbTargetId %s", ErrQueryCampaignValidation, dbTargetID)
	}
	if gormDB == nil {
		return nil, fmt.Errorf("%w: query campaign pool not available for dbTargetId %s", ErrQueryCampaignValidation, dbTargetID)
	}
	return gormDB.DB()
}

// beginPreviewJob cancels any in-flight preview for campaignID and returns a
// fresh cancelable context plus generation token for the new job.
func (s *QueryCampaignService) beginPreviewJob(campaignID int64, startedAt time.Time) (context.Context, uint64) {
	s.previewMu.Lock()
	defer s.previewMu.Unlock()
	if s.previewJobs == nil {
		s.previewJobs = make(map[int64]previewJobEntry)
	}
	if prev, ok := s.previewJobs[campaignID]; ok && prev.cancel != nil {
		prev.cancel()
	}
	s.previewGen++
	gen := s.previewGen
	ctx, cancel := context.WithCancel(context.Background())
	s.previewJobs[campaignID] = previewJobEntry{cancel: cancel, generation: gen, startedAt: startedAt}
	return ctx, gen
}

func (s *QueryCampaignService) cancelPreviewJob(campaignID int64) {
	s.previewMu.Lock()
	defer s.previewMu.Unlock()
	if prev, ok := s.previewJobs[campaignID]; ok && prev.cancel != nil {
		prev.cancel()
		delete(s.previewJobs, campaignID)
	}
}

func (s *QueryCampaignService) endPreviewJob(campaignID int64, generation uint64) {
	s.previewMu.Lock()
	defer s.previewMu.Unlock()
	cur, ok := s.previewJobs[campaignID]
	if !ok || cur.generation != generation {
		return
	}
	if cur.cancel != nil {
		cur.cancel()
	}
	delete(s.previewJobs, campaignID)
}

func (s *QueryCampaignService) isCurrentPreviewJob(campaignID int64, generation uint64) bool {
	s.previewMu.Lock()
	defer s.previewMu.Unlock()
	cur, ok := s.previewJobs[campaignID]
	return ok && cur.generation == generation
}

// BeginPreviewJobForTest exposes beginPreviewJob for unit tests (no live DB).
func (s *QueryCampaignService) BeginPreviewJobForTest(campaignID int64, startedAt time.Time) (context.Context, uint64) {
	return s.beginPreviewJob(campaignID, startedAt)
}

// CancelPreviewJobForTest exposes cancelPreviewJob for unit tests.
func (s *QueryCampaignService) CancelPreviewJobForTest(campaignID int64) {
	s.cancelPreviewJob(campaignID)
}

// IsCurrentPreviewJobForTest exposes isCurrentPreviewJob for unit tests.
func (s *QueryCampaignService) IsCurrentPreviewJobForTest(campaignID int64, generation uint64) bool {
	return s.isCurrentPreviewJob(campaignID, generation)
}

func (s *QueryCampaignService) maxQuerySeconds() int {
	n, _ := strconv.Atoi(strings.TrimSpace(config.Configs.QueryCampaignMaxQuerySeconds))
	return n
}

func (s *QueryCampaignService) previewMaxAge() time.Duration {
	h, _ := strconv.Atoi(strings.TrimSpace(config.Configs.QueryCampaignPreviewMaxAgeHours))
	return time.Duration(h) * time.Hour
}

func (s *QueryCampaignService) previewStaleBuffer() time.Duration {
	sec, _ := strconv.Atoi(strings.TrimSpace(config.Configs.QueryCampaignPreviewStaleBufferSeconds))
	return time.Duration(sec) * time.Second
}

func validationErr(msg string) error {
	return fmt.Errorf("%w: %s", ErrQueryCampaignValidation, msg)
}

func (s *QueryCampaignService) getByID(id int64) (*apiModels.QueryCampaign, error) {
	var c apiModels.QueryCampaign
	if err := s.ReadDB.First(&c, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrQueryCampaignNotFound
		}
		return nil, err
	}
	return &c, nil
}

func scopeAllows(c *apiModels.QueryCampaign, unrestricted bool, scopedClient string) bool {
	if unrestricted {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(c.Client), strings.TrimSpace(scopedClient))
}

func (s *QueryCampaignService) List(params apiModels.QueryCampaignListParams) ([]apiModels.QueryCampaign, int64, error) {
	q := s.ReadDB.Model(&apiModels.QueryCampaign{})
	if !params.Unrestricted {
		q = q.Where("Client = ?", strings.ToLower(params.ScopedClient))
	} else if params.Client != "" {
		q = q.Where("Client = ?", strings.ToLower(params.Client))
	}
	if params.Channel != "" {
		q = q.Where("Channel = ?", strings.ToUpper(params.Channel))
	}
	if params.Status != "" {
		q = q.Where("Status = ?", strings.ToLower(params.Status))
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, size := params.Page, params.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 25
	}
	var items []apiModels.QueryCampaign
	err := q.Order("Id DESC").Offset((page - 1) * size).Limit(size).Find(&items).Error
	return items, total, err
}

func (s *QueryCampaignService) Get(id int64, unrestricted bool, scopedClient string) (*apiModels.QueryCampaign, error) {
	c, err := s.getByID(id)
	if err != nil {
		return nil, err
	}
	if !scopeAllows(c, unrestricted, scopedClient) {
		return nil, ErrQueryCampaignNotFound
	}
	s.attachPreview(c)
	return c, nil
}

func (s *QueryCampaignService) attachPreview(c *apiModels.QueryCampaign) {
	if c == nil {
		return
	}
	var p apiModels.QueryCampaignPreview
	if err := s.ReadDB.Where("CampaignId = ?", c.Id).First(&p).Error; err != nil {
		return
	}
	c.Preview = &p
}

func (s *QueryCampaignService) clearPreviewRow(campaignID int64) error {
	return s.WriteDB.Where("CampaignId = ?", campaignID).Delete(&apiModels.QueryCampaignPreview{}).Error
}

func normalizeCreate(req *apiModels.QueryCampaignCreateRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Client = strings.ToLower(strings.TrimSpace(req.Client))
	req.Channel = strings.ToUpper(strings.TrimSpace(req.Channel))
	req.DbTargetId = strings.TrimSpace(req.DbTargetId)
	req.SqlText = strings.TrimSpace(req.SqlText)
	if req.Name == "" {
		return validationErr("name is required")
	}
	if !sdkHelper.AllowedClient(req.Client) {
		return validationErr("client must be zapcash|wecredit")
	}
	switch req.Channel {
	case sdkHelper.ChannelSMS, sdkHelper.ChannelWhatsApp:
	default:
		return validationErr("channel must be SMS or WHATSAPP")
	}
	if !sdkHelper.DbTargetAllowedForClient(req.DbTargetId, req.Client) {
		return validationErr("dbTargetId is not allowed for client")
	}
	if err := sdkHelper.CheckSingleSelect(req.SqlText); err != nil {
		return validationErr(err.Error())
	}
	if req.QueryTimeoutSec < 1 {
		return validationErr("queryTimeoutSec must be positive")
	}
	if _, err := sdkHelper.ParseSchedule(req.ScheduleJson, req.QueryTimeoutSec); err != nil {
		return validationErr(err.Error())
	}
	return nil
}

func (s *QueryCampaignService) Create(req apiModels.QueryCampaignCreateRequest, actor string) (*apiModels.QueryCampaign, error) {
	if err := normalizeCreate(&req); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	c := apiModels.QueryCampaign{
		Name:            req.Name,
		Client:     req.Client,
		Channel:         req.Channel,
		DbTargetId:      req.DbTargetId,
		SqlText:         req.SqlText,
		QueryTimeoutSec: req.QueryTimeoutSec,
		ScheduleJson:    req.ScheduleJson,
		Status:          sdkHelper.StatusDraft,
		CreatedOn:       now,
		CreatedBy:       actor,
		UpdatedOn:       now,
		UpdatedBy:       actor,
	}
	if err := s.WriteDB.Create(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *QueryCampaignService) Update(id int64, req apiModels.QueryCampaignUpdateRequest, actor string, unrestricted bool, scopedClient string) (*apiModels.QueryCampaign, error) {
	c, err := s.Get(id, unrestricted, scopedClient)
	if err != nil {
		return nil, err
	}
	material := false
	if req.Name != nil {
		c.Name = strings.TrimSpace(*req.Name)
	}
	if req.Client != nil {
		c.Client = strings.ToLower(strings.TrimSpace(*req.Client))
		material = true
	}
	if req.Channel != nil {
		c.Channel = strings.ToUpper(strings.TrimSpace(*req.Channel))
		material = true
	}
	if req.DbTargetId != nil {
		c.DbTargetId = strings.TrimSpace(*req.DbTargetId)
		material = true
	}
	if req.SqlText != nil {
		c.SqlText = strings.TrimSpace(*req.SqlText)
		material = true
	}
	if req.QueryTimeoutSec != nil {
		c.QueryTimeoutSec = *req.QueryTimeoutSec
		material = true
	}
	if req.ScheduleJson != nil {
		c.ScheduleJson = *req.ScheduleJson
		material = true
	}
	createShape := apiModels.QueryCampaignCreateRequest{
		Name: c.Name, Client: c.Client, Channel: c.Channel,
		DbTargetId: c.DbTargetId, SqlText: c.SqlText, QueryTimeoutSec: c.QueryTimeoutSec,
		ScheduleJson: c.ScheduleJson,
	}
	if err := normalizeCreate(&createShape); err != nil {
		return nil, err
	}
	c.Name, c.Client, c.Channel = createShape.Name, createShape.Client, createShape.Channel
	c.DbTargetId, c.SqlText, c.QueryTimeoutSec = createShape.DbTargetId, createShape.SqlText, createShape.QueryTimeoutSec
	c.ScheduleJson = createShape.ScheduleJson
	if material {
		s.cancelPreviewJob(c.Id)
		c.Status = sdkHelper.StatusDraft
		c.ApprovedExpectedRows = nil
		c.Preview = nil
		if err := s.clearPreviewRow(c.Id); err != nil {
			return nil, err
		}
	}
	c.UpdatedBy = actor
	c.UpdatedOn = time.Now().UTC()
	if err := s.WriteDB.Omit("Preview").Save(c).Error; err != nil {
		return nil, err
	}
	return c, nil
}

type gormTemplateChecker struct {
	db *gorm.DB
}

func (t gormTemplateChecker) TemplateApproved(ctx context.Context, client, channel, process, templateReference string) (bool, error) {
	table := config.Configs.TemplateDetailsTable
	if strings.TrimSpace(table) == "" {
		table = "TemplateDetails"
	}
	var count int64
	err := t.db.WithContext(ctx).Table(table).
		Where("IsActive = 1 AND Client = ? AND Channel = ? AND Process = ? AND (TemplateName = ? OR CAST(DltTemplateId AS CHAR) = ?)",
			client, channel, process, templateReference, templateReference).
		Count(&count).Error
	return count > 0, err
}

func (s *QueryCampaignService) PreviewAsync(id int64, actor string, unrestricted bool, scopedClient string) (*apiModels.QueryCampaign, error) {
	c, err := s.Get(id, unrestricted, scopedClient)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if c.Preview != nil && strings.EqualFold(c.Preview.Status, sdkHelper.PreviewRunning) && c.Preview.StartedAt != nil {
		effective := time.Duration(c.QueryTimeoutSec)*time.Second + s.previewStaleBuffer()
		if max := time.Duration(s.maxQuerySeconds()) * time.Second; effective > max+s.previewStaleBuffer() {
			effective = max + s.previewStaleBuffer()
		}
		if now.Sub(*c.Preview.StartedAt) < effective {
			return nil, fmt.Errorf("%w: preview already running", ErrQueryCampaignConflict)
		}
		// Cancel the stuck SELECT before replacing the preview row.
		s.cancelPreviewJob(id)
		stale := "PREVIEW_STALE"
		_ = s.WriteDB.Model(&apiModels.QueryCampaignPreview{}).Where("CampaignId = ?", id).Updates(map[string]interface{}{
			"Status":       sdkHelper.PreviewFailed,
			"ErrorMessage": stale,
			"FinishedAt":   now,
			"UpdatedOn":    now,
		}).Error
	}
	preview := apiModels.QueryCampaignPreview{
		CampaignId:   id,
		Status:       sdkHelper.PreviewRunning,
		StartedAt:    &now,
		FinishedAt:   nil,
		TotalRows:    nil,
		DurationSec:  nil,
		ErrorMessage: nil,
		UpdatedOn:    now,
	}
	if err := s.WriteDB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "CampaignId"}},
		DoUpdates: clause.AssignmentColumns([]string{"Status", "StartedAt", "FinishedAt", "TotalRows", "DurationSec", "ErrorMessage", "UpdatedOn"}),
	}).Create(&preview).Error; err != nil {
		return nil, err
	}
	c.Preview = &preview
	c.UpdatedBy = actor
	c.UpdatedOn = now
	_ = s.WriteDB.Model(c).Updates(map[string]interface{}{
		"UpdatedOn": now,
		"UpdatedBy": actor,
	}).Error

	jobCtx, generation := s.beginPreviewJob(id, now)
	go s.runPreviewJob(id, generation, jobCtx, now)
	return c, nil
}

func (s *QueryCampaignService) runPreviewJob(id int64, generation uint64, jobCtx context.Context, jobStartedAt time.Time) {
	defer func() {
		if r := recover(); r != nil {
			utils.Error(fmt.Errorf("query campaign preview panic campaignId=%d: %v", id, r))
		}
		s.endPreviewJob(id, generation)
	}()
	var c apiModels.QueryCampaign
	if err := s.WriteDB.First(&c, id).Error; err != nil {
		return
	}
	db, err := s.ROPool(c.DbTargetId)
	started := time.Now()
	finish := func(status, errMsg string, res *sdkHelper.PreviewResult) {
		if !s.isCurrentPreviewJob(id, generation) {
			return
		}
		now := time.Now().UTC()
		row := apiModels.QueryCampaignPreview{
			CampaignId: id,
			Status:     status,
			FinishedAt: &now,
			UpdatedOn:  now,
		}
		if res != nil {
			sec := int(res.Duration.Seconds())
			total := res.SampleRows
			row.TotalRows = &total
			row.DurationSec = &sec
		} else {
			sec := int(time.Since(started).Seconds())
			row.DurationSec = &sec
		}
		if errMsg != "" {
			msg := truncate(errMsg, 1024)
			row.ErrorMessage = &msg
		}
		updates := map[string]interface{}{
			"Status":       row.Status,
			"FinishedAt":   row.FinishedAt,
			"TotalRows":    row.TotalRows,
			"DurationSec":  row.DurationSec,
			"ErrorMessage": row.ErrorMessage,
			"UpdatedOn":    now,
		}
		// StartedAt match prevents a cancelled/stale job from overwriting a newer preview.
		_ = s.WriteDB.Model(&apiModels.QueryCampaignPreview{}).
			Where("CampaignId = ? AND StartedAt = ?", id, jobStartedAt).
			Updates(updates).Error
	}
	if err != nil {
		finish(sdkHelper.PreviewFailed, err.Error(), nil)
		return
	}
	// Shared service pool — do not Close.

	cfg := sdkHelper.RunConfig{
		SqlText:         c.SqlText,
		DbTargetID:      c.DbTargetId,
		Client:          c.Client,
		Channel:         c.Channel,
		QueryTimeoutSec: c.QueryTimeoutSec,
		MaxQuerySeconds: s.maxQuerySeconds(),
		PreviewMaxRows:  sdkHelper.DefaultPreviewMaxRows,
		DB:              db,
		Templates:       gormTemplateChecker{db: s.ReadDB},
	}
	res, runErr := sdkHelper.RunPreview(jobCtx, cfg)
	if runErr != nil {
		errMsg := runErr.Error()
		switch {
		case errors.Is(runErr, context.Canceled):
			errMsg = "PREVIEW_CANCELLED"
		case errors.Is(runErr, context.DeadlineExceeded):
			errMsg = "PREVIEW_TIMEOUT"
		}
		finish(sdkHelper.PreviewFailed, errMsg, &res)
		return
	}
	finish(sdkHelper.PreviewDone, "", &res)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func (s *QueryCampaignService) Approve(id int64, req apiModels.QueryCampaignApproveRequest, actor string, unrestricted bool, scopedClient string) (*apiModels.QueryCampaign, error) {
	c, err := s.Get(id, unrestricted, scopedClient)
	if err != nil {
		return nil, err
	}
	status := strings.ToLower(c.Status)
	if status != sdkHelper.StatusDraft && status != sdkHelper.StatusPaused {
		return nil, validationErr("approve is only allowed from draft or paused")
	}
	if c.Preview == nil || !strings.EqualFold(c.Preview.Status, sdkHelper.PreviewDone) {
		return nil, validationErr("a successful preview is required before approve")
	}
	if c.Preview.ErrorMessage != nil && strings.TrimSpace(*c.Preview.ErrorMessage) != "" {
		return nil, validationErr("preview has an error; re-run preview")
	}
	if c.Preview.FinishedAt == nil || time.Since(*c.Preview.FinishedAt) > s.previewMaxAge() {
		return nil, validationErr("preview is missing or older than allowed max age; re-run preview")
	}
	if req.ApprovedExpectedRows < 1 {
		return nil, validationErr("approvedExpectedRows must be a positive integer")
	}
	now := time.Now().UTC()
	c.Status = sdkHelper.StatusApproved
	c.ApprovedAt = &now
	c.ApprovedBy = &actor
	c.ApprovedExpectedRows = &req.ApprovedExpectedRows
	c.UpdatedBy = actor
	c.UpdatedOn = now
	if err := s.WriteDB.Omit("Preview").Save(c).Error; err != nil {
		return nil, err
	}
	return c, nil
}

func (s *QueryCampaignService) Enable(id int64, actor string, unrestricted bool, scopedClient string) (*apiModels.QueryCampaign, error) {
	c, err := s.Get(id, unrestricted, scopedClient)
	if err != nil {
		return nil, err
	}
	status := strings.ToLower(c.Status)
	if status != sdkHelper.StatusApproved && status != sdkHelper.StatusPaused {
		return nil, validationErr("enable is only allowed from approved or paused")
	}
	sched, err := sdkHelper.ParseSchedule(c.ScheduleJson, c.QueryTimeoutSec)
	if err != nil {
		return nil, validationErr(err.Error())
	}
	nowIST := time.Now().In(sdkHelper.IST())
	if sdkHelper.OnceDateBeforeToday(sched, nowIST) {
		return nil, validationErr("onceDate is before today IST; cannot enable")
	}
	next, err := sdkHelper.NextFireAt(sched, nowIST)
	if err != nil {
		return nil, validationErr(err.Error())
	}
	now := time.Now().UTC()
	c.Status = sdkHelper.StatusEnabled
	c.EnabledAt = &now
	c.EnabledBy = &actor
	if !next.IsZero() {
		utc := next.UTC()
		c.NextFireAt = &utc
	} else {
		c.NextFireAt = nil
	}
	c.UpdatedBy = actor
	c.UpdatedOn = now
	if err := s.WriteDB.Omit("Preview").Save(c).Error; err != nil {
		return nil, err
	}
	return c, nil
}

func (s *QueryCampaignService) Pause(id int64, actor string, unrestricted bool, scopedClient string) (*apiModels.QueryCampaign, error) {
	c, err := s.Get(id, unrestricted, scopedClient)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(c.Status, sdkHelper.StatusEnabled) {
		return nil, validationErr("pause is only allowed from enabled")
	}
	now := time.Now().UTC()
	c.Status = sdkHelper.StatusPaused
	c.PausedAt = &now
	c.PausedBy = &actor
	c.UpdatedBy = actor
	c.UpdatedOn = now
	if err := s.WriteDB.Omit("Preview").Save(c).Error; err != nil {
		return nil, err
	}
	return c, nil
}

func (s *QueryCampaignService) ListRuns(campaignID int64, unrestricted bool, scopedClient string, page, pageSize int) ([]apiModels.QueryCampaignRun, int64, error) {
	camp, err := s.Get(campaignID, unrestricted, scopedClient)
	if err != nil {
		return nil, 0, err
	}
	q := s.ReadDB.Model(&apiModels.QueryCampaignRun{}).Where("CampaignId = ?", campaignID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	var runs []apiModels.QueryCampaignRun
	if err := q.Order("Id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&runs).Error; err != nil {
		return nil, 0, err
	}
	for i := range runs {
		counts, _ := s.ledgerStatusCounts(camp.Client, runs[i].Id)
		runs[i].LedgerStatusCounts = counts
	}
	return runs, total, nil
}

func (s *QueryCampaignService) GetRun(runID int64, unrestricted bool, scopedClient string) (*apiModels.QueryCampaignRun, error) {
	var run apiModels.QueryCampaignRun
	if err := s.ReadDB.First(&run, runID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrQueryCampaignNotFound
		}
		return nil, err
	}
	camp, err := s.Get(run.CampaignId, unrestricted, scopedClient)
	if err != nil {
		return nil, err
	}
	counts, err := s.ledgerStatusCounts(camp.Client, run.Id)
	if err != nil {
		return nil, err
	}
	run.LedgerStatusCounts = counts
	return &run, nil
}

func (s *QueryCampaignService) ledgerStatusCounts(client string, runID int64) (map[string]int64, error) {
	out := map[string]int64{
		sdkHelper.LedgerPending: 0, sdkHelper.LedgerProcessing: 0,
		sdkHelper.LedgerSending: 0, sdkHelper.LedgerSent: 0,
		sdkHelper.LedgerFailed: 0, sdkHelper.LedgerExpired: 0,
		sdkHelper.LedgerNotHit: 0,
	}
	ledgerDB, err := database.QueryCampaignLedgerDB(client)
	if err != nil {
		// ZapCash-first: WeCredit ledger not wired yet — return zeros without failing run detail.
		if strings.EqualFold(client, sdkHelper.ClientWeCredit) {
			return out, nil
		}
		return out, err
	}
	type row struct {
		Status string
		Cnt    int64
	}
	var rows []row
	err = ledgerDB.Table("CommQueryCampaignLedgerInput").
		Select("Status, COUNT(*) AS Cnt").
		Where("RunId = ?", runID).
		Group("Status").
		Scan(&rows).Error
	for _, r := range rows {
		out[strings.ToUpper(r.Status)] = r.Cnt
	}
	return out, err
}
