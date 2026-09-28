package cache_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
	"github.com/wecredit/communication-sdk/pkg/cache"
)

func TestSinchWhatsappTokenCacheKeyIsolation(t *testing.T) {
	cache.ResetSinchWhatsappTokenCacheForTest()
	defer cache.ResetSinchWhatsappTokenCacheForTest()

	cache.SetSinchWhatsappTokenFetchHookForTest(func(client string) (*extapimodels.SinchTokenResponse, error) {
		return &extapimodels.SinchTokenResponse{
			AccessToken: "tok-" + client,
			ExpiresIn:   5, // skip scheduled refresh
		}, nil
	})

	wc, err := cache.GetSinchWhatsappAccessToken("wecredit")
	if err != nil || wc != "tok-wecredit" {
		t.Fatalf("wecredit token=%q err=%v", wc, err)
	}
	cs, err := cache.GetSinchWhatsappAccessToken("creditsea")
	if err != nil || cs != "tok-creditsea" {
		t.Fatalf("creditsea token=%q err=%v", cs, err)
	}
	if wc == cs {
		t.Fatal("clients must not share tokens")
	}
}

func TestSinchWhatsappInvalidateClearsEntry(t *testing.T) {
	cache.ResetSinchWhatsappTokenCacheForTest()
	defer cache.ResetSinchWhatsappTokenCacheForTest()

	var fetches atomic.Int32
	cache.SetSinchWhatsappTokenFetchHookForTest(func(client string) (*extapimodels.SinchTokenResponse, error) {
		n := fetches.Add(1)
		return &extapimodels.SinchTokenResponse{
			AccessToken: "v" + string(rune('0'+n)),
			ExpiresIn:   5,
		}, nil
	})

	first, err := cache.GetSinchWhatsappAccessToken("wecredit")
	if err != nil || first != "v1" {
		t.Fatalf("first=%q err=%v", first, err)
	}

	cache.InvalidateSinchWhatsappToken("wecredit")
	second, err := cache.GetSinchWhatsappAccessToken("wecredit")
	if err != nil || second != "v2" {
		t.Fatalf("after invalidate second=%q err=%v fetches=%d", second, err, fetches.Load())
	}
	if fetches.Load() != 2 {
		t.Fatalf("fetches=%d, want 2", fetches.Load())
	}
}

func TestSinchWhatsappSingleflightDoesNotStampedeOnConcurrentMiss(t *testing.T) {
	cache.ResetSinchWhatsappTokenCacheForTest()
	defer cache.ResetSinchWhatsappTokenCacheForTest()

	var fetches atomic.Int32
	cache.SetSinchWhatsappTokenFetchHookForTest(func(client string) (*extapimodels.SinchTokenResponse, error) {
		fetches.Add(1)
		time.Sleep(20 * time.Millisecond)
		return &extapimodels.SinchTokenResponse{AccessToken: "shared", ExpiresIn: 5}, nil
	})

	var wg sync.WaitGroup
	tokens := make([]string, 8)
	errs := make([]error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tokens[i], errs[i] = cache.GetSinchWhatsappAccessToken("wecredit")
		}(i)
	}
	wg.Wait()

	if fetches.Load() != 1 {
		t.Fatalf("fetches=%d, want 1 (singleflight)", fetches.Load())
	}
	for i := 0; i < 8; i++ {
		if errs[i] != nil || tokens[i] != "shared" {
			t.Fatalf("goroutine %d token=%q err=%v", i, tokens[i], errs[i])
		}
	}
}
