package main

import (
	"fmt"
	"log"

	"github.com/wecredit/communication-sdk/sdk"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func main() {

	client, err := sdk.NewSdkClient("creditsea", "FvQyZzTp8ckR2wL9gnO7bXEoHVQ5Ijf0A4KmsNt8J2pry1Ba6d9", "SMS")
	// client, err := sdk.NewSdkClient("nurtureengine", "Q29tbXVuaWNhdGlvbkNsaWVudE51cnR1cmVFbmdpbmU=", "WHATSAPP")
	if err != nil {
		fmt.Printf("Error in creating SDK Client: %v", err)
	}

	fmt.Println("\nClient Created:", client)

	request := &sdkModels.CommApiRequestBody{
		Mobile:            "9220146969", //"7579214351",
		Email:             "",
		Channel:           "SMS",
		ProcessName:       "CREDITSEA",
		Stage:             1,
		IsPriority:        true,
		EmiAmount:         "10002",
		CustomerName:      "Arvind",
		LoanId:            "1234567890",
		ApplicationNumber: "1234567890",
		DueDate:           "2023-10-31",
	}

	// Call your SDK's Send function
	response, err := client.Send(request)
	if err != nil {
		log.Fatalf("❌ Failed to send message: %v", err)
	}

	fmt.Println("Response:", response)

	log.Println("✅ Message sent successfully!")
}
