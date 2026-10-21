package whatsapp_test

import (
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/internal/channels/whatsapp/vendormetrics"
)

func TestVendorMetricsRecord(t *testing.T) {
	vendormetrics.ResetForTest()
	vendormetrics.Record("TIMES", 12*time.Millisecond, true)
	vendormetrics.Record("times", 8*time.Millisecond, false)
	sends, accepted := vendormetrics.SnapshotForTest("TIMES")
	if sends != 2 || accepted != 1 {
		t.Fatalf("sends=%d accepted=%d", sends, accepted)
	}
}