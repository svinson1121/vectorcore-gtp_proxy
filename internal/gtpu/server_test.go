package gtpu

import (
	"encoding/binary"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vectorcore/gtp_proxy/internal/config"
	"github.com/vectorcore/gtp_proxy/internal/metrics"
	"github.com/vectorcore/gtp_proxy/internal/session"
)

func TestHandlePacketForwardsUsingSessionState(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(`
proxy:
  gtpc:
    advertise_address_ipv4: 127.0.0.1
  gtpu:
    advertise_address_ipv4: 127.0.0.1
  timeouts:
    session_idle: 15m
    cleanup_interval: 30s
peers:
  - name: pgw
    address: 127.0.0.1:3123
    enabled: true
routing:
  default_peer: pgw
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	manager, err := config.LoadManager(cfgPath)
	if err != nil {
		t.Fatalf("LoadManager() error = %v", err)
	}

	table := session.NewTable()
	logger := slog.New(slog.NewTextHandler(ioDiscard{}, nil))
	registry := metrics.New()
	server := NewServer(manager, table, registry, logger)

	visited := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 2123}
	sess := table.Create(visited, "198.51.100.10:2123", "001010123456789", "internet", "apn", "internet", "pgw", 1111, time.Minute)

	destConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP(dest) error = %v", err)
	}
	defer destConn.Close()

	sess, err = table.UpsertHomeUserPlane(sess.ID, destConn.LocalAddr().String(), 0x12345678, time.Minute)
	if err != nil {
		t.Fatalf("UpsertHomeUserPlane() error = %v", err)
	}

	srcConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP(src) error = %v", err)
	}
	defer srcConn.Close()

	packet := make([]byte, 8)
	packet[0] = 0x30
	packet[1] = 0xff
	binary.BigEndian.PutUint16(packet[2:4], 8)
	binary.BigEndian.PutUint32(packet[4:8], sess.ProxyHomeUserTEID)

	if err := server.handlePacket(srcConn, visited, packet); err != nil {
		t.Fatalf("handlePacket() error = %v", err)
	}

	buf := make([]byte, 64)
	if err := destConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	n, _, err := destConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP() error = %v", err)
	}
	if got := binary.BigEndian.Uint32(buf[4:8]); got != 0x12345678 {
		t.Fatalf("unexpected forwarded TEID %x", got)
	}
	if n != len(packet) {
		t.Fatalf("unexpected forwarded packet length %d", n)
	}

	snapshot := registry.Snapshot()
	if snapshot.GTPUForwardHits != 1 || snapshot.GTPUPacketsForwarded != 1 {
		t.Fatalf("unexpected metrics snapshot %#v", snapshot)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
