package apiServices

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/configurationcache"
	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	internalredis "github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrConfigurationNotFound      = errors.New("stage configuration not found")
	ErrConfigurationAlreadyExists = errors.New("stage configuration already exists")
	ErrConfigurationStale         = errors.New("stage configuration changed while acquiring locks")
	ErrConfigurationInUse         = errors.New("stage configuration is referenced by a template")
	ErrConfigurationValidation    = errors.New("stage configuration validation failed")
)

const maxWholeStage = 99999999

type StageConfigurationService struct {
	ReadDB  *gorm.DB
	WriteDB *gorm.DB
}

func NewStageConfigurationService(readDB, writeDB *gorm.DB) *StageConfigurationService {
	return &StageConfigurationService{ReadDB: readDB, WriteDB: writeDB}
}

func NormalizeStageConfigurationRequest(request *apiModels.StageConfigurationRequest) error {
	request.LenderName = strings.ToLower(strings.TrimSpace(request.LenderName))
	request.CommType = strings.ToUpper(strings.TrimSpace(request.CommType))
	if request.LenderName == "" {
		return errors.New("lenderName is required")
	}
	switch request.CommType {
	case "SMS", "RCS", "WHATSAPP", "EMAIL":
	default:
		return fmt.Errorf("unsupported commType %q", request.CommType)
	}
	if request.Stage < 0 || request.Stage > maxWholeStage {
		return fmt.Errorf("stage must be an integer between 0 and %d", maxWholeStage)
	}
	if request.Interval == nil && request.TemplateStages == nil {
		return errors.New("at least one of interval or templateStages is required")
	}
	seenSubStages := make(map[int]struct{}, len(request.TemplateStages))
	for _, mapping := range request.TemplateStages {
		if mapping.SubStage < 1 || mapping.SubStage > 99 {
			return errors.New("subStage must be an integer between 1 and 99")
		}
		if _, duplicate := seenSubStages[mapping.SubStage]; duplicate {
			return fmt.Errorf("duplicate subStage %d", mapping.SubStage)
		}
		seenSubStages[mapping.SubStage] = struct{}{}
	}
	sort.Slice(request.TemplateStages, func(i, j int) bool {
		return request.TemplateStages[i].SubStage < request.TemplateStages[j].SubStage
	})
	if request.Interval != nil {
		interval, err := normalizeStageIntervals(*request.Interval)
		if err != nil {
			return err
		}
		request.Interval = &interval
	}
	return nil
}

func normalizeStageIntervals(raw string) (string, error) {
	parts := strings.Split(raw, ";")
	if len(parts) == 0 || strings.TrimSpace(raw) == "" {
		return "", errors.New("interval must contain at least one token")
	}
	weekdays := map[string]struct{}{
		"monday": {}, "tuesday": {}, "wednesday": {}, "thursday": {},
		"friday": {}, "saturday": {}, "sunday": {},
	}
	seen := make(map[string]struct{}, len(parts))
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		token := strings.ToLower(strings.TrimSpace(part))
		if token == "" {
			return "", errors.New("interval cannot contain empty tokens")
		}
		if _, duplicate := seen[token]; duplicate {
			return "", fmt.Errorf("duplicate interval token %q", token)
		}
		seen[token] = struct{}{}
		if _, weekday := weekdays[token]; !weekday {
			if len(token) < 2 || !strings.ContainsRune("dhm", rune(token[len(token)-1])) {
				return "", fmt.Errorf("invalid interval token %q", token)
			}
			if _, err := strconv.Atoi(token[:len(token)-1]); err != nil {
				return "", fmt.Errorf("invalid interval token %q", token)
			}
		}
		normalized = append(normalized, token)
	}
	return strings.Join(normalized, ";"), nil
}

func scheduleFromRequest(request apiModels.StageConfigurationRequest) apiModels.LenderSchedule {
	interval := ""
	if request.Interval != nil {
		interval = *request.Interval
	}
	return apiModels.LenderSchedule{LenderName: request.LenderName, CommType: request.CommType, Stage: request.Stage, Interval: interval}
}

func mappingsFromRequest(request apiModels.StageConfigurationRequest) []apiModels.StageMapping {
	mappings := make([]apiModels.StageMapping, 0, len(request.TemplateStages))
	for _, input := range request.TemplateStages {
		mappings = append(mappings, apiModels.StageMapping{
			LenderName: request.LenderName, CommType: request.CommType, Stage: request.Stage, SubStage: input.SubStage,
		})
	}
	return mappings
}

func sameScheduleIdentity(a, b apiModels.LenderSchedule) bool {
	return a.LenderName == b.LenderName && a.CommType == b.CommType && a.Stage == b.Stage
}

func (s *StageConfigurationService) Create(request apiModels.StageConfigurationRequest) (*apiModels.StageConfigurationResponse, error) {
	if err := NormalizeStageConfigurationRequest(&request); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConfigurationValidation, err)
	}
	scheduleRequested := request.Interval != nil
	mappingsRequested := len(request.TemplateStages) > 0
	if !scheduleRequested && !mappingsRequested {
		return nil, fmt.Errorf("%w: templateStages must contain at least one mapping when interval is omitted", ErrConfigurationValidation)
	}
	schedule := scheduleFromRequest(request)
	mappings := mappingsFromRequest(request)
	var versions configurationcache.StageConfigurationVersions
	err := s.WriteDB.Connection(func(conn *gorm.DB) error {
		locks, err := acquireStageConfigurationLocks(conn, stageConfigurationLockIdentity(schedule.LenderName, schedule.CommType, schedule.Stage))
		if err != nil {
			return err
		}
		defer releaseStageConfigurationLocks(conn, locks)
		return conn.Transaction(func(tx *gorm.DB) error {
			if scheduleRequested {
				if err := ensureScheduleIdentityAvailable(tx, schedule, 0); err != nil {
					return err
				}
			}
			if mappingsRequested {
				if err := ensureDesiredMappingsAvailable(tx, mappings); err != nil {
					return err
				}
			}
			if scheduleRequested {
				if err := tx.Table(config.Configs.LenderStagesTable).Create(&schedule).Error; err != nil {
					return fmt.Errorf("create lender schedule: %w", err)
				}
			}
			if mappingsRequested {
				if err := tx.Table(config.Configs.TemplateStageTable).Create(&mappings).Error; err != nil {
					return fmt.Errorf("create stage mappings: %w", err)
				}
			}
			versions, err = configurationcache.IncrementStageConfigurationVersions(tx, config.Configs.ConfigurationVersionTable, scheduleRequested, mappingsRequested)
			return err
		})
	})
	if err != nil {
		return nil, err
	}
	publishStageConfigurationInvalidation(versions)
	response := &apiModels.StageConfigurationResponse{TemplateStages: mappings}
	if scheduleRequested {
		response.LenderSchedule = &schedule
	}
	return response, nil
}

func (s *StageConfigurationService) Update(id int, request apiModels.StageConfigurationRequest) (*apiModels.StageConfigurationResponse, error) {
	if err := NormalizeStageConfigurationRequest(&request); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConfigurationValidation, err)
	}
	var discovered apiModels.LenderSchedule
	if err := s.WriteDB.Table(config.Configs.LenderStagesTable).Where("Id = ?", id).First(&discovered).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrConfigurationNotFound
		}
		return nil, fmt.Errorf("discover lender schedule: %w", err)
	}
	target := scheduleFromRequest(request)
	if request.Interval == nil {
		target.Interval = discovered.Interval
	}
	target.ID = id
	mappingsRequested := request.TemplateStages != nil
	var savedMappings []apiModels.StageMapping
	var versions configurationcache.StageConfigurationVersions
	var scheduleChanged, mappingsChanged bool
	err := s.WriteDB.Connection(func(conn *gorm.DB) error {
		identities := []string{
			stageConfigurationLockIdentity(discovered.LenderName, discovered.CommType, discovered.Stage),
			stageConfigurationLockIdentity(target.LenderName, target.CommType, target.Stage),
		}
		locks, err := acquireStageConfigurationLocks(conn, identities...)
		if err != nil {
			return err
		}
		defer releaseStageConfigurationLocks(conn, locks)
		return conn.Transaction(func(tx *gorm.DB) error {
			var current apiModels.LenderSchedule
			if err := tx.Table(config.Configs.LenderStagesTable).Clauses(clause.Locking{Strength: "UPDATE"}).Where("Id = ?", id).First(&current).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrConfigurationNotFound
				}
				return err
			}
			if !sameScheduleIdentity(current, discovered) {
				return ErrConfigurationStale
			}
			if err := ensureScheduleIdentityAvailable(tx, target, id); err != nil {
				return err
			}
			if mappingsRequested && !sameScheduleIdentity(current, target) {
				if err := ensureMappingIdentityAvailable(tx, target); err != nil {
					return err
				}
			}
			var existing []apiModels.StageMapping
			if mappingsRequested {
				if err := tx.Table(config.Configs.TemplateStageTable).Clauses(clause.Locking{Strength: "UPDATE"}).
					Where("LenderName = ? AND CommType = ? AND Stage = ?", current.LenderName, current.CommType, current.Stage).
					Order("SubStage ASC, Id ASC").Find(&existing).Error; err != nil {
					return err
				}
			}
			desired := mappingsFromRequest(request)
			existingBySub := make(map[int]apiModels.StageMapping, len(existing))
			for _, mapping := range existing {
				existingBySub[mapping.SubStage] = mapping
			}
			desiredSubs := make(map[int]struct{}, len(desired))
			for i := range desired {
				desiredSubs[desired[i].SubStage] = struct{}{}
				if mapping, ok := existingBySub[desired[i].SubStage]; ok && sameScheduleIdentity(current, target) {
					desired[i].ID = mapping.ID
				}
			}
			if mappingsRequested {
				for _, mapping := range existing {
					if _, keep := desiredSubs[mapping.SubStage]; keep && sameScheduleIdentity(current, target) {
						continue
					}
					if err := ensureMappingUnused(tx, mapping); err != nil {
						return err
					}
				}
			}

			scheduleChanged = current.LenderName != target.LenderName || current.CommType != target.CommType || current.Stage != target.Stage || current.Interval != target.Interval
			mappingsChanged = mappingsRequested && !sameMappingSet(existing, desired, sameScheduleIdentity(current, target))
			if scheduleChanged {
				if err := tx.Table(config.Configs.LenderStagesTable).Where("Id = ?", id).Updates(map[string]interface{}{
					"LenderName": target.LenderName, "CommType": target.CommType, "Stage": target.Stage, "Interval": target.Interval,
				}).Error; err != nil {
					return fmt.Errorf("update lender schedule: %w", err)
				}
			}
			if mappingsChanged {
				deleteIDs := make([]int, 0, len(existing))
				for _, mapping := range existing {
					_, keep := desiredSubs[mapping.SubStage]
					if !keep || !sameScheduleIdentity(current, target) {
						deleteIDs = append(deleteIDs, mapping.ID)
					}
				}
				if len(deleteIDs) > 0 {
					if err := tx.Table(config.Configs.TemplateStageTable).Where("Id IN ?", deleteIDs).Delete(&apiModels.StageMapping{}).Error; err != nil {
						return err
					}
				}
				toCreate := make([]apiModels.StageMapping, 0, len(desired))
				for i := range desired {
					if desired[i].ID == 0 {
						toCreate = append(toCreate, desired[i])
					}
				}
				if len(toCreate) > 0 {
					if err := tx.Table(config.Configs.TemplateStageTable).Create(&toCreate).Error; err != nil {
						return err
					}
					createdBySub := make(map[int]int, len(toCreate))
					for _, mapping := range toCreate {
						createdBySub[mapping.SubStage] = mapping.ID
					}
					for i := range desired {
						if desired[i].ID == 0 {
							desired[i].ID = createdBySub[desired[i].SubStage]
						}
					}
				}
			}
			savedMappings = desired
			if !mappingsChanged {
				savedMappings = existing
			}
			target.ID = id
			versions, err = configurationcache.IncrementStageConfigurationVersions(tx, config.Configs.ConfigurationVersionTable, scheduleChanged, mappingsChanged)
			return err
		})
	})
	if err != nil {
		return nil, err
	}
	if scheduleChanged || mappingsChanged {
		publishStageConfigurationInvalidation(versions)
	}
	if !mappingsRequested {
		if err := s.WriteDB.Table(config.Configs.TemplateStageTable).
			Where("LenderName = ? AND CommType = ? AND Stage = ?", target.LenderName, target.CommType, target.Stage).
			Order("SubStage ASC, Id ASC").Find(&savedMappings).Error; err != nil {
			return nil, err
		}
	}
	return &apiModels.StageConfigurationResponse{LenderSchedule: &target, TemplateStages: savedMappings}, nil
}

func sameMappingSet(existing, desired []apiModels.StageMapping, sameIdentity bool) bool {
	if !sameIdentity || len(existing) != len(desired) {
		return false
	}
	set := make(map[int]struct{}, len(existing))
	for _, mapping := range existing {
		set[mapping.SubStage] = struct{}{}
	}
	for _, mapping := range desired {
		if _, ok := set[mapping.SubStage]; !ok {
			return false
		}
	}
	return true
}

func ensureScheduleIdentityAvailable(tx *gorm.DB, schedule apiModels.LenderSchedule, excludeID int) error {
	query := tx.Table(config.Configs.LenderStagesTable).Where("LenderName = ? AND CommType = ? AND Stage = ?", schedule.LenderName, schedule.CommType, schedule.Stage)
	if excludeID > 0 {
		query = query.Where("Id <> ?", excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrConfigurationAlreadyExists
	}
	return nil
}

func ensureMappingIdentityAvailable(tx *gorm.DB, schedule apiModels.LenderSchedule) error {
	var count int64
	if err := tx.Table(config.Configs.TemplateStageTable).
		Where("LenderName = ? AND CommType = ? AND Stage = ?", schedule.LenderName, schedule.CommType, schedule.Stage).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrConfigurationAlreadyExists
	}
	return nil
}

func ensureDesiredMappingsAvailable(tx *gorm.DB, mappings []apiModels.StageMapping) error {
	if len(mappings) == 0 {
		return nil
	}
	subStages := make([]int, 0, len(mappings))
	for _, mapping := range mappings {
		subStages = append(subStages, mapping.SubStage)
	}
	identity := mappings[0]
	var count int64
	if err := tx.Table(config.Configs.TemplateStageTable).
		Where("LenderName = ? AND CommType = ? AND Stage = ? AND SubStage IN ?", identity.LenderName, identity.CommType, identity.Stage, subStages).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrConfigurationAlreadyExists
	}
	return nil
}

func ensureScheduleUnused(tx *gorm.DB, schedule apiModels.LenderSchedule) error {
	var count int64
	err := tx.Table(config.Configs.TemplateDetailsTable).
		Where("Stage IS NOT NULL AND Process = ? AND Channel = ? AND Stage >= ? AND Stage < ?", strings.ToUpper(schedule.LenderName), schedule.CommType, schedule.Stage, schedule.Stage+1).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrConfigurationInUse
	}
	return nil
}

func ensureMappingUnused(tx *gorm.DB, mapping apiModels.StageMapping) error {
	derived := fmt.Sprintf("%d.%02d", mapping.Stage, mapping.SubStage)
	var count int64
	err := tx.Table(config.Configs.TemplateDetailsTable).
		Where("Stage IS NOT NULL AND Process = ? AND Channel = ? AND Stage = CAST(? AS DECIMAL(10,2))", strings.ToUpper(mapping.LenderName), mapping.CommType, derived).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrConfigurationInUse
	}
	return nil
}

func (s *StageConfigurationService) GetLenderSchedules(params apiModels.StageConfigurationListParams) (*apiModels.LenderScheduleListResult, error) {
	query := applyStageConfigurationFilters(s.ReadDB.Table(config.Configs.LenderStagesTable), params, false)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	items := make([]apiModels.LenderSchedule, 0, params.PageSize)
	if err := query.Order("LenderName ASC, CommType ASC, Stage ASC, Id ASC").Limit(params.PageSize).Offset((params.Page - 1) * params.PageSize).Find(&items).Error; err != nil {
		return nil, err
	}
	return &apiModels.LenderScheduleListResult{Items: items, TotalItems: total}, nil
}

func (s *StageConfigurationService) GetLenderSchedule(id int) (*apiModels.StageConfigurationResponse, error) {
	var schedule apiModels.LenderSchedule
	if err := s.ReadDB.Table(config.Configs.LenderStagesTable).Where("Id = ?", id).First(&schedule).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrConfigurationNotFound
		}
		return nil, err
	}
	var mappings []apiModels.StageMapping
	if err := s.ReadDB.Table(config.Configs.TemplateStageTable).Where("LenderName = ? AND CommType = ? AND Stage = ?", schedule.LenderName, schedule.CommType, schedule.Stage).Order("SubStage ASC, Id ASC").Find(&mappings).Error; err != nil {
		return nil, err
	}
	return &apiModels.StageConfigurationResponse{LenderSchedule: &schedule, TemplateStages: mappings}, nil
}

func (s *StageConfigurationService) GetStageMappings(params apiModels.StageConfigurationListParams) (*apiModels.StageMappingListResult, error) {
	query := applyStageConfigurationFilters(s.ReadDB.Table(config.Configs.TemplateStageTable), params, true)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	items := make([]apiModels.StageMapping, 0, params.PageSize)
	if err := query.Order("LenderName ASC, CommType ASC, Stage ASC, SubStage ASC, Id ASC").Limit(params.PageSize).Offset((params.Page - 1) * params.PageSize).Find(&items).Error; err != nil {
		return nil, err
	}
	return &apiModels.StageMappingListResult{Items: items, TotalItems: total}, nil
}

func applyStageConfigurationFilters(query *gorm.DB, params apiModels.StageConfigurationListParams, includeSubStage bool) *gorm.DB {
	if params.LenderName != "" {
		query = query.Where("LenderName = ?", strings.ToLower(strings.TrimSpace(params.LenderName)))
	}
	if params.CommType != "" {
		query = query.Where("CommType = ?", strings.ToUpper(strings.TrimSpace(params.CommType)))
	}
	if params.Stage != nil {
		query = query.Where("Stage = ?", *params.Stage)
	}
	if includeSubStage && params.SubStage != nil {
		query = query.Where("SubStage = ?", *params.SubStage)
	}
	return query
}

func (s *StageConfigurationService) DeleteLenderSchedule(id int) error {
	var discovered apiModels.LenderSchedule
	if err := s.WriteDB.Table(config.Configs.LenderStagesTable).Where("Id = ?", id).First(&discovered).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrConfigurationNotFound
		}
		return err
	}
	var versions configurationcache.StageConfigurationVersions
	err := s.WriteDB.Connection(func(conn *gorm.DB) error {
		locks, err := acquireStageConfigurationLocks(conn, stageConfigurationLockIdentity(discovered.LenderName, discovered.CommType, discovered.Stage))
		if err != nil {
			return err
		}
		defer releaseStageConfigurationLocks(conn, locks)
		return conn.Transaction(func(tx *gorm.DB) error {
			var current apiModels.LenderSchedule
			if err := tx.Table(config.Configs.LenderStagesTable).Clauses(clause.Locking{Strength: "UPDATE"}).Where("Id = ?", id).First(&current).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrConfigurationNotFound
				}
				return err
			}
			if !sameScheduleIdentity(current, discovered) {
				return ErrConfigurationStale
			}
			if err := tx.Table(config.Configs.LenderStagesTable).Where("Id = ?", id).Delete(&apiModels.LenderSchedule{}).Error; err != nil {
				return err
			}
			versions, err = configurationcache.IncrementStageConfigurationVersions(tx, config.Configs.ConfigurationVersionTable, true, false)
			return err
		})
	})
	if err != nil {
		return err
	}
	publishStageConfigurationInvalidation(versions)
	return nil
}

func (s *StageConfigurationService) DeleteStageMapping(id int) error {
	var discovered apiModels.StageMapping
	if err := s.WriteDB.Table(config.Configs.TemplateStageTable).Where("Id = ?", id).First(&discovered).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrConfigurationNotFound
		}
		return err
	}
	var versions configurationcache.StageConfigurationVersions
	err := s.WriteDB.Connection(func(conn *gorm.DB) error {
		locks, err := acquireStageConfigurationLocks(conn, stageConfigurationLockIdentity(discovered.LenderName, discovered.CommType, discovered.Stage))
		if err != nil {
			return err
		}
		defer releaseStageConfigurationLocks(conn, locks)
		return conn.Transaction(func(tx *gorm.DB) error {
			var current apiModels.StageMapping
			if err := tx.Table(config.Configs.TemplateStageTable).Clauses(clause.Locking{Strength: "UPDATE"}).Where("Id = ?", id).First(&current).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrConfigurationNotFound
				}
				return err
			}
			if current.LenderName != discovered.LenderName || current.CommType != discovered.CommType || current.Stage != discovered.Stage {
				return ErrConfigurationStale
			}
			if err := ensureMappingUnused(tx, current); err != nil {
				return err
			}
			if err := tx.Table(config.Configs.TemplateStageTable).Where("Id = ?", id).Delete(&apiModels.StageMapping{}).Error; err != nil {
				return err
			}
			versions, err = configurationcache.IncrementStageConfigurationVersions(tx, config.Configs.ConfigurationVersionTable, false, true)
			return err
		})
	})
	if err != nil {
		return err
	}
	publishStageConfigurationInvalidation(versions)
	return nil
}

func publishStageConfigurationInvalidation(versions configurationcache.StageConfigurationVersions) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := configurationcache.PublishStageConfigurationInvalidation(ctx, internalredis.RDB, config.Configs.Environment, versions); err != nil {
		utils.Error(err)
	}
}
