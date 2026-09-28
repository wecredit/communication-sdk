package apiModels

import "time"

type StageMappingInput struct {
	SubStage int `json:"subStage"`
}

type StageConfigurationRequest struct {
	LenderName     string              `json:"lenderName"`
	CommType       string              `json:"commType"`
	Stage          int                 `json:"stage"`
	Interval       *string             `json:"interval,omitempty"`
	TemplateStages []StageMappingInput `json:"templateStages,omitempty"`
}

type LenderSchedule struct {
	ID         int       `gorm:"column:Id" json:"id"`
	LenderName string    `gorm:"column:LenderName" json:"lenderName"`
	CommType   string    `gorm:"column:CommType" json:"commType"`
	Stage      int       `gorm:"column:Stage" json:"stage"`
	Interval   string    `gorm:"column:Interval" json:"interval"`
	CreatedOn  time.Time `gorm:"column:CreatedOn" json:"createdOn"`
	UpdatedOn  time.Time `gorm:"column:UpdatedOn" json:"updatedOn"`
	CreatedBy  string    `gorm:"column:CreatedBy" json:"createdBy,omitempty"`
	UpdatedBy  string    `gorm:"column:UpdatedBy" json:"updatedBy,omitempty"`
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
