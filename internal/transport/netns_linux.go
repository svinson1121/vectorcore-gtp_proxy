//go:build linux

package transport

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

func ListenUDPInNetNS(network string, laddr *net.UDPAddr, netnsPath string) (*net.UDPConn, error) {
	if netnsPath == "" {
		return net.ListenUDP(network, laddr)
	}

	var (
		conn *net.UDPConn
		err  error
	)
	if err := switchNetNS(netnsPath, func() error {
		conn, err = net.ListenUDP(network, laddr)
		return err
	}); err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func DialContextInNetNS(ctx context.Context, network, address, netnsPath string) (net.Conn, error) {
	if netnsPath == "" {
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, address)
	}

	var (
		conn net.Conn
		err  error
	)
	if err := switchNetNS(netnsPath, func() error {
		var dialer net.Dialer
		conn, err = dialer.DialContext(ctx, network, address)
		return err
	}); err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func switchNetNS(netnsPath string, fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	currentNS, err := os.Open("/proc/self/ns/net")
	if err != nil {
		return fmt.Errorf("open current netns: %w", err)
	}
	defer currentNS.Close()

	targetNS, err := os.Open(netnsPath)
	if err != nil {
		return fmt.Errorf("open target netns %q: %w", netnsPath, err)
	}
	defer targetNS.Close()

	if err := unix.Setns(int(targetNS.Fd()), unix.CLONE_NEWNET); err != nil {
		return fmt.Errorf("setns target %q: %w", netnsPath, err)
	}
	callErr := fn()
	restoreErr := unix.Setns(int(currentNS.Fd()), unix.CLONE_NEWNET)
	if restoreErr != nil {
		return fmt.Errorf("restore original netns: %w", restoreErr)
	}
	if callErr != nil {
		return callErr
	}
	return nil
}
