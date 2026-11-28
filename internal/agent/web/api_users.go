package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"

	"github.com/chenpinetree/SimCallHub/internal/agent/store"
)

type userJSON struct {
	Username       string                `json:"username"`
	Role           string                `json:"role"`
	Disabled       bool                  `json:"disabled"`
	Permissions    store.UserPermissions `json:"permissions"`
	DeviceScope    string                `json:"deviceScope"`
	AllowedDevices []string              `json:"allowedDevices"`
	Note           string                `json:"note"`
	CreatedAt      *time.Time            `json:"createdAt,omitempty"`
	UpdatedAt      *time.Time            `json:"updatedAt,omitempty"`
}

func toUserJSON(u store.User) userJSON {
	devs := u.AllowedDevices
	if devs == nil {
		devs = []string{}
	}
	return userJSON{
		Username:       u.Username,
		Role:           u.Role,
		Disabled:       u.Disabled,
		Permissions:    u.EffectivePermissions(),
		DeviceScope:    u.DeviceScope,
		AllowedDevices: devs,
		Note:           u.Note,
		CreatedAt:      timePtr(u.CreatedAt),
		UpdatedAt:      timePtr(u.UpdatedAt),
	}
}

func isDefaultAdminPassword(u store.User) bool {
	if !u.IsAdmin() || u.Username != Username || u.PasswordHash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(DefaultAdminPassword)) == nil
}

// ValidateUsername checks that a username is non-empty, short, and safe in URLs.
func ValidateUsername(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("用户名不能为空")
	}
	runes := []rune(name)
	if len(runes) < 2 || len(runes) > 32 {
		return "", errors.New("用户名长度需在 2 到 32 个字符之间")
	}
	for _, r := range runes {
		if unicode.IsSpace(r) || r == '/' || r == '\\' || r == '?' || r == '#' || r == '%' {
			return "", errors.New("用户名不能包含空格或 / \\ ? # % 符号")
		}
	}
	return name, nil
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListUsers(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	items := make([]userJSON, 0, len(list))
	defaultWarn := false
	for _, u := range list {
		if isDefaultAdminPassword(u) {
			defaultWarn = true
		}
		items = append(items, toUserJSON(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":                  items,
		"defaultPasswordWarning": defaultWarn,
	})
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username       string                 `json:"username"`
		Password       string                 `json:"password"`
		Disabled       bool                   `json:"disabled"`
		Permissions    *store.UserPermissions `json:"permissions"`
		DeviceScope    string                 `json:"deviceScope"`
		AllowedDevices []string               `json:"allowedDevices"`
		Note           string                 `json:"note"`
	}
	if !decode(w, r, &req) {
		return
	}
	username, err := ValidateUsername(req.Username)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	hash, err := HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	perms := store.DefaultSubUserPermissions()
	if req.Permissions != nil {
		perms = *req.Permissions
	}
	scope := req.DeviceScope
	if scope != store.DeviceScopeSelected {
		scope = store.DeviceScopeAll
	}
	u := store.User{
		Username:       username,
		Role:           store.RoleUser,
		Disabled:       req.Disabled,
		Permissions:    perms,
		DeviceScope:    scope,
		AllowedDevices: req.AllowedDevices,
		Note:           strings.TrimSpace(req.Note),
	}
	if err := s.Store.CreateUser(r.Context(), u, hash); err != nil {
		if errors.Is(err, store.ErrUserExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		s.serverError(w, err)
		return
	}
	created, err := s.Store.GetUser(r.Context(), username)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.Devices.Note(r.Context(), "info", "已创建子账号："+username)
	writeJSON(w, http.StatusOK, toUserJSON(created))
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(r.PathValue("username"))
	cur, err := s.Store.GetUser(r.Context(), target)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "账号不存在")
		return
	}
	if err != nil {
		s.serverError(w, err)
		return
	}

	var req struct {
		Username       *string                `json:"username"`
		Password       *string                `json:"password"`
		Disabled       *bool                  `json:"disabled"`
		Permissions    *store.UserPermissions `json:"permissions"`
		DeviceScope    *string                `json:"deviceScope"`
		AllowedDevices *[]string              `json:"allowedDevices"`
		Note           *string                `json:"note"`
	}
	if !decode(w, r, &req) {
		return
	}

	next := cur
	if req.Username != nil {
		validated, err := ValidateUsername(*req.Username)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		next.Username = validated
	}
	if req.Note != nil {
		next.Note = strings.TrimSpace(*req.Note)
	}
	if !cur.IsAdmin() {
		if req.Disabled != nil {
			next.Disabled = *req.Disabled
		}
		if req.Permissions != nil {
			next.Permissions = *req.Permissions
		}
		if req.DeviceScope != nil {
			if *req.DeviceScope == store.DeviceScopeSelected {
				next.DeviceScope = store.DeviceScopeSelected
			} else {
				next.DeviceScope = store.DeviceScopeAll
			}
		}
		if req.AllowedDevices != nil {
			next.AllowedDevices = *req.AllowedDevices
		}
	}

	var newHash *string
	if req.Password != nil && *req.Password != "" {
		h, err := HashPassword(*req.Password)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		newHash = &h
	}

	cleared, err := s.Store.UpdateUser(r.Context(), cur.Username, next, newHash)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "账号不存在")
		case errors.Is(err, store.ErrUserExists):
			writeError(w, http.StatusConflict, err.Error())
		default:
			s.serverError(w, err)
		}
		return
	}

	// If the admin modified their own username or password, issue a fresh session
	// so their browser stays logged in while other sessions are signed out.
	actor := currentUser(r)
	if cleared && actor.Username == cur.Username {
		if _, err := s.startSession(w, r, next.Username); err != nil {
			s.serverError(w, err)
			return
		}
	}

	updated, err := s.Store.GetUser(r.Context(), next.Username)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if cur.IsAdmin() {
		s.Devices.Note(r.Context(), "info", "管理员账号信息已更新")
	} else {
		s.Devices.Note(r.Context(), "info", "子账号已更新："+updated.Username)
	}
	writeJSON(w, http.StatusOK, toUserJSON(updated))
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(r.PathValue("username"))
	err := s.Store.DeleteUser(r.Context(), target)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "账号不存在")
		case errors.Is(err, store.ErrCannotDeleteAdmin):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			s.serverError(w, err)
		}
		return
	}
	s.Devices.Note(r.Context(), "info", "已删除子账号："+target)
	ok(w)
}

// EnsureDefaultAdmin creates the default admin/admin account on a fresh database.
// Existing databases with any account are left untouched.
func EnsureDefaultAdmin(ctx context.Context, st *store.Store) (bool, error) {
	exists, err := st.HasUsers(ctx)
	if err != nil || exists {
		return false, err
	}
	hash, err := HashPassword(DefaultAdminPassword)
	if err != nil {
		return false, err
	}
	if err := st.CreateUser(ctx, store.User{
		Username: Username,
		Role:     store.RoleAdmin,
	}, hash); err != nil {
		return false, err
	}
	return true, nil
}
