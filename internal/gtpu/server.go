package gtpu

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/vectorcore/gtp_proxy/internal/config"
	"github.com/vectorcore/gtp_proxy/internal/metrics"
	"github.com/vectorcore/gtp_proxy/internal/session"
)

type Server struct {
	cfg      *config.Manager
	sessions *session.Table
	metrics  *metrics.Registry
	logger   *slog.Logger
}

func NewServer(cfg *config.Manager, sessions *session.Table, metrics *metrics.Registry, logger *slog.Logger) *Server {
	return &Server{cfg: cfg, sessions: sessions, metrics: metrics, logger: logger}
}

func (s *Server) Start(ctx context.Context) error {
	listenAddr, err := net.ResolveUDPAddr("udp", s.cfg.Snapshot().Proxy.GTPU.Listen)
	if err != nil {
		return fmt.Errorf("resolve GTPU listen address: %w", err)
	}
	conn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		return fmt.Errorf("listen GTPU: %w", err)
	}
	defer conn.Close()

	s.logger.Info("gtpu listener started", "listen", listenAddr.String())

	buf := make([]byte, 64*1024)
	for {
		if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			return fmt.Errorf("set GTPU read deadline: %w", err)
		}
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				select {
				case <-ctx.Done():
					return nil
				default:
					continue
				}
			}
			return fmt.Errorf("read GTPU packet: %w", err)
		}
		if err := s.handlePacket(conn, src, buf[:n]); err != nil {
			s.logger.Warn("gtpu packet handling failed", "src", src.String(), "err", err)
		}
	}
}

func (s *Server) handlePacket(conn *net.UDPConn, src *net.UDPAddr, data []byte) error {
	if len(data) < 8 {
		s.metrics.IncMessageError("gtpu", "packet")
		return fmt.Errorf("gtpu packet too short")
	}
	teid := binary.BigEndian.Uint32(data[4:8])

	if sess, ok := s.sessions.GetByProxyHomeUserTEID(teid); ok {
		s.metrics.IncGTPUForwardHit()
		return s.forwardPacket(conn, data, sess.HomeUserTEID, sess.HomeUserEndpoint, sess.ID)
	}
	if sess, ok := s.sessions.GetByProxyVisitedUserTEID(teid); ok {
		s.metrics.IncGTPUForwardHit()
		return s.forwardPacket(conn, data, sess.VisitedUserTEID, sess.VisitedUserEndpoint, sess.ID)
	}

	s.metrics.IncGTPUForwardMiss()
	s.metrics.IncUnknownTEIDDrop()
	s.metrics.IncMessageError("gtpu", "packet")
	return fmt.Errorf("unknown GTPU proxy TEID %d from %s", teid, src.String())
}

func (s *Server) forwardPacket(conn *net.UDPConn, data []byte, targetTEID uint32, endpoint, sessionID string) error {
	if endpoint == "" || targetTEID == 0 {
		s.metrics.IncGTPUForwardMiss()
		s.metrics.IncUnknownTEIDDrop()
		s.metrics.IncMessageError("gtpu", "packet")
		return fmt.Errorf("session %q has incomplete user-plane mapping", sessionID)
	}
	target, err := net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		return fmt.Errorf("resolve GTPU target %q: %w", endpoint, err)
	}
	out := append([]byte(nil), data...)
	binary.BigEndian.PutUint32(out[4:8], targetTEID)

	ttl := s.cfg.Snapshot().Proxy.Timeouts.SessionIdleDuration()
	_, _ = s.sessions.Touch(sessionID, ttl)
	_, err = conn.WriteToUDP(out, target)
	if err == nil {
		s.metrics.IncGTPUPacketsForwarded()
		if sess, ok := s.sessions.Get(sessionID); ok {
			s.metrics.IncPeerUserPlanePacket(sess.RoutePeer)
		}
	} else {
		s.metrics.IncMessageError("gtpu", "packet")
	}
	return err
}
