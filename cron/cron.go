package cron

import (
	"context"
	"fmt"

	"github.com/robfig/cron/v3"
	"github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

func StartMidnightResetCron() {
	utils.Debug("Starting midnight reset cron job...")
	c := cron.New(cron.WithSeconds()) // With seconds for full control
	_, err := c.AddFunc("0 0 0 * * *", func() {
		err := redis.ResetCreditSeaCounter(context.Background(), redis.RDB, redis.CreditSeaWhatsappCount)
		if err != nil {
			utils.Error(fmt.Errorf("cron reset failed: %v", err))
		} else {
			utils.Info("Cron: Counter reset at 12:00 AM")
		}
	})
	if err != nil {
		utils.Error(fmt.Errorf("failed to schedule midnight reset: %v", err))
	}
	c.Start()
}
