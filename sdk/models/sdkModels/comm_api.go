package sdkModels

type CommApiRequestBody struct {
	// DsnAnalytics string
	// DsnTech      string
	CommId      string `json:"commId" gorm:"CommId"`
	Mobile      string `json:"mobile" gorm:"Mobile"`
	Email       string `json:"email" gorm:-`
	Channel     string `json:"channel" gorm:-` // Channel used for sending message
	ProcessName string `json:"processName" gorm:"ProcessName"`
	Stage       int    `json:"stage" gorm:"Stage"`
	IsPriority  bool   `json:"isPriority" gorm:"IsPriority"`
	Vendor      string `json:"vendor" gorm:-`        // vendor who we use to send the message through
	Client      string `json:"client" gorm:"Client"` // User using this sdk
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
