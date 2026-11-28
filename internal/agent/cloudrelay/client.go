package cloudrelay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/hashicorp/yamux"
)

const (
	StateDisabled   = "disabled"
	StateUnpaired   = "unpaired"
	StateConnecting = "connecting"
	StateConnected  = "connected"
	StateError      = "error"
)

// PushRequest matches SimCallCloud's POST /agent/push payload.
type PushRequest struct {
	Type    string         `json:"type"` // "alert" or "voip"
	Title   string         `json:"title,omitempty"`
	Body    string         `json:"body,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// Register exchanges a 6-digit pairing code from the iOS App for a deviceId
// and deviceSecret from SimCallCloud.
func Register(ctx context.Context, apiBase, code, name string) (deviceID, deviceSecret string, err error) {
	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if apiBase == "" {
		apiBase = DefaultAPIBase
	}
	if err := validateHTTPBase(apiBase); err != nil {
		return "", "", err
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return "", "", errors.New("请填写配对码")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "SimCallHub"
	}

	body, _ := json.Marshal(map[string]string{
		"code": code,
		"name": name,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/api/v1/agent/register", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("连接云端注册服务失败：%w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("云端配对失败（%d）：%s", resp.StatusCode, parseAPIError(raw, resp.Status))
	}
	var out struct {
		DeviceID     string `json:"deviceId"`
		DeviceSecret string `json:"deviceSecret"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.DeviceID == "" || out.DeviceSecret == "" {
		return "", "", errors.New("云端返回了无效的配对凭证")
	}
	return out.DeviceID, out.DeviceSecret, nil
}

// Push sends an alert or VoIP notification request to SimCallCloud.
func Push(ctx context.Context, cfg Config, pr PushRequest) error {
	cfg.Normalize()
	if !cfg.Ready() {
		return errors.New("云中继未配置或未配对")
	}
	body, err := json.Marshal(pr)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.APIBase+"/agent/push", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Device-ID", cfg.DeviceID)
	req.Header.Set("X-Device-Secret", cfg.DeviceSecret)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求云端推送失败：%w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return &PushError{
			StatusCode: resp.StatusCode,
			Message:    parseAPIError(raw, resp.Status),
		}
	}
	return nil
}

// PushError represents a non-200 HTTP response from SimCallCloud /agent/push.
type PushError struct {
	StatusCode int
	Message    string
}

func (e *PushError) Error() string {
	return fmt.Sprintf("云端推送拒绝（%d）：%s", e.StatusCode, e.Message)
}

// Permanent reports whether retrying the exact same push cannot succeed.
func (e *PushError) Permanent() bool {
	return e.StatusCode == http.StatusBadRequest ||
		e.StatusCode == http.StatusUnauthorized ||
		e.StatusCode == http.StatusPaymentRequired ||
		e.StatusCode == http.StatusPreconditionFailed
}

// Client manages the outbound yamux reverse tunnel to SimCallCloud when mode == "cloud".
type Client struct {
	Config func(context.Context) (Config, error)
	Note   func(ctx context.Context, level, text string)
	Log    *slog.Logger

	reloadCh chan struct{}

	mu      sync.RWMutex
	handler http.Handler
	status  Status
	cancel  context.CancelFunc // cancels the current active tunnel
}

// NewClient creates a cloud relay client. Call SetHandler before Run.
func NewClient(cfgFn func(context.Context) (Config, error), noteFn func(context.Context, string, string), log *slog.Logger) *Client {
	return &Client{
		Config:   cfgFn,
		Note:     noteFn,
		Log:      log,
		reloadCh: make(chan struct{}, 1),
		status:   Status{State: StateDisabled},
	}
}

// SetHandler sets the local HTTP handler that serves requests tunneled from SimCallCloud.
func (c *Client) SetHandler(h http.Handler) {
	c.mu.Lock()
	c.handler = h
	c.mu.Unlock()
}

// Reload wakes the client loop and tears down any active tunnel so updated
// mode or credentials take effect immediately.
func (c *Client) Reload() {
	c.mu.RLock()
	cancel := c.cancel
	c.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
	select {
	case c.reloadCh <- struct{}{}:
	default:
	}
}

// Status returns a snapshot of the tunnel state.
func (c *Client) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *Client) setStatus(s Status) {
	c.mu.Lock()
	c.status = s
	c.mu.Unlock()
}

func (c *Client) setCancel(fn context.CancelFunc) {
	c.mu.Lock()
	c.cancel = fn
	c.mu.Unlock()
}

func (c *Client) note(ctx context.Context, level, text string) {
	if c.Note != nil {
		c.Note(ctx, level, text)
	} else if c.Log != nil {
		c.Log.Info(text)
	}
}

// Run maintains the reverse tunnel while mode == "cloud" until ctx is canceled.
func (c *Client) Run(ctx context.Context) {
	backoff := 2 * time.Second
	wasFailing := false
	lastErrMsg := ""

	for {
		if ctx.Err() != nil {
			return
		}
		cfg, err := c.Config(ctx)
		if err != nil {
			c.setStatus(Status{State: StateError, LastError: err.Error()})
			if !c.waitOrReload(ctx, 5*time.Second) {
				return
			}
			continue
		}
		cfg.Normalize()

		if cfg.Mode != ModeCloud {
			c.setStatus(Status{State: StateDisabled})
			backoff = 2 * time.Second
			wasFailing = false
			lastErrMsg = ""
			if !c.waitReloadOnly(ctx) {
				return
			}
			continue
		}
		if !cfg.Paired() {
			c.setStatus(Status{State: StateUnpaired})
			backoff = 2 * time.Second
			wasFailing = false
			lastErrMsg = ""
			if !c.waitReloadOnly(ctx) {
				return
			}
			continue
		}

		c.setStatus(Status{State: StateConnecting, LastError: lastErrMsg})
		started := time.Now()
		err = c.connectAndServe(ctx, cfg)
		if ctx.Err() != nil {
			return
		}

		// If reload was triggered intentionally, reconnect immediately without backoff.
		if c.consumeReload() {
			backoff = 2 * time.Second
			continue
		}

		dur := time.Since(started)
		if dur > 30*time.Second {
			backoff = 2 * time.Second
			wasFailing = false
		}

		msg := "云中继连接中断"
		wait := backoff
		if err != nil {
			msg = err.Error()
			var dialErr *dialStatusError
			if errors.As(err, &dialErr) && dialErr.SlowRetry() {
				wait = 60 * time.Second
			} else {
				backoff *= 2
				if backoff > 60*time.Second {
					backoff = 60 * time.Second
				}
			}
		}
		lastErrMsg = msg
		c.setStatus(Status{State: StateError, LastError: msg})
		if !wasFailing {
			wasFailing = true
			c.note(ctx, "warn", "云端中继断开，将自动重连："+msg)
		} else if c.Log != nil {
			c.Log.Warn("cloud relay reconnect scheduled", "wait", wait, "error", msg)
		}

		if !c.waitOrReload(ctx, wait) {
			return
		}
	}
}

func (c *Client) connectAndServe(ctx context.Context, cfg Config) error {
	c.mu.RLock()
	h := c.handler
	c.mu.RUnlock()
	if h == nil {
		return errors.New("本地 Web 服务尚未就绪")
	}

	sessCtx, cancel := context.WithCancel(ctx)
	c.setCancel(cancel)
	defer func() {
		cancel()
		c.setCancel(nil)
	}()

	dialCtx, dialCancel := context.WithTimeout(sessCtx, 15*time.Second)
	defer dialCancel()

	relayURL := strings.TrimRight(cfg.RelayBase, "/") + "/agent/connect"
	headers := http.Header{}
	headers.Set("X-Device-ID", cfg.DeviceID)
	headers.Set("X-Device-Secret", cfg.DeviceSecret)

	ws, resp, err := websocket.Dial(dialCtx, relayURL, &websocket.DialOptions{
		HTTPHeader: headers,
	})
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
			return &dialStatusError{
				StatusCode: resp.StatusCode,
				Message:    humanizeCloudReject(resp.StatusCode, parseAPIError(raw, resp.Status)),
			}
		}
		return fmt.Errorf("连接云端失败：%w", err)
	}
	defer ws.CloseNow()

	nc := websocket.NetConn(sessCtx, ws, websocket.MessageBinary)
	ycfg := yamux.DefaultConfig()
	ycfg.KeepAliveInterval = 20 * time.Second
	ycfg.ConnectionWriteTimeout = 10 * time.Second
	ycfg.EnableKeepAlive = true

	sess, err := yamux.Client(nc, ycfg)
	if err != nil {
		return fmt.Errorf("初始化 yamux 隧道失败：%w", err)
	}
	defer sess.Close()

	now := time.Now()
	prev := c.Status()
	c.setStatus(Status{State: StateConnected, ConnectedSince: &now})
	if prev.State == StateError || prev.State == StateConnecting {
		c.note(ctx, "info", "已连接 SimCallCloud 云端中继")
	}
	if c.Log != nil {
		c.Log.Info("cloud relay tunnel connected", "device", cfg.DeviceID, "relay", cfg.RelayBase)
	}

	srv := &http.Server{
		Handler:           WithRelayContext(h, cfg.DeviceID),
		ReadHeaderTimeout: 10 * time.Second,
	}
	// Close the yamux session when sessCtx is canceled (shutdown or reload).
	go func() {
		<-sessCtx.Done()
		_ = sess.Close()
		_ = srv.Close()
	}()

	serveErr := srv.Serve(sess)
	if sessCtx.Err() != nil {
		return nil
	}
	return serveErr
}

func (c *Client) waitReloadOnly(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-c.reloadCh:
		return true
	}
}

func (c *Client) waitOrReload(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-c.reloadCh:
		return true
	case <-t.C:
		return true
	}
}

func (c *Client) consumeReload() bool {
	select {
	case <-c.reloadCh:
		return true
	default:
		return false
	}
}

type dialStatusError struct {
	StatusCode int
	Message    string
}

func (e *dialStatusError) Error() string { return e.Message }

func (e *dialStatusError) SlowRetry() bool {
	return e.StatusCode == http.StatusUnauthorized ||
		e.StatusCode == http.StatusPaymentRequired ||
		e.StatusCode == http.StatusTooManyRequests
}

func humanizeCloudReject(status int, detail string) string {
	switch status {
	case http.StatusUnauthorized:
		return "设备未绑定或密钥已失效（请在 App 重新生成配对码绑定）"
	case http.StatusPaymentRequired:
		return "云端订阅已过期，续费后将自动恢复中继"
	case http.StatusTooManyRequests:
		return "检测到多处重复部署冲突，云端已临时锁定该设备 10 分钟"
	default:
		return fmt.Sprintf("云端拒绝接入（%d）：%s", status, detail)
	}
}

func parseAPIError(raw []byte, fallback string) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error != "" {
		return e.Error
	}
	if s := strings.TrimSpace(string(raw)); s != "" && len(s) < 200 {
		return s
	}
	return fallback
}
