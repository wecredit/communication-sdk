package redis_test

import (
	"testing"

	"github.com/wecredit/communication-sdk/internal/models/redisModels"
	commredis "github.com/wecredit/communication-sdk/internal/redis"
)

func TestAuditSideWritten(t *testing.T) {
	var blank redisModels.MobileChannelRedisData
	if commredis.AuditSideWritten(blank, commredis.AuditSideInput) || commredis.AuditSideWritten(blank, commredis.AuditSideOutput) {
		t.Fatal("blank claim must not report audit sides written")
	}

	partial := redisModels.MobileChannelRedisData{InputAuditWritten: true}
	if !commredis.AuditSideWritten(partial, "INPUT") {
		t.Fatal("input flag should match case-insensitively")
	}
	if commredis.AuditSideWritten(partial, commredis.AuditSideOutput) {
		t.Fatal("input-only claim must not report output written")
	}

	both := redisModels.MobileChannelRedisData{InputAuditWritten: true, OutputAuditWritten: true}
	if !commredis.AuditSideWritten(both, commredis.AuditSideInput) || !commredis.AuditSideWritten(both, commredis.AuditSideOutput) {
		t.Fatal("both flags should report written")
	}
	if commredis.AuditSideWritten(both, "other") {
		t.Fatal("unknown side must be false")
	}
}

func TestIsAuditSideWrittenNilClient(t *testing.T) {
	written, err := commredis.IsAuditSideWritten(nil, "hash", "field", commredis.AuditSideInput)
	if err != nil || written {
		t.Fatalf("nil client = (%v, %v), want (false, nil)", written, err)
	}
}

func TestMarkAuditSideWrittenNilClient(t *testing.T) {
	if err := commredis.MarkAuditSideWritten(nil, "hash", "field", commredis.AuditSideOutput); err != nil {
		t.Fatalf("nil client mark error = %v", err)
	}
}
