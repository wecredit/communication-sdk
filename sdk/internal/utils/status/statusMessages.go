package status

var (
	SuccessMessage    string = "ACK"

	UnauthorizedMissingHeaderMssg   string = "Unauthorized: Missing or invalid Authorization header"
	UnauthorizedInvalidBaseMssg     string = "Unauthorized: Invalid Base64 encoding"
	UnauthorizedInvalidCredFormat   string = "Unauthorized: Invalid credentials format"
	UnauthorizedInvalidUserPassMssg string = "Unauthorized: Invalid username or password"

	InvalidContentType string = "Invalid Content-Type."
	InvalidRequestBody string = "Failed to process the request body"
	InvalidJsonRequest string = "Invalid JSON request"


	InternalServerError = "Some Internal Server Error"

)