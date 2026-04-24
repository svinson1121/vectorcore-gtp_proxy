package routing

import (
	"fmt"
	"strings"

	"github.com/vectorcore/gtp_proxy/internal/config"
)

type Match struct {
	Peer            config.PeerConfig `json:"peer"`
	ActionType      string            `json:"action_type"`
	TransportDomain string            `json:"transport_domain,omitempty"`
	FQDN            string            `json:"fqdn,omitempty"`
	Service         string            `json:"service,omitempty"`
	MatchType       string            `json:"match_type"`
	MatchValue      string            `json:"match_value"`
}

type Input struct {
	IMSI string
	APN  string
}

func Select(cfg config.Config, input Input) (Match, error) {
	peers := make(map[string]config.PeerConfig, len(cfg.Peers))
	for _, peer := range cfg.Peers {
		if peer.Enabled {
			peers[peer.Name] = peer
		}
	}

	imsi := normalizeDigits(input.IMSI)
	for _, route := range cfg.Routing.IMSIRoutes {
		if normalizeDigits(route.IMSI) == imsi && imsi != "" {
			peer, err := peerForRoute(peers, route.Peer, route.ActionType, "IMSI route", route.IMSI)
			if err != nil {
				return Match{}, err
			}
			return matchFromRoute(peer, route.TransportDomain, route.FQDN, route.Service, route.ActionType, "imsi", route.IMSI), nil
		}
	}

	var prefixMatch *config.IMSIPrefixRoute
	for i := range cfg.Routing.IMSIPrefixRoutes {
		prefix := normalizeDigits(cfg.Routing.IMSIPrefixRoutes[i].Prefix)
		if prefix != "" && strings.HasPrefix(imsi, prefix) {
			if prefixMatch == nil || len(prefix) > len(normalizeDigits(prefixMatch.Prefix)) {
				prefixMatch = &cfg.Routing.IMSIPrefixRoutes[i]
			}
		}
	}
	if prefixMatch != nil {
		peer, err := peerForRoute(peers, prefixMatch.Peer, prefixMatch.ActionType, "IMSI prefix route", prefixMatch.Prefix)
		if err != nil {
			return Match{}, err
		}
		return matchFromRoute(peer, prefixMatch.TransportDomain, prefixMatch.FQDN, prefixMatch.Service, prefixMatch.ActionType, "imsi_prefix", prefixMatch.Prefix), nil
	}

	needle := strings.ToLower(strings.TrimSpace(input.APN))
	for _, route := range cfg.Routing.APNRoutes {
		if strings.ToLower(strings.TrimSpace(route.APN)) == needle {
			peer, err := peerForRoute(peers, route.Peer, route.ActionType, "APN route", route.APN)
			if err != nil {
				return Match{}, err
			}
			return matchFromRoute(peer, route.TransportDomain, route.FQDN, route.Service, route.ActionType, "apn", route.APN), nil
		}
	}

	for _, plmn := range plmnCandidates(imsi) {
		for _, route := range cfg.Routing.PLMNRoutes {
			if normalizeDigits(route.PLMN) == plmn {
				peer, err := peerForRoute(peers, route.Peer, route.ActionType, "PLMN route", route.PLMN)
				if err != nil {
					return Match{}, err
				}
				return matchFromRoute(peer, route.TransportDomain, route.FQDN, route.Service, route.ActionType, "plmn", route.PLMN), nil
			}
		}
	}

	peer, ok := peers[cfg.Routing.DefaultPeer]
	if !ok {
		return Match{}, fmt.Errorf("default peer %q is unavailable", cfg.Routing.DefaultPeer)
	}
	return Match{
		Peer:            peer,
		ActionType:      "static_peer",
		TransportDomain: peer.TransportDomain,
		MatchType:       "default",
		MatchValue:      cfg.Routing.DefaultPeer,
	}, nil
}

func peerForRoute(peers map[string]config.PeerConfig, peerName, actionType, label, value string) (config.PeerConfig, error) {
	if normalizeAction(actionType) == "dns_discovery" {
		return config.PeerConfig{}, nil
	}
	peer, ok := peers[peerName]
	if !ok {
		return config.PeerConfig{}, fmt.Errorf("%s %q references unavailable peer %q", label, value, peerName)
	}
	return peer, nil
}

func normalizeDigits(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func plmnCandidates(imsi string) []string {
	switch {
	case len(imsi) >= 6:
		return []string{imsi[:6], imsi[:5]}
	case len(imsi) == 5:
		return []string{imsi[:5]}
	default:
		return nil
	}
}

func matchFromRoute(peer config.PeerConfig, transportDomain, fqdn, service, actionType, matchType, matchValue string) Match {
	if normalizeAction(actionType) == "static_peer" && strings.TrimSpace(transportDomain) == "" {
		transportDomain = peer.TransportDomain
	}
	return Match{
		Peer:            peer,
		ActionType:      normalizeAction(actionType),
		TransportDomain: transportDomain,
		FQDN:            fqdn,
		Service:         service,
		MatchType:       matchType,
		MatchValue:      matchValue,
	}
}

func normalizeAction(actionType string) string {
	if strings.TrimSpace(actionType) == "" {
		return "static_peer"
	}
	return strings.ToLower(strings.TrimSpace(actionType))
}
