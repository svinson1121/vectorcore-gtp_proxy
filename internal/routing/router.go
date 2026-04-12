package routing

import (
	"fmt"
	"strings"

	"github.com/vectorcore/gtp_proxy/internal/config"
)

type Match struct {
	Peer       config.PeerConfig `json:"peer"`
	MatchType  string            `json:"match_type"`
	MatchValue string            `json:"match_value"`
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
			peer, ok := peers[route.Peer]
			if !ok {
				return Match{}, fmt.Errorf("IMSI route %q references unavailable peer %q", route.IMSI, route.Peer)
			}
			return Match{Peer: peer, MatchType: "imsi", MatchValue: route.IMSI}, nil
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
		peer, ok := peers[prefixMatch.Peer]
		if !ok {
			return Match{}, fmt.Errorf("IMSI prefix route %q references unavailable peer %q", prefixMatch.Prefix, prefixMatch.Peer)
		}
		return Match{Peer: peer, MatchType: "imsi_prefix", MatchValue: prefixMatch.Prefix}, nil
	}

	needle := strings.ToLower(strings.TrimSpace(input.APN))
	for _, route := range cfg.Routing.APNRoutes {
		if strings.ToLower(strings.TrimSpace(route.APN)) == needle {
			peer, ok := peers[route.Peer]
			if !ok {
				return Match{}, fmt.Errorf("APN route %q references unavailable peer %q", route.APN, route.Peer)
			}
			return Match{Peer: peer, MatchType: "apn", MatchValue: route.APN}, nil
		}
	}

	for _, plmn := range plmnCandidates(imsi) {
		for _, route := range cfg.Routing.PLMNRoutes {
			if normalizeDigits(route.PLMN) == plmn {
				peer, ok := peers[route.Peer]
				if !ok {
					return Match{}, fmt.Errorf("PLMN route %q references unavailable peer %q", route.PLMN, route.Peer)
				}
				return Match{Peer: peer, MatchType: "plmn", MatchValue: route.PLMN}, nil
			}
		}
	}

	peer, ok := peers[cfg.Routing.DefaultPeer]
	if !ok {
		return Match{}, fmt.Errorf("default peer %q is unavailable", cfg.Routing.DefaultPeer)
	}
	return Match{Peer: peer, MatchType: "default", MatchValue: cfg.Routing.DefaultPeer}, nil
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
