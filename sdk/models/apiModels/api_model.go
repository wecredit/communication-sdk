package apiModels

import "time"

type WpApiResponseData struct {
	StatusCode int    `json:"statusCode"`
	Status     bool   `json:"status"`
	Message    string `json:"statusMessage"`
	ResponseId string `json:"responseId"`
}

type Vendor struct {
	Id        int        `json:"id"`
	Name      string     `gorm:"column:Name" json:"name" binding:"required"`
	Channel   string     `gorm:"column:Channel" json:"channel" binding:"required"`
	Status    int        `gorm:"column:Status" json:"status"` // 1 = active, 0 = inactive
	IsHealthy int        `gorm:"column:IsHealthy" json:"isHealthy" binding:"required"`
	Weight    int        `gorm:"column:Weight" json:"weight" binding:"required"`
	CreatedOn time.Time  `gorm:"column:CreatedOn" json:"createdOn"`
	UpdatedOn *time.Time `gorm:"column:UpdatedOn" json:"updatedOn,omitempty"`
}

type Client struct {
	Id                 int       `json:"id"`
	Name               string    `gorm:"column:Name" json:"name" binding:"required"`
	Channel            string    `gorm:"column:Channel" json:"channel" binding:"required"`
	Status             int       `gorm:"column:Status" json:"status"` // 1 = active, 0 = inactive
	RateLimitPerMinute int       `gorm:"column:RateLimitPerMinute" json:"rateLimitPerMinute" binding:"required"`
	CreatedOn          time.Time `gorm:"column:CreatedOn" json:"createdOn"`
}

type Userbasicauth struct {
	Id        int       `json:"Id"`
	Username  string    `gorm:"username" json:"username" binding:"required"`
	Password  string    `gorm:"password" json:"password" binding:"required"`
	CreatedOn time.Time `gorm:"createdOn" json:"createdOn,omitempty"`
}
