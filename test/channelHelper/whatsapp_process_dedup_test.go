package channelHelper_test

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"

	goredis "github.com/redis/go-redis/v9"
	"github.com/wecredit/communication-sdk/internal/channels/channelHelper"
	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

func TestGenerateWhatsappProcessDedupKey(t *testing.T) {
	if got := channelHelper.GenerateWhatsappProcessDedupKey(" Offer ", "7014850582"); got != "wecredit_whatsapp_offer_7014850582" {
		t.Fatalf("process key = %q", got)
	}
	unflagged := sdkModels.CommApiRequestBody{Client: "wecredit", Channel: "WHATSAPP", Mobile: "7014850582", ProcessName: "Offer"}
	if got := channelHelper.GenerateMarketingCampaignDedupKey(unflagged); got != "wecredit_whatsapp_7014850582" {
		t.Fatalf("unflagged mobile key = %q", got)
	}
}

func TestFlaggedWhatsappDedup(t *testing.T) {
	client, stub := newDedupRedis(t)

	first := flaggedRow("marketing-1", "Offer", "7014850582")
	if _, err := channelHelper.ClaimFlaggedWeCreditWhatsapp(client, first); err != nil {
		t.Fatal(err)
	}

	second := flaggedRow("marketing-2", "Offer", "7014850582")
	_, err := channelHelper.ClaimFlaggedWeCreditWhatsapp(client, second)
	if !errors.Is(err, channelHelper.ErrWhatsappAlreadySentForProcess) {
		t.Fatalf("second row error = %v", err)
	}

	otherProcess := flaggedRow("marketing-3", "FollowUp", "7014850582")
	stub.set("wecredit_whatsapp_7014850582", "marketing-1")
	if _, err := channelHelper.ClaimFlaggedWeCreditWhatsapp(client, otherProcess); err != nil {
		t.Fatalf("mobile key present should still send another process: %v", err)
	}
	if got, _ := stub.get("wecredit_whatsapp_7014850582"); got != "marketing-1" {
		t.Fatalf("foreign mobile key changed to %q", got)
	}

	channelHelper.ReleaseFlaggedWeCreditWhatsapp(client, otherProcess)
	if _, ok := stub.get("wecredit_whatsapp_followup_7014850582"); ok {
		t.Fatal("failed send kept the process key")
	}
	if got, _ := stub.get("wecredit_whatsapp_7014850582"); got != "marketing-1" {
		t.Fatal("failed send deleted a foreign mobile key")
	}
}

func TestFlaggedWhatsappEmptyProcessAndRedisError(t *testing.T) {
	client, stub := newDedupRedis(t)
	empty := flaggedRow("marketing-9", " ", "7014850582")
	_, err := channelHelper.ClaimFlaggedWeCreditWhatsapp(client, empty)
	if !errors.Is(err, channelHelper.ErrWhatsappProcessRequired) {
		t.Fatalf("empty process error = %v", err)
	}
	if n := stub.len(); n != 0 {
		t.Fatalf("empty process wrote %d keys", n)
	}

	stub.fail("wecredit_whatsapp_7014850582")
	row := flaggedRow("marketing-4", "Offer", "7014850582")
	_, err = channelHelper.ClaimFlaggedWeCreditWhatsapp(client, row)
	if err == nil || errors.Is(err, channelHelper.ErrWhatsappAlreadySentForProcess) || errors.Is(err, channelHelper.ErrWhatsappProcessRequired) {
		t.Fatalf("redis error = %v, want a retryable claim error", err)
	}
	if _, ok := stub.get("wecredit_whatsapp_offer_7014850582"); ok {
		t.Fatal("redis error left the process key")
	}
}

func TestCampaignDuplicateMessage(t *testing.T) {
	unflagged := flaggedRow("marketing-1", "Offer", "7014850582")
	unflagged.IsFollowUpMessage = 0
	if got := channelHelper.CampaignDuplicateMessage(unflagged); got != "whatsapp already sent today for mobile 7014850582" {
		t.Fatalf("flag 0 message = %q", got)
	}
	flagged := flaggedRow("marketing-1", "Offer", "7014850582")
	got := channelHelper.CampaignDuplicateMessage(flagged)
	if got == channelHelper.CampaignDuplicateMessage(unflagged) || !strings.Contains(got, "for this process offer") || strings.HasPrefix(got, "campaign duplicate:") {
		t.Fatalf("flag 1 message = %q", got)
	}
}

func flaggedRow(eventID, process, mobile string) sdkModels.CommApiRequestBody {
	return sdkModels.CommApiRequestBody{
		EventId:           eventID,
		Client:            "wecredit",
		Channel:           "WHATSAPP",
		ProcessName:       process,
		Mobile:            mobile,
		IsFollowUpMessage: 1,
	}
}

func newDedupRedis(t *testing.T) (*goredis.Client, *dedupRedis) {
	t.Helper()
	server := &dedupRedis{strings: map[string]string{}, failSetNX: map[string]bool{}}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server.listener = listener
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go server.serve(conn)
		}
	}()
	client := goredis.NewClient(&goredis.Options{Addr: listener.Addr().String(), Protocol: 2, DisableIdentity: true})
	t.Cleanup(func() {
		_ = client.Close()
		_ = listener.Close()
	})
	return client, server
}

type dedupRedis struct {
	listener  net.Listener
	mu        sync.Mutex
	strings   map[string]string
	failSetNX map[string]bool
}

func (s *dedupRedis) set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.strings[key] = value
}

func (s *dedupRedis) get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.strings[key]
	return value, ok
}

func (s *dedupRedis) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.strings)
}

func (s *dedupRedis) fail(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failSetNX[key] = true
}

func (s *dedupRedis) serve(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	for {
		args, err := readDedupRESP(r)
		if err != nil {
			return
		}
		s.mu.Lock()
		switch strings.ToUpper(args[0]) {
		case "SETNX":
			if s.failSetNX[args[1]] {
				_, _ = w.WriteString("-ERR redis down\r\n")
				break
			}
			if _, ok := s.strings[args[1]]; ok {
				_, _ = w.WriteString(":0\r\n")
			} else {
				s.strings[args[1]] = args[2]
				_, _ = w.WriteString(":1\r\n")
			}
		case "GET":
			value, ok := s.strings[args[1]]
			if !ok {
				_, _ = w.WriteString("$-1\r\n")
			} else {
				_, _ = fmt.Fprintf(w, "$%d\r\n%s\r\n", len(value), value)
			}
		case "DEL":
			if _, ok := s.strings[args[1]]; ok {
				delete(s.strings, args[1])
				_, _ = w.WriteString(":1\r\n")
			} else {
				_, _ = w.WriteString(":0\r\n")
			}
		case "EVAL":
			if len(args) < 5 {
				_, _ = w.WriteString("-ERR eval arity\r\n")
				break
			}
			key, value := args[3], args[4]
			if current, ok := s.strings[key]; ok && current == value {
				delete(s.strings, key)
				_, _ = w.WriteString(":1\r\n")
			} else {
				_, _ = w.WriteString(":0\r\n")
			}
		default:
			_, _ = fmt.Fprintf(w, "-ERR unsupported %s\r\n", args[0])
		}
		s.mu.Unlock()
		if err := w.Flush(); err != nil {
			return
		}
	}
}

func readDedupRESP(r *bufio.Reader) ([]string, error) {
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
