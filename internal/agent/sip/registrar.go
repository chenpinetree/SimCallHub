package sip

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Extension represents a registered SIP client.
type Extension struct {
	Username  string
	Contact   string
	Addr      *net.UDPAddr
	ExpiresAt time.Time
	CallID    string
}

// Registrar manages extension credentials and live SIP registrations.
type Registrar struct {
	mu         sync.RWMutex
	realm      string
	extensions map[string]string // username -> password
	active     map[string]*Extension
	nonces     map[string]time.Time
}

// NewRegistrar creates a registrar with given realm and credentials.
func NewRegistrar(realm, defaultUser, defaultPass string) *Registrar {
	if realm == "" {
		realm = "SimCallHub"
	}
	r := &Registrar{
		realm:      realm,
		extensions: make(map[string]string),
		active:     make(map[string]*Extension),
		nonces:     make(map[string]time.Time),
	}
	if defaultUser != "" && defaultPass != "" {
		r.extensions[defaultUser] = defaultPass
	}
	return r
}

// AddAccount adds or updates an extension account.
func (r *Registrar) AddAccount(username, password string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.extensions[username] = password
}

// GetAccountPassword gets the password for an extension.
func (r *Registrar) GetAccountPassword(username string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.extensions[username]
	return p, ok
}

// Realm returns the authentication realm.
func (r *Registrar) Realm() string {
	return r.realm
}

// GenerateNonce creates a new challenge nonce.
func (r *Registrar) GenerateNonce() string {
	b := make([]byte, 16)
	rand.Read(b)
	nonce := hex.EncodeToString(b)
	r.mu.Lock()
	r.nonces[nonce] = time.Now().Add(5 * time.Minute)
	r.mu.Unlock()
	return nonce
}

// CheckNonce verifies that a nonce is valid and fresh.
func (r *Registrar) CheckNonce(nonce string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	exp, ok := r.nonces[nonce]
	if !ok || time.Now().After(exp) {
		delete(r.nonces, nonce)
		return false
	}
	return true
}

// Authenticate verifies the Digest Authorization header.
func (r *Registrar) Authenticate(msg *SIPMessage) (string, bool) {
	authHeader := msg.GetHeader("Authorization")
	if authHeader == "" {
		authHeader = msg.GetHeader("Proxy-Authorization")
	}
	if authHeader == "" {
		return "", false
	}
	auth := ParseDigestAuth(authHeader)
	if auth == nil {
		return "", false
	}
	expectedPass, ok := r.GetAccountPassword(auth.Username)
	if !ok {
		return "", false
	}

	expectedResp := CalculateDigestResponse(auth.Username, auth.Realm, expectedPass, msg.Method, auth.URI, auth.Nonce, auth.CNonce, auth.NC, auth.QOP)
	if expectedResp != auth.Response {
		return "", false
	}
	return auth.Username, true
}

// Register adds or updates a registered extension.
func (r *Registrar) Register(username, contact string, addr *net.UDPAddr, expiresSec int, callID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if expiresSec <= 0 {
		delete(r.active, username)
		return
	}

	r.active[username] = &Extension{
		Username:  username,
		Contact:   contact,
		Addr:      addr,
		ExpiresAt: time.Now().Add(time.Duration(expiresSec) * time.Second),
		CallID:    callID,
	}
}

// GetActiveExtensions returns all currently online and valid extensions.
func (r *Registrar) GetActiveExtensions() []*Extension {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	var list []*Extension
	for u, ext := range r.active {
		if now.After(ext.ExpiresAt) {
			delete(r.active, u)
			continue
		}
		list = append(list, ext)
	}
	return list
}

// ParseExpires extracts expires seconds from Expires header or Contact header parameter.
func ParseExpires(msg *SIPMessage) int {
	if expStr := msg.GetHeader("Expires"); expStr != "" {
		if val, err := strconv.Atoi(strings.TrimSpace(expStr)); err == nil {
			return val
		}
	}
	contact := msg.GetHeader("Contact")
	if idx := strings.Index(contact, "expires="); idx != -1 {
		param := contact[idx+8:]
		if semi := strings.Index(param, ";"); semi != -1 {
			param = param[:semi]
		}
		if val, err := strconv.Atoi(strings.Trim(strings.TrimSpace(param), "\"")); err == nil {
			return val
		}
	}
	return 3600 // default 1 hour
}

// ExtractContactAddr extracts IP and port from Contact header (e.g. <sip:1001@192.168.1.50:5060>).
func ExtractContactAddr(contact string) (*net.UDPAddr, error) {
	contact = strings.TrimPrefix(contact, "<")
	if idx := strings.Index(contact, ">"); idx != -1 {
		contact = contact[:idx]
	}
	if idx := strings.Index(contact, ";"); idx != -1 {
		contact = contact[:idx]
	}
	if strings.HasPrefix(contact, "sip:") {
		contact = contact[4:]
	}
	if idx := strings.Index(contact, "@"); idx != -1 {
		contact = contact[idx+1:]
	}
	host, portStr, err := net.SplitHostPort(contact)
	if err != nil {
		// Default port 5060
		host = contact
		portStr = "5060"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		port = 5060
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP in contact: %s", host)
	}
	return &net.UDPAddr{IP: ip, Port: port}, nil
}
