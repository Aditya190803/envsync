CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_unique
ON users (email)
WHERE email IS NOT NULL;

CREATE TABLE IF NOT EXISTS personal_access_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_prefix TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  scopes TEXT[] NOT NULL DEFAULT ARRAY['profile:read','store:read','store:write']::text[],
  expires_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,
  last_used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_personal_access_tokens_prefix
ON personal_access_tokens (token_prefix);

CREATE INDEX IF NOT EXISTS idx_personal_access_tokens_user
ON personal_access_tokens (user_id);

CREATE INDEX IF NOT EXISTS idx_organization_members_user
ON organization_members (user_id);

CREATE INDEX IF NOT EXISTS idx_organization_members_org
ON organization_members (organization_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_vaults_owner_project_unique
ON vaults (owner_type, owner_id, project_name);

CREATE INDEX IF NOT EXISTS idx_audit_events_vault_created
ON audit_events (vault_owner_user_id, created_at DESC);
