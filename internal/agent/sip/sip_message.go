package sip

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// SIPMessage represents a SIP 2.0 Request or Response.
type SIPMessage struct {
	IsResponse bool

	// Request line
	Method string // INVITE, REGISTER, ACK, BYE, CANCEL, OPTIONS
	URI    string // sip:user@host:port

	// Status line (if IsResponse)
	StatusCode   int    // e.g. 200, 401, 180
	ReasonPhrase string // e.g. "OK", "Unauthorized", "Ringing"

	Headers map[string]string
	Body    []byte
}

// NormalizeHeaderName maps compact or mixed-case headers to canonical names.
func NormalizeHeaderName(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch lower {
	case "v", "via":
		return "Via"
	case "f", "from":
		return "From"
	case "t", "to":
		return "To"
	case "i", "call-id":
		return "Call-ID"
	case "cseq":
		return "CSeq"
	case "m", "contact":
		return "Contact"
	case "c", "content-type":
		return "Content-Type"
	case "l", "content-length":
		return "Content-Length"
	case "expires":
		return "Expires"
	case "authorization":
		return "Authorization"
	case "www-authenticate":
		return "WWW-Authenticate"
	case "user-agent":
		return "User-Agent"
	case "allow":
		return "Allow"
	case "supported":
		return "Supported"
	default:
		// Title-case by default
		return strings.Title(lower)
	}
}

// GetHeader retrieves a header value case-insensitively.
func (m *SIPMessage) GetHeader(name string) string {
	canon := NormalizeHeaderName(name)
	for k, v := range m.Headers {
		if NormalizeHeaderName(k) == canon {
			return v
		}
	}
	return ""
}

// SetHeader sets a canonical header value.
func (m *SIPMessage) SetHeader(name, value string) {
	if m.Headers == nil {
		m.Headers = make(map[string]string)
	}
	canon := NormalizeHeaderName(name)
	// Remove any existing variant
	for k := range m.Headers {
		if NormalizeHeaderName(k) == canon {
			delete(m.Headers, k)
		}
	}
	m.Headers[canon] = value
}

// ParseSIPMessage parses raw bytes of a SIP message.
func ParseSIPMessage(data []byte) (*SIPMessage, error) {
	msg := &SIPMessage{
		Headers: make(map[string]string),
	}

	headerEnd := bytes.Index(data, []byte("\r\n\r\n"))
	var headerPart []byte
	if headerEnd != -1 {
		headerPart = data[:headerEnd]
		msg.Body = data[headerEnd+4:]
	} else {
		// Try single \n\n
		headerEnd = bytes.Index(data, []byte("\n\n"))
		if headerEnd == -1 {
			return nil, fmt.Errorf("malformed sip packet: no header-body separator")
		}
		headerPart = data[:headerEnd]
		msg.Body = data[headerEnd+2:]
	}

	lines := strings.Split(string(headerPart), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty sip packet")
	}

	startLine := strings.TrimRight(lines[0], "\r")
	parts := strings.SplitN(startLine, " ", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid sip start line: %s", startLine)
	}

	if strings.HasPrefix(parts[0], "SIP/") {
		// Response: SIP/2.0 200 OK
		msg.IsResponse = true
		code, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid status code: %s", parts[1])
		}
		msg.StatusCode = code
		msg.ReasonPhrase = parts[2]
	} else {
		// Request: INVITE sip:1001@host SIP/2.0
		msg.IsResponse = false
		msg.Method = strings.ToUpper(parts[0])
		msg.URI = parts[1]
	}

	// Parse headers with line unfolding support
	var currentKey, currentValue string
	for _, rawLine := range lines[1:] {
		line := strings.TrimRight(rawLine, "\r")
		if line == "" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			// Folded header line
			currentValue += " " + strings.TrimSpace(line)
			if currentKey != "" {
				msg.SetHeader(currentKey, currentValue)
			}
			continue
		}
		colon := strings.Index(line, ":")
		if colon == -1 {
			continue
		}
		currentKey = strings.TrimSpace(line[:colon])
		currentValue = strings.TrimSpace(line[colon+1:])
		msg.SetHeader(currentKey, currentValue)
	}

	return msg, nil
}

// String serializes the message into SIP wire format.
func (m *SIPMessage) String() string {
	var sb strings.Builder
	if m.IsResponse {
		sb.WriteString(fmt.Sprintf("SIP/2.0 %d %s\r\n", m.StatusCode, m.ReasonPhrase))
	} else {
		sb.WriteString(fmt.Sprintf("%s %s SIP/2.0\r\n", m.Method, m.URI))
	}

	// Make sure Content-Length is set correctly
	if len(m.Body) > 0 {
		m.SetHeader("Content-Length", strconv.Itoa(len(m.Body)))
	} else {
		m.SetHeader("Content-Length", "0")
	}

	// Important headers order convention
	order := []string{"Via", "From", "To", "Call-ID", "CSeq", "Contact", "User-Agent", "Allow", "Supported", "Content-Type", "Content-Length"}
	written := make(map[string]bool)

	for _, k := range order {
		if v, ok := m.Headers[k]; ok {
			sb.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
			written[k] = true
		}
	}
	for k, v := range m.Headers {
		if !written[k] {
			sb.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
		}
	}
	sb.WriteString("\r\n")
	if len(m.Body) > 0 {
		sb.Write(m.Body)
	}
	return sb.String()
}

// DigestAuth helper to parse Authorization: Digest ...
type DigestAuth struct {
	Username string
	Realm    string
	Nonce    string
	URI      string
	Response string
	CNonce   string
	NC       string
	QOP      string
}

// ParseDigestAuth parses the Authorization or Proxy-Authorization header.
func ParseDigestAuth(val string) *DigestAuth {
	if !strings.HasPrefix(strings.ToLower(val), "digest ") {
		return nil
	}
	auth := &DigestAuth{}
	pairs := strings.Split(val[7:], ",")
	for _, p := range pairs {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(kv[0]))
		v := strings.Trim(strings.TrimSpace(kv[1]), "\"")
		switch k {
		case "username":
			auth.Username = v
		case "realm":
			auth.Realm = v
		case "nonce":
			auth.Nonce = v
		case "uri":
			auth.URI = v
		case "response":
			auth.Response = v
		case "cnonce":
			auth.CNonce = v
		case "nc":
			auth.NC = v
		case "qop":
			auth.QOP = v
		}
	}
	return auth
}

// CalculateDigestResponse computes RFC 2617 digest response.
func CalculateDigestResponse(username, realm, password, method, uri, nonce, cnonce, nc, qop string) string {
	ha1 := md5Hex(fmt.Sprintf("%s:%s:%s", username, realm, password))
	ha2 := md5Hex(fmt.Sprintf("%s:%s", method, uri))

	var raw string
	if qop == "auth" || qop == "auth-int" {
		raw = fmt.Sprintf("%s:%s:%s:%s:%s:%s", ha1, nonce, nc, cnonce, qop, ha2)
	} else {
		raw = fmt.Sprintf("%s:%s:%s", ha1, nonce, ha2)
	}
	return md5Hex(raw)
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// ExtractURIUsername parses sip:1001@domain -> 1001
func ExtractURIUsername(uri string) string {
	uri = strings.TrimPrefix(uri, "<")
	uri = strings.TrimSuffix(uri, ">")
	if idx := strings.Index(uri, ";"); idx != -1 {
		uri = uri[:idx]
	}
	if strings.HasPrefix(uri, "sip:") {
		uri = uri[4:]
	}
	if idx := strings.Index(uri, "@"); idx != -1 {
		return uri[:idx]
	}
	return uri
}
