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

	request := sdkModels.CommApiRequestBody{
		// DsnAnalytics: "sqlserver://Amartya:WeCred!T@2302$@10.1.0.21:1433?database=master",
		Mobile:      "8003366950",
		Email:       "",
		Channel:     "WHATSAPP",
		ProcessName: "lnt_ag",
		Vendor:      "sinch",
	}

	// Call your SDK's Send function
	response, err := client.Send(request)
	if err != nil {
		log.Fatalf("❌ Failed to send message: %v", err)
	}

	fmt.Println("Response:", response)

	log.Println("✅ Message sent successfully!")
}
