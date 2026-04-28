package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// JiraClient handles Jira integration
type JiraClient struct {
	baseURL  string
	username string
	apiToken string
}

// NewJiraClient creates a new Jira client
func NewJiraClient() *JiraClient {
	baseURL := os.Getenv("JIRA_BASE_URL")
	username := os.Getenv("JIRA_USERNAME")
	apiToken := os.Getenv("JIRA_API_TOKEN")

	if baseURL == "" || username == "" || apiToken == "" {
		return nil // Jira not configured
	}

	return &JiraClient{
		baseURL:  baseURL,
		username: username,
		apiToken: apiToken,
	}
}

// CreateTicket creates a Jira ticket
func (jc *JiraClient) CreateTicket(ctx context.Context, identityID int64, actionType string, description string) (string, error) {
	if jc == nil {
		return "", fmt.Errorf("jira client not configured")
	}

	// Jira issue structure
	issue := map[string]interface{}{
		"fields": map[string]interface{}{
			"project": map[string]interface{}{
				"key": os.Getenv("JIRA_PROJECT_KEY"), // Default or from env
			},
			"summary":     fmt.Sprintf("Remediation: %s for Identity %d", actionType, identityID),
			"description": description,
			"issuetype": map[string]interface{}{
				"name": "Task",
			},
		},
	}

	data, err := json.Marshal(issue)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/rest/api/3/issue", jc.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(jc.username, jc.apiToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("jira API error: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	key, _ := result["key"].(string)
	return key, nil
}
