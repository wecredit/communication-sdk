package monitoring

import "github.com/wecredit/communication-sdk/sdk/models/sdkModels"

// Profile contains safe, non-production values used to render monitoring copies.
type Profile struct {
	CustomerName       string `json:"customerName"`
	EmiAmount          string `json:"emiAmount"`
	LoanID             string `json:"loanId"`
	ApplicationNumber  string `json:"applicationNumber"`
	DueDate            string `json:"dueDate"`
	Description        string `json:"description"`
	PaymentLink        string `json:"paymentLink"`
	TotalPayableAmount string `json:"totalPayableAmount"`
	TodayPayableAmount string `json:"todayPayableAmount"`
	SavingAmount       string `json:"savingAmount"`
	BounceCharge       string `json:"bounceCharge"`
}

// AcceptedResult is an immutable snapshot of a provider-accepted ZapCash send.
type AcceptedResult struct {
	Payload              sdkModels.CommApiRequestBody
	ResolvedVendor       string
	ResolvedTemplate     string
	TemplateVariables    string
	SMSFallbackVariables string
	TransactionID        string
}

type RuntimeConfig struct {
	Enabled    bool
	Recipients []string
	Profile    Profile
}
