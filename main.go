package main

import (
	"fmt"
	"log"

	"github.com/wecredit/communication-sdk/sdk"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func main() {

	client, err := sdk.NewSdkClient("nurtureengine", "Q29tbXVuaWNhdGlvbkNsaWVudE51cnR1cmVFbmdpbmU=")
	if err != nil {
		fmt.Printf("Error in creating SDK Client: %v", err)
	}

	fmt.Println("Client Created:", client)

	request := &sdkModels.CommApiRequestBody{
		// DsnAnalytics: "sqlserver://Amartya:WeCred!T@2302$@10.1.0.21:1433?database=master",
		Mobile:              "9123359755",
		Email:               "",
		Channel:             "WHATSAPP",
		ProcessName:         "OLYV",
		Stage:               1,
		IsPriority:          true,
		AzureIdempotencyKey: "N",
	}

	// Call your SDK's Send function
	response, err := client.Send(request)
	if err != nil {
		log.Fatalf("❌ Failed to send message: %v", err)
	}

	fmt.Println("Response:", response)

	log.Println("✅ Message sent successfully!")
}
