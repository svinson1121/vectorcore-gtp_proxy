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
proxy:
  gtpc:
    advertise_address: 127.0.0.1
`)

	cfg, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.Proxy.GTPC.Listen != "0.0.0.0:2123" {
		t.Fatalf("unexpected GTPC listen default %q", cfg.Proxy.GTPC.Listen)
	}
	if cfg.Proxy.GTPU.Listen != "0.0.0.0:2152" {
		t.Fatalf("unexpected GTPU listen default %q", cfg.Proxy.GTPU.Listen)
	}
	if cfg.API.Listen != "0.0.0.0:8080" {
		t.Fatalf("unexpected API listen default %q", cfg.API.Listen)
	}
	if cfg.Log.Level != "info" {
		t.Fatalf("unexpected log level default %q", cfg.Log.Level)
	}
	if cfg.Database.Path != "./gtp_proxy.db" {
		t.Fatalf("unexpected database path default %q", cfg.Database.Path)
	}
	if cfg.Proxy.GTPC.AdvertiseAddressIPv4 != "127.0.0.1" {
		t.Fatalf("unexpected IPv4 advertise default %q", cfg.Proxy.GTPC.AdvertiseAddressIPv4)
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
			GTPC: GTPCConfig{
				Listen:               "[::]:2123",
				AdvertiseAddressIPv6: "2001:db8::10",
			},
			GTPU: GTPUConfig{
				Listen:               "[::]:2152",
				AdvertiseAddressIPv6: "2001:db8::11",
			},
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
