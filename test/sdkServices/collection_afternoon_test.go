package sdkServices_test

import (
	"bufio"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/config"
	redisInteraction "github.com/wecredit/communication-sdk/internal/redis"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
	"github.com/wecredit/communication-sdk/sdk/queue"
	sdkServices "github.com/wecredit/communication-sdk/sdk/services"
)

func TestCollectionAfternoonSlotValidation(t *testing.T) {
	valid := sdkModels.CommApiRequestBody{CollectionSlot: "afternoon", Client: "zapcash", Channel: "RCS", Stage: 12.07}
	if ok, stage, err := sdkServices.ValidateCollectionAfternoonRequest(valid); !ok || stage != 12 || err != nil {
		t.Fatalf("valid slot = (%t, %d, %v)", ok, stage, err)
	}
	for _, request := range []sdkModels.CommApiRequestBody{
		{CollectionSlot: "morning", Client: "zapcash", Channel: "RCS", Stage: 12.07},
		{CollectionSlot: "afternoon", Client: "other", Channel: "RCS", Stage: 12.07},
		{CollectionSlot: "afternoon", Client: "zapcash", Channel: "SMS", Stage: 12.07},
		{CollectionSlot: "afternoon", Client: "zapcash", Channel: "RCS", Stage: 10.01},
	} {
		if _, _, err := sdkServices.ValidateCollectionAfternoonRequest(request); err == nil {
			t.Fatalf("expected invalid request to fail: %+v", request)
		}
	}
}

func TestCollectionAfternoonClaimKeyUsesISTDateAndWholeStage(t *testing.T) {
	key := sdkServices.CollectionAfternoonClaimKey(time.Date(2026, 10, 1, 15, 5, 0, 0, time.FixedZone("IST", 19800)), "9999999999", 12)
	if key != "zapcash:collection:rcs:afternoon:2026-10-01:9999999999:12" {
		t.Fatalf("claim key = %q", key)
	}
}

func TestCollectionAfternoonClaimRejectsDuplicateAndChecksReleaseToken(t *testing.T) {
	redisClient, mini := testRedis(t)
	defer mini.Close()
	key := "zapcash:collection:rcs:afternoon:2026-10-01:9999999999:12"
	claimed, err := redisInteraction.ClaimCollectionAfternoonKey(redisClient, key, "owner-a", time.Hour)
	if err != nil || !claimed {
		t.Fatalf("first claim = (%t, %v), want (true, nil)", claimed, err)
	}
	claimed, err = redisInteraction.ClaimCollectionAfternoonKey(redisClient, key, "owner-b", time.Hour)
	if err != nil || claimed {
		t.Fatalf("duplicate claim = (%t, %v), want (false, nil)", claimed, err)
	}
	released, err := redisInteraction.ReleaseCollectionAfternoonKey(redisClient, key, "owner-b")
	if err != nil || released || !mini.Exists(key) {
		t.Fatalf("wrong-token release = (%t, %v), exists=%t", released, err, mini.Exists(key))
	}
	released, err = redisInteraction.ReleaseCollectionAfternoonKey(redisClient, key, "owner-a")
	if err != nil || !released || mini.Exists(key) {
		t.Fatalf("owner release = (%t, %v), exists=%t", released, err, mini.Exists(key))
	}
}

func TestProcessCollectionAfternoonClaimLifecycle(t *testing.T) {
	t.Run("claim retained after accepted enqueue and duplicate is rejected", func(t *testing.T) {
		redisClient, mini := testRedis(t)
		defer mini.Close()
		server, requests := fakeSDKQueue(t, http.StatusOK)
		defer server.Close()
		restoreQueue := setSDKQueue(t, server.URL)
		defer restoreQueue()

		request := validAfternoonRequest()
		response, err := sdkServices.ProcessCommApiData(&request, nil, "", server.URL+"/queue", redisClient)
		if err != nil || !response.Success {
			t.Fatalf("first send = (%+v, %v)", response, err)
		}
		_, err = sdkServices.ProcessCommApiData(&request, nil, "", server.URL+"/queue", redisClient)
		if err == nil || !strings.Contains(err.Error(), "data already exists") {
			t.Fatalf("duplicate error = %v, want duplicate rejection", err)
		}
		if got := requests.Load(); got != 1 {
			t.Fatalf("accepted queue calls = %d, want 1", got)
		}
		key := sdkServices.CollectionAfternoonClaimKey(time.Now(), request.Mobile, 12)
		if !mini.Exists(key) {
			t.Fatal("accepted enqueue did not retain afternoon claim")
		}
	})

	t.Run("enqueue failure releases claim for retry", func(t *testing.T) {
		redisClient, mini := testRedis(t)
		defer mini.Close()
		server, requests := fakeSDKQueue(t, http.StatusInternalServerError)
		defer server.Close()
		restoreQueue := setSDKQueue(t, server.URL)
		defer restoreQueue()

		request := validAfternoonRequest()
		if _, err := sdkServices.ProcessCommApiData(&request, nil, "", server.URL+"/queue", redisClient); err == nil {
			t.Fatal("enqueue failure unexpectedly succeeded")
		}
		key := sdkServices.CollectionAfternoonClaimKey(time.Now(), request.Mobile, 12)
		if mini.Exists(key) {
			t.Fatal("enqueue failure retained afternoon claim")
		}
		if _, err := sdkServices.ProcessCommApiData(&request, nil, "", server.URL+"/queue", redisClient); err == nil {
			t.Fatal("retry unexpectedly succeeded against failing queue")
		}
		if got := requests.Load(); got < 2 {
			t.Fatalf("queue calls = %d, want at least 2; retry was blocked by stale claim", got)
		}
	})
}

func TestUntaggedRequestUsesLegacyHashDedupe(t *testing.T) {
	redisClient, mini := testRedis(t)
	defer mini.Close()
	server, requests := fakeSDKQueue(t, http.StatusOK)
	defer server.Close()
	restoreQueue := setSDKQueue(t, server.URL)
	defer restoreQueue()

	oldKey := config.Configs.CommIdempotentKey
	config.Configs.CommIdempotentKey = "test-legacy-idempotency"
	defer func() { config.Configs.CommIdempotentKey = oldKey }()

	request := validAfternoonRequest()
	request.CollectionSlot = ""
	if response, err := sdkServices.ProcessCommApiData(&request, nil, "", server.URL+"/queue", redisClient); err != nil || !response.Success {
		t.Fatalf("legacy first send = (%+v, %v)", response, err)
	}
	if _, err := sdkServices.ProcessCommApiData(&request, nil, "", server.URL+"/queue", redisClient); err == nil {
		t.Fatal("legacy duplicate was not rejected")
	}
	keys, err := mini.HKeys(config.Configs.CommIdempotentKey)
	if err != nil || !contains(keys, request.Mobile+"_RCS_12") {
		t.Fatal("untagged request did not use legacy COMM_IDEMPOTENT_KEY hash")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("legacy queue calls = %d, want 1", got)
	}
}

func validAfternoonRequest() sdkModels.CommApiRequestBody {
	return sdkModels.CommApiRequestBody{Mobile: "9999999999", Client: "zapcash", Channel: "RCS", ProcessName: "ZAPCASH", Stage: 12.07, CollectionSlot: "afternoon"}
}

func testRedis(t *testing.T) (*redis.Client, *testRedisServer) {
	t.Helper()
	server := newTestRedisServer(t)
	client := redis.NewClient(&redis.Options{Addr: server.listener.Addr().String(), Protocol: 2, DisableIdentity: true})
	t.Cleanup(func() { _ = client.Close() })
	return client, server
}

// testRedisServer implements only the Redis commands touched by the SDK's
// afternoon claim and legacy idempotency flows. It uses the Go standard library
// so tests do not need a third-party Redis emulator.
type testRedisServer struct {
	listener net.Listener
	mu       sync.Mutex
	strings  map[string]string
	hashes   map[string]map[string]string
}

func newTestRedisServer(t *testing.T) *testRedisServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &testRedisServer{listener: listener, strings: make(map[string]string), hashes: make(map[string]map[string]string)}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	t.Cleanup(func() { _ = listener.Close() })
	return s
}

func (s *testRedisServer) Close() { _ = s.listener.Close() }

func (s *testRedisServer) Exists(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.strings[key]
	return ok
}

func (s *testRedisServer) HKeys(key string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fields := make([]string, 0, len(s.hashes[key]))
	for field := range s.hashes[key] {
		fields = append(fields, field)
	}
	return fields, nil
}

func (s *testRedisServer) serve(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	for {
		args, err := readRESPCommand(r)
		if err != nil {
			return
		}
		s.mu.Lock()
		switch strings.ToUpper(args[0]) {
		case "SET":
			if _, exists := s.strings[args[1]]; exists {
				_, _ = w.WriteString("$-1\r\n")
			} else {
				s.strings[args[1]] = args[2]
				_, _ = w.WriteString("+OK\r\n")
			}
		case "EVAL":
			key, token := args[3], args[4]
			if s.strings[key] == token {
				delete(s.strings, key)
				_, _ = w.WriteString(":1\r\n")
			} else {
				_, _ = w.WriteString(":0\r\n")
			}
		case "HGET":
			value, ok := s.hashes[args[1]][args[2]]
			if !ok {
				_, _ = w.WriteString("$-1\r\n")
			} else {
				writeRESPBulk(w, value)
			}
		case "HSETNX":
			if s.hashes[args[1]] == nil {
				s.hashes[args[1]] = make(map[string]string)
			}
			if _, ok := s.hashes[args[1]][args[2]]; ok {
				_, _ = w.WriteString(":0\r\n")
			} else {
				s.hashes[args[1]][args[2]] = args[3]
				_, _ = w.WriteString(":1\r\n")
			}
		case "HDEL":
			if _, ok := s.hashes[args[1]][args[2]]; ok {
				delete(s.hashes[args[1]], args[2])
				_, _ = w.WriteString(":1\r\n")
			} else {
				_, _ = w.WriteString(":0\r\n")
			}
		case "HKEYS":
			fields := make([]string, 0, len(s.hashes[args[1]]))
			for field := range s.hashes[args[1]] {
				fields = append(fields, field)
			}
			_, _ = fmt.Fprintf(w, "*%d\r\n", len(fields))
			for _, field := range fields {
				writeRESPBulk(w, field)
			}
		default:
			_, _ = w.WriteString("-ERR unsupported command\r\n")
		}
		s.mu.Unlock()
		if err := w.Flush(); err != nil {
			return
		}
	}
}

func readRESPCommand(r *bufio.Reader) ([]string, error) {
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

func writeRESPBulk(w *bufio.Writer, value string) {
	_, _ = fmt.Fprintf(w, "$%d\r\n%s\r\n", len(value), value)
}

func fakeSDKQueue(t *testing.T, status int) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	requests := &atomic.Int64{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		var request struct {
			MessageBody string `json:"MessageBody"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("parse SQS request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		checksum := md5.Sum([]byte(request.MessageBody))
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = fmt.Fprintf(w, `{"MessageId":"test-message","MD5OfMessageBody":"%x"}`, checksum)
	}))
	return server, requests
}

func setSDKQueue(t *testing.T, endpoint string) func() {
	t.Helper()
	oldClient := queue.SQSClient
	queue.SQSClient = sqs.New(session.Must(session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials("test", "test", ""),
		Endpoint:    aws.String(endpoint),
		Region:      aws.String("ap-south-1"),
	})))
	return func() { queue.SQSClient = oldClient }
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
