package httpmw

import (
	"net"
	"net/http"
)

// TrustedSubnetMiddleware returns a middleware that blocks requests whose
// X-Real-IP header does not belong to subnet.
// If subnet is nil the middleware is a no-op pass-through.
// Returns 403 Forbidden when the IP is missing or outside the subnet.
func TrustedSubnetMiddleware(subnet *net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if subnet == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := net.ParseIP(r.Header.Get("X-Real-IP"))
			if ip == nil || !subnet.Contains(ip) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
