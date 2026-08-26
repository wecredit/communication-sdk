package pinnacleApi_test

import (
	"strings"
	"testing"

	"github.com/wecredit/communication-sdk/internal/channels/sms/outcome"
	pinnacleApi "github.com/wecredit/communication-sdk/internal/channels/sms/pinnacle"
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

func TestBuildPinnacleURLEscapesAndSetsQuery(t *testing.T) {
	got, err := pinnacleApi.BuildPinnacleURL(
		"https://api.pinnacle.in/index.php/sms/send",
		"secret",
		"WECRWP",
		"7014850582",
		"hello world",
		1777178764360614607,
		"1701170417883448407",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"/WECRWP/",
		"/7014850582/",
		"hello%20world",
		"/TXT",
		"apikey=secret",
		"dlttempid=1777178764360614607",
		"dltentityid=1701170417883448407",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("url %q missing %q", got, want)
		}
	}
}
