//go:build !linux

package transport

import (
	"context"
	"fmt"
	"net"
)

func ListenUDPInNetNS(network string, laddr *net.UDPAddr, netnsPath string) (*net.UDPConn, error) {
	if netnsPath != "" {
		return nil, fmt.Errorf("netns-aware listeners are only supported on linux")
	}
	return net.ListenUDP(network, laddr)
}

func DialContextInNetNS(ctx context.Context, network, address, netnsPath string) (net.Conn, error) {
	if netnsPath != "" {
		return nil, fmt.Errorf("netns-aware dial is only supported on linux")
	}
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, address)
}
