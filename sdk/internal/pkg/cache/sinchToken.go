package cache

import (
	"sync"
	"time"

	"dev.azure.com/wctec/communication-engine/sdk/helper"
	extapimodels "dev.azure.com/wctec/communication-engine/sdk/internal/models/extApiModels"
)

type TokenCache struct {
	sync.Map
}

var cache = &TokenCache{}

func SetToken(token *extapimodels.SinchTokenResponse) {
	cache.Store("access_token", token.AccessToken)
	cache.Store("refresh_token", token.RefreshToken)

	go func() {
		time.Sleep(time.Duration(token.ExpiresIn-5) * time.Second)
		_, ok := cache.Load("refresh_token")
		if ok {
			if newToken, err := helper.RefreshAccessToken(token.RefreshToken); err == nil {
				SetToken(newToken)
			}
		}
	}()
}

func GetAccessToken() (string, bool) {
	val, ok := cache.Load("access_token")
	if !ok {
		return "", false
	}
	return val.(string), true
}
