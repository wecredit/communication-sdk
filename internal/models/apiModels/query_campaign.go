package apiModels

import (
	"encoding/json"
	"time"
)

// QueryCampaign is the CommQueryCampaign admin API model.
type QueryCampaign struct {
	Id                   int64           `json:"id" gorm:"column:Id;primaryKey"`
	Name                 string          `json:"name" gorm:"column:Name"`
	Client               string          `json:"client" gorm:"column:Client"`
	Channel              string          `json:"channel" gorm:"column:Channel"`
	DbTargetId           string          `json:"dbTargetId" gorm:"column:DbTargetId"`
	SqlText              string          `json:"sqlText" gorm:"column:SqlText"`
	QueryTimeoutSec      int             `json:"queryTimeoutSec" gorm:"column:QueryTimeoutSec"`
	ScheduleJson         json.RawMessage `json:"scheduleJson" gorm:"column:ScheduleJson;type:json"`
	Status               string          `json:"status" gorm:"column:Status"`
	ApprovedAt           *time.Time      `json:"approvedAt,omitempty" gorm:"column:ApprovedAt"`
	ApprovedBy           *string         `json:"approvedBy,omitempty" gorm:"column:ApprovedBy"`
	EnabledAt            *time.Time      `json:"enabledAt,omitempty" gorm:"column:EnabledAt"`
	EnabledBy            *string         `json:"enabledBy,omitempty" gorm:"column:EnabledBy"`
	PausedAt             *time.Time      `json:"pausedAt,omitempty" gorm:"column:PausedAt"`
	PausedBy             *string         `json:"pausedBy,omitempty" gorm:"column:PausedBy"`
	ApprovedExpectedRows *int            `json:"approvedExpectedRows,omitempty" gorm:"column:ApprovedExpectedRows"`
	LastRunAt            *time.Time      `json:"lastRunAt,omitempty" gorm:"column:LastRunAt"`
	NextFireAt           *time.Time      `json:"nextFireAt,omitempty" gorm:"column:NextFireAt"`
	ManualRunRequestedAt *time.Time      `json:"manualRunRequestedAt,omitempty" gorm:"column:ManualRunRequestedAt"`
	ManualRunRequestedBy *string         `json:"manualRunRequestedBy,omitempty" gorm:"column:ManualRunRequestedBy"`
	CreatedOn            time.Time       `json:"createdOn" gorm:"column:CreatedOn"`
	CreatedBy            string          `json:"createdBy" gorm:"column:CreatedBy"`
	UpdatedOn            time.Time       `json:"updatedOn" gorm:"column:UpdatedOn"`
	UpdatedBy            string          `json:"updatedBy" gorm:"column:UpdatedBy"`
	// Preview is the latest CommQueryCampaignPreview row (not a campaign column).
	Preview *QueryCampaignPreview `json:"preview,omitempty" gorm:"-"`
}

func (QueryCampaign) TableName() string { return "CommQueryCampaign" }

// QueryCampaignPreview is the latest-only preview row (Refresh overwrites).
// TotalRows is the preview sample size (≤100), not full SELECT cardinality.
type QueryCampaignPreview struct {
	CampaignId   int64      `json:"campaignId" gorm:"column:CampaignId;primaryKey"`
	Status       string     `json:"status" gorm:"column:Status"`
	StartedAt    *time.Time `json:"startedAt,omitempty" gorm:"column:StartedAt"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty" gorm:"column:FinishedAt"`
	TotalRows    *int       `json:"totalRows,omitempty" gorm:"column:TotalRows"`
	DurationSec  *int       `json:"durationSec,omitempty" gorm:"column:DurationSec"`
	ErrorMessage *string    `json:"errorMessage,omitempty" gorm:"column:ErrorMessage"`
	UpdatedOn    time.Time  `json:"updatedOn" gorm:"column:UpdatedOn"`
}

func (QueryCampaignPreview) TableName() string { return "CommQueryCampaignPreview" }

type QueryCampaignCreateRequest struct {
	Name            string          `json:"name"`
	Client          string          `json:"client"`
	Channel         string          `json:"channel"`
	DbTargetId      string          `json:"dbTargetId"`
	SqlText         string          `json:"sqlText"`
	QueryTimeoutSec int             `json:"queryTimeoutSec"`
	ScheduleJson    json.RawMessage `json:"scheduleJson"`
}

type QueryCampaignUpdateRequest struct {
	Name            *string          `json:"name"`
	Client          *string          `json:"client"`
	Channel         *string          `json:"channel"`
	DbTargetId      *string          `json:"dbTargetId"`
	SqlText         *string          `json:"sqlText"`
	QueryTimeoutSec *int             `json:"queryTimeoutSec"`
	ScheduleJson    *json.RawMessage `json:"scheduleJson"`
}

type QueryCampaignApproveRequest struct {
	ApprovedExpectedRows int `json:"approvedExpectedRows"`
}

type QueryCampaignRun struct {
	Id                 int64           `json:"id" gorm:"column:Id;primaryKey"`
	CampaignId         int64           `json:"campaignId" gorm:"column:CampaignId"`
	TriggeredBy        string          `json:"triggeredBy" gorm:"column:TriggeredBy"`
	Status             string          `json:"status" gorm:"column:Status"`
	StartedAt          time.Time       `json:"startedAt" gorm:"column:StartedAt"`
	FinishedAt         *time.Time      `json:"finishedAt,omitempty" gorm:"column:FinishedAt"`
	ScheduledAt        *time.Time      `json:"scheduledAt,omitempty" gorm:"column:ScheduledAt"`
	SqlHash            *string         `json:"sqlHash,omitempty" gorm:"column:SqlHash"`
	RowsSelected       *int            `json:"rowsSelected,omitempty" gorm:"column:RowsSelected"`
	RowsInserted       *int            `json:"rowsInserted,omitempty" gorm:"column:RowsInserted"`
	RowsSkippedInvalid *int            `json:"rowsSkippedInvalid,omitempty" gorm:"column:RowsSkippedInvalid"`
	ErrorMessage       *string         `json:"errorMessage,omitempty" gorm:"column:ErrorMessage"`
	LedgerStatusCounts map[string]int64 `json:"ledgerStatusCounts,omitempty" gorm:"-"`
}

func (QueryCampaignRun) TableName() string { return "CommQueryCampaignRun" }

type QueryCampaignListParams struct {
	Client       string
	Channel      string
	Status       string
	Unrestricted bool
	ScopedClient string
	Page         int
	PageSize     int
}
