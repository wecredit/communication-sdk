package templateCategorySync_test

import (
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/internal/services/templateCategorySync"
)

func TestTimesTemplateListHTTPTimeout(t *testing.T) {
	if got := templateCategorySync.TimesTemplateListHTTPTimeout(""); got != 60*time.Second {
		t.Fatalf("blank = %s, want 60s", got)
	}
	if got := templateCategorySync.TimesTemplateListHTTPTimeout("0"); got != 60*time.Second {
		t.Fatalf("zero = %s, want 60s", got)
	}
	if got := templateCategorySync.TimesTemplateListHTTPTimeout("abc"); got != 60*time.Second {
		t.Fatalf("invalid = %s, want 60s", got)
	}
	if got := templateCategorySync.TimesTemplateListHTTPTimeout("90"); got != 90*time.Second {
		t.Fatalf("override = %s, want 90s", got)
	}
}
