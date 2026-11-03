package apiModels

import (
	"strings"
	"time"
)

type StageMappingInput struct {
	SubStage int `json:"subStage"`
}

type StageConfigurationRequest struct {
	LenderName     string              `json:"lenderName"`
	CommType       string              `json:"commType"`
	Stage          int                 `json:"stage"`
	Interval       *string             `json:"interval,omitempty"`
	TemplateStages []StageMappingInput `json:"templateStages,omitempty"`
	// ExecutionTimes is ZapCash-only. Nil omits/preserves times on update.
	// Non-nil replaces the three columns after sort + de-dupe + compact (max 3, HH:MM).
	ExecutionTimes *[]string `json:"executionTimes,omitempty"`
}

type LenderSchedule struct {
	ID             int       `gorm:"column:Id" json:"id"`
	LenderName     string    `gorm:"column:LenderName" json:"lenderName"`
	CommType       string    `gorm:"column:CommType" json:"commType"`
	Stage          int       `gorm:"column:Stage" json:"stage"`
	Interval       string    `gorm:"column:Interval" json:"interval"`
	// Always emit for frontend shape: null when unset, HH:MM when set.
	ExecutionTime1 *string  `gorm:"column:ExecutionTime1" json:"executionTime1"`
	ExecutionTime2 *string  `gorm:"column:ExecutionTime2" json:"executionTime2"`
	ExecutionTime3 *string  `gorm:"column:ExecutionTime3" json:"executionTime3"`
	ExecutionTimes []string `gorm:"-" json:"executionTimes"`
	CreatedOn      time.Time `gorm:"column:CreatedOn" json:"createdOn"`
	UpdatedOn      time.Time `gorm:"column:UpdatedOn" json:"updatedOn"`
	CreatedBy      string    `gorm:"column:CreatedBy" json:"createdBy,omitempty"`
	UpdatedBy      string    `gorm:"column:UpdatedBy" json:"updatedBy,omitempty"`
}

// FillExecutionTimes normalizes TIME columns to HH:MM and always sets
// executionTimes (empty slice when all null) so list/get JSON keeps a stable shape.
func (s *LenderSchedule) FillExecutionTimes() {
	if s == nil {
		return
	}

	normalizePtr := func(raw *string) *string {
		if raw == nil {
			return nil
		}
		trimmed := formatExecutionTimeHHMM(*raw)
		if trimmed == "" {
			return nil
		}
		v := trimmed
		return &v
	}

	s.ExecutionTime1 = normalizePtr(s.ExecutionTime1)
	s.ExecutionTime2 = normalizePtr(s.ExecutionTime2)
	s.ExecutionTime3 = normalizePtr(s.ExecutionTime3)

	out := make([]string, 0, 3)
	for _, raw := range []*string{s.ExecutionTime1, s.ExecutionTime2, s.ExecutionTime3} {
		if raw == nil {
			continue
		}
		out = append(out, *raw)
	}
	s.ExecutionTimes = out
}

func formatExecutionTimeHHMM(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// MySQL TIME may come back as "15:04:05" or "15:04:05.000000".
	if len(raw) >= 5 && raw[2] == ':' {
		return raw[:5]
	}
	return raw
}

// FormatExecutionTimeForCompare returns HH:MM for equality checks.
func FormatExecutionTimeForCompare(raw string) string {
	return formatExecutionTimeHHMM(raw)
}

type StageMapping struct {
	ID         int       `gorm:"column:Id" json:"id"`
	LenderName string    `gorm:"column:LenderName" json:"lenderName"`
	CommType   string    `gorm:"column:CommType" json:"commType"`
	Stage      int       `gorm:"column:Stage" json:"stage"`
	SubStage   int       `gorm:"column:SubStage" json:"subStage"`
	CreatedOn  time.Time `gorm:"column:CreatedOn" json:"createdOn"`
	UpdatedOn  time.Time `gorm:"column:UpdatedOn" json:"updatedOn"`
	CreatedBy  string    `gorm:"column:CreatedBy" json:"createdBy,omitempty"`
	UpdatedBy  string    `gorm:"column:UpdatedBy" json:"updatedBy,omitempty"`
}

type StageConfigurationResponse struct {
	LenderSchedule *LenderSchedule `json:"lenderSchedule,omitempty"`
	TemplateStages []StageMapping  `json:"templateStages,omitempty"`
}

type StageConfigurationListParams struct {
	LenderName string
	CommType   string
	Stage      *int
	SubStage   *int
	SortBy     string
	SortDir    string
	Page       int
	PageSize   int
}

type LenderScheduleListResult struct {
	Items      []LenderSchedule
	TotalItems int64
}

type StageMappingListResult struct {
	Items      []StageMapping
	TotalItems int64
}
