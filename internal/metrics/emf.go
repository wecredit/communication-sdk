package metrics

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/wecredit/communication-sdk/sdk/utils"
)

const namespace = "WeCredit/CommunicationSDK"

type channelMeasurement struct {
	name    string
	unit    string
	client  string
	channel string
	count   int64
	sum     float64
	max     float64
}

type counterMeasurement struct {
	name   string
	vendor string
	client string
	value  int64
}

type reasonCounterMeasurement struct {
	name   string
	lane   string
	reason string
	value  int64
}

var (
	channelMeasurementMu sync.Mutex
	channelMeasurements  = make(map[string]*channelMeasurement)
	counterMeasurementMu sync.Mutex
	counterMeasurements  = make(map[string]*counterMeasurement)
	reasonCounterMu      sync.Mutex
	reasonCounters       = make(map[string]*reasonCounterMeasurement)
	metricFlushOnce      sync.Once
)

// Count accumulates hot-path counters in memory and emits one CloudWatch EMF
// record per metric/dimension tuple each minute.
func Count(name, vendor, client string, value int) {
	if value == 0 {
		return
	}

	vendor = strings.TrimSpace(vendor)
	client = strings.ToLower(strings.TrimSpace(client))
	key := name + "\x00" + vendor + "\x00" + client
	counterMeasurementMu.Lock()
	m := counterMeasurements[key]
	if m == nil {
		m = &counterMeasurement{name: name, vendor: vendor, client: client}
		counterMeasurements[key] = m
	}

	m.value += int64(value)
	counterMeasurementMu.Unlock()
	startMetricFlusher()
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

// GaugeByChannel emits a gauge immediately. Callers use it from their
// once-per-minute writer monitor, so it does not create per-message logs.
func GaugeByChannel(name, unit, client, channel string, value float64) {
	if unit == "" {
		unit = "None"
	}
	payload := map[string]interface{}{
		"_aws": map[string]interface{}{
			"Timestamp": time.Now().UnixMilli(),
			"CloudWatchMetrics": []map[string]interface{}{{
				"Namespace":  namespace,
				"Dimensions": [][]string{{"Client", "Channel"}},
				"Metrics":    []map[string]string{{"Name": name, "Unit": unit}},
			}},
		},
		"Client":  strings.ToLower(strings.TrimSpace(client)),
		"Channel": strings.ToUpper(strings.TrimSpace(channel)),
		name:      value,
	}

	raw, err := json.Marshal(payload)
	if err == nil {
		utils.Info(string(raw))
	}
}

// ValueByChannel aggregates hot-path measurements and emits one summary per
// minute, avoiding one CloudWatch log event per message.
func ValueByChannel(name, unit, client, channel string, value float64) {
	if unit == "" {
		unit = "None"
	}

	client = strings.ToLower(strings.TrimSpace(client))
	channel = strings.ToUpper(strings.TrimSpace(channel))
	key := name + "\x00" + client + "\x00" + channel
	channelMeasurementMu.Lock()
	m := channelMeasurements[key]
	if m == nil {
		m = &channelMeasurement{name: name, unit: unit, client: client, channel: channel}
		channelMeasurements[key] = m
	}

	m.count++
	m.sum += value
	if m.count == 1 || value > m.max {
		m.max = value
	}

	channelMeasurementMu.Unlock()

	startMetricFlusher()
}

func startMetricFlusher() {
	metricFlushOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				flushCounterMeasurements()
				flushReasonCounters()
				flushChannelMeasurements()
			}
		}()
	})
}

func flushCounterMeasurements() {
	counterMeasurementMu.Lock()
	snapshot := counterMeasurements
	counterMeasurements = make(map[string]*counterMeasurement)
	counterMeasurementMu.Unlock()

	for _, m := range snapshot {
		if m.value != 0 {
			emit(m.name, "Count", m.vendor, m.client, m.value)
		}
	}
}

func flushReasonCounters() {
	reasonCounterMu.Lock()
	snapshot := reasonCounters
	reasonCounters = make(map[string]*reasonCounterMeasurement)
	reasonCounterMu.Unlock()

	for _, m := range snapshot {
		if m.value != 0 {
			emitReason(m.name, m.lane, m.reason, m.value)
		}
	}
}

func flushChannelMeasurements() {
	channelMeasurementMu.Lock()
	snapshot := channelMeasurements
	channelMeasurements = make(map[string]*channelMeasurement)
	channelMeasurementMu.Unlock()

	for _, m := range snapshot {
		if m.count == 0 {
			continue
		}
		averageName := m.name + "Average"
		maxName := m.name + "Max"
		countName := m.name + "Samples"
		payload := map[string]interface{}{
			"_aws": map[string]interface{}{
				"Timestamp": time.Now().UnixMilli(),
				"CloudWatchMetrics": []map[string]interface{}{{
					"Namespace":  namespace,
					"Dimensions": [][]string{{"Client", "Channel"}},
					"Metrics": []map[string]string{
						{"Name": averageName, "Unit": m.unit},
						{"Name": maxName, "Unit": m.unit},
						{"Name": countName, "Unit": "Count"},
					},
				}},
			},
			"Client":    m.client,
			"Channel":   m.channel,
			averageName: m.sum / float64(m.count),
			maxName:     m.max,
			countName:   m.count,
		}

		raw, err := json.Marshal(payload)
		if err == nil {
			utils.Info(string(raw))
		}
	}
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

// CountByReason accumulates a low-cardinality operational counter for a queue
// lane and reason. Message identifiers never belong in metric dimensions.
func CountByReason(name, lane, reason string, value int) {
	if value == 0 {
		return
	}

	lane = strings.ToLower(strings.TrimSpace(lane))
	reason = strings.ToLower(strings.TrimSpace(reason))
	key := name + "\x00" + lane + "\x00" + reason
	reasonCounterMu.Lock()
	m := reasonCounters[key]
	if m == nil {
		m = &reasonCounterMeasurement{name: name, lane: lane, reason: reason}
		reasonCounters[key] = m
	}

	m.value += int64(value)
	reasonCounterMu.Unlock()
	startMetricFlusher()
}

func emitReason(name, lane, reason string, value int64) {
	payload := map[string]interface{}{
		"_aws": map[string]interface{}{
			"Timestamp": time.Now().UnixMilli(),
			"CloudWatchMetrics": []map[string]interface{}{
				{
					"Namespace":  namespace,
					"Dimensions": [][]string{{"Lane", "Reason"}},
					"Metrics": []map[string]string{
						{"Name": name, "Unit": "Count"},
					},
				},
			},
		},
		"Lane":   lane,
		"Reason": reason,
		name:     value,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	utils.Info(string(raw))
}
