package metrics

import (
	"maps"
	"sync"
	"sync/atomic"
)

type PeerCounters struct {
	ControlPlanePackets uint64 `json:"control_plane_packets"`
	UserPlanePackets    uint64 `json:"user_plane_packets"`
}

type Registry struct {
	sessionCreates        atomic.Uint64
	sessionDeletes        atomic.Uint64
	sessionTimeoutDeletes atomic.Uint64
	gtpuForwardHits       atomic.Uint64
	gtpuForwardMisses     atomic.Uint64
	gtpuPacketsForwarded  atomic.Uint64
	unknownTEIDDrops      atomic.Uint64

	mu            sync.RWMutex
	peerCounters  map[string]PeerCounters
	messageErrors map[string]uint64
}

type Snapshot struct {
	SessionCreates        uint64                  `json:"session_creates"`
	SessionDeletes        uint64                  `json:"session_deletes"`
	SessionTimeoutDeletes uint64                  `json:"session_timeout_deletes"`
	GTPUForwardHits       uint64                  `json:"gtpu_forward_hits"`
	GTPUForwardMisses     uint64                  `json:"gtpu_forward_misses"`
	GTPUPacketsForwarded  uint64                  `json:"gtpu_packets_forwarded"`
	UnknownTEIDDrops      uint64                  `json:"unknown_teid_drops"`
	PeerCounters          map[string]PeerCounters `json:"peer_counters"`
	MessageErrors         map[string]uint64       `json:"message_errors"`
}

func New() *Registry {
	return &Registry{
		peerCounters:  map[string]PeerCounters{},
		messageErrors: map[string]uint64{},
	}
}

func (r *Registry) IncSessionCreate() {
	r.sessionCreates.Add(1)
}

func (r *Registry) IncSessionDelete() {
	r.sessionDeletes.Add(1)
}

func (r *Registry) AddSessionTimeoutDeletes(n int) {
	if n > 0 {
		r.sessionTimeoutDeletes.Add(uint64(n))
	}
}

func (r *Registry) IncGTPUForwardHit() {
	r.gtpuForwardHits.Add(1)
}

func (r *Registry) IncGTPUForwardMiss() {
	r.gtpuForwardMisses.Add(1)
}

func (r *Registry) IncGTPUPacketsForwarded() {
	r.gtpuPacketsForwarded.Add(1)
}

func (r *Registry) IncUnknownTEIDDrop() {
	r.unknownTEIDDrops.Add(1)
}

func (r *Registry) IncPeerControlPlanePacket(peer string) {
	if peer == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	counters := r.peerCounters[peer]
	counters.ControlPlanePackets++
	r.peerCounters[peer] = counters
}

func (r *Registry) IncPeerUserPlanePacket(peer string) {
	if peer == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	counters := r.peerCounters[peer]
	counters.UserPlanePackets++
	r.peerCounters[peer] = counters
}

func (r *Registry) IncMessageError(protocol, message string) {
	if protocol == "" {
		protocol = "unknown"
	}
	if message == "" {
		message = "unknown"
	}
	key := protocol + ":" + message
	r.mu.Lock()
	r.messageErrors[key]++
	r.mu.Unlock()
}

func (r *Registry) Snapshot() Snapshot {
	r.mu.RLock()
	peerCounters := maps.Clone(r.peerCounters)
	messageErrors := maps.Clone(r.messageErrors)
	r.mu.RUnlock()

	return Snapshot{
		SessionCreates:        r.sessionCreates.Load(),
		SessionDeletes:        r.sessionDeletes.Load(),
		SessionTimeoutDeletes: r.sessionTimeoutDeletes.Load(),
		GTPUForwardHits:       r.gtpuForwardHits.Load(),
		GTPUForwardMisses:     r.gtpuForwardMisses.Load(),
		GTPUPacketsForwarded:  r.gtpuPacketsForwarded.Load(),
		UnknownTEIDDrops:      r.unknownTEIDDrops.Load(),
		PeerCounters:          peerCounters,
		MessageErrors:         messageErrors,
	}
}
