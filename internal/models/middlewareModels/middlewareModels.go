package middlewareModels

type MiddlewareResponseModel struct {
	StatusCode   int    `json:"statusCode"`
	StatusMessage string `json:"statusMessage"`
}