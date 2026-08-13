package consumerServices_test

import (
	"testing"

	"github.com/wecredit/communication-sdk/config"
	services "github.com/wecredit/communication-sdk/internal/services/consumerServices"
)

func TestClientWorkerCountUsesDefaultAndOverrides(t *testing.T) {
	original := config.Configs
	t.Cleanup(func() { config.Configs = original })
	config.Configs.ConsumerDefaultClientWorkers = "5"
	config.Configs.ConsumerClientWorkerOverrides = "wecredit:25,zapcash:5,creditsea:7"

	for _, test := range []struct {
		client string
		want   int
	}{
		{client: "wecredit", want: 25},
		{client: "ZapCash", want: 5},
		{client: "creditsea", want: 7},
		{client: "trustfin", want: 5},
	} {
		if got := services.ClientWorkerCount(test.client); got != test.want {
			t.Fatalf("ClientWorkerCount(%q) = %d, want %d", test.client, got, test.want)
		}
	}
}
