package gtpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/vectorcore/gtp_proxy/internal/config"
	"github.com/vectorcore/gtp_proxy/internal/discovery"
	"github.com/vectorcore/gtp_proxy/internal/metrics"
	"github.com/vectorcore/gtp_proxy/internal/routing"
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
	bound    map[string]gtpcBoundDomain
	started  map[string]bool
}

type gtpcBoundDomain struct {
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
		bound:    map[string]gtpcBoundDomain{},
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
	packet, err := ParsePacket(data)
	if err != nil {
		s.metrics.IncMessageError("gtpc", "parse")
		return err
	}

	if tx, ok := s.sessions.PopPending(ingressDomain, src.String(), packet.Sequence, packet.MessageType); ok {
		return s.handleResponse(packet, tx)
	}

	switch packet.MessageType {
	case messageTypeEchoRequest:
		resp := EchoResponse(packet)
		err := s.writeToDomain(ingressDomain, resp.Marshal(), src)
		return err
	case messageTypeCreateSessionRequest:
		return s.handleCreateSessionRequest(ingressDomain, src, packet)
	case messageTypeModifyBearerRequest:
		return s.handleModifyBearerRequest(ingressDomain, src, packet)
	case messageTypeCreateBearerRequest:
		return s.handleBearerLikeRequest(ingressDomain, src, packet, messageTypeCreateBearerResp, "create_bearer_request")
	case messageTypeUpdateBearerRequest:
		return s.handleBearerLikeRequest(ingressDomain, src, packet, messageTypeUpdateBearerResp, "update_bearer_request")
	case messageTypeDeleteBearerRequest:
		return s.handleBearerLikeRequest(ingressDomain, src, packet, messageTypeDeleteBearerResp, "delete_bearer_request")
	case messageTypeReleaseAccessReq:
		return s.handleGenericControlRequest(ingressDomain, src, packet, messageTypeReleaseAccessResp, "release_access_bearers_request")
	case messageTypeDeleteSessionRequest:
		return s.handleDeleteSessionRequest(ingressDomain, src, packet)
	default:
		s.metrics.IncMessageError("gtpc", fmt.Sprintf("unsupported_%d", packet.MessageType))
		return fmt.Errorf("unsupported message type %d", packet.MessageType)
	}
}

func (s *Server) handleCreateSessionRequest(ingressDomain string, src *net.UDPAddr, packet Packet) error {
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
	resolvedTarget, err := discovery.Resolve(context.Background(), cfg, match)
	if err != nil {
		s.metrics.IncMessageError("gtpc", "create_session_request")
		return fmt.Errorf("resolve route target: %w", err)
	}
	target, err := net.ResolveUDPAddr("udp", resolvedTarget.ControlEndpoint)
	if err != nil {
		s.metrics.IncMessageError("gtpc", "create_session_request")
		return fmt.Errorf("resolve home peer: %w", err)
	}

	sessionTTL := cfg.Proxy.Timeouts.SessionIdleDuration()
	originalPayload := packet.Payload
	routePeer := match.Peer.Name
	if routePeer == "" {
		routePeer = resolvedTarget.FQDN
	}
	sess := s.sessions.Create(src, target.String(), imsi, apn, match.MatchType, match.MatchValue, routePeer, match.ActionType, ingressDomain, match.TransportDomain, resolvedTarget.FQDN, resolvedTarget.Method, visitedTEID, sessionTTL)
	sess = s.captureVisitedUserPlane(sess, originalPayload, sessionTTL)
	payload, err := RewriteFTEIDs(originalPayload, func(index int, current FTEID) (FTEID, bool, error) {
		next := current
		if isControlPlaneInterfaceType(current.InterfaceType) {
			effectiveGTPC, ok := gtpcConfigForDomain(cfg, match.TransportDomain)
			if !ok {
				return FTEID{}, false, fmt.Errorf("missing GTPC runtime config for transport domain %q", match.TransportDomain)
			}
			if err := rewriteAdvertiseAddresses(&next, current, effectiveGTPC); err != nil {
				return FTEID{}, false, err
			}
			next.TEID = sess.ProxyVisitedControlTEID
		} else if isUserPlaneInterfaceType(current.InterfaceType) {
			effectiveGTPU, ok := gtpuConfigForDomain(cfg, match.TransportDomain)
			if !ok {
				return FTEID{}, false, fmt.Errorf("missing GTPU runtime config for transport domain %q", match.TransportDomain)
			}
			if err := rewriteAdvertiseAddresses(&next, current, effectiveGTPU); err != nil {
				return FTEID{}, false, err
			}
			if sess.ProxyVisitedUserTEID != 0 {
				next.TEID = sess.ProxyVisitedUserTEID
			} else {
				next.TEID = s.sessions.AllocateTEID()
			}
		} else {
			return next, false, nil
		}
		return next, true, nil
	})
	if err != nil {
		s.metrics.IncMessageError("gtpc", "create_session_request")
		return fmt.Errorf("rewrite request F-TEIDs: %w", err)
	}
	packet.Payload = payload

	s.sessions.AddPending(session.PendingTransaction{
		PeerDomain:         match.TransportDomain,
		PeerAddr:           target.String(),
		Sequence:           packet.Sequence,
		MessageType:        messageTypeCreateSessionResp,
		OriginalPeerDomain: ingressDomain,
		OriginalPeerAddr:   src.String(),
		SessionID:          sess.ID,
	})

	s.logger.Info("forwarding create-session request",
		"imsi", sess.IMSI,
		"apn", sess.APN,
		"src", src.String(),
		"dst", target.String(),
		"route_type", match.MatchType,
		"route_value", match.MatchValue,
		"route_peer", routePeer,
		"route_action", match.ActionType,
		"egress_transport_domain", match.TransportDomain,
		"resolved_target", target.String(),
		"inbound_teid", visitedTEID,
		"outbound_teid", sess.ProxyVisitedControlTEID,
		"session_id", sess.ID,
	)
	err = s.writeToDomain(match.TransportDomain, packet.Marshal(), target)
	if err == nil {
		s.metrics.IncPeerControlPlanePacket(routePeer)
	} else {
		s.sessions.DeletePending(match.TransportDomain, target.String(), packet.Sequence, messageTypeCreateSessionResp)
		s.sessions.Delete(sess.ID)
		s.metrics.IncMessageError("gtpc", "create_session_request")
	}
	return err
}

func (s *Server) handleModifyBearerRequest(ingressDomain string, src *net.UDPAddr, packet Packet) error {
	cfg := s.cfg.Snapshot()
	sessionTTL := cfg.Proxy.Timeouts.SessionIdleDuration()
	sess, ok := s.sessions.GetByProxyHomeControlTEIDFromPeer(packet.TEID, ingressDomain, src.String())
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
	sess = s.captureVisitedUserPlane(sess, originalPayload, sessionTTL)
	payload, err := RewriteFTEIDs(originalPayload, func(index int, current FTEID) (FTEID, bool, error) {
		next := current
		effectiveGTPU, ok := gtpuConfigForDomain(cfg, sess.EgressTransportDomain)
		if !ok {
			return FTEID{}, false, fmt.Errorf("missing effective GTPU runtime config")
		}
		if err := rewriteAdvertiseAddresses(&next, current, effectiveGTPU); err != nil {
			return FTEID{}, false, err
		}
		if !isUserPlaneInterfaceType(current.InterfaceType) {
			return next, false, nil
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
	_, _ = s.sessions.Touch(sess.ID, sessionTTL)

	s.sessions.AddPending(session.PendingTransaction{
		PeerDomain:         sess.EgressTransportDomain,
		PeerAddr:           target.String(),
		Sequence:           packet.Sequence,
		MessageType:        messageTypeModifyBearerResp,
		OriginalPeerDomain: ingressDomain,
		OriginalPeerAddr:   src.String(),
		SessionID:          sess.ID,
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
	err = s.writeToDomain(sess.EgressTransportDomain, packet.Marshal(), target)
	if err == nil {
		s.metrics.IncPeerControlPlanePacket(sess.RoutePeer)
	} else {
		s.sessions.DeletePending(sess.EgressTransportDomain, target.String(), packet.Sequence, messageTypeModifyBearerResp)
		s.metrics.IncMessageError("gtpc", "modify_bearer_request")
	}
	return err
}

func (s *Server) handleDeleteSessionRequest(ingressDomain string, src *net.UDPAddr, packet Packet) error {
	sess, ok := s.sessions.GetByProxyHomeControlTEIDFromPeer(packet.TEID, ingressDomain, src.String())
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
		PeerDomain:         sess.EgressTransportDomain,
		PeerAddr:           target.String(),
		Sequence:           packet.Sequence,
		MessageType:        messageTypeDeleteSessionResp,
		OriginalPeerDomain: ingressDomain,
		OriginalPeerAddr:   src.String(),
		SessionID:          sess.ID,
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
	err = s.writeToDomain(sess.EgressTransportDomain, packet.Marshal(), target)
	if err == nil {
		s.metrics.IncPeerControlPlanePacket(sess.RoutePeer)
	} else {
		s.sessions.DeletePending(sess.EgressTransportDomain, target.String(), packet.Sequence, messageTypeDeleteSessionResp)
		s.metrics.IncMessageError("gtpc", "delete_session_request")
	}
	return err
}

func (s *Server) handleResponse(packet Packet, tx session.PendingTransaction) error {
	switch packet.MessageType {
	case messageTypeCreateSessionResp:
		return s.handleCreateSessionResponse(packet, tx)
	case messageTypeModifyBearerResp:
		return s.handleModifyBearerResponse(packet, tx)
	case messageTypeCreateBearerResp:
		return s.handleBearerLikeResponse(packet, tx, "create_bearer_response")
	case messageTypeUpdateBearerResp:
		return s.handleBearerLikeResponse(packet, tx, "update_bearer_response")
	case messageTypeDeleteBearerResp:
		return s.handleGenericControlResponse(packet, tx, "delete_bearer_response")
	case messageTypeReleaseAccessResp:
		return s.handleGenericControlResponse(packet, tx, "release_access_bearers_response")
	case messageTypeDeleteSessionResp:
		return s.handleDeleteSessionResponse(packet, tx)
	default:
		return fmt.Errorf("unexpected response message type %d", packet.MessageType)
	}
}

func (s *Server) handleCreateSessionResponse(packet Packet, tx session.PendingTransaction) error {
	sess, ok := s.sessions.Get(tx.SessionID)
	if !ok {
		return fmt.Errorf("session %q not found", tx.SessionID)
	}
	cfg := s.cfg.Snapshot()
	sessionTTL := cfg.Proxy.Timeouts.SessionIdleDuration()
	originalPayload := packet.Payload
	if homeFTEID, ok := ExtractFirstControlPlaneFTEID(originalPayload); ok {
		updated, err := s.sessions.BindHomeControl(sess.ID, homeFTEID.TEID, sessionTTL)
		if err != nil {
			return err
		}
		sess = updated
	}
	if cause, ok := ExtractCause(originalPayload); !ok || createSessionAccepted(cause) {
		sess = s.captureHomeUserPlane(sess, originalPayload, sessionTTL)
	}

	payload, err := RewriteFTEIDs(originalPayload, func(index int, current FTEID) (FTEID, bool, error) {
		next := current
		if isControlPlaneInterfaceType(current.InterfaceType) {
			effectiveGTPC, ok := gtpcConfigForDomain(cfg, tx.OriginalPeerDomain)
			if !ok {
				return FTEID{}, false, fmt.Errorf("missing GTPC runtime config for transport domain %q", tx.OriginalPeerDomain)
			}
			if err := rewriteAdvertiseAddresses(&next, current, effectiveGTPC); err != nil {
				return FTEID{}, false, err
			}
			if sess.ProxyHomeControlTEID != 0 {
				next.TEID = sess.ProxyHomeControlTEID
			}
		} else if isUserPlaneInterfaceType(current.InterfaceType) {
			effectiveGTPU, ok := gtpuConfigForDomain(cfg, tx.OriginalPeerDomain)
			if !ok {
				return FTEID{}, false, fmt.Errorf("missing GTPU runtime config for transport domain %q", tx.OriginalPeerDomain)
			}
			if err := rewriteAdvertiseAddresses(&next, current, effectiveGTPU); err != nil {
				return FTEID{}, false, err
			}
			if sess.ProxyHomeUserTEID != 0 {
				next.TEID = sess.ProxyHomeUserTEID
			} else {
				next.TEID = s.sessions.AllocateTEID()
			}
		} else {
			return next, false, nil
		}
		return next, true, nil
	})
	if err != nil {
		return fmt.Errorf("rewrite response F-TEIDs: %w", err)
	}
	packet.Payload = payload
	packet.TEID = sess.VisitedControlTEID
	if cause, ok := ExtractCause(originalPayload); !ok || createSessionAccepted(cause) {
		_, _ = s.sessions.Touch(sess.ID, sessionTTL)
	}

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
	err = s.writeToDomain(tx.OriginalPeerDomain, packet.Marshal(), dst)
	if err != nil {
		return err
	}
	if cause, ok := ExtractCause(originalPayload); ok && !createSessionAccepted(cause) {
		s.sessions.Delete(sess.ID)
		return nil
	}
	s.metrics.IncSessionCreate()
	return nil
}

func (s *Server) handleModifyBearerResponse(packet Packet, tx session.PendingTransaction) error {
	sess, ok := s.sessions.Get(tx.SessionID)
	if !ok {
		return fmt.Errorf("session %q not found", tx.SessionID)
	}
	cfg := s.cfg.Snapshot()
	sessionTTL := cfg.Proxy.Timeouts.SessionIdleDuration()
	originalPayload := packet.Payload
	sess = s.captureVisitedUserPlane(sess, originalPayload, sessionTTL)
	payload, err := RewriteFTEIDs(originalPayload, func(index int, current FTEID) (FTEID, bool, error) {
		next := current
		effectiveGTPU, ok := gtpuConfigForDomain(cfg, tx.OriginalPeerDomain)
		if !ok {
			return FTEID{}, false, fmt.Errorf("missing effective GTPU runtime config")
		}
		if err := rewriteAdvertiseAddresses(&next, current, effectiveGTPU); err != nil {
			return FTEID{}, false, err
		}
		if !isUserPlaneInterfaceType(current.InterfaceType) {
			return next, false, nil
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
	err = s.writeToDomain(tx.OriginalPeerDomain, packet.Marshal(), dst)
	return err
}

func (s *Server) handleDeleteSessionResponse(packet Packet, tx session.PendingTransaction) error {
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
	err = s.writeToDomain(tx.OriginalPeerDomain, packet.Marshal(), dst)
	s.sessions.Delete(sess.ID)
	s.metrics.IncSessionDelete()
	return err
}

func (s *Server) handleBearerLikeRequest(ingressDomain string, src *net.UDPAddr, packet Packet, responseMessageType uint8, metricLabel string) error {
	cfg := s.cfg.Snapshot()
	sessionTTL := cfg.Proxy.Timeouts.SessionIdleDuration()
	sess, ok := s.sessions.GetByProxyHomeControlTEIDFromPeer(packet.TEID, ingressDomain, src.String())
	if !ok {
		s.metrics.IncMessageError("gtpc", metricLabel)
		return fmt.Errorf("unknown proxy TEID %d", packet.TEID)
	}
	target, err := net.ResolveUDPAddr("udp", sess.HomeControlEndpoint)
	if err != nil {
		s.metrics.IncMessageError("gtpc", metricLabel)
		return fmt.Errorf("resolve home peer: %w", err)
	}
	originalPayload := packet.Payload
	payload, err := RewriteFTEIDs(originalPayload, func(index int, current FTEID) (FTEID, bool, error) {
		next := current
		effectiveGTPU, ok := gtpuConfigForDomain(cfg, sess.EgressTransportDomain)
		if !ok {
			return FTEID{}, false, fmt.Errorf("missing effective GTPU runtime config")
		}
		if err := rewriteAdvertiseAddresses(&next, current, effectiveGTPU); err != nil {
			return FTEID{}, false, err
		}
		if !isUserPlaneInterfaceType(current.InterfaceType) {
			return next, false, nil
		}
		if sess.ProxyVisitedUserTEID != 0 {
			next.TEID = sess.ProxyVisitedUserTEID
		} else {
			next.TEID = s.sessions.AllocateTEID()
		}
		return next, true, nil
	})
	if err != nil {
		s.metrics.IncMessageError("gtpc", metricLabel)
		return fmt.Errorf("rewrite bearer-like request F-TEIDs: %w", err)
	}
	packet.Payload = payload
	packet.TEID = sess.HomeControlTEID
	_, _ = s.sessions.Touch(sess.ID, sessionTTL)
	s.sessions.AddPending(session.PendingTransaction{
		PeerDomain:         sess.EgressTransportDomain,
		PeerAddr:           target.String(),
		Sequence:           packet.Sequence,
		MessageType:        responseMessageType,
		OriginalPeerDomain: ingressDomain,
		OriginalPeerAddr:   src.String(),
		SessionID:          sess.ID,
	})
	err = s.writeToDomain(sess.EgressTransportDomain, packet.Marshal(), target)
	if err == nil {
		s.metrics.IncPeerControlPlanePacket(sess.RoutePeer)
	} else {
		s.sessions.DeletePending(sess.EgressTransportDomain, target.String(), packet.Sequence, responseMessageType)
		s.metrics.IncMessageError("gtpc", metricLabel)
	}
	return err
}

func (s *Server) handleGenericControlRequest(ingressDomain string, src *net.UDPAddr, packet Packet, responseMessageType uint8, metricLabel string) error {
	sess, ok := s.sessions.GetByProxyHomeControlTEIDFromPeer(packet.TEID, ingressDomain, src.String())
	if !ok {
		s.metrics.IncMessageError("gtpc", metricLabel)
		return fmt.Errorf("unknown proxy TEID %d", packet.TEID)
	}
	target, err := net.ResolveUDPAddr("udp", sess.HomeControlEndpoint)
	if err != nil {
		s.metrics.IncMessageError("gtpc", metricLabel)
		return fmt.Errorf("resolve home peer: %w", err)
	}
	packet.TEID = sess.HomeControlTEID
	s.sessions.AddPending(session.PendingTransaction{
		PeerDomain:         sess.EgressTransportDomain,
		PeerAddr:           target.String(),
		Sequence:           packet.Sequence,
		MessageType:        responseMessageType,
		OriginalPeerDomain: ingressDomain,
		OriginalPeerAddr:   src.String(),
		SessionID:          sess.ID,
	})
	err = s.writeToDomain(sess.EgressTransportDomain, packet.Marshal(), target)
	if err == nil {
		s.metrics.IncPeerControlPlanePacket(sess.RoutePeer)
	} else {
		s.sessions.DeletePending(sess.EgressTransportDomain, target.String(), packet.Sequence, responseMessageType)
		s.metrics.IncMessageError("gtpc", metricLabel)
	}
	return err
}

func (s *Server) handleBearerLikeResponse(packet Packet, tx session.PendingTransaction, metricLabel string) error {
	sess, ok := s.sessions.Get(tx.SessionID)
	if !ok {
		return fmt.Errorf("session %q not found", tx.SessionID)
	}
	cfg := s.cfg.Snapshot()
	sessionTTL := cfg.Proxy.Timeouts.SessionIdleDuration()
	originalPayload := packet.Payload
	sess = s.captureHomeUserPlane(sess, originalPayload, sessionTTL)
	payload, err := RewriteFTEIDs(originalPayload, func(index int, current FTEID) (FTEID, bool, error) {
		next := current
		effectiveGTPU, ok := gtpuConfigForDomain(cfg, tx.OriginalPeerDomain)
		if !ok {
			return FTEID{}, false, fmt.Errorf("missing effective GTPU runtime config")
		}
		if err := rewriteAdvertiseAddresses(&next, current, effectiveGTPU); err != nil {
			return FTEID{}, false, err
		}
		if !isUserPlaneInterfaceType(current.InterfaceType) {
			return next, false, nil
		}
		if sess.ProxyHomeUserTEID != 0 {
			next.TEID = sess.ProxyHomeUserTEID
		} else {
			next.TEID = s.sessions.AllocateTEID()
		}
		return next, true, nil
	})
	if err != nil {
		s.metrics.IncMessageError("gtpc", metricLabel)
		return fmt.Errorf("rewrite bearer-like response F-TEIDs: %w", err)
	}
	packet.Payload = payload
	packet.TEID = sess.VisitedControlTEID
	_, _ = s.sessions.Touch(sess.ID, sessionTTL)
	dst, err := net.ResolveUDPAddr("udp", tx.OriginalPeerAddr)
	if err != nil {
		return fmt.Errorf("resolve visited peer: %w", err)
	}
	err = s.writeToDomain(tx.OriginalPeerDomain, packet.Marshal(), dst)
	return err
}

func (s *Server) handleGenericControlResponse(packet Packet, tx session.PendingTransaction, metricLabel string) error {
	sess, ok := s.sessions.Get(tx.SessionID)
	if !ok {
		return fmt.Errorf("session %q not found", tx.SessionID)
	}
	packet.TEID = sess.VisitedControlTEID
	dst, err := net.ResolveUDPAddr("udp", tx.OriginalPeerAddr)
	if err != nil {
		return fmt.Errorf("resolve visited peer: %w", err)
	}
	err = s.writeToDomain(tx.OriginalPeerDomain, packet.Marshal(), dst)
	if err != nil {
		s.metrics.IncMessageError("gtpc", metricLabel)
	}
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
	if err != nil {
		return sess
	}
	fteid, ok := selectUserPlaneFTEID(fteids)
	if !ok {
		return sess
	}
	endpoint := userPlaneEndpoint(fteid)
	if endpoint == "" {
		return sess
	}
	updated, err := s.sessions.UpsertVisitedUserPlane(sess.ID, endpoint, fteid.TEID, ttl)
	if err != nil {
		return sess
	}
	return updated
}

func (s *Server) captureHomeUserPlane(sess *session.Session, payload []byte, ttl time.Duration) *session.Session {
	fteids, err := ExtractAllFTEIDs(payload)
	if err != nil {
		return sess
	}
	fteid, ok := selectUserPlaneFTEID(fteids)
	if !ok {
		return sess
	}
	endpoint := userPlaneEndpoint(fteid)
	if endpoint == "" {
		return sess
	}
	updated, err := s.sessions.UpsertHomeUserPlane(sess.ID, endpoint, fteid.TEID, ttl)
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

func selectUserPlaneFTEID(fteids []FTEID) (FTEID, bool) {
	for _, fteid := range fteids {
		if isUserPlaneInterfaceType(fteid.InterfaceType) {
			return fteid, true
		}
	}
	return FTEID{}, false
}

func createSessionAccepted(cause uint8) bool {
	return cause == 16
}

func (s *Server) reconcile(ctx context.Context, cfg config.Config) {
	desired := desiredGTPCDomains(cfg)

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
				s.runtime.SetGTPC(transport.ListenerStatus{
					Protocol: "gtpc",
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
				s.runtime.SetGTPC(transport.ListenerStatus{
					Protocol:      "gtpc",
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
				s.runtime.SetGTPC(transport.ListenerStatus{
					Protocol:      "gtpc",
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
			s.runtime.SetGTPC(transport.ListenerStatus{
				Protocol: "gtpc",
				State:    "active",
				Domain:   domain,
				Listen:   bound.listen,
			})
		}
		s.logger.Info("gtpc listener started", "listen", bound.listen, "domain", domain, "netns_path", bound.netns)
	}
}

func (s *Server) readLoop(ctx context.Context, domain string, conn *net.UDPConn) {
	buf := make([]byte, 64*1024)
	for {
		if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			s.setDomainError(domain, conn, "", fmt.Errorf("set GTPC read deadline: %w", err))
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
			s.setDomainError(domain, conn, "", fmt.Errorf("read GTPC packet: %w", err))
			return
		}
		if err := s.handlePacket(domain, src, buf[:n]); err != nil {
			s.logger.Warn("gtpc packet handling failed", "domain", domain, "src", src.String(), "err", err)
		}
	}
}

func (s *Server) writeToDomain(domain string, data []byte, target *net.UDPAddr) error {
	conn, err := s.connForDomain(domain)
	if err != nil {
		return err
	}
	_, err = conn.WriteToUDP(data, target)
	return err
}

func (s *Server) connForDomain(domain string) (*net.UDPConn, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	conn, ok := s.conns[domain]
	if ok && conn != nil {
		return conn, nil
	}
	return nil, fmt.Errorf("no active GTPC listener for transport domain %q", domain)
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
		s.runtime.SetGTPC(transport.ListenerStatus{
			Protocol:      "gtpc",
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
	s.bound = map[string]gtpcBoundDomain{}
	s.started = map[string]bool{}
}

func desiredGTPCDomains(cfg config.Config) map[string]gtpcBoundDomain {
	out := map[string]gtpcBoundDomain{}
	if len(cfg.TransportDomains) == 0 {
		if effective, ok := cfg.EffectiveGTPCConfig(); ok {
			out[""] = gtpcBoundDomain{listen: effective.Listen}
		}
		return out
	}
	for _, domain := range cfg.TransportDomains {
		if !domain.Enabled {
			continue
		}
		out[domain.Name] = gtpcBoundDomain{
			name:   domain.Name,
			netns:  domain.NetNSPath,
			listen: net.JoinHostPort(domain.GTPCListenHost, fmt.Sprintf("%d", domain.GTPCPort)),
		}
	}
	return out
}

func gtpcConfigForDomain(cfg config.Config, domainName string) (config.GTPCConfig, bool) {
	if len(cfg.TransportDomains) == 0 {
		return cfg.EffectiveGTPCConfig()
	}
	domain, ok := cfg.TransportDomainByName(domainName)
	if !ok || !domain.Enabled {
		return config.GTPCConfig{}, false
	}
	return config.GTPCConfig{
		Listen:               net.JoinHostPort(domain.GTPCListenHost, fmt.Sprintf("%d", domain.GTPCPort)),
		AdvertiseAddressIPv4: domain.GTPCAdvertiseIPv4,
		AdvertiseAddressIPv6: domain.GTPCAdvertiseIPv6,
	}, true
}

func gtpuConfigForDomain(cfg config.Config, domainName string) (config.GTPUConfig, bool) {
	if len(cfg.TransportDomains) == 0 {
		return cfg.EffectiveGTPUConfig()
	}
	domain, ok := cfg.TransportDomainByName(domainName)
	if !ok || !domain.Enabled {
		return config.GTPUConfig{}, false
	}
	return config.GTPUConfig{
		Listen:               net.JoinHostPort(domain.GTPUListenHost, fmt.Sprintf("%d", domain.GTPUPort)),
		AdvertiseAddressIPv4: domain.GTPUAdvertiseIPv4,
		AdvertiseAddressIPv6: domain.GTPUAdvertiseIPv6,
	}, true
}
