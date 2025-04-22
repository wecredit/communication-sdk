package handlers

import (
	"encoding/json"
	"net/http"

	"dev.azure.com/wctec/communication-engine/sdk/internal/models/sdkModels"
	services "dev.azure.com/wctec/communication-engine/sdk/internal/services/apiServices"
	"dev.azure.com/wctec/communication-engine/sdk/internal/utils"
)

func HandleCommApi(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		// Return HTTP 405 Method Not Allowed if the request method is not POST
		response := sdkModels.CommApiErrorResponseBody{
			StatusCode:    http.StatusMethodNotAllowed,
			StatusMessage: "Invalid request method",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(response)
		return
	}

	var requestData sdkModels.CommApiRequestBody

	// Decode the JSON request into the struct
	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		// Return HTTP 400 Bad Request if JSON is invalid
		response := sdkModels.CommApiErrorResponseBody{
			StatusCode:    http.StatusBadRequest,
			StatusMessage: "Invalid JSON",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Log the requested data
	requestJSON, _ := json.Marshal(requestData)
	utils.Info("Requested Data: " + string(requestJSON))

	// Process the requested Data
	apiResponseStatus, apiResponseData := services.ProcessCommApiData(requestData)

	// Log the response data
	responseJSON, _ := json.Marshal(apiResponseData)
	utils.Info("Response Data: " + string(responseJSON))

	// Send the response with the appropriate status code
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(apiResponseStatus)
	json.NewEncoder(w).Encode(apiResponseData)

}
