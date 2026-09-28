package monitoring

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	redisclient "github.com/redis/go-redis/v9"
)

var istLocation = time.FixedZone("IST", 5*60*60+30*60)

func VariantKey(now time.Time, result AcceptedResult, recipient string) string {
	date := now.In(istLocation).Format("2006-01-02")
	parts := []string{
		"zapcash:monitor", date,
		strings.ToUpper(strings.TrimSpace(result.Payload.Channel)),
		fmt.Sprintf("%.2f", result.Payload.Stage),
		strings.ToUpper(strings.TrimSpace(result.ResolvedVendor)),
		url.QueryEscape(strings.TrimSpace(result.ResolvedTemplate)),
		recipient,
	}
	return strings.Join(parts, ":")
}

func ReservationTTL(now time.Time) time.Duration {
	local := now.In(istLocation)
	nextDay := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 5, 0, 0, istLocation)
	ttl := nextDay.Sub(local)
	if ttl <= 0 {
		return 24 * time.Hour
	}
	return ttl
}

func reserve(ctx context.Context, client *redisclient.Client, key string, now time.Time) (bool, error) {
	if client == nil {
		return false, fmt.Errorf("redis client is not initialized")
	}
	return client.SetNX(ctx, key, "reserved", ReservationTTL(now)).Result()
}

func release(ctx context.Context, client *redisclient.Client, key string) error {
	if client == nil {
		return fmt.Errorf("redis client is not initialized")
	}
	return client.Del(ctx, key).Err()
}
