-- Public status page: a per-server unguessable token that makes a
-- read-only, unauthenticated status view available at /status/<token>.
-- Empty means the feature is off; regenerating the token revokes old URLs.
ALTER TABLE servers ADD COLUMN public_token TEXT NOT NULL DEFAULT '';
