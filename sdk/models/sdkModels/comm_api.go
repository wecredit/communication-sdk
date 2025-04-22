package sdkModels

type CommApiRequestBody struct {
	DSN         string
	Mobile      string `json:"mobile"`
	Email       string `json:"email"`
	Channel     string `json:"channel"`
	ProcessName string `json:"processName"`
	Source      string `json:"source"`
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
