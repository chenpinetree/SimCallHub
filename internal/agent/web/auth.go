package web

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/chenpinetree/SimCallHub/internal/agent/store"
)

// DefaultAdminPassword is the initial password for fresh installs.
const DefaultAdminPassword = "admin"

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	u, ok, err := s.authenticated(w, r)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated":          true,
		"username":               u.Username,
		"role":                   u.Role,
		"permissions":            u.EffectivePermissions(),
		"deviceScope":            u.DeviceScope,
		"allowedDevices":         u.AllowedDevices,
		"defaultPasswordWarning": isDefaultAdminPassword(u),
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if wait := s.limiter.blockedFor(ip); wait > 0 {
		writeError(w, http.StatusTooManyRequests, "尝试次数过多，请 "+strconv.Itoa(int(wait.Minutes())+1)+" 分钟后再试")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	u, matched := s.authenticatePassword(r, strings.TrimSpace(req.Username), req.Password)
	if !matched {
		s.limiter.fail(ip)
		s.Log.Warn("web login failed", "remote", ip, "username", strings.TrimSpace(req.Username))
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	if u.Disabled {
		s.Log.Warn("disabled user login attempt", "remote", ip, "username", u.Username)
		writeError(w, http.StatusForbidden, "该账号已被管理员停用")
		return
	}
	s.limiter.reset(ip)
	token, err := s.startSession(w, r, u.Username)
	if err != nil {
		s.serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                     true,
		"token":                  token,
		"username":               u.Username,
		"role":                   u.Role,
		"permissions":            u.EffectivePermissions(),
		"deviceScope":            u.DeviceScope,
		"allowedDevices":         u.AllowedDevices,
		"defaultPasswordWarning": isDefaultAdminPassword(u),
	})
}

// authenticatePassword checks credentials. When username is omitted (legacy
// clients/scripts), it checks against the sole admin account.
func (s *Server) authenticatePassword(r *http.Request, username, password string) (store.User, bool) {
	var (
		u   store.User
		err error
	)
	if username == "" {
		u, err = s.Store.GetAdmin(r.Context())
	} else {
		u, err = s.Store.GetUser(r.Context(), username)
	}
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.Log.Error("read user for login", "error", err)
		}
		return store.User{}, false
	}
	if u.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return store.User{}, false
	}
	return u, true
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, username string) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	if err := s.Store.CreateSession(r.Context(), hashToken(token), username, time.Now().Add(sessionTTL)); err != nil {
		return "", err
	}
	s.setSessionCookie(w, token)
	return token, nil
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionTTL / time.Second),
		HttpOnly: true,
		Secure:   s.SecureCookie,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if token := cmp.Or(bearerToken(r), sessionToken(r)); token != "" {
		if err := s.Store.DeleteSession(r.Context(), hashToken(token)); err != nil {
			s.serverError(w, err)
			return
		}
	}
	s.clearCookie(w, sessionCookie)
	ok(w)
}

func (s *Server) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.SecureCookie, SameSite: http.SameSiteStrictMode})
}

// putPassword changes the admin password (legacy endpoint kept for API compatibility).
func (s *Server) putPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !decode(w, r, &req) {
		return
	}
	ip := clientIP(r)
	if wait := s.limiter.blockedFor(ip); wait > 0 {
		writeError(w, http.StatusTooManyRequests, "尝试次数过多，请稍后再试")
		return
	}
	actor := currentUser(r)
	if _, matched := s.authenticatePassword(r, actor.Username, req.Current); !matched {
		s.limiter.fail(ip)
		writeError(w, http.StatusUnprocessableEntity, "当前密码不正确")
		return
	}
	if err := SetUserPassword(r.Context(), s.Store, actor.Username, req.New); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	s.Devices.Note(r.Context(), "info", "后台登录密码已修改，其他登录会话已退出")
	if _, err := s.startSession(w, r, actor.Username); err != nil {
		s.serverError(w, err)
		return
	}
	ok(w)
}

// HashPassword validates length and returns a bcrypt hash.
func HashPassword(password string) (string, error) {
	if len(password) < 4 {
		return "", errors.New("密码至少 4 位")
	}
	if len(password) > 72 {
		return "", errors.New("密码最多 72 个字符")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// SetUserPassword hashes and stores the password for a specific account.
func SetUserPassword(ctx context.Context, st *store.Store, username, password string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return st.SetPasswordHash(ctx, username, hash)
}

// SetPassword hashes and stores the admin password (targeting the current admin
// username, or default "admin" if no users exist yet).
func SetPassword(ctx context.Context, st *store.Store, password string) error {
	target := Username
	if admin, err := st.GetAdmin(ctx); err == nil && admin.Username != "" {
		target = admin.Username
	}
	return SetUserPassword(ctx, st, target, password)
}

// RandomPassword returns a password suitable for the first start.
func RandomPassword() (string, error) {
	const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b), nil
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashToken means a leaked database does not expose live session cookies.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// loginLimiter locks an address out for a while after repeated failures.
type loginLimiter struct {
	mu       sync.Mutex
	failures map[string]*loginFailures
}

type loginFailures struct {
	count int
	last  time.Time
	until time.Time
}

const (
	maxLoginFailures = 5
	loginLockout     = 5 * time.Minute
)

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{failures: make(map[string]*loginFailures)}
}

func (l *loginLimiter) blockedFor(ip string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if f, ok := l.failures[ip]; ok {
		return time.Until(f.until)
	}
	return 0
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	// Forget addresses that are not locked and have been quiet for a while.
	for key, f := range l.failures {
		if now.After(f.until) && now.Sub(f.last) > loginLockout {
			delete(l.failures, key)
		}
	}
	f, ok := l.failures[ip]
	if !ok || (f.count >= maxLoginFailures && now.After(f.until)) {
		f = &loginFailures{}
		l.failures[ip] = f
	}
	f.count++
	f.last = now
	if f.count >= maxLoginFailures {
		f.until = now.Add(loginLockout)
	}
}

func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, ip)
}
