package queue

import (
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

var (
	SNSClient *sns.SNS
	SQSClient *sqs.SQS
	mut       sync.Mutex
)

// InitSNSClient initializes the global AWS SNS client
func InitAWSClients(region string) error {
	mut.Lock()
	defer mut.Unlock()

	if SNSClient != nil && SQSClient != nil {
		return nil // Already initialized
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		return fmt.Errorf("failed to create AWS session: %w", err)
	}

	// Initialize SNS client if not yet
	if SNSClient == nil {
		SNSClient = sns.New(sess)
		utils.Info("AWS SNS client initialized successfully")
	}
	// Initialize SQS client if not yet
	if SQSClient == nil {
		SQSClient = sqs.New(sess)
		utils.Info("AWS SQS client initialized successfully")
	}

	return nil
}

// GetSNSClient returns the global SNS client
func GetSNSClient(region string) *sns.SNS {
	if SNSClient == nil {
		if err := InitAWSClients(region); err != nil {
			utils.Error(fmt.Errorf("failed to initialize AWS SNS client: %w", err))
			panic(err)
		}
	}
	return SNSClient
}

// GetSQSClient returns the global SQS client
func GetSQSClient(region string) *sqs.SQS {
	if SQSClient == nil {
		if err := InitAWSClients(region); err != nil {
			utils.Error(fmt.Errorf("failed to initialize AWS SQS client: %w", err))
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
