package extapimodels

type TimesAPIModel struct {
	Mobile        string
	TemplateName  string
	ImageUrl      string
	Process       string
	ButtonLink    string
	AccessToken   string
	Stage         int
	CommId        string
	TemplateText  string
	DltTemplateId int64
}

type SmsRequestBody struct {
	Client            string
	Process           string
	DltTemplateId     int64
	TemplateText      string
	TemplateCategory  string
	TemplateVariables string
	Mobile            string
	EmiAmount         string
	CustomerName      string
	LoanId            string
	ApplicationNumber string
	DueDate           string
	Description       string
}

type SmsResponse struct {
	DltTemplateId   int64  `json:"dltTemplateId" gorm:"DltTemplateId"`
	IsSent          bool   `json:"isSent" gorm:"IsSent"`
	CommId          string `json:"CommId" gorm:"CommId"`
	Vendor          string `json:"Vendor" gorm:"Vendor"`
	TransactionId   string `json:"transactionId" gorm:"TransactionId"`
	ResponseMessage string `json:"responseMessage" gorm:"ResponseMessage"`
	MobileNumber    string `json:"mobileNumber" gorm:"MobileNumber"`
}

type WhatsappRequestBody struct {
	AppId             string
	Mobile            string
	Process           string
	TemplateName      string
	ImageUrl          string
	ImageID           string
	ButtonLink        string
	TemplateVariables string
	TemplateCategory  string
	AccessToken       string
	Client            string
	EmiAmount         string // Variables
	CustomerName      string // Variables
	LoanId            string // Variables
	ApplicationNumber string // Variables
	DueDate           string // Variables
}

type WhatsappResponse struct {
	TemplateName    string `json:"templateName" gorm:"TemplateName"`
	IsSent          bool   `json:"isSent" gorm:"IsSent"`
	CommId          string `json:"CommId" gorm:"CommId"`
	Vendor          string `json:"Vendor" gorm:"Vendor"`
	MobileNumber    string `json:"mobileNumber" gorm:"MobileNumber"`
	TransactionId   string `json:"transactionId" gorm:"TransactionId"`
	ResponseMessage string `json:"responseMessage" gorm:"ResponseMessage"`
}

type RcsRequesBody struct {
	Mobile       string
	Process      string
	TemplateName string
	AppId        string
	AppIdKey     string
	ProjectId    string
	ApiKey       string
}

type RcsResponse struct {
	TemplateName    string `json:"templateName" gorm:"TemplateName"`
	CommId          string `json:"CommId" gorm:"CommId"`
	IsSent          bool   `json:"isSent" gorm:"IsSent"`
	Vendor          string `json:"Vendor" gorm:"Vendor"`
	TransactionId   string `json:"transactionId" gorm:"TransactionId"`
	ResponseMessage string `json:"responseMessage" gorm:"ResponseMessage"`
}
