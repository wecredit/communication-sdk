package consumerServices_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	services "github.com/wecredit/communication-sdk/internal/services/consumerServices"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

type whatsappTestState struct {
	assignCalls  int
	sendCalls    int
	updateCalls  int
	inputCalls   int
	outputCalls  int
	deleteCalls  int
	releaseCalls int
	blankCalls   int
	lastOutput   map[string]interface{}
}

func marketingWhatsappTestData() sdkModels.CommApiRequestBody {
	return sdkModels.CommApiRequestBody{
		CommId:      "comm-1",
		EventId:     "marketing-42",
		Source:      "marketing",
		SourceRowId: 42,
		Channel:     "WHATSAPP",
		Client:      "wecredit",
		Vendor:      "SINCH",
		ProcessName: "WECREDIT",
	}
}

func marketingWhatsappTestDependencies(state *whatsappTestState) services.MarketingWhatsappDependencies {
	return services.MarketingWhatsappDependencies{
		Claim: func(sdkModels.CommApiRequestBody) (bool, bool, string, string, error) {
			return false, false, "", "", nil
		},
		Assign: func(*sdkModels.CommApiRequestBody) bool {
			state.assignCalls++
			return true
		},
		Send: func(sdkModels.CommApiRequestBody) (bool, map[string]interface{}, error) {
			return true, map[string]interface{}{
				"IsSent":          true,
				"TransactionId":   "txn-1",
				"ResponseMessage": "submitted",
			}, nil
		},
		UpdateError: func(sdkModels.CommApiRequestBody, string) error {
			state.updateCalls++
			return nil
		},
		WriteInputAudit: func(sdkModels.CommApiRequestBody, map[string]interface{}) error {
			state.inputCalls++
			return nil
		},
		WriteOutput: func(payload sdkModels.CommApiRequestBody, output map[string]interface{}) error {
			state.outputCalls++
			state.lastOutput = output
			return nil
		},
		Delete: func(sdkModels.CommApiRequestBody) (bool, error) {
			state.deleteCalls++
			return true, nil
		},
		Release: func(sdkModels.CommApiRequestBody) { state.releaseCalls++ },
		Blank:   func(sdkModels.CommApiRequestBody, *sqs.Message, int) { state.blankCalls++ },
	}
}

func outputIsSent(output map[string]interface{}) bool {
	if output == nil {
		return false
	}
	switch v := output["IsSent"].(type) {
	case bool:
		return v
	case int:
		return v == 1
	default:
		return false
	}
}

func TestMarketingWhatsappTerminalOutcomesAcknowledgeAfterOutput(t *testing.T) {
	tests := []struct {
		name          string
		configure     func(*services.MarketingWhatsappDependencies)
		wantSend      int
		wantUpdate    int
		wantOutput    int
		wantIsSent    *bool
		wantProcessed bool
		wantDeleted   bool
	}{
		{
			name:          "sent",
			configure:     func(*services.MarketingWhatsappDependencies) {},
			wantSend:      1,
			wantOutput:    1,
			wantIsSent:    boolPtr(true),
			wantProcessed: true,
			wantDeleted:   true,
		},
		{
			name: "processed terminal rejection",
			configure: func(deps *services.MarketingWhatsappDependencies) {
				deps.Send = func(sdkModels.CommApiRequestBody) (bool, map[string]interface{}, error) {
					return true, map[string]interface{}{"IsSent": false, "ResponseMessage": "template rejected"}, nil
				}
			},
			wantSend:      1,
			wantUpdate:    1,
			wantOutput:    1,
			wantIsSent:    boolPtr(false),
			wantProcessed: true,
			wantDeleted:   true,
		},
		{
			name: "inactive vendor",
			configure: func(deps *services.MarketingWhatsappDependencies) {
				deps.Assign = func(*sdkModels.CommApiRequestBody) bool { return false }
			},
			wantUpdate:    1,
			wantOutput:    1,
			wantIsSent:    boolPtr(false),
			wantProcessed: true,
			wantDeleted:   true,
		},
		{
			name: "campaign duplicate",
			configure: func(deps *services.MarketingWhatsappDependencies) {
				deps.Claim = func(sdkModels.CommApiRequestBody) (bool, bool, string, string, error) {
					return false, true, "", "", nil
				}
			},
			wantUpdate:    1,
			wantOutput:    1,
			wantIsSent:    boolPtr(false),
			wantProcessed: true,
			wantDeleted:   true,
		},
		{
			name: "terminal Redis duplicate",
			configure: func(deps *services.MarketingWhatsappDependencies) {
				deps.Claim = func(sdkModels.CommApiRequestBody) (bool, bool, string, string, error) {
					return true, false, "txn-existing", "", nil
				}
			},
			wantOutput:    1,
			wantIsSent:    boolPtr(true),
			wantProcessed: true,
			wantDeleted:   true,
		},
		{
			name: "terminal Redis duplicate output already recorded",
			configure: func(deps *services.MarketingWhatsappDependencies) {
				deps.Claim = func(sdkModels.CommApiRequestBody) (bool, bool, string, string, error) {
					return true, false, "txn-existing", "", nil
				}
				deps.OutputRecorded = func(int64) (bool, error) { return true, nil }
			},
			wantOutput:    0,
			wantProcessed: true,
			wantDeleted:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := &whatsappTestState{}
			deps := marketingWhatsappTestDependencies(state)
			test.configure(&deps)
			configuredSend := deps.Send
			deps.Send = func(data sdkModels.CommApiRequestBody) (bool, map[string]interface{}, error) {
				state.sendCalls++
				return configuredSend(data)
			}
			processed, deleted := services.HandleMarketingWhatsappWithDependencies(marketingWhatsappTestData(), nil, nil, 5, deps)
			if processed != test.wantProcessed || deleted != test.wantDeleted {
				t.Fatalf("result = (%t, %t), want (%t, %t)", processed, deleted, test.wantProcessed, test.wantDeleted)
			}
			if state.updateCalls != test.wantUpdate || state.outputCalls != test.wantOutput {
				t.Fatalf("state update/output = (%d, %d), want (%d, %d)", state.updateCalls, state.outputCalls, test.wantUpdate, test.wantOutput)
			}
			if state.sendCalls != test.wantSend {
				t.Fatalf("send calls = %d, want %d", state.sendCalls, test.wantSend)
			}
			wantInput := 0
			if test.wantSend > 0 {
				wantInput = 1 // input audit only on the vendor-send path (after Assign)
			}
			if state.inputCalls != wantInput {
				t.Fatalf("input audit calls = %d, want %d", state.inputCalls, wantInput)
			}
			if state.deleteCalls != 1 {
				t.Fatalf("delete calls = %d, want 1", state.deleteCalls)
			}
			if test.wantIsSent != nil && outputIsSent(state.lastOutput) != *test.wantIsSent {
				t.Fatalf("output IsSent = %v, want %v (output=%v)", outputIsSent(state.lastOutput), *test.wantIsSent, state.lastOutput)
			}
		})
	}
}

func boolPtr(v bool) *bool { return &v }

func TestMarketingWhatsappRetryBlankAndOutputFailureDoNotAcknowledge(t *testing.T) {
	t.Run("retryable send", func(t *testing.T) {
		state := &whatsappTestState{}
		deps := marketingWhatsappTestDependencies(state)
		deps.Send = func(sdkModels.CommApiRequestBody) (bool, map[string]interface{}, error) {
			state.sendCalls++
			return false, nil, errors.New("timeout")
		}
		processed, deleted := services.HandleMarketingWhatsappWithDependencies(marketingWhatsappTestData(), nil, nil, 5, deps)
		if processed || deleted || state.releaseCalls != 1 || state.outputCalls != 0 || state.deleteCalls != 0 {
			t.Fatalf("unexpected retry state: %+v, result=(%t,%t)", state, processed, deleted)
		}
	})

	t.Run("blank Redis claim", func(t *testing.T) {
		state := &whatsappTestState{}
		deps := marketingWhatsappTestDependencies(state)
		deps.Claim = func(sdkModels.CommApiRequestBody) (bool, bool, string, string, error) {
			return true, false, "", "", nil
		}
		processed, deleted := services.HandleMarketingWhatsappWithDependencies(marketingWhatsappTestData(), nil, nil, 5, deps)
		if processed || deleted || state.blankCalls != 1 || state.sendCalls != 0 || state.outputCalls != 0 || state.deleteCalls != 0 {
			t.Fatalf("unexpected blank state: %+v, result=(%t,%t)", state, processed, deleted)
		}
	})

	t.Run("output failure", func(t *testing.T) {
		state := &whatsappTestState{}
		deps := marketingWhatsappTestDependencies(state)
		configuredSend := deps.Send
		deps.Send = func(data sdkModels.CommApiRequestBody) (bool, map[string]interface{}, error) {
			state.sendCalls++
			return configuredSend(data)
		}
		deps.WriteOutput = func(sdkModels.CommApiRequestBody, map[string]interface{}) error {
			state.outputCalls++
			return errors.New("output unavailable")
		}
		processed, deleted := services.HandleMarketingWhatsappWithDependencies(marketingWhatsappTestData(), nil, nil, 5, deps)
		if processed || deleted || state.sendCalls != 1 || state.outputCalls != 1 || state.deleteCalls != 0 {
			t.Fatalf("unexpected output failure state: %+v, result=(%t,%t)", state, processed, deleted)
		}
	})
}

func TestTerminalRejectionOutputRetryDoesNotRepeatVendorCall(t *testing.T) {
	state := &whatsappTestState{}
	deps := marketingWhatsappTestDependencies(state)
	deps.Send = func(sdkModels.CommApiRequestBody) (bool, map[string]interface{}, error) {
		state.sendCalls++
		return true, map[string]interface{}{"IsSent": false, "ResponseMessage": "rejected"}, nil
	}
	deps.WriteOutput = func(sdkModels.CommApiRequestBody, map[string]interface{}) error {
		state.outputCalls++
		return errors.New("output unavailable")
	}
	services.HandleMarketingWhatsappWithDependencies(marketingWhatsappTestData(), nil, nil, 5, deps)
	if state.sendCalls != 1 || state.deleteCalls != 0 {
		t.Fatalf("first attempt state: %+v", state)
	}

	deps.Claim = func(sdkModels.CommApiRequestBody) (bool, bool, string, string, error) {
		return true, false, "", "rejected", nil
	}
	deps.WriteOutput = func(_ sdkModels.CommApiRequestBody, output map[string]interface{}) error {
		state.outputCalls++
		state.lastOutput = output
		return nil
	}
	processed, deleted := services.HandleMarketingWhatsappWithDependencies(marketingWhatsappTestData(), nil, nil, 5, deps)
	if !processed || !deleted || state.sendCalls != 1 {
		t.Fatalf("redelivery repeated vendor or failed ack: %+v, result=(%t,%t)", state, processed, deleted)
	}
	if outputIsSent(state.lastOutput) {
		t.Fatalf("redis terminal replay should write IsSent=false, got %v", state.lastOutput)
	}
}

func TestWhatsappDLQImminentUsesDiscoveredThreshold(t *testing.T) {
	msg := &sqs.Message{Attributes: map[string]*string{"ApproximateReceiveCount": aws.String("5")}}
	if !services.ShouldEmitWhatsappDLQImminent(msg, 3) {
		t.Fatal("expected imminent metric when receive count exceeds discovered threshold")
	}
	if services.ShouldEmitWhatsappDLQImminent(msg, 10) {
		t.Fatal("fired imminent metric before discovered threshold")
	}
}

func TestMarketingWhatsappWriteOutputSeesAssignedVendorAndResolvedCommId(t *testing.T) {
	state := &whatsappTestState{}
	var captured sdkModels.CommApiRequestBody
	deps := marketingWhatsappTestDependencies(state)
	deps.Assign = func(data *sdkModels.CommApiRequestBody) bool {
		data.Vendor = "TIMES"
		return true
	}
	deps.WriteOutput = func(payload sdkModels.CommApiRequestBody, output map[string]interface{}) error {
		captured = payload
		state.outputCalls++
		state.lastOutput = output
		return nil
	}
	data := marketingWhatsappTestData()
	data.CommId = ""
	data.Vendor = ""
	processed, deleted := services.HandleMarketingWhatsappWithDependencies(data, nil, nil, 5, deps)
	if !processed || !deleted {
		t.Fatalf("result=(%t,%t)", processed, deleted)
	}
	if captured.Vendor != "TIMES" {
		t.Fatalf("WriteOutput vendor=%q, want TIMES", captured.Vendor)
	}
	if strings.TrimSpace(captured.CommId) == "" {
		t.Fatal("WriteOutput CommId should be resolved")
	}
}
