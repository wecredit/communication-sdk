package templateCategorySync_test

import (
	"strings"
	"testing"

	"github.com/wecredit/communication-sdk/internal/services/templateCategorySync"
)

func TestSanitizePagingNextSameHost(t *testing.T) {
	t.Parallel()
	base := "https://partnersv1.pinbot.ai/v3"
	next := "https://partnersv1.pinbot.ai/v3/1000/message_templates?after=abc"
	got, err := templateCategorySync.SanitizePagingNext(base, next)
	if err != nil {
		t.Fatal(err)
	}
	if got != next {
		t.Fatalf("got %q want %q", got, next)
	}
}

func TestSanitizePagingNextEmpty(t *testing.T) {
	t.Parallel()
	got, err := templateCategorySync.SanitizePagingNext("https://partnersv1.pinbot.ai/v3", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q want empty", got)
	}
}

func TestSanitizePagingNextRejectsOffHost(t *testing.T) {
	t.Parallel()
	_, err := templateCategorySync.SanitizePagingNext(
		"https://partnersv1.pinbot.ai/v3",
		"https://evil.example/steal",
	)
	if err == nil {
		t.Fatal("expected host mismatch error")
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestSanitizePagingNextRejectsSchemeChange(t *testing.T) {
	t.Parallel()
	_, err := templateCategorySync.SanitizePagingNext(
		"https://partnersv1.pinbot.ai/v3",
		"http://partnersv1.pinbot.ai/v3/next",
	)
	if err == nil {
		t.Fatal("expected scheme mismatch error")
	}
}
