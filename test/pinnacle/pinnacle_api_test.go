package pinnacleApi_test

import (
	"strings"
	"testing"

	"github.com/wecredit/communication-sdk/internal/channels/sms/outcome"
	pinnacleApi "github.com/wecredit/communication-sdk/internal/channels/sms/pinnacle"
	pinnaclepayloads "github.com/wecredit/communication-sdk/internal/channels/sms/pinnacle/pinnaclePayloads"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
)

func TestExtractTransactionIdFromUniqueID(t *testing.T) {
	got := pinnacleApi.ExtractTransactionId(map[string]interface{}{
		"code":   float64(200),
		"status": "success",
		"data": []interface{}{
			map[string]interface{}{
				"mobile":   "917014850582",
				"uniqueid": "60670829786_17877210184629",
			},
		},
	})
	if got != "60670829786_17877210184629" {
		t.Fatalf("transaction id = %q", got)
	}
}

func TestClassifyPinnacleResponseSuccessCode(t *testing.T) {
	status, msg := pinnacleApi.ClassifyPinnacleResponse(map[string]interface{}{
		"code":          float64(200),
		"status":        "success",
		"ApistatusCode": 200,
		"data":          []interface{}{},
	})
	if status != outcome.Submitted {
		t.Fatalf("outcome = %q, want %s", status, outcome.Submitted)
	}
	if !strings.Contains(msg, "bodyCode=200") {
		t.Fatalf("message = %q, want bodyCode=200", msg)
	}
}

func TestClassifyPinnacleResponsePrefersBodyFailureOverHTTPSuccess(t *testing.T) {
	status, _ := pinnacleApi.ClassifyPinnacleResponse(map[string]interface{}{
		"code":          float64(400),
		"status":        "failed",
		"message":       "invalid template",
		"ApistatusCode": 200,
	})
	if status != outcome.FailedFinal {
		t.Fatalf("outcome = %q, want %s", status, outcome.FailedFinal)
	}
}

func TestClassifyPinnacleResponseECStringCode(t *testing.T) {
	status, _ := pinnacleApi.ClassifyPinnacleResponse(map[string]interface{}{
		"code":          "EC1009",
		"status":        "Sorry unable to process request",
		"ApistatusCode": 200,
	})
	if status != outcome.FailedFinal {
		t.Fatalf("outcome = %q, want %s", status, outcome.FailedFinal)
	}
}

func TestResolvePinnacleJSONURL(t *testing.T) {
	cases := map[string]string{
		"https://api.pinnacle.in/index.php/sms/send":  "https://api.pinnacle.in/index.php/sms/json",
		"https://api.pinnacle.in/index.php/sms/json":  "https://api.pinnacle.in/index.php/sms/json",
		"https://api.pinnacle.in/index.php/sms/send/": "https://api.pinnacle.in/index.php/sms/json",
	}
	for in, want := range cases {
		if got := pinnacleApi.ResolvePinnacleJSONURL(in); got != want {
			t.Fatalf("ResolvePinnacleJSONURL(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNormalizePinnacleMSISDN(t *testing.T) {
	got, err := pinnaclepayloads.NormalizeMSISDN("7014850582")
	if err != nil {
		t.Fatal(err)
	}
	if got != "917014850582" {
		t.Fatalf("got %q", got)
	}
	got, err = pinnaclepayloads.NormalizeMSISDN("917014850582")
	if err != nil || got != "917014850582" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestBuildPinnacleJSONPayloadConsoleShape(t *testing.T) {
	payload, err := pinnaclepayloads.BuildConsoleJSONPayload(extapimodels.SmsRequestBody{
		Mobile:        "7014850582",
		CommId:        "WC-WECREDIT-abc-123",
		DltTemplateId: 1777178764367201169,
	}, "WECRLP", "Abhi explore karein https://branch.co/BRNCHI/BBqHUGs WeCredit", "1701170417883448407")
	if err != nil {
		t.Fatal(err)
	}
	if payload["sender"] != "WECRLP" {
		t.Fatalf("sender=%v", payload["sender"])
	}
	if payload["messagetype"] != "TXT" {
		t.Fatalf("messagetype=%v want TXT", payload["messagetype"])
	}
	if payload["dlttempid"] != "1777178764367201169" {
		t.Fatalf("dlttempid=%v", payload["dlttempid"])
	}
	if payload["dltentityid"] != "1701170417883448407" {
		t.Fatalf("dltentityid=%v", payload["dltentityid"])
	}
	msgs, ok := payload["message"].([]map[string]interface{})
	if !ok || len(msgs) != 1 {
		t.Fatalf("message=%T %#v", payload["message"], payload["message"])
	}
	if msgs[0]["number"] != "917014850582" {
		t.Fatalf("number=%v", msgs[0]["number"])
	}
	if !strings.Contains(msgs[0]["text"].(string), "https://branch.co/") {
		t.Fatalf("text=%v", msgs[0]["text"])
	}
	uid, _ := msgs[0]["clientuid"].(string)
	if uid == "" || strings.ContainsAny(uid, "-_") {
		t.Fatalf("clientuid should be alphanumeric, got %q", uid)
	}
}

func TestBuildPinnacleJSONPayloadTXTWithoutURL(t *testing.T) {
	payload, err := pinnaclepayloads.BuildConsoleJSONPayload(extapimodels.SmsRequestBody{
		Mobile: "9220146969",
	}, "WECRLP", "Abhi explore karein  wecredit.co.in WeCredit", "1701170417883448407")
	if err != nil {
		t.Fatal(err)
	}
	if payload["messagetype"] != "TXT" {
		t.Fatalf("messagetype=%v want TXT", payload["messagetype"])
	}
}
