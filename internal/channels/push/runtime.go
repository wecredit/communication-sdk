package push

import (
	"context"
	"errors"
	"sync"

	"github.com/wecredit/communication-sdk/internal/channels/push/fcm"
	"github.com/wecredit/communication-sdk/internal/channels/push/ledger"
	"github.com/wecredit/communication-sdk/sdk/models"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"gorm.io/gorm"
)

var (
	runtimeMu      sync.RWMutex
	runtimeService *Service
)

// Init constructs the per-client FCM sender, bounded retry executor, and
// durable ledger service. It does not contact FCM or mutate database schema.
func Init(cfg models.Config, db *gorm.DB) error {
	clientConfigs, err := fcm.ParseClientConfigs(cfg.FCMClientConfigJSON)
	if err != nil {
		return err
	}

	tokenProvider := fcm.NewTokenProvider(nil)
	sender, err := fcm.NewSender(clientConfigs, tokenProvider, nil)
	if err != nil {
		return err
	}
	executor, err := fcm.NewRetryExecutor(sender)
	if err != nil {
		return err
	}
	store, err := ledger.NewStore(db, cfg.PushDispatchLedgerTable)
	if err != nil {
		return err
	}
	service, err := NewService(store, executor)
	if err != nil {
		return err
	}

	runtimeMu.Lock()
	runtimeService = service
	runtimeMu.Unlock()
	return nil
}

func Send(ctx context.Context, request sdkModels.CommApiRequestBody) (Result, error) {
	runtimeMu.RLock()
	service := runtimeService
	runtimeMu.RUnlock()
	if service == nil {
		return Result{}, errors.New("PUSH service is not initialized")
	}
	return service.Send(ctx, request)
}
