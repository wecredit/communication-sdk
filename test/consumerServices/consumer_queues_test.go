package consumerServices_test

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/wecredit/communication-sdk/config"
	services "github.com/wecredit/communication-sdk/internal/services/consumerServices"
	"github.com/wecredit/communication-sdk/sdk/models"
)

func TestConsumerQueueURLs(t *testing.T) {
	prev := config.Configs
	t.Cleanup(func() { config.Configs = prev })

	config.Configs = models.Config{
		AwsQueueUrl:                 "https://sqs.example/priority",
		AwsWeCreditSmsQueueUrl:      "",
		AwsWeCreditWhatsappQueueUrl: "",
	}
	got := services.ConsumerQueueURLs()
	if len(got) != 1 || got[0] != "https://sqs.example/priority" {
		t.Fatalf("legacy only: %#v", got)
	}

	config.Configs.AwsWeCreditSmsQueueUrl = "https://sqs.example/wecredit-sms"
	got = services.ConsumerQueueURLs()
	if len(got) != 2 || got[0] != "https://sqs.example/priority" || got[1] != "https://sqs.example/wecredit-sms" {
		t.Fatalf("both queues: %#v", got)
	}

	config.Configs.AwsWeCreditSmsQueueUrl = "https://sqs.example/priority"
	got = services.ConsumerQueueURLs()
	if len(got) != 1 {
		t.Fatalf("dedupe: %#v", got)
	}

	config.Configs.AwsWeCreditSmsQueueUrl = "https://sqs.example/wecredit-sms"
	config.Configs.AwsWeCreditWhatsappQueueUrl = "https://sqs.example/wecredit-whatsapp"
	got = services.ConsumerQueueURLs()
	if len(got) != 3 || got[2] != "https://sqs.example/wecredit-whatsapp" {
		t.Fatalf("WhatsApp queue missing: %#v", got)
	}
}

type queueAttributesStub struct {
	output *sqs.GetQueueAttributesOutput
	err    error
}

func (s queueAttributesStub) GetQueueAttributes(*sqs.GetQueueAttributesInput) (*sqs.GetQueueAttributesOutput, error) {
	return s.output, s.err
}

func TestParseRedriveMaxReceiveCount(t *testing.T) {
	valid := `{"deadLetterTargetArn":"arn:aws:sqs:ap-south-1:123:wp-dlq","maxReceiveCount":"5"}`
	if got, err := services.ParseRedriveMaxReceiveCount(valid); err != nil || got != 5 {
		t.Fatalf("valid policy = %d, %v", got, err)
	}
	for _, raw := range []string{
		``,
		`not-json`,
		`{"maxReceiveCount":"5"}`,
		`{"deadLetterTargetArn":"arn:dlq","maxReceiveCount":"0"}`,
		`{"deadLetterTargetArn":"arn:dlq","maxReceiveCount":"invalid"}`,
	} {
		if _, err := services.ParseRedriveMaxReceiveCount(raw); err == nil {
			t.Fatalf("accepted invalid policy %q", raw)
		}
	}
}

func TestLoadWhatsappRedriveMaxReceiveCount(t *testing.T) {
	policy := `{"deadLetterTargetArn":"arn:aws:sqs:ap-south-1:123:wp-dlq","maxReceiveCount":"3"}`
	stub := queueAttributesStub{output: &sqs.GetQueueAttributesOutput{
		Attributes: map[string]*string{"RedrivePolicy": aws.String(policy)},
	}}
	if got, err := services.LoadWhatsappRedriveMaxReceiveCount(stub, "https://sqs.example/wp"); err != nil || got != 3 {
		t.Fatalf("lookup = %d, %v", got, err)
	}
	if _, err := services.LoadWhatsappRedriveMaxReceiveCount(queueAttributesStub{err: errors.New("denied")}, "https://sqs.example/wp"); err == nil {
		t.Fatal("accepted AWS lookup failure")
	}
	if _, err := services.LoadWhatsappRedriveMaxReceiveCount(queueAttributesStub{}, "https://sqs.example/wp"); err == nil {
		t.Fatal("accepted missing attributes")
	}
}

func TestPrepareConsumerQueuesInvalidWhatsappPolicyIsIsolated(t *testing.T) {
	urls := []string{
		"https://sqs.example/wecredit-sms",
		"https://sqs.example/wecredit-whatsapp",
		"https://sqs.example/zapcash",
	}
	runtimes := services.PrepareConsumerQueues(queueAttributesStub{}, urls, urls[1])
	if len(runtimes) != 2 {
		t.Fatalf("enabled runtime count = %d, want 2", len(runtimes))
	}
	if runtimes[0].URL != urls[0] || runtimes[1].URL != urls[2] {
		t.Fatalf("invalid WhatsApp policy affected other queues: %#v", runtimes)
	}
}
