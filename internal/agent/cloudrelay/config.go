package cloudrelay

import (
	"errors"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	ModeDirect = "direct"
	ModeCloud  = "cloud"

	DefaultAPIBase   = "https://api.sim-callhub.com"
	DefaultRelayBase = "wss://relay.sim-callhub.com"
)

// Config configures the dual-mode access architecture:
//   - "direct": completely offline/private, zero external connections.
//   - "cloud": maintains a yamux reverse tunnel to SimCallCloud and relays APNs pushes.
type Config struct {
	Mode         string `json:"mode"`
	APIBase      string `json:"apiBase"`
	RelayBase    string `json:"relayBase"`
	DeviceID     string `json:"deviceId"`
	DeviceName   string `json:"deviceName"`
	PushEnabled  bool   `json:"pushEnabled"`
	DeviceSecret string `json:"-"`
}

// DefaultConfig returns the safe default configuration (free direct mode).
func DefaultConfig() Config {
	apiBase := strings.TrimSpace(os.Getenv("SIMCALLHUB_CLOUD_API"))
	if apiBase == "" {
		apiBase = DefaultAPIBase
	}
	relayBase := strings.TrimSpace(os.Getenv("SIMCALLHUB_CLOUD_RELAY"))
	if relayBase == "" {
		relayBase = DefaultRelayBase
	}
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("SIMCALLHUB_MODE")))
	if mode != ModeCloud {
		mode = ModeDirect
	}
	return Config{
		Mode:        mode,
		APIBase:     strings.TrimRight(apiBase, "/"),
		RelayBase:   strings.TrimRight(relayBase, "/"),
		DeviceName:  "SimCallHub",
		PushEnabled: true,
	}
}

// Normalize fills missing defaults and trims whitespace.
func (c *Config) Normalize() {
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	if c.Mode == "" {
		c.Mode = ModeDirect
	}
	c.APIBase = strings.TrimRight(strings.TrimSpace(c.APIBase), "/")
	if c.APIBase == "" {
		c.APIBase = DefaultAPIBase
	}
	c.RelayBase = strings.TrimRight(strings.TrimSpace(c.RelayBase), "/")
	if c.RelayBase == "" {
		c.RelayBase = DefaultRelayBase
	}
	c.DeviceID = strings.TrimSpace(c.DeviceID)
	c.DeviceName = strings.TrimSpace(c.DeviceName)
	if c.DeviceName == "" {
		c.DeviceName = "SimCallHub"
	}
	c.DeviceSecret = strings.TrimSpace(c.DeviceSecret)
}

// Validate checks fields before persisting.
func (c Config) Validate() error {
	if c.Mode != ModeDirect && c.Mode != ModeCloud {
		return errors.New("接入模式必须是 direct（直连）或 cloud（云中继）")
	}
	if err := validateHTTPBase(c.APIBase); err != nil {
		return errors.New("云端 API 地址无效：" + err.Error())
	}
	if err := validateWSBase(c.RelayBase); err != nil {
		return errors.New("云端中继地址无效：" + err.Error())
	}
	return nil
}

// Paired reports whether the device holds a registered cloud credential.
func (c Config) Paired() bool {
	return c.DeviceID != "" && c.DeviceSecret != ""
}

// Ready reports whether cloud relay mode is enabled and paired.
func (c Config) Ready() bool {
	return c.Mode == ModeCloud && c.Paired()
}

// PushReady reports whether iOS APNs/VoIP push via SimCallCloud should run.
func (c Config) PushReady() bool {
	return c.Ready() && c.PushEnabled
}

// Status reflects the live runtime state of the yamux cloud tunnel.
type Status struct {
	State          string     `json:"state"` // disabled, unpaired, connecting, connected, error
	ConnectedSince *time.Time `json:"connectedSince,omitempty"`
	LastError      string     `json:"lastError,omitempty"`
}

func validateHTTPBase(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("需要完整的 URL，例如 https://api.sim-callhub.com")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("协议必须是 https:// 或 http://")
	}
	return nil
}

func validateWSBase(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("需要完整的 WebSocket URL，例如 wss://relay.sim-callhub.com")
	}
	if u.Scheme != "wss" && u.Scheme != "ws" {
		return errors.New("协议必须是 wss:// 或 ws://")
	}
	return nil
}
