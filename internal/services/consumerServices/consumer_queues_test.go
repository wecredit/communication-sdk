package services

import (
	"testing"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/sdk/models"
)

func TestConsumerQueueURLs(t *testing.T) {
	prev := config.Configs
	t.Cleanup(func() { config.Configs = prev })

	config.Configs = models.Config{
		AwsQueueUrl:            "https://sqs.example/priority",
		AwsWeCreditSmsQueueUrl: "",
	}
	got := consumerQueueURLs()
	if len(got) != 1 || got[0] != "https://sqs.example/priority" {
		t.Fatalf("legacy only: %#v", got)
	}

	config.Configs.AwsWeCreditSmsQueueUrl = "https://sqs.example/wecredit-sms"
	got = consumerQueueURLs()
	if len(got) != 2 || got[0] != "https://sqs.example/priority" || got[1] != "https://sqs.example/wecredit-sms" {
		t.Fatalf("both queues: %#v", got)
	}

	config.Configs.AwsWeCreditSmsQueueUrl = "https://sqs.example/priority"
	got = consumerQueueURLs()
	if len(got) != 1 {
		t.Fatalf("dedupe: %#v", got)
	}
}
