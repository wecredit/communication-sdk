package consumerServices_test

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/wecredit/communication-sdk/internal/database"
	services "github.com/wecredit/communication-sdk/internal/services/consumerServices"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

type whatsappTestState struct {
	assignCalls  int
	sendCalls    int
	updateCalls  int
	outputCalls  int
	trackCalls   int
	deleteCalls  int
	releaseCalls int
	blankCalls   int
	outcome      string
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
		WriteOutput: func(map[string]interface{}) error {
			state.outputCalls++
			return nil
		},
		Track: func(_ sdkModels.CommApiRequestBody, outcome, _, _ string) error {
			state.trackCalls++
			state.outcome = outcome
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

func TestMarketingWhatsappTerminalOutcomesAcknowledgeAfterTracking(t *testing.T) {
	tests := []struct {
		name          string
		configure     func(*services.MarketingWhatsappDependencies)
		wantOutcome   string
		wantSend      int
		wantUpdate    int
		wantOutput    int
		wantProcessed bool
		wantDeleted   bool
	}{
		{
			name:          "sent",
			configure:     func(*services.MarketingWhatsappDependencies) {},
			wantOutcome:   database.DispatchTrackingSent,
			wantSend:      1,
			wantOutput:    1,
			wantProcessed: true,
			wantDeleted:   true,
		},
		{
			name: "processed terminal rejection",
			configure: func(deps *services.MarketingWhatsappDependencies) {
				deps.Send = func(sdkModels.CommApiRequestBody) (bool, map[string]interface{}, error) {
					// This replacement is used only to control the provider outcome.
					return true, map[string]interface{}{"IsSent": false, "ResponseMessage": "template rejected"}, nil
				}
			},
			wantOutcome:   database.DispatchTrackingFailed,
			wantSend:      1,
			wantUpdate:    1,
			wantOutput:    1,
			wantProcessed: true,
			wantDeleted:   true,
		},
		{
			name: "inactive vendor",
			configure: func(deps *services.MarketingWhatsappDependencies) {
				deps.Assign = func(*sdkModels.CommApiRequestBody) bool { return false }
			},
			wantOutcome:   database.DispatchTrackingFailed,
			wantUpdate:    1,
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
			wantOutcome:   database.DispatchTrackingSkippedDuplicate,
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
			wantOutcome:   database.DispatchTrackingSent,
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
			if state.outcome != test.wantOutcome || state.updateCalls != test.wantUpdate || state.outputCalls != test.wantOutput {
				t.Fatalf("state outcome/update/output = (%q, %d, %d)", state.outcome, state.updateCalls, state.outputCalls)
			}
			if state.sendCalls != test.wantSend {
				t.Fatalf("send calls = %d, want %d", state.sendCalls, test.wantSend)
			}
			if state.deleteCalls != 1 || state.trackCalls != 1 {
				t.Fatalf("delete/track calls = (%d, %d), want (1, 1)", state.deleteCalls, state.trackCalls)
			}
		})
	}
}

func TestMarketingWhatsappRetryBlankAndTrackingFailureDoNotAcknowledge(t *testing.T) {
	t.Run("retryable send", func(t *testing.T) {
		state := &whatsappTestState{}
		deps := marketingWhatsappTestDependencies(state)
		deps.Send = func(sdkModels.CommApiRequestBody) (bool, map[string]interface{}, error) {
			state.sendCalls++
			return false, nil, errors.New("timeout")
		}
		processed, deleted := services.HandleMarketingWhatsappWithDependencies(marketingWhatsappTestData(), nil, nil, 5, deps)
		if processed || deleted || state.releaseCalls != 1 || state.trackCalls != 0 || state.deleteCalls != 0 {
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
		if processed || deleted || state.blankCalls != 1 || state.sendCalls != 0 || state.trackCalls != 0 || state.deleteCalls != 0 {
			t.Fatalf("unexpected blank state: %+v, result=(%t,%t)", state, processed, deleted)
		}
	})

	t.Run("tracking failure", func(t *testing.T) {
		state := &whatsappTestState{}
		deps := marketingWhatsappTestDependencies(state)
		configuredSend := deps.Send
		deps.Send = func(data sdkModels.CommApiRequestBody) (bool, map[string]interface{}, error) {
			state.sendCalls++
			return configuredSend(data)
		}
		deps.Track = func(sdkModels.CommApiRequestBody, string, string, string) error {
			state.trackCalls++
			return errors.New("tracking unavailable")
		}
		processed, deleted := services.HandleMarketingWhatsappWithDependencies(marketingWhatsappTestData(), nil, nil, 5, deps)
		if processed || deleted || state.sendCalls != 1 || state.trackCalls != 1 || state.deleteCalls != 0 {
			t.Fatalf("unexpected tracking failure state: %+v, result=(%t,%t)", state, processed, deleted)
		}
	})
}

func TestTerminalRejectionTrackingRetryDoesNotRepeatVendorCall(t *testing.T) {
	state := &whatsappTestState{}
	deps := marketingWhatsappTestDependencies(state)
	deps.Send = func(sdkModels.CommApiRequestBody) (bool, map[string]interface{}, error) {
		state.sendCalls++
		return true, map[string]interface{}{"IsSent": false, "ResponseMessage": "rejected"}, nil
	}
	deps.Track = func(sdkModels.CommApiRequestBody, string, string, string) error {
		state.trackCalls++
		return errors.New("tracking unavailable")
	}
	services.HandleMarketingWhatsappWithDependencies(marketingWhatsappTestData(), nil, nil, 5, deps)
	if state.sendCalls != 1 || state.deleteCalls != 0 {
		t.Fatalf("first attempt state: %+v", state)
	}

	deps.Claim = func(sdkModels.CommApiRequestBody) (bool, bool, string, string, error) {
		return true, false, "", "rejected", nil
	}
	deps.Track = func(sdkModels.CommApiRequestBody, string, string, string) error {
		state.trackCalls++
		return nil
	}
	processed, deleted := services.HandleMarketingWhatsappWithDependencies(marketingWhatsappTestData(), nil, nil, 5, deps)
	if !processed || !deleted || state.sendCalls != 1 {
		t.Fatalf("redelivery repeated vendor or failed ack: %+v, result=(%t,%t)", state, processed, deleted)
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
