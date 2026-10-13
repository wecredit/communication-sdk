package consumerServices_test

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/wecredit/communication-sdk/config"
	push "github.com/wecredit/communication-sdk/internal/channels/push"
	services "github.com/wecredit/communication-sdk/internal/services/consumerServices"
	"github.com/wecredit/communication-sdk/sdk/models"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/variables"
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

func TestValidateConsumerQueueURLsRejectsZapCashLegacyCollision(t *testing.T) {
	prev := config.Configs
	t.Cleanup(func() { config.Configs = prev })

	shared := "https://sqs.example/shared"
	for _, tc := range []struct {
		name string
		set  func()
	}{
		{
			name: "legacy queue",
			set: func() {
				config.Configs.AwsQueueUrl = " " + shared + " "
				config.Configs.AwsWeCreditSmsQueueUrl = ""
				config.Configs.AwsWeCreditWhatsappQueueUrl = ""
			},
		},
		{
			name: "WeCredit SMS queue",
			set: func() {
				config.Configs.AwsQueueUrl = ""
				config.Configs.AwsWeCreditSmsQueueUrl = shared
				config.Configs.AwsWeCreditWhatsappQueueUrl = ""
			},
		},
		{
			name: "WeCredit WhatsApp queue",
			set: func() {
				config.Configs.AwsQueueUrl = ""
				config.Configs.AwsWeCreditSmsQueueUrl = ""
				config.Configs.AwsWeCreditWhatsappQueueUrl = shared
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config.Configs.AwsZapCashQueueUrl = shared
			tc.set()
			if err := services.ValidateConsumerQueueURLs(); err == nil {
				t.Fatal("accepted shared consumer and ZapCash queue URL")
			}
		})
	}

	config.Configs.AwsQueueUrl = "https://sqs.example/legacy"
	config.Configs.AwsWeCreditSmsQueueUrl = "https://sqs.example/sms"
	config.Configs.AwsWeCreditWhatsappQueueUrl = "https://sqs.example/whatsapp"
	config.Configs.AwsZapCashQueueUrl = "https://sqs.example/zapcash"
	if err := services.ValidateConsumerQueueURLs(); err != nil {
		t.Fatalf("rejected distinct queue URLs: %v", err)
	}
}

func TestMessageWrapperRetainsItsSourceQueue(t *testing.T) {
	legacyQueue := "https://sqs.example/legacy"
	zapCashQueue := "https://sqs.example/zapcash"

	// A client's shared worker pool can receive work from both queues. Queue
	// identity must travel with each message rather than being captured when
	// the pool's first worker starts.
	work := []services.MessageWrapper{
		{QueueURL: legacyQueue},
		{QueueURL: zapCashQueue},
	}

	if work[0].QueueURL != legacyQueue {
		t.Fatalf("legacy work queue = %q, want %q", work[0].QueueURL, legacyQueue)
	}
	if work[1].QueueURL != zapCashQueue {
		t.Fatalf("ZapCash work queue = %q, want %q", work[1].QueueURL, zapCashQueue)
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
	for _, tc := range []struct {
		name string
		raw  string
		want int
	}{
		{
			name: "string maxReceiveCount",
			raw:  `{"deadLetterTargetArn":"arn:aws:sqs:ap-south-1:123:wp-dlq","maxReceiveCount":"5"}`,
			want: 5,
		},
		{
			name: "numeric maxReceiveCount",
			raw:  `{"deadLetterTargetArn":"arn:aws:sqs:ap-south-1:123:wp-dlq","maxReceiveCount":5}`,
			want: 5,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := services.ParseRedriveMaxReceiveCount(tc.raw)
			if err != nil || got != tc.want {
				t.Fatalf("got = %d, %v want %d", got, err, tc.want)
			}
		})
	}

	for _, raw := range []string{
		``,
		`not-json`,
		`{"maxReceiveCount":"5"}`,
		`{"deadLetterTargetArn":"arn:dlq","maxReceiveCount":"0"}`,
		`{"deadLetterTargetArn":"arn:dlq","maxReceiveCount":0}`,
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

func TestZapCashDirectInputAuditTableLeavesHandlerChannelsAlone(t *testing.T) {
	prev := config.Configs
	t.Cleanup(func() { config.Configs = prev })
	config.Configs.SdkSmsInputTable = "SmsInputAuditTable"
	config.Configs.SdkEmailInputTable = "EmailInputAuditTable"
	config.Configs.SdkWhatsappInputTable = "WhatsappInputAuditTable"
	config.Configs.SdkRcsInputTable = "RcsInputAuditTable"

	if got := services.ZapCashDirectInputAuditTable(variables.SMS); got != "SmsInputAuditTable" {
		t.Fatalf("SMS input table = %q", got)
	}
	if got := services.ZapCashDirectInputAuditTable(variables.Email); got != "EmailInputAuditTable" {
		t.Fatalf("email input table = %q", got)
	}
	if got := services.ZapCashDirectInputAuditTable(variables.WhatsApp); got != "" {
		t.Fatalf("WhatsApp input table = %q, want handler-owned", got)
	}
	if got := services.ZapCashDirectInputAuditTable(variables.RCS); got != "" {
		t.Fatalf("RCS input table = %q, want handler-owned", got)
	}
}

func TestLenderEventClaimAppliesToPushOnEveryQueue(t *testing.T) {
	prev := config.Configs
	t.Cleanup(func() { config.Configs = prev })
	config.Configs.AwsZapCashQueueUrl = "https://sqs.example/zapcash"

	pushMsg := sdkModels.CommApiRequestBody{Channel: variables.PUSH}
	whatsapp := sdkModels.CommApiRequestBody{Channel: variables.WhatsApp}
	if !services.LenderEventClaimApplies(pushMsg, "https://sqs.example/other") {
		t.Fatal("PUSH on a non-direct queue must claim EventId")
	}
	if services.LenderEventClaimApplies(whatsapp, "https://sqs.example/other") {
		t.Fatal("WhatsApp on the shared queue must stay on sdk.Send")
	}
	if !services.LenderEventClaimApplies(whatsapp, "https://sqs.example/zapcash") {
		t.Fatal("WhatsApp on the ZapCash queue must claim EventId")
	}
	marketing := sdkModels.CommApiRequestBody{Channel: variables.WhatsApp, Source: "marketing", SourceRowId: 9}
	if services.LenderEventClaimApplies(marketing, "https://sqs.example/zapcash") {
		t.Fatal("marketing WhatsApp keeps its own claim")
	}
	if !services.PushRedeliveryWaits(variables.PUSH, "", "") {
		t.Fatal("a live PUSH claim must keep the SQS message")
	}
	if services.PushRedeliveryWaits(variables.PUSH, "fcm-1", "") || services.PushRedeliveryWaits(variables.WhatsApp, "", "") {
		t.Fatal("a sealed PUSH claim and other channels are not an in-flight wait")
	}
}

func TestZapCashPushClaimSealStopsBlankRedelivery(t *testing.T) {
	txn, errMsg := services.ZapCashPushClaimSeal(push.Result{
		AckSQS:    true,
		Submitted: 1,
		OutputAudits: []map[string]interface{}{
			{"ProviderMessageId": " fcm-1 "},
		},
	})
	if txn != "fcm-1" || errMsg != "" {
		t.Fatalf("submitted seal = %q %q", txn, errMsg)
	}
	if services.ZapCashBlankClaimShouldReclaim(txn, errMsg, 2) {
		t.Fatal("a submitted PUSH claim must not be reclaimed")
	}

	txn, errMsg = services.ZapCashPushClaimSeal(push.Result{AckSQS: true, Skipped: 2})
	if txn != "" || errMsg == "" {
		t.Fatalf("skipped seal = %q %q", txn, errMsg)
	}
	if services.ZapCashBlankClaimShouldReclaim(txn, errMsg, 2) {
		t.Fatal("a terminal PUSH skip must not be reclaimed")
	}

	txn, errMsg = services.ZapCashPushClaimSeal(push.Result{AckSQS: false, Submitted: 1})
	if txn != "" || errMsg != "" {
		t.Fatalf("unfinished seal = %q %q, want empty", txn, errMsg)
	}
}

func TestZapCashBlankClaimShouldReclaim(t *testing.T) {
	if services.ZapCashBlankClaimShouldReclaim("", "", 1) {
		t.Fatal("first receive must keep a blank in-flight claim")
	}
	if !services.ZapCashBlankClaimShouldReclaim("", "", 2) {
		t.Fatal("redelivery must reclaim a blank claim")
	}
	if services.ZapCashBlankClaimShouldReclaim("txn-1", "", 2) || services.ZapCashBlankClaimShouldReclaim("", "vendor off", 2) {
		t.Fatal("a recorded outcome must stay")
	}
}
