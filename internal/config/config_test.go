package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAppliesDefaultsAndValidates(t *testing.T) {
	data := []byte(`
database:
  path: ./runtime.db
`)

	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.API.Listen != "0.0.0.0:8080" {
		t.Fatalf("unexpected API listen default %q", cfg.API.Listen)
	}
	if cfg.Log.Level != "info" {
		t.Fatalf("unexpected log level default %q", cfg.Log.Level)
	}
	if cfg.Log.File != "" {
		t.Fatalf("unexpected log file default %q", cfg.Log.File)
	}
	if cfg.Database.Path != "./runtime.db" {
		t.Fatalf("unexpected database path default %q", cfg.Database.Path)
	}
	if cfg.Proxy.Timeouts.SessionIdleDuration() <= 0 {
		t.Fatal("expected session idle timeout default")
	}
}

func TestValidateRuntimeRejectsInvalidDefaultPeer(t *testing.T) {
	cfg := Config{
		Proxy: ProxyConfig{
			GTPC:     GTPCConfig{Listen: "0.0.0.0:2123", AdvertiseAddress: "127.0.0.1"},
			GTPU:     GTPUConfig{Listen: "0.0.0.0:2152", AdvertiseAddress: "127.0.0.1"},
			Timeouts: TimeoutsConfig{SessionIdle: "15m", CleanupInterval: "30s"},
		},
		API:      APIConfig{Listen: "0.0.0.0:8080"},
		Log:      LogConfig{Level: "info"},
		Database: DatabaseConfig{Path: filepath.Join(t.TempDir(), "config.db")},
		Peers: []PeerConfig{
			{Name: "pgw-a", Address: "127.0.0.1:3123", Enabled: true},
		},
		Routing: RoutingConfig{
			DefaultPeer: "missing",
		},
	}

	if err := cfg.ValidateRuntime(); err == nil {
		t.Fatal("Validate() expected error, got nil")
	}
}

func TestValidateBootstrapAcceptsIPv6Addresses(t *testing.T) {
	cfg := Config{
		Proxy: ProxyConfig{
			Timeouts: TimeoutsConfig{SessionIdle: "15m", CleanupInterval: "30s"},
		},
		API:      APIConfig{Listen: "[::1]:8080"},
		Log:      LogConfig{Level: "info"},
		Database: DatabaseConfig{Path: filepath.Join(t.TempDir(), "config.db")},
	}

	if err := cfg.ValidateBootstrap(); err != nil {
		t.Fatalf("Validate() IPv6 error = %v", err)
	}
}

func TestValidateBootstrapRejectsLogFileDirectory(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Proxy: ProxyConfig{
			Timeouts: TimeoutsConfig{SessionIdle: "15m", CleanupInterval: "30s"},
		},
		API:      APIConfig{Listen: "127.0.0.1:8080"},
		Log:      LogConfig{Level: "info", File: dir},
		Database: DatabaseConfig{Path: filepath.Join(dir, "config.db")},
	}

	if err := cfg.ValidateBootstrap(); err == nil {
		t.Fatal("ValidateBootstrap() expected error for directory log.file, got nil")
	}
}

func TestEffectiveTransportConfigsUsePrimaryTransportDomain(t *testing.T) {
	cfg := Config{
		Proxy: ProxyConfig{
			Timeouts: TimeoutsConfig{SessionIdle: "15m", CleanupInterval: "30s"},
		},
		API:      APIConfig{Listen: "0.0.0.0:8080"},
		Log:      LogConfig{Level: "info"},
		Database: DatabaseConfig{Path: filepath.Join(t.TempDir(), "config.db")},
		TransportDomains: []TransportDomainConfig{
			{
				Name:              "home-a",
				NetNSPath:         "/var/run/netns/home-a",
				Enabled:           true,
				GTPCListenHost:    "192.0.2.10",
				GTPCPort:          2123,
				GTPUListenHost:    "192.0.2.10",
				GTPUPort:          2152,
				GTPCAdvertiseIPv4: "192.0.2.10",
				GTPUAdvertiseIPv4: "192.0.2.20",
			},
		},
	}

	gtpc, ok := cfg.EffectiveGTPCConfig()
	if !ok || gtpc.Listen != "192.0.2.10:2123" || gtpc.AdvertiseAddressIPv4 != "192.0.2.10" {
		t.Fatalf("unexpected effective GTPC config %+v ok=%v", gtpc, ok)
	}
	gtpu, ok := cfg.EffectiveGTPUConfig()
	if !ok || gtpu.Listen != "192.0.2.10:2152" || gtpu.AdvertiseAddressIPv4 != "192.0.2.20" {
		t.Fatalf("unexpected effective GTPU config %+v ok=%v", gtpu, ok)
	}
}

func TestValidateRuntimeRejectsDNSResolverWithUnknownTransportDomain(t *testing.T) {
	cfg := Config{
		Proxy: ProxyConfig{
			GTPC:     GTPCConfig{Listen: "0.0.0.0:2123", AdvertiseAddress: "127.0.0.1"},
			GTPU:     GTPUConfig{Listen: "0.0.0.0:2152", AdvertiseAddress: "127.0.0.1"},
			Timeouts: TimeoutsConfig{SessionIdle: "15m", CleanupInterval: "30s"},
		},
		API:      APIConfig{Listen: "0.0.0.0:8080"},
		Log:      LogConfig{Level: "info"},
		Database: DatabaseConfig{Path: filepath.Join(t.TempDir(), "config.db")},
		DNSResolvers: []DNSResolverConfig{
			{
				Name:            "home-dns",
				TransportDomain: "missing",
				Server:          "127.0.0.1:53",
				Priority:        100,
				TimeoutMS:       2000,
				Attempts:        2,
				Enabled:         true,
			},
		},
	}

	if err := cfg.ValidateRuntime(); err == nil {
		t.Fatal("ValidateRuntime() expected error, got nil")
	}
}

func TestValidateRuntimeRejectsDNSDiscoveryRouteWithoutFQDN(t *testing.T) {
	cfg := Config{
		Proxy: ProxyConfig{
			Timeouts: TimeoutsConfig{SessionIdle: "15m", CleanupInterval: "30s"},
		},
		API:      APIConfig{Listen: "0.0.0.0:8080"},
		Log:      LogConfig{Level: "info"},
		Database: DatabaseConfig{Path: filepath.Join(t.TempDir(), "config.db")},
		TransportDomains: []TransportDomainConfig{
			{
				Name:              "home-a",
				NetNSPath:         "/var/run/netns/home-a",
				Enabled:           true,
				GTPCListenHost:    "192.0.2.10",
				GTPCPort:          2123,
				GTPUListenHost:    "192.0.2.10",
				GTPUPort:          2152,
				GTPCAdvertiseIPv4: "192.0.2.10",
				GTPUAdvertiseIPv4: "192.0.2.10",
			},
		},
		Routing: RoutingConfig{
			APNRoutes: []APNRoute{
				{
					APN:             "ims",
					ActionType:      "dns_discovery",
					TransportDomain: "home-a",
				},
			},
		},
	}

	if err := cfg.ValidateRuntime(); err == nil {
		t.Fatal("ValidateRuntime() expected error, got nil")
	}
}

func TestManagerPersistsMutableConfigToSQLiteWithoutRewritingYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	dbPath := filepath.Join(dir, "runtime.db")
	initialYAML := strings.TrimSpace(`
proxy:
  gtpc:
    advertise_address_ipv4: 127.0.0.1
database:
  path: `+dbPath+`
`) + "\n"
	if err := os.WriteFile(cfgPath, []byte(initialYAML), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	manager, err := LoadManager(cfgPath)
	if err != nil {
		t.Fatalf("LoadManager() error = %v", err)
	}
	defer manager.Close()

	if got := manager.Snapshot(); len(got.Peers) != 0 || got.Routing.DefaultPeer != "" {
		t.Fatalf("unexpected initial runtime snapshot: %+v", got)
	}

	if _, err := manager.UpsertPeer(PeerConfig{Name: "pgw-a", Address: "127.0.0.1:3123", Enabled: true}); err != nil {
		t.Fatalf("UpsertPeer() error = %v", err)
	}
	if _, err := manager.SetDefaultPeer("pgw-a"); err != nil {
		t.Fatalf("SetDefaultPeer() error = %v", err)
	}
	if _, err := manager.UpsertAPNRoute(APNRoute{APN: "internet", Peer: "pgw-a"}); err != nil {
		t.Fatalf("UpsertAPNRoute() error = %v", err)
	}

	snapshot := manager.Snapshot()
	if len(snapshot.Peers) != 1 || snapshot.Routing.DefaultPeer != "pgw-a" || len(snapshot.Routing.APNRoutes) != 1 {
		t.Fatalf("unexpected runtime snapshot after updates: %+v", snapshot)
	}

	manager2, err := LoadManager(cfgPath)
	if err != nil {
		t.Fatalf("LoadManager() reload error = %v", err)
	}
	defer manager2.Close()

	reloaded := manager2.Snapshot()
	if len(reloaded.Peers) != 1 || reloaded.Routing.DefaultPeer != "pgw-a" || len(reloaded.Routing.APNRoutes) != 1 {
		t.Fatalf("unexpected reloaded runtime snapshot: %+v", reloaded)
	}

	afterYAML, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(afterYAML) != initialYAML {
		t.Fatalf("expected bootstrap YAML to remain unchanged\ngot:\n%s\nwant:\n%s", string(afterYAML), initialYAML)
	}
}

func TestLoadManagerAllowsMinimalBootstrapWithEmptyMutableRuntime(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	dbPath := filepath.Join(dir, "runtime.db")
	initialYAML := strings.TrimSpace(`
database:
  path: `+dbPath+`
`) + "\n"
	if err := os.WriteFile(cfgPath, []byte(initialYAML), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	manager, err := LoadManager(cfgPath)
	if err != nil {
		t.Fatalf("LoadManager() error = %v", err)
	}
	defer manager.Close()

	snapshot := manager.Snapshot()
	if len(snapshot.TransportDomains) != 0 || len(snapshot.DNSResolvers) != 0 || len(snapshot.Peers) != 0 {
		t.Fatalf("unexpected non-empty runtime snapshot: %+v", snapshot)
	}
}

func TestManagerRollsBackInvalidPeerDelete(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	dbPath := filepath.Join(dir, "runtime.db")
	yaml := strings.TrimSpace(`
proxy:
  gtpc:
    advertise_address_ipv4: 127.0.0.1
database:
  path: `+dbPath+`
`) + "\n"
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	manager, err := LoadManager(cfgPath)
	if err != nil {
		t.Fatalf("LoadManager() error = %v", err)
	}
	defer manager.Close()

	if _, err := manager.UpsertPeer(PeerConfig{Name: "pgw-a", Address: "127.0.0.1:3123", Enabled: true}); err != nil {
		t.Fatalf("UpsertPeer() error = %v", err)
	}
	if _, err := manager.SetDefaultPeer("pgw-a"); err != nil {
		t.Fatalf("SetDefaultPeer() error = %v", err)
	}

	if _, err := manager.DeletePeer("pgw-a"); err == nil {
		t.Fatal("DeletePeer() expected error, got nil")
	}

	snapshot := manager.Snapshot()
	if len(snapshot.Peers) != 1 || snapshot.Routing.DefaultPeer != "pgw-a" {
		t.Fatalf("unexpected runtime snapshot after rollback: %+v", snapshot)
	}
}

func TestManagerPersistsTransportDomainsAndResolversToSQLiteWithoutRewritingYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	dbPath := filepath.Join(dir, "runtime.db")
	initialYAML := strings.TrimSpace(`
proxy:
  gtpc:
    advertise_address_ipv4: 127.0.0.1
database:
  path: `+dbPath+`
`) + "\n"
	if err := os.WriteFile(cfgPath, []byte(initialYAML), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	manager, err := LoadManager(cfgPath)
	if err != nil {
		t.Fatalf("LoadManager() error = %v", err)
	}
	defer manager.Close()

	if _, err := manager.UpsertTransportDomain(TransportDomainConfig{
		Name:              "home-a",
		NetNSPath:         "/var/run/netns/home-a",
		Enabled:           true,
		GTPCListenHost:    "192.0.2.10",
		GTPCPort:          2123,
		GTPUListenHost:    "192.0.2.10",
		GTPUPort:          2152,
		GTPCAdvertiseIPv4: "192.0.2.10",
		GTPUAdvertiseIPv4: "192.0.2.10",
	}); err != nil {
		t.Fatalf("UpsertTransportDomain() error = %v", err)
	}
	if _, err := manager.UpsertDNSResolver(DNSResolverConfig{
		Name:            "home-a-primary",
		TransportDomain: "home-a",
		Server:          "192.0.2.53:53",
		Priority:        10,
		TimeoutMS:       1500,
		Attempts:        2,
		SearchDomain:    "epc.mnc001.mcc001.3gppnetwork.org",
		Enabled:         true,
	}); err != nil {
		t.Fatalf("UpsertDNSResolver() error = %v", err)
	}
	if _, err := manager.UpsertPeer(PeerConfig{
		Name:            "pgw-a",
		Address:         "192.0.2.20:2123",
		TransportDomain: "home-a",
		Enabled:         true,
	}); err != nil {
		t.Fatalf("UpsertPeer() error = %v", err)
	}

	snapshot := manager.Snapshot()
	if len(snapshot.TransportDomains) != 1 || len(snapshot.DNSResolvers) != 1 || len(snapshot.Peers) != 1 {
		t.Fatalf("unexpected runtime snapshot after transport updates: %+v", snapshot)
	}
	if snapshot.Peers[0].TransportDomain != "home-a" {
		t.Fatalf("unexpected peer transport domain %+v", snapshot.Peers[0])
	}

	manager2, err := LoadManager(cfgPath)
	if err != nil {
		t.Fatalf("LoadManager() reload error = %v", err)
	}
	defer manager2.Close()

	reloaded := manager2.Snapshot()
	if len(reloaded.TransportDomains) != 1 || len(reloaded.DNSResolvers) != 1 || len(reloaded.Peers) != 1 {
		t.Fatalf("unexpected reloaded runtime snapshot: %+v", reloaded)
	}
	if reloaded.DNSResolvers[0].TransportDomain != "home-a" {
		t.Fatalf("unexpected reloaded resolver %+v", reloaded.DNSResolvers[0])
	}

	afterYAML, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(afterYAML) != initialYAML {
		t.Fatalf("expected bootstrap YAML to remain unchanged\ngot:\n%s\nwant:\n%s", string(afterYAML), initialYAML)
	}
}

func TestManagerPersistsDNSDiscoveryRouteFields(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	dbPath := filepath.Join(dir, "runtime.db")
	initialYAML := strings.TrimSpace(`
database:
  path: `+dbPath+`
`) + "\n"
	if err := os.WriteFile(cfgPath, []byte(initialYAML), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	manager, err := LoadManager(cfgPath)
	if err != nil {
		t.Fatalf("LoadManager() error = %v", err)
	}
	defer manager.Close()

	if _, err := manager.UpsertTransportDomain(TransportDomainConfig{
		Name:              "home-a",
		NetNSPath:         "/var/run/netns/home-a",
		Enabled:           true,
		GTPCListenHost:    "192.0.2.10",
		GTPCPort:          2123,
		GTPUListenHost:    "192.0.2.10",
		GTPUPort:          2152,
		GTPCAdvertiseIPv4: "192.0.2.10",
		GTPUAdvertiseIPv4: "192.0.2.20",
	}); err != nil {
		t.Fatalf("UpsertTransportDomain() error = %v", err)
	}
	if _, err := manager.UpsertDNSResolver(DNSResolverConfig{
		Name:            "home-a-primary",
		TransportDomain: "home-a",
		Server:          "192.0.2.53:53",
		Priority:        10,
		TimeoutMS:       1500,
		Attempts:        2,
		SearchDomain:    "epc.example.net",
		Enabled:         true,
	}); err != nil {
		t.Fatalf("UpsertDNSResolver() error = %v", err)
	}
	if _, err := manager.UpsertAPNRoute(APNRoute{
		APN:             "ims",
		ActionType:      "dns_discovery",
		TransportDomain: "home-a",
		FQDN:            "topon.s8.pgw.epc.example.net",
		Service:         "x-3gpp-pgw",
	}); err != nil {
		t.Fatalf("UpsertAPNRoute() error = %v", err)
	}

	manager2, err := LoadManager(cfgPath)
	if err != nil {
		t.Fatalf("LoadManager() reload error = %v", err)
	}
	defer manager2.Close()

	reloaded := manager2.Snapshot()
	if len(reloaded.Routing.APNRoutes) != 1 {
		t.Fatalf("unexpected APN route count: %+v", reloaded.Routing.APNRoutes)
	}
	route := reloaded.Routing.APNRoutes[0]
	if route.ActionType != "dns_discovery" || route.TransportDomain != "home-a" || route.FQDN != "topon.s8.pgw.epc.example.net" || route.Service != "x-3gpp-pgw" {
		t.Fatalf("unexpected reloaded DNS discovery route: %+v", route)
	}
}

func TestManagerWritesAuditHistoryForMutableConfigChanges(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	dbPath := filepath.Join(dir, "runtime.db")
	yaml := strings.TrimSpace(`
proxy:
  gtpc:
    advertise_address_ipv4: 127.0.0.1
database:
  path: `+dbPath+`
`) + "\n"
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	manager, err := LoadManager(cfgPath)
	if err != nil {
		t.Fatalf("LoadManager() error = %v", err)
	}
	defer manager.Close()

	if _, err := manager.UpsertPeer(PeerConfig{Name: "pgw-a", Address: "127.0.0.1:3123", Enabled: true}); err != nil {
		t.Fatalf("UpsertPeer() error = %v", err)
	}
	if _, err := manager.SetDefaultPeer("pgw-a"); err != nil {
		t.Fatalf("SetDefaultPeer() error = %v", err)
	}
	if _, err := manager.UpsertAPNRoute(APNRoute{APN: "internet", Peer: "pgw-a"}); err != nil {
		t.Fatalf("UpsertAPNRoute() error = %v", err)
	}

	entries, err := manager.ListAudit(10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("unexpected audit entry count %d", len(entries))
	}
	if entries[0].ObjectType != "apn_route" || entries[0].Action != "upsert" {
		t.Fatalf("unexpected latest audit entry %+v", entries[0])
	}
	if entries[1].ObjectType != "routing_default_peer" || entries[1].AfterJSON != `"pgw-a"` {
		t.Fatalf("unexpected default-peer audit entry %+v", entries[1])
	}
	if entries[2].ObjectType != "peer" || entries[2].ObjectKey != "pgw-a" {
		t.Fatalf("unexpected peer audit entry %+v", entries[2])
	}
}

func TestManagerPrunesAuditHistoryToMostRecent100Entries(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	dbPath := filepath.Join(dir, "runtime.db")
	yaml := strings.TrimSpace(`
proxy:
  gtpc:
    advertise_address_ipv4: 127.0.0.1
database:
  path: `+dbPath+`
`) + "\n"
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	manager, err := LoadManager(cfgPath)
	if err != nil {
		t.Fatalf("LoadManager() error = %v", err)
	}
	defer manager.Close()

	for i := 0; i < 105; i++ {
		if _, err := manager.UpsertPeer(PeerConfig{
			Name:        "pgw-a",
			Address:     "127.0.0.1:3123",
			Enabled:     true,
			Description: fmt.Sprintf("peer-%03d", i),
		}); err != nil {
			t.Fatalf("UpsertPeer(%d) error = %v", i, err)
		}
	}

	entries, err := manager.ListAudit(200)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(entries) != 100 {
		t.Fatalf("unexpected audit entry count %d", len(entries))
	}
	if !strings.Contains(entries[0].AfterJSON, "peer-104") {
		t.Fatalf("unexpected newest audit entry %+v", entries[0])
	}
	if !strings.Contains(entries[len(entries)-1].AfterJSON, "peer-005") {
		t.Fatalf("unexpected oldest retained audit entry %+v", entries[len(entries)-1])
	}
}
