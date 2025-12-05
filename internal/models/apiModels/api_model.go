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
	Client    string     `gorm:"column:Client" json:"client" binding:"required"`
	Status    int        `gorm:"column:Status" json:"status"` // 1 = active, 0 = inactive
	IsHealthy int        `gorm:"column:IsHealthy" json:"isHealthy" binding:"required"`
	Weight    int        `gorm:"column:Weight" json:"weight" binding:"required"`
	CreatedOn time.Time  `gorm:"column:CreatedOn" json:"createdOn"`
	UpdatedOn *time.Time `gorm:"column:UpdatedOn" json:"updatedOn,omitempty"`
}

type Client struct {
	Id                 int        `json:"id"`
	Name               string     `gorm:"column:Name" json:"name"`
	Channel            string     `gorm:"column:Channel" json:"channel"`
	Status             int        `gorm:"column:Status" json:"status"` // 1 = active, 0 = inactive
	RateLimitPerMinute int        `gorm:"column:RateLimitPerMinute" json:"rateLimitPerMinute"`
	CreatedOn          time.Time  `gorm:"column:CreatedOn" json:"createdOn"`
	UpdatedOn          *time.Time `gorm:"column:UpdatedOn" json:"updatedOn,omitempty"`
}

// TableName overrides default GORM table name
func (Client) TableName() string {
	return "Clients"
}

type Userbasicauth struct {
	Id        int       `json:"Id"`
	Username  string    `gorm:"column:username" json:"username" binding:"required"`
	Password  string    `gorm:"column:password" json:"password" binding:"required"`
	CreatedOn time.Time `gorm:"column:createdOn" json:"createdOn,omitempty"`
}

func (Userbasicauth) TableName() string {
	return "userBasicAuths"
}

type Templatedetails struct {
	Id                int        `json:"id"`
	Client            string     `gorm:"column:Client" json:"client,omitempty"`
	Channel           string     `gorm:"column:Channel" json:"channel" binding:"required"`
	Process           string     `gorm:"column:Process" json:"process"`
	Stage             float64    `gorm:"column:Stage" json:"stage" binding:"required"`
	Vendor            string     `gorm:"column:Vendor" json:"vendor" binding:"required"`
	TemplateName      string     `gorm:"column:TemplateName" json:"templateName" binding:"required"`
	ImageId           string     `gorm:"column:ImageId" json:"imageId,omitempty"`
	ImageUrl          string     `gorm:"column:ImageUrl" json:"imageUrl,omitempty"`
	DltTemplateId     int64      `gorm:"column:DltTemplateId" json:"dltTemplateId,omitempty"`
	IsActive          bool       `gorm:"column:IsActive" json:"isActive"`
	TemplateText      string     `gorm:"column:TemplateText" json:"templateText,omitempty"`
	Link              string     `gorm:"column:Link" json:"link,omitempty"`
	CreatedOn         time.Time  `gorm:"column:CreatedOn" json:"createdOn"`
	UpdatedOn         *time.Time `gorm:"column:UpdatedOn" json:"updatedOn,omitempty"`
	TemplateCategory  int64      `gorm:"column:TemplateCategory" json:"templateCategory,omitempty"`
	TemplateVariables string     `gorm:"column:TemplateVariables" json:"templateVariables,omitempty"`
	Subject           string     `gorm:"column:Subject" json:"subject,omitempty"`
	FromEmail         string     `gorm:"column:FromEmail" json:"fromEmail,omitempty"`
}

// TableName overrides the default table name used by GORM
func (Templatedetails) TableName() string {
	return "TemplateDetails"
}
