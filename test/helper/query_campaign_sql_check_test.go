package helper_test

import (
	"testing"
	"time"

	sdkHelper "github.com/wecredit/communication-sdk/sdk/helper"
)

func TestCheckSingleSelect(t *testing.T) {
	if err := sdkHelper.CheckSingleSelect("SELECT 1 AS client"); err != nil {
		t.Fatalf("expected ok: %v", err)
	}
	if err := sdkHelper.CheckSingleSelect("WITH x AS (SELECT 1 AS a) SELECT * FROM x"); err != nil {
		t.Fatalf("expected WITH ok: %v", err)
	}
	if err := sdkHelper.CheckSingleSelect("DELETE FROM t"); err == nil {
		t.Fatal("expected reject DELETE")
	}
	if err := sdkHelper.CheckSingleSelect("SELECT 1; SELECT 2"); err == nil {
		t.Fatal("expected reject multi-statement")
	}
}

func TestMapRowAliasesSMS(t *testing.T) {
	row, err := sdkHelper.MapRowAliases(map[string]string{
		"client": "wecredit", "channel": "SMS", "vendor": "PINNACLE",
		"processname": "OFFER", "templatereference": "123", "mobile": "9999999999",
	}, "SMS", "wecredit")
	if err != nil {
		t.Fatal(err)
	}
	if row.ProcessName != "OFFER" {
		t.Fatalf("process=%s", row.ProcessName)
	}
}

func TestScheduleOnceWindow(t *testing.T) {
	raw := []byte(`{"timezone":"Asia/Kolkata","time":"10:30","frequency":"once","onceDate":"2026-11-25"}`)
	s, err := sdkHelper.ParseSchedule(raw, 120)
	if err != nil {
		t.Fatal(err)
	}
	if s.Frequency != "once" {
		t.Fatalf("freq=%s", s.Frequency)
	}
	_, err = sdkHelper.ParseSchedule([]byte(`{"timezone":"Asia/Kolkata","time":"19:50","frequency":"daily"}`), 600)
	if err == nil {
		t.Fatal("expected reject time+timeout past 20:00")
	}
}

func TestExpiresAtIs2000ISTOnScheduledDate(t *testing.T) {
	ist := sdkHelper.IST()
	sched := time.Date(2026, 11, 25, 10, 30, 0, 0, ist)
	exp := sdkHelper.ExpiresAtForScheduled(sched)
	want := time.Date(2026, 11, 25, 20, 0, 0, 0, ist)
	if !exp.Equal(want) {
		t.Fatalf("expiresAt=%v want %v", exp, want)
	}
}
