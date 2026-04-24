package transport

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vectorcore/gtp_proxy/internal/config"
	"github.com/vectorcore/gtp_proxy/internal/session"
)

func TestDomainDiagnosticsReflectsRuntimeAndSessions(t *testing.T) {
	netnsPath := filepath.Join(t.TempDir(), "side_a")
	if err := os.WriteFile(netnsPath, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg := config.Config{
		TransportDomains: []config.TransportDomainConfig{
			{
				Name:              "side_a",
				Description:       "Side A",
				NetNSPath:         netnsPath,
				Enabled:           true,
				GTPCListenHost:    "192.0.2.10",
				GTPCPort:          2123,
				GTPUListenHost:    "192.0.2.20",
				GTPUPort:          2152,
				GTPCAdvertiseIPv4: "192.0.2.10",
				GTPUAdvertiseIPv4: "192.0.2.20",
			},
		},
	}

	table := session.NewTable()
	visited := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 2123}
	table.Create(visited, "198.51.100.10:2123", "001010123456789", "ims", "apn", "ims", "pgw-a", "dns_discovery", "visited-a", "side_a", "topon.s8.pgw.epc.example.net", "dns_naptr_srv", 1111, time.Minute)

	runtime := NewRuntime()
	runtime.SetGTPC(ListenerStatus{Protocol: "gtpc", State: "active", Domain: "side_a", Listen: "192.0.2.10:2123"})
	runtime.SetGTPU(ListenerStatus{Protocol: "gtpu", State: "active", Domain: "side_a", Listen: "192.0.2.20:2152"})

	diagnostics := DomainDiagnostics(cfg, table.List(), runtime)
	if len(diagnostics) != 1 {
		t.Fatalf("unexpected diagnostics count %d", len(diagnostics))
	}
	if !diagnostics[0].Effective || !diagnostics[0].NamespacePresent || diagnostics[0].ActiveSessions != 1 {
		t.Fatalf("unexpected diagnostics %+v", diagnostics[0])
	}
	if diagnostics[0].GTPCSocketState != "active" || diagnostics[0].GTPUSocketState != "active" {
		t.Fatalf("unexpected socket states %+v", diagnostics[0])
	}
}
