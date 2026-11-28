package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"

	DeviceScopeAll      = "all"
	DeviceScopeSelected = "selected"
)

var (
	ErrUserExists        = errors.New("用户名已存在")
	ErrCannotDeleteAdmin = errors.New("管理员账号不能删除")
)

// UserPermissions controls which pages and settings tabs a sub-account can access.
// Admin accounts always have every permission enabled.
type UserPermissions struct {
	Messages           bool `json:"messages"`
	Calls              bool `json:"calls"`
	Contacts           bool `json:"contacts"`
	Device             bool `json:"device"`
	Logs               bool `json:"logs"`
	SettingsNotify     bool `json:"settingsNotify"`
	SettingsAssistant  bool `json:"settingsAssistant"`
	SettingsSIP        bool `json:"settingsSip"`
	SettingsCloud      bool `json:"settingsCloud"`
	SettingsClassifier bool `json:"settingsClassifier"`
	SettingsDevices    bool `json:"settingsDevices"`
	SettingsGeneral    bool `json:"settingsGeneral"`
}

// AllPermissions returns a UserPermissions with every permission enabled.
func AllPermissions() UserPermissions {
	return UserPermissions{
		Messages:           true,
		Calls:              true,
		Contacts:           true,
		Device:             true,
		Logs:               true,
		SettingsNotify:     true,
		SettingsAssistant:  true,
		SettingsSIP:        true,
		SettingsCloud:      true,
		SettingsClassifier: true,
		SettingsDevices:    true,
		SettingsGeneral:    true,
	}
}

// DefaultSubUserPermissions returns the default permission set for a newly created sub-account:
// SMS, Calls, and Contacts enabled; Device, Logs, and all Settings tabs disabled.
func DefaultSubUserPermissions() UserPermissions {
	return UserPermissions{
		Messages: true,
		Calls:    true,
		Contacts: true,
	}
}

// User represents a web/API account (either the single admin or a sub-account).
type User struct {
	Username       string          `json:"username"`
	Role           string          `json:"role"`
	Disabled       bool            `json:"disabled"`
	Permissions    UserPermissions `json:"permissions"`
	DeviceScope    string          `json:"deviceScope"`
	AllowedDevices []string        `json:"allowedDevices"`
	Note           string          `json:"note"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	PasswordHash   string          `json:"-"`
}

func (u User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

func (u User) EffectivePermissions() UserPermissions {
	if u.IsAdmin() {
		return AllPermissions()
	}
	return u.Permissions
}

func (u User) HasPerm(key string) bool {
	if u.IsAdmin() {
		return true
	}
	p := u.Permissions
	switch key {
	case "admin":
		return false
	case "messages":
		return p.Messages
	case "calls":
		return p.Calls
	case "contacts":
		return p.Contacts
	case "device":
		return p.Device
	case "logs":
		return p.Logs
	case "settingsNotify":
		return p.SettingsNotify
	case "settingsAssistant":
		return p.SettingsAssistant
	case "settingsSip":
		return p.SettingsSIP
	case "settingsCloud":
		return p.SettingsCloud
	case "settingsClassifier":
		return p.SettingsClassifier
	case "settingsDevices":
		return p.SettingsDevices
	case "settingsGeneral":
		return p.SettingsGeneral
	default:
		return false
	}
}

func (u User) HasAnySettingsPerm() bool {
	if u.IsAdmin() {
		return true
	}
	p := u.Permissions
	return p.SettingsNotify || p.SettingsAssistant || p.SettingsSIP ||
		p.SettingsCloud || p.SettingsClassifier || p.SettingsDevices || p.SettingsGeneral
}

func (u User) AllowsDevice(deviceID string) bool {
	if u.IsAdmin() || u.DeviceScope != DeviceScopeSelected {
		return true
	}
	if deviceID == "" {
		return false
	}
	return slices.Contains(u.AllowedDevices, deviceID)
}

// AllowedDeviceFilter returns nil when the user may access all devices, or a
// non-nil slice (possibly empty) of permitted device IDs when restricted.
func (u User) AllowedDeviceFilter() []string {
	if u.IsAdmin() || u.DeviceScope != DeviceScopeSelected {
		return nil
	}
	if u.AllowedDevices == nil {
		return []string{}
	}
	return u.AllowedDevices
}

const userColumns = `username, password_hash, role, disabled, permissions, device_scope, allowed_devices, note, created_at, updated_at`

func scanUser(row scanner) (User, error) {
	var u User
	var disabled int
	var permsRaw, devicesRaw string
	var createdAt, updatedAt int64
	if err := row.Scan(&u.Username, &u.PasswordHash, &u.Role, &disabled, &permsRaw, &u.DeviceScope, &devicesRaw, &u.Note, &createdAt, &updatedAt); err != nil {
		return u, err
	}
	if u.Role == "" {
		u.Role = RoleAdmin
	}
	u.Disabled = disabled != 0
	if u.DeviceScope != DeviceScopeSelected {
		u.DeviceScope = DeviceScopeAll
	}
	if u.IsAdmin() {
		u.Permissions = AllPermissions()
		u.DeviceScope = DeviceScopeAll
		u.AllowedDevices = []string{}
	} else {
		u.Permissions = DefaultSubUserPermissions()
		if permsRaw != "" && permsRaw != "{}" {
			var p UserPermissions
			if json.Unmarshal([]byte(permsRaw), &p) == nil {
				u.Permissions = p
			}
		}
		u.AllowedDevices = []string{}
		if devicesRaw != "" {
			var list []string
			if json.Unmarshal([]byte(devicesRaw), &list) == nil && list != nil {
				u.AllowedDevices = list
			}
		}
	}
	if createdAt != 0 {
		u.CreatedAt = time.Unix(createdAt, 0)
	}
	if updatedAt != 0 {
		u.UpdatedAt = time.Unix(updatedAt, 0)
	}
	return u, nil
}

func normalizeAllowedDevices(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, id := range in {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userColumns+` FROM users ORDER BY CASE WHEN role = 'admin' THEN 0 ELSE 1 END, created_at ASC, username ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) GetUser(ctx context.Context, username string) (User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE username = ?`, username))
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (s *Store) GetAdmin(ctx context.Context) (User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE role = 'admin' ORDER BY created_at ASC, username ASC LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// CreateUser inserts a new account (admin bootstrap or sub-account).
func (s *Store) CreateUser(ctx context.Context, u User, passwordHash string) error {
	now := time.Now().Unix()
	if u.Role != RoleAdmin {
		u.Role = RoleUser
	}
	if u.IsAdmin() {
		u.Disabled = false
		u.Permissions = AllPermissions()
		u.DeviceScope = DeviceScopeAll
		u.AllowedDevices = []string{}
	} else {
		if u.DeviceScope != DeviceScopeSelected {
			u.DeviceScope = DeviceScopeAll
		}
		u.AllowedDevices = normalizeAllowedDevices(u.AllowedDevices)
	}
	permsJSON, _ := json.Marshal(u.Permissions)
	devsJSON, _ := json.Marshal(u.AllowedDevices)
	disabledInt := 0
	if u.Disabled {
		disabledInt = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, role, disabled, permissions, device_scope, allowed_devices, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.Username, passwordHash, u.Role, disabledInt, string(permsJSON), u.DeviceScope, string(devsJSON), strings.TrimSpace(u.Note), now, now)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") {
		return ErrUserExists
	}
	return err
}

// UpdateUser updates an existing user (including renaming, password reset,
// permissions, device scope, disabled flag, and note). Existing sessions are
// cleared whenever the username, password, or disabled status changes.
func (s *Store) UpdateUser(ctx context.Context, oldUsername string, next User, newPasswordHash *string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	cur, err := scanUser(tx.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE username = ?`, oldUsername))
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}

	targetName := strings.TrimSpace(next.Username)
	if targetName == "" {
		targetName = cur.Username
	}
	renamed := targetName != cur.Username
	if renamed {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE username = ?`, targetName).Scan(&exists); err != nil {
			return false, err
		}
		if exists > 0 {
			return false, ErrUserExists
		}
	}

	// Preserve role: only one admin exists, sub-accounts remain sub-accounts.
	role := cur.Role
	disabled := next.Disabled
	perms := next.Permissions
	scope := next.DeviceScope
	allowed := normalizeAllowedDevices(next.AllowedDevices)
	if role == RoleAdmin {
		disabled = false
		perms = AllPermissions()
		scope = DeviceScopeAll
		allowed = []string{}
	} else if scope != DeviceScopeSelected {
		scope = DeviceScopeAll
	}

	hash := cur.PasswordHash
	passwordChanged := newPasswordHash != nil && *newPasswordHash != ""
	if passwordChanged {
		hash = *newPasswordHash
	}

	clearSessions := renamed || passwordChanged || disabled
	if clearSessions {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE username = ?`, cur.Username); err != nil {
			return false, err
		}
	}

	permsJSON, _ := json.Marshal(perms)
	devsJSON, _ := json.Marshal(allowed)
	disabledInt := 0
	if disabled {
		disabledInt = 1
	}
	now := time.Now().Unix()

	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET username = ?, password_hash = ?, disabled = ?, permissions = ?, device_scope = ?, allowed_devices = ?, note = ?, updated_at = ?
		WHERE username = ?`,
		targetName, hash, disabledInt, string(permsJSON), scope, string(devsJSON), strings.TrimSpace(next.Note), now, cur.Username); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return false, ErrUserExists
		}
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return clearSessions, nil
}

// DeleteUser removes a sub-account and cascades its sessions. The admin account
// cannot be deleted.
func (s *Store) DeleteUser(ctx context.Context, username string) error {
	u, err := s.GetUser(ctx, username)
	if err != nil {
		return err
	}
	if u.IsAdmin() {
		return ErrCannotDeleteAdmin
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM users WHERE username = ?`, username)
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, username string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE username = ?`, username)
	return err
}
