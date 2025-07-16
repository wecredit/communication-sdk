package main

import (
	"fmt"
	"log"

	"github.com/wecredit/communication-sdk/sdk"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func main() {
	// For Creditsea local testing
	// username := "creditsea"
	// password := "FvQyZzTp8ckR2wL9gnO7bXEoHVQ5Ijf0A4KmsNt8J2pry1Ba6d9"
	// channel := "EMAIL"
	// baseUrl := "http://localhost:8080"

	// For Creditsea UAT testing
	username := "creditsea"
	password := "FvQyZzTp8ckR2wL9gnO7bXEoHVQ5Ijf0A4KmsNt8J2pry1Ba6d9"
	channel := "EMAIL"
	baseUrl := "http://172.16.34.155:8080"

	// For Nurture Engine local testing
	// username := "nurtureengine"
	// password := "Q29tbXVuaWNhdGlvbkNsaWVudE51cnR1cmVFbmdpbmU="
	// channel := "SMS"
	// baseUrl := "http://localhost:8080"

	client, err := sdk.NewSdkClient(username, password, channel, baseUrl)
	// client, err := sdk.NewSdkClient("nurtureengine", "Q29tbXVuaWNhdGlvbkNsaWVudE51cnR1cmVFbmdpbmU=", "SMS")
	if err != nil {
		fmt.Printf("Error in creating SDK Client: %v", err)
	}

	fmt.Println("\nClient Created:", client)

	request := &sdkModels.CommApiRequestBody{
		Mobile:            "7570897034", //"7579214351",
		Email:             "nikhil.srivastava@wecredit.co.in",
		Channel:           "EMAIL",
		ProcessName:       "CREDITSEA",
		Stage:             2.01,
		IsPriority:        true,
		EmiAmount:         "2",
		CustomerName:      "Brajendra",
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
