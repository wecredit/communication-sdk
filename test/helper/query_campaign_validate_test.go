package helper_test

import (
	"testing"

	sdkHelper "github.com/wecredit/communication-sdk/sdk/helper"
)

func TestDbTargetAllowedForClient(t *testing.T) {
	if !sdkHelper.DbTargetAllowedForClient(sdkHelper.DbTargetZapCashMySQLV1, sdkHelper.ClientZapCash) {
		t.Fatal("zapcash target should allow zapcash client")
	}
	if sdkHelper.DbTargetAllowedForClient(sdkHelper.DbTargetZapCashMySQLV1, sdkHelper.ClientWeCredit) {
		t.Fatal("zapcash target must not allow wecredit")
	}
	if sdkHelper.DbTargetAllowedForClient(sdkHelper.DbTargetMarketingSQLServer, "all") {
		t.Fatal("all client must be rejected")
	}
}

func TestMapRowAliasesClientMustMatch(t *testing.T) {
	cols := map[string]string{
		"client": "zapcash", "channel": "SMS", "vendor": "PINNACLE",
		"processname": "OFFER", "templatereference": "t1", "mobile": "9999999999",
	}
	if _, err := sdkHelper.MapRowAliases(cols, "SMS", sdkHelper.ClientZapCash); err != nil {
		t.Fatalf("expected ok: %v", err)
	}
	if _, err := sdkHelper.MapRowAliases(cols, "SMS", sdkHelper.ClientWeCredit); err == nil {
		t.Fatal("expected client mismatch error")
	}
}
