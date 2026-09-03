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
	emit(name, "Count", vendor, client, value)
}

// Value emits a CloudWatch Embedded Metric Format value with the supplied
// unit. It is used for measurements such as active PUSH claim age, which must
// not be accumulated as counters.
func Value(name, unit, vendor, client string, value float64) {
	if unit == "" {
		unit = "None"
	}
	emit(name, unit, vendor, client, value)
}

func emit(name, unit, vendor, client string, value interface{}) {
	payload := map[string]interface{}{
		"_aws": map[string]interface{}{
			"Timestamp": time.Now().UnixMilli(),
			"CloudWatchMetrics": []map[string]interface{}{
				{
					"Namespace":  namespace,
					"Dimensions": [][]string{{"Vendor", "Client"}},
					"Metrics": []map[string]string{
						{"Name": name, "Unit": unit},
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
