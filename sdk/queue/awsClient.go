package queue

import (
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
)

var (
	SNSClient *sns.SNS
	SQSClient *sqs.SQS
	mut       sync.Mutex
)

// InitSNSClient initializes the global AWS SNS client
func InitAWSClients(region string) (*sns.SNS, error) {
	mut.Lock()
	defer mut.Unlock()

	if SNSClient != nil && SQSClient != nil {
		return SNSClient, nil // Already initialized
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	// Initialize SNS client if not yet
	if SNSClient == nil {
		SNSClient = sns.New(sess)
		fmt.Println("AWS SNS client initialized successfully")
	}
	// Initialize SQS client if not yet
	if SQSClient == nil {
		SQSClient = sqs.New(sess)
		fmt.Println("AWS SQS client initialized successfully")
	}

	return SNSClient, nil
}

// GetSNSClient returns the global SNS client
func GetSNSClient(region string) *sns.SNS {
	if SNSClient == nil {
		if _, err := InitAWSClients(region); err != nil {
			fmt.Println("Failed to initialize AWS SNS client.")
			panic(err)
		}
	}
	return SNSClient
}

// GetSQSClient returns the global SQS client
func GetSQSClient(region string) *sqs.SQS {
	if SQSClient == nil {
		if _, err := InitAWSClients(region); err != nil {
			fmt.Println("Failed to initialize AWS SQS client.")
			panic(err)
		}
	}
	return SQSClient
}

func GetSdkSnsClient(region string) (*sns.SNS, error) {
	mut.Lock()
	defer mut.Unlock()

	var err error
	var client *sns.SNS
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}
	client = sns.New(sess)
	return client, nil
}
