package apiModels

type WpApiResponseData struct {
	StatusCode float64 `json:"statusCode"`
	Status     bool    `json:"status"`
	Message    string  `json:"statusMessage"`
}
