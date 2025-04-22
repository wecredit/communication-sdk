package status

var (
	SuccessMessageCodeID int = 2000 // ACK

	UnauthorizedMissingHeaderCode int = 3001 // Unauthorized: Missing or invalid Authorization header

	InvalidContentTypeCodeID    int = 4001 // Invalid Content-Type. Expected application/xml
	InvalidRequestBodyCodeID    int = 4002 // Failed to process the request body
	InvalidJsonRequestCodeID    int = 4003 // Invalid JSON request
	BadRequestNoRecordFoundCode int = 4004 // Bad Request

	InternalServerErrorCode int = 5000 // Internal Server Error Status Code (4-digit)

)
