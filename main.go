package main

import (
	"fmt"
	"log"

	"github.com/wecredit/communication-sdk/sdk"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func main() {

	client, err := sdk.NewSdkClient("creditsea", "FvQyZzTp8ckR2wL9gnO7bXEoHVQ5Ijf0A4KmsNt8J2pry1Ba6d9", "SMS")
	// client, err := sdk.NewSdkClient("nurtureengine", "Q29tbXVuaWNhdGlvbkNsaWVudE51cnR1cmVFbmdpbmU=", "SMS")
	if err != nil {
		fmt.Printf("Error in creating SDK Client: %v", err)
	}

	fmt.Println("\nClient Created:", client)

	request := &sdkModels.CommApiRequestBody{
		Mobile:            "9220146969", //"7579214351",
		Email:             "",
		Channel:           "SMS",
		ProcessName:       "CREDITSEA",
		Stage:             45,
		IsPriority:        true,
		EmiAmount:         "2",
		CustomerName:      "Arvind",
		LoanId:            "",
		ApplicationNumber: "10021",
		DueDate:           "2025-06-08T00:00:00Z",
	}

	// Call your SDK's Send function
	response, err := client.Send(request)
	if err != nil {
		log.Fatalf("❌ Failed to send message: %v", err)
	}

	fmt.Println("Response:", response)

	log.Println("✅ Message sent successfully!")
}
