package cloudrelay

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hashicorp/yamux"
)

func TestRegisterAndPush(t *testing.T) {
	var gotPush PushRequest
	var gotID, gotSecret string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/register":
			var req struct {
				Code string `json:"code"`
				Name string `json:"name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Code != "123456" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":"invalid code"}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"deviceId":     "dev-abc",
				"deviceSecret": "sec-xyz",
			})
		case "/agent/push":
			gotID = r.Header.Get("X-Device-ID")
			gotSecret = r.Header.Get("X-Device-Secret")
			_ = json.NewDecoder(r.Body).Decode(&gotPush)
			_, _ = io.WriteString(w, `{"ok":true}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	ctx := context.Background()
	devID, devSecret, err := Register(ctx, ts.URL, "123456", "MyNAS")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if devID != "dev-abc" || devSecret != "sec-xyz" {
		t.Fatalf("unexpected credentials: %q %q", devID, devSecret)
	}

	cfg := Config{
		Mode:         ModeCloud,
		APIBase:      ts.URL,
		RelayBase:    "ws" + strings.TrimPrefix(ts.URL, "http"),
		DeviceID:     devID,
		DeviceSecret: devSecret,
		PushEnabled:  true,
	}
	if err := Push(ctx, cfg, PushRequest{
		Type:    "voip",
		Payload: map[string]any{"caller": "10086"},
	}); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if gotID != "dev-abc" || gotSecret != "sec-xyz" || gotPush.Type != "voip" {
		t.Fatalf("unexpected push captured: id=%q sec=%q req=%+v", gotID, gotSecret, gotPush)
	}
}

func TestTunnelHTTPWebSocketAndModeSwitch(t *testing.T) {
	var (
		mu      sync.Mutex
		cloudSs *yamux.Session
		closed  atomic.Bool
	)

	relaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent/connect" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("X-Device-ID") != "dev-1" || r.Header.Get("X-Device-Secret") != "sec-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer ws.CloseNow()

		nc := websocket.NetConn(r.Context(), ws, websocket.MessageBinary)
		sess, err := yamux.Server(nc, yamux.DefaultConfig())
		if err != nil {
			return
		}
		mu.Lock()
		cloudSs = sess
		mu.Unlock()
		<-sess.CloseChan()
		closed.Store(true)
	}))
	defer relaySrv.Close()

	wsURL := "ws" + strings.TrimPrefix(relaySrv.URL, "http")

	var cfgMu sync.Mutex
	curCfg := Config{
		Mode:         ModeCloud,
		APIBase:      relaySrv.URL,
		RelayBase:    wsURL,
		DeviceID:     "dev-1",
		DeviceSecret: "sec-1",
		PushEnabled:  true,
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewClient(func(context.Context) (Config, error) {
		cfgMu.Lock()
		defer cfgMu.Unlock()
		return curCfg, nil
	}, nil, log)

	localMux := http.NewServeMux()
	localMux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		info, ok := FromRequest(r)
		if !ok || info.DeviceID != "dev-1" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = io.WriteString(w, "pong:"+info.UserID)
	})
	localMux.HandleFunc("GET /api/ws", func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer ws.CloseNow()
		typ, msg, err := ws.Read(r.Context())
		if err != nil {
			return
		}
		_ = ws.Write(r.Context(), typ, append([]byte("echo:"), msg...))
	})
	client.SetHandler(localMux)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go client.Run(ctx)

	// Wait for tunnel to connect.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		s := cloudSs
		mu.Unlock()
		if s != nil && client.Status().State == StateConnected {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	sess := cloudSs
	mu.Unlock()
	if sess == nil || client.Status().State != StateConnected {
		t.Fatalf("tunnel did not connect, status=%+v", client.Status())
	}

	// Open HTTP stream through yamux from simulated cloud side.
	stream, err := sess.Open()
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	req, _ := http.NewRequest("GET", "http://127.0.0.1/api/ping", nil)
	req.Header.Set("X-SimCallCloud-User", "user-42")
	if err := req.Write(stream); err != nil {
		t.Fatalf("write http req: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(stream), req)
	if err != nil {
		t.Fatalf("read http resp: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	stream.Close()
	if resp.StatusCode != 200 || string(body) != "pong:user-42" {
		t.Fatalf("unexpected http response: %d %q", resp.StatusCode, string(body))
	}

	// Open WebSocket Upgrade stream through yamux (matching /api/devices/{id}/call/audio).
	wsStream, err := sess.Open()
	if err != nil {
		t.Fatalf("open ws stream: %v", err)
	}
	wsReq, _ := http.NewRequest("GET", "http://127.0.0.1/api/ws", nil)
	wsReq.Header.Set("Connection", "Upgrade")
	wsReq.Header.Set("Upgrade", "websocket")
	wsReq.Header.Set("Sec-WebSocket-Version", "13")
	wsReq.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	if err := wsReq.Write(wsStream); err != nil {
		t.Fatalf("write ws upgrade: %v", err)
	}
	wsResp, err := http.ReadResponse(bufio.NewReader(wsStream), wsReq)
	if err != nil {
		t.Fatalf("read ws upgrade response: %v", err)
	}
	if wsResp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101 Switching Protocols over yamux, got %d", wsResp.StatusCode)
	}
	wsStream.Close()

	// Switch back to free direct mode and ensure tunnel closes immediately.
	cfgMu.Lock()
	curCfg.Mode = ModeDirect
	cfgMu.Unlock()
	client.Reload()

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if client.Status().State == StateDisabled && closed.Load() {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if client.Status().State != StateDisabled || !closed.Load() {
		t.Fatalf("expected tunnel to disable & close, status=%+v closed=%v", client.Status(), closed.Load())
	}
}
