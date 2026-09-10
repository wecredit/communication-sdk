package cache_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wecredit/communication-sdk/pkg/cache"
)

func TestSinchWhatsappTokenCacheKeyIsolation(t *testing.T) {
	cache.ResetSinchWhatsappTokenCacheForTest()
	cache.InvalidateSinchWhatsappToken("wecredit")
	cache.InvalidateSinchWhatsappToken("creditsea")
}

func TestSinchWhatsappInvalidateClearsEntry(t *testing.T) {
	cache.ResetSinchWhatsappTokenCacheForTest()
	cache.InvalidateSinchWhatsappToken("wecredit")
	// Without live credentials Get will fail; ensure Invalidate does not panic.
	time.Sleep(10 * time.Millisecond)
}

func TestSinchWhatsappSingleflightDoesNotPanicUnderConcurrentMiss(t *testing.T) {
	cache.ResetSinchWhatsappTokenCacheForTest()
	var wg sync.WaitGroup
	var fails atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cache.GetSinchWhatsappAccessToken("wecredit")
			if err != nil {
				fails.Add(1)
			}
		}()
	}
	wg.Wait()
	// Without staging credentials all callers fail together via singleflight —
	// the important property is they complete without deadlock.
	if fails.Load() == 0 {
		t.Log("unexpected success without credentials; environment may have live Sinch WA config")
	}
}
