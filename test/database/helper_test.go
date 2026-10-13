package database_test

import (
	"strings"
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/internal/database"
	"gorm.io/gorm"
)

func TestPoolLimitUsesFallbackForBlankAndInvalid(t *testing.T) {
	if got := database.ParsePoolLimit("", "DB_MAX_OPEN_CONNS_MARKETING", "Marketing DB", 50); got != 50 {
		t.Fatalf("blank = %d, want 50", got)
	}
	if got := database.ParsePoolLimit("0", "DB_MAX_OPEN_CONNS_ZAPCASH_V1", "ZapCash v1 audit DB", 30); got != 30 {
		t.Fatalf("zero = %d, want 30", got)
	}
	if got := database.ParsePoolLimit("40", "DB_MAX_OPEN_CONNS_MARKETING", "Marketing DB", 50); got != 40 {
		t.Fatalf("parsed = %d, want 40", got)
	}
}

func TestEnqueueAuditInsertReturnsWhileWorkRuns(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	if !database.EnqueueAuditInsert(func() {
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
	done := make(chan struct{}, database.AuditInsertSlotCap())
	for i := 0; i < database.AuditInsertSlotCap(); i++ {
		if !database.EnqueueAuditInsert(func() {
			<-release
			done <- struct{}{}
		}) {
			t.Fatalf("slot %d was not free", i)
		}
	}
	if database.EnqueueAuditInsert(func() {}) {
		t.Fatal("enqueue succeeded while every slot was in use")
	}
	close(release)
	for i := 0; i < database.AuditInsertSlotCap(); i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("audit insert did not release its slot")
		}
	}
}

func TestInsertRowSyncSkipsWhenNoPool(t *testing.T) {
	if err := database.InsertRowSync("wecredit", "PushInputAuditTable", map[string]interface{}{"CommId": "c1"}); err != nil {
		t.Fatal(err)
	}
}

func TestInsertRowSyncReturnsBusyBeforeInsert(t *testing.T) {
	prevDB := database.DBtechWrite
	weCredit, zapCash, v1 := database.ChannelAuditFlags()
	t.Cleanup(func() {
		database.DBtechWrite = prevDB
		database.SetChannelAuditFlags(weCredit, zapCash, v1)
	})
	database.DBtechWrite = &gorm.DB{}
	database.SetChannelAuditFlags("true", zapCash, v1)

	release := make(chan struct{})
	done := make(chan struct{}, database.AuditInsertSlotCap())
	for i := 0; i < database.AuditInsertSlotCap(); i++ {
		if !database.EnqueueAuditInsert(func() {
			<-release
			done <- struct{}{}
		}) {
			t.Fatalf("slot %d was not free", i)
		}
	}
	err := database.InsertRowSync("wecredit", "PushInputAuditTable", map[string]interface{}{"CommId": "c1"})
	close(release)
	for i := 0; i < database.AuditInsertSlotCap(); i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("audit insert did not release its slot")
		}
	}
	if err == nil || !strings.Contains(err.Error(), "audit insert busy") {
		t.Fatalf("InsertRowSync error = %v, want audit insert busy", err)
	}
}
