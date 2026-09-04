package apiServices_test

import (
	"errors"
	"testing"

	apiServices "github.com/wecredit/communication-sdk/internal/services/apiServices"
)

func TestResolveTemplateListOrderDefaults(t *testing.T) {
	got, err := apiServices.ResolveTemplateListOrder("", "", true)
	if err != nil {
		t.Fatal(err)
	}
	want := apiServices.TemplateListOrderClause(true)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveTemplateListOrderAllowlist(t *testing.T) {
	got, err := apiServices.ResolveTemplateListOrder("process", "asc", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Process ASC, Id DESC" {
		t.Fatalf("got %q", got)
	}

	got, err = apiServices.ResolveTemplateListOrder("updatedOn", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "COALESCE(UpdatedOn, CreatedOn) DESC, Id DESC" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveTemplateListOrderRejectsUnknown(t *testing.T) {
	_, err := apiServices.ResolveTemplateListOrder("drop table", "asc", false)
	if !errors.Is(err, apiServices.ErrInvalidSort) {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveTemplateListOrderRejectsBadDir(t *testing.T) {
	_, err := apiServices.ResolveTemplateListOrder("process", "sideways", false)
	if !errors.Is(err, apiServices.ErrInvalidSort) {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveStageMappingListOrder(t *testing.T) {
	got, err := apiServices.ResolveStageMappingListOrder("", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Stage ASC, SubStage ASC, Id ASC" {
		t.Fatalf("got %q", got)
	}

	got, err = apiServices.ResolveStageMappingListOrder("subStage", "desc")
	if err != nil {
		t.Fatal(err)
	}
	if got != "SubStage DESC, Id DESC" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveLenderScheduleListOrder(t *testing.T) {
	got, err := apiServices.ResolveLenderScheduleListOrder("", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Stage ASC, LenderName ASC, CommType ASC, Id ASC" {
		t.Fatalf("got %q", got)
	}
}
