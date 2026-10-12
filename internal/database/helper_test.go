package database

import (
	"testing"
	"time"
)

func TestPoolLimitUsesFallbackForBlankAndInvalid(t *testing.T) {
	if got := poolLimit("", "DB_MAX_OPEN_CONNS_MARKETING", "Marketing DB", 50); got != 50 {
		t.Fatalf("blank = %d, want 50", got)
	}
	if got := poolLimit("0", "DB_MAX_OPEN_CONNS_ZAPCASH_V1", "ZapCash v1 audit DB", 30); got != 30 {
		t.Fatalf("zero = %d, want 30", got)
	}
	if got := poolLimit("40", "DB_MAX_OPEN_CONNS_MARKETING", "Marketing DB", 50); got != 40 {
		t.Fatalf("parsed = %d, want 40", got)
	}
}

func TestEnqueueAuditInsertReturnsWhileWorkRuns(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	if !enqueueAuditInsert(func() {
		close(started)
		<-release
		close(done)
	}) {
		t.Fatal("enqueue failed with a free slot")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("audit insert did not start")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("audit insert did not finish")
	}
}

func TestEnqueueAuditInsertSkipsWhenBusy(t *testing.T) {
	release := make(chan struct{})
	done := make(chan struct{}, cap(auditInsertSlots))
	for i := 0; i < cap(auditInsertSlots); i++ {
		if !enqueueAuditInsert(func() {
			<-release
			done <- struct{}{}
		}) {
			t.Fatalf("slot %d was not free", i)
		}
	}
	if enqueueAuditInsert(func() {}) {
		t.Fatal("enqueue succeeded while every slot was in use")
	}
	close(release)
	for i := 0; i < cap(auditInsertSlots); i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("audit insert did not release its slot")
		}
	}
}
