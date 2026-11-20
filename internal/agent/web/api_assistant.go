package web

import (
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chenpinetree/SimCallHub/internal/agent/assistant"
	"github.com/chenpinetree/SimCallHub/internal/agent/service"
)

type assistantJSON struct {
	AnswerAt *time.Time           `json:"answerAt"`
	Active   bool                 `json:"active"`
	RecordID int64                `json:"recordId"`
	Phase    string               `json:"phase"`
	Error    string               `json:"error"`
	Lines    []transcriptLineJSON `json:"lines"`
}

type transcriptLineJSON struct {
	Role  string    `json:"role"`
	Text  string    `json:"text"`
	Final bool      `json:"final"`
	At    time.Time `json:"at"`
}

func toAssistantJSON(a service.AssistantState) assistantJSON {
	out := assistantJSON{AnswerAt: timePtr(a.AnswerAt), Active: a.Active, RecordID: a.RecordID, Phase: a.Phase, Error: a.Error, Lines: []transcriptLineJSON{}}
	for _, l := range a.Lines {
		out.Lines = append(out.Lines, transcriptLineJSON{l.Role, l.Text, l.Final, l.At})
	}
	return out
}

type assistantSettingsJSON struct {
	assistant.Config
	// Endpoint is the realtime URL in use, derived or configured;
	// DerivedEndpoint is what an empty URL means.
	Endpoint        string   `json:"endpoint"`
	DerivedEndpoint string   `json:"derivedEndpoint"`
	WorkspaceID     string   `json:"workspaceId"` // from the classifier's Base URL, if any
	Models          []string `json:"models"`
	HasOwnKey       bool     `json:"hasOwnKey"`
	OwnKeyHint      string   `json:"ownKeyHint"`
	HasSharedKey    bool     `json:"hasSharedKey"` // the classifier has a key
	Problem         string   `json:"problem"`      // why calls cannot be answered with this setup
}

// assistantRequest is the form. APIKey nil keeps the stored key; "" removes
// it.
type assistantRequest struct {
	assistant.Config
	APIKey *string `json:"apiKey"`
}

func (s *Server) assistantSettings(r *http.Request, cfg assistant.Config) assistantSettingsJSON {
	out := assistantSettingsJSON{Config: cfg, Models: assistant.Models, HasOwnKey: cfg.APIKey != "", OwnKeyHint: keyHint(cfg.APIKey)}
	llm, err := s.Settings.LLM(r.Context())
	if err != nil {
		out.Problem = err.Error()
		return out
	}
	out.HasSharedKey = llm.APIKey != ""
	derived := cfg
	derived.URL = ""
	if d, err := derived.Endpoint(llm); err == nil {
		out.DerivedEndpoint = d
		if u, err := url.Parse(d); err == nil {
			if id, _, ok := strings.Cut(u.Host, "."); ok && strings.HasPrefix(id, "ws-") {
				out.WorkspaceID = id
			}
		}
	}
	if err := cfg.Validate(); err != nil {
		out.Problem = err.Error()
	} else if out.Endpoint, err = cfg.Endpoint(llm); err != nil {
		out.Problem = err.Error()
	} else if cfg.Key(llm) == "" {
		out.Problem = "还没有 API Key：在这里填写，或在“智能分类”里配置"
	}
	return out
}

// withAssistantKey applies the form's key over the stored one.
func (s *Server) withAssistantKey(r *http.Request, req assistantRequest) (assistant.Config, error) {
	cfg := req.Config
	cfg.Normalize()
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.Model = strings.TrimSpace(cfg.Model)
	if req.APIKey != nil {
		cfg.APIKey = strings.TrimSpace(*req.APIKey)
		return cfg, nil
	}
	stored, err := s.Settings.Assistant(r.Context())
	if err != nil {
		return cfg, err
	}
	cfg.APIKey = stored.APIKey
	// The key used is the assistant's own or the classifier's; either may
	// go to the endpoint it was saved for, or to Bailian itself.
	llm, err := s.Settings.LLM(r.Context())
	if err != nil {
		return cfg, err
	}
	if cfg.Key(llm) == "" {
		return cfg, nil
	}
	target, err := cfg.Endpoint(llm)
	if err != nil {
		return cfg, nil // Validate reports it; nothing is sent
	}
	host := hostOf(target)
	was, _ := stored.Endpoint(llm)
	derived := cfg
	derived.URL = ""
	from, _ := derived.Endpoint(llm)
	if host != hostOf(was) && host != hostOf(from) && !bailianHost(host) {
		return cfg, errRetypeKey
	}
	return cfg, nil
}

// bailianHost reports Alibaba Cloud Model Studio (Bailian) hosts, where a
// Bailian key may always go.
func bailianHost(host string) bool {
	return strings.HasSuffix(host, ".aliyuncs.com") || strings.HasSuffix(host, ".alibabacloud.com")
}

func (s *Server) getAssistant(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.Settings.Assistant(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.assistantSettings(r, cfg))
}

func (s *Server) putAssistant(w http.ResponseWriter, r *http.Request) {
	var req assistantRequest
	if !decode(w, r, &req) {
		return
	}
	stored, err := s.Settings.Assistant(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	cfg, err := s.withAssistantKey(r, req)
	if err != nil {
		s.keyError(w, err)
		return
	}
	// Preserve saved voicemail items and their file metadata while applying renames from the form.
	if len(stored.Voicemail.Items) > 0 {
		nameByID := map[string]string{}
		for _, it := range req.Voicemail.Items {
			if n := strings.TrimSpace(it.Name); n != "" {
				nameByID[it.ID] = n
			}
		}
		items := make([]assistant.VoicemailItem, len(stored.Voicemail.Items))
		copy(items, stored.Voicemail.Items)
		for i := range items {
			if n, ok := nameByID[items[i].ID]; ok {
				items[i].Name = n
			}
		}
		cfg.Voicemail.Items = items
	} else {
		cfg.Voicemail.Items = []assistant.VoicemailItem{}
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	var key *string
	if req.APIKey != nil {
		key = &cfg.APIKey
	}
	if err := s.Settings.SetAssistant(r.Context(), cfg, key); err != nil {
		s.serverError(w, err)
		return
	}
	switch {
	case cfg.Voicemail.Enabled:
		s.Devices.Note(r.Context(), "info", "语音留言已开启（模型 "+cfg.Model+"）")
	case cfg.Enabled:
		s.Devices.Note(r.Context(), "info", "AI 接听助手已开启（模型 "+cfg.Model+"）")
	default:
		s.Devices.Note(r.Context(), "info", "AI 接听与语音留言设置已保存（均处于关闭状态）")
	}
	writeJSON(w, http.StatusOK, s.assistantSettings(r, cfg))
}

func (s *Server) testAssistant(w http.ResponseWriter, r *http.Request) {
	var req assistantRequest
	if !decode(w, r, &req) {
		return
	}
	cfg, err := s.withAssistantKey(r, req)
	if err != nil {
		s.keyError(w, err)
		return
	}
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if s.Assistant == nil {
		writeError(w, http.StatusServiceUnavailable, "AI 接听助手不可用")
		return
	}
	latency, format, err := s.Assistant.Test(r.Context(), cfg)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"latencyMs": latency.Milliseconds(), "format": format})
}

type generateVoicemailRequest struct {
	assistantRequest
	Name      string `json:"name"`
	ReplaceID string `json:"replaceId"`
}

func (s *Server) generateVoicemail(w http.ResponseWriter, r *http.Request) {
	var req generateVoicemailRequest
	if !decode(w, r, &req) {
		return
	}
	if s.Assistant == nil {
		writeError(w, http.StatusServiceUnavailable, "语音合成服务不可用")
		return
	}
	stored, err := s.Settings.Assistant(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	cfg, err := s.withAssistantKey(r, req.assistantRequest)
	if err != nil {
		s.keyError(w, err)
		return
	}
	text := strings.TrimSpace(cfg.Voicemail.Text)
	if text == "" {
		writeError(w, http.StatusUnprocessableEntity, "请输入要播放的语音留言文本内容")
		return
	}
	if len([]rune(text)) > 500 {
		writeError(w, http.StatusUnprocessableEntity, "语音留言文本不能超过 500 字")
		return
	}

	replaceIdx := -1
	if req.ReplaceID != "" {
		for i, it := range stored.Voicemail.Items {
			if it.ID == req.ReplaceID {
				replaceIdx = i
				break
			}
		}
	}
	if replaceIdx < 0 && len(stored.Voicemail.Items) >= assistant.MaxVoicemailItems {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("最多只能保存 %d 段语音留言，请先删除或替换已有语音", assistant.MaxVoicemailItems))
		return
	}

	// Build synthesis config using the selected model/endpoint/key and the voicemail section's voice/rate/style.
	synthCfg := cfg
	synthCfg.Enabled = false
	synthCfg.Voicemail.Enabled = false
	synthCfg.Voice = cmp.Or(cfg.Voicemail.Voice, cfg.Voice)
	synthCfg.SpeechRate = cmp.Or(cfg.Voicemail.SpeechRate, cfg.SpeechRate)
	synthCfg.Style = cmp.Or(cfg.Voicemail.Style, cfg.Style)
	synthCfg.Normalize()

	wav, dur, err := s.Assistant.SynthesizeVoicemail(r.Context(), synthCfg, text)
	if err != nil {
		writeError(w, http.StatusBadGateway, "AI 生成语音失败："+err.Error())
		return
	}

	if err := os.MkdirAll(s.VoicemailDir, 0o700); err != nil {
		s.serverError(w, fmt.Errorf("创建语音留言目录失败：%w", err))
		return
	}

	now := time.Now()
	var suffix [2]byte
	_, _ = rand.Read(suffix[:])
	id := fmt.Sprintf("vm-%d-%s", now.UnixMilli(), hex.EncodeToString(suffix[:]))
	fileName := id + ".wav"
	filePath := filepath.Join(s.VoicemailDir, fileName)
	if err := os.WriteFile(filePath, wav, 0o600); err != nil {
		s.serverError(w, fmt.Errorf("保存语音留言音频失败：%w", err))
		return
	}

	// Re-read latest settings after long synthesis to avoid overwriting concurrent changes.
	latest, err := s.Settings.Assistant(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	replaceIdx = -1
	if req.ReplaceID != "" {
		for i, it := range latest.Voicemail.Items {
			if it.ID == req.ReplaceID {
				replaceIdx = i
				break
			}
		}
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		if replaceIdx >= 0 {
			name = latest.Voicemail.Items[replaceIdx].Name
		} else {
			name = fmt.Sprintf("语音留言 %d", len(latest.Voicemail.Items)+1)
		}
	}
	item := assistant.VoicemailItem{
		ID:          id,
		Name:        name,
		Text:        text,
		FileName:    fileName,
		Voice:       synthCfg.Voice,
		SpeechRate:  synthCfg.SpeechRate,
		Style:       synthCfg.Style,
		DurationSec: dur,
		CreatedAt:   now,
	}

	if replaceIdx >= 0 {
		old := latest.Voicemail.Items[replaceIdx]
		if old.FileName != "" && old.FileName != fileName {
			_ = os.Remove(filepath.Join(s.VoicemailDir, filepath.Base(old.FileName)))
		}
		latest.Voicemail.Items[replaceIdx] = item
	} else {
		latest.Voicemail.Items = append(latest.Voicemail.Items, item)
	}
	latest.Voicemail.ActiveID = item.ID
	latest.Voicemail.Text = text
	latest.Voicemail.Voice = synthCfg.Voice
	latest.Voicemail.SpeechRate = synthCfg.SpeechRate
	latest.Voicemail.Style = synthCfg.Style
	latest.Voicemail.RingDelaySec = cfg.Voicemail.RingDelaySec
	latest.Normalize()

	if err := s.Settings.SetAssistant(r.Context(), latest, nil); err != nil {
		s.serverError(w, err)
		return
	}
	s.Devices.Note(r.Context(), "info", fmt.Sprintf("已生成语音留言「%s」（%.1f 秒，保存至 data/voicemail/%s）", item.Name, dur, fileName))
	writeJSON(w, http.StatusOK, s.assistantSettings(r, latest))
}

func (s *Server) patchVoicemailItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Name   string `json:"name"`
		Active bool   `json:"active"`
	}
	if !decode(w, r, &req) {
		return
	}
	stored, err := s.Settings.Assistant(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	found := false
	for i := range stored.Voicemail.Items {
		if stored.Voicemail.Items[i].ID == id {
			found = true
			if n := strings.TrimSpace(req.Name); n != "" {
				stored.Voicemail.Items[i].Name = n
			}
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "未找到该段语音留言")
		return
	}
	if req.Active {
		stored.Voicemail.ActiveID = id
	}
	stored.Normalize()
	if err := s.Settings.SetAssistant(r.Context(), stored, nil); err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.assistantSettings(r, stored))
}

func (s *Server) deleteVoicemailItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	stored, err := s.Settings.Assistant(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	idx := -1
	for i, it := range stored.Voicemail.Items {
		if it.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		writeError(w, http.StatusNotFound, "未找到该段语音留言")
		return
	}
	removed := stored.Voicemail.Items[idx]
	stored.Voicemail.Items = append(stored.Voicemail.Items[:idx], stored.Voicemail.Items[idx+1:]...)
	if removed.FileName != "" {
		_ = os.Remove(filepath.Join(s.VoicemailDir, filepath.Base(removed.FileName)))
	}
	stored.Normalize()
	if err := s.Settings.SetAssistant(r.Context(), stored, nil); err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.assistantSettings(r, stored))
}

func (s *Server) voicemailAudio(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	stored, err := s.Settings.Assistant(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	var fileName string
	for _, it := range stored.Voicemail.Items {
		if it.ID == id {
			fileName = it.FileName
			break
		}
	}
	if fileName == "" {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.VoicemailDir, filepath.Base(fileName))
	w.Header().Set("Content-Type", "audio/wav")
	http.ServeFile(w, r, path)
}

// rawJSON passes a stored JSON note through, or null.
func rawJSON(text string) json.RawMessage {
	if text == "" || !json.Valid([]byte(text)) {
		return nil
	}
	return json.RawMessage(text)
}

func (s *Server) routeAssistant(authed func(string, http.HandlerFunc)) {
	authed("GET /api/settings/assistant", s.getAssistant)
	authed("PUT /api/settings/assistant", s.putAssistant)
	authed("POST /api/settings/assistant/test", s.testAssistant)
	authed("POST /api/settings/assistant/voicemail/generate", s.generateVoicemail)
	authed("PATCH /api/settings/assistant/voicemail/{id}", s.patchVoicemailItem)
	authed("DELETE /api/settings/assistant/voicemail/{id}", s.deleteVoicemailItem)
	authed("GET /api/settings/assistant/voicemail/{id}/audio", s.voicemailAudio)
	authed("POST /api/devices/{id}/call/assistant", s.callAction((*service.Service).AnswerWithAssistant))
	authed("POST /api/devices/{id}/call/assistant/skip", s.callAction((*service.Service).SkipAssistant))
	authed("POST /api/devices/{id}/call/takeover", s.callAction((*service.Service).TakeOver))
}
