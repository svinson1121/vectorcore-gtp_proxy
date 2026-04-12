package gtpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/vectorcore/gtp_proxy/internal/config"
	"github.com/vectorcore/gtp_proxy/internal/metrics"
	"github.com/vectorcore/gtp_proxy/internal/routing"
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
	listenAddr, err := net.ResolveUDPAddr("udp", s.cfg.Snapshot().Proxy.GTPC.Listen)
	if err != nil {
		return fmt.Errorf("resolve GTPC listen address: %w", err)
	}
	conn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		return fmt.Errorf("listen GTPC: %w", err)
	}
	defer conn.Close()

	s.logger.Info("gtpc listener started", "listen", listenAddr.String())

	buf := make([]byte, 64*1024)
	for {
		if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			return fmt.Errorf("set GTPC read deadline: %w", err)
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
			return fmt.Errorf("read GTPC packet: %w", err)
		}
		if err := s.handlePacket(conn, src, buf[:n]); err != nil {
			s.logger.Warn("gtpc packet handling failed", "src", src.String(), "err", err)
		}
	}
}

func (s *Server) handlePacket(conn *net.UDPConn, src *net.UDPAddr, data []byte) error {
	packet, err := ParsePacket(data)
	if err != nil {
		s.metrics.IncMessageError("gtpc", "parse")
		return err
	}

	if tx, ok := s.sessions.PopPending(src.String(), packet.Sequence, packet.MessageType); ok {
		return s.handleResponse(conn, src, packet, tx)
	}

	switch packet.MessageType {
	case messageTypeEchoRequest:
		resp := EchoResponse(packet)
		_, err := conn.WriteToUDP(resp.Marshal(), src)
		return err
	case messageTypeCreateSessionRequest:
		return s.handleCreateSessionRequest(conn, src, packet)
	case messageTypeModifyBearerRequest:
		return s.handleModifyBearerRequest(conn, src, packet)
	case messageTypeDeleteSessionRequest:
		return s.handleDeleteSessionRequest(conn, src, packet)
	default:
		s.metrics.IncMessageError("gtpc", fmt.Sprintf("unsupported_%d", packet.MessageType))
		return fmt.Errorf("unsupported message type %d", packet.MessageType)
	}
}

func (s *Server) handleCreateSessionRequest(conn *net.UDPConn, src *net.UDPAddr, packet Packet) error {
	cfg := s.cfg.Snapshot()
	imsi, apn, visitedTEID, err := ExtractCreateSessionMetadata(packet.Payload)
	if err != nil {
		s.metrics.IncMessageError("gtpc", "create_session_request")
		return fmt.Errorf("extract create-session metadata: %w", err)
	}
	match, err := routing.Select(cfg, routing.Input{IMSI: imsi, APN: apn})
	if err != nil {
		s.metrics.IncMessageError("gtpc", "create_session_request")
		return err
	}
	target, err := net.ResolveUDPAddr("udp", match.Peer.Address)
	if err != nil {
		s.metrics.IncMessageError("gtpc", "create_session_request")
		return fmt.Errorf("resolve home peer: %w", err)
	}

	sessionTTL := cfg.Proxy.Timeouts.SessionIdleDuration()
	originalPayload := packet.Payload
	sess := s.sessions.Create(src, target.String(), imsi, apn, match.MatchType, match.MatchValue, match.Peer.Name, visitedTEID, sessionTTL)
	payload, err := RewriteFTEIDs(originalPayload, func(index int, current FTEID) (FTEID, bool, error) {
		next := current
		targetCfg := cfg.Proxy.GTPU
		if index == 0 {
			targetCfg = config.GTPUConfig{
				AdvertiseAddress:     cfg.Proxy.GTPC.AdvertiseAddress,
				AdvertiseAddressIPv4: cfg.Proxy.GTPC.AdvertiseAddressIPv4,
				AdvertiseAddressIPv6: cfg.Proxy.GTPC.AdvertiseAddressIPv6,
			}
		}
		if err := rewriteAdvertiseAddresses(&next, current, targetCfg); err != nil {
			return FTEID{}, false, err
		}
		if index == 0 {
			next.TEID = sess.ProxyVisitedControlTEID
		} else {
			next.TEID = s.sessions.AllocateTEID()
		}
		return next, true, nil
	})
	if err != nil {
		s.metrics.IncMessageError("gtpc", "create_session_request")
		return fmt.Errorf("rewrite request F-TEIDs: %w", err)
	}
	packet.Payload = payload
	sess = s.captureVisitedUserPlane(sess, originalPayload, sessionTTL)

	s.sessions.AddPending(session.PendingTransaction{
		PeerAddr:         target.String(),
		Sequence:         packet.Sequence,
		MessageType:      messageTypeCreateSessionResp,
		OriginalPeerAddr: src.String(),
		SessionID:        sess.ID,
	})

	s.logger.Info("forwarding create-session request",
		"imsi", sess.IMSI,
		"apn", sess.APN,
		"src", src.String(),
		"dst", target.String(),
		"route_type", match.MatchType,
		"route_value", match.MatchValue,
		"route_peer", match.Peer.Name,
		"inbound_teid", visitedTEID,
		"outbound_teid", sess.ProxyVisitedControlTEID,
		"session_id", sess.ID,
	)
	_, err = conn.WriteToUDP(packet.Marshal(), target)
	if err == nil {
		s.metrics.IncSessionCreate()
		s.metrics.IncPeerControlPlanePacket(match.Peer.Name)
	} else {
		s.metrics.IncMessageError("gtpc", "create_session_request")
	}
	return err
}

func (s *Server) handleModifyBearerRequest(conn *net.UDPConn, src *net.UDPAddr, packet Packet) error {
	cfg := s.cfg.Snapshot()
	sessionTTL := cfg.Proxy.Timeouts.SessionIdleDuration()
	sess, ok := s.sessions.GetByProxyHomeControlTEID(packet.TEID)
	if !ok {
		s.metrics.IncMessageError("gtpc", "modify_bearer_request")
		return fmt.Errorf("unknown proxy TEID %d", packet.TEID)
	}
	target, err := net.ResolveUDPAddr("udp", sess.HomeControlEndpoint)
	if err != nil {
		s.metrics.IncMessageError("gtpc", "modify_bearer_request")
		return fmt.Errorf("resolve home peer: %w", err)
	}

	originalPayload := packet.Payload
	payload, err := RewriteFTEIDs(originalPayload, func(index int, current FTEID) (FTEID, bool, error) {
		next := current
		if err := rewriteAdvertiseAddresses(&next, current, cfg.Proxy.GTPU); err != nil {
			return FTEID{}, false, err
		}
		if sess.ProxyVisitedUserTEID != 0 {
			next.TEID = sess.ProxyVisitedUserTEID
		} else {
			next.TEID = s.sessions.AllocateTEID()
		}
		return next, true, nil
	})
	if err != nil {
		s.metrics.IncMessageError("gtpc", "modify_bearer_request")
		return fmt.Errorf("rewrite modify-bearer request F-TEIDs: %w", err)
	}
	packet.Payload = payload
	packet.TEID = sess.HomeControlTEID
	sess = s.captureVisitedUserPlane(sess, originalPayload, sessionTTL)
	_, _ = s.sessions.Touch(sess.ID, sessionTTL)

	s.sessions.AddPending(session.PendingTransaction{
		PeerAddr:         target.String(),
		Sequence:         packet.Sequence,
		MessageType:      messageTypeModifyBearerResp,
		OriginalPeerAddr: src.String(),
		SessionID:        sess.ID,
	})

	s.logger.Info("forwarding modify-bearer request",
		"imsi", sess.IMSI,
		"apn", sess.APN,
		"src", src.String(),
		"dst", target.String(),
		"inbound_teid", sess.ProxyHomeControlTEID,
		"outbound_teid", sess.HomeControlTEID,
		"session_id", sess.ID,
	)
	_, err = conn.WriteToUDP(packet.Marshal(), target)
	if err == nil {
		s.metrics.IncPeerControlPlanePacket(sess.RoutePeer)
	} else {
		s.metrics.IncMessageError("gtpc", "modify_bearer_request")
	}
	return err
}

func (s *Server) handleDeleteSessionRequest(conn *net.UDPConn, src *net.UDPAddr, packet Packet) error {
	sess, ok := s.sessions.GetByProxyHomeControlTEID(packet.TEID)
	if !ok {
		s.metrics.IncUnknownTEIDDrop()
		s.metrics.IncMessageError("gtpc", "delete_session_request")
		return fmt.Errorf("unknown proxy TEID %d", packet.TEID)
	}
	target, err := net.ResolveUDPAddr("udp", sess.HomeControlEndpoint)
	if err != nil {
		s.metrics.IncMessageError("gtpc", "delete_session_request")
		return fmt.Errorf("resolve home peer: %w", err)
	}

	packet.TEID = sess.HomeControlTEID
	s.sessions.AddPending(session.PendingTransaction{
		PeerAddr:         target.String(),
		Sequence:         packet.Sequence,
		MessageType:      messageTypeDeleteSessionResp,
		OriginalPeerAddr: src.String(),
		SessionID:        sess.ID,
	})

	s.logger.Info("forwarding delete-session request",
		"imsi", sess.IMSI,
		"apn", sess.APN,
		"src", src.String(),
		"dst", target.String(),
		"inbound_teid", sess.ProxyHomeControlTEID,
		"outbound_teid", sess.HomeControlTEID,
		"session_id", sess.ID,
	)
	_, err = conn.WriteToUDP(packet.Marshal(), target)
	if err == nil {
		s.metrics.IncPeerControlPlanePacket(sess.RoutePeer)
	} else {
		s.metrics.IncMessageError("gtpc", "delete_session_request")
	}
	return err
}

func (s *Server) handleResponse(conn *net.UDPConn, src *net.UDPAddr, packet Packet, tx session.PendingTransaction) error {
	switch packet.MessageType {
	case messageTypeCreateSessionResp:
		return s.handleCreateSessionResponse(conn, packet, tx)
	case messageTypeModifyBearerResp:
		return s.handleModifyBearerResponse(conn, packet, tx)
	case messageTypeDeleteSessionResp:
		return s.handleDeleteSessionResponse(conn, packet, tx)
	default:
		return fmt.Errorf("unexpected response message type %d", packet.MessageType)
	}
}

func (s *Server) handleCreateSessionResponse(conn *net.UDPConn, packet Packet, tx session.PendingTransaction) error {
	sess, ok := s.sessions.Get(tx.SessionID)
	if !ok {
		return fmt.Errorf("session %q not found", tx.SessionID)
	}
	if homeFTEID, ok := ExtractFirstFTEID(packet.Payload); ok {
		updated, err := s.sessions.BindHomeControl(sess.ID, homeFTEID.TEID)
		if err != nil {
			return err
		}
		sess = updated
	}

	cfg := s.cfg.Snapshot()
	sessionTTL := cfg.Proxy.Timeouts.SessionIdleDuration()
	originalPayload := packet.Payload
	payload, err := RewriteFTEIDs(originalPayload, func(index int, current FTEID) (FTEID, bool, error) {
		next := current
		targetCfg := cfg.Proxy.GTPU
		if index == 0 {
			targetCfg = config.GTPUConfig{
				AdvertiseAddress:     cfg.Proxy.GTPC.AdvertiseAddress,
				AdvertiseAddressIPv4: cfg.Proxy.GTPC.AdvertiseAddressIPv4,
				AdvertiseAddressIPv6: cfg.Proxy.GTPC.AdvertiseAddressIPv6,
			}
		}
		if err := rewriteAdvertiseAddresses(&next, current, targetCfg); err != nil {
			return FTEID{}, false, err
		}
		if index == 0 && sess.ProxyHomeControlTEID != 0 {
			next.TEID = sess.ProxyHomeControlTEID
		} else {
			next.TEID = s.sessions.AllocateTEID()
		}
		return next, true, nil
	})
	if err != nil {
		return fmt.Errorf("rewrite response F-TEIDs: %w", err)
	}
	packet.Payload = payload
	packet.TEID = sess.VisitedControlTEID
	sess = s.captureHomeUserPlane(sess, originalPayload, sessionTTL)
	_, _ = s.sessions.Touch(sess.ID, sessionTTL)

	dst, err := net.ResolveUDPAddr("udp", tx.OriginalPeerAddr)
	if err != nil {
		return fmt.Errorf("resolve visited peer: %w", err)
	}
	s.logger.Info("forwarding create-session response",
		"imsi", sess.IMSI,
		"apn", sess.APN,
		"src", sess.HomeControlEndpoint,
		"dst", dst.String(),
		"inbound_teid", sess.HomeControlTEID,
		"outbound_teid", sess.ProxyHomeControlTEID,
		"session_id", sess.ID,
	)
	_, err = conn.WriteToUDP(packet.Marshal(), dst)
	return err
}

func (s *Server) handleModifyBearerResponse(conn *net.UDPConn, packet Packet, tx session.PendingTransaction) error {
	sess, ok := s.sessions.Get(tx.SessionID)
	if !ok {
		return fmt.Errorf("session %q not found", tx.SessionID)
	}
	cfg := s.cfg.Snapshot()
	sessionTTL := cfg.Proxy.Timeouts.SessionIdleDuration()
	originalPayload := packet.Payload
	payload, err := RewriteFTEIDs(originalPayload, func(index int, current FTEID) (FTEID, bool, error) {
		next := current
		if err := rewriteAdvertiseAddresses(&next, current, cfg.Proxy.GTPU); err != nil {
			return FTEID{}, false, err
		}
		if sess.ProxyHomeUserTEID != 0 {
			next.TEID = sess.ProxyHomeUserTEID
		} else {
			next.TEID = s.sessions.AllocateTEID()
		}
		return next, true, nil
	})
	if err != nil {
		return fmt.Errorf("rewrite modify-bearer response F-TEIDs: %w", err)
	}
	packet.Payload = payload
	packet.TEID = sess.VisitedControlTEID
	sess = s.captureHomeUserPlane(sess, originalPayload, sessionTTL)
	_, _ = s.sessions.Touch(sess.ID, sessionTTL)

	dst, err := net.ResolveUDPAddr("udp", tx.OriginalPeerAddr)
	if err != nil {
		return fmt.Errorf("resolve visited peer: %w", err)
	}
	s.logger.Info("forwarding modify-bearer response",
		"imsi", sess.IMSI,
		"apn", sess.APN,
		"src", sess.HomeControlEndpoint,
		"dst", dst.String(),
		"inbound_teid", sess.HomeControlTEID,
		"outbound_teid", sess.VisitedControlTEID,
		"session_id", sess.ID,
	)
	_, err = conn.WriteToUDP(packet.Marshal(), dst)
	return err
}

func (s *Server) handleDeleteSessionResponse(conn *net.UDPConn, packet Packet, tx session.PendingTransaction) error {
	sess, ok := s.sessions.Get(tx.SessionID)
	if !ok {
		return fmt.Errorf("session %q not found", tx.SessionID)
	}
	packet.TEID = sess.VisitedControlTEID
	dst, err := net.ResolveUDPAddr("udp", tx.OriginalPeerAddr)
	if err != nil {
		return fmt.Errorf("resolve visited peer: %w", err)
	}
	s.logger.Info("forwarding delete-session response",
		"imsi", sess.IMSI,
		"apn", sess.APN,
		"src", sess.HomeControlEndpoint,
		"dst", dst.String(),
		"inbound_teid", sess.HomeControlTEID,
		"outbound_teid", sess.VisitedControlTEID,
		"session_id", sess.ID,
	)
	_, err = conn.WriteToUDP(packet.Marshal(), dst)
	s.sessions.Delete(sess.ID)
	s.metrics.IncSessionDelete()
	return err
}

type advertiseConfig interface {
	AdvertiseIP(isIPv6 bool) (net.IP, bool)
}

func rewriteAdvertiseAddresses(next *FTEID, current FTEID, cfg advertiseConfig) error {
	if current.HasIPv4() {
		ip, ok := cfg.AdvertiseIP(false)
		if !ok {
			return fmt.Errorf("missing IPv4 advertise address for IPv4 F-TEID rewrite")
		}
		next.IPv4 = ip
	}
	if current.HasIPv6() {
		ip, ok := cfg.AdvertiseIP(true)
		if !ok {
			return fmt.Errorf("missing IPv6 advertise address for IPv6 F-TEID rewrite")
		}
		next.IPv6 = ip
	}
	return nil
}

func (s *Server) captureVisitedUserPlane(sess *session.Session, payload []byte, ttl time.Duration) *session.Session {
	fteids, err := ExtractAllFTEIDs(payload)
	if err != nil || len(fteids) < 2 {
		return sess
	}
	endpoint := userPlaneEndpoint(fteids[1])
	if endpoint == "" {
		return sess
	}
	updated, err := s.sessions.UpsertVisitedUserPlane(sess.ID, endpoint, fteids[1].TEID, ttl)
	if err != nil {
		return sess
	}
	return updated
}

func (s *Server) captureHomeUserPlane(sess *session.Session, payload []byte, ttl time.Duration) *session.Session {
	fteids, err := ExtractAllFTEIDs(payload)
	if err != nil || len(fteids) < 2 {
		return sess
	}
	endpoint := userPlaneEndpoint(fteids[1])
	if endpoint == "" {
		return sess
	}
	updated, err := s.sessions.UpsertHomeUserPlane(sess.ID, endpoint, fteids[1].TEID, ttl)
	if err != nil {
		return sess
	}
	return updated
}

func userPlaneEndpoint(fteid FTEID) string {
	if ip := fteid.IPv4.To4(); ip != nil {
		return net.JoinHostPort(ip.String(), "2152")
	}
	if ip := fteid.IPv6.To16(); ip != nil && ip.To4() == nil {
		return net.JoinHostPort(ip.String(), "2152")
	}
	return ""
}
