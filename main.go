package main

import (
	"fmt"
	"log"

	"github.com/wecredit/communication-sdk/sdk"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func main() {

	client, err := sdk.NewSdkClient("creditsea", "FvQyZzTp8ckR2wL9gnO7bXEoHVQ5Ijf0A4KmsNt8J2pry1Ba6d9", "WHATSAPP")
	// client, err := sdk.NewSdkClient("nurtureengine", "Q29tbXVuaWNhdGlvbkNsaWVudE51cnR1cmVFbmdpbmU=", "SMS")
	if err != nil {
		fmt.Printf("Error in creating SDK Client: %v", err)
	}

	fmt.Println("\nClient Created:", client)

	request := &sdkModels.CommApiRequestBody{
		Mobile:            "6200807541", //"7579214351",
		Email:             "",
		Channel:           "WHATSAPP",
		ProcessName:       "CREDITSEA",
		Stage:             3.02,
		IsPriority:        true,
		EmiAmount:         "2",
		// CustomerName:      "Brajendra",
		LoanId:            "123833",
		ApplicationNumber: "575676353657",
		DueDate:           "2025-06-08",
		Description:       "TEST",
	}

	// Call your SDK's Send function
	response, err := client.Send(request)
	if err != nil {
		log.Fatalf("❌ Failed to send message: %v", err)
	}

	fmt.Println("Response:", response)

	log.Println("✅ Message sent successfully!")
}
