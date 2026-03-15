package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/mr-exz/cibot/internal/config"
)

type Client struct {
	apiKey     string
	httpClient *http.Client
	teamCache  map[string]string // key/ID -> UUID
	userCache  map[string]string // username -> user ID
	labelCache map[string]string // "teamID:labelName" -> label UUID
	cacheMu    sync.RWMutex
}

func New(ctx context.Context, cfg *config.Config) (*Client, error) {
	client := &Client{
		apiKey:     cfg.LinearAPIKey,
		httpClient: &http.Client{},
		teamCache:  make(map[string]string),
		userCache:  make(map[string]string),
		labelCache: make(map[string]string),
	}

	log.Printf("✓ Linear client initialized")

	return client, nil
}

func (c *Client) resolveTeamID(ctx context.Context, teamKeyOrID string) (string, error) {
	// Check cache first
	c.cacheMu.RLock()
	if cached, ok := c.teamCache[teamKeyOrID]; ok {
		c.cacheMu.RUnlock()
		return cached, nil
	}
	c.cacheMu.RUnlock()

	// If it looks like a UUID (36 chars with dashes), assume it's already a UUID
	if len(teamKeyOrID) == 36 && teamKeyOrID[8] == '-' {
		// Cache it and return
		c.cacheMu.Lock()
		c.teamCache[teamKeyOrID] = teamKeyOrID
		c.cacheMu.Unlock()
		return teamKeyOrID, nil
	}

	// Otherwise, fetch all teams and find by key
	query := `{
		teams(first: 100) {
			edges {
				node {
					id
					name
					key
				}
			}
		}
	}`

	payload := map[string]interface{}{
		"query": query,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.linear.app/graphql", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch teams: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		Data struct {
			Teams struct {
				Edges []struct {
					Node struct {
						ID   string `json:"id"`
						Name string `json:"name"`
						Key  string `json:"key"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"teams"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse teams response: %w", err)
	}

	if len(result.Errors) > 0 {
		return "", fmt.Errorf("Linear API error: %s", result.Errors[0].Message)
	}

	// Find team by key or exact ID match
	var foundID string
	var availableTeams string

	for _, edge := range result.Data.Teams.Edges {
		node := edge.Node
		availableTeams += fmt.Sprintf("\n  - %s (key: %s, id: %s)", node.Name, node.Key, node.ID)

		if node.Key == teamKeyOrID || node.ID == teamKeyOrID {
			foundID = node.ID
		}
	}

	if foundID != "" {
		// Cache the result
		c.cacheMu.Lock()
		c.teamCache[teamKeyOrID] = foundID
		c.cacheMu.Unlock()
		return foundID, nil
	}

	return "", fmt.Errorf("team %q not found. Available teams:%s", teamKeyOrID, availableTeams)
}

func (c *Client) resolveUserID(ctx context.Context, linearUsername string) (string, error) {
	if linearUsername == "" {
		return "", nil // No assignee
	}

	// Check cache first
	c.cacheMu.RLock()
	if cached, ok := c.userCache[linearUsername]; ok {
		c.cacheMu.RUnlock()
		return cached, nil
	}
	c.cacheMu.RUnlock()

	// Query for user by displayName (which typically matches username)
	query := `{
		users(first: 100, filter: {displayName: {contains: "` + linearUsername + `"}}) {
			edges {
				node {
					id
					displayName
					email
				}
			}
		}
	}`

	payload := map[string]interface{}{
		"query": query,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal user request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.linear.app/graphql", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create user request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch user: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read user response: %w", err)
	}

	var result struct {
		Data struct {
			Users struct {
				Edges []struct {
					Node struct {
						ID          string `json:"id"`
						DisplayName string `json:"displayName"`
						Email       string `json:"email"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"users"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse user response: %w", err)
	}

	if len(result.Errors) > 0 {
		return "", fmt.Errorf("Linear API error: %s", result.Errors[0].Message)
	}

	if len(result.Data.Users.Edges) == 0 {
		return "", fmt.Errorf("user %q not found in Linear", linearUsername)
	}

	userID := result.Data.Users.Edges[0].Node.ID

	// Cache the result
	c.cacheMu.Lock()
	c.userCache[linearUsername] = userID
	c.cacheMu.Unlock()

	return userID, nil
}

func (c *Client) resolveLabelID(ctx context.Context, labelName, teamID string) (string, error) {
	cacheKey := teamID + ":" + labelName
	c.cacheMu.RLock()
	if cached, ok := c.labelCache[cacheKey]; ok {
		c.cacheMu.RUnlock()
		return cached, nil
	}
	c.cacheMu.RUnlock()

	// Query for label by name in the team
	query := `{
		issueLabels(first: 100, filter: {name: {eqIgnoreCase: "` + labelName + `"}, team: {id: {eq: "` + teamID + `"}}}) {
			edges {
				node {
					id
					name
				}
			}
		}
	}`

	payload := map[string]interface{}{
		"query": query,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal label request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.linear.app/graphql", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create label request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch label: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read label response: %w", err)
	}

	var result struct {
		Data struct {
			IssueLabels struct {
				Edges []struct {
					Node struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"issueLabels"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse label response: %w", err)
	}

	if len(result.Errors) > 0 {
		return "", fmt.Errorf("Linear API error: %s", result.Errors[0].Message)
	}

	if len(result.Data.IssueLabels.Edges) == 0 {
		// Label doesn't exist — create it
		return c.createLabel(ctx, labelName, teamID)
	}

	labelID := result.Data.IssueLabels.Edges[0].Node.ID

	c.cacheMu.Lock()
	c.labelCache[cacheKey] = labelID
	c.cacheMu.Unlock()

	return labelID, nil
}

func (c *Client) createLabel(ctx context.Context, labelName, teamID string) (string, error) {
	payload := map[string]interface{}{
		"query": `mutation CreateLabel($name: String!, $teamId: String!) {
			issueLabelCreate(input: { name: $name, teamId: $teamId }) {
				issueLabel { id name }
			}
		}`,
		"variables": map[string]interface{}{
			"name":   labelName,
			"teamId": teamID,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal create label request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.linear.app/graphql", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create label request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to create label: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read create label response: %w", err)
	}

	var result struct {
		Data struct {
			IssueLabelCreate struct {
				IssueLabel struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"issueLabel"`
			} `json:"issueLabelCreate"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse create label response: %w", err)
	}
	if len(result.Errors) > 0 {
		return "", fmt.Errorf("Linear API error: %s", result.Errors[0].Message)
	}

	labelID := result.Data.IssueLabelCreate.IssueLabel.ID
	if labelID == "" {
		return "", fmt.Errorf("create label returned empty ID")
	}

	log.Printf("✓ Created Linear label %q in team %s", labelName, teamID)

	cacheKey := teamID + ":" + labelName
	c.cacheMu.Lock()
	c.labelCache[cacheKey] = labelID
	c.cacheMu.Unlock()

	return labelID, nil
}

func (c *Client) CreateIssue(ctx context.Context, title, description, teamKey, assigneeUsername string, labels []string) (string, error) {
	// Resolve team key to UUID
	teamID, err := c.resolveTeamID(ctx, teamKey)
	if err != nil {
		return "", fmt.Errorf("failed to resolve team: %w", err)
	}

	// Resolve assignee username to user ID (if provided)
	var assigneeID string
	if assigneeUsername != "" {
		assigneeID, err = c.resolveUserID(ctx, assigneeUsername)
		if err != nil {
			log.Printf("⚠️  Failed to resolve assignee %s: %v", assigneeUsername, err)
		}
	}

	// Resolve each label name to a label ID; skip ones not found
	var labelIDs []string
	for _, label := range labels {
		if label == "" {
			continue
		}
		labelID, err := c.resolveLabelID(ctx, label, teamID)
		if err != nil {
			log.Printf("⚠️  Label %q not found in Linear (team %s): %v", label, teamKey, err)
			continue
		}
		labelIDs = append(labelIDs, labelID)
	}

	// Build mutation query
	query := `mutation CreateIssue($title: String!, $description: String, $teamId: String!, $assigneeId: String, $labelIds: [String!]) {
		issueCreate(input: {
			title: $title
			description: $description
			teamId: $teamId
			assigneeId: $assigneeId
			labelIds: $labelIds
		}) {
			issue {
				id
				url
			}
		}
	}`

	variables := map[string]interface{}{
		"title":  title,
		"teamId": teamID,
	}
	// Only include description if it's not empty
	if description != "" {
		variables["description"] = description
	}
	// Only include assignee if resolved
	if assigneeID != "" {
		variables["assigneeId"] = assigneeID
	}
	// Only include labels if resolved
	if len(labelIDs) > 0 {
		variables["labelIds"] = labelIDs
	}

	payload := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.linear.app/graphql", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Linear API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	log.Printf("Linear API response: %s\n", string(respBody))

	var result struct {
		Data struct {
			IssueCreate struct {
				Issue struct {
					ID  string `json:"id"`
					URL string `json:"url"`
				} `json:"issue"`
			} `json:"issueCreate"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Errors) > 0 {
		return "", fmt.Errorf("Linear API error: %s", result.Errors[0].Message)
	}

	if result.Data.IssueCreate.Issue.URL == "" {
		return "", fmt.Errorf("failed to create issue: no URL in response")
	}

	return result.Data.IssueCreate.Issue.URL, nil
}
