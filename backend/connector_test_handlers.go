package main

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Test connection request bodies (user submits from Connect Systems page)
type OktaTestRequest struct {
	Domain string `json:"domain"` // e.g. https://your-org.okta.com
	Token  string `json:"token"`
}

type SailPointTestRequest struct {
	Tenant   string `json:"tenant"`   // e.g. https://mytenant.api.identitynow.com
	ClientID string `json:"client_id"`
	Secret   string `json:"secret"`
}

type GCPTestRequest struct {
	ProjectID string `json:"project_id"`
	// JSON key is typically uploaded as file; for test we could accept base64 or skip validation
}

// TestOktaConnection validates Okta URL + API token
func TestOktaConnection() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req OktaTestRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid request body"})
			return
		}
		domain := strings.TrimSuffix(strings.TrimSpace(req.Domain), "/")
		if domain == "" || req.Token == "" {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "domain and token required"})
			return
		}
		// Lightweight check: GET /api/v1/users?limit=1 with Authorization
		client := &http.Client{}
		r, err := http.NewRequest("GET", domain+"/api/v1/users?limit=1", nil)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"ok": false, "error": "invalid domain URL"})
			return
		}
		r.Header.Set("Authorization", "SSWS "+req.Token)
		resp, err := client.Do(r)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"ok": false, "error": "connection failed: " + err.Error()})
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent {
			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Connection successful"})
			return
		}
		if resp.StatusCode == 401 {
			c.JSON(http.StatusOK, gin.H{"ok": false, "error": "Invalid API token (401)"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "Unexpected status: " + resp.Status})
	}
}

// TestSailPointConnection validates SailPoint IdentityNow client credentials
func TestSailPointConnection() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SailPointTestRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid request body"})
			return
		}
		tenant := strings.TrimSuffix(strings.TrimSpace(req.Tenant), "/")
		if tenant == "" || req.ClientID == "" || req.Secret == "" {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "tenant, client_id and secret required"})
			return
		}
		// OAuth2 token request to IdentityNow
		// POST {tenant}/oauth/token with grant_type=client_credentials
		client := &http.Client{}
		r, err := http.NewRequest("POST", tenant+"/oauth/token", strings.NewReader("grant_type=client_credentials"))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"ok": false, "error": "invalid tenant URL"})
			return
		}
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.SetBasicAuth(req.ClientID, req.Secret)
		resp, err := client.Do(r)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"ok": false, "error": "connection failed: " + err.Error()})
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Connection successful"})
			return
		}
		if resp.StatusCode == 401 {
			c.JSON(http.StatusOK, gin.H{"ok": false, "error": "Invalid client_id or secret (401)"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "Unexpected status: " + resp.Status})
	}
}

// TestGCPConnection validates GCP project (minimal: project ID format or optional API call)
func TestGCPConnection() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req GCPTestRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "invalid request body"})
			return
		}
		pid := strings.TrimSpace(req.ProjectID)
		if pid == "" {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "project_id required"})
			return
		}
		// No credentials in body for security; user configures GOOGLE_APPLICATION_CREDENTIALS when running connector
		c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Project ID accepted. Run the GCP connector with GOOGLE_APPLICATION_CREDENTIALS set to your service account JSON path."})
	}
}
