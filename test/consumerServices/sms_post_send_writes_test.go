package consumerServices_test

import (
	"errors"
	"testing"
	"time"

	services "github.com/wecredit/communication-sdk/internal/services/consumerServices"
)

func TestRunParallelSMSPostSendWritesStartsBothWrites(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	done := make(chan struct{})

	go func() {
		services.RunParallelSMSPostSendWrites(
			func() error {
				started <- "output"
				<-release
				return nil
			},
			func() error {
				started <- "tracking"
				<-release
				return nil
			},
		)
		close(done)
	}()

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case write := <-started:
			seen[write] = true
		case <-time.After(time.Second):
			t.Fatal("both writes did not start concurrently")
		}
	}
	close(release)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("parallel writes did not finish")
	}
}

func TestRunParallelSMSPostSendWritesReturnsSinkErrors(t *testing.T) {
	outputFailure := errors.New("output failed")
	trackingFailure := errors.New("tracking failed")

	outputErr, trackingErr := services.RunParallelSMSPostSendWrites(
		func() error { return outputFailure },
		func() error { return trackingFailure },
	)

	if !errors.Is(outputErr, outputFailure) {
		t.Fatalf("output error = %v, want %v", outputErr, outputFailure)
	}
	if !errors.Is(trackingErr, trackingFailure) {
		t.Fatalf("tracking error = %v, want %v", trackingErr, trackingFailure)
	}
}
