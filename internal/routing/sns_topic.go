package routing

import (
	"strings"

	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

// EnqueueTarget is where SDK Send places accepted work for async consumers.
type EnqueueTarget struct {
	// QueueURL set => publish directly to SQS (WeCredit SMS plan A).
	QueueURL string
	// TopicArn set => publish via SNS (legacy / other clients).
	TopicArn string
}

// ResolveEnqueueTarget picks SQS-direct for WeCredit+SMS when configured;
// otherwise uses the legacy SNS topic (AWS_COMM_TOPIC_ARN).
func ResolveEnqueueTarget(client, channel string) EnqueueTarget {
	defaultArn := strings.TrimSpace(config.Configs.AwsSnsArn)
	client = strings.TrimSpace(client)
	channel = strings.TrimSpace(channel)

	if strings.EqualFold(client, variables.WeCredit) && strings.EqualFold(channel, variables.SMS) {
		if q := strings.TrimSpace(config.Configs.AwsWeCreditSmsQueueUrl); q != "" {
			return EnqueueTarget{QueueURL: q}
		}
		// Optional legacy WeCredit SMS topic (prefer queue URL above).
		if arn := strings.TrimSpace(config.Configs.AwsWeCreditSmsTopicArn); arn != "" {
			return EnqueueTarget{TopicArn: arn}
		}
	}
	return EnqueueTarget{TopicArn: defaultArn}
}

// ResolveSnsTopicArn is kept for callers that only need a topic ARN.
// Prefer ResolveEnqueueTarget for new code.
func ResolveSnsTopicArn(client, channel string) string {
	return ResolveEnqueueTarget(client, channel).TopicArn
}
