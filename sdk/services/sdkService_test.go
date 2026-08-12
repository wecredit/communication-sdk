package sdkServices

import (
	"strings"
	"testing"
)

func TestResolveCommIDPreservesCallerValue(t *testing.T) {
	got := ResolveCommID("  marketing-42  ", "wecredit")
	if got != "marketing-42" {
		t.Fatalf("ResolveCommID() = %q, want marketing-42", got)
	}
}

func TestResolveCommIDGeneratesWhenEmpty(t *testing.T) {
	got := ResolveCommID("", "wecredit")
	if got == "" {
		t.Fatal("expected generated CommId")
	}
	if !strings.HasPrefix(got, "WC-WECREDIT-") {
		t.Fatalf("ResolveCommID() = %q, want WC-WECREDIT- prefix", got)
	}
}
