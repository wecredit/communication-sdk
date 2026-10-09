package database_test

import (
	"errors"
	"testing"

	"github.com/wecredit/communication-sdk/internal/database"
	"github.com/wecredit/communication-sdk/sdk/variables"
)

func TestChannelAuditDestinations(t *testing.T) {
	tests := []struct {
		name     string
		client   string
		weCredit string
		zapCash  string
		v1       string
		wantComm bool
		wantCore bool
	}{
		{
			name:     "wecredit off writes nowhere",
			client:   variables.WeCredit,
			weCredit: "false",
			zapCash:  "false",
			v1:       "true",
		},
		{
			name:     "zapcash communication off and v1 on writes only core",
			client:   variables.ZapCash,
			weCredit: "true",
			zapCash:  "false",
			v1:       "true",
			wantCore: true,
		},
		{
			name:     "unknown client still writes communication",
			client:   variables.CreditSea,
			weCredit: "false",
			zapCash:  "false",
			v1:       "true",
			wantComm: true,
		},
		{
			name:     "both zapcash flags on writes both",
			client:   " ZapCash ",
			weCredit: "false",
			zapCash:  "true",
			v1:       "true",
			wantComm: true,
			wantCore: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotComm, gotCore := database.ChannelAuditDestinations(tc.client, tc.weCredit, tc.zapCash, tc.v1)
			if gotComm != tc.wantComm || gotCore != tc.wantCore {
				t.Fatalf("destinations = comm %t core %t, want comm %t core %t", gotComm, gotCore, tc.wantComm, tc.wantCore)
			}
		})
	}
}

func TestFlagParsing(t *testing.T) {
	commCases := []struct {
		name string
		flag string
		want bool
	}{
		{name: "missing", flag: "", want: true},
		{name: "false", flag: "false", want: false},
		{name: "FALSE space", flag: "FALSE ", want: false},
		{name: "flase", flag: "flase", want: true},
		{name: "true", flag: "true", want: true},
	}
	for _, tc := range commCases {
		t.Run("comm/"+tc.name, func(t *testing.T) {
			if got := database.CommWriteEnabled(variables.WeCredit, tc.flag, "false"); got != tc.want {
				t.Fatalf("CommWriteEnabled(%q) = %t, want %t", tc.flag, got, tc.want)
			}
		})
	}

	v1Cases := []struct {
		name string
		flag string
		want bool
	}{
		{name: "missing", flag: "", want: false},
		{name: "false", flag: "false", want: false},
		{name: "FALSE space", flag: "FALSE ", want: false},
		{name: "flase", flag: "flase", want: false},
		{name: "true", flag: "true", want: true},
		{name: "TRUE space", flag: " TRUE", want: true},
	}
	for _, tc := range v1Cases {
		t.Run("v1/"+tc.name, func(t *testing.T) {
			if got := database.ZapCashV1WriteEnabled(tc.flag); got != tc.want {
				t.Fatalf("ZapCashV1WriteEnabled(%q) = %t, want %t", tc.flag, got, tc.want)
			}
		})
	}
}

func TestRunChannelAuditInsertSwallowsError(t *testing.T) {
	err := errors.New("insert failed")
	database.RunChannelAuditInsert(variables.ZapCash, "SmsOutputTable", func() error {
		return err
	})
}
