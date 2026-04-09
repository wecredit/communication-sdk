package sdkModels

import "gorm.io/gorm"

type CommApiRequestBody struct {
	DbClient            *gorm.DB `json:"-" gorm:-`
	InputTableName      string   `json:"inputTableName" gorm:-`
	CommId              string   `json:"commId" gorm:"CommId"`
	Mobile              string   `json:"mobile" gorm:"Mobile"`
	Email               string   `json:"email" gorm:-`
	Channel             string   `json:"channel" gorm:-` // Channel used for sending message
	ProcessName         string   `json:"processName" gorm:"ProcessName"`
	Stage               float64  `json:"stage" gorm:"Stage"`
	IsPriority          bool     `json:"isPriority" gorm:"IsPriority"`
	Vendor              string   `json:"vendor" gorm:-`                      // vendor who we use to send the message through
	Client              string   `json:"client" gorm:"Client"`               // User using this sdk
	EmiAmount           string   `json:"emiAmount,omitempty" gorm:-`         // variables used in creditsea Template
	CustomerName        string   `json:"customerName,omitempty" gorm:-`      // variables used in creditsea, zapcash Template
	LoanId              string   `json:"loanId,omitempty" gorm:-`            // variables used in creditsea, zapcash Template
	ApplicationNumber   string   `json:"applicationNumber,omitempty" gorm:-` // variables used in creditsea Template
	DueDate             string   `json:"dueDate,omitempty" gorm:-`           // variables used in creditsea, zapcash Template
	AzureIdempotencyKey string   `json:"azureIdempotencyKey,omitempty" gorm:"AzureIdempotencyKey"`
	Description         string   `json:"description,omitempty" gorm:-`        // variables used in creditsea, zapcash Template
	PaymentLink         string   `json:"paymentLink,omitempty" gorm:-`        // payment link for the message
	TotalPayableAmount  string   `json:"totalPayableAmount,omitempty" gorm:-` // total payable amount for the message
	TodayPayableAmount  string   `json:"todayPayableAmount,omitempty" gorm:-` // today payable amount for the message
	SavingAmount        string   `json:"savingAmount,omitempty" gorm:-`       // savings amount for the message
	BounceCharge        string   `json:"bounceCharge,omitempty" gorm:-`       // bounce charge for the message

}

type CommApiResponseBody struct {
	CommId  string `json:"commId"`
	Success bool   `json:"success"`
	// ReqTimeStamp  string `json:"reqTimeStamp,omitempty"` // After processing
}

type CommApiErrorResponseBody struct {
	StatusCode    int    `json:"statusCode"`
	StatusMessage string `json:"statusMessage,omitempty"`
}
