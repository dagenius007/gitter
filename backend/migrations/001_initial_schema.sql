-- Create github_auth table for storing GitHub OAuth tokens and user information
-- This table associates session IDs with GitHub tokens and owners

CREATE TABLE IF NOT EXISTS github_auth (
    session_id VARCHAR(255) PRIMARY KEY,
    github_token TEXT NOT NULL,
    github_owner VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Create index on github_owner for faster lookups
CREATE INDEX IF NOT EXISTS idx_github_auth_owner ON github_auth(github_owner);

-- Create index on created_at for cleanup of old records
CREATE INDEX IF NOT EXISTS idx_github_auth_created_at ON github_auth(created_at);
