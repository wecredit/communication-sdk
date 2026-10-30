package consumerServices_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	redislib "github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/config"
	"github.com/wecredit/communication-sdk/internal/persistence"
	services "github.com/wecredit/communication-sdk/internal/services/consumerServices"
)

type laneLengthRedis struct {
	mu      sync.Mutex
	lengths map[string]int64
}

func (f *laneLengthRedis) XLen(_ context.Context, stream string) *redislib.IntCmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	return redislib.NewIntResult(f.lengths[stream], nil)
}

func (f *laneLengthRedis) Eval(context.Context, string, []string, ...interface{}) *redislib.Cmd {
	return redislib.NewCmdResult("1-0", nil)
}

func TestDedicatedQueueLaneMappingIsIndependent(t *testing.T) {
	old := config.Configs
	t.Cleanup(func() { config.Configs = old })
	config.Configs.AwsWeCreditSmsQueueUrl = "https://sqs.example/sms"
	config.Configs.AwsWeCreditWhatsappQueueUrl = "https://sqs.example/whatsapp"

	client, channel := services.QueuePersistenceLaneForTest(config.Configs.AwsWeCreditSmsQueueUrl)
	if client != "wecredit" || channel != "sms" {
		t.Fatalf("SMS lane = %s:%s", client, channel)
	}
	client, channel = services.QueuePersistenceLaneForTest(config.Configs.AwsWeCreditWhatsappQueueUrl)
	if client != "wecredit" || channel != "whatsapp" {
		t.Fatalf("WhatsApp lane = %s:%s", client, channel)
	}
	client, channel = services.QueuePersistenceLaneForTest("https://sqs.example/unrelated")
	if client != "" || channel != "" {
		t.Fatalf("unrelated queue mapped to %s:%s", client, channel)
	}
}

func TestVisibilityGuardRenewsBeforeReceiveVisibilityExpires(t *testing.T) {
	calls := make(chan *sqs.ChangeMessageVisibilityInput, 1)
	restore := services.SetVisibilityHeartbeatForTest(10*time.Millisecond, 0,
		func(_ context.Context, _ *sqs.SQS, input *sqs.ChangeMessageVisibilityInput) error {
			calls <- input
			return nil
		})
	t.Cleanup(restore)

	ctx, cancel := context.WithCancel(context.Background())
	stop := services.StartVisibilityGuardForTest(ctx, "queue-url", &sqs.Message{ReceiptHandle: aws.String("receipt")}, "wecredit:sms")
	defer stop()
	select {
	case input := <-calls:
		if aws.StringValue(input.QueueUrl) != "queue-url" || aws.StringValue(input.ReceiptHandle) != "receipt" || aws.Int64Value(input.VisibilityTimeout) != 300 {
			t.Fatalf("unexpected visibility input: %+v", input)
		}
	case <-time.After(time.Second):
		t.Fatal("visibility was not renewed")
	}
	cancel()
}

func TestVisibilityGuardReleaseMakesAbandonedMessageVisible(t *testing.T) {
	calls := make(chan *sqs.ChangeMessageVisibilityInput, 1)
	restore := services.SetVisibilityHeartbeatForTest(time.Hour, 0,
		func(_ context.Context, _ *sqs.SQS, input *sqs.ChangeMessageVisibilityInput) error {
			calls <- input
			return nil
		})
	t.Cleanup(restore)

	stop, release := services.StartVisibilityGuardControlsForTest(
		context.Background(), "queue-url", &sqs.Message{ReceiptHandle: aws.String("receipt")}, "wecredit:sms",
	)
	release()
	stop() // idempotent after release
	select {
	case input := <-calls:
		if aws.Int64Value(input.VisibilityTimeout) != 0 {
			t.Fatalf("release visibility = %d, want 0", aws.Int64Value(input.VisibilityTimeout))
		}
	case <-time.After(time.Second):
		t.Fatal("abandoned message visibility was not released")
	}
}

func TestClosingClientBufferReleasesPrefetchedVisibility(t *testing.T) {
	calls := make(chan *sqs.ChangeMessageVisibilityInput, 1)
	restore := services.SetVisibilityHeartbeatForTest(time.Hour, 0,
		func(_ context.Context, _ *sqs.SQS, input *sqs.ChangeMessageVisibilityInput) error {
			calls <- input
			return nil
		})
	t.Cleanup(restore)

	handler := services.NewClientRoutine(1)
	handler.BufferFor("SMS", 1)
	wrapper := services.MessageWrapperWithVisibilityForTest(
		context.Background(), "queue-url", &sqs.Message{ReceiptHandle: aws.String("receipt")}, "wecredit:sms",
	)
	if !handler.Enqueue(context.Background(), "SMS", wrapper) {
		t.Fatal("failed to enqueue prefetched message")
	}
	handler.CloseForTest()
	select {
	case input := <-calls:
		if aws.Int64Value(input.VisibilityTimeout) != 0 {
			t.Fatalf("shutdown visibility = %d, want 0", aws.Int64Value(input.VisibilityTimeout))
		}
	case <-time.After(time.Second):
		t.Fatal("buffered message was not released on close")
	}
}

func TestPausedDedicatedLanePreventsReceiveAndDoesNotBlockOtherLane(t *testing.T) {
	old := config.Configs
	t.Cleanup(func() { config.Configs = old })
	config.Configs.AwsWeCreditSmsQueueUrl = "https://sqs.example/sms"
	config.Configs.AwsWeCreditWhatsappQueueUrl = "https://sqs.example/whatsapp"
	config.Configs.CommAsyncWriteChannels = "wecredit:sms,wecredit:whatsapp"
	config.Configs.CommWriteStreamBudgetOverrides = "wecredit:sms:100,wecredit:whatsapp:100"

	fakeRedis := &laneLengthRedis{lengths: map[string]int64{
		persistence.StreamKey("wecredit", "sms"):      80,
		persistence.StreamKey("wecredit", "whatsapp"): 0,
	}}
	restoreProducer := persistence.SetProducerForTest(persistence.NewProducer(fakeRedis))
	t.Cleanup(restoreProducer)
	t.Cleanup(persistence.SetWriterActiveForTest("wecredit", "sms", true))
	t.Cleanup(persistence.SetWriterActiveForTest("wecredit", "whatsapp", true))
	t.Cleanup(persistence.SetAdmissionVerifyIntervalForTest(10 * time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var smsReceives atomic.Int32
	var whatsappReceives atomic.Int32
	restoreReceive := services.SetReceiveMessagesForTest(func(receiveCtx context.Context, _ *sqs.SQS, input *sqs.ReceiveMessageInput) (*sqs.ReceiveMessageOutput, error) {
		if err := receiveCtx.Err(); err != nil {
			return nil, err
		}
		switch aws.StringValue(input.QueueUrl) {
		case config.Configs.AwsWeCreditSmsQueueUrl:
			smsReceives.Add(1)
		case config.Configs.AwsWeCreditWhatsappQueueUrl:
			whatsappReceives.Add(1)
			cancel()
		}
		return &sqs.ReceiveMessageOutput{}, nil
	})
	t.Cleanup(restoreReceive)

	done := make(chan struct{}, 2)
	go func() {
		services.PollCommunicationQueueForTest(ctx, config.Configs.AwsWeCreditSmsQueueUrl)
		done <- struct{}{}
	}()
	go func() {
		services.PollCommunicationQueueForTest(ctx, config.Configs.AwsWeCreditWhatsappQueueUrl)
		done <- struct{}{}
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pollers did not stop")
	}
	<-done
	if got := smsReceives.Load(); got != 0 {
		t.Fatalf("paused SMS lane received %d times", got)
	}
	if got := whatsappReceives.Load(); got == 0 {
		t.Fatal("healthy WhatsApp lane was blocked by SMS pressure")
	}
}
