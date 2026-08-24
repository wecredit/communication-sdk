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
	Client            string `json:"Client"`
	Process           string `json:"Process"`
	Channel           string `json:"Channel,omitempty"`
	CommId            string `json:"CommId,omitempty"`
	Vendor            string `json:"Vendor,omitempty"`
	DltTemplateId     int64  `json:"DltTemplateId,omitempty"`
	TemplateEntityId  string `json:"TemplateEntityId,omitempty"`
	TemplateHeader    string `json:"TemplateHeader,omitempty"`
	TemplateText      string `json:"TemplateText,omitempty"`
	TemplateCategory  string `json:"TemplateCategory,omitempty"`
	TemplateVariables string `json:"TemplateVariables,omitempty"`
	Mobile            string `json:"Mobile,omitempty"`
	EmiAmount         string `json:"EmiAmount,omitempty"`
	CustomerName      string `json:"CustomerName,omitempty"`
	LoanId            string `json:"LoanId,omitempty"`
	ApplicationNumber string `json:"ApplicationNumber,omitempty"`
	DueDate           string `json:"DueDate,omitempty"`
	Description       string `json:"Description,omitempty"`
	PaymentLink       string `json:"PaymentLink,omitempty"`
	// TemplateVariableValues is optional positional CSV (CommMarketingInput.VariablesValue).
	// ZapCash/legacy leave this empty and already set named fields above.
	TemplateVariableValues string `json:"TemplateVariableValues,omitempty"`

	// Compliance identity for the CommMarketingInput WeCredit SMS path.
	Source       string `json:"Source,omitempty"`
	SourceRowId  int64  `json:"SourceRowId,omitempty"`
	CampaignDate string `json:"CampaignDate,omitempty"`
}

type SmsResponse struct {
	DltTemplateId   int64  `json:"dltTemplateId" gorm:"DltTemplateId"`
	IsSent          bool   `json:"isSent" gorm:"IsSent"`
	CommId          string `json:"CommId" gorm:"CommId"`
	Vendor          string `json:"Vendor" gorm:"Vendor"`
	TransactionId   string `json:"transactionId" gorm:"TransactionId"`
	ResponseMessage string `json:"responseMessage" gorm:"ResponseMessage"`
	MobileNumber    string `json:"mobileNumber" gorm:"MobileNumber"`
	Outcome         string `json:"ProviderOutcome,omitempty" gorm:"-"`
}

type WhatsappRequestBody struct {
	AppId              string
	CommId             string
	Mobile             string
	Process            string
	TemplateName       string
	ImageUrl           string
	ImageID            string
	ButtonLink         string
	TemplateVariables  string
	TemplateCategory   string
	AccessToken        string
	Client             string
	EmiAmount          string // Variables
	CustomerName       string // Variables
	LoanId             string // Variables
	ApplicationNumber  string // Variables
	DueDate            string // Variables
	Description        string // Variables
	TotalPayableAmount string // Variables
	TodayPayableAmount string // Variables
	SavingAmount       string // Variables
	BounceCharge       string // Variables
}

type WhatsappResponse struct {
	TemplateName    string `json:"templateName" gorm:"TemplateName"`
	IsSent          bool   `json:"isSent" gorm:"IsSent"`
	CommId          string `json:"CommId" gorm:"CommId"`
	Vendor          string `json:"Vendor" gorm:"Vendor"`
	MobileNumber    string `json:"mobileNumber" gorm:"MobileNumber"`
	TransactionId   string `json:"transactionId" gorm:"TransactionId"`
	ResponseMessage string `json:"responseMessage" gorm:"ResponseMessage"`
	PaymentLink     string `json:"paymentLink" gorm:"PaymentLink"`
}

type RcsRequestBody struct {
	Mobile               string
	Process              string
	Client               string
	CommId               string
	TemplateName         string
	AppId                string
	AppIdKey             string
	ProjectId            string
	ApiKey               string
	TemplateVariables    string
	SmsFallbackVariables string
	TemplateCategory     string
	EmiAmount            string
	CustomerName         string
	LoanId               string
	ApplicationNumber    string
	DueDate              string
	Description          string
	TotalPayableAmount   string
	TodayPayableAmount   string
	SavingAmount         string
	BounceCharge         string
	PaymentLink          string
}

type RcsResponse struct {
	TemplateName    string `json:"templateName" gorm:"TemplateName"`
	CommId          string `json:"CommId" gorm:"CommId"`
	IsSent          bool   `json:"isSent" gorm:"IsSent"`
	Vendor          string `json:"Vendor" gorm:"Vendor"`
	MobileNumber    string `json:"mobileNumber" gorm:"MobileNumber"`
	TransactionId   string `json:"transactionId" gorm:"TransactionId"`
	ResponseMessage string `json:"responseMessage" gorm:"ResponseMessage"`
}

type EmailRequestBody struct {
	Client            string
	Process           string
	TemplateId        string
	EmailSubject      string
	TemplateVariables string
	FromEmail         string
	ToEmail           string
	EmiAmount         string
	CustomerName      string
	LoanId            string
	ApplicationNumber string
	DueDate           string
	Description       string
}

type EmailResponse struct {
	TemplateName    string `json:"templateName" gorm:"TemplateName"`
	CommId          string `json:"CommId" gorm:"CommId"`
	IsSent          bool   `json:"isSent" gorm:"IsSent"`
	Vendor          string `json:"Vendor" gorm:"Vendor"`
	TransactionId   string `json:"transactionId" gorm:"TransactionId"`
	ResponseMessage string `json:"responseMessage" gorm:"ResponseMessage"`
	Email           string `json:"email" gorm:"email"`
}
