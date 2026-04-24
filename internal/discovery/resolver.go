package discovery

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/vectorcore/gtp_proxy/internal/config"
	"github.com/vectorcore/gtp_proxy/internal/routing"
	"github.com/vectorcore/gtp_proxy/internal/transport"
)

const (
	dnsTypeA     = 1
	dnsTypeAAAA  = 28
	dnsTypeSRV   = 33
	dnsTypeNAPTR = 35
	dnsClassIN   = 1
)

type Result struct {
	ControlEndpoint string
	Method          string
	FQDN            string
}

type resolverContext struct {
	cfg       config.DNSResolverConfig
	server    string
	netnsPath string
	resolver  *net.Resolver
}

type naptrRecord struct {
	Order       uint16
	Preference  uint16
	Flags       string
	Service     string
	Regexp      string
	Replacement string
}

func Resolve(ctx context.Context, cfg config.Config, match routing.Match) (Result, error) {
	switch strings.ToLower(strings.TrimSpace(match.ActionType)) {
	case "", "static_peer":
		if match.Peer.Address == "" {
			return Result{}, fmt.Errorf("static route selected without peer address")
		}
		return Result{
			ControlEndpoint: match.Peer.Address,
			Method:          "static_peer",
		}, nil
	case "dns_discovery":
		return resolveDNS(ctx, cfg, match)
	default:
		return Result{}, fmt.Errorf("unsupported route action %q", match.ActionType)
	}
}

func resolveDNS(ctx context.Context, cfg config.Config, match routing.Match) (Result, error) {
	if strings.TrimSpace(match.TransportDomain) == "" {
		return Result{}, fmt.Errorf("dns_discovery route missing transport domain")
	}
	if strings.TrimSpace(match.FQDN) == "" {
		return Result{}, fmt.Errorf("dns_discovery route missing fqdn")
	}

	resolverCtx, err := resolverForDomain(cfg, match.TransportDomain)
	if err != nil {
		return Result{}, err
	}

	var lastErr error
	for _, candidate := range candidateFQDNs(match.FQDN, resolverCtx.cfg.SearchDomain) {
		if result, err := resolveNAPTR(ctx, resolverCtx, candidate, match.Service); err == nil {
			return result, nil
		} else {
			lastErr = err
		}
	}

	for _, candidate := range candidateFQDNs(match.FQDN, resolverCtx.cfg.SearchDomain) {
		if strings.TrimSpace(match.Service) != "" {
			endpoint, target, err := resolveSRV(ctx, resolverCtx, match.Service, "udp", candidate)
			if err == nil {
				return Result{ControlEndpoint: endpoint, Method: "dns_srv", FQDN: target}, nil
			}
			lastErr = err
		}
		endpoint, err := lookupHostPort(ctx, resolverCtx, candidate, 2123)
		if err == nil {
			return Result{ControlEndpoint: endpoint, Method: "dns_a_aaaa", FQDN: candidate}, nil
		}
		lastErr = err
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no DNS result for %q", match.FQDN)
	}
	return Result{}, lastErr
}

func resolveNAPTR(ctx context.Context, resolverCtx resolverContext, name, wantedService string) (Result, error) {
	records, err := resolverCtx.lookupNAPTR(ctx, name)
	if err != nil {
		return Result{}, err
	}
	if len(records) == 0 {
		return Result{}, fmt.Errorf("no NAPTR records for %q", name)
	}

	filtered := make([]naptrRecord, 0, len(records))
	for _, record := range records {
		if serviceMatches(record.Service, wantedService) {
			filtered = append(filtered, record)
		}
	}
	if len(filtered) == 0 {
		return Result{}, fmt.Errorf("no matching NAPTR records for %q and service %q", name, wantedService)
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Order == filtered[j].Order {
			if filtered[i].Preference == filtered[j].Preference {
				return filtered[i].Replacement < filtered[j].Replacement
			}
			return filtered[i].Preference < filtered[j].Preference
		}
		return filtered[i].Order < filtered[j].Order
	})

	for _, record := range filtered {
		flags := strings.ToLower(strings.TrimSpace(record.Flags))
		replacement := strings.TrimSuffix(strings.TrimSpace(record.Replacement), ".")
		switch {
		case strings.Contains(flags, "s"):
			if replacement == "" || replacement == "." {
				continue
			}
			endpoint, target, err := resolveSRV(ctx, resolverCtx, "", "", replacement)
			if err == nil {
				return Result{ControlEndpoint: endpoint, Method: "dns_naptr_srv", FQDN: target}, nil
			}
		case strings.Contains(flags, "a"):
			target := replacement
			if target == "" || target == "." {
				target = name
			}
			endpoint, err := lookupHostPort(ctx, resolverCtx, target, 2123)
			if err == nil {
				return Result{ControlEndpoint: endpoint, Method: "dns_naptr_a_aaaa", FQDN: target}, nil
			}
		}
	}

	return Result{}, fmt.Errorf("no usable NAPTR targets for %q", name)
}

func resolveSRV(ctx context.Context, resolverCtx resolverContext, service, proto, name string) (string, string, error) {
	_, records, err := resolverCtx.lookupSRV(ctx, service, proto, name)
	if err != nil {
		return "", "", err
	}
	if len(records) == 0 {
		return "", "", fmt.Errorf("SRV lookup for %q returned no records", name)
	}
	target := strings.TrimSuffix(records[0].Target, ".")
	endpoint, err := lookupHostPort(ctx, resolverCtx, target, int(records[0].Port))
	if err != nil {
		return "", "", err
	}
	return endpoint, target, nil
}

func resolverForDomain(cfg config.Config, domainName string) (resolverContext, error) {
	type candidate struct {
		cfg config.DNSResolverConfig
	}

	var candidates []candidate
	for _, resolver := range cfg.DNSResolvers {
		if resolver.Enabled && resolver.TransportDomain == domainName {
			candidates = append(candidates, candidate{cfg: resolver})
		}
	}
	if len(candidates) == 0 {
		return resolverContext{}, fmt.Errorf("no enabled DNS resolver configured for transport domain %q", domainName)
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].cfg.Priority == candidates[j].cfg.Priority {
			return candidates[i].cfg.Server < candidates[j].cfg.Server
		}
		return candidates[i].cfg.Priority < candidates[j].cfg.Priority
	})

	chosen := candidates[0].cfg
	domain, ok := cfg.TransportDomainByName(domainName)
	if !ok {
		return resolverContext{}, fmt.Errorf("transport domain %q not found", domainName)
	}
	return resolverContext{
		cfg:       chosen,
		server:    chosen.Server,
		netnsPath: domain.NetNSPath,
		resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return transport.DialContextInNetNS(ctx, network, chosen.Server, domain.NetNSPath)
			},
		},
	}, nil
}

func (r resolverContext) lookupSRV(ctx context.Context, service, proto, name string) (string, []*net.SRV, error) {
	var (
		cname string
		srvs  []*net.SRV
		err   error
	)
	err = r.withAttempts(ctx, func(attemptCtx context.Context) error {
		cname, srvs, err = r.resolver.LookupSRV(attemptCtx, service, proto, name)
		return err
	})
	if err != nil {
		return "", nil, fmt.Errorf("lookup SRV %q: %w", name, err)
	}
	return cname, srvs, nil
}

func (r resolverContext) lookupIP(ctx context.Context, host string) ([]net.IPAddr, error) {
	var (
		ips []net.IPAddr
		err error
	)
	err = r.withAttempts(ctx, func(attemptCtx context.Context) error {
		ips, err = r.resolver.LookupIPAddr(attemptCtx, host)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("lookup host %q: %w", host, err)
	}
	return ips, nil
}

func (r resolverContext) lookupNAPTR(ctx context.Context, name string) ([]naptrRecord, error) {
	var (
		records []naptrRecord
		err     error
	)
	err = r.withAttempts(ctx, func(attemptCtx context.Context) error {
		records, err = queryNAPTR(attemptCtx, r.server, r.netnsPath, name)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("lookup NAPTR %q: %w", name, err)
	}
	return records, nil
}

func (r resolverContext) withAttempts(ctx context.Context, fn func(context.Context) error) error {
	attempts := r.cfg.Attempts
	if attempts <= 0 {
		attempts = 1
	}
	timeout := time.Duration(r.cfg.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		attemptCtx, cancel := context.WithTimeout(ctx, timeout)
		err := fn(attemptCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return lastErr
}

func lookupHostPort(ctx context.Context, resolverCtx resolverContext, host string, port int) (string, error) {
	ips, err := resolverCtx.lookupIP(ctx, host)
	if err != nil {
		return "", err
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("lookup host %q returned no addresses", host)
	}
	return net.JoinHostPort(ips[0].IP.String(), fmt.Sprintf("%d", port)), nil
}

func candidateFQDNs(name, searchDomain string) []string {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".")
	searchDomain = strings.Trim(strings.TrimSpace(searchDomain), ".")
	if name == "" {
		return nil
	}

	candidates := []string{name}
	if searchDomain != "" && !strings.Contains(name, ".") {
		candidates = append(candidates, name+"."+searchDomain)
	}
	return candidates
}

func serviceMatches(recordService, wantedService string) bool {
	recordService = strings.ToLower(strings.TrimSpace(recordService))
	wantedService = strings.ToLower(strings.TrimSpace(wantedService))
	if wantedService == "" {
		return true
	}
	if recordService == wantedService {
		return true
	}
	for _, token := range strings.Split(recordService, ":") {
		if token == wantedService {
			return true
		}
	}
	return strings.Contains(recordService, wantedService)
}

func queryNAPTR(ctx context.Context, server, netnsPath, name string) ([]naptrRecord, error) {
	query, id, err := buildDNSQuery(name, dnsTypeNAPTR)
	if err != nil {
		return nil, err
	}

	conn, err := transport.DialContextInNetNS(ctx, "udp", server, netnsPath)
	if err != nil {
		return nil, fmt.Errorf("dial DNS server %q: %w", server, err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if _, err := conn.Write(query); err != nil {
		return nil, fmt.Errorf("write DNS query: %w", err)
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("read DNS response: %w", err)
	}

	records, responseID, err := parseNAPTRResponse(buf[:n])
	if err != nil {
		return nil, err
	}
	if responseID != id {
		return nil, fmt.Errorf("DNS response ID mismatch")
	}
	return records, nil
}

func buildDNSQuery(name string, qtype uint16) ([]byte, uint16, error) {
	id := uint16(rand.Uint32())
	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[0:2], id)
	binary.BigEndian.PutUint16(header[2:4], 0x0100)
	binary.BigEndian.PutUint16(header[4:6], 1)

	qname, err := encodeDNSName(name)
	if err != nil {
		return nil, 0, err
	}
	query := append(header, qname...)
	question := make([]byte, 4)
	binary.BigEndian.PutUint16(question[0:2], qtype)
	binary.BigEndian.PutUint16(question[2:4], dnsClassIN)
	query = append(query, question...)
	return query, id, nil
}

func parseNAPTRResponse(msg []byte) ([]naptrRecord, uint16, error) {
	if len(msg) < 12 {
		return nil, 0, fmt.Errorf("DNS response too short")
	}
	id := binary.BigEndian.Uint16(msg[0:2])
	flags := binary.BigEndian.Uint16(msg[2:4])
	if flags&0x8000 == 0 {
		return nil, id, fmt.Errorf("DNS response is not a response")
	}
	if rcode := flags & 0x000f; rcode != 0 {
		return nil, id, fmt.Errorf("DNS response rcode %d", rcode)
	}

	qdCount := int(binary.BigEndian.Uint16(msg[4:6]))
	anCount := int(binary.BigEndian.Uint16(msg[6:8]))
	offset := 12

	for i := 0; i < qdCount; i++ {
		var err error
		_, offset, err = parseDNSName(msg, offset)
		if err != nil {
			return nil, id, err
		}
		if offset+4 > len(msg) {
			return nil, id, fmt.Errorf("DNS question truncated")
		}
		offset += 4
	}

	records := make([]naptrRecord, 0, anCount)
	for i := 0; i < anCount; i++ {
		var err error
		_, offset, err = parseDNSName(msg, offset)
		if err != nil {
			return nil, id, err
		}
		if offset+10 > len(msg) {
			return nil, id, fmt.Errorf("DNS answer truncated")
		}
		rrType := binary.BigEndian.Uint16(msg[offset : offset+2])
		rrClass := binary.BigEndian.Uint16(msg[offset+2 : offset+4])
		rdLength := int(binary.BigEndian.Uint16(msg[offset+8 : offset+10]))
		rdataOffset := offset + 10
		offset = rdataOffset + rdLength
		if offset > len(msg) {
			return nil, id, fmt.Errorf("DNS rdata truncated")
		}
		if rrType != dnsTypeNAPTR || rrClass != dnsClassIN {
			continue
		}
		record, err := parseNAPTRRData(msg, rdataOffset, rdLength)
		if err != nil {
			return nil, id, err
		}
		records = append(records, record)
	}

	return records, id, nil
}

func parseNAPTRRData(msg []byte, offset, length int) (naptrRecord, error) {
	limit := offset + length
	if limit > len(msg) || offset+4 > limit {
		return naptrRecord{}, fmt.Errorf("NAPTR rdata too short")
	}

	record := naptrRecord{
		Order:      binary.BigEndian.Uint16(msg[offset : offset+2]),
		Preference: binary.BigEndian.Uint16(msg[offset+2 : offset+4]),
	}
	cursor := offset + 4
	var err error
	record.Flags, cursor, err = parseDNSCharString(msg, cursor, limit)
	if err != nil {
		return naptrRecord{}, err
	}
	record.Service, cursor, err = parseDNSCharString(msg, cursor, limit)
	if err != nil {
		return naptrRecord{}, err
	}
	record.Regexp, cursor, err = parseDNSCharString(msg, cursor, limit)
	if err != nil {
		return naptrRecord{}, err
	}
	record.Replacement, _, err = parseDNSName(msg, cursor)
	if err != nil {
		return naptrRecord{}, err
	}
	return record, nil
}

func parseDNSCharString(msg []byte, offset, limit int) (string, int, error) {
	if offset >= limit {
		return "", offset, fmt.Errorf("DNS char-string truncated")
	}
	size := int(msg[offset])
	offset++
	if offset+size > limit {
		return "", offset, fmt.Errorf("DNS char-string payload truncated")
	}
	return string(msg[offset : offset+size]), offset + size, nil
}

func encodeDNSName(name string) ([]byte, error) {
	name = strings.Trim(strings.TrimSpace(name), ".")
	if name == "" {
		return []byte{0}, nil
	}
	labels := strings.Split(name, ".")
	out := make([]byte, 0, len(name)+2)
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return nil, fmt.Errorf("invalid DNS label %q", label)
		}
		out = append(out, byte(len(label)))
		out = append(out, label...)
	}
	out = append(out, 0)
	return out, nil
}

func parseDNSName(msg []byte, offset int) (string, int, error) {
	start := offset
	jumped := false
	labels := make([]string, 0, 4)

	for steps := 0; steps < 32; steps++ {
		if offset >= len(msg) {
			return "", 0, fmt.Errorf("DNS name truncated")
		}
		length := int(msg[offset])
		switch {
		case length == 0:
			offset++
			if jumped {
				return strings.Join(labels, "."), start, nil
			}
			return strings.Join(labels, "."), offset, nil
		case length&0xc0 == 0xc0:
			if offset+1 >= len(msg) {
				return "", 0, fmt.Errorf("DNS compression pointer truncated")
			}
			pointer := int(binary.BigEndian.Uint16(msg[offset:offset+2]) & 0x3fff)
			if pointer >= len(msg) {
				return "", 0, fmt.Errorf("DNS compression pointer out of range")
			}
			if !jumped {
				start = offset + 2
			}
			offset = pointer
			jumped = true
		default:
			offset++
			if offset+length > len(msg) {
				return "", 0, fmt.Errorf("DNS label truncated")
			}
			labels = append(labels, string(msg[offset:offset+length]))
			offset += length
			if !jumped {
				start = offset
			}
		}
	}

	return "", 0, fmt.Errorf("DNS name exceeded compression limit")
}
