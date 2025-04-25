package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

const (
	maxRetries   = 3               // Maximum retries for message processing
	retryBackoff = time.Second * 5 // Initial backoff duration
	pollInterval = time.Second * 3 // Interval for checking the queue when empty
)

// SendMessage allows putting data in Azure Topic with a subject for a specific subscription
func SendMessage(messageMap interface{}, topicName string) error {
	// Serialize the map to JSON
	messageBytes, err := json.Marshal(messageMap)
	if err != nil {
		return fmt.Errorf("failed to marshal message to JSON: %w", err)
	}

	// Create sender for the topic
	sender, err := Client.NewSender(topicName, nil)
	if err != nil {
		return fmt.Errorf("failed to create sender: %w", err)
	}
	defer sender.Close(context.TODO())
	fmt.Println("Sender:", sender)

	// Prepare and send the message with a subject
	sbMessage := &azservicebus.Message{
		Body: messageBytes,
		// Subject: &subject,
	}
	err = sender.SendMessage(context.TODO(), sbMessage, nil)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}
