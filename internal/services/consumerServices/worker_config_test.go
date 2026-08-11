package services

import (
	"testing"

	"github.com/wecredit/communication-sdk/config"
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
		if got := clientWorkerCount(test.client); got != test.want {
			t.Fatalf("clientWorkerCount(%q) = %d, want %d", test.client, got, test.want)
		}
	}
}
