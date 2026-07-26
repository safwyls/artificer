package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

// Server is the decrypted, application-facing view of a servers row.
// RCONPassword/RESTPassword are only populated when explicitly needed
// (e.g. to build a palworld.Client) and are never serialized to the API.
type Server struct {
	ID           int64
	Name         string
	Host         string
	RCONPort     int
	RCONPassword string
	RESTPort     int
	RESTPassword string
	UseREST      bool
	Enabled      bool
	// SavePath is an optional container-local path to the directory holding
	// the server's Level.sav (phase 5 Pal viewer), bind-mounted read-only.
	// Empty = not configured.
	SavePath string
	// ConfigPath is an optional container-local path to the directory holding
	// the server's PalWorldSettings.ini, bind-mounted read-write so the
	// settings editor can change it. Separate from SavePath so save data
	// stays read-only. Empty = settings editor off.
	ConfigPath string
	// ContainerName is the Docker container this server runs in, used for
	// start/stop/restart via the socket proxy. Empty = power control off.
	ContainerName string
	// Watchdog restarts the container after an unclean exit. Toggled via
	// SetWatchdog, not UpdateServer — the server-edit form doesn't carry it,
	// and a form save must never silently switch the watchdog off.
	Watchdog bool
	// PublicToken makes a read-only status page available at
	// /status/<token>; empty = off. Managed via SetPublicToken, outside
	// UpdateServer for the same reason as Watchdog.
	PublicToken string
}

type serverRow struct {
	ID              int64
	Name            string
	Host            string
	RCONPort        int
	RCONPasswordEnc string
	RESTPort        int
	RESTPasswordEnc string
	UseREST         int
	Enabled         int
	SavePath        string
	ConfigPath      string
	ContainerName   string
	Watchdog        int
	PublicToken     string
}

func (s *Store) decryptServer(r serverRow) (*Server, error) {
	rconPass, err := s.box.Decrypt(r.RCONPasswordEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypting rcon password: %w", err)
	}
	restPass, err := s.box.Decrypt(r.RESTPasswordEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypting rest password: %w", err)
	}
	return &Server{
		ID:            r.ID,
		Name:          r.Name,
		Host:          r.Host,
		RCONPort:      r.RCONPort,
		RCONPassword:  rconPass,
		RESTPort:      r.RESTPort,
		RESTPassword:  restPass,
		UseREST:       r.UseREST != 0,
		Enabled:       r.Enabled != 0,
		SavePath:      r.SavePath,
		ConfigPath:    r.ConfigPath,
		ContainerName: r.ContainerName,
		Watchdog:      r.Watchdog != 0,
		PublicToken:   r.PublicToken,
	}, nil
}

const serverColumns = `id, name, host, rcon_port, rcon_password_enc, rest_port, rest_password_enc, use_rest, enabled, save_path, config_path, container_name, watchdog, public_token`

func scanServerRow(scan func(dest ...any) error) (serverRow, error) {
	var r serverRow
	err := scan(&r.ID, &r.Name, &r.Host, &r.RCONPort, &r.RCONPasswordEnc, &r.RESTPort, &r.RESTPasswordEnc, &r.UseREST, &r.Enabled, &r.SavePath, &r.ConfigPath, &r.ContainerName, &r.Watchdog, &r.PublicToken)
	return r, err
}

func (s *Store) ListServers(ctx context.Context) ([]*Server, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+serverColumns+` FROM servers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Server
	for rows.Next() {
		r, err := scanServerRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		srv, err := s.decryptServer(r)
		if err != nil {
			return nil, err
		}
		out = append(out, srv)
	}
	return out, rows.Err()
}

func (s *Store) GetServer(ctx context.Context, id int64) (*Server, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+serverColumns+` FROM servers WHERE id = ?`, id)
	r, err := scanServerRow(row.Scan)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.decryptServer(r)
}

// CreateServer inserts a new server, encrypting the given plaintext
// passwords before they touch disk.
func (s *Store) CreateServer(ctx context.Context, srv *Server) (int64, error) {
	rconEnc, err := s.box.Encrypt(srv.RCONPassword)
	if err != nil {
		return 0, err
	}
	restEnc, err := s.box.Encrypt(srv.RESTPassword)
	if err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO servers (name, host, rcon_port, rcon_password_enc, rest_port, rest_password_enc, use_rest, enabled, save_path, config_path, container_name)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		srv.Name, srv.Host, srv.RCONPort, rconEnc, srv.RESTPort, restEnc, boolToInt(srv.UseREST), boolToInt(srv.Enabled), srv.SavePath, srv.ConfigPath, srv.ContainerName)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateServer updates fields on an existing server. Passwords are only
// re-encrypted and overwritten when non-empty, so callers can update other
// fields without resending credentials.
func (s *Store) UpdateServer(ctx context.Context, srv *Server) error {
	existing, err := s.GetServer(ctx, srv.ID)
	if err != nil {
		return err
	}
	if srv.RCONPassword == "" {
		srv.RCONPassword = existing.RCONPassword
	}
	if srv.RESTPassword == "" {
		srv.RESTPassword = existing.RESTPassword
	}

	rconEnc, err := s.box.Encrypt(srv.RCONPassword)
	if err != nil {
		return err
	}
	restEnc, err := s.box.Encrypt(srv.RESTPassword)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE servers
		SET name = ?, host = ?, rcon_port = ?, rcon_password_enc = ?,
		    rest_port = ?, rest_password_enc = ?, use_rest = ?, enabled = ?,
		    save_path = ?, config_path = ?, container_name = ?
		WHERE id = ?`,
		srv.Name, srv.Host, srv.RCONPort, rconEnc, srv.RESTPort, restEnc,
		boolToInt(srv.UseREST), boolToInt(srv.Enabled), srv.SavePath, srv.ConfigPath, srv.ContainerName, srv.ID)
	return err
}

// SetWatchdog flips the crash watchdog on its own — see the field comment
// for why this isn't part of UpdateServer.
func (s *Store) SetWatchdog(ctx context.Context, id int64, enabled bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE servers SET watchdog = ? WHERE id = ?`, boolToInt(enabled), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// SetPublicToken sets or clears the public status token, outside
// UpdateServer for the same never-wiped-by-a-form-save reason as Watchdog.
func (s *Store) SetPublicToken(ctx context.Context, id int64, token string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE servers SET public_token = ? WHERE id = ?`, token, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// GetServerByPublicToken resolves a public status token. An empty token
// never matches — it's the "feature off" value on every row.
func (s *Store) GetServerByPublicToken(ctx context.Context, token string) (*Server, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	row := s.db.QueryRowContext(ctx, `SELECT `+serverColumns+` FROM servers WHERE public_token = ?`, token)
	r, err := scanServerRow(row.Scan)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.decryptServer(r)
}

func (s *Store) DeleteServer(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM servers WHERE id = ?`, id)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
