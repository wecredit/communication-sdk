package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

const (
	maxRetries   = 3               // Max retry attempts
	retryBackoff = 2 * time.Second // Wait between retries
)

// SendMessage sends a message to an AWS SNS topic with the subject as a message attribute
func SendMessageToAwsQueue(client *sns.SNS, messageMap interface{}, topicARN string, subject string) error {
	// Convert message to JSON
	messageBytes, err := json.Marshal(messageMap)
	if err != nil {
		return fmt.Errorf("failed to marshal message to JSON: %w", err)
	}

	// Prepare message attributes (for filtering)
	messageAttributes := map[string]*sns.MessageAttributeValue{
		"SubjectKey": {
			DataType:    aws.String("String"),
			StringValue: aws.String(subject),
		},
	}

	// Publish message using global SNSClient
	response, err := client.Publish(&sns.PublishInput{
		Message:           aws.String(string(messageBytes)),
		TopicArn:          aws.String(topicARN),
		MessageAttributes: messageAttributes,
	})
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	// Validate response to ensure message was actually published
	if response == nil {
		return fmt.Errorf("publish returned nil response - message may not have been published")
	}

	// MessageId is required to confirm successful publication
	if response.MessageId == nil || *response.MessageId == "" {
		return fmt.Errorf("publish returned empty or nil MessageId - message may not have been published")
	}

	return nil
}

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
