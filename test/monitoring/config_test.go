package monitoring_test

import (
	"testing"

	"github.com/wecredit/communication-sdk/internal/services/monitoring"
)

func TestMasterSwitchAndConfigValidation(t *testing.T) {
	if got := monitoring.ParseConfiguration("false", "9899074649", `{"customerName":"Monitor"}`); got.Enabled {
		t.Fatal("monitoring must remain disabled when master switch is false")
	}

	got := monitoring.ParseConfiguration("true", "9899074649,bad,+919799156111,9899074649", `{"customerName":"Monitor"}`)
	if !got.Enabled || len(got.Recipients) != 2 || got.Recipients[0] != "9899074649" || got.Recipients[1] != "9799156111" {
		t.Fatalf("unexpected parsed configuration: %+v", got)
	}

	if got := monitoring.ParseConfiguration("true", "9899074649", `{broken`); got.Enabled {
		t.Fatal("malformed shared profile must disable monitoring")
	}

	for name, profile := range map[string]string{
		"null":         `null`,
		"empty object": `{}`,
		"blank values": `{"customerName":"  "}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := monitoring.ParseConfiguration("true", "9899074649", profile); got.Enabled {
				t.Fatalf("empty shared profile %s must disable monitoring", profile)
			}
		})
	}
}
