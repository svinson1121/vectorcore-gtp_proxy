package transport

import (
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vectorcore/gtp_proxy/internal/config"
	"github.com/vectorcore/gtp_proxy/internal/session"
)

type ListenerStatus struct {
	Protocol      string    `json:"protocol"`
	State         string    `json:"state"`
	Domain        string    `json:"domain,omitempty"`
	Listen        string    `json:"listen,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	LastChangeAt  time.Time `json:"last_change_at"`
	RebindCount   int       `json:"rebind_count"`
	LastBindError string    `json:"last_bind_error,omitempty"`
}

type DomainStatus struct {
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	NetNSPath         string   `json:"netns_path"`
	Enabled           bool     `json:"enabled"`
	Effective         bool     `json:"effective"`
	NamespacePresent  bool     `json:"namespace_present"`
	GTPCListen        string   `json:"gtpc_listen"`
	GTPUListen        string   `json:"gtpu_listen"`
	GTPCAdvertiseIPv4 string   `json:"gtpc_advertise_ipv4,omitempty"`
	GTPCAdvertiseIPv6 string   `json:"gtpc_advertise_ipv6,omitempty"`
	GTPUAdvertiseIPv4 string   `json:"gtpu_advertise_ipv4,omitempty"`
	GTPUAdvertiseIPv6 string   `json:"gtpu_advertise_ipv6,omitempty"`
	GTPCSocketState   string   `json:"gtpc_socket_state"`
	GTPUSocketState   string   `json:"gtpu_socket_state"`
	ActiveSessions    int      `json:"active_sessions"`
	LastSessionUpdate string   `json:"last_session_update,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
	ValidationErrors  []string `json:"validation_errors,omitempty"`
}

type Runtime struct {
	mu           sync.RWMutex
	gtpc         ListenerStatus
	gtpu         ListenerStatus
	gtpcByDomain map[string]ListenerStatus
	gtpuByDomain map[string]ListenerStatus
}

func NewRuntime() *Runtime {
	now := time.Now().UTC()
	return &Runtime{
		gtpc:         ListenerStatus{Protocol: "gtpc", State: "init", LastChangeAt: now},
		gtpu:         ListenerStatus{Protocol: "gtpu", State: "init", LastChangeAt: now},
		gtpcByDomain: map[string]ListenerStatus{},
		gtpuByDomain: map[string]ListenerStatus{},
	}
}

func (r *Runtime) SetGTPC(status ListenerStatus) {
	r.set("gtpc", status)
}

func (r *Runtime) SetGTPU(status ListenerStatus) {
	r.set("gtpu", status)
}

func (r *Runtime) Snapshot() (ListenerStatus, ListenerStatus) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.gtpc, r.gtpu
}

func (r *Runtime) SnapshotByProtocol(protocol string) []ListenerStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var source map[string]ListenerStatus
	if protocol == "gtpu" {
		source = r.gtpuByDomain
	} else {
		source = r.gtpcByDomain
	}

	out := make([]ListenerStatus, 0, len(source))
	for _, status := range source {
		out = append(out, status)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Domain < out[j].Domain
	})
	return out
}

func (r *Runtime) StatusForDomain(protocol, domain string) (ListenerStatus, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var (
		status ListenerStatus
		ok     bool
	)
	if protocol == "gtpu" {
		status, ok = r.gtpuByDomain[domain]
	} else {
		status, ok = r.gtpcByDomain[domain]
	}
	return status, ok
}

func (r *Runtime) set(protocol string, status ListenerStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current := &r.gtpc
	byDomain := r.gtpcByDomain
	if protocol == "gtpu" {
		current = &r.gtpu
		byDomain = r.gtpuByDomain
	}

	if status.Protocol == "" {
		status.Protocol = protocol
	}
	if status.LastChangeAt.IsZero() {
		status.LastChangeAt = time.Now().UTC()
	}
	if status.Domain != "" {
		currentDomain := byDomain[status.Domain]
		if currentDomain.Listen != "" && currentDomain.Listen != status.Listen {
			status.RebindCount = currentDomain.RebindCount + 1
		} else if currentDomain.State == "active" && status.State == "active" && currentDomain.Listen == status.Listen {
			status.RebindCount = currentDomain.RebindCount
		} else if status.RebindCount == 0 {
			status.RebindCount = currentDomain.RebindCount
		}
		byDomain[status.Domain] = status
	}
	if current.Listen != "" && current.Listen != status.Listen {
		status.RebindCount = current.RebindCount + 1
	} else if current.Domain != "" && current.Domain != status.Domain {
		status.RebindCount = current.RebindCount + 1
	} else if current.State == "active" && status.State == "active" && current.Listen == status.Listen && current.Domain == status.Domain {
		status.RebindCount = current.RebindCount
	} else if status.RebindCount == 0 {
		status.RebindCount = current.RebindCount
	}
	*current = status
}

func DomainDiagnostics(cfg config.Config, sessions []session.Session, runtime *Runtime) []DomainStatus {
	sessionCounts := map[string]int{}
	lastUpdate := map[string]time.Time{}
	for _, sess := range sessions {
		for _, domain := range []string{sess.IngressTransportDomain, sess.EgressTransportDomain} {
			if domain == "" {
				continue
			}
			sessionCounts[domain]++
			if sess.UpdatedAt.After(lastUpdate[domain]) {
				lastUpdate[domain] = sess.UpdatedAt
			}
		}
	}

	effectiveDomain := ""
	if primary, ok := cfg.PrimaryTransportDomain(); ok {
		effectiveDomain = primary.Name
	}

	out := make([]DomainStatus, 0, len(cfg.TransportDomains))
	for _, domain := range cfg.TransportDomains {
		gtpcStatus, _ := runtimeStatus(runtime, "gtpc", domain.Name)
		gtpuStatus, _ := runtimeStatus(runtime, "gtpu", domain.Name)
		status := DomainStatus{
			Name:              domain.Name,
			Description:       domain.Description,
			NetNSPath:         domain.NetNSPath,
			Enabled:           domain.Enabled,
			Effective:         domain.Name == effectiveDomain,
			GTPCListen:        net.JoinHostPort(domain.GTPCListenHost, strconv.Itoa(domain.GTPCPort)),
			GTPUListen:        net.JoinHostPort(domain.GTPUListenHost, strconv.Itoa(domain.GTPUPort)),
			GTPCAdvertiseIPv4: domain.GTPCAdvertiseIPv4,
			GTPCAdvertiseIPv6: domain.GTPCAdvertiseIPv6,
			GTPUAdvertiseIPv4: domain.GTPUAdvertiseIPv4,
			GTPUAdvertiseIPv6: domain.GTPUAdvertiseIPv6,
			GTPCSocketState:   listenerStateForDomain(gtpcStatus, domain.Name),
			GTPUSocketState:   listenerStateForDomain(gtpuStatus, domain.Name),
			ActiveSessions:    sessionCounts[domain.Name],
		}
		if !lastUpdate[domain.Name].IsZero() {
			status.LastSessionUpdate = lastUpdate[domain.Name].Format(time.RFC3339)
		}

		if _, err := os.Stat(domain.NetNSPath); err == nil {
			status.NamespacePresent = true
		} else {
			status.ValidationErrors = append(status.ValidationErrors, "netns path is missing or inaccessible")
		}
		if _, err := net.ResolveUDPAddr("udp", status.GTPCListen); err != nil {
			status.ValidationErrors = append(status.ValidationErrors, "gtpc listen address does not resolve")
		}
		if _, err := net.ResolveUDPAddr("udp", status.GTPUListen); err != nil {
			status.ValidationErrors = append(status.ValidationErrors, "gtpu listen address does not resolve")
		}
		if !domain.Enabled {
			status.Warnings = append(status.Warnings, "domain disabled")
		}
		if status.Effective && len(status.ValidationErrors) > 0 {
			status.Warnings = append(status.Warnings, "effective domain is not host-ready")
		}
		if gtpcStatus.LastBindError != "" {
			status.Warnings = append(status.Warnings, "gtpc bind error: "+gtpcStatus.LastBindError)
		}
		if gtpuStatus.LastBindError != "" {
			status.Warnings = append(status.Warnings, "gtpu bind error: "+gtpuStatus.LastBindError)
		}

		out = append(out, status)
	}
	return out
}

func listenerStateForDomain(listener ListenerStatus, domain string) string {
	if listener.Domain == "" || listener.Domain != domain {
		return "inactive"
	}
	if strings.TrimSpace(listener.State) == "" {
		return "unknown"
	}
	return listener.State
}

func runtimeStatus(runtime *Runtime, protocol, domain string) (ListenerStatus, bool) {
	if runtime == nil {
		return ListenerStatus{}, false
	}
	return runtime.StatusForDomain(protocol, domain)
}
