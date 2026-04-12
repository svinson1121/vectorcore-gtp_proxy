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
	PeerAddr         string
	Sequence         uint32
	MessageType      uint8
	OriginalPeerAddr string
	SessionID        string
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

func (t *Table) Create(visitedAddr *net.UDPAddr, homeAddr, imsi, apn, routeMatchType, routeMatchValue, routePeer string, visitedControlTEID uint32, sessionTTL time.Duration) *Session {
	now := time.Now().UTC()
	s := &Session{
		ID:                      fmt.Sprintf("%d-%d", now.UnixNano(), t.AllocateTEID()),
		IMSI:                    imsi,
		APN:                     apn,
		RouteMatchType:          routeMatchType,
		RouteMatchValue:         routeMatchValue,
		RoutePeer:               routePeer,
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

func (t *Table) BindHomeControl(sessionID string, homeControlTEID uint32) (*Session, error) {
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
	s.ExpiresAt = s.UpdatedAt
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

func (t *Table) GetByProxyVisitedUserTEID(teid uint32) (*Session, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.byProxyVisitedUser[teid]
	if !ok {
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
	t.pendingTransactions[pendingKey{
		peerAddr:    tx.PeerAddr,
		sequence:    tx.Sequence,
		messageType: tx.MessageType,
	}] = tx
}

func (t *Table) PopPending(peerAddr string, sequence uint32, messageType uint8) (PendingTransaction, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := pendingKey{peerAddr: peerAddr, sequence: sequence, messageType: messageType}
	tx, ok := t.pendingTransactions[key]
	if !ok {
		return PendingTransaction{}, false
	}
	delete(t.pendingTransactions, key)
	return tx, true
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
	return deleted
}

func cloneSession(s *Session) *Session {
	cp := *s
	return &cp
}
