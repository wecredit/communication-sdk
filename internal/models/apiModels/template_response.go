package apiModels

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

type TemplateListResult struct {
	Items      []Templatedetails
	TotalItems int64
}
