package gtpu

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/vectorcore/gtp_proxy/internal/config"
	"github.com/vectorcore/gtp_proxy/internal/metrics"
	"github.com/vectorcore/gtp_proxy/internal/session"
	"github.com/vectorcore/gtp_proxy/internal/transport"
)

type Server struct {
	cfg      *config.Manager
	sessions *session.Table
	metrics  *metrics.Registry
	logger   *slog.Logger
	runtime  *transport.Runtime
	mu       sync.RWMutex
	conns    map[string]*net.UDPConn
	bound    map[string]gtpuBoundDomain
	started  map[string]bool
}

type gtpuBoundDomain struct {
	name   string
	netns  string
	listen string
}

func NewServer(cfg *config.Manager, sessions *session.Table, metrics *metrics.Registry, runtime *transport.Runtime, logger *slog.Logger) *Server {
	return &Server{
		cfg:      cfg,
		sessions: sessions,
		metrics:  metrics,
		runtime:  runtime,
		logger:   logger,
		conns:    map[string]*net.UDPConn{},
		bound:    map[string]gtpuBoundDomain{},
		started:  map[string]bool{},
	}
}

func (s *Server) Start(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer s.closeAll()

	for {
		s.reconcile(ctx, s.cfg.Snapshot())
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *Server) handlePacket(ingressDomain string, src *net.UDPAddr, data []byte) error {
	if len(data) < 8 {
		s.metrics.IncMessageError("gtpu", "packet")
		return fmt.Errorf("gtpu packet too short")
	}
	teid := binary.BigEndian.Uint32(data[4:8])

	if sess, ok := s.sessions.GetByProxyHomeUserTEIDFromPeer(teid, ingressDomain, src.String()); ok {
		s.metrics.IncGTPUForwardHit()
		return s.forwardPacket(sess.EgressTransportDomain, data, sess.HomeUserTEID, sess.HomeUserEndpoint, sess.ID)
	}
	if sess, ok := s.sessions.GetByProxyVisitedUserTEIDFromPeer(teid, ingressDomain, src.String()); ok {
		s.metrics.IncGTPUForwardHit()
		return s.forwardPacket(sess.IngressTransportDomain, data, sess.VisitedUserTEID, sess.VisitedUserEndpoint, sess.ID)
	}

	s.metrics.IncGTPUForwardMiss()
	s.metrics.IncUnknownTEIDDrop()
	s.metrics.IncMessageError("gtpu", "packet")
	return fmt.Errorf("unknown GTPU proxy TEID %d from %s", teid, src.String())
}

func (s *Server) forwardPacket(egressDomain string, data []byte, targetTEID uint32, endpoint, sessionID string) error {
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
	conn, err := s.connForDomain(egressDomain)
	if err != nil {
		s.metrics.IncMessageError("gtpu", "packet")
		return err
	}
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

func (s *Server) reconcile(ctx context.Context, cfg config.Config) {
	desired := desiredGTPUDomains(cfg)

	s.mu.Lock()
	for domain, current := range s.bound {
		next, ok := desired[domain]
		if !ok || next != current {
			if conn := s.conns[domain]; conn != nil {
				_ = conn.Close()
			}
			delete(s.conns, domain)
			delete(s.bound, domain)
			delete(s.started, domain)
			if s.runtime != nil {
				s.runtime.SetGTPU(transport.ListenerStatus{
					Protocol: "gtpu",
					State:    "disabled",
					Domain:   domain,
					Listen:   current.listen,
					Reason:   "transport domain removed or changed",
				})
			}
		}
	}
	s.mu.Unlock()

	for domain, bound := range desired {
		s.mu.RLock()
		_, ok := s.conns[domain]
		s.mu.RUnlock()
		if ok {
			continue
		}

		listenAddr, err := net.ResolveUDPAddr("udp", bound.listen)
		if err != nil {
			if s.runtime != nil {
				s.runtime.SetGTPU(transport.ListenerStatus{
					Protocol:      "gtpu",
					State:         "error",
					Domain:        domain,
					Listen:        bound.listen,
					LastBindError: err.Error(),
					Reason:        "resolve listen address failed",
				})
			}
			continue
		}
		conn, err := transport.ListenUDPInNetNS("udp", listenAddr, bound.netns)
		if err != nil {
			if s.runtime != nil {
				s.runtime.SetGTPU(transport.ListenerStatus{
					Protocol:      "gtpu",
					State:         "error",
					Domain:        domain,
					Listen:        bound.listen,
					LastBindError: err.Error(),
					Reason:        "listen failed",
				})
			}
			continue
		}

		s.mu.Lock()
		s.conns[domain] = conn
		s.bound[domain] = bound
		if !s.started[domain] {
			s.started[domain] = true
			go s.readLoop(ctx, domain, conn)
		}
		s.mu.Unlock()

		if s.runtime != nil {
			s.runtime.SetGTPU(transport.ListenerStatus{
				Protocol: "gtpu",
				State:    "active",
				Domain:   domain,
				Listen:   bound.listen,
			})
		}
		s.logger.Info("gtpu listener started", "listen", bound.listen, "domain", domain, "netns_path", bound.netns)
	}
}

func (s *Server) readLoop(ctx context.Context, domain string, conn *net.UDPConn) {
	buf := make([]byte, 64*1024)
	for {
		if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			s.setDomainError(domain, conn, "", fmt.Errorf("set GTPU read deadline: %w", err))
			return
		}
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				if ctx.Err() != nil {
					return
				}
				if !s.isCurrentConn(domain, conn) {
					return
				}
				continue
			}
			if ctx.Err() != nil || !s.isCurrentConn(domain, conn) {
				return
			}
			s.setDomainError(domain, conn, "", fmt.Errorf("read GTPU packet: %w", err))
			return
		}
		if err := s.handlePacket(domain, src, buf[:n]); err != nil {
			s.logger.Warn("gtpu packet handling failed", "domain", domain, "src", src.String(), "err", err)
		}
	}
}

func (s *Server) connForDomain(domain string) (*net.UDPConn, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	conn, ok := s.conns[domain]
	if ok && conn != nil {
		return conn, nil
	}
	return nil, fmt.Errorf("no active GTPU listener for transport domain %q", domain)
}

func (s *Server) isCurrentConn(domain string, conn *net.UDPConn) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.conns[domain] == conn
}

func (s *Server) setDomainError(domain string, conn *net.UDPConn, listen string, err error) {
	s.mu.Lock()
	if s.conns[domain] == conn {
		delete(s.conns, domain)
		delete(s.bound, domain)
	}
	delete(s.started, domain)
	s.mu.Unlock()
	_ = conn.Close()
	if s.runtime != nil {
		s.runtime.SetGTPU(transport.ListenerStatus{
			Protocol:      "gtpu",
			State:         "error",
			Domain:        domain,
			Listen:        listen,
			LastBindError: err.Error(),
			Reason:        "listener read failed",
		})
	}
}

func (s *Server) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for domain, conn := range s.conns {
		_ = conn.Close()
		delete(s.conns, domain)
	}
	s.bound = map[string]gtpuBoundDomain{}
	s.started = map[string]bool{}
}

func desiredGTPUDomains(cfg config.Config) map[string]gtpuBoundDomain {
	out := map[string]gtpuBoundDomain{}
	if len(cfg.TransportDomains) == 0 {
		if effective, ok := cfg.EffectiveGTPUConfig(); ok {
			out[""] = gtpuBoundDomain{listen: effective.Listen}
		}
		return out
	}
	for _, domain := range cfg.TransportDomains {
		if !domain.Enabled {
			continue
		}
		out[domain.Name] = gtpuBoundDomain{
			name:   domain.Name,
			netns:  domain.NetNSPath,
			listen: net.JoinHostPort(domain.GTPUListenHost, fmt.Sprintf("%d", domain.GTPUPort)),
		}
	}
	return out
}
