// Package web serves the admin UI: a JSON API, a live event stream, and the
// single-page app built from ui/ and embedded from dist/.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/chenpinetree/SimCallHub/internal/agent/assistant"
	"github.com/chenpinetree/SimCallHub/internal/agent/classify"
	"github.com/chenpinetree/SimCallHub/internal/agent/cloudrelay"
	"github.com/chenpinetree/SimCallHub/internal/agent/notify"
	"github.com/chenpinetree/SimCallHub/internal/agent/phonebook"
	"github.com/chenpinetree/SimCallHub/internal/agent/service"
	"github.com/chenpinetree/SimCallHub/internal/agent/settings"
	"github.com/chenpinetree/SimCallHub/internal/agent/sip"
	"github.com/chenpinetree/SimCallHub/internal/agent/store"
)

//go:embed all:dist
var dist embed.FS

const (
	sessionCookie = "simcallhub_session"
	// Logins are kept as long as browsers allow: Chrome caps cookie lifetime
	// at 400 days. The session is extended (at most once per
	// sessionRenewEvery) whenever the UI is used, so only 400 days without
	// a visit, logging out or changing the password ends it.
	sessionTTL        = 400 * 24 * time.Hour
	sessionRenewEvery = 24 * time.Hour
	maxBody           = 64 << 10
	// Username is the single web account.
	Username = "admin"
)

// Deps are the parts of the agent the web UI drives.
type Deps struct {
	Store    *store.Store
	Devices  *service.Manager
	Settings *settings.Settings
	Worker   *classify.Worker
	Client   *classify.Client
	// Assistant answers calls; its Test checks the realtime endpoint.
	Assistant *assistant.Agent
	// Notifier pushes to Telegram and group bots; the settings page tests it.
	Notifier *notify.Notifier
	// Phonebook names numbers; the contacts page edits it.
	Phonebook *phonebook.Book
	// RecordingDir holds call recordings.
	RecordingDir string
	// VoicemailDir holds generated WAV voicemail greetings (data/voicemail).
	VoicemailDir string
	// SIP is the embedded SIP VoIP extension PBX server.
	SIP *sip.Server
	// Cloud manages the optional SimCallCloud yamux reverse tunnel.
	Cloud *cloudrelay.Client
	Log   *slog.Logger
	// Version is the SimCallHub build version string.
	Version string
	// SecureCookie marks the session cookie Secure (HTTPS reverse proxy).
	SecureCookie bool
}

type Server struct {
	Deps
	limiter *loginLimiter
	app     fs.FS
}

func New(d Deps) (*Server, error) {
	app, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, err
	}
	if d.Phonebook == nil {
		d.Phonebook = &phonebook.Book{Store: d.Store}
	}
	if d.VoicemailDir == "" {
		if d.RecordingDir != "" {
			d.VoicemailDir = filepath.Join(filepath.Dir(d.RecordingDir), "voicemail")
		} else {
			d.VoicemailDir = filepath.Join("data", "voicemail")
		}
	}
	if d.Assistant != nil && d.Assistant.VoicemailDir == "" {
		d.Assistant.VoicemailDir = d.VoicemailDir
	}
	return &Server{Deps: d, limiter: newLoginLimiter(), app: app}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	api := func(pattern string, h http.HandlerFunc) {
		method, _, _ := strings.Cut(pattern, " ")
		if method != http.MethodGet {
			h = s.sameOrigin(h)
		}
		mux.HandleFunc(pattern, h)
	}
	authed := func(pattern string, h http.HandlerFunc) { api(pattern, s.requireAuth(h)) }
	perm := func(keys ...string) func(string, http.HandlerFunc) {
		return func(pattern string, h http.HandlerFunc) {
			api(pattern, s.requireAuth(s.requirePerm(keys, h)))
		}
	}
	adminOnly := perm("admin")
	msgPerm := perm("messages")
	callPerm := perm("calls")
	contactPerm := perm("contacts")
	devicePerm := perm("device")
	simSettingsPerm := perm("device", "settingsDevices")
	logPerm := perm("logs")
	classifierPerm := perm("settingsClassifier")
	sipPerm := perm("settingsSip")
	cloudPerm := perm("settingsCloud")
	notifyPerm := perm("settingsNotify")
	assistantPerm := perm("settingsAssistant")

	api("GET /api/session", s.session)
	api("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"version": s.Version})
	})
	api("POST /api/login", s.login)
	authed("POST /api/logout", s.logout)
	authed("GET /api/stream", s.stream)

	authed("GET /api/status", s.status)
	authed("GET /api/activity", s.activity)
	authed("GET /api/categories", s.getCategories)
	logPerm("GET /api/events", s.events)

	devicePerm("PATCH /api/devices/{id}", s.patchDevice)
	devicePerm("DELETE /api/devices/{id}", s.forgetDevice)
	devicePerm("POST /api/devices/{id}/adopt", s.adoptOrphans)
	devicePerm("POST /api/devices/{id}/selftest", s.selfTest)
	devicePerm("PUT /api/devices/{id}/recovery", s.setRecovery)
	devicePerm("POST /api/devices/{id}/refresh", s.deviceRefresh)
	devicePerm("POST /api/devices/{id}/reboot", s.deviceReboot)
	devicePerm("POST /api/devices/{id}/radio", s.deviceRadio)
	devicePerm("POST /api/devices/{id}/scan-mode", s.deviceScanMode)
	devicePerm("POST /api/devices/{id}/console", s.deviceConsole)
	devicePerm("POST /api/devices/{id}/sim/pin", s.simPIN)
	devicePerm("POST /api/devices/{id}/sim/puk", s.simPUK)
	devicePerm("POST /api/devices/{id}/sim/lock", s.simLock)
	devicePerm("POST /api/devices/{id}/sim/change-pin", s.simChangePIN)
	simSettingsPerm("GET /api/devices/{id}/sim-settings", s.getSIMSettings)
	simSettingsPerm("PUT /api/devices/{id}/sim-settings", s.putSIMSettings)

	devicePerm("GET /api/devices/{id}/esim", s.deviceESimInfo)
	devicePerm("POST /api/devices/{id}/esim/enable", s.deviceESimEnable)
	devicePerm("POST /api/devices/{id}/esim/disable", s.deviceESimDisable)
	devicePerm("POST /api/devices/{id}/esim/delete", s.deviceESimDelete)
	devicePerm("POST /api/devices/{id}/esim/nickname", s.deviceESimNickname)

	msgPerm("GET /api/conversations", s.listConversations)
	msgPerm("GET /api/conversations/messages", s.conversationMessages)
	msgPerm("POST /api/conversations/read", s.readConversation)
	msgPerm("POST /api/conversations/delete", s.deleteConversation)
	msgPerm("POST /api/messages/send", s.sendMessage)
	msgPerm("POST /api/messages/{id}/resend", s.resendMessage)

	msgPerm("GET /api/messages", s.listMessages)
	msgPerm("GET /api/messages/{id}", s.getMessage)
	msgPerm("GET /api/messages/{id}/redacted", s.redactedMessage)
	msgPerm("GET /api/messages/{id}/classification", s.messageClassification)
	classifierPerm("GET /api/classify/logs", s.classifyLogs)
	msgPerm("PATCH /api/messages/{id}", s.patchMessage)
	msgPerm("DELETE /api/messages/{id}", s.deleteMessage)
	msgPerm("POST /api/messages/bulk", s.bulkMessages)
	msgPerm("POST /api/messages/{id}/mms/download", s.downloadMMS)
	msgPerm("GET /api/messages/{id}/mms/{part}", s.mmsPartData)

	adminOnly("PUT /api/settings/password", s.putPassword)
	adminOnly("GET /api/users", s.listUsers)
	adminOnly("POST /api/users", s.createUser)
	adminOnly("PUT /api/users/{username}", s.updateUser)
	adminOnly("DELETE /api/users/{username}", s.deleteUser)

	classifierPerm("GET /api/settings/classifier", s.getClassifier)
	classifierPerm("PUT /api/settings/classifier", s.putClassifier)
	classifierPerm("POST /api/settings/classifier/test", s.testClassifier)
	classifierPerm("POST /api/settings/classifier/preview", s.previewClassifier)
	classifierPerm("POST /api/settings/classifier/reclassify", s.reclassifyAll)
	sipPerm("GET /api/settings/sip", s.getSIP)
	sipPerm("PUT /api/settings/sip", s.putSIP)
	cloudPerm("GET /api/settings/cloud", s.getCloud)
	cloudPerm("PUT /api/settings/cloud", s.putCloud)
	cloudPerm("POST /api/settings/cloud/pair", s.pairCloud)
	cloudPerm("POST /api/settings/cloud/unbind", s.unbindCloud)
	cloudPerm("POST /api/settings/cloud/test-push", s.testCloudPush)

	s.routeCalls(callPerm)
	s.routeAssistant(assistantPerm, callPerm)
	s.routeNotify(notifyPerm)
	s.routeContacts(contactPerm)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "接口不存在")
	})
	mux.HandleFunc("/", s.serveApp)
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; media-src 'self'; font-src 'self'; connect-src 'self'; manifest-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Permissions-Policy", "microphone=(self), camera=(), geolocation=()")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// serveApp serves the built UI. Unknown paths get index.html so client-side
// routes survive a reload.
func (s *Server) serveApp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name != "" && name != "index.html" {
		if info, err := fs.Stat(s.app, name); err == nil && !info.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				// Vite puts a content hash in every asset name.
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.ServeFileFS(w, r, s.app, name)
			return
		}
	}
	index, err := fs.ReadFile(s.app, "index.html")
	if err != nil {
		http.Error(w, "UI not built", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(index)
}

// sameOrigin rejects cross-site requests (CSRF). Browsers set Sec-Fetch-Site
// themselves; older ones fall back to comparing Origin with Host. Requiring a
// JSON body adds a second layer: a cross-site form cannot send one without a
// CORS preflight, which this server never approves.
func (s *Server) sameOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, viaCloud := cloudrelay.FromRequest(r)
		allowMobile := viaCloud || (r.URL.Path != "/api/login" && bearerToken(r) != "")
		if !allowMobile {
			if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
				if site != "same-origin" && site != "none" {
					writeError(w, http.StatusForbidden, "cross-site request rejected")
					return
				}
			} else if origin := r.Header.Get("Origin"); origin != "" {
				parsed, err := url.Parse(origin)
				if err != nil || parsed.Host != r.Host {
					writeError(w, http.StatusForbidden, "cross-site request rejected")
					return
				}
			}
		}
		if r.ContentLength != 0 && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeError(w, http.StatusUnsupportedMediaType, "需要 JSON 请求体")
			return
		}
		next(w, r)
	}
}

type userContextKey struct{}

func currentUser(r *http.Request) store.User {
	if u, ok := r.Context().Value(userContextKey{}).(store.User); ok {
		return u
	}
	return store.User{}
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok, err := s.authenticated(w, r)
		if err != nil {
			s.serverError(w, err)
			return
		}
		if !ok {
			writeError(w, http.StatusUnauthorized, "请先登录")
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey{}, u)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) requirePerm(keys []string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		for _, k := range keys {
			if u.HasPerm(k) {
				next(w, r)
				return
			}
		}
		writeError(w, http.StatusForbidden, "当前账号没有此功能的访问权限")
	}
}

// authenticated checks the cloud relay context, bearer token, or session cookie
// and returns the active user (extending the session when needed).
func (s *Server) authenticated(w http.ResponseWriter, r *http.Request) (store.User, bool, error) {
	if _, viaCloud := cloudrelay.FromRequest(r); viaCloud {
		admin, err := s.Store.GetAdmin(r.Context())
		if err != nil {
			admin = store.User{Username: Username, Role: store.RoleAdmin, Permissions: store.AllPermissions(), DeviceScope: store.DeviceScopeAll}
		}
		return admin, true, nil
	}
	now := time.Now()
	if token := bearerToken(r); token != "" {
		username, err := s.Store.SessionUser(r.Context(), hashToken(token), now)
		if errors.Is(err, store.ErrNotFound) {
			return store.User{}, false, nil
		}
		if err != nil {
			return store.User{}, false, err
		}
		u, err := s.Store.GetUser(r.Context(), username)
		if errors.Is(err, store.ErrNotFound) || (err == nil && u.Disabled) {
			return store.User{}, false, nil
		}
		if err != nil {
			return store.User{}, false, err
		}
		_, err = s.Store.ExtendSession(r.Context(), hashToken(token), now, sessionTTL, sessionRenewEvery)
		return u, true, err
	}
	token := sessionToken(r)
	if token == "" {
		return store.User{}, false, nil
	}
	username, err := s.Store.SessionUser(r.Context(), hashToken(token), now)
	if errors.Is(err, store.ErrNotFound) {
		return store.User{}, false, nil
	}
	if err != nil {
		return store.User{}, false, err
	}
	u, err := s.Store.GetUser(r.Context(), username)
	if errors.Is(err, store.ErrNotFound) || (err == nil && u.Disabled) {
		return store.User{}, false, nil
	}
	if err != nil {
		return store.User{}, false, err
	}
	extended, err := s.Store.ExtendSession(r.Context(), hashToken(token), now, sessionTTL, sessionRenewEvery)
	if err != nil {
		return store.User{}, false, err
	}
	if extended {
		s.setSessionCookie(w, token)
	}
	return u, true, nil
}

// bearerToken extracts an API Bearer token from Authorization header (or ?token= for GET/WebSocket).
func bearerToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
		if tok := strings.TrimSpace(auth[7:]); tok != "" {
			return tok
		}
	}
	if r.Method == http.MethodGet {
		if tok := strings.TrimSpace(r.URL.Query().Get("token")); tok != "" {
			return tok
		}
	}
	return ""
}

// sessionToken returns the session cookie's value.
func sessionToken(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
}

type apiError struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func (s *Server) serverError(w http.ResponseWriter, err error) {
	s.Log.Error("web request failed", "error", err)
	writeError(w, http.StatusInternalServerError, "服务器内部错误")
}

// opError reports a failed device operation with a status that tells the UI
// what to do next.
func (s *Server) opError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrOffline), errors.Is(err, service.ErrNotReady):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, service.ErrLastAttempt):
		writeJSON(w, http.StatusConflict, apiError{Error: err.Error(), Code: "last_attempt"})
	default:
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	return decodeLimit(w, r, v, maxBody)
}

// decodeLimit is decode for the few requests that carry files, such as an
// address book to import.
func decodeLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	body := http.MaxBytesReader(w, r.Body, limit)
	if err := json.NewDecoder(body).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return false
	}
	return true
}

func ok(w http.ResponseWriter) { writeJSON(w, http.StatusOK, map[string]bool{"ok": true}) }
