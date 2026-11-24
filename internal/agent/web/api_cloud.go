package web

import (
	"cmp"
	"net/http"
	"strings"

	"github.com/chenpinetree/SimCallHub/internal/agent/cloudrelay"
)

type cloudJSON struct {
	cloudrelay.Config
	HasSecret  bool              `json:"hasSecret"`
	SecretHint string            `json:"secretHint"`
	Paired     bool              `json:"paired"`
	Status     cloudrelay.Status `json:"status"`
}

func (s *Server) toCloudJSON(cfg cloudrelay.Config) cloudJSON {
	st := cloudrelay.Status{State: cloudrelay.StateDisabled}
	if s.Cloud != nil {
		st = s.Cloud.Status()
	} else if cfg.Mode == cloudrelay.ModeCloud && !cfg.Paired() {
		st.State = cloudrelay.StateUnpaired
	}
	return cloudJSON{
		Config:     cfg,
		HasSecret:  cfg.DeviceSecret != "",
		SecretHint: keyHint(cfg.DeviceSecret),
		Paired:     cfg.Paired(),
		Status:     st,
	}
}

func (s *Server) getCloud(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Settings.Cloud(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.toCloudJSON(cfg))
}

func (s *Server) putCloud(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode        string `json:"mode"`
		APIBase     string `json:"apiBase"`
		RelayBase   string `json:"relayBase"`
		DeviceName  string `json:"deviceName"`
		PushEnabled *bool  `json:"pushEnabled"`
	}
	if !decode(w, r, &req) {
		return
	}
	saved, err := s.Settings.Cloud(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	next := saved
	if req.Mode != "" {
		next.Mode = req.Mode
	}
	if req.APIBase != "" {
		next.APIBase = req.APIBase
	}
	if req.RelayBase != "" {
		next.RelayBase = req.RelayBase
	}
	if req.DeviceName != "" {
		next.DeviceName = req.DeviceName
	}
	if req.PushEnabled != nil {
		next.PushEnabled = *req.PushEnabled
	}
	next.Normalize()
	if err := next.Validate(); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if saved.Paired() && (hostOf(next.APIBase) != hostOf(saved.APIBase) || hostOf(next.RelayBase) != hostOf(saved.RelayBase)) {
		writeError(w, http.StatusUnprocessableEntity, "更改云端服务器地址前，请先解绑当前云端设备凭证")
		return
	}
	if err := s.Settings.SetCloud(r.Context(), next, nil); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if s.Cloud != nil {
		s.Cloud.Reload()
	}
	if next.Mode != saved.Mode {
		if next.Mode == cloudrelay.ModeCloud {
			s.Devices.Note(r.Context(), "info", "已切换至付费云中继模式")
		} else {
			s.Devices.Note(r.Context(), "info", "已切换至免费直连模式（已断开云端连接）")
		}
	}
	s.getCloud(w, r)
}

func (s *Server) pairCloud(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code      string `json:"code"`
		Name      string `json:"name"`
		APIBase   string `json:"apiBase"`
		RelayBase string `json:"relayBase"`
	}
	if !decode(w, r, &req) {
		return
	}
	saved, err := s.Settings.Cloud(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	apiBase := cmp.Or(strings.TrimSpace(req.APIBase), saved.APIBase, cloudrelay.DefaultAPIBase)
	relayBase := cmp.Or(strings.TrimSpace(req.RelayBase), saved.RelayBase, cloudrelay.DefaultRelayBase)
	name := cmp.Or(strings.TrimSpace(req.Name), saved.DeviceName, "SimCallHub")

	devID, devSecret, err := cloudrelay.Register(r.Context(), apiBase, req.Code, name)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	cfg := cloudrelay.Config{
		Mode:         cloudrelay.ModeCloud,
		APIBase:      apiBase,
		RelayBase:    relayBase,
		DeviceID:     devID,
		DeviceName:   name,
		PushEnabled:  true,
		DeviceSecret: devSecret,
	}
	if err := s.Settings.SetCloud(r.Context(), cfg, &devSecret); err != nil {
		s.serverError(w, err)
		return
	}
	if s.Cloud != nil {
		s.Cloud.Reload()
	}
	s.Devices.Note(r.Context(), "info", "已成功配对 SimCallCloud 并启用云中继模式")
	s.getCloud(w, r)
}

func (s *Server) unbindCloud(w http.ResponseWriter, r *http.Request) {
	if err := s.Settings.ClearCloudDevice(r.Context()); err != nil {
		s.serverError(w, err)
		return
	}
	if s.Cloud != nil {
		s.Cloud.Reload()
	}
	s.Devices.Note(r.Context(), "info", "已清除本地云端凭证并切回免费直连模式")
	s.getCloud(w, r)
}

func (s *Server) testCloudPush(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Settings.Cloud(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !cfg.Paired() {
		writeError(w, http.StatusUnprocessableEntity, "请先输入配对码完成设备绑定")
		return
	}
	err = cloudrelay.Push(r.Context(), cfg, cloudrelay.PushRequest{
		Type:  "alert",
		Title: "SimCallHub 测试推送",
		Body:  "收到这条通知，说明 SimCallCloud 与 iPhone APNs 推送通道连接正常。",
		Payload: map[string]any{
			"event":    "test",
			"deviceId": cfg.DeviceID,
		},
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	ok(w)
}
