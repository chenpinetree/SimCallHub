package web

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/chenpinetree/SimCallHub/internal/agent/esim"
	"github.com/chenpinetree/SimCallHub/internal/agent/modem"
)

var esimManager = esim.NewManager()

// GET /api/devices/{id}/esim
func (s *Server) deviceESimInfo(w http.ResponseWriter, r *http.Request) {
	d, ok := s.device(w, r)
	if !ok {
		return
	}

	var info *esim.EUICCInfo
	err := d.Do(r.Context(), func(ctx context.Context, m *modem.Modem) error {
		var qErr error
		info, qErr = esimManager.GetInfo(ctx, m)
		return qErr
	})

	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"supported": false,
			"error":     err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, info)
}

// POST /api/devices/{id}/esim/enable
func (s *Server) deviceESimEnable(w http.ResponseWriter, r *http.Request) {
	d, ok := s.device(w, r)
	if !ok {
		return
	}

	var req struct {
		ICCID string `json:"iccid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ICCID == "" {
		writeError(w, http.StatusBadRequest, "invalid iccid")
		return
	}

	err := d.Do(r.Context(), func(ctx context.Context, m *modem.Modem) error {
		return esimManager.EnableProfile(ctx, m, req.ICCID)
	})
	if err != nil {
		s.serverError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /api/devices/{id}/esim/disable
func (s *Server) deviceESimDisable(w http.ResponseWriter, r *http.Request) {
	d, ok := s.device(w, r)
	if !ok {
		return
	}

	var req struct {
		ICCID string `json:"iccid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ICCID == "" {
		writeError(w, http.StatusBadRequest, "invalid iccid")
		return
	}

	err := d.Do(r.Context(), func(ctx context.Context, m *modem.Modem) error {
		return esimManager.DisableProfile(ctx, m, req.ICCID)
	})
	if err != nil {
		s.serverError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /api/devices/{id}/esim/delete
func (s *Server) deviceESimDelete(w http.ResponseWriter, r *http.Request) {
	d, ok := s.device(w, r)
	if !ok {
		return
	}

	var req struct {
		ICCID string `json:"iccid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ICCID == "" {
		writeError(w, http.StatusBadRequest, "invalid iccid")
		return
	}

	err := d.Do(r.Context(), func(ctx context.Context, m *modem.Modem) error {
		return esimManager.DeleteProfile(ctx, m, req.ICCID)
	})
	if err != nil {
		s.serverError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /api/devices/{id}/esim/nickname
func (s *Server) deviceESimNickname(w http.ResponseWriter, r *http.Request) {
	d, ok := s.device(w, r)
	if !ok {
		return
	}

	var req struct {
		ICCID    string `json:"iccid"`
		Nickname string `json:"nickname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ICCID == "" {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	err := d.Do(r.Context(), func(ctx context.Context, m *modem.Modem) error {
		return esimManager.SetNickname(ctx, m, req.ICCID, req.Nickname)
	})
	if err != nil {
		s.serverError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
