package config

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

type sqliteStore struct {
	db *sql.DB
}

const maxAuditEntries = 100

type mutableSnapshot struct {
	Peers   []PeerConfig
	Routing RoutingConfig
}

func openSQLiteStore(cfg DatabaseConfig) (*sqliteStore, error) {
	db, err := sql.Open("sqlite", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database %q: %w", cfg.Path, err)
	}

	store := &sqliteStore{db: db}
	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *sqliteStore) Close() error {
	return s.db.Close()
}

func (s *sqliteStore) init(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS peers (
			name TEXT PRIMARY KEY,
			address TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			description TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS routing_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS apn_routes (
			apn TEXT PRIMARY KEY,
			peer TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS imsi_routes (
			imsi TEXT PRIMARY KEY,
			peer TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS imsi_prefix_routes (
			prefix TEXT PRIMARY KEY,
			peer TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS plmn_routes (
			plmn TEXT PRIMARY KEY,
			peer TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action TEXT NOT NULL,
			object_type TEXT NOT NULL,
			object_key TEXT NOT NULL,
			before_json TEXT NOT NULL DEFAULT '',
			after_json TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
	}

	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("initialize sqlite config store: %w", err)
		}
	}
	return nil
}

func (s *sqliteStore) load(ctx context.Context, tx *sql.Tx) (mutableSnapshot, error) {
	query := querier(s.db)
	if tx != nil {
		query = tx
	}

	peers, err := loadPeers(ctx, query)
	if err != nil {
		return mutableSnapshot{}, err
	}
	defaultPeer, err := loadDefaultPeer(ctx, query)
	if err != nil {
		return mutableSnapshot{}, err
	}
	imsiRoutes, err := loadIMSIRoutes(ctx, query)
	if err != nil {
		return mutableSnapshot{}, err
	}
	imsiPrefixRoutes, err := loadIMSIPrefixRoutes(ctx, query)
	if err != nil {
		return mutableSnapshot{}, err
	}
	apnRoutes, err := loadAPNRoutes(ctx, query)
	if err != nil {
		return mutableSnapshot{}, err
	}
	plmnRoutes, err := loadPLMNRoutes(ctx, query)
	if err != nil {
		return mutableSnapshot{}, err
	}

	return mutableSnapshot{
		Peers: peers,
		Routing: RoutingConfig{
			DefaultPeer:      defaultPeer,
			IMSIRoutes:       imsiRoutes,
			IMSIPrefixRoutes: imsiPrefixRoutes,
			APNRoutes:        apnRoutes,
			PLMNRoutes:       plmnRoutes,
		},
	}, nil
}

func (s *sqliteStore) upsertPeer(ctx context.Context, tx *sql.Tx, peer PeerConfig) error {
	if strings.TrimSpace(peer.Description) == "" {
		peer.Description = peer.Name
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO peers(name, address, enabled, description)
		 VALUES(?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		   address = excluded.address,
		   enabled = excluded.enabled,
		   description = excluded.description`,
		peer.Name,
		peer.Address,
		boolToInt(peer.Enabled),
		peer.Description,
	)
	if err != nil {
		return fmt.Errorf("upsert peer %q: %w", peer.Name, err)
	}
	return nil
}

func (s *sqliteStore) deletePeer(ctx context.Context, tx *sql.Tx, name string) error {
	result, err := tx.ExecContext(ctx, `DELETE FROM peers WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete peer %q: %w", name, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("peer %q not found", name)
	}
	return nil
}

func (s *sqliteStore) setDefaultPeer(ctx context.Context, tx *sql.Tx, name string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO routing_settings(key, value)
		 VALUES('default_peer', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		name,
	)
	if err != nil {
		return fmt.Errorf("set default peer %q: %w", name, err)
	}
	return nil
}

func (s *sqliteStore) upsertAPNRoute(ctx context.Context, tx *sql.Tx, route APNRoute) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO apn_routes(apn, peer)
		 VALUES(?, ?)
		 ON CONFLICT(apn) DO UPDATE SET peer = excluded.peer`,
		route.APN,
		route.Peer,
	)
	if err != nil {
		return fmt.Errorf("upsert APN route %q: %w", route.APN, err)
	}
	return nil
}

func (s *sqliteStore) deleteAPNRoute(ctx context.Context, tx *sql.Tx, apn string) error {
	result, err := tx.ExecContext(ctx, `DELETE FROM apn_routes WHERE lower(trim(apn)) = lower(trim(?))`, apn)
	if err != nil {
		return fmt.Errorf("delete APN route %q: %w", apn, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("APN route %q not found", apn)
	}
	return nil
}

func (s *sqliteStore) upsertIMSIRoute(ctx context.Context, tx *sql.Tx, route IMSIRoute) error {
	key := normalizeDigits(route.IMSI)
	_, err := tx.ExecContext(ctx,
		`INSERT INTO imsi_routes(imsi, peer)
		 VALUES(?, ?)
		 ON CONFLICT(imsi) DO UPDATE SET peer = excluded.peer`,
		key,
		route.Peer,
	)
	if err != nil {
		return fmt.Errorf("upsert IMSI route %q: %w", route.IMSI, err)
	}
	return nil
}

func (s *sqliteStore) deleteIMSIRoute(ctx context.Context, tx *sql.Tx, imsi string) error {
	key := normalizeDigits(imsi)
	result, err := tx.ExecContext(ctx, `DELETE FROM imsi_routes WHERE imsi = ?`, key)
	if err != nil {
		return fmt.Errorf("delete IMSI route %q: %w", imsi, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("IMSI route %q not found", imsi)
	}
	return nil
}

func (s *sqliteStore) upsertIMSIPrefixRoute(ctx context.Context, tx *sql.Tx, route IMSIPrefixRoute) error {
	key := normalizeDigits(route.Prefix)
	_, err := tx.ExecContext(ctx,
		`INSERT INTO imsi_prefix_routes(prefix, peer)
		 VALUES(?, ?)
		 ON CONFLICT(prefix) DO UPDATE SET peer = excluded.peer`,
		key,
		route.Peer,
	)
	if err != nil {
		return fmt.Errorf("upsert IMSI prefix route %q: %w", route.Prefix, err)
	}
	return nil
}

func (s *sqliteStore) deleteIMSIPrefixRoute(ctx context.Context, tx *sql.Tx, prefix string) error {
	key := normalizeDigits(prefix)
	result, err := tx.ExecContext(ctx, `DELETE FROM imsi_prefix_routes WHERE prefix = ?`, key)
	if err != nil {
		return fmt.Errorf("delete IMSI prefix route %q: %w", prefix, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("IMSI prefix route %q not found", prefix)
	}
	return nil
}

func (s *sqliteStore) upsertPLMNRoute(ctx context.Context, tx *sql.Tx, route PLMNRoute) error {
	key := normalizeDigits(route.PLMN)
	_, err := tx.ExecContext(ctx,
		`INSERT INTO plmn_routes(plmn, peer)
		 VALUES(?, ?)
		 ON CONFLICT(plmn) DO UPDATE SET peer = excluded.peer`,
		key,
		route.Peer,
	)
	if err != nil {
		return fmt.Errorf("upsert PLMN route %q: %w", route.PLMN, err)
	}
	return nil
}

func (s *sqliteStore) deletePLMNRoute(ctx context.Context, tx *sql.Tx, plmn string) error {
	key := normalizeDigits(plmn)
	result, err := tx.ExecContext(ctx, `DELETE FROM plmn_routes WHERE plmn = ?`, key)
	if err != nil {
		return fmt.Errorf("delete PLMN route %q: %w", plmn, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("PLMN route %q not found", plmn)
	}
	return nil
}

func (s *sqliteStore) insertAudit(ctx context.Context, tx *sql.Tx, entry AuditEntry) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO audit_log(action, object_type, object_key, before_json, after_json, created_at)
		 VALUES(?, ?, ?, ?, ?, datetime('now'))`,
		entry.Action,
		entry.ObjectType,
		entry.ObjectKey,
		entry.BeforeJSON,
		entry.AfterJSON,
	)
	if err != nil {
		return fmt.Errorf("insert audit log entry: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM audit_log
		 WHERE id NOT IN (
		   SELECT id
		   FROM audit_log
		   ORDER BY id DESC
		   LIMIT ?
		 )`, maxAuditEntries); err != nil {
		return fmt.Errorf("prune audit log: %w", err)
	}
	return nil
}

func (s *sqliteStore) listAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, action, object_type, object_key, before_json, after_json, created_at
		 FROM audit_log
		 ORDER BY id DESC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("load audit log: %w", err)
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var entry AuditEntry
		if err := rows.Scan(&entry.ID, &entry.Action, &entry.ObjectType, &entry.ObjectKey, &entry.BeforeJSON, &entry.AfterJSON, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit log: %w", err)
	}
	return entries, nil
}

type querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadPeers(ctx context.Context, q querier) ([]PeerConfig, error) {
	rows, err := q.QueryContext(ctx, `SELECT name, address, enabled, description FROM peers ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("load peers: %w", err)
	}
	defer rows.Close()

	var peers []PeerConfig
	for rows.Next() {
		var peer PeerConfig
		var enabled int
		if err := rows.Scan(&peer.Name, &peer.Address, &enabled, &peer.Description); err != nil {
			return nil, fmt.Errorf("scan peer: %w", err)
		}
		peer.Enabled = enabled != 0
		peers = append(peers, peer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate peers: %w", err)
	}
	return peers, nil
}

func loadDefaultPeer(ctx context.Context, q querier) (string, error) {
	var value string
	err := q.QueryRowContext(ctx, `SELECT value FROM routing_settings WHERE key = 'default_peer'`).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load default peer: %w", err)
	}
	return value, nil
}

func loadAPNRoutes(ctx context.Context, q querier) ([]APNRoute, error) {
	rows, err := q.QueryContext(ctx, `SELECT apn, peer FROM apn_routes ORDER BY lower(trim(apn))`)
	if err != nil {
		return nil, fmt.Errorf("load APN routes: %w", err)
	}
	defer rows.Close()

	var routes []APNRoute
	for rows.Next() {
		var route APNRoute
		if err := rows.Scan(&route.APN, &route.Peer); err != nil {
			return nil, fmt.Errorf("scan APN route: %w", err)
		}
		routes = append(routes, route)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate APN routes: %w", err)
	}
	return routes, nil
}

func loadIMSIRoutes(ctx context.Context, q querier) ([]IMSIRoute, error) {
	rows, err := q.QueryContext(ctx, `SELECT imsi, peer FROM imsi_routes ORDER BY imsi`)
	if err != nil {
		return nil, fmt.Errorf("load IMSI routes: %w", err)
	}
	defer rows.Close()

	var routes []IMSIRoute
	for rows.Next() {
		var route IMSIRoute
		if err := rows.Scan(&route.IMSI, &route.Peer); err != nil {
			return nil, fmt.Errorf("scan IMSI route: %w", err)
		}
		routes = append(routes, route)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate IMSI routes: %w", err)
	}
	return routes, nil
}

func loadIMSIPrefixRoutes(ctx context.Context, q querier) ([]IMSIPrefixRoute, error) {
	rows, err := q.QueryContext(ctx, `SELECT prefix, peer FROM imsi_prefix_routes ORDER BY length(prefix) DESC, prefix`)
	if err != nil {
		return nil, fmt.Errorf("load IMSI prefix routes: %w", err)
	}
	defer rows.Close()

	var routes []IMSIPrefixRoute
	for rows.Next() {
		var route IMSIPrefixRoute
		if err := rows.Scan(&route.Prefix, &route.Peer); err != nil {
			return nil, fmt.Errorf("scan IMSI prefix route: %w", err)
		}
		routes = append(routes, route)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate IMSI prefix routes: %w", err)
	}
	return routes, nil
}

func loadPLMNRoutes(ctx context.Context, q querier) ([]PLMNRoute, error) {
	rows, err := q.QueryContext(ctx, `SELECT plmn, peer FROM plmn_routes ORDER BY plmn`)
	if err != nil {
		return nil, fmt.Errorf("load PLMN routes: %w", err)
	}
	defer rows.Close()

	var routes []PLMNRoute
	for rows.Next() {
		var route PLMNRoute
		if err := rows.Scan(&route.PLMN, &route.Peer); err != nil {
			return nil, fmt.Errorf("scan PLMN route: %w", err)
		}
		routes = append(routes, route)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate PLMN routes: %w", err)
	}
	return routes, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
