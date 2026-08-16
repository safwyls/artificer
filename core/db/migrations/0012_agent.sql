-- Sidecar agent (docs/sidecar-agent.md): optional URL of the server's
-- flameagent container and the bearer token palcon presents to it. The token
-- is encrypted at rest exactly like the RCON/REST passwords, and like them
-- is never sent back out through the API. Empty URL = no agent; the local
-- bind-mount paths remain a fully supported fallback.
ALTER TABLE servers ADD COLUMN agent_url TEXT NOT NULL DEFAULT '';
ALTER TABLE servers ADD COLUMN agent_token_enc TEXT NOT NULL DEFAULT '';
