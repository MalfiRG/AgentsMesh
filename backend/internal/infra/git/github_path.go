package git

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// canonicalProjectPath resolves a GitHub project identifier to "owner/repo".
// Imports persist the numeric repository ID in external_id, but GitHub's named
// REST endpoints (/repos/{owner}/{repo}/...) reject numeric IDs with 404. A
// numeric ID is resolved through /repositories/{id}, which returns full_name;
// an already-qualified "owner/repo" passes through without an extra request.
func (p *GitHubProvider) canonicalProjectPath(ctx context.Context, projectID string) (string, error) {
	if strings.Contains(projectID, "/") {
		return projectID, nil
	}

	resp, err := p.doRequest(ctx, "GET", "/repositories/"+projectID, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var repo struct {
		FullName string `json:"full_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repo); err != nil {
		return "", err
	}
	if repo.FullName == "" {
		return "", fmt.Errorf("github: empty full_name resolving repository %s", projectID)
	}
	return repo.FullName, nil
}
