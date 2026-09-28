package configurationcache_test

import (
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/internal/configurationcache"
	"github.com/wecredit/communication-sdk/sdk/models"
)

func TestOptionsFromConfig(t *testing.T) {
	options, err := configurationcache.OptionsFromConfig(models.Config{
		Environment: "test", ConfigurationVersionTable: "ConfigurationVersion",
		TemplateDetailsTable:            "TemplateDetails",
		CacheVersionPollIntervalSeconds: "120", CacheReloadMinIntervalSeconds: "60",
	})
	if err != nil {
		t.Fatalf("OptionsFromConfig() error = %v", err)
	}
	if options.PollInterval != 120*time.Second || options.MinimumReloadInterval != 60*time.Second {
		t.Fatalf("unexpected intervals: %+v", options)
	}

	_, err = configurationcache.OptionsFromConfig(models.Config{
		Environment: "test", ConfigurationVersionTable: "ConfigurationVersion",
		TemplateDetailsTable:            "TemplateDetails",
		CacheVersionPollIntervalSeconds: "59", CacheReloadMinIntervalSeconds: "60",
	})
	if err == nil {
		t.Fatal("OptionsFromConfig() accepted a poll interval below the allowed range")
	}
}
