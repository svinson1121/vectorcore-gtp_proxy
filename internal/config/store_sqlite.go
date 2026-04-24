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
	TransportDomains []TransportDomainConfig
	DNSResolvers     []DNSResolverConfig
	Peers            []PeerConfig
	Routing          RoutingConfig
}

func openSQLiteStore(cfg DatabaseConfig) (*sqliteStore, error) {
	db, err := sql.Open("sqlite", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database %q: %w", cfg.Path, err)
	}

	store := &sqliteStore{db: db}
	if err := applySQLiteMigrations(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *sqliteStore) Close() error {
	return s.db.Close()
}

func (s *sqliteStore) load(ctx context.Context, tx *sql.Tx) (mutableSnapshot, error) {
	query := querier(s.db)
	if tx != nil {
		query = tx
	}

	transportDomains, err := loadTransportDomains(ctx, query)
	if err != nil {
		return mutableSnapshot{}, err
	}
	dnsResolvers, err := loadDNSResolvers(ctx, query)
	if err != nil {
		return mutableSnapshot{}, err
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
		TransportDomains: transportDomains,
		DNSResolvers:     dnsResolvers,
		Peers:            peers,
		Routing: RoutingConfig{
			DefaultPeer:      defaultPeer,
			IMSIRoutes:       imsiRoutes,
			IMSIPrefixRoutes: imsiPrefixRoutes,
			APNRoutes:        apnRoutes,
			PLMNRoutes:       plmnRoutes,
		},
	}, nil
}

func (s *sqliteStore) upsertTransportDomain(ctx context.Context, tx *sql.Tx, domain TransportDomainConfig) error {
	if strings.TrimSpace(domain.Description) == "" {
		domain.Description = domain.Name
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO transport_domains(
			name, description, netns_path, enabled,
			gtpc_listen_host, gtpc_port, gtpu_listen_host, gtpu_port,
			gtpc_advertise_ipv4, gtpc_advertise_ipv6, gtpu_advertise_ipv4, gtpu_advertise_ipv6
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			description = excluded.description,
			netns_path = excluded.netns_path,
			enabled = excluded.enabled,
			gtpc_listen_host = excluded.gtpc_listen_host,
			gtpc_port = excluded.gtpc_port,
			gtpu_listen_host = excluded.gtpu_listen_host,
			gtpu_port = excluded.gtpu_port,
			gtpc_advertise_ipv4 = excluded.gtpc_advertise_ipv4,
			gtpc_advertise_ipv6 = excluded.gtpc_advertise_ipv6,
			gtpu_advertise_ipv4 = excluded.gtpu_advertise_ipv4,
			gtpu_advertise_ipv6 = excluded.gtpu_advertise_ipv6`,
		domain.Name,
		domain.Description,
		domain.NetNSPath,
		boolToInt(domain.Enabled),
		domain.GTPCListenHost,
		domain.GTPCPort,
		domain.GTPUListenHost,
		domain.GTPUPort,
		domain.GTPCAdvertiseIPv4,
		domain.GTPCAdvertiseIPv6,
		domain.GTPUAdvertiseIPv4,
		domain.GTPUAdvertiseIPv6,
	)
	if err != nil {
		return fmt.Errorf("upsert transport domain %q: %w", domain.Name, err)
	}
	return nil
}

func (s *sqliteStore) deleteTransportDomain(ctx context.Context, tx *sql.Tx, name string) error {
	result, err := tx.ExecContext(ctx, `DELETE FROM transport_domains WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete transport domain %q: %w", name, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("transport domain %q not found", name)
	}
	return nil
}

func (s *sqliteStore) upsertDNSResolver(ctx context.Context, tx *sql.Tx, resolver DNSResolverConfig) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO dns_resolvers(name, transport_domain, server, priority, timeout_ms, attempts, search_domain, enabled)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
			transport_domain = excluded.transport_domain,
			server = excluded.server,
			priority = excluded.priority,
			timeout_ms = excluded.timeout_ms,
			attempts = excluded.attempts,
			search_domain = excluded.search_domain,
			enabled = excluded.enabled`,
		resolver.Name,
		resolver.TransportDomain,
		resolver.Server,
		resolver.Priority,
		resolver.TimeoutMS,
		resolver.Attempts,
		resolver.SearchDomain,
		boolToInt(resolver.Enabled),
	)
	if err != nil {
		return fmt.Errorf("upsert DNS resolver %q: %w", resolver.Name, err)
	}
	return nil
}

func (s *sqliteStore) deleteDNSResolver(ctx context.Context, tx *sql.Tx, name string) error {
	result, err := tx.ExecContext(ctx, `DELETE FROM dns_resolvers WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete DNS resolver %q: %w", name, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("DNS resolver %q not found", name)
	}
	return nil
}

func (s *sqliteStore) upsertPeer(ctx context.Context, tx *sql.Tx, peer PeerConfig) error {
	if strings.TrimSpace(peer.Description) == "" {
		peer.Description = peer.Name
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO peers(name, address, transport_domain, enabled, description)
		 VALUES(?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		   address = excluded.address,
		   transport_domain = excluded.transport_domain,
		   enabled = excluded.enabled,
		   description = excluded.description`,
		peer.Name,
		peer.Address,
		peer.TransportDomain,
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
		`INSERT INTO apn_routes(apn, peer, action_type, transport_domain, fqdn, service)
		 VALUES(?, ?, ?, ?, ?, ?)
		 ON CONFLICT(apn) DO UPDATE SET
			peer = excluded.peer,
			action_type = excluded.action_type,
			transport_domain = excluded.transport_domain,
			fqdn = excluded.fqdn,
			service = excluded.service`,
		route.APN,
		route.Peer,
		normalizeRouteActionType(route.ActionType),
		route.TransportDomain,
		route.FQDN,
		route.Service,
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
		`INSERT INTO imsi_routes(imsi, peer, action_type, transport_domain, fqdn, service)
		 VALUES(?, ?, ?, ?, ?, ?)
		 ON CONFLICT(imsi) DO UPDATE SET
			peer = excluded.peer,
			action_type = excluded.action_type,
			transport_domain = excluded.transport_domain,
			fqdn = excluded.fqdn,
			service = excluded.service`,
		key,
		route.Peer,
		normalizeRouteActionType(route.ActionType),
		route.TransportDomain,
		route.FQDN,
		route.Service,
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
		`INSERT INTO imsi_prefix_routes(prefix, peer, action_type, transport_domain, fqdn, service)
		 VALUES(?, ?, ?, ?, ?, ?)
		 ON CONFLICT(prefix) DO UPDATE SET
			peer = excluded.peer,
			action_type = excluded.action_type,
			transport_domain = excluded.transport_domain,
			fqdn = excluded.fqdn,
			service = excluded.service`,
		key,
		route.Peer,
		normalizeRouteActionType(route.ActionType),
		route.TransportDomain,
		route.FQDN,
		route.Service,
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
		`INSERT INTO plmn_routes(plmn, peer, action_type, transport_domain, fqdn, service)
		 VALUES(?, ?, ?, ?, ?, ?)
		 ON CONFLICT(plmn) DO UPDATE SET
			peer = excluded.peer,
			action_type = excluded.action_type,
			transport_domain = excluded.transport_domain,
			fqdn = excluded.fqdn,
			service = excluded.service`,
		key,
		route.Peer,
		normalizeRouteActionType(route.ActionType),
		route.TransportDomain,
		route.FQDN,
		route.Service,
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
	rows, err := q.QueryContext(ctx, `SELECT name, address, transport_domain, enabled, description FROM peers ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("load peers: %w", err)
	}
	defer rows.Close()

	var peers []PeerConfig
	for rows.Next() {
		var peer PeerConfig
		var enabled int
		if err := rows.Scan(&peer.Name, &peer.Address, &peer.TransportDomain, &enabled, &peer.Description); err != nil {
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

func loadTransportDomains(ctx context.Context, q querier) ([]TransportDomainConfig, error) {
	rows, err := q.QueryContext(ctx, `SELECT
		name, description, netns_path, enabled,
		gtpc_listen_host, gtpc_port, gtpu_listen_host, gtpu_port,
		gtpc_advertise_ipv4, gtpc_advertise_ipv6, gtpu_advertise_ipv4, gtpu_advertise_ipv6
		FROM transport_domains
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("load transport domains: %w", err)
	}
	defer rows.Close()

	var domains []TransportDomainConfig
	for rows.Next() {
		var domain TransportDomainConfig
		var enabled int
		if err := rows.Scan(
			&domain.Name,
			&domain.Description,
			&domain.NetNSPath,
			&enabled,
			&domain.GTPCListenHost,
			&domain.GTPCPort,
			&domain.GTPUListenHost,
			&domain.GTPUPort,
			&domain.GTPCAdvertiseIPv4,
			&domain.GTPCAdvertiseIPv6,
			&domain.GTPUAdvertiseIPv4,
			&domain.GTPUAdvertiseIPv6,
		); err != nil {
			return nil, fmt.Errorf("scan transport domain: %w", err)
		}
		domain.Enabled = enabled != 0
		domains = append(domains, domain)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transport domains: %w", err)
	}
	return domains, nil
}

func loadDNSResolvers(ctx context.Context, q querier) ([]DNSResolverConfig, error) {
	rows, err := q.QueryContext(ctx, `SELECT
		name, transport_domain, server, priority, timeout_ms, attempts, search_domain, enabled
		FROM dns_resolvers
		ORDER BY transport_domain, priority, name`)
	if err != nil {
		return nil, fmt.Errorf("load DNS resolvers: %w", err)
	}
	defer rows.Close()

	var resolvers []DNSResolverConfig
	for rows.Next() {
		var resolver DNSResolverConfig
		var enabled int
		if err := rows.Scan(
			&resolver.Name,
			&resolver.TransportDomain,
			&resolver.Server,
			&resolver.Priority,
			&resolver.TimeoutMS,
			&resolver.Attempts,
			&resolver.SearchDomain,
			&enabled,
		); err != nil {
			return nil, fmt.Errorf("scan DNS resolver: %w", err)
		}
		resolver.Enabled = enabled != 0
		resolvers = append(resolvers, resolver)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate DNS resolvers: %w", err)
	}
	return resolvers, nil
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
	rows, err := q.QueryContext(ctx, `SELECT apn, peer, action_type, transport_domain, fqdn, service FROM apn_routes ORDER BY lower(trim(apn))`)
	if err != nil {
		return nil, fmt.Errorf("load APN routes: %w", err)
	}
	defer rows.Close()

	var routes []APNRoute
	for rows.Next() {
		var route APNRoute
		if err := rows.Scan(&route.APN, &route.Peer, &route.ActionType, &route.TransportDomain, &route.FQDN, &route.Service); err != nil {
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
	rows, err := q.QueryContext(ctx, `SELECT imsi, peer, action_type, transport_domain, fqdn, service FROM imsi_routes ORDER BY imsi`)
	if err != nil {
		return nil, fmt.Errorf("load IMSI routes: %w", err)
	}
	defer rows.Close()

	var routes []IMSIRoute
	for rows.Next() {
		var route IMSIRoute
		if err := rows.Scan(&route.IMSI, &route.Peer, &route.ActionType, &route.TransportDomain, &route.FQDN, &route.Service); err != nil {
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
	rows, err := q.QueryContext(ctx, `SELECT prefix, peer, action_type, transport_domain, fqdn, service FROM imsi_prefix_routes ORDER BY length(prefix) DESC, prefix`)
	if err != nil {
		return nil, fmt.Errorf("load IMSI prefix routes: %w", err)
	}
	defer rows.Close()

	var routes []IMSIPrefixRoute
	for rows.Next() {
		var route IMSIPrefixRoute
		if err := rows.Scan(&route.Prefix, &route.Peer, &route.ActionType, &route.TransportDomain, &route.FQDN, &route.Service); err != nil {
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
	rows, err := q.QueryContext(ctx, `SELECT plmn, peer, action_type, transport_domain, fqdn, service FROM plmn_routes ORDER BY plmn`)
	if err != nil {
		return nil, fmt.Errorf("load PLMN routes: %w", err)
	}
	defer rows.Close()

	var routes []PLMNRoute
	for rows.Next() {
		var route PLMNRoute
		if err := rows.Scan(&route.PLMN, &route.Peer, &route.ActionType, &route.TransportDomain, &route.FQDN, &route.Service); err != nil {
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
