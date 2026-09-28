package monitoring_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func TestProductionPayloadOmitsMonitoringMetadata(t *testing.T) {
	raw, err := json.Marshal(sdkModels.CommApiRequestBody{Client: "zapcash", Channel: "SMS"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "isMonitorCopy") || strings.Contains(string(raw), "monitorVariantId") {
		t.Fatalf("ordinary production payload shape changed: %s", raw)
	}
}
