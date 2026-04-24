package config

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
)

type Manager struct {
	path      string
	mu        sync.RWMutex
	bootstrap Config
	runtime   Config
	store     *sqliteStore
}

type AuditEntry struct {
	ID         int64  `json:"id"`
	Action     string `json:"action"`
	ObjectType string `json:"object_type"`
	ObjectKey  string `json:"object_key"`
	BeforeJSON string `json:"before_json,omitempty"`
	AfterJSON  string `json:"after_json,omitempty"`
	CreatedAt  string `json:"created_at"`
}

func LoadManager(path string) (*Manager, error) {
	bootstrap, err := Load(path)
	if err != nil {
		return nil, err
	}
	bootstrap = bootstrap.BootstrapOnly()

	store, err := openSQLiteStore(bootstrap.Database)
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		path:      path,
		bootstrap: bootstrap,
		store:     store,
	}
	if err := manager.reloadLocked(context.Background(), nil); err != nil {
		_ = store.Close()
		return nil, err
	}
	return manager, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.store == nil {
		return nil
	}
	err := m.store.Close()
	m.store = nil
	return err
}

func (m *Manager) Path() string {
	return m.path
}

func (m *Manager) Snapshot() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.runtime.Clone()
}

func (m *Manager) UpsertPeer(peer PeerConfig) (Config, error) {
	return m.mutate("upsert", "peer", peer.Name, findPeer(m.Snapshot(), peer.Name), func(ctx context.Context, tx *sql.Tx) error {
		return m.store.upsertPeer(ctx, tx, peer)
	}, func(cfg Config) any {
		return findPeer(cfg, peer.Name)
	})
}

func (m *Manager) UpsertTransportDomain(domain TransportDomainConfig) (Config, error) {
	return m.mutate("upsert", "transport_domain", domain.Name, findTransportDomain(m.Snapshot(), domain.Name), func(ctx context.Context, tx *sql.Tx) error {
		return m.store.upsertTransportDomain(ctx, tx, domain)
	}, func(cfg Config) any {
		return findTransportDomain(cfg, domain.Name)
	})
}

func (m *Manager) DeleteTransportDomain(name string) (Config, error) {
	return m.mutate("delete", "transport_domain", name, findTransportDomain(m.Snapshot(), name), func(ctx context.Context, tx *sql.Tx) error {
		return m.store.deleteTransportDomain(ctx, tx, name)
	}, func(cfg Config) any {
		return nil
	})
}

func (m *Manager) UpsertDNSResolver(resolver DNSResolverConfig) (Config, error) {
	return m.mutate("upsert", "dns_resolver", resolver.Name, findDNSResolver(m.Snapshot(), resolver.Name), func(ctx context.Context, tx *sql.Tx) error {
		return m.store.upsertDNSResolver(ctx, tx, resolver)
	}, func(cfg Config) any {
		return findDNSResolver(cfg, resolver.Name)
	})
}

func (m *Manager) DeleteDNSResolver(name string) (Config, error) {
	return m.mutate("delete", "dns_resolver", name, findDNSResolver(m.Snapshot(), name), func(ctx context.Context, tx *sql.Tx) error {
		return m.store.deleteDNSResolver(ctx, tx, name)
	}, func(cfg Config) any {
		return nil
	})
}

func (m *Manager) DeletePeer(name string) (Config, error) {
	return m.mutate("delete", "peer", name, findPeer(m.Snapshot(), name), func(ctx context.Context, tx *sql.Tx) error {
		return m.store.deletePeer(ctx, tx, name)
	}, func(cfg Config) any {
		return nil
	})
}

func (m *Manager) UpsertAPNRoute(route APNRoute) (Config, error) {
	return m.mutate("upsert", "apn_route", route.APN, findAPNRoute(m.Snapshot(), route.APN), func(ctx context.Context, tx *sql.Tx) error {
		return m.store.upsertAPNRoute(ctx, tx, route)
	}, func(cfg Config) any {
		return findAPNRoute(cfg, route.APN)
	})
}

func (m *Manager) DeleteAPNRoute(apn string) (Config, error) {
	return m.mutate("delete", "apn_route", apn, findAPNRoute(m.Snapshot(), apn), func(ctx context.Context, tx *sql.Tx) error {
		return m.store.deleteAPNRoute(ctx, tx, apn)
	}, func(cfg Config) any {
		return nil
	})
}

func (m *Manager) UpsertIMSIRoute(route IMSIRoute) (Config, error) {
	return m.mutate("upsert", "imsi_route", route.IMSI, findIMSIRoute(m.Snapshot(), route.IMSI), func(ctx context.Context, tx *sql.Tx) error {
		return m.store.upsertIMSIRoute(ctx, tx, route)
	}, func(cfg Config) any {
		return findIMSIRoute(cfg, route.IMSI)
	})
}

func (m *Manager) DeleteIMSIRoute(imsi string) (Config, error) {
	return m.mutate("delete", "imsi_route", imsi, findIMSIRoute(m.Snapshot(), imsi), func(ctx context.Context, tx *sql.Tx) error {
		return m.store.deleteIMSIRoute(ctx, tx, imsi)
	}, func(cfg Config) any {
		return nil
	})
}

func (m *Manager) UpsertIMSIPrefixRoute(route IMSIPrefixRoute) (Config, error) {
	return m.mutate("upsert", "imsi_prefix_route", route.Prefix, findIMSIPrefixRoute(m.Snapshot(), route.Prefix), func(ctx context.Context, tx *sql.Tx) error {
		return m.store.upsertIMSIPrefixRoute(ctx, tx, route)
	}, func(cfg Config) any {
		return findIMSIPrefixRoute(cfg, route.Prefix)
	})
}

func (m *Manager) DeleteIMSIPrefixRoute(prefix string) (Config, error) {
	return m.mutate("delete", "imsi_prefix_route", prefix, findIMSIPrefixRoute(m.Snapshot(), prefix), func(ctx context.Context, tx *sql.Tx) error {
		return m.store.deleteIMSIPrefixRoute(ctx, tx, prefix)
	}, func(cfg Config) any {
		return nil
	})
}

func (m *Manager) UpsertPLMNRoute(route PLMNRoute) (Config, error) {
	return m.mutate("upsert", "plmn_route", route.PLMN, findPLMNRoute(m.Snapshot(), route.PLMN), func(ctx context.Context, tx *sql.Tx) error {
		return m.store.upsertPLMNRoute(ctx, tx, route)
	}, func(cfg Config) any {
		return findPLMNRoute(cfg, route.PLMN)
	})
}

func (m *Manager) DeletePLMNRoute(plmn string) (Config, error) {
	return m.mutate("delete", "plmn_route", plmn, findPLMNRoute(m.Snapshot(), plmn), func(ctx context.Context, tx *sql.Tx) error {
		return m.store.deletePLMNRoute(ctx, tx, plmn)
	}, func(cfg Config) any {
		return nil
	})
}

func (m *Manager) SetDefaultPeer(name string) (Config, error) {
	return m.mutate("set", "routing_default_peer", "default_peer", m.Snapshot().Routing.DefaultPeer, func(ctx context.Context, tx *sql.Tx) error {
		return m.store.setDefaultPeer(ctx, tx, name)
	}, func(cfg Config) any {
		return cfg.Routing.DefaultPeer
	})
}

func (m *Manager) ListAudit(limit int) ([]AuditEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.store.listAudit(context.Background(), limit)
}

func (m *Manager) mutate(action, objectType, objectKey string, before any, apply func(context.Context, *sql.Tx) error, after func(Config) any) (Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx := context.Background()
	tx, err := m.store.db.BeginTx(ctx, nil)
	if err != nil {
		return Config{}, fmt.Errorf("begin config transaction: %w", err)
	}

	if err := apply(ctx, tx); err != nil {
		_ = tx.Rollback()
		return Config{}, err
	}
	if err := m.reloadLocked(ctx, tx); err != nil {
		_ = tx.Rollback()
		return Config{}, err
	}
	entry := AuditEntry{
		Action:     action,
		ObjectType: objectType,
		ObjectKey:  objectKey,
		BeforeJSON: marshalAuditValue(before),
		AfterJSON:  marshalAuditValue(after(m.runtime)),
	}
	if err := m.store.insertAudit(ctx, tx, entry); err != nil {
		_ = tx.Rollback()
		return Config{}, err
	}
	if err := tx.Commit(); err != nil {
		return Config{}, fmt.Errorf("commit config transaction: %w", err)
	}
	return m.runtime.Clone(), nil
}

func (m *Manager) reloadLocked(ctx context.Context, tx *sql.Tx) error {
	mutable, err := m.store.load(ctx, tx)
	if err != nil {
		return err
	}
	runtime := m.bootstrap.Clone()
	runtime.TransportDomains = mutable.TransportDomains
	runtime.DNSResolvers = mutable.DNSResolvers
	runtime.Peers = mutable.Peers
	runtime.Routing = mutable.Routing
	runtime.ApplyDefaults()
	if err := runtime.ValidateRuntime(); err != nil {
		return err
	}
	m.runtime = runtime
	return nil
}

func marshalAuditValue(value any) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func findPeer(cfg Config, name string) *PeerConfig {
	for i := range cfg.Peers {
		if cfg.Peers[i].Name == name {
			peer := cfg.Peers[i]
			return &peer
		}
	}
	return nil
}

func findTransportDomain(cfg Config, name string) *TransportDomainConfig {
	for i := range cfg.TransportDomains {
		if cfg.TransportDomains[i].Name == name {
			domain := cfg.TransportDomains[i]
			return &domain
		}
	}
	return nil
}

func findDNSResolver(cfg Config, name string) *DNSResolverConfig {
	for i := range cfg.DNSResolvers {
		if cfg.DNSResolvers[i].Name == name {
			resolver := cfg.DNSResolvers[i]
			return &resolver
		}
	}
	return nil
}

func findAPNRoute(cfg Config, apn string) *APNRoute {
	key := normalizeAPN(apn)
	for i := range cfg.Routing.APNRoutes {
		if normalizeAPN(cfg.Routing.APNRoutes[i].APN) == key {
			route := cfg.Routing.APNRoutes[i]
			return &route
		}
	}
	return nil
}

func findIMSIRoute(cfg Config, imsi string) *IMSIRoute {
	key := normalizeDigits(imsi)
	for i := range cfg.Routing.IMSIRoutes {
		if normalizeDigits(cfg.Routing.IMSIRoutes[i].IMSI) == key {
			route := cfg.Routing.IMSIRoutes[i]
			return &route
		}
	}
	return nil
}

func findIMSIPrefixRoute(cfg Config, prefix string) *IMSIPrefixRoute {
	key := normalizeDigits(prefix)
	for i := range cfg.Routing.IMSIPrefixRoutes {
		if normalizeDigits(cfg.Routing.IMSIPrefixRoutes[i].Prefix) == key {
			route := cfg.Routing.IMSIPrefixRoutes[i]
			return &route
		}
	}
	return nil
}

func findPLMNRoute(cfg Config, plmn string) *PLMNRoute {
	key := normalizeDigits(plmn)
	for i := range cfg.Routing.PLMNRoutes {
		if normalizeDigits(cfg.Routing.PLMNRoutes[i].PLMN) == key {
			route := cfg.Routing.PLMNRoutes[i]
			return &route
		}
	}
	return nil
}
