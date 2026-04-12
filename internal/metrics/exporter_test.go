package metrics

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vectorcore/gtp_proxy/internal/session"
)

func TestHandlerExposesPrometheusMetrics(t *testing.T) {
	registry := New()
	table := session.NewTable()
	visited := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 2123}
	table.Create(visited, "198.51.100.10:2123", "001010123456789", "internet", "apn", "internet", "pgw", 1111, time.Minute)

	registry.IncSessionCreate()
	registry.IncSessionDelete()
	registry.AddSessionTimeoutDeletes(2)
	registry.IncGTPUForwardHit()
	registry.IncGTPUForwardMiss()
	registry.IncGTPUPacketsForwarded()
	registry.IncUnknownTEIDDrop()
	registry.IncPeerControlPlanePacket("pgw")
	registry.IncPeerUserPlanePacket("pgw")
	registry.IncMessageError("gtpc", "create_session_request")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	Handler(registry, table).ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	text := string(body)
	for _, needle := range []string{
		"# TYPE gtp_proxy_active_sessions gauge",
		"gtp_proxy_active_sessions 1",
		"gtp_proxy_session_creates_total 1",
		"gtp_proxy_session_timeout_deletes_total 2",
		"gtp_proxy_gtpu_forward_hits_total 1",
		"gtp_proxy_gtpu_forward_misses_total 1",
		"gtp_proxy_gtpu_packets_forwarded_total 1",
		"gtp_proxy_unknown_teid_drops_total 1",
		`gtp_proxy_peer_control_plane_packets_total{peer="pgw"} 1`,
		`gtp_proxy_peer_user_plane_packets_total{peer="pgw"} 1`,
		`gtp_proxy_message_errors_total{protocol="gtpc",message="create_session_request"} 1`,
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("metrics output missing %q:\n%s", needle, text)
		}
	}
}
