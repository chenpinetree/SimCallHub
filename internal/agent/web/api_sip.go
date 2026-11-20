package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/chenpinetree/SimCallHub/internal/agent/sip"
)

type sipUpdateRequest struct {
	Username    string                `json:"username"`
	Password    string                `json:"password"`
	RingTimeout *int                  `json:"ring_timeout"`
	Extensions  []sip.ExtensionConfig `json:"extensions"`
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
			Extensions:  []sip.ExtensionConfig{},
			Clients:     []sip.ClientStatus{},
		})
		return
	}

	writeJSON(w, http.StatusOK, s.SIP.Status())
}

func (s *Server) putSIP(w http.ResponseWriter, r *http.Request) {
	var req sipUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求参数格式无效")
		return
	}

	ringTimeout := 20
	if req.RingTimeout != nil {
		ringTimeout = *req.RingTimeout
	}
	if ringTimeout < -1 {
		ringTimeout = -1
	} else if ringTimeout > 120 {
		ringTimeout = 60
	}
	req.RingTimeout = &ringTimeout

	if len(req.Extensions) > 0 {
		var cleaned []sip.ExtensionConfig
		for _, ext := range req.Extensions {
			ext.Username = strings.TrimSpace(ext.Username)
			ext.Password = strings.TrimSpace(ext.Password)
			ext.DeviceID = strings.TrimSpace(ext.DeviceID)
			ext.Name = strings.TrimSpace(ext.Name)
			if ext.Username != "" && ext.Password != "" {
				cleaned = append(cleaned, ext)
			}
		}
		if len(cleaned) == 0 {
			writeError(w, http.StatusBadRequest, "至少需要保留一个有效的分机账号")
			return
		}
		req.Extensions = cleaned
		req.Username = cleaned[0].Username
		req.Password = cleaned[0].Password
	} else {
		req.Username = strings.TrimSpace(req.Username)
		req.Password = strings.TrimSpace(req.Password)
		if req.Username == "" {
			writeError(w, http.StatusBadRequest, "分机号不能为空")
			return
		}
		if req.Password == "" {
			writeError(w, http.StatusBadRequest, "分机密码不能为空")
			return
		}
	}

	// Persist encrypted to store before applying to live server
	data, err := json.Marshal(req)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if s.Settings != nil {
		if err := s.Settings.SetSIPConfig(r.Context(), string(data)); err != nil {
			s.serverError(w, err)
			return
		}
	} else if s.Store != nil {
		if err := s.Store.SetSetting(r.Context(), "sip_config", string(data)); err != nil {
			s.serverError(w, err)
			return
		}
	}

	if s.SIP != nil {
		if len(req.Extensions) > 0 {
			s.SIP.UpdateExtensions(req.Extensions, ringTimeout)
		} else {
			s.SIP.UpdateConfig(req.Username, req.Password, ringTimeout)
		}
		writeJSON(w, http.StatusOK, s.SIP.Status())
	} else {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
