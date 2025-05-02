package main

import (
	"fmt"
	"log"

	"github.com/wecredit/communication-sdk/sdk"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func main() {

	client, err := sdk.NewSdkClient("", "")

	if err != nil {
		fmt.Println("Error in creating SDK Client")
	}

	fmt.Println("Client Created:", client)

	request := &sdkModels.CommApiRequestBody{
		// DsnAnalytics: "sqlserver://Amartya:WeCred!T@2302$@10.1.0.21:1433?database=master",
		Mobile:              "9220146969",
		Email:               "",
		Channel:             "SMS",
		ProcessName:         "RAMFINCORP",
		Stage:               3,
		IsPriority:          true,
		AzureIdempotencyKey: "WC-aa813e999-273d-11f0-9c68-a0b339552d6a-1786180625564719503",
		Vendor:              "TIMES",
	}

	// Call your SDK's Send function
	response, err := client.Send(request)
	if err != nil {
		log.Fatalf("❌ Failed to send message: %v", err)
	}

	fmt.Println("Response:", response)

	log.Println("✅ Message sent successfully!")
}
