package api

import (
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

func (server *Server) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Referrer-Policy", "same-origin")
		writer.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		switch {
		case !server.hostAllowed(request.Host):
			writeError(writer, http.StatusForbidden, "host_not_allowed")
		case !sameOrigin(request):
			writeError(writer, http.StatusForbidden, "cross_origin")
		default:
			next.ServeHTTP(writer, request)
		}
	})
}

// Any DNS name could be pointed at a LAN listener, so names must be allowlisted
// to stop DNS rebinding; literal IPs and localhost are safe to accept.
func (server *Server) hostAllowed(host string) bool {
	if len(server.settings.AllowedHosts) > 0 {
		return slices.ContainsFunc(server.settings.AllowedHosts, func(allowed string) bool { return strings.EqualFold(host, allowed) })
	}

	hostname := host
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		hostname = parsed
	}

	hostname = strings.Trim(hostname, "[]")
	if strings.EqualFold(hostname, "localhost") {
		return true
	}

	address := net.ParseIP(hostname)

	return address != nil && (!loopbackListener(server.settings.Listen) || address.IsLoopback())
}

func loopbackListener(listen string) bool {
	host, _, _ := net.SplitHostPort(listen)
	address := net.ParseIP(host)

	return address != nil && address.IsLoopback()
}

func sameOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)

	return err == nil && parsed.Host == request.Host && (parsed.Scheme == "http" || parsed.Scheme == "https")
}
