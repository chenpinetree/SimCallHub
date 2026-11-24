package cloudrelay

import (
	"context"
	"net/http"
)

type contextKey struct{}

// RelayInfo describes a request that arrived over the authenticated yamux cloud tunnel.
type RelayInfo struct {
	DeviceID string
	UserID   string
}

// WithRelayContext wraps next to tag requests coming from the local yamux
// tunnel with trusted cloud relay identity. Requests on the normal TCP listener
// never receive this context value, preventing header spoofing.
func WithRelayContext(next http.Handler, deviceID string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := RelayInfo{
			DeviceID: deviceID,
			UserID:   r.Header.Get("X-SimCallCloud-User"),
		}
		ctx := context.WithValue(r.Context(), contextKey{}, info)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FromRequest returns the cloud relay identity when r entered through the
// authenticated yamux reverse tunnel.
func FromRequest(r *http.Request) (RelayInfo, bool) {
	info, ok := r.Context().Value(contextKey{}).(RelayInfo)
	return info, ok && info.DeviceID != ""
}
