package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/chenpinetree/SimCallHub/internal/agent/sip"
)

type sipUpdateRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	RingTimeout int    `json:"ring_timeout"`
}

func (s *Server) getSIP(w http.ResponseWriter, r *http.Request) {
	if s.SIP == nil {
		writeJSON(w, http.StatusOK, sip.ServerStatus{
			Enabled:     false,
			ListenAddr:  "0.0.0.0:5060",
			Domain:      "SimCallHub",
			Username:    "1001",
			Password:    "12345678",
			RingTimeout: 20,
			Clients:     []sip.ClientStatus{},
		})
		return
	}

	writeJSON(w, http.StatusOK, s.SIP.Status())
}

func (s *Server) putSIP(w http.ResponseWriter, r *http.Request) {
	var req sipUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)
	if req.Username == "" {
		http.Error(w, "分机号不能为空", http.StatusBadRequest)
		return
	}
	if req.Password == "" {
		http.Error(w, "分机密码不能为空", http.StatusBadRequest)
		return
	}
	if req.RingTimeout <= 0 {
		req.RingTimeout = 20
	}

	if s.SIP != nil {
		s.SIP.UpdateConfig(req.Username, req.Password, req.RingTimeout)
	}

	// Persist to store
	if s.Store != nil {
		data, _ := json.Marshal(req)
		_ = s.Store.SetSetting(r.Context(), "sip_config", string(data))
	}

	if s.SIP != nil {
		writeJSON(w, http.StatusOK, s.SIP.Status())
	} else {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
