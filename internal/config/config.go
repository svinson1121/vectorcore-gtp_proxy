package config

import (
	"fmt"
	"net"
	"os"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Proxy    ProxyConfig    `yaml:"proxy" json:"proxy"`
	API      APIConfig      `yaml:"api" json:"api"`
	Log      LogConfig      `yaml:"log" json:"log"`
	Database DatabaseConfig `yaml:"database" json:"database"`
	Peers    []PeerConfig   `yaml:"peers,omitempty" json:"peers"`
	Routing  RoutingConfig  `yaml:"routing,omitempty" json:"routing"`
}

type ProxyConfig struct {
	GTPC     GTPCConfig     `yaml:"gtpc" json:"gtpc"`
	GTPU     GTPUConfig     `yaml:"gtpu" json:"gtpu"`
	Timeouts TimeoutsConfig `yaml:"timeouts" json:"timeouts"`
}

type GTPCConfig struct {
	Listen               string `yaml:"listen" json:"listen"`
	AdvertiseAddress     string `yaml:"advertise_address" json:"advertise_address"`
	AdvertiseAddressIPv4 string `yaml:"advertise_address_ipv4,omitempty" json:"advertise_address_ipv4,omitempty"`
	AdvertiseAddressIPv6 string `yaml:"advertise_address_ipv6,omitempty" json:"advertise_address_ipv6,omitempty"`
}

type GTPUConfig struct {
	Listen               string `yaml:"listen" json:"listen"`
	AdvertiseAddress     string `yaml:"advertise_address" json:"advertise_address"`
	AdvertiseAddressIPv4 string `yaml:"advertise_address_ipv4,omitempty" json:"advertise_address_ipv4,omitempty"`
	AdvertiseAddressIPv6 string `yaml:"advertise_address_ipv6,omitempty" json:"advertise_address_ipv6,omitempty"`
}

type TimeoutsConfig struct {
	SessionIdle     string `yaml:"session_idle" json:"session_idle"`
	CleanupInterval string `yaml:"cleanup_interval" json:"cleanup_interval"`
}

type APIConfig struct {
	Listen string `yaml:"listen" json:"listen"`
}

type LogConfig struct {
	Level string `yaml:"level" json:"level"`
}

type DatabaseConfig struct {
	Path string `yaml:"path" json:"path"`
}

type PeerConfig struct {
	Name        string `yaml:"name" json:"name"`
	Address     string `yaml:"address" json:"address"`
	Enabled     bool   `yaml:"enabled" json:"enabled"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

type RoutingConfig struct {
	DefaultPeer      string            `yaml:"default_peer" json:"default_peer"`
	IMSIRoutes       []IMSIRoute       `yaml:"imsi_routes,omitempty" json:"imsi_routes,omitempty"`
	IMSIPrefixRoutes []IMSIPrefixRoute `yaml:"imsi_prefix_routes,omitempty" json:"imsi_prefix_routes,omitempty"`
	APNRoutes        []APNRoute        `yaml:"apn_routes" json:"apn_routes"`
	PLMNRoutes       []PLMNRoute       `yaml:"plmn_routes,omitempty" json:"plmn_routes,omitempty"`
}

type APNRoute struct {
	APN  string `yaml:"apn" json:"apn"`
	Peer string `yaml:"peer" json:"peer"`
}

type IMSIRoute struct {
	IMSI string `yaml:"imsi" json:"imsi"`
	Peer string `yaml:"peer" json:"peer"`
}

type IMSIPrefixRoute struct {
	Prefix string `yaml:"prefix" json:"prefix"`
	Peer   string `yaml:"peer" json:"peer"`
}

type PLMNRoute struct {
	PLMN string `yaml:"plmn" json:"plmn"`
	Peer string `yaml:"peer" json:"peer"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	return Parse(data)
}

func Parse(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	cfg.ApplyDefaults()
	if err := cfg.ValidateBootstrap(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) ApplyDefaults() {
	if c.Proxy.GTPC.Listen == "" {
		c.Proxy.GTPC.Listen = "0.0.0.0:2123"
	}
	applyAdvertiseDefaults(&c.Proxy.GTPC.AdvertiseAddress, &c.Proxy.GTPC.AdvertiseAddressIPv4, &c.Proxy.GTPC.AdvertiseAddressIPv6)
	if c.Proxy.GTPU.Listen == "" {
		c.Proxy.GTPU.Listen = "0.0.0.0:2152"
	}
	if c.Proxy.GTPU.AdvertiseAddress == "" && c.Proxy.GTPC.AdvertiseAddress != "" {
		c.Proxy.GTPU.AdvertiseAddress = c.Proxy.GTPC.AdvertiseAddress
	}
	if c.Proxy.GTPU.AdvertiseAddressIPv4 == "" {
		c.Proxy.GTPU.AdvertiseAddressIPv4 = c.Proxy.GTPC.AdvertiseAddressIPv4
	}
	if c.Proxy.GTPU.AdvertiseAddressIPv6 == "" {
		c.Proxy.GTPU.AdvertiseAddressIPv6 = c.Proxy.GTPC.AdvertiseAddressIPv6
	}
	applyAdvertiseDefaults(&c.Proxy.GTPU.AdvertiseAddress, &c.Proxy.GTPU.AdvertiseAddressIPv4, &c.Proxy.GTPU.AdvertiseAddressIPv6)
	if c.Proxy.Timeouts.SessionIdle == "" {
		c.Proxy.Timeouts.SessionIdle = "15m"
	}
	if c.Proxy.Timeouts.CleanupInterval == "" {
		c.Proxy.Timeouts.CleanupInterval = "30s"
	}
	if c.API.Listen == "" {
		c.API.Listen = "0.0.0.0:8080"
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Database.Path == "" {
		c.Database.Path = "./gtp_proxy.db"
	}
	for i := range c.Peers {
		if c.Peers[i].Name == "" {
			continue
		}
		if c.Peers[i].Description == "" {
			c.Peers[i].Description = c.Peers[i].Name
		}
	}
}

func (c Config) ValidateBootstrap() error {
	if _, err := net.ResolveUDPAddr("udp", c.Proxy.GTPC.Listen); err != nil {
		return fmt.Errorf("proxy.gtpc.listen: %w", err)
	}
	if _, err := net.ResolveUDPAddr("udp", c.Proxy.GTPU.Listen); err != nil {
		return fmt.Errorf("proxy.gtpu.listen: %w", err)
	}
	if _, err := net.ResolveTCPAddr("tcp", c.API.Listen); err != nil {
		return fmt.Errorf("api.listen: %w", err)
	}
	if err := validateAdvertiseConfig("proxy.gtpc", c.Proxy.GTPC.AdvertiseAddress, c.Proxy.GTPC.AdvertiseAddressIPv4, c.Proxy.GTPC.AdvertiseAddressIPv6); err != nil {
		return err
	}
	if err := validateAdvertiseConfig("proxy.gtpu", c.Proxy.GTPU.AdvertiseAddress, c.Proxy.GTPU.AdvertiseAddressIPv4, c.Proxy.GTPU.AdvertiseAddressIPv6); err != nil {
		return err
	}
	if c.Proxy.Timeouts.SessionIdleDuration() <= 0 {
		return fmt.Errorf("proxy.timeouts.session_idle must be a positive duration")
	}
	if c.Proxy.Timeouts.CleanupIntervalDuration() <= 0 {
		return fmt.Errorf("proxy.timeouts.cleanup_interval must be a positive duration")
	}
	switch strings.ToLower(c.Log.Level) {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log.level must be one of debug, info, warn, error")
	}
	if strings.TrimSpace(c.Database.Path) == "" {
		return fmt.Errorf("database.path is required")
	}
	return nil
}

func (c Config) ValidateRuntime() error {
	if err := c.ValidateBootstrap(); err != nil {
		return err
	}
	return validateMutable(c.Peers, c.Routing)
}

func validateMutable(peers []PeerConfig, routing RoutingConfig) error {
	seenPeers := map[string]struct{}{}
	enabledPeers := map[string]struct{}{}
	for _, peer := range peers {
		if peer.Name == "" {
			return fmt.Errorf("peer name is required")
		}
		if _, ok := seenPeers[peer.Name]; ok {
			return fmt.Errorf("duplicate peer name %q", peer.Name)
		}
		seenPeers[peer.Name] = struct{}{}
		if _, err := net.ResolveUDPAddr("udp", peer.Address); err != nil {
			return fmt.Errorf("peer %q address: %w", peer.Name, err)
		}
		if peer.Enabled {
			enabledPeers[peer.Name] = struct{}{}
		}
	}
	if len(enabledPeers) == 0 {
		if routing.DefaultPeer != "" {
			return fmt.Errorf("routing.default_peer %q must reference an enabled peer", routing.DefaultPeer)
		}
	} else if routing.DefaultPeer != "" {
		if _, ok := enabledPeers[routing.DefaultPeer]; !ok {
			return fmt.Errorf("routing.default_peer %q must reference an enabled peer", routing.DefaultPeer)
		}
	}

	seenIMSIs := map[string]struct{}{}
	for _, route := range routing.IMSIRoutes {
		if route.IMSI == "" {
			return fmt.Errorf("routing.imsi_routes[].imsi is required")
		}
		if route.Peer == "" {
			return fmt.Errorf("routing.imsi_routes[%q].peer is required", route.IMSI)
		}
		imsi := normalizeDigits(route.IMSI)
		if imsi == "" {
			return fmt.Errorf("routing.imsi_routes[%q].imsi must contain digits", route.IMSI)
		}
		if _, ok := seenIMSIs[imsi]; ok {
			return fmt.Errorf("duplicate IMSI route %q", route.IMSI)
		}
		seenIMSIs[imsi] = struct{}{}
		if _, ok := enabledPeers[route.Peer]; !ok {
			return fmt.Errorf("routing.imsi_routes[%q].peer %q must reference an enabled peer", route.IMSI, route.Peer)
		}
	}

	seenPrefixes := map[string]struct{}{}
	for _, route := range routing.IMSIPrefixRoutes {
		if route.Prefix == "" {
			return fmt.Errorf("routing.imsi_prefix_routes[].prefix is required")
		}
		if route.Peer == "" {
			return fmt.Errorf("routing.imsi_prefix_routes[%q].peer is required", route.Prefix)
		}
		prefix := normalizeDigits(route.Prefix)
		if prefix == "" {
			return fmt.Errorf("routing.imsi_prefix_routes[%q].prefix must contain digits", route.Prefix)
		}
		if _, ok := seenPrefixes[prefix]; ok {
			return fmt.Errorf("duplicate IMSI prefix route %q", route.Prefix)
		}
		seenPrefixes[prefix] = struct{}{}
		if _, ok := enabledPeers[route.Peer]; !ok {
			return fmt.Errorf("routing.imsi_prefix_routes[%q].peer %q must reference an enabled peer", route.Prefix, route.Peer)
		}
	}

	seenAPNs := map[string]struct{}{}
	for _, route := range routing.APNRoutes {
		if route.APN == "" {
			return fmt.Errorf("routing.apn_routes[].apn is required")
		}
		if route.Peer == "" {
			return fmt.Errorf("routing.apn_routes[%q].peer is required", route.APN)
		}
		apn := normalizeAPN(route.APN)
		if _, ok := seenAPNs[apn]; ok {
			return fmt.Errorf("duplicate APN route %q", route.APN)
		}
		seenAPNs[apn] = struct{}{}
		if _, ok := enabledPeers[route.Peer]; !ok {
			return fmt.Errorf("routing.apn_routes[%q].peer %q must reference an enabled peer", route.APN, route.Peer)
		}
	}

	seenPLMNs := map[string]struct{}{}
	for _, route := range routing.PLMNRoutes {
		if route.PLMN == "" {
			return fmt.Errorf("routing.plmn_routes[].plmn is required")
		}
		if route.Peer == "" {
			return fmt.Errorf("routing.plmn_routes[%q].peer is required", route.PLMN)
		}
		plmn := normalizeDigits(route.PLMN)
		if len(plmn) != 5 && len(plmn) != 6 {
			return fmt.Errorf("routing.plmn_routes[%q].plmn must be 5 or 6 digits", route.PLMN)
		}
		if _, ok := seenPLMNs[plmn]; ok {
			return fmt.Errorf("duplicate PLMN route %q", route.PLMN)
		}
		seenPLMNs[plmn] = struct{}{}
		if _, ok := enabledPeers[route.Peer]; !ok {
			return fmt.Errorf("routing.plmn_routes[%q].peer %q must reference an enabled peer", route.PLMN, route.Peer)
		}
	}
	return nil
}

func (c Config) Clone() Config {
	out := c
	out.Peers = slices.Clone(c.Peers)
	out.Routing.IMSIRoutes = slices.Clone(c.Routing.IMSIRoutes)
	out.Routing.IMSIPrefixRoutes = slices.Clone(c.Routing.IMSIPrefixRoutes)
	out.Routing.APNRoutes = slices.Clone(c.Routing.APNRoutes)
	out.Routing.PLMNRoutes = slices.Clone(c.Routing.PLMNRoutes)
	return out
}

func (c Config) BootstrapOnly() Config {
	out := c.Clone()
	out.Peers = nil
	out.Routing = RoutingConfig{}
	return out
}

func normalizeAPN(apn string) string {
	return strings.ToLower(strings.TrimSpace(apn))
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

func (c GTPCConfig) AdvertiseIP(isIPv6 bool) (net.IP, bool) {
	return advertiseIP(c.AdvertiseAddressIPv4, c.AdvertiseAddressIPv6, isIPv6)
}

func (c GTPUConfig) AdvertiseIP(isIPv6 bool) (net.IP, bool) {
	return advertiseIP(c.AdvertiseAddressIPv4, c.AdvertiseAddressIPv6, isIPv6)
}

func (t TimeoutsConfig) SessionIdleDuration() time.Duration {
	d, _ := time.ParseDuration(t.SessionIdle)
	return d
}

func (t TimeoutsConfig) CleanupIntervalDuration() time.Duration {
	d, _ := time.ParseDuration(t.CleanupInterval)
	return d
}

func advertiseIP(ipv4, ipv6 string, isIPv6 bool) (net.IP, bool) {
	if isIPv6 {
		if ipv6 == "" {
			return nil, false
		}
		ip := net.ParseIP(ipv6)
		if ip == nil {
			return nil, false
		}
		return ip, true
	}
	if ipv4 == "" {
		return nil, false
	}
	ip := net.ParseIP(ipv4)
	if ip == nil {
		return nil, false
	}
	return ip.To4(), true
}

func applyAdvertiseDefaults(legacy, ipv4, ipv6 *string) {
	if *legacy == "" {
		return
	}
	if ip := net.ParseIP(*legacy); ip != nil {
		if v4 := ip.To4(); v4 != nil && *ipv4 == "" {
			*ipv4 = v4.String()
		}
		if v4 := ip.To4(); v4 == nil && *ipv6 == "" {
			*ipv6 = ip.String()
		}
	}
}

func validateAdvertiseConfig(prefix, legacy, ipv4, ipv6 string) error {
	if legacy != "" {
		if ip := net.ParseIP(legacy); ip == nil {
			return fmt.Errorf("%s.advertise_address must be a valid IP address", prefix)
		}
	}
	if ipv4 == "" && ipv6 == "" {
		return fmt.Errorf("%s advertise IPv4 or IPv6 address is required", prefix)
	}
	if ipv4 != "" {
		ip := net.ParseIP(ipv4)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("%s.advertise_address_ipv4 must be a valid IPv4 address", prefix)
		}
	}
	if ipv6 != "" {
		ip := net.ParseIP(ipv6)
		if ip == nil || ip.To4() != nil {
			return fmt.Errorf("%s.advertise_address_ipv6 must be a valid IPv6 address", prefix)
		}
	}
	return nil
}
