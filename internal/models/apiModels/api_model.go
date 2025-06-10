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
	Id                 int        `json:"id"`
	Name               string     `gorm:"column:Name" json:"name" binding:"required"`
	Channel            string     `gorm:"column:Channel" json:"channel" binding:"required"`
	Status             int        `gorm:"column:Status" json:"status"` // 1 = active, 0 = inactive
	RateLimitPerMinute int        `gorm:"column:RateLimitPerMinute" json:"rateLimitPerMinute" binding:"required"`
	CreatedOn          time.Time  `gorm:"column:CreatedOn" json:"createdOn"`
	UpdatedOn          *time.Time `gorm:"column:UpdatedOn" json:"updatedOn,omitempty"`
}

type Userbasicauth struct {
	Id        int       `json:"Id"`
	Username  string    `gorm:"column:username" json:"username" binding:"required"`
	Password  string    `gorm:"column:password" json:"password" binding:"required"`
	CreatedOn time.Time `gorm:"column:createdOn" json:"createdOn,omitempty"`
}

type Templatedetails struct {
	Id            int        `json:"id"`
	TemplateName  string     `gorm:"column:TemplateName" json:"templateName" binding:"required"`
	ImageId       string     `gorm:"column:ImageId" json:"imageId,omitempty"`
	Process       string     `gorm:"column:Process" json:"process"`
	Stage         int        `gorm:"column:Stage" json:"stage"`
	ImageUrl      string     `gorm:"column:ImageUrl" json:"imageUrl,omitempty"`
	DltTemplateId int64      `gorm:"column:DltTemplateId" json:"dltTemplateId,omitempty"`
	Channel       string     `gorm:"column:Channel" json:"channel"`
	Vendor        string     `gorm:"column:Vendor" json:"vendor"`
	IsActive      bool       `gorm:"column:IsActive" json:"isActive"`
	TemplateText  string     `gorm:"column:TemplateText" json:"templateText,omitempty"`
	Link          string     `gorm:"column:Link" json:"link,omitempty"`
	CreatedOn     time.Time  `gorm:"column:CreatedOn" json:"createdOn"`
	UpdatedOn     *time.Time `gorm:"column:UpdatedOn" json:"updatedOn,omitempty"`
	Client        string     `gorm:"column:Client" json:"client,omitempty"`
}
