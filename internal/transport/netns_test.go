package transport

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestListenUDPInNetNSWithoutNamespaceFallsBackToDefault(t *testing.T) {
	conn, err := ListenUDPInNetNS("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0}, "")
	if err != nil {
		t.Fatalf("ListenUDPInNetNS() error = %v", err)
	}
	defer conn.Close()

	if _, ok := conn.LocalAddr().(*net.UDPAddr); !ok {
		t.Fatalf("unexpected local addr type %T", conn.LocalAddr())
	}
}

func TestDialContextInNetNSWithoutNamespaceFallsBackToDefault(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	accepted := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			accepted <- err
			return
		}
		_ = conn.Close()
		accepted <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := DialContextInNetNS(ctx, "tcp", ln.Addr().String(), "")
	if err != nil {
		t.Fatalf("DialContextInNetNS() error = %v", err)
	}
	_ = conn.Close()

	if err := <-accepted; err != nil {
		t.Fatalf("Accept() error = %v", err)
	}
}
