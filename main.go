package main

import (
	"fmt"
	"log"

	"github.com/wecredit/communication-sdk/sdk"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func main() {

	request := sdkModels.CommApiRequestBody{
		Mobile:      "7570897034",
		Email:       "",
		Channel:     "WHATSAPP",
		ProcessName: "lnt_ag",
		Source:      "sinch",
	}

	// Call your SDK's Send function
	response, err := sdk.Send(request)
	if err != nil {
		log.Fatalf("❌ Failed to send message: %v", err)
	}

	fmt.Println("Response:", response)

	log.Println("✅ Message sent successfully!")
}
