package apiModels

import "time"

type WpApiResponseData struct {
	StatusCode int    `json:"statusCode"`
	Status     bool   `json:"status"`
	Message    string `json:"statusMessage"`
	ResponseId string `json:"responseId"`
}

type Vendor struct {
	Name      string     `gorm:"Name" json:"name" binding:"required"`
	Channel   string     `json:"channel" binding:"required"`
	Status    int        `json:"status"` // 1 = active, 0 = inactive
	IsHealthy int        `json:"isHealthy"`
	Weight    int        `json:"weight" binding:"required"`
	CreatedOn time.Time  `json:"createdOn"`
	UpdatedOn *time.Time `json:"updatedOn,omitempty"`
}
