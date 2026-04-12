package metrics

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/vectorcore/gtp_proxy/internal/session"
)

func Handler(registry *Registry, sessions *session.Table) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		stats := sessions.Stats()
		snapshot := registry.Snapshot()

		var b strings.Builder
		writeGauge(&b, "gtp_proxy_active_sessions", "Number of active sessions currently tracked by the proxy.", float64(stats.ActiveSessions))
		writeGauge(&b, "gtp_proxy_pending_transactions", "Number of in-flight control-plane transactions awaiting a response.", float64(stats.PendingTransactions))
		writeCounter(&b, "gtp_proxy_session_creates_total", "Total number of session create operations handled by the proxy.", snapshot.SessionCreates)
		writeCounter(&b, "gtp_proxy_session_deletes_total", "Total number of session delete operations handled by the proxy.", snapshot.SessionDeletes)
		writeCounter(&b, "gtp_proxy_session_timeout_deletes_total", "Total number of sessions removed by timeout cleanup.", snapshot.SessionTimeoutDeletes)
		writeCounter(&b, "gtp_proxy_gtpu_forward_hits_total", "Total number of GTP-U packets matched to known session state.", snapshot.GTPUForwardHits)
		writeCounter(&b, "gtp_proxy_gtpu_forward_misses_total", "Total number of GTP-U packets that missed session lookup.", snapshot.GTPUForwardMisses)
		writeCounter(&b, "gtp_proxy_gtpu_packets_forwarded_total", "Total number of GTP-U packets forwarded by the proxy.", snapshot.GTPUPacketsForwarded)
		writeCounter(&b, "gtp_proxy_unknown_teid_drops_total", "Total number of packets dropped because the TEID was unknown.", snapshot.UnknownTEIDDrops)
		writePeerCounters(&b, snapshot)
		writeMessageErrors(&b, snapshot)

		fmt.Fprint(w, b.String())
	})
}

func writeGauge(b *strings.Builder, name, help string, value float64) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s gauge\n", name)
	fmt.Fprintf(b, "%s %v\n", name, value)
}

func writeCounter(b *strings.Builder, name, help string, value uint64) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s counter\n", name)
	fmt.Fprintf(b, "%s %d\n", name, value)
}

func writePeerCounters(b *strings.Builder, snapshot Snapshot) {
	if len(snapshot.PeerCounters) == 0 {
		return
	}
	peers := make([]string, 0, len(snapshot.PeerCounters))
	for peer := range snapshot.PeerCounters {
		peers = append(peers, peer)
	}
	slices.Sort(peers)

	fmt.Fprintf(b, "# HELP %s %s\n", "gtp_proxy_peer_control_plane_packets_total", "Total number of control-plane packets forwarded to each configured peer.")
	fmt.Fprintf(b, "# TYPE %s counter\n", "gtp_proxy_peer_control_plane_packets_total")
	for _, peer := range peers {
		fmt.Fprintf(b, "gtp_proxy_peer_control_plane_packets_total{peer=%q} %d\n", peer, snapshot.PeerCounters[peer].ControlPlanePackets)
	}

	fmt.Fprintf(b, "# HELP %s %s\n", "gtp_proxy_peer_user_plane_packets_total", "Total number of user-plane packets forwarded for each configured peer.")
	fmt.Fprintf(b, "# TYPE %s counter\n", "gtp_proxy_peer_user_plane_packets_total")
	for _, peer := range peers {
		fmt.Fprintf(b, "gtp_proxy_peer_user_plane_packets_total{peer=%q} %d\n", peer, snapshot.PeerCounters[peer].UserPlanePackets)
	}
}

func writeMessageErrors(b *strings.Builder, snapshot Snapshot) {
	if len(snapshot.MessageErrors) == 0 {
		return
	}
	keys := make([]string, 0, len(snapshot.MessageErrors))
	for key := range snapshot.MessageErrors {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	fmt.Fprintf(b, "# HELP %s %s\n", "gtp_proxy_message_errors_total", "Total number of protocol/message errors observed by the proxy.")
	fmt.Fprintf(b, "# TYPE %s counter\n", "gtp_proxy_message_errors_total")
	for _, key := range keys {
		protocol, message, _ := strings.Cut(key, ":")
		fmt.Fprintf(b, "gtp_proxy_message_errors_total{protocol=%q,message=%q} %d\n", protocol, message, snapshot.MessageErrors[key])
	}
}
