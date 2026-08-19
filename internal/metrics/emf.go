package metrics

import (
	"encoding/json"
	"time"

	"github.com/wecredit/communication-sdk/sdk/utils"
)

const namespace = "WeCredit/CommunicationSDK"

// Count emits a CloudWatch Embedded Metric Format log line (no extra AWS SDK dep).
func Count(name, vendor, client string, value int) {
	if value == 0 {
		return
	}
	payload := map[string]interface{}{
		"_aws": map[string]interface{}{
			"Timestamp": time.Now().UnixMilli(),
			"CloudWatchMetrics": []map[string]interface{}{
				{
					"Namespace":  namespace,
					"Dimensions": [][]string{{"Vendor", "Client"}},
					"Metrics": []map[string]string{
						{"Name": name, "Unit": "Count"},
					},
				},
			},
		},
		"Vendor": vendor,
		"Client": client,
		name:     value,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	utils.Info(string(raw))
}
