package cron

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/wecredit/communication-sdk/internal/services/templateCategorySync"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

// templateCategorySyncRunning mirrors hermis cron max_instances=1: skip a tick
// if the previous sync is still in progress. Hermis also does not take the
// template mutation lock — category sync and admin updates are last-writer-wins
// on ProviderTemplateCategory / IsActive / Error / CategoryUpdatedOn.
var templateCategorySyncRunning atomic.Bool

// StartWhatsappTemplateCategorySyncCron ports hermis Times/Pinnacle category checkers
// onto TemplateDetails (every 30m during 09:00–20:59 IST).
func StartWhatsappTemplateCategorySyncCron() {
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		utils.Error(fmt.Errorf("template category sync: load IST: %v", err))
		return
	}

	c := cron.New(cron.WithLocation(ist))
	_, err = c.AddFunc("*/30 9-20 * * *", func() {
		if !templateCategorySyncRunning.CompareAndSwap(false, true) {
			utils.Info("WhatsApp template category sync skipped: previous run still in progress")
			return
		}

		defer templateCategorySyncRunning.Store(false)

		utils.Info("WhatsApp template category sync starting")
		if err := templateCategorySync.SyncTimesAndPinnacle(); err != nil {
			utils.Error(fmt.Errorf("WhatsApp template category sync failed: %v", err))
			return
		}
		utils.Info("WhatsApp template category sync finished")
	})

	if err != nil {
		utils.Error(fmt.Errorf("schedule WhatsApp template category sync: %v", err))
		return
	}

	c.Start()
	utils.Info("WhatsApp template category sync cron scheduled (*/30 9-20 IST)")
}
