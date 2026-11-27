package apiServices_test

import (
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/internal/services/apiServices"
	sdkHelper "github.com/wecredit/communication-sdk/sdk/helper"
)

func TestDefaultMaterializeHardCapIs250k(t *testing.T) {
	if sdkHelper.DefaultMaterializeHardCap != 250000 {
		t.Fatalf("DefaultMaterializeHardCap = %d, want 250000", sdkHelper.DefaultMaterializeHardCap)
	}
}

func TestBeginPreviewJobCancelsPrior(t *testing.T) {
	s := apiServices.NewQueryCampaignService(nil, nil)
	started1 := time.Now().UTC()
	ctx1, gen1 := s.BeginPreviewJobForTest(42, started1)
	if gen1 == 0 {
		t.Fatal("expected non-zero generation")
	}
	select {
	case <-ctx1.Done():
		t.Fatal("first preview context should still be live")
	default:
	}

	started2 := started1.Add(time.Second)
	ctx2, gen2 := s.BeginPreviewJobForTest(42, started2)
	if gen2 <= gen1 {
		t.Fatalf("generation should increase: got %d after %d", gen2, gen1)
	}
	select {
	case <-ctx1.Done():
	case <-time.After(time.Second):
		t.Fatal("first preview context should be cancelled when a new job starts")
	}
	select {
	case <-ctx2.Done():
		t.Fatal("second preview context should still be live")
	default:
	}
	if !s.IsCurrentPreviewJobForTest(42, gen2) {
		t.Fatal("expected gen2 to be current")
	}
	if s.IsCurrentPreviewJobForTest(42, gen1) {
		t.Fatal("gen1 must not remain current after replace")
	}

	s.CancelPreviewJobForTest(42)
	select {
	case <-ctx2.Done():
	case <-time.After(time.Second):
		t.Fatal("cancel should abort the current preview context")
	}
	if s.IsCurrentPreviewJobForTest(42, gen2) {
		t.Fatal("cancelled job should be removed from the registry")
	}
}
