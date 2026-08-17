package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
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

// SendMessageToSqsQueue publishes the payload body directly to SQS (no SNS hop).
// Body is the raw JSON map so consumers can parse CommApiRequestBody without an SNS envelope.
func SendMessageToSqsQueue(sqsClient *sqs.SQS, messageMap interface{}, queueURL, subject string) error {
	if sqsClient == nil {
		return fmt.Errorf("SQS client is not initialized")
	}
	queueURL = strings.TrimSpace(queueURL)
	if queueURL == "" {
		return fmt.Errorf("SQS queue URL is empty")
	}

	messageBytes, err := json.Marshal(messageMap)
	if err != nil {
		return fmt.Errorf("failed to marshal message to JSON: %w", err)
	}

	attrs := map[string]*sqs.MessageAttributeValue{}
	if subject != "" {
		attrs["SubjectKey"] = &sqs.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(subject),
		}
	}

	input := &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(string(messageBytes)),
	}
	if len(attrs) > 0 {
		input.MessageAttributes = attrs
	}

	resp, err := sqsClient.SendMessage(input)
	if err != nil {
		return fmt.Errorf("failed to send message to SQS: %w", err)
	}
	if resp == nil || resp.MessageId == nil || *resp.MessageId == "" {
		return fmt.Errorf("SQS SendMessage returned empty MessageId")
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

// SendMessageWithSubject moves the message to the shared error queue for inspection.
// Subject is the failure class (e.g. apiHitsFails). Client is always written into the
// body and as an SQS message attribute so ops can identify the originating process
// without creating per-client error queues.
func SendMessageWithSubject(sqsClient *sqs.SQS, messageMap interface{}, queueURL string, subject, errMsg string) error {
	if sqsClient == nil {
		return fmt.Errorf("SQS client is not initialized")
	}

	payload, err := NormalizeErrorPayload(messageMap)
	if err != nil {
		return err
	}

	client := FirstNonEmptyString(payload, "Client", "client")
	process := FirstNonEmptyString(payload, "Process", "process", "ProcessName", "processName")
	channel := FirstNonEmptyString(payload, "Channel", "channel")
	commID := FirstNonEmptyString(payload, "CommId", "commId")

	if client == "" {
		client = process
	}
	if client == "" {
		client = "unknown"
		utils.Error(fmt.Errorf("error queue payload missing Client/Process; subject=%s", subject))
	}
	payload["Client"] = client
	if process != "" {
		payload["Process"] = process
	}
	if channel != "" {
		payload["Channel"] = channel
	}
	if commID != "" {
		payload["CommId"] = commID
	}

	messageBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal message to JSON: %w", err)
	}

	messageAttributes := map[string]*sqs.MessageAttributeValue{
		"Subject": {
			DataType:    aws.String("String"),
			StringValue: aws.String(subject),
		},
		"Error": {
			DataType:    aws.String("String"),
			StringValue: aws.String(errMsg),
		},
		"Client": {
			DataType:    aws.String("String"),
			StringValue: aws.String(client),
		},
	}
	if process != "" {
		messageAttributes["Process"] = &sqs.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(process),
		}
	}
	if channel != "" {
		messageAttributes["Channel"] = &sqs.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(channel),
		}
	}
	if commID != "" {
		messageAttributes["CommId"] = &sqs.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(commID),
		}
	}

	_, err = sqsClient.SendMessage(&sqs.SendMessageInput{
		QueueUrl:          aws.String(queueURL),
		MessageBody:       aws.String(string(messageBytes)),
		MessageAttributes: messageAttributes,
	})
	if err != nil {
		return fmt.Errorf("failed to send message to SQS: %w", err)
	}

	return nil
}

func NormalizeErrorPayload(messageMap interface{}) (map[string]interface{}, error) {
	if messageMap == nil {
		return map[string]interface{}{}, nil
	}
	if asMap, ok := messageMap.(map[string]interface{}); ok {
		out := make(map[string]interface{}, len(asMap)+4)
		for k, v := range asMap {
			out[k] = v
		}
		return out, nil
	}
	messageBytes, err := json.Marshal(messageMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message to JSON: %w", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(messageBytes, &payload); err != nil {
		return nil, fmt.Errorf("failed to convert message to map: %w", err)
	}
	if payload == nil {
		payload = map[string]interface{}{}
	}
	return payload, nil
}

func FirstNonEmptyString(payload map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if raw, ok := payload[key]; ok {
			switch v := raw.(type) {
			case string:
				if s := strings.TrimSpace(v); s != "" {
					return s
				}
			}
		}
	}
	return ""
}
