package vapioSms_test

import (
	"strings"
	"testing"

	"github.com/wecredit/communication-sdk/internal/channels/sms/outcome"
	vapio "github.com/wecredit/communication-sdk/internal/channels/sms/vapio"
	vapiopayloads "github.com/wecredit/communication-sdk/internal/channels/sms/vapio/vapioPayloads"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	models "github.com/wecredit/communication-sdk/sdk/models"
)

func testConfig() models.Config {
	return models.Config{
		WeCreditVapioSmsUsername: "wecredit",
		WeCreditVapioSmsApiKey:   "secret",
		WeCreditVapioSmsRoute:    "Trans",
	}
}

func TestVapioPayloadUsesTenDigitMobileAndDLT(t *testing.T) {
	payload, err := vapiopayloads.GetTemplatePayload(extapimodels.SmsRequestBody{
		Mobile:         "917014850582",
		TemplateHeader: "WECRWP",
		TemplateText:   "Loan dekhein https://example.test WeCredit",
		DltTemplateId:  1707178333854271272,
	}, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if payload["mobile"] != "7014850582" {
		t.Fatalf("mobile = %q", payload["mobile"])
	}
	if payload["format"] != "json" {
		t.Fatalf("format = %q", payload["format"])
	}
	if payload["TID"] != "1707178333854271272" {
		t.Fatalf("TID = %q", payload["TID"])
	}
	if payload["senderid"] != "WECRWP" || payload["route"] != "Trans" || payload["username"] != "wecredit" {
		t.Fatalf("identity fields = %#v", map[string]string{
			"senderid": payload["senderid"], "route": payload["route"], "username": payload["username"],
		})
	}
	if strings.Contains(payload["text"], "{#var#}") {
		t.Fatal("payload rebuilt a placeholder that SendSmsByProcess should already have replaced")
	}
	if payload["apikey"] != "secret" {
		t.Fatal("apikey was not placed in the form body")
	}
}

func TestVapioPayloadRequiresTemplateHeader(t *testing.T) {
	_, err := vapiopayloads.GetTemplatePayload(extapimodels.SmsRequestBody{
		Mobile: "9876543210", TemplateText: "Hello", DltTemplateId: 1,
	}, testConfig())
	if err == nil || !strings.Contains(err.Error(), "template header is empty") {
		t.Fatalf("error = %v", err)
	}
}

func TestVapioPayloadRejectsEmptyText(t *testing.T) {
	_, err := vapiopayloads.GetTemplatePayload(extapimodels.SmsRequestBody{
		Mobile: "9876543210", TemplateHeader: "WECRWP", DltTemplateId: 1,
	}, testConfig())
	if err == nil || !strings.Contains(err.Error(), "message text is empty") {
		t.Fatalf("error = %v", err)
	}
}

func TestClassifyVapioSubmitted(t *testing.T) {
	status, message, txnID := vapio.ClassifyVapioResponse(map[string]interface{}{
		"status":        "OK",
		"msg-id":        "0VITWK8wDMRfg5t",
		"message":       "Message Submitted successfully",
		"ApistatusCode": 200,
	})
	if status != outcome.Submitted {
		t.Fatalf("outcome = %q", status)
	}
	if txnID != "0VITWK8wDMRfg5t" {
		t.Fatalf("transaction id = %q", txnID)
	}
	if message != "Message Submitted successfully" {
		t.Fatalf("message = %q", message)
	}
}

func TestClassifyVapioOtherStatusIsFinal(t *testing.T) {
	status, message, txnID := vapio.ClassifyVapioResponse(map[string]interface{}{
		"status":        "ERROR",
		"message":       "Invalid route",
		"ApistatusCode": 200,
	})
	if status != outcome.FailedFinal {
		t.Fatalf("outcome = %q", status)
	}
	if message != "Invalid route" || txnID != "" {
		t.Fatalf("message = %q txn = %q", message, txnID)
	}
}

func TestClassifyVapioMissingIdentityIsUnknown(t *testing.T) {
	status, _, _ := vapio.ClassifyVapioResponse(map[string]interface{}{
		"ApistatusCode": 200,
	})
	if status != outcome.Unknown {
		t.Fatalf("outcome = %q", status)
	}
}

func TestClassifyVapioHTTP429IsRetryable(t *testing.T) {
	status, _, _ := vapio.ClassifyVapioResponse(map[string]interface{}{
		"status":        "ERROR",
		"message":       "busy",
		"ApistatusCode": 429,
	})
	if status != outcome.FailedRetryable {
		t.Fatalf("outcome = %q", status)
	}
}

func TestClassifyVapioHTTP400IsFinal(t *testing.T) {
	status, _, _ := vapio.ClassifyVapioResponse(map[string]interface{}{
		"status":        "ERROR",
		"message":       "bad request",
		"ApistatusCode": 400,
	})
	if status != outcome.FailedFinal {
		t.Fatalf("outcome = %q", status)
	}
}

func TestResolveVapioURLDefaults(t *testing.T) {
	if got := vapio.ResolveVapioURL(""); got != "https://vapio.in/api.php" {
		t.Fatalf("url = %q", got)
	}
	if got := vapio.ResolveVapioURL(" https://vapio.in/api.php "); got != "https://vapio.in/api.php" {
		t.Fatalf("url = %q", got)
	}
}
