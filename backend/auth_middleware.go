package main

import (
	"encoding/base64"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// BasicAuthMiddleware requires HTTP Basic auth when AUTH_USER and AUTH_PASSWORD are set.
// Sets "user" and "role" in context (role from env AUTH_ROLE, default "analyst").
func BasicAuthMiddleware() gin.HandlerFunc {
	user := os.Getenv("AUTH_USER")
	pass := os.Getenv("AUTH_PASSWORD")
	role := os.Getenv("AUTH_ROLE")
	if role == "" {
		role = "analyst"
	}
	return func(c *gin.Context) {
		if !strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Set("user", "anonymous")
			c.Set("role", "analyst")
			c.Next()
			return
		}
		if user == "" || pass == "" {
			c.Set("user", "anonymous")
			c.Set("role", "analyst")
			c.Next()
			return
		}
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Basic ") {
			c.Header("WWW-Authenticate", `Basic realm="Identity Observability"`)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		payload := strings.TrimPrefix(auth, "Basic ")
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		decodedStr := string(decoded)
		parts := strings.SplitN(decodedStr, ":", 2)
		if len(parts) != 2 || parts[0] != user || parts[1] != pass {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set("user", parts[0])
		c.Set("role", role)
		c.Next()
	}
}

// RequireAdmin aborts with 403 if context role is not "admin". When auth is not configured, allows access.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if os.Getenv("AUTH_USER") == "" || os.Getenv("AUTH_PASSWORD") == "" {
			c.Next()
			return
		}
		r, _ := c.Get("role")
		if r != "admin" {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}

// RequireNotReadOnly blocks read_only role from export endpoints when auth is configured.
func RequireNotReadOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if os.Getenv("AUTH_USER") == "" || os.Getenv("AUTH_PASSWORD") == "" {
			c.Next()
			return
		}
		r, _ := c.Get("role")
		if r == "read_only" || r == "readonly" {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}
