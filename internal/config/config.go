package config

import (
	"fmt"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Proxy            ProxyConfig             `yaml:"proxy" json:"proxy"`
	API              APIConfig               `yaml:"api" json:"api"`
	Log              LogConfig               `yaml:"log" json:"log"`
	Database         DatabaseConfig          `yaml:"database" json:"database"`
	TransportDomains []TransportDomainConfig `yaml:"transport_domains,omitempty" json:"transport_domains"`
	DNSResolvers     []DNSResolverConfig     `yaml:"dns_resolvers,omitempty" json:"dns_resolvers"`
	Peers            []PeerConfig            `yaml:"peers,omitempty" json:"peers"`
	Routing          RoutingConfig           `yaml:"routing,omitempty" json:"routing"`
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
	File  string `yaml:"file,omitempty" json:"file,omitempty"`
}

type DatabaseConfig struct {
	Path string `yaml:"path" json:"path"`
}

type TransportDomainConfig struct {
	Name              string `yaml:"name" json:"name"`
	Description       string `yaml:"description,omitempty" json:"description,omitempty"`
	NetNSPath         string `yaml:"netns_path" json:"netns_path"`
	Enabled           bool   `yaml:"enabled" json:"enabled"`
	GTPCListenHost    string `yaml:"gtpc_listen_host" json:"gtpc_listen_host"`
	GTPCPort          int    `yaml:"gtpc_port" json:"gtpc_port"`
	GTPUListenHost    string `yaml:"gtpu_listen_host" json:"gtpu_listen_host"`
	GTPUPort          int    `yaml:"gtpu_port" json:"gtpu_port"`
	GTPCAdvertiseIPv4 string `yaml:"gtpc_advertise_ipv4,omitempty" json:"gtpc_advertise_ipv4,omitempty"`
	GTPCAdvertiseIPv6 string `yaml:"gtpc_advertise_ipv6,omitempty" json:"gtpc_advertise_ipv6,omitempty"`
	GTPUAdvertiseIPv4 string `yaml:"gtpu_advertise_ipv4,omitempty" json:"gtpu_advertise_ipv4,omitempty"`
	GTPUAdvertiseIPv6 string `yaml:"gtpu_advertise_ipv6,omitempty" json:"gtpu_advertise_ipv6,omitempty"`
}

type DNSResolverConfig struct {
	Name            string `yaml:"name" json:"name"`
	TransportDomain string `yaml:"transport_domain" json:"transport_domain"`
	Server          string `yaml:"server" json:"server"`
	Priority        int    `yaml:"priority" json:"priority"`
	TimeoutMS       int    `yaml:"timeout_ms" json:"timeout_ms"`
	Attempts        int    `yaml:"attempts" json:"attempts"`
	SearchDomain    string `yaml:"search_domain,omitempty" json:"search_domain,omitempty"`
	Enabled         bool   `yaml:"enabled" json:"enabled"`
}

type PeerConfig struct {
	Name            string `yaml:"name" json:"name"`
	Address         string `yaml:"address" json:"address"`
	TransportDomain string `yaml:"transport_domain,omitempty" json:"transport_domain,omitempty"`
	Enabled         bool   `yaml:"enabled" json:"enabled"`
	Description     string `yaml:"description,omitempty" json:"description,omitempty"`
}

type RoutingConfig struct {
	DefaultPeer      string            `yaml:"default_peer" json:"default_peer"`
	IMSIRoutes       []IMSIRoute       `yaml:"imsi_routes,omitempty" json:"imsi_routes,omitempty"`
	IMSIPrefixRoutes []IMSIPrefixRoute `yaml:"imsi_prefix_routes,omitempty" json:"imsi_prefix_routes,omitempty"`
	APNRoutes        []APNRoute        `yaml:"apn_routes" json:"apn_routes"`
	PLMNRoutes       []PLMNRoute       `yaml:"plmn_routes,omitempty" json:"plmn_routes,omitempty"`
}

type APNRoute struct {
	APN             string `yaml:"apn" json:"apn"`
	Peer            string `yaml:"peer,omitempty" json:"peer,omitempty"`
	ActionType      string `yaml:"action_type,omitempty" json:"action_type,omitempty"`
	TransportDomain string `yaml:"transport_domain,omitempty" json:"transport_domain,omitempty"`
	FQDN            string `yaml:"fqdn,omitempty" json:"fqdn,omitempty"`
	Service         string `yaml:"service,omitempty" json:"service,omitempty"`
}

type IMSIRoute struct {
	IMSI            string `yaml:"imsi" json:"imsi"`
	Peer            string `yaml:"peer,omitempty" json:"peer,omitempty"`
	ActionType      string `yaml:"action_type,omitempty" json:"action_type,omitempty"`
	TransportDomain string `yaml:"transport_domain,omitempty" json:"transport_domain,omitempty"`
	FQDN            string `yaml:"fqdn,omitempty" json:"fqdn,omitempty"`
	Service         string `yaml:"service,omitempty" json:"service,omitempty"`
}

type IMSIPrefixRoute struct {
	Prefix          string `yaml:"prefix" json:"prefix"`
	Peer            string `yaml:"peer,omitempty" json:"peer,omitempty"`
	ActionType      string `yaml:"action_type,omitempty" json:"action_type,omitempty"`
	TransportDomain string `yaml:"transport_domain,omitempty" json:"transport_domain,omitempty"`
	FQDN            string `yaml:"fqdn,omitempty" json:"fqdn,omitempty"`
	Service         string `yaml:"service,omitempty" json:"service,omitempty"`
}

type PLMNRoute struct {
	PLMN            string `yaml:"plmn" json:"plmn"`
	Peer            string `yaml:"peer,omitempty" json:"peer,omitempty"`
	ActionType      string `yaml:"action_type,omitempty" json:"action_type,omitempty"`
	TransportDomain string `yaml:"transport_domain,omitempty" json:"transport_domain,omitempty"`
	FQDN            string `yaml:"fqdn,omitempty" json:"fqdn,omitempty"`
	Service         string `yaml:"service,omitempty" json:"service,omitempty"`
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
	if c.Proxy.GTPC.Listen == "" && hasLegacyGTPCConfig(c.Proxy.GTPC) {
		c.Proxy.GTPC.Listen = "0.0.0.0:2123"
	}
	applyAdvertiseDefaults(&c.Proxy.GTPC.AdvertiseAddress, &c.Proxy.GTPC.AdvertiseAddressIPv4, &c.Proxy.GTPC.AdvertiseAddressIPv6)
	if c.Proxy.GTPU.Listen == "" && hasLegacyGTPUConfig(c.Proxy.GTPU, c.Proxy.GTPC) {
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
	for i := range c.TransportDomains {
		if c.TransportDomains[i].Description == "" && c.TransportDomains[i].Name != "" {
			c.TransportDomains[i].Description = c.TransportDomains[i].Name
		}
		if c.TransportDomains[i].GTPCPort == 0 {
			c.TransportDomains[i].GTPCPort = 2123
		}
		if c.TransportDomains[i].GTPUPort == 0 {
			c.TransportDomains[i].GTPUPort = 2152
		}
	}
	for i := range c.DNSResolvers {
		if c.DNSResolvers[i].Priority == 0 {
			c.DNSResolvers[i].Priority = 100
		}
		if c.DNSResolvers[i].TimeoutMS == 0 {
			c.DNSResolvers[i].TimeoutMS = 2000
		}
		if c.DNSResolvers[i].Attempts == 0 {
			c.DNSResolvers[i].Attempts = 2
		}
	}
	for i := range c.Peers {
		if c.Peers[i].Name == "" {
			continue
		}
		if c.Peers[i].Description == "" {
			c.Peers[i].Description = c.Peers[i].Name
		}
	}
	for i := range c.Routing.IMSIRoutes {
		c.Routing.IMSIRoutes[i].ActionType = normalizeRouteActionType(c.Routing.IMSIRoutes[i].ActionType)
	}
	for i := range c.Routing.IMSIPrefixRoutes {
		c.Routing.IMSIPrefixRoutes[i].ActionType = normalizeRouteActionType(c.Routing.IMSIPrefixRoutes[i].ActionType)
	}
	for i := range c.Routing.APNRoutes {
		c.Routing.APNRoutes[i].ActionType = normalizeRouteActionType(c.Routing.APNRoutes[i].ActionType)
	}
	for i := range c.Routing.PLMNRoutes {
		c.Routing.PLMNRoutes[i].ActionType = normalizeRouteActionType(c.Routing.PLMNRoutes[i].ActionType)
	}
}

func (c Config) ValidateBootstrap() error {
	if _, err := net.ResolveTCPAddr("tcp", c.API.Listen); err != nil {
		return fmt.Errorf("api.listen: %w", err)
	}
	if hasLegacyGTPCConfig(c.Proxy.GTPC) {
		if c.Proxy.GTPC.Listen == "" {
			return fmt.Errorf("proxy.gtpc.listen is required when legacy GTPC bootstrap config is used")
		}
		if _, err := net.ResolveUDPAddr("udp", c.Proxy.GTPC.Listen); err != nil {
			return fmt.Errorf("proxy.gtpc.listen: %w", err)
		}
		if err := validateAdvertiseConfig("proxy.gtpc", c.Proxy.GTPC.AdvertiseAddress, c.Proxy.GTPC.AdvertiseAddressIPv4, c.Proxy.GTPC.AdvertiseAddressIPv6); err != nil {
			return err
		}
	}
	if hasLegacyGTPUConfig(c.Proxy.GTPU, c.Proxy.GTPC) {
		if c.Proxy.GTPU.Listen == "" {
			return fmt.Errorf("proxy.gtpu.listen is required when legacy GTPU bootstrap config is used")
		}
		if _, err := net.ResolveUDPAddr("udp", c.Proxy.GTPU.Listen); err != nil {
			return fmt.Errorf("proxy.gtpu.listen: %w", err)
		}
		if err := validateAdvertiseConfig("proxy.gtpu", c.Proxy.GTPU.AdvertiseAddress, c.Proxy.GTPU.AdvertiseAddressIPv4, c.Proxy.GTPU.AdvertiseAddressIPv6); err != nil {
			return err
		}
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
	if strings.TrimSpace(c.Log.File) != "" {
		if stat, err := os.Stat(c.Log.File); err == nil && stat.IsDir() {
			return fmt.Errorf("log.file %q must be a file path, not a directory", c.Log.File)
		}
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
	return validateMutable(c.TransportDomains, c.DNSResolvers, c.Peers, c.Routing)
}

func validateMutable(domains []TransportDomainConfig, resolvers []DNSResolverConfig, peers []PeerConfig, routing RoutingConfig) error {
	knownDomains := map[string]struct{}{}
	enabledDomains := map[string]struct{}{}
	for _, domain := range domains {
		if domain.Name == "" {
			return fmt.Errorf("transport_domains[].name is required")
		}
		if _, ok := knownDomains[domain.Name]; ok {
			return fmt.Errorf("duplicate transport domain %q", domain.Name)
		}
		knownDomains[domain.Name] = struct{}{}
		if strings.TrimSpace(domain.NetNSPath) == "" {
			return fmt.Errorf("transport_domains[%q].netns_path is required", domain.Name)
		}
		if err := validateSocketAddress("transport_domains["+strconv.Quote(domain.Name)+"].gtpc", domain.GTPCListenHost, domain.GTPCPort); err != nil {
			return err
		}
		if err := validateSocketAddress("transport_domains["+strconv.Quote(domain.Name)+"].gtpu", domain.GTPUListenHost, domain.GTPUPort); err != nil {
			return err
		}
		if err := validateDomainAdvertisePair("transport_domains["+strconv.Quote(domain.Name)+"].gtpc", domain.GTPCAdvertiseIPv4, domain.GTPCAdvertiseIPv6); err != nil {
			return err
		}
		if err := validateDomainAdvertisePair("transport_domains["+strconv.Quote(domain.Name)+"].gtpu", domain.GTPUAdvertiseIPv4, domain.GTPUAdvertiseIPv6); err != nil {
			return err
		}
		if domain.Enabled {
			enabledDomains[domain.Name] = struct{}{}
		}
	}

	seenResolvers := map[string]struct{}{}
	for _, resolver := range resolvers {
		if resolver.Name == "" {
			return fmt.Errorf("dns_resolvers[].name is required")
		}
		if _, ok := seenResolvers[resolver.Name]; ok {
			return fmt.Errorf("duplicate DNS resolver %q", resolver.Name)
		}
		seenResolvers[resolver.Name] = struct{}{}
		if resolver.TransportDomain == "" {
			return fmt.Errorf("dns_resolvers[%q].transport_domain is required", resolver.Name)
		}
		if _, ok := knownDomains[resolver.TransportDomain]; !ok {
			return fmt.Errorf("dns_resolvers[%q].transport_domain %q must reference a configured transport domain", resolver.Name, resolver.TransportDomain)
		}
		if _, err := net.ResolveUDPAddr("udp", resolver.Server); err != nil {
			return fmt.Errorf("dns_resolvers[%q].server: %w", resolver.Name, err)
		}
		if resolver.Priority < 0 {
			return fmt.Errorf("dns_resolvers[%q].priority must be non-negative", resolver.Name)
		}
		if resolver.TimeoutMS <= 0 {
			return fmt.Errorf("dns_resolvers[%q].timeout_ms must be positive", resolver.Name)
		}
		if resolver.Attempts <= 0 {
			return fmt.Errorf("dns_resolvers[%q].attempts must be positive", resolver.Name)
		}
	}

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
		if peer.TransportDomain != "" {
			if _, ok := knownDomains[peer.TransportDomain]; !ok {
				return fmt.Errorf("peer %q transport_domain %q must reference a configured transport domain", peer.Name, peer.TransportDomain)
			}
			if _, ok := enabledDomains[peer.TransportDomain]; !ok {
				return fmt.Errorf("peer %q transport_domain %q must reference an enabled transport domain", peer.Name, peer.TransportDomain)
			}
		} else if len(knownDomains) > 0 {
			return fmt.Errorf("peer %q transport_domain is required when transport domains are configured", peer.Name)
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
		if err := validateRouteTarget("routing.imsi_routes["+strconv.Quote(route.IMSI)+"]", route, enabledPeers, enabledDomains); err != nil {
			return err
		}
		imsi := normalizeDigits(route.IMSI)
		if imsi == "" {
			return fmt.Errorf("routing.imsi_routes[%q].imsi must contain digits", route.IMSI)
		}
		if _, ok := seenIMSIs[imsi]; ok {
			return fmt.Errorf("duplicate IMSI route %q", route.IMSI)
		}
		seenIMSIs[imsi] = struct{}{}
	}

	seenPrefixes := map[string]struct{}{}
	for _, route := range routing.IMSIPrefixRoutes {
		if route.Prefix == "" {
			return fmt.Errorf("routing.imsi_prefix_routes[].prefix is required")
		}
		if err := validateRouteTarget("routing.imsi_prefix_routes["+strconv.Quote(route.Prefix)+"]", route, enabledPeers, enabledDomains); err != nil {
			return err
		}
		prefix := normalizeDigits(route.Prefix)
		if prefix == "" {
			return fmt.Errorf("routing.imsi_prefix_routes[%q].prefix must contain digits", route.Prefix)
		}
		if _, ok := seenPrefixes[prefix]; ok {
			return fmt.Errorf("duplicate IMSI prefix route %q", route.Prefix)
		}
		seenPrefixes[prefix] = struct{}{}
	}

	seenAPNs := map[string]struct{}{}
	for _, route := range routing.APNRoutes {
		if route.APN == "" {
			return fmt.Errorf("routing.apn_routes[].apn is required")
		}
		if err := validateRouteTarget("routing.apn_routes["+strconv.Quote(route.APN)+"]", route, enabledPeers, enabledDomains); err != nil {
			return err
		}
		apn := normalizeAPN(route.APN)
		if _, ok := seenAPNs[apn]; ok {
			return fmt.Errorf("duplicate APN route %q", route.APN)
		}
		seenAPNs[apn] = struct{}{}
	}

	seenPLMNs := map[string]struct{}{}
	for _, route := range routing.PLMNRoutes {
		if route.PLMN == "" {
			return fmt.Errorf("routing.plmn_routes[].plmn is required")
		}
		if err := validateRouteTarget("routing.plmn_routes["+strconv.Quote(route.PLMN)+"]", route, enabledPeers, enabledDomains); err != nil {
			return err
		}
		plmn := normalizeDigits(route.PLMN)
		if len(plmn) != 5 && len(plmn) != 6 {
			return fmt.Errorf("routing.plmn_routes[%q].plmn must be 5 or 6 digits", route.PLMN)
		}
		if _, ok := seenPLMNs[plmn]; ok {
			return fmt.Errorf("duplicate PLMN route %q", route.PLMN)
		}
		seenPLMNs[plmn] = struct{}{}
	}
	return nil
}

func (c Config) Clone() Config {
	out := c
	out.TransportDomains = slices.Clone(c.TransportDomains)
	out.DNSResolvers = slices.Clone(c.DNSResolvers)
	out.Peers = slices.Clone(c.Peers)
	out.Routing.IMSIRoutes = slices.Clone(c.Routing.IMSIRoutes)
	out.Routing.IMSIPrefixRoutes = slices.Clone(c.Routing.IMSIPrefixRoutes)
	out.Routing.APNRoutes = slices.Clone(c.Routing.APNRoutes)
	out.Routing.PLMNRoutes = slices.Clone(c.Routing.PLMNRoutes)
	return out
}

func (c Config) BootstrapOnly() Config {
	out := c.Clone()
	out.TransportDomains = nil
	out.DNSResolvers = nil
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

func validateSocketAddress(prefix, host string, port int) error {
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("%s host is required", prefix)
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("%s port must be between 1 and 65535", prefix)
	}
	if _, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, strconv.Itoa(port))); err != nil {
		return fmt.Errorf("%s address: %w", prefix, err)
	}
	return nil
}

func validateDomainAdvertisePair(prefix, ipv4, ipv6 string) error {
	if ipv4 == "" && ipv6 == "" {
		return fmt.Errorf("%s advertise IPv4 or IPv6 address is required", prefix)
	}
	if ipv4 != "" {
		ip := net.ParseIP(ipv4)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("%s_advertise_ipv4 must be a valid IPv4 address", prefix)
		}
	}
	if ipv6 != "" {
		ip := net.ParseIP(ipv6)
		if ip == nil || ip.To4() != nil {
			return fmt.Errorf("%s_advertise_ipv6 must be a valid IPv6 address", prefix)
		}
	}
	return nil
}

type routeTarget interface {
	GetPeer() string
	GetActionType() string
	GetTransportDomain() string
	GetFQDN() string
}

func validateRouteTarget(prefix string, route routeTarget, enabledPeers, enabledDomains map[string]struct{}) error {
	actionType := normalizeRouteActionType(route.GetActionType())
	switch actionType {
	case "", "static_peer":
		if route.GetPeer() == "" {
			return fmt.Errorf("%s.peer is required for static routes", prefix)
		}
		if _, ok := enabledPeers[route.GetPeer()]; !ok {
			return fmt.Errorf("%s.peer %q must reference an enabled peer", prefix, route.GetPeer())
		}
	case "dns_discovery":
		if route.GetTransportDomain() == "" {
			return fmt.Errorf("%s.transport_domain is required for dns_discovery routes", prefix)
		}
		if _, ok := enabledDomains[route.GetTransportDomain()]; !ok {
			return fmt.Errorf("%s.transport_domain %q must reference an enabled transport domain", prefix, route.GetTransportDomain())
		}
		if strings.TrimSpace(route.GetFQDN()) == "" {
			return fmt.Errorf("%s.fqdn is required for dns_discovery routes", prefix)
		}
	default:
		return fmt.Errorf("%s.action_type %q must be static_peer or dns_discovery", prefix, route.GetActionType())
	}
	return nil
}

func normalizeRouteActionType(actionType string) string {
	if strings.TrimSpace(actionType) == "" {
		return "static_peer"
	}
	return strings.ToLower(strings.TrimSpace(actionType))
}

func (r APNRoute) GetPeer() string        { return r.Peer }
func (r IMSIRoute) GetPeer() string       { return r.Peer }
func (r IMSIPrefixRoute) GetPeer() string { return r.Peer }
func (r PLMNRoute) GetPeer() string       { return r.Peer }
func (r APNRoute) GetActionType() string  { return r.ActionType }
func (r IMSIRoute) GetActionType() string { return r.ActionType }
func (r IMSIPrefixRoute) GetActionType() string {
	return r.ActionType
}
func (r PLMNRoute) GetActionType() string      { return r.ActionType }
func (r APNRoute) GetTransportDomain() string  { return r.TransportDomain }
func (r IMSIRoute) GetTransportDomain() string { return r.TransportDomain }
func (r IMSIPrefixRoute) GetTransportDomain() string {
	return r.TransportDomain
}
func (r PLMNRoute) GetTransportDomain() string { return r.TransportDomain }
func (r APNRoute) GetFQDN() string             { return r.FQDN }
func (r IMSIRoute) GetFQDN() string            { return r.FQDN }
func (r IMSIPrefixRoute) GetFQDN() string      { return r.FQDN }
func (r PLMNRoute) GetFQDN() string            { return r.FQDN }

func (c Config) PrimaryTransportDomain() (TransportDomainConfig, bool) {
	for _, domain := range c.TransportDomains {
		if domain.Enabled {
			return domain, true
		}
	}
	return TransportDomainConfig{}, false
}

func (c Config) TransportDomainByName(name string) (TransportDomainConfig, bool) {
	for _, domain := range c.TransportDomains {
		if domain.Name == name {
			return domain, true
		}
	}
	return TransportDomainConfig{}, false
}

func (c Config) EffectiveGTPCConfig() (GTPCConfig, bool) {
	if domain, ok := c.PrimaryTransportDomain(); ok {
		return GTPCConfig{
			Listen:               net.JoinHostPort(domain.GTPCListenHost, strconv.Itoa(domain.GTPCPort)),
			AdvertiseAddressIPv4: domain.GTPCAdvertiseIPv4,
			AdvertiseAddressIPv6: domain.GTPCAdvertiseIPv6,
		}, true
	}
	if hasLegacyGTPCConfig(c.Proxy.GTPC) {
		return c.Proxy.GTPC, true
	}
	return GTPCConfig{}, false
}

func (c Config) EffectiveGTPUConfig() (GTPUConfig, bool) {
	if domain, ok := c.PrimaryTransportDomain(); ok {
		return GTPUConfig{
			Listen:               net.JoinHostPort(domain.GTPUListenHost, strconv.Itoa(domain.GTPUPort)),
			AdvertiseAddressIPv4: domain.GTPUAdvertiseIPv4,
			AdvertiseAddressIPv6: domain.GTPUAdvertiseIPv6,
		}, true
	}
	if hasLegacyGTPUConfig(c.Proxy.GTPU, c.Proxy.GTPC) {
		return c.Proxy.GTPU, true
	}
	return GTPUConfig{}, false
}

func hasLegacyGTPCConfig(cfg GTPCConfig) bool {
	return strings.TrimSpace(cfg.Listen) != "" ||
		strings.TrimSpace(cfg.AdvertiseAddress) != "" ||
		strings.TrimSpace(cfg.AdvertiseAddressIPv4) != "" ||
		strings.TrimSpace(cfg.AdvertiseAddressIPv6) != ""
}

func hasLegacyGTPUConfig(cfg GTPUConfig, gtpc GTPCConfig) bool {
	return strings.TrimSpace(cfg.Listen) != "" ||
		strings.TrimSpace(cfg.AdvertiseAddress) != "" ||
		strings.TrimSpace(cfg.AdvertiseAddressIPv4) != "" ||
		strings.TrimSpace(cfg.AdvertiseAddressIPv6) != "" ||
		strings.TrimSpace(gtpc.AdvertiseAddress) != "" ||
		strings.TrimSpace(gtpc.AdvertiseAddressIPv4) != "" ||
		strings.TrimSpace(gtpc.AdvertiseAddressIPv6) != ""
}
