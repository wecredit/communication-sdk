package apiModels

import (
	"strconv"
	"time"
)

type TemplateAPIResponse struct {
	Success bool                    `json:"success"`
	Data    interface{}             `json:"data,omitempty"`
	Message string                  `json:"message,omitempty"`
	Meta    *TemplatePaginationMeta `json:"meta,omitempty"`
	Error   *TemplateAPIError       `json:"error,omitempty"`
}

type TemplateAPIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type TemplatePaginationMeta struct {
	Page            int   `json:"page"`
	PageSize        int   `json:"pageSize"`
	TotalItems      int64 `json:"totalItems"`
	TotalPages      int64 `json:"totalPages"`
	HasNextPage     bool  `json:"hasNextPage"`
	HasPreviousPage bool  `json:"hasPreviousPage"`
}

type TemplateListParams struct {
	Process  string
	Stage    string
	Client   string
	Channel  string
	Vendor   string
	Page     int
	PageSize int
}

// TemplateListItem contains only fields needed to identify and manage a
// template in list views. Payload content remains available from the detail API.
type TemplateListItem struct {
	Id            int        `gorm:"column:Id" json:"id"`
	Client        string     `gorm:"column:Client" json:"client"`
	Channel       string     `gorm:"column:Channel" json:"channel"`
	Process       string     `gorm:"column:Process" json:"process"`
	Stage         *string    `gorm:"column:Stage" json:"stage"`
	Vendor        string     `gorm:"column:Vendor" json:"vendor"`
	TemplateName  string     `gorm:"column:TemplateName" json:"templateName,omitempty"`
	DltTemplateId int64      `gorm:"column:DltTemplateId" json:"dltTemplateId,omitempty"`
	IsActive      bool       `gorm:"column:IsActive" json:"isActive"`
	CreatedOn     time.Time `gorm:"column:CreatedOn" json:"createdOn"`
	UpdatedOn     time.Time `gorm:"column:UpdatedOn" json:"updatedOn"`
	CreatedBy     string    `gorm:"column:CreatedBy" json:"createdBy,omitempty"`
	UpdatedBy     string    `gorm:"column:UpdatedBy" json:"updatedBy,omitempty"`
}

type TemplateListResult struct {
	Items      []TemplateListItem
	TotalItems int64
}

// TemplateDetailsResponse keeps the complete template payload while exposing
// Stage as a canonical decimal string so JSON does not collapse 2.10 to 2.1.
type TemplateDetailsResponse struct {
	Templatedetails
	Stage *string `json:"stage"`
}

func NewTemplateDetailsResponse(template *Templatedetails) TemplateDetailsResponse {
	response := TemplateDetailsResponse{Templatedetails: *template}
	if template.Stage != nil {
		stage := strconv.FormatFloat(*template.Stage, 'f', 2, 64)
		response.Stage = &stage
	}
	// Legacy rows may have NULL UpdatedOn; expose CreatedOn so UI sorting/display works.
	if response.UpdatedOn == nil && !response.CreatedOn.IsZero() {
		createdOn := response.CreatedOn
		response.UpdatedOn = &createdOn
	}
	return response
}
