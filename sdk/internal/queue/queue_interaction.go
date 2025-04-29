package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

const (
	maxRetries   = 3               // Max retry attempts
	retryBackoff = 2 * time.Second // Wait between retries
)

// SendMessage allows putting data in Azure Topic with a subject for a specific subscription
func SendMessage(queueClient *azservicebus.Client, messageMap interface{}, topicName, subject, messageId string) error {
	// Serialize the map to JSON
	messageBytes, err := json.Marshal(messageMap)
	if err != nil {
		return fmt.Errorf("failed to marshal message to JSON: %w", err)
	}
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Create sender for the topic
		sender, err := queueClient.NewSender(topicName, nil)
		if err != nil {
			lastErr = fmt.Errorf("failed to create sender: %w", err)
			utils.Debug(fmt.Sprintf("Attempt %d: %v\n", attempt, lastErr))
			time.Sleep(retryBackoff)
			continue
		}

		// Prepare and send the message with a subject
		sbMessage := &azservicebus.Message{
			Body:      messageBytes,
			Subject:   &subject,
			MessageID: &messageId,
		}

		sendErr := sender.SendMessage(context.TODO(), sbMessage, nil)
		_ = sender.Close(context.TODO()) // Close sender no matter success/fail

		if sendErr == nil {
			// success
			return nil
		}

		lastErr = fmt.Errorf("failed to send message: %w", sendErr)
		utils.Debug(fmt.Sprintf("Attempt %d: %v\n", attempt, lastErr))

		time.Sleep(retryBackoff)
	}

	return fmt.Errorf("failed to send message after %d attempts: %w", maxRetries, lastErr)
}
