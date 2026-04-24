package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/vectorcore/gtp_proxy/internal/config"
	"github.com/vectorcore/gtp_proxy/internal/metrics"
	"github.com/vectorcore/gtp_proxy/internal/session"
	"github.com/vectorcore/gtp_proxy/internal/transport"
)

type Server struct {
	cfg       *config.Manager
	sessions  *session.Table
	metrics   *metrics.Registry
	transport *transport.Runtime
	version   string
	startAt   time.Time
	logger    *slog.Logger
}

type PeerStatus struct {
	Name        string `json:"name"`
	Address     string `json:"address"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
	RouteCount  int    `json:"route_count"`
}

func New(cfg *config.Manager, sessions *session.Table, metrics *metrics.Registry, transportRuntime *transport.Runtime, version string, logger *slog.Logger) *Server {
	return &Server{
		cfg:       cfg,
		sessions:  sessions,
		metrics:   metrics,
		transport: transportRuntime,
		version:   version,
		startAt:   time.Now().UTC(),
		logger:    logger,
	}
}

func (s *Server) Handler() http.Handler {
	mux := chi.NewRouter()
	mux.Use(middleware.Recoverer)
	mux.Use(middleware.RealIP)
	mux.Use(requestLogger(s.logger))

	humaConfig := huma.DefaultConfig("VectorCore GTP Proxy API", s.version)
	humaConfig.OpenAPIPath = "/api/v1/openapi.json"
	humaConfig.DocsPath = "/api/v1/docs"
	humaConfig.SchemasPath = "/api/v1/schemas"
	api := humachi.New(mux, humaConfig)

	registerStatus(api, s)
	registerConfig(api, s)
	registerTransport(api, s)
	registerPeers(api, s)
	registerRouting(api, s)
	registerSessions(api, s)
	registerDiagnostics(api, s)
	mux.Handle("/metrics", metrics.Handler(s.metrics, s.sessions))

	mux.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	mux.Handle("/ui", http.RedirectHandler("/ui/", http.StatusMovedPermanently))
	ui := uiHandler()
	mux.Handle("/ui/", ui)
	mux.Handle("/ui/*", ui)
	mux.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusFound)
	})
	return mux
}

func (s *Server) Start(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("api server started", "listen", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func registerStatus(api huma.API, s *Server) {
	type statusBody struct {
		Version              string  `json:"version"`
		Uptime               string  `json:"uptime"`
		UptimeSeconds        float64 `json:"uptime_seconds"`
		ConfigPath           string  `json:"config_path"`
		GTPCListen           string  `json:"gtpc_listen"`
		GTPUListen           string  `json:"gtpu_listen"`
		GTPCState            string  `json:"gtpc_state"`
		GTPUState            string  `json:"gtpu_state"`
		EffectiveDomain      string  `json:"effective_transport_domain"`
		APIListen            string  `json:"api_listen"`
		ActiveSessions       int     `json:"active_sessions"`
		PendingTransactions  int     `json:"pending_transactions"`
		LogLevel             string  `json:"log_level"`
		TransportDomainCount int     `json:"transport_domain_count"`
		DNSResolverCount     int     `json:"dns_resolver_count"`
		DefaultPeer          string  `json:"default_peer"`
		APNRouteCount        int     `json:"apn_route_count"`
		PeerCount            int     `json:"peer_count"`
		SessionCreates       uint64  `json:"session_creates"`
		SessionDeletes       uint64  `json:"session_deletes"`
		SessionTimeouts      uint64  `json:"session_timeouts"`
		GTPUForwardHits      uint64  `json:"gtpu_forward_hits"`
		GTPUForwardMisses    uint64  `json:"gtpu_forward_misses"`
		GTPUPacketsForwarded uint64  `json:"gtpu_packets_forwarded"`
		UnknownTEIDDrops     uint64  `json:"unknown_teid_drops"`
	}

	huma.Register(api, huma.Operation{
		OperationID: "get-status",
		Method:      http.MethodGet,
		Path:        "/api/v1/status",
		Summary:     "Get proxy status",
		Tags:        []string{"Status"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body statusBody }, error) {
		cfg := s.cfg.Snapshot()
		stats := s.sessions.Stats()
		metricSnapshot := s.metrics.Snapshot()
		up := time.Since(s.startAt)
		effectiveGTPC, _ := cfg.EffectiveGTPCConfig()
		effectiveGTPU, _ := cfg.EffectiveGTPUConfig()
		gtpcRuntime, gtpuRuntime := transport.ListenerStatus{}, transport.ListenerStatus{}
		if s.transport != nil {
			gtpcRuntime, gtpuRuntime = s.transport.Snapshot()
		}
		effectiveDomain := ""
		if domain, ok := cfg.PrimaryTransportDomain(); ok {
			effectiveDomain = domain.Name
		}
		body := statusBody{
			Version:              s.version,
			Uptime:               up.Round(time.Second).String(),
			UptimeSeconds:        up.Seconds(),
			ConfigPath:           s.cfg.Path(),
			GTPCListen:           effectiveGTPC.Listen,
			GTPUListen:           effectiveGTPU.Listen,
			GTPCState:            gtpcRuntime.State,
			GTPUState:            gtpuRuntime.State,
			EffectiveDomain:      effectiveDomain,
			APIListen:            cfg.API.Listen,
			ActiveSessions:       stats.ActiveSessions,
			PendingTransactions:  stats.PendingTransactions,
			LogLevel:             cfg.Log.Level,
			TransportDomainCount: len(cfg.TransportDomains),
			DNSResolverCount:     len(cfg.DNSResolvers),
			DefaultPeer:          cfg.Routing.DefaultPeer,
			APNRouteCount:        len(cfg.Routing.APNRoutes),
			PeerCount:            len(cfg.Peers),
			SessionCreates:       metricSnapshot.SessionCreates,
			SessionDeletes:       metricSnapshot.SessionDeletes,
			SessionTimeouts:      metricSnapshot.SessionTimeoutDeletes,
			GTPUForwardHits:      metricSnapshot.GTPUForwardHits,
			GTPUForwardMisses:    metricSnapshot.GTPUForwardMisses,
			GTPUPacketsForwarded: metricSnapshot.GTPUPacketsForwarded,
			UnknownTEIDDrops:     metricSnapshot.UnknownTEIDDrops,
		}
		return &struct{ Body statusBody }{Body: body}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-peer-status",
		Method:      http.MethodGet,
		Path:        "/api/v1/status/peers",
		Summary:     "Get configured peers and route usage",
		Tags:        []string{"Status"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body []PeerStatus }, error) {
		cfg := s.cfg.Snapshot()
		routeCounts := map[string]int{}
		for _, route := range cfg.Routing.IMSIRoutes {
			routeCounts[route.Peer]++
		}
		for _, route := range cfg.Routing.IMSIPrefixRoutes {
			routeCounts[route.Peer]++
		}
		for _, route := range cfg.Routing.APNRoutes {
			routeCounts[route.Peer]++
		}
		for _, route := range cfg.Routing.PLMNRoutes {
			routeCounts[route.Peer]++
		}
		out := make([]PeerStatus, 0, len(cfg.Peers))
		for _, peer := range cfg.Peers {
			out = append(out, PeerStatus{
				Name:        peer.Name,
				Address:     peer.Address,
				Enabled:     peer.Enabled,
				Description: peer.Description,
				RouteCount:  routeCounts[peer.Name],
			})
		}
		return &struct{ Body []PeerStatus }{Body: out}, nil
	})
}

func registerConfig(api huma.API, s *Server) {
	type mutableConfigBody struct {
		TransportDomains []config.TransportDomainConfig `json:"transport_domains"`
		DNSResolvers     []config.DNSResolverConfig     `json:"dns_resolvers"`
		Peers            []config.PeerConfig            `json:"peers"`
		Routing          config.RoutingConfig           `json:"routing"`
	}
	huma.Register(api, huma.Operation{
		OperationID: "get-config",
		Method:      http.MethodGet,
		Path:        "/api/v1/config",
		Summary:     "Get current mutable config",
		Tags:        []string{"Config"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body mutableConfigBody }, error) {
		cfg := s.cfg.Snapshot()
		return &struct{ Body mutableConfigBody }{Body: mutableConfigBody{
			TransportDomains: cfg.TransportDomains,
			DNSResolvers:     cfg.DNSResolvers,
			Peers:            cfg.Peers,
			Routing:          cfg.Routing,
		}}, nil
	})
}

func registerTransport(api huma.API, s *Server) {
	huma.Register(api, huma.Operation{
		OperationID: "list-transport-domains",
		Method:      http.MethodGet,
		Path:        "/api/v1/transport-domains",
		Summary:     "List transport domains",
		Tags:        []string{"Transport"},
	}, func(ctx context.Context, input *struct{}) (*struct {
		Body []config.TransportDomainConfig
	}, error) {
		return &struct {
			Body []config.TransportDomainConfig
		}{Body: s.cfg.Snapshot().TransportDomains}, nil
	})

	type upsertTransportDomainInput struct {
		Name string `path:"name"`
		Body config.TransportDomainConfig
	}
	huma.Register(api, huma.Operation{
		OperationID: "upsert-transport-domain",
		Method:      http.MethodPut,
		Path:        "/api/v1/transport-domains/{name}",
		Summary:     "Create or update a transport domain",
		Tags:        []string{"Transport"},
	}, func(ctx context.Context, input *upsertTransportDomainInput) (*struct{ Body config.Config }, error) {
		input.Body.Name = input.Name
		if existing := findTransportDomain(s.cfg.Snapshot(), input.Name); existing != nil {
			if transportMutationImpactsActiveSessions(*existing, input.Body) && s.sessions.CountByTransportDomain(input.Name) > 0 {
				return nil, huma.Error400BadRequest("transport domain change requires session drain", fmt.Errorf("transport domain %q has active sessions and cannot be disabled or rebinding-mutated", input.Name))
			}
		}
		cfg, err := s.cfg.UpsertTransportDomain(input.Body)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid transport domain", err)
		}
		return &struct{ Body config.Config }{Body: cfg}, nil
	})

	type deleteTransportDomainInput struct {
		Name string `path:"name"`
	}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-transport-domain",
		Method:        http.MethodDelete,
		Path:          "/api/v1/transport-domains/{name}",
		Summary:       "Delete a transport domain",
		Tags:          []string{"Transport"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *deleteTransportDomainInput) (*struct{}, error) {
		if s.sessions.CountByTransportDomain(input.Name) > 0 {
			return nil, huma.Error400BadRequest("delete transport domain failed", fmt.Errorf("transport domain %q has active sessions", input.Name))
		}
		if _, err := s.cfg.DeleteTransportDomain(input.Name); err != nil {
			return nil, huma.Error400BadRequest("delete transport domain failed", err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-dns-resolvers",
		Method:      http.MethodGet,
		Path:        "/api/v1/dns-resolvers",
		Summary:     "List DNS resolvers",
		Tags:        []string{"Transport"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body []config.DNSResolverConfig }, error) {
		return &struct{ Body []config.DNSResolverConfig }{Body: s.cfg.Snapshot().DNSResolvers}, nil
	})

	type upsertDNSResolverInput struct {
		Name string `path:"name"`
		Body config.DNSResolverConfig
	}
	huma.Register(api, huma.Operation{
		OperationID: "upsert-dns-resolver",
		Method:      http.MethodPut,
		Path:        "/api/v1/dns-resolvers/{name}",
		Summary:     "Create or update a DNS resolver",
		Tags:        []string{"Transport"},
	}, func(ctx context.Context, input *upsertDNSResolverInput) (*struct{ Body config.Config }, error) {
		input.Body.Name = input.Name
		cfg, err := s.cfg.UpsertDNSResolver(input.Body)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid DNS resolver", err)
		}
		return &struct{ Body config.Config }{Body: cfg}, nil
	})

	type deleteDNSResolverInput struct {
		Name string `path:"name"`
	}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-dns-resolver",
		Method:        http.MethodDelete,
		Path:          "/api/v1/dns-resolvers/{name}",
		Summary:       "Delete a DNS resolver",
		Tags:          []string{"Transport"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *deleteDNSResolverInput) (*struct{}, error) {
		if _, err := s.cfg.DeleteDNSResolver(input.Name); err != nil {
			return nil, huma.Error400BadRequest("delete DNS resolver failed", err)
		}
		return &struct{}{}, nil
	})
}

func registerPeers(api huma.API, s *Server) {
	huma.Register(api, huma.Operation{
		OperationID: "list-peers",
		Method:      http.MethodGet,
		Path:        "/api/v1/peers",
		Summary:     "List home-side peers",
		Tags:        []string{"Peers"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body []config.PeerConfig }, error) {
		return &struct{ Body []config.PeerConfig }{Body: s.cfg.Snapshot().Peers}, nil
	})

	type upsertPeerInput struct {
		Name string `path:"name"`
		Body config.PeerConfig
	}
	huma.Register(api, huma.Operation{
		OperationID: "upsert-peer",
		Method:      http.MethodPut,
		Path:        "/api/v1/peers/{name}",
		Summary:     "Create or update a peer",
		Tags:        []string{"Peers"},
	}, func(ctx context.Context, input *upsertPeerInput) (*struct{ Body config.Config }, error) {
		input.Body.Name = input.Name
		if existing := findPeerByName(s.cfg.Snapshot(), input.Name); existing != nil && existing.Enabled && !input.Body.Enabled && s.sessions.CountByRoutePeer(input.Name) > 0 {
			return nil, huma.Error400BadRequest("invalid peer", fmt.Errorf("peer %q has active sessions and cannot be disabled", input.Name))
		}
		cfg, err := s.cfg.UpsertPeer(input.Body)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid peer", err)
		}
		return &struct{ Body config.Config }{Body: cfg}, nil
	})

	type deletePeerInput struct {
		Name string `path:"name"`
	}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-peer",
		Method:        http.MethodDelete,
		Path:          "/api/v1/peers/{name}",
		Summary:       "Delete a peer",
		Tags:          []string{"Peers"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *deletePeerInput) (*struct{}, error) {
		if s.sessions.CountByRoutePeer(input.Name) > 0 {
			return nil, huma.Error400BadRequest("delete peer failed", fmt.Errorf("peer %q has active sessions", input.Name))
		}
		if _, err := s.cfg.DeletePeer(input.Name); err != nil {
			return nil, huma.Error400BadRequest("delete peer failed", err)
		}
		return &struct{}{}, nil
	})
}

func registerRouting(api huma.API, s *Server) {
	type routingBody struct {
		DefaultPeer      string                   `json:"default_peer"`
		IMSIRoutes       []config.IMSIRoute       `json:"imsi_routes"`
		IMSIPrefixRoutes []config.IMSIPrefixRoute `json:"imsi_prefix_routes"`
		APNRoutes        []config.APNRoute        `json:"apn_routes"`
		PLMNRoutes       []config.PLMNRoute       `json:"plmn_routes"`
	}
	huma.Register(api, huma.Operation{
		OperationID: "get-routing",
		Method:      http.MethodGet,
		Path:        "/api/v1/routing",
		Summary:     "Get routing configuration",
		Tags:        []string{"Routing"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body routingBody }, error) {
		cfg := s.cfg.Snapshot()
		return &struct{ Body routingBody }{Body: routingBody{
			DefaultPeer:      cfg.Routing.DefaultPeer,
			IMSIRoutes:       cfg.Routing.IMSIRoutes,
			IMSIPrefixRoutes: cfg.Routing.IMSIPrefixRoutes,
			APNRoutes:        cfg.Routing.APNRoutes,
			PLMNRoutes:       cfg.Routing.PLMNRoutes,
		}}, nil
	})

	type setDefaultPeerInput struct {
		Body struct {
			DefaultPeer string `json:"default_peer"`
		}
	}
	type routeBody struct {
		Peer            string `json:"peer,omitempty"`
		ActionType      string `json:"action_type,omitempty"`
		TransportDomain string `json:"transport_domain,omitempty"`
		FQDN            string `json:"fqdn,omitempty"`
		Service         string `json:"service,omitempty"`
	}
	huma.Register(api, huma.Operation{
		OperationID: "set-default-peer",
		Method:      http.MethodPut,
		Path:        "/api/v1/routing/default-peer",
		Summary:     "Set default routing peer",
		Tags:        []string{"Routing"},
	}, func(ctx context.Context, input *setDefaultPeerInput) (*struct{ Body config.Config }, error) {
		cfg, err := s.cfg.SetDefaultPeer(input.Body.DefaultPeer)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid default peer: "+err.Error(), err)
		}
		return &struct{ Body config.Config }{Body: cfg}, nil
	})

	type upsertRouteInput struct {
		APN  string `path:"apn"`
		Body routeBody
	}
	huma.Register(api, huma.Operation{
		OperationID: "upsert-apn-route",
		Method:      http.MethodPut,
		Path:        "/api/v1/routing/apn-routes/{apn}",
		Summary:     "Create or update an APN route",
		Tags:        []string{"Routing"},
	}, func(ctx context.Context, input *upsertRouteInput) (*struct{ Body config.Config }, error) {
		cfg, err := s.cfg.UpsertAPNRoute(config.APNRoute{
			APN:             input.APN,
			Peer:            input.Body.Peer,
			ActionType:      input.Body.ActionType,
			TransportDomain: input.Body.TransportDomain,
			FQDN:            input.Body.FQDN,
			Service:         input.Body.Service,
		})
		if err != nil {
			return nil, huma.Error400BadRequest("invalid APN route: "+err.Error(), err)
		}
		return &struct{ Body config.Config }{Body: cfg}, nil
	})

	type deleteRouteInput struct {
		APN string `path:"apn"`
	}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-apn-route",
		Method:        http.MethodDelete,
		Path:          "/api/v1/routing/apn-routes/{apn}",
		Summary:       "Delete an APN route",
		Tags:          []string{"Routing"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *deleteRouteInput) (*struct{}, error) {
		if _, err := s.cfg.DeleteAPNRoute(input.APN); err != nil {
			return nil, huma.Error400BadRequest("delete APN route failed: "+err.Error(), err)
		}
		return &struct{}{}, nil
	})

	type upsertIMSIRouteInput struct {
		IMSI string `path:"imsi"`
		Body routeBody
	}
	huma.Register(api, huma.Operation{
		OperationID: "upsert-imsi-route",
		Method:      http.MethodPut,
		Path:        "/api/v1/routing/imsi-routes/{imsi}",
		Summary:     "Create or update an IMSI exact-match route",
		Tags:        []string{"Routing"},
	}, func(ctx context.Context, input *upsertIMSIRouteInput) (*struct{ Body config.Config }, error) {
		cfg, err := s.cfg.UpsertIMSIRoute(config.IMSIRoute{
			IMSI:            input.IMSI,
			Peer:            input.Body.Peer,
			ActionType:      input.Body.ActionType,
			TransportDomain: input.Body.TransportDomain,
			FQDN:            input.Body.FQDN,
			Service:         input.Body.Service,
		})
		if err != nil {
			return nil, huma.Error400BadRequest("invalid IMSI route: "+err.Error(), err)
		}
		return &struct{ Body config.Config }{Body: cfg}, nil
	})

	type deleteIMSIRouteInput struct {
		IMSI string `path:"imsi"`
	}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-imsi-route",
		Method:        http.MethodDelete,
		Path:          "/api/v1/routing/imsi-routes/{imsi}",
		Summary:       "Delete an IMSI exact-match route",
		Tags:          []string{"Routing"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *deleteIMSIRouteInput) (*struct{}, error) {
		if _, err := s.cfg.DeleteIMSIRoute(input.IMSI); err != nil {
			return nil, huma.Error400BadRequest("delete IMSI route failed: "+err.Error(), err)
		}
		return &struct{}{}, nil
	})

	type upsertIMSIPrefixRouteInput struct {
		Prefix string `path:"prefix"`
		Body   routeBody
	}
	huma.Register(api, huma.Operation{
		OperationID: "upsert-imsi-prefix-route",
		Method:      http.MethodPut,
		Path:        "/api/v1/routing/imsi-prefix-routes/{prefix}",
		Summary:     "Create or update an IMSI prefix route",
		Tags:        []string{"Routing"},
	}, func(ctx context.Context, input *upsertIMSIPrefixRouteInput) (*struct{ Body config.Config }, error) {
		cfg, err := s.cfg.UpsertIMSIPrefixRoute(config.IMSIPrefixRoute{
			Prefix:          input.Prefix,
			Peer:            input.Body.Peer,
			ActionType:      input.Body.ActionType,
			TransportDomain: input.Body.TransportDomain,
			FQDN:            input.Body.FQDN,
			Service:         input.Body.Service,
		})
		if err != nil {
			return nil, huma.Error400BadRequest("invalid IMSI prefix route: "+err.Error(), err)
		}
		return &struct{ Body config.Config }{Body: cfg}, nil
	})

	type deleteIMSIPrefixRouteInput struct {
		Prefix string `path:"prefix"`
	}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-imsi-prefix-route",
		Method:        http.MethodDelete,
		Path:          "/api/v1/routing/imsi-prefix-routes/{prefix}",
		Summary:       "Delete an IMSI prefix route",
		Tags:          []string{"Routing"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *deleteIMSIPrefixRouteInput) (*struct{}, error) {
		if _, err := s.cfg.DeleteIMSIPrefixRoute(input.Prefix); err != nil {
			return nil, huma.Error400BadRequest("delete IMSI prefix route failed: "+err.Error(), err)
		}
		return &struct{}{}, nil
	})

	type upsertPLMNRouteInput struct {
		PLMN string `path:"plmn"`
		Body routeBody
	}
	huma.Register(api, huma.Operation{
		OperationID: "upsert-plmn-route",
		Method:      http.MethodPut,
		Path:        "/api/v1/routing/plmn-routes/{plmn}",
		Summary:     "Create or update a PLMN route",
		Tags:        []string{"Routing"},
	}, func(ctx context.Context, input *upsertPLMNRouteInput) (*struct{ Body config.Config }, error) {
		cfg, err := s.cfg.UpsertPLMNRoute(config.PLMNRoute{
			PLMN:            input.PLMN,
			Peer:            input.Body.Peer,
			ActionType:      input.Body.ActionType,
			TransportDomain: input.Body.TransportDomain,
			FQDN:            input.Body.FQDN,
			Service:         input.Body.Service,
		})
		if err != nil {
			return nil, huma.Error400BadRequest("invalid PLMN route: "+err.Error(), err)
		}
		return &struct{ Body config.Config }{Body: cfg}, nil
	})

	type deletePLMNRouteInput struct {
		PLMN string `path:"plmn"`
	}
	huma.Register(api, huma.Operation{
		OperationID:   "delete-plmn-route",
		Method:        http.MethodDelete,
		Path:          "/api/v1/routing/plmn-routes/{plmn}",
		Summary:       "Delete a PLMN route",
		Tags:          []string{"Routing"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *deletePLMNRouteInput) (*struct{}, error) {
		if _, err := s.cfg.DeletePLMNRoute(input.PLMN); err != nil {
			return nil, huma.Error400BadRequest("delete PLMN route failed: "+err.Error(), err)
		}
		return &struct{}{}, nil
	})
}

func registerSessions(api huma.API, s *Server) {
	huma.Register(api, huma.Operation{
		OperationID: "list-sessions",
		Method:      http.MethodGet,
		Path:        "/api/v1/sessions",
		Summary:     "List active control-plane sessions",
		Tags:        []string{"Sessions"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body []session.Session }, error) {
		return &struct{ Body []session.Session }{Body: s.sessions.List()}, nil
	})
}

func registerDiagnostics(api huma.API, s *Server) {
	type peerDiagnosticsBody struct {
		Name                string `json:"name"`
		Address             string `json:"address"`
		Enabled             bool   `json:"enabled"`
		Description         string `json:"description,omitempty"`
		RouteCount          int    `json:"route_count"`
		ActiveSessions      int    `json:"active_sessions"`
		Status              string `json:"status"`
		LastSessionUpdate   string `json:"last_session_update,omitempty"`
		ControlPlanePackets uint64 `json:"control_plane_packets"`
		UserPlanePackets    uint64 `json:"user_plane_packets"`
	}
	type routeDecisionBody struct {
		SessionID             string `json:"session_id"`
		IMSI                  string `json:"imsi,omitempty"`
		APN                   string `json:"apn,omitempty"`
		RouteMatchType        string `json:"route_match_type,omitempty"`
		RouteMatchValue       string `json:"route_match_value,omitempty"`
		RoutePeer             string `json:"route_peer,omitempty"`
		RouteActionType       string `json:"route_action_type,omitempty"`
		EgressTransportDomain string `json:"egress_transport_domain,omitempty"`
		DiscoveryFQDN         string `json:"discovery_fqdn,omitempty"`
		DiscoveryMethod       string `json:"discovery_method,omitempty"`
		HomeControlEndpoint   string `json:"home_control_endpoint,omitempty"`
		UpdatedAt             string `json:"updated_at"`
	}
	type transportDiagnosticsBody struct {
		Domains []transport.DomainStatus `json:"domains"`
		GTPC    transport.ListenerStatus `json:"gtpc"`
		GTPU    transport.ListenerStatus `json:"gtpu"`
	}
	type metricDetailsBody struct {
		PeerCounters  map[string]metrics.PeerCounters `json:"peer_counters"`
		MessageErrors map[string]uint64               `json:"message_errors"`
	}

	huma.Register(api, huma.Operation{
		OperationID: "get-peer-diagnostics",
		Method:      http.MethodGet,
		Path:        "/api/v1/diagnostics/peers",
		Summary:     "Get peer health, status, and counter diagnostics",
		Tags:        []string{"Diagnostics"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body []peerDiagnosticsBody }, error) {
		cfg := s.cfg.Snapshot()
		sessions := s.sessions.List()
		metricSnapshot := s.metrics.Snapshot()
		routeCounts := map[string]int{}
		activeSessions := map[string]int{}
		lastUpdate := map[string]time.Time{}
		for _, route := range cfg.Routing.IMSIRoutes {
			routeCounts[route.Peer]++
		}
		for _, route := range cfg.Routing.IMSIPrefixRoutes {
			routeCounts[route.Peer]++
		}
		for _, route := range cfg.Routing.APNRoutes {
			routeCounts[route.Peer]++
		}
		for _, route := range cfg.Routing.PLMNRoutes {
			routeCounts[route.Peer]++
		}
		for _, sess := range sessions {
			activeSessions[sess.RoutePeer]++
			if sess.UpdatedAt.After(lastUpdate[sess.RoutePeer]) {
				lastUpdate[sess.RoutePeer] = sess.UpdatedAt
			}
		}

		out := make([]peerDiagnosticsBody, 0, len(cfg.Peers))
		for _, peer := range cfg.Peers {
			status := "configured"
			if !peer.Enabled {
				status = "disabled"
			} else if activeSessions[peer.Name] > 0 {
				status = "active"
			}
			body := peerDiagnosticsBody{
				Name:                peer.Name,
				Address:             peer.Address,
				Enabled:             peer.Enabled,
				Description:         peer.Description,
				RouteCount:          routeCounts[peer.Name],
				ActiveSessions:      activeSessions[peer.Name],
				Status:              status,
				ControlPlanePackets: metricSnapshot.PeerCounters[peer.Name].ControlPlanePackets,
				UserPlanePackets:    metricSnapshot.PeerCounters[peer.Name].UserPlanePackets,
			}
			if !lastUpdate[peer.Name].IsZero() {
				body.LastSessionUpdate = lastUpdate[peer.Name].Format(time.RFC3339)
			}
			out = append(out, body)
		}
		return &struct{ Body []peerDiagnosticsBody }{Body: out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-route-decisions",
		Method:      http.MethodGet,
		Path:        "/api/v1/diagnostics/routes",
		Summary:     "Get active route decisions from current sessions",
		Tags:        []string{"Diagnostics"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body []routeDecisionBody }, error) {
		sessions := s.sessions.List()
		out := make([]routeDecisionBody, 0, len(sessions))
		for _, sess := range sessions {
			out = append(out, routeDecisionBody{
				SessionID:             sess.ID,
				IMSI:                  sess.IMSI,
				APN:                   sess.APN,
				RouteMatchType:        sess.RouteMatchType,
				RouteMatchValue:       sess.RouteMatchValue,
				RoutePeer:             sess.RoutePeer,
				RouteActionType:       sess.RouteActionType,
				EgressTransportDomain: sess.EgressTransportDomain,
				DiscoveryFQDN:         sess.DiscoveryFQDN,
				DiscoveryMethod:       sess.DiscoveryMethod,
				HomeControlEndpoint:   sess.HomeControlEndpoint,
				UpdatedAt:             sess.UpdatedAt.Format(time.RFC3339),
			})
		}
		return &struct{ Body []routeDecisionBody }{Body: out}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-transport-diagnostics",
		Method:      http.MethodGet,
		Path:        "/api/v1/diagnostics/transport",
		Summary:     "Get transport-domain runtime and host-readiness diagnostics",
		Tags:        []string{"Diagnostics"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body transportDiagnosticsBody }, error) {
		cfg := s.cfg.Snapshot()
		sessions := s.sessions.List()
		gtpcRuntime, gtpuRuntime := transport.ListenerStatus{}, transport.ListenerStatus{}
		if s.transport != nil {
			gtpcRuntime, gtpuRuntime = s.transport.Snapshot()
		}
		return &struct{ Body transportDiagnosticsBody }{Body: transportDiagnosticsBody{
			Domains: transport.DomainDiagnostics(cfg, sessions, s.transport),
			GTPC:    gtpcRuntime,
			GTPU:    gtpuRuntime,
		}}, nil
	})

	type auditInput struct {
		Limit int `query:"limit"`
	}
	huma.Register(api, huma.Operation{
		OperationID: "get-audit-history",
		Method:      http.MethodGet,
		Path:        "/api/v1/diagnostics/audit",
		Summary:     "Get mutable config audit history",
		Tags:        []string{"Diagnostics"},
	}, func(ctx context.Context, input *auditInput) (*struct{ Body []config.AuditEntry }, error) {
		entries, err := s.cfg.ListAudit(input.Limit)
		if err != nil {
			return nil, huma.Error500InternalServerError("load audit history failed", err)
		}
		return &struct{ Body []config.AuditEntry }{Body: entries}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-metric-details",
		Method:      http.MethodGet,
		Path:        "/api/v1/diagnostics/metrics",
		Summary:     "Get detailed operational metrics",
		Tags:        []string{"Diagnostics"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body metricDetailsBody }, error) {
		snapshot := s.metrics.Snapshot()
		return &struct{ Body metricDetailsBody }{Body: metricDetailsBody{
			PeerCounters:  snapshot.PeerCounters,
			MessageErrors: snapshot.MessageErrors,
		}}, nil
	})
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Debug("api request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

func findTransportDomain(cfg config.Config, name string) *config.TransportDomainConfig {
	for i := range cfg.TransportDomains {
		if cfg.TransportDomains[i].Name == name {
			domain := cfg.TransportDomains[i]
			return &domain
		}
	}
	return nil
}

func findPeerByName(cfg config.Config, name string) *config.PeerConfig {
	for i := range cfg.Peers {
		if cfg.Peers[i].Name == name {
			peer := cfg.Peers[i]
			return &peer
		}
	}
	return nil
}

func transportMutationImpactsActiveSessions(before, after config.TransportDomainConfig) bool {
	if before.Enabled && !after.Enabled {
		return true
	}
	return before.NetNSPath != after.NetNSPath ||
		before.GTPCListenHost != after.GTPCListenHost ||
		before.GTPCPort != after.GTPCPort ||
		before.GTPUListenHost != after.GTPUListenHost ||
		before.GTPUPort != after.GTPUPort ||
		before.GTPCAdvertiseIPv4 != after.GTPCAdvertiseIPv4 ||
		before.GTPCAdvertiseIPv6 != after.GTPCAdvertiseIPv6 ||
		before.GTPUAdvertiseIPv4 != after.GTPUAdvertiseIPv4 ||
		before.GTPUAdvertiseIPv6 != after.GTPUAdvertiseIPv6
}
