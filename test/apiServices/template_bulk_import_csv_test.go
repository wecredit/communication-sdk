package apiServices_test

import (
	"strings"
	"testing"

	services "github.com/wecredit/communication-sdk/internal/services/apiServices"
)

func TestParseBulkTemplateCSVStageDecimalParity(t *testing.T) {
	for _, stage := range []string{"2", "2.1", "2.10", "2.01"} {
		csv := "Client,Channel,Process,Vendor,Is Active,Stage\nwecredit,SMS,collections,TIMES,true," + stage + "\n"
		rows, err := services.ParseBulkTemplateCSV(strings.NewReader(csv), 2000, 5*1024*1024)
		if err != nil {
			t.Fatalf("stage %q: %v", stage, err)
		}
		if len(rows) != 1 || rows[0].Template.Stage == nil {
			t.Fatalf("stage %q was not parsed", stage)
		}
	}
}

func TestParseBulkTemplateCSVRejectsContentAndTemplateTextTogether(t *testing.T) {
	_, err := services.ParseBulkTemplateCSV(strings.NewReader("Client,Channel,Process,Vendor,Is Active,Content,Template Text\na,SMS,p,v,false,x,y\n"), 2000, 5*1024*1024)
	if err == nil || !strings.Contains(err.Error(), "unknown header") {
		t.Fatalf("expected fixed-header error, got %v", err)
	}
}

func TestParseBulkTemplateCSVTracksMultilinePhysicalRows(t *testing.T) {
	input := "Client,Channel,Process,Vendor,Is Active,Template Text\n" +
		"a,SMS,p,v,false,\"first\nsecond\"\n" +
		"a,SMS,p,v,false,third\n"
	rows, err := services.ParseBulkTemplateCSV(strings.NewReader(input), 2000, 5*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[1].Row != 4 {
		t.Fatalf("rows = %#v, want second row at physical row 4", rows)
	}
}

func TestParseBulkTemplateCSVDefaultsIsActiveToTrue(t *testing.T) {
	rows, err := services.ParseBulkTemplateCSV(strings.NewReader("Client,Channel,Process,Vendor,Template Name\na,WHATSAPP,p,v,name\n"), 2000, 5*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].Template.IsActive {
		t.Fatal("expected Is Active to default to true")
	}
}
