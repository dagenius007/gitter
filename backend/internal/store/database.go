package store

import (
	"database/sql"
	"fmt"
	"time"

	"zana-speech-backend/internal/db"
)

// DatabaseStore stores GitHub authentication data in PostgreSQL
type DatabaseStore struct {
	db *db.DB
}

// NewDatabaseStore creates a new database store
func NewDatabaseStore(database *db.DB) *DatabaseStore {
	return &DatabaseStore{db: database}
}

// GitHubAuth represents GitHub authentication data
type GitHubAuth struct {
	SessionID   string
	GitHubToken string
	GitHubOwner string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// SaveGitHubAuth saves or updates GitHub authentication data for a session
func (ds *DatabaseStore) SaveGitHubAuth(sessionID, githubToken, githubOwner string) error {
	if sessionID == "" || githubToken == "" || githubOwner == "" {
		return fmt.Errorf("session_id, github_token, and github_owner are required")
	}

	query := `
		INSERT INTO github_auth (session_id, github_token, github_owner, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (session_id) 
		DO UPDATE SET 
			github_token = EXCLUDED.github_token,
			github_owner = EXCLUDED.github_owner,
			updated_at = NOW()
	`

	_, err := ds.db.Exec(query, sessionID, githubToken, githubOwner)
	if err != nil {
		return fmt.Errorf("failed to save GitHub auth: %w", err)
	}

	return nil
}

// GetGitHubAuth retrieves GitHub authentication data for a session
func (ds *DatabaseStore) GetGitHubAuth(sessionID string) (*GitHubAuth, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	var auth GitHubAuth
	query := `
		SELECT session_id, github_token, github_owner, created_at, updated_at
		FROM github_auth
		WHERE session_id = $1
	`

	err := ds.db.QueryRow(query, sessionID).Scan(
		&auth.SessionID,
		&auth.GitHubToken,
		&auth.GitHubOwner,
		&auth.CreatedAt,
		&auth.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Not found, return nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub auth: %w", err)
	}

	return &auth, nil
}

// DeleteGitHubAuth removes GitHub authentication data for a session
func (ds *DatabaseStore) DeleteGitHubAuth(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}

	query := `DELETE FROM github_auth WHERE session_id = $1`
	_, err := ds.db.Exec(query, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete GitHub auth: %w", err)
	}

	return nil
}

// GetGitHubAuthByOwner retrieves GitHub authentication data by owner username
func (ds *DatabaseStore) GetGitHubAuthByOwner(owner string) (*GitHubAuth, error) {
	if owner == "" {
		return nil, fmt.Errorf("owner is required")
	}

	var auth GitHubAuth
	query := `
		SELECT session_id, github_token, github_owner, created_at, updated_at
		FROM github_auth
		WHERE github_owner = $1
		ORDER BY updated_at DESC
		LIMIT 1
	`

	err := ds.db.QueryRow(query, owner).Scan(
		&auth.SessionID,
		&auth.GitHubToken,
		&auth.GitHubOwner,
		&auth.CreatedAt,
		&auth.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Not found
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub auth by owner: %w", err)
	}

	return &auth, nil
}
