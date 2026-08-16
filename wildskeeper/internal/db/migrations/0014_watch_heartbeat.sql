-- server_watch records the last moment the collector successfully observed
-- a server's player list.
--
-- Palcon's own downtime is invisible from the server's side. A server
-- outage closes every open session (the collector sees the probes fail),
-- but palcon restarting used to close nothing: a player online at shutdown
-- kept a join with no matching leave, which reads downstream as a session
-- that is still running now and grows by a day every day. The heartbeat is
-- the honest upper bound on what palcon actually watched, so on startup
-- those sessions can be closed where observation stopped.
CREATE TABLE server_watch (
    server_id INTEGER PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
    last_seen TEXT NOT NULL
);
