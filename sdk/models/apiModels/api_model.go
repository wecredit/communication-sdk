package apiModels

type WpApiResponseData struct {
	StatusCode int    `json:"statusCode"`
	Status     bool   `json:"status"`
	Message    string `json:"statusMessage"`
	ResponseId string `json:"responseId"`
}
