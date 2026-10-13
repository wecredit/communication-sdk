package consumerServices_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	services "github.com/wecredit/communication-sdk/internal/services/consumerServices"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func TestReceiveWakesWhenChannelBufferIsAdded(t *testing.T) {
	handler := services.NewClientRoutine(1)
	handler.BufferFor("WHATSAPP", 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan services.MessageWrapper, 1)
	go func() {
		msg, ok := handler.Receive(ctx)
		if ok {
			got <- msg
		}
	}()

	time.Sleep(50 * time.Millisecond)
	handler.BufferFor("SMS", 1)
	handler.Enqueue(ctx, "SMS", messageFor("SMS", "sms-1"))

	select {
	case msg := <-got:
		if msg.Payload.CommId != "sms-1" {
			t.Fatalf("received CommId %q", msg.Payload.CommId)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SMS message was not received after its buffer was added")
	}
}

func TestReceiveStillReadsTheOriginalChannel(t *testing.T) {
	handler := services.NewClientRoutine(1)
	handler.BufferFor("WHATSAPP", 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler.Enqueue(ctx, "WHATSAPP", messageFor("WHATSAPP", "wa-1"))

	msg, ok := handler.Receive(ctx)
	if !ok {
		t.Fatal("receive ended before the WhatsApp message")
	}
	if msg.Payload.CommId != "wa-1" {
		t.Fatalf("received CommId %q", msg.Payload.CommId)
	}
}

func TestPoolSMSOnlyDeliversEveryMessage(t *testing.T) {
	assertPoolDelivers(t, []string{"SMS"}, 8, 250)
}

func TestPoolWhatsAppOnlyDeliversEveryMessage(t *testing.T) {
	assertPoolDelivers(t, []string{"WHATSAPP"}, 8, 250)
}

func TestPoolSMSAndWhatsAppSimultaneousLoad(t *testing.T) {
	assertPoolDelivers(t, []string{"SMS", "WHATSAPP"}, 8, 250)
}

func TestPoolWhatsAppParkedThenSMSArrives(t *testing.T) {
	assertSecondChannelWakesParkedWorkers(t, "WHATSAPP", "SMS")
}

func TestPoolSMSParkedThenWhatsAppArrives(t *testing.T) {
	assertSecondChannelWakesParkedWorkers(t, "SMS", "WHATSAPP")
}

func TestFullSMSBufferDoesNotBlockWhatsApp(t *testing.T) {
	handler := services.NewClientRoutine(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler.BufferFor("SMS", 1)
	handler.BufferFor("WHATSAPP", 1)
	handler.Enqueue(ctx, "SMS", messageFor("SMS", "sms-1"))

	smsDone := make(chan struct{})
	go func() {
		handler.Enqueue(ctx, "SMS", messageFor("SMS", "sms-2"))
		close(smsDone)
	}()
	select {
	case <-smsDone:
		t.Fatal("second SMS enqueue should wait while its buffer is full")
	case <-time.After(50 * time.Millisecond):
	}

	waDone := make(chan struct{})
	go func() {
		handler.Enqueue(ctx, "WHATSAPP", messageFor("WHATSAPP", "wa-1"))
		close(waDone)
	}()
	select {
	case <-waDone:
	case <-time.After(time.Second):
		t.Fatal("WhatsApp enqueue blocked on a full SMS buffer")
	}

	incoming := make(chan services.MessageWrapper, 3)
	go func() {
		for {
			msg, ok := handler.Receive(ctx)
			if !ok {
				return
			}
			incoming <- msg
		}
	}()

	got := make(map[string]bool, 3)
	deadline := time.After(2 * time.Second)
	for len(got) < 3 {
		select {
		case msg := <-incoming:
			if got[msg.Payload.CommId] {
				t.Fatalf("duplicate delivery %s", msg.Payload.CommId)
			}
			got[msg.Payload.CommId] = true
		case <-deadline:
			t.Fatalf("delivered %v", got)
		}
	}
	select {
	case <-smsDone:
	case <-time.After(time.Second):
		t.Fatal("blocked SMS enqueue did not finish after its buffer drained")
	}
}

func assertPoolDelivers(t *testing.T, channels []string, workers, perChannel int) {
	t.Helper()
	handler := services.NewClientRoutine(workers)
	got, stop := startPoolReceivers(handler, workers)
	defer stop()

	time.Sleep(30 * time.Millisecond)

	var producers sync.WaitGroup
	for _, channel := range channels {
		producers.Add(1)
		go func(channel string) {
			defer producers.Done()
			for i := 0; i < perChannel; i++ {
				handler.BufferFor(channel, 64)
				handler.Enqueue(context.Background(), channel, messageFor(channel, fmt.Sprintf("%s-%d", channel, i)))
			}
		}(channel)
	}
	producers.Wait()

	want := len(channels) * perChannel
	seen := collectExactIDs(t, got, want)
	if len(seen) != want {
		t.Fatalf("delivered %d, want %d", len(seen), want)
	}
	select {
	case id := <-got:
		t.Fatalf("extra delivery %s", id)
	case <-time.After(100 * time.Millisecond):
	}
}

func assertSecondChannelWakesParkedWorkers(t *testing.T, first, second string) {
	t.Helper()
	const perChannel = 100
	handler := services.NewClientRoutine(4)
	handler.BufferFor(first, 64)
	got, stop := startPoolReceivers(handler, 4)
	defer stop()

	time.Sleep(50 * time.Millisecond)

	var producers sync.WaitGroup
	producers.Add(2)
	go func() {
		defer producers.Done()
		for i := 0; i < perChannel; i++ {
			handler.Enqueue(context.Background(), first, messageFor(first, fmt.Sprintf("%s-%d", first, i)))
		}
	}()
	go func() {
		defer producers.Done()
		handler.BufferFor(second, 64)
		for i := 0; i < perChannel; i++ {
			handler.Enqueue(context.Background(), second, messageFor(second, fmt.Sprintf("%s-%d", second, i)))
		}
	}()
	producers.Wait()

	seen := collectExactIDs(t, got, perChannel*2)
	if len(seen) != perChannel*2 {
		t.Fatalf("delivered %d, want %d", len(seen), perChannel*2)
	}
	select {
	case id := <-got:
		t.Fatalf("extra delivery %s", id)
	case <-time.After(100 * time.Millisecond):
	}
}

func startPoolReceivers(handler *services.ClientRoutine, workers int) (<-chan string, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan string, 8192)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				msg, ok := handler.Receive(ctx)
				if !ok {
					return
				}
				got <- msg.Payload.CommId
			}
		}()
	}
	return got, func() {
		cancel()
		wg.Wait()
	}
}

func collectExactIDs(t *testing.T, got <-chan string, want int) map[string]int {
	t.Helper()
	seen := make(map[string]int, want)
	deadline := time.After(5 * time.Second)
	for len(seen) < want {
		select {
		case id := <-got:
			seen[id]++
			if seen[id] > 1 {
				t.Fatalf("duplicate delivery %s", id)
			}
		case <-deadline:
			t.Fatalf("delivered %d of %d", len(seen), want)
		}
	}
	return seen
}

func messageFor(channel, commID string) services.MessageWrapper {
	return services.MessageWrapper{Payload: sdkModels.CommApiRequestBody{Channel: channel, CommId: commID}}
}
