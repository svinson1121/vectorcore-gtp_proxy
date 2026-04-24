package session

import (
	"net"
	"testing"
	"time"
)

func TestTableCreateBindAndDelete(t *testing.T) {
	table := NewTable()
	visited := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 2123}

	sess := table.Create(visited, "198.51.100.10:2123", "001010123456789", "internet", "apn", "internet", "pgw", "static_peer", "visited-a", "home-a", "", "", 1111, time.Minute)
	if sess.ProxyVisitedControlTEID == 0 {
		t.Fatal("expected proxy visited TEID to be allocated")
	}

	updated, err := table.BindHomeControl(sess.ID, 2222, time.Minute)
	if err != nil {
		t.Fatalf("BindHomeControl() error = %v", err)
	}
	if updated.HomeControlTEID != 2222 || updated.ProxyHomeControlTEID == 0 {
		t.Fatalf("unexpected bound session %#v", updated)
	}

	got, ok := table.GetByProxyHomeControlTEID(updated.ProxyHomeControlTEID)
	if !ok {
		t.Fatal("GetByProxyHomeControlTEID() did not find session")
	}
	if got.ID != sess.ID {
		t.Fatalf("unexpected session ID %q", got.ID)
	}
	if _, ok := table.GetByProxyHomeControlTEIDFromPeer(updated.ProxyHomeControlTEID, "visited-a", visited.String()); !ok {
		t.Fatal("GetByProxyHomeControlTEIDFromPeer() did not validate expected endpoint")
	}
	if _, ok := table.GetByProxyHomeControlTEIDFromPeer(updated.ProxyHomeControlTEID, "visited-a", "10.0.0.2:2123"); ok {
		t.Fatal("GetByProxyHomeControlTEIDFromPeer() matched unexpected endpoint")
	}

	table.Delete(sess.ID)
	if _, ok := table.Get(sess.ID); ok {
		t.Fatal("session still present after delete")
	}
}

func TestCleanupExpiredDeletesSession(t *testing.T) {
	table := NewTable()
	visited := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 2123}

	sess := table.Create(visited, "198.51.100.10:2123", "001010123456789", "internet", "apn", "internet", "pgw", "static_peer", "visited-a", "home-a", "", "", 1111, time.Millisecond)
	deleted := table.CleanupExpired(time.Now().Add(time.Second))
	if len(deleted) != 1 || deleted[0] != sess.ID {
		t.Fatalf("unexpected deleted sessions %#v", deleted)
	}
	if _, ok := table.Get(sess.ID); ok {
		t.Fatal("session still present after cleanup")
	}
}

func TestCleanupExpiredDeletesPendingTransactions(t *testing.T) {
	table := NewTable()
	table.AddPending(PendingTransaction{
		PeerAddr:    "198.51.100.10:2123",
		Sequence:    7,
		MessageType: 33,
		SessionID:   "pending",
		ExpiresAt:   time.Now().Add(-time.Second),
	})
	table.CleanupExpired(time.Now())
	if _, ok := table.PopPending("", "198.51.100.10:2123", 7, 33); ok {
		t.Fatal("pending transaction still present after cleanup")
	}
}

func TestDeletePendingRemovesPendingTransaction(t *testing.T) {
	table := NewTable()
	table.AddPending(PendingTransaction{
		PeerAddr:    "198.51.100.10:2123",
		Sequence:    7,
		MessageType: 33,
		SessionID:   "pending",
	})
	table.DeletePending("", "198.51.100.10:2123", 7, 33)
	if _, ok := table.PopPending("", "198.51.100.10:2123", 7, 33); ok {
		t.Fatal("pending transaction still present after delete")
	}
}

func TestCountByPeerAndTransportDomain(t *testing.T) {
	table := NewTable()
	visited := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 2123}

	table.Create(visited, "198.51.100.10:2123", "001010123456789", "ims", "apn", "ims", "pgw-a", "dns_discovery", "visited-a", "home-a", "topon.s8.pgw.epc.example.net", "dns_naptr_srv", 1111, time.Minute)
	table.Create(visited, "198.51.100.11:2123", "001010123456780", "internet", "apn", "internet", "pgw-a", "static_peer", "visited-a", "home-a", "", "static_peer", 2222, time.Minute)

	if got := table.CountByRoutePeer("pgw-a"); got != 2 {
		t.Fatalf("unexpected CountByRoutePeer() = %d", got)
	}
	if got := table.CountByTransportDomain("home-a"); got != 2 {
		t.Fatalf("unexpected CountByTransportDomain() = %d", got)
	}
}
