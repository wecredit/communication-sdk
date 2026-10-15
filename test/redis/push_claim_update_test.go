package redis_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/internal/models/redisModels"
	"github.com/wecredit/communication-sdk/internal/redis"
)

func TestPushClaimUpdateIgnoresOtherHashFields(t *testing.T) {
	client, stub := newClaimRedis(t)
	hash := "CommIdempotentKeyValMap"
	if err := client.HSet(context.Background(), hash, "event-a", claimJSON(t, redisModels.MobileChannelRedisData{ClaimedAtUnix: 10, ClaimID: "owner"}), "event-b", "sibling").Err(); err != nil {
		t.Fatal(err)
	}
	stub.onHGet = func(key, field string) {
		if key == hash && field == "event-a" {
			stub.hashes[key]["event-b"] = "sibling-written-during-read"
		}
	}

	if err := redis.RefreshPushClaim(client, hash, "event-a", "owner"); err != nil {
		t.Fatalf("refresh while another field changed: %v", err)
	}
	stored := readClaim(t, client, hash, "event-a")
	if stored.ClaimID != "owner" || stored.ClaimedAtUnix < time.Now().UTC().Unix()-2 {
		t.Fatalf("refreshed claim = %+v", stored)
	}
	if got := stub.field(hash, "event-b"); got != "sibling-written-during-read" {
		t.Fatalf("sibling field = %q", got)
	}
}

func TestPushClaimUpdateMergesSameFieldChange(t *testing.T) {
	client, stub := newClaimRedis(t)
	hash := "CommIdempotentKeyValMap"
	if err := client.HSet(context.Background(), hash, "event-a", claimJSON(t, redisModels.MobileChannelRedisData{ClaimedAtUnix: 10, ClaimID: "owner"})).Err(); err != nil {
		t.Fatal(err)
	}
	mutated := claimJSON(t, redisModels.MobileChannelRedisData{TransactionId: "fcm-1", ClaimedAtUnix: 10, ClaimID: "owner"})
	var once sync.Once
	stub.beforeSwap = func(key, field, _, _ string) {
		once.Do(func() {
			stub.hashes[key][field] = mutated
		})
	}

	if err := redis.RefreshPushClaim(client, hash, "event-a", "owner"); err != nil {
		t.Fatal(err)
	}
	stored := readClaim(t, client, hash, "event-a")
	if stored.TransactionId != "fcm-1" || stored.ClaimID != "owner" || stored.ClaimedAtUnix < time.Now().UTC().Unix()-2 {
		t.Fatalf("merged claim = %+v", stored)
	}
}

func TestPushClaimUpdateRejectsOtherOwner(t *testing.T) {
	client, stub := newClaimRedis(t)
	hash := "CommIdempotentKeyValMap"
	original := claimJSON(t, redisModels.MobileChannelRedisData{ClaimedAtUnix: 10, ClaimID: "owner"})
	if err := client.HSet(context.Background(), hash, "event-a", original).Err(); err != nil {
		t.Fatal(err)
	}

	err := redis.RefreshPushClaim(client, hash, "event-a", "other")
	if err == nil || !strings.Contains(err.Error(), "no longer owned") {
		t.Fatalf("other owner error = %v", err)
	}
	if got := stub.field(hash, "event-a"); got != original {
		t.Fatalf("claim changed to %s", got)
	}
}

func TestPushClaimUpdateRequiresExistingJSONClaim(t *testing.T) {
	client, _ := newClaimRedis(t)
	hash := "CommIdempotentKeyValMap"
	if err := redis.RefreshPushClaim(client, hash, "missing", "owner"); !errors.Is(err, goredis.Nil) {
		t.Fatalf("missing field error = %v", err)
	}
	if err := client.HSet(context.Background(), hash, "bad", "not-json").Err(); err != nil {
		t.Fatal(err)
	}
	if err := redis.RefreshPushClaim(client, hash, "bad", "owner"); err == nil || !strings.Contains(err.Error(), "invalid PUSH claim") {
		t.Fatalf("invalid claim error = %v", err)
	}
	if err := redis.RefreshPushClaim(nil, hash, "event-a", "owner"); err == nil || !strings.Contains(err.Error(), "redis client is nil") {
		t.Fatalf("nil client error = %v", err)
	}
	if err := redis.UpdatePushAttemptCount(client, hash, "event-a", " ", 1); err == nil || !strings.Contains(err.Error(), "claim id is required") {
		t.Fatalf("blank claim error = %v", err)
	}
}

func claimJSON(t *testing.T, data redisModels.MobileChannelRedisData) string {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func readClaim(t *testing.T, client *goredis.Client, hash, field string) redisModels.MobileChannelRedisData {
	t.Helper()
	raw, err := client.HGet(context.Background(), hash, field).Result()
	if err != nil {
		t.Fatal(err)
	}
	var data redisModels.MobileChannelRedisData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatal(err)
	}
	return data
}

type claimRedis struct {
	listener   net.Listener
	mu         sync.Mutex
	hashes     map[string]map[string]string
	onHGet     func(key, field string)
	beforeSwap func(key, field, expected, next string)
}

func newClaimRedis(t *testing.T) (*goredis.Client, *claimRedis) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stub := &claimRedis{listener: listener, hashes: make(map[string]map[string]string)}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go stub.serve(conn)
		}
	}()
	client := goredis.NewClient(&goredis.Options{Addr: listener.Addr().String(), Protocol: 2, DisableIdentity: true})
	t.Cleanup(func() {
		_ = client.Close()
		_ = listener.Close()
	})
	return client, stub
}

func (s *claimRedis) field(key, field string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hashes[key][field]
}

func (s *claimRedis) serve(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	for {
		args, err := readRESP(reader)
		if err != nil {
			return
		}
		s.mu.Lock()
		switch strings.ToUpper(args[0]) {
		case "HSET":
			if s.hashes[args[1]] == nil {
				s.hashes[args[1]] = make(map[string]string)
			}
			added := 0
			for i := 2; i+1 < len(args); i += 2 {
				if _, exists := s.hashes[args[1]][args[i]]; !exists {
					added++
				}
				s.hashes[args[1]][args[i]] = args[i+1]
			}
			_, _ = fmt.Fprintf(writer, ":%d\r\n", added)
		case "HGET":
			if s.onHGet != nil {
				s.onHGet(args[1], args[2])
			}
			value, ok := s.hashes[args[1]][args[2]]
			if !ok {
				_, _ = writer.WriteString("$-1\r\n")
			} else {
				writeBulk(writer, value)
			}
		case "EVAL":
			key, field, expected, next := args[3], args[4], args[5], args[6]
			if s.beforeSwap != nil {
				s.beforeSwap(key, field, expected, next)
			}
			current, ok := s.hashes[key][field]
			if !ok || current != expected {
				_, _ = writer.WriteString(":0\r\n")
			} else {
				s.hashes[key][field] = next
				_, _ = writer.WriteString(":1\r\n")
			}
		default:
			_, _ = fmt.Fprintf(writer, "-ERR unsupported command %s\r\n", args[0])
		}
		s.mu.Unlock()
		if err := writer.Flush(); err != nil {
			return
		}
	}
}

func readRESP(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	var count int
	if _, err := fmt.Sscanf(line, "*%d", &count); err != nil {
		return nil, err
	}
	args := make([]string, count)
	for i := range args {
		line, err = r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		var size int
		if _, err = fmt.Sscanf(line, "$%d", &size); err != nil {
			return nil, err
		}
		value := make([]byte, size+2)
		if _, err = io.ReadFull(r, value); err != nil {
			return nil, err
		}
		args[i] = string(value[:size])
	}
	return args, nil
}

func writeBulk(w *bufio.Writer, value string) {
	_, _ = fmt.Fprintf(w, "$%d\r\n%s\r\n", len(value), value)
}
