package apiServices_test

import (
	"testing"

	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	services "github.com/wecredit/communication-sdk/internal/services/apiServices"
)

func TestBulkActiveIdentityMatchesCreateUniquenessBranches(t *testing.T) {
	base := apiModels.Templatedetails{
		Client:       "wecredit",
		Channel:      "RCS",
		Vendor:       "PINNACLE",
		Process:      "MARKETING",
		TemplateName: "same_name",
		IsActive:     true,
	}

	left := base
	left.AppId = "app-a"
	right := base
	right.AppId = "app-b"
	if services.BulkActiveIdentity(left) != services.BulkActiveIdentity(right) {
		t.Fatal("RCS active identity must ignore AppId so same TemplateName conflicts")
	}

	emailLeft := base
	emailLeft.Channel = "EMAIL"
	emailLeft.AppId = "app-a"
	emailRight := emailLeft
	emailRight.AppId = "app-b"
	if services.BulkActiveIdentity(emailLeft) != services.BulkActiveIdentity(emailRight) {
		t.Fatal("EMAIL active identity must ignore AppId")
	}

	pushLeft := base
	pushLeft.Channel = "PUSH"
	pushLeft.AppId = "app-a"
	pushRight := pushLeft
	pushRight.AppId = "app-b"
	if services.BulkActiveIdentity(pushLeft) != services.BulkActiveIdentity(pushRight) {
		t.Fatal("PUSH active identity must ignore AppId")
	}

	waLeft := base
	waLeft.Channel = "WHATSAPP"
	waLeft.AppId = "app-a"
	waRight := waLeft
	waRight.AppId = "app-b"
	if services.BulkActiveIdentity(waLeft) == services.BulkActiveIdentity(waRight) {
		t.Fatal("WhatsApp active identity must include AppId")
	}
}
