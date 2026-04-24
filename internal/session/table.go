package session

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Session struct {
	ID                      string    `json:"id"`
	IMSI                    string    `json:"imsi,omitempty"`
	APN                     string    `json:"apn,omitempty"`
	RouteMatchType          string    `json:"route_match_type,omitempty"`
	RouteMatchValue         string    `json:"route_match_value,omitempty"`
	RoutePeer               string    `json:"route_peer,omitempty"`
	RouteActionType         string    `json:"route_action_type,omitempty"`
	IngressTransportDomain  string    `json:"ingress_transport_domain,omitempty"`
	EgressTransportDomain   string    `json:"egress_transport_domain,omitempty"`
	DiscoveryFQDN           string    `json:"discovery_fqdn,omitempty"`
	DiscoveryMethod         string    `json:"discovery_method,omitempty"`
	VisitedControlEndpoint  string    `json:"visited_control_endpoint"`
	HomeControlEndpoint     string    `json:"home_control_endpoint"`
	VisitedUserEndpoint     string    `json:"visited_user_endpoint,omitempty"`
	HomeUserEndpoint        string    `json:"home_user_endpoint,omitempty"`
	VisitedControlTEID      uint32    `json:"visited_control_teid"`
	HomeControlTEID         uint32    `json:"home_control_teid"`
	VisitedUserTEID         uint32    `json:"visited_user_teid"`
	HomeUserTEID            uint32    `json:"home_user_teid"`
	ProxyVisitedControlTEID uint32    `json:"proxy_visited_control_teid"`
	ProxyHomeControlTEID    uint32    `json:"proxy_home_control_teid"`
	ProxyVisitedUserTEID    uint32    `json:"proxy_visited_user_teid"`
	ProxyHomeUserTEID       uint32    `json:"proxy_home_user_teid"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
	ExpiresAt               time.Time `json:"expires_at"`
}

type PendingTransaction struct {
	PeerDomain       string
	PeerAddr         string
	Sequence         uint32
	MessageType      uint8
	OriginalPeerDomain string
	OriginalPeerAddr string
	SessionID        string
	CreatedAt        time.Time
	ExpiresAt        time.Time
}

type Snapshot struct {
	ActiveSessions      int `json:"active_sessions"`
	PendingTransactions int `json:"pending_transactions"`
}

type Table struct {
	nextTEID atomic.Uint32

	mu                  sync.RWMutex
	byID                map[string]*Session
	byProxyVisitedTEID  map[uint32]*Session
	byProxyHomeTEID     map[uint32]*Session
	byProxyVisitedUser  map[uint32]*Session
	byProxyHomeUser     map[uint32]*Session
	pendingTransactions map[pendingKey]PendingTransaction
}

type pendingKey struct {
	peerDomain  string
	peerAddr    string
	sequence    uint32
	messageType uint8
}

func NewTable() *Table {
	t := &Table{
		byID:                make(map[string]*Session),
		byProxyVisitedTEID:  make(map[uint32]*Session),
		byProxyHomeTEID:     make(map[uint32]*Session),
		byProxyVisitedUser:  make(map[uint32]*Session),
		byProxyHomeUser:     make(map[uint32]*Session),
		pendingTransactions: make(map[pendingKey]PendingTransaction),
	}
	t.nextTEID.Store(0x10000000)
	return t
}

func (t *Table) AllocateTEID() uint32 {
	return t.nextTEID.Add(1)
}

func (t *Table) Create(visitedAddr *net.UDPAddr, homeAddr, imsi, apn, routeMatchType, routeMatchValue, routePeer, routeActionType, ingressTransportDomain, egressTransportDomain, discoveryFQDN, discoveryMethod string, visitedControlTEID uint32, sessionTTL time.Duration) *Session {
	now := time.Now().UTC()
	s := &Session{
		ID:                      fmt.Sprintf("%d-%d", now.UnixNano(), t.AllocateTEID()),
		IMSI:                    imsi,
		APN:                     apn,
		RouteMatchType:          routeMatchType,
		RouteMatchValue:         routeMatchValue,
		RoutePeer:               routePeer,
		RouteActionType:         routeActionType,
		IngressTransportDomain:  ingressTransportDomain,
		EgressTransportDomain:   egressTransportDomain,
		DiscoveryFQDN:           discoveryFQDN,
		DiscoveryMethod:         discoveryMethod,
		VisitedControlEndpoint:  visitedAddr.String(),
		HomeControlEndpoint:     homeAddr,
		VisitedControlTEID:      visitedControlTEID,
		ProxyVisitedControlTEID: t.AllocateTEID(),
		CreatedAt:               now,
		UpdatedAt:               now,
		ExpiresAt:               now.Add(sessionTTL),
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.byID[s.ID] = s
	t.byProxyVisitedTEID[s.ProxyVisitedControlTEID] = s
	return cloneSession(s)
}

func (t *Table) BindHomeControl(sessionID string, homeControlTEID uint32, sessionTTL time.Duration) (*Session, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	s, ok := t.byID[sessionID]
	if !ok {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	s.HomeControlTEID = homeControlTEID
	if s.ProxyHomeControlTEID == 0 {
		s.ProxyHomeControlTEID = t.AllocateTEID()
	}
	s.UpdatedAt = time.Now().UTC()
	s.ExpiresAt = s.UpdatedAt.Add(sessionTTL)
	t.byProxyHomeTEID[s.ProxyHomeControlTEID] = s
	return cloneSession(s), nil
}

func (t *Table) UpsertVisitedUserPlane(sessionID, endpoint string, teid uint32, sessionTTL time.Duration) (*Session, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	s, ok := t.byID[sessionID]
	if !ok {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	s.VisitedUserEndpoint = endpoint
	s.VisitedUserTEID = teid
	if s.ProxyVisitedUserTEID == 0 {
		s.ProxyVisitedUserTEID = t.AllocateTEID()
	}
	s.UpdatedAt = time.Now().UTC()
	s.ExpiresAt = s.UpdatedAt.Add(sessionTTL)
	t.byProxyVisitedUser[s.ProxyVisitedUserTEID] = s
	return cloneSession(s), nil
}

func (t *Table) UpsertHomeUserPlane(sessionID, endpoint string, teid uint32, sessionTTL time.Duration) (*Session, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	s, ok := t.byID[sessionID]
	if !ok {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	s.HomeUserEndpoint = endpoint
	s.HomeUserTEID = teid
	if s.ProxyHomeUserTEID == 0 {
		s.ProxyHomeUserTEID = t.AllocateTEID()
	}
	s.UpdatedAt = time.Now().UTC()
	s.ExpiresAt = s.UpdatedAt.Add(sessionTTL)
	t.byProxyHomeUser[s.ProxyHomeUserTEID] = s
	return cloneSession(s), nil
}

func (t *Table) GetByProxyHomeControlTEID(teid uint32) (*Session, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.byProxyHomeTEID[teid]
	if !ok {
		return nil, false
	}
	return cloneSession(s), true
}

func (t *Table) GetByProxyHomeControlTEIDFromPeer(teid uint32, peerDomain, peerAddr string) (*Session, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.byProxyHomeTEID[teid]
	if !ok || s.IngressTransportDomain != peerDomain || !sameEndpoint(s.VisitedControlEndpoint, peerAddr) {
		return nil, false
	}
	return cloneSession(s), true
}

func (t *Table) GetByProxyVisitedUserTEID(teid uint32) (*Session, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.byProxyVisitedUser[teid]
	if !ok {
		return nil, false
	}
	return cloneSession(s), true
}

func (t *Table) GetByProxyVisitedUserTEIDFromPeer(teid uint32, peerDomain, peerAddr string) (*Session, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.byProxyVisitedUser[teid]
	if !ok || s.EgressTransportDomain != peerDomain || !sameEndpoint(s.HomeUserEndpoint, peerAddr) {
		return nil, false
	}
	return cloneSession(s), true
}

func (t *Table) GetByProxyHomeUserTEID(teid uint32) (*Session, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.byProxyHomeUser[teid]
	if !ok {
		return nil, false
	}
	return cloneSession(s), true
}

func (t *Table) GetByProxyHomeUserTEIDFromPeer(teid uint32, peerDomain, peerAddr string) (*Session, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.byProxyHomeUser[teid]
	if !ok || s.IngressTransportDomain != peerDomain || !sameEndpoint(s.VisitedUserEndpoint, peerAddr) {
		return nil, false
	}
	return cloneSession(s), true
}

func (t *Table) Get(id string) (*Session, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.byID[id]
	if !ok {
		return nil, false
	}
	return cloneSession(s), true
}

func (t *Table) Delete(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.byID[id]
	if !ok {
		return
	}
	delete(t.byID, id)
	delete(t.byProxyVisitedTEID, s.ProxyVisitedControlTEID)
	if s.ProxyHomeControlTEID != 0 {
		delete(t.byProxyHomeTEID, s.ProxyHomeControlTEID)
	}
	if s.ProxyVisitedUserTEID != 0 {
		delete(t.byProxyVisitedUser, s.ProxyVisitedUserTEID)
	}
	if s.ProxyHomeUserTEID != 0 {
		delete(t.byProxyHomeUser, s.ProxyHomeUserTEID)
	}
}

func (t *Table) AddPending(tx PendingTransaction) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now().UTC()
	if tx.CreatedAt.IsZero() {
		tx.CreatedAt = now
	}
	if tx.ExpiresAt.IsZero() {
		tx.ExpiresAt = now.Add(30 * time.Second)
	}
	t.pendingTransactions[pendingKey{
		peerDomain:  tx.PeerDomain,
		peerAddr:    tx.PeerAddr,
		sequence:    tx.Sequence,
		messageType: tx.MessageType,
	}] = tx
}

func (t *Table) PopPending(peerDomain, peerAddr string, sequence uint32, messageType uint8) (PendingTransaction, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := pendingKey{peerDomain: peerDomain, peerAddr: peerAddr, sequence: sequence, messageType: messageType}
	tx, ok := t.pendingTransactions[key]
	if !ok {
		return PendingTransaction{}, false
	}
	delete(t.pendingTransactions, key)
	return tx, true
}

func (t *Table) DeletePending(peerDomain, peerAddr string, sequence uint32, messageType uint8) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.pendingTransactions, pendingKey{
		peerDomain:  peerDomain,
		peerAddr:    peerAddr,
		sequence:    sequence,
		messageType: messageType,
	})
}

func (t *Table) List() []Session {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]Session, 0, len(t.byID))
	for _, s := range t.byID {
		out = append(out, *cloneSession(s))
	}
	return out
}

func (t *Table) CountByRoutePeer(peer string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	count := 0
	for _, s := range t.byID {
		if s.RoutePeer == peer {
			count++
		}
	}
	return count
}

func (t *Table) CountByTransportDomain(domain string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	count := 0
	for _, s := range t.byID {
		if s.EgressTransportDomain == domain || s.IngressTransportDomain == domain {
			count++
		}
	}
	return count
}

func (t *Table) Stats() Snapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return Snapshot{
		ActiveSessions:      len(t.byID),
		PendingTransactions: len(t.pendingTransactions),
	}
}

func (t *Table) Touch(id string, sessionTTL time.Duration) (*Session, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s, ok := t.byID[id]
	if !ok {
		return nil, fmt.Errorf("session %q not found", id)
	}
	s.UpdatedAt = time.Now().UTC()
	s.ExpiresAt = s.UpdatedAt.Add(sessionTTL)
	return cloneSession(s), nil
}

func (t *Table) CleanupExpired(now time.Time) []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	var deleted []string
	for id, s := range t.byID {
		if !s.ExpiresAt.IsZero() && !now.Before(s.ExpiresAt) {
			deleted = append(deleted, id)
			delete(t.byID, id)
			delete(t.byProxyVisitedTEID, s.ProxyVisitedControlTEID)
			delete(t.byProxyHomeTEID, s.ProxyHomeControlTEID)
			delete(t.byProxyVisitedUser, s.ProxyVisitedUserTEID)
			delete(t.byProxyHomeUser, s.ProxyHomeUserTEID)
		}
	}
	for key, tx := range t.pendingTransactions {
		if !tx.ExpiresAt.IsZero() && !now.Before(tx.ExpiresAt) {
			delete(t.pendingTransactions, key)
		}
	}
	return deleted
}

func cloneSession(s *Session) *Session {
	cp := *s
	return &cp
}

func sameEndpoint(expected, actual string) bool {
	expectedAddr, errExpected := net.ResolveUDPAddr("udp", expected)
	actualAddr, errActual := net.ResolveUDPAddr("udp", actual)
	if errExpected != nil || errActual != nil {
		return expected == actual
	}
	if expectedAddr.Port != actualAddr.Port {
		return false
	}
	if expectedAddr.IP == nil || actualAddr.IP == nil {
		return expected == actual
	}
	return expectedAddr.IP.Equal(actualAddr.IP)
}
