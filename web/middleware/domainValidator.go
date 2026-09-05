// Package middleware provides HTTP middleware functions for the 3x-ui web panel,
// including domain validation and URL redirection utilities.
package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// agentPaths are the gost agent endpoints. Agents connect from relay servers
// by raw IP, so they must bypass the panel's domain whitelist (they
// authenticate with a per-node secret instead).
var agentPaths = map[string]bool{
	"/system-info": true,
	"/flow/upload": true,
	"/flow/config": true,
}

// DomainValidatorMiddleware returns a Gin middleware that validates the request domain.
// It extracts the host from the request, strips any port number, and compares it
// against the configured domain. Requests from unauthorized domains are rejected
// with HTTP 403 Forbidden status.
func DomainValidatorMiddleware(domain string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if agentPaths[c.Request.URL.Path] {
			c.Next()
			return
		}

		host := c.Request.Host
		if colonIndex := strings.LastIndex(host, ":"); colonIndex != -1 {
			host, _, _ = net.SplitHostPort(c.Request.Host)
		}

		if host != domain {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		c.Next()
	}
}
