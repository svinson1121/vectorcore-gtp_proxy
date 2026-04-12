package routing

import (
	"testing"

	"github.com/vectorcore/gtp_proxy/internal/config"
)

func TestSelectUsesAPNThenDefault(t *testing.T) {
	cfg := config.Config{
		Peers: []config.PeerConfig{
			{Name: "default", Address: "127.0.0.1:3123", Enabled: true},
			{Name: "ims", Address: "127.0.0.1:4123", Enabled: true},
		},
		Routing: config.RoutingConfig{
			DefaultPeer: "default",
			APNRoutes: []config.APNRoute{
				{APN: "ims", Peer: "ims"},
			},
		},
	}

	match, err := Select(cfg, Input{APN: "ims"})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if match.Peer.Name != "ims" || match.MatchType != "apn" {
		t.Fatalf("unexpected APN match %#v", match)
	}

	match, err = Select(cfg, Input{APN: "internet"})
	if err != nil {
		t.Fatalf("Select() default error = %v", err)
	}
	if match.Peer.Name != "default" || match.MatchType != "default" {
		t.Fatalf("unexpected default match %#v", match)
	}
}

func TestSelectUsesPhaseThreePrecedence(t *testing.T) {
	cfg := config.Config{
		Peers: []config.PeerConfig{
			{Name: "default", Address: "127.0.0.1:3123", Enabled: true},
			{Name: "imsi", Address: "127.0.0.1:4123", Enabled: true},
			{Name: "prefix", Address: "127.0.0.1:5123", Enabled: true},
			{Name: "apn", Address: "127.0.0.1:6123", Enabled: true},
			{Name: "plmn", Address: "127.0.0.1:7123", Enabled: true},
		},
		Routing: config.RoutingConfig{
			DefaultPeer: "default",
			IMSIRoutes: []config.IMSIRoute{
				{IMSI: "001010123456789", Peer: "imsi"},
			},
			IMSIPrefixRoutes: []config.IMSIPrefixRoute{
				{Prefix: "001010", Peer: "prefix"},
			},
			APNRoutes: []config.APNRoute{
				{APN: "internet", Peer: "apn"},
			},
			PLMNRoutes: []config.PLMNRoute{
				{PLMN: "00101", Peer: "plmn"},
			},
		},
	}

	match, err := Select(cfg, Input{IMSI: "001010123456789", APN: "internet"})
	if err != nil {
		t.Fatalf("Select() exact IMSI error = %v", err)
	}
	if match.Peer.Name != "imsi" || match.MatchType != "imsi" {
		t.Fatalf("unexpected IMSI match %#v", match)
	}

	match, err = Select(cfg, Input{IMSI: "001010999999999", APN: "internet"})
	if err != nil {
		t.Fatalf("Select() prefix error = %v", err)
	}
	if match.Peer.Name != "prefix" || match.MatchType != "imsi_prefix" {
		t.Fatalf("unexpected IMSI prefix match %#v", match)
	}

	match, err = Select(cfg, Input{IMSI: "999999123456789", APN: "internet"})
	if err != nil {
		t.Fatalf("Select() APN error = %v", err)
	}
	if match.Peer.Name != "apn" || match.MatchType != "apn" {
		t.Fatalf("unexpected APN match %#v", match)
	}

	match, err = Select(cfg, Input{IMSI: "001019123456789", APN: "unknown"})
	if err != nil {
		t.Fatalf("Select() PLMN error = %v", err)
	}
	if match.Peer.Name != "plmn" || match.MatchType != "plmn" {
		t.Fatalf("unexpected PLMN match %#v", match)
	}
}
