package routing

import (
	"testing"

	"github.com/wecredit/communication-sdk/config"
)

func TestResolveEnqueueTargetWeCreditSMSUsesQueue(t *testing.T) {
	prevQ := config.Configs.AwsWeCreditSmsQueueUrl
	prevT := config.Configs.AwsWeCreditSmsTopicArn
	prevD := config.Configs.AwsSnsArn
	t.Cleanup(func() {
		config.Configs.AwsWeCreditSmsQueueUrl = prevQ
		config.Configs.AwsWeCreditSmsTopicArn = prevT
		config.Configs.AwsSnsArn = prevD
	})

	config.Configs.AwsSnsArn = "arn:aws:sns:legacy"
	config.Configs.AwsWeCreditSmsTopicArn = "arn:aws:sns:wecredit-sms"
	config.Configs.AwsWeCreditSmsQueueUrl = "https://sqs.example/comm-wecredit-sms-staging"

	got := ResolveEnqueueTarget("wecredit", "SMS")
	if got.QueueURL == "" || got.TopicArn != "" {
		t.Fatalf("expected SQS-direct only, got %+v", got)
	}

	gotZap := ResolveEnqueueTarget("zapcash", "SMS")
	if gotZap.QueueURL != "" || gotZap.TopicArn != "arn:aws:sns:legacy" {
		t.Fatalf("zapcash should stay on legacy SNS, got %+v", gotZap)
	}
}

func TestResolveSnsTopicArnFallsBackToDefault(t *testing.T) {
	gotZap := ResolveSnsTopicArn("zapcash", "SMS")
	if gotZap != ResolveSnsTopicArn("zapcash", "WHATSAPP") {
		t.Fatalf("non-WeCredit topics should resolve the same default")
	}
}
