package main

import (
	"fmt"
	"log"

	"github.com/wecredit/communication-sdk/sdk"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func main() {

	client, err := sdk.NewSdkClient("creditsea", "FvQyZzTp8ckR2wL9gnO7bXEoHVQ5Ijf0A4KmsNt8J2pry1Ba6d9", "WHATSAPP")
	// client, err := sdk.NewSdkClient("nurtureengine", "Q29tbXVuaWNhdGlvbkNsaWVudE51cnR1cmVFbmdpbmU=", "WHATSAPP")
	if err != nil {
		fmt.Printf("Error in creating SDK Client: %v", err)
	}

	fmt.Println("\nClient Created:", client)

	request := &sdkModels.CommApiRequestBody{
		Mobile:      "8003366950", //"7579214351",
		Email:       "",
		Channel:     "WHATSAPP",
		ProcessName: "CREDITSEA",
		Stage:       1,
		IsPriority:  true,
	}

	// Call your SDK's Send function
	response, err := client.Send(request)
	if err != nil {
		log.Fatalf("❌ Failed to send message: %v", err)
	}

	fmt.Println("Response:", response)

	log.Println("✅ Message sent successfully!")
}
