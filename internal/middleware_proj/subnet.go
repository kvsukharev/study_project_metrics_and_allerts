package middlewareproj

import (
	"net"
	"net/http"
)

// TrustedSubnetMiddleware returns a middleware that blocks requests whose
// X-Real-IP header does not belong to the given CIDR subnet.
// If cidr is empty the middleware is a no-op pass-through.
// Returns 403 Forbidden when the IP is missing or outside the subnet.
func TrustedSubnetMiddleware(cidr string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if cidr == "" {
			return next
		}
		_, subnet, err := net.ParseCIDR(cidr)
		if err != nil {
			// Invalid CIDR — block all requests to fail safely.
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "forbidden", http.StatusForbidden)
			})
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ipStr := r.Header.Get("X-Real-IP")
			if ipStr == "" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			ip := net.ParseIP(ipStr)
			if ip == nil || !subnet.Contains(ip) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
