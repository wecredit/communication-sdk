package monitoring_test

import (
	"strings"
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/internal/services/monitoring"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func TestVariantKeyAndISTExpiry(t *testing.T) {
	ist := time.FixedZone("IST", 5*60*60+30*60)
	result := monitoring.AcceptedResult{
		Payload:          sdkModels.CommApiRequestBody{Channel: "rcs", Stage: 12.03},
		ResolvedVendor:   "PINNACLE",
		ResolvedTemplate: "template/one",
	}
	key := monitoring.VariantKey(time.Date(2026, 9, 1, 23, 0, 0, 0, ist), result, "9899074649")
	for _, expected := range []string{"2026-09-01", "RCS", "12.03", "PINNACLE", "template%2Fone", "9899074649"} {
		if !strings.Contains(key, expected) {
			t.Fatalf("key %q missing %q", key, expected)
		}
	}
	if ttl := monitoring.ReservationTTL(time.Date(2026, 9, 1, 23, 59, 0, 0, ist)); ttl != 6*time.Minute {
		t.Fatalf("expected expiry at 00:05 IST, got %v", ttl)
	}
}
