package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

type Client struct {
	apiKey    string
	teamID    string
	httpClient *http.Client
}

func New(ctx context.Context) (*Client, error) {
	apiKey := os.Getenv("LINEAR_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("LINEAR_API_KEY env var not set")
	}

	teamKeyOrID := os.Getenv("LINEAR_TEAM_ID")
	if teamKeyOrID == "" {
		return nil, fmt.Errorf("LINEAR_TEAM_ID env var not set")
	}

	client := &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}

	// Try to resolve team key to UUID
	teamID, err := client.resolveTeamID(ctx, teamKeyOrID)
	if err != nil {
		return nil, err
	}

	client.teamID = teamID
	log.Printf("✓ Linear team resolved: %s -> %s\n", teamKeyOrID, teamID)

	return client, nil
}

func (c *Client) resolveTeamID(ctx context.Context, teamKeyOrID string) (string, error) {
	// If it looks like a UUID (36 chars with dashes), assume it's already a UUID
	if len(teamKeyOrID) == 36 && teamKeyOrID[8] == '-' {
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
		return foundID, nil
	}

	return "", fmt.Errorf("team %q not found. Available teams:%s", teamKeyOrID, availableTeams)
}

func (c *Client) CreateIssue(ctx context.Context, title, description string) (string, error) {
	query := `mutation CreateIssue($title: String!, $description: String, $teamId: String!) {
		issueCreate(input: {
			title: $title
			description: $description
			teamId: $teamId
		}) {
			issue {
				id
				url
			}
		}
	}`

	variables := map[string]interface{}{
		"title":  title,
		"teamId": c.teamID,
	}
	// Only include description if it's not empty
	if description != "" {
		variables["description"] = description
	} else {
		variables["description"] = nil
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
