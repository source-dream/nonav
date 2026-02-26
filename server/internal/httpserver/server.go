package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"nonav/server/internal/config"
	"nonav/server/internal/core"
	"nonav/server/internal/store"
)

type Server struct {
	cfg               config.Config
	store             *store.SQLiteStore
	mux               *http.ServeMux
	apiProxy          *httputil.ReverseProxy
	apiClient         *InternalAPIClient
	frontendDevProxy  *httputil.ReverseProxy
	serveFrontend     bool
	frpManager        *FRPProcessManager
	frpsManager       *FRPServerProcessManager
	siteLocks         sync.Map
	shareCtxMu        sync.Mutex
	shareCtxByID      map[string]shareProxyContext
	shareCtxIDByToken map[string]string
}

const apiTunnelShareID int64 = -1
const gatewayRevisionHeaderValue = "20260226-sharectx-v5"

type shareProxyContext struct {
	ID          string
	Share       core.Share
	HasPassword bool
	ExpiresAt   time.Time
}

func NewAPI(cfg config.Config, st *store.SQLiteStore) (*Server, error) {
	s := &Server{
		cfg:           cfg,
		store:         st,
		mux:           http.NewServeMux(),
		serveFrontend: true,
		frpManager:    NewFRPProcessManager(cfg),
	}

	s.routesAPI()
	return s, nil
}

func NewGateway(cfg config.Config) (*Server, error) {
	apiURL, err := url.Parse(cfg.APIBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid api base url: %w", err)
	}

	var frontendProxy *httputil.ReverseProxy
	if strings.TrimSpace(cfg.FrontendDevProxyURL) != "" {
		frontendURL, parseErr := url.Parse(cfg.FrontendDevProxyURL)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid frontend dev proxy url: %w", parseErr)
		}
		frontendProxy = httputil.NewSingleHostReverseProxy(frontendURL)
		frontendProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			respondError(w, http.StatusBadGateway, "frontend dev server unavailable, run make dev and ensure Vite is on :5173")
		}
	}

	s := &Server{
		cfg:               cfg,
		mux:               http.NewServeMux(),
		apiProxy:          httputil.NewSingleHostReverseProxy(apiURL),
		apiClient:         NewInternalAPIClient(cfg.APIBaseURL),
		frontendDevProxy:  frontendProxy,
		serveFrontend:     false,
		frpsManager:       NewFRPServerProcessManager(cfg),
		shareCtxByID:      make(map[string]shareProxyContext),
		shareCtxIDByToken: make(map[string]string),
	}

	s.routesGateway()
	return s, nil
}

func (s *Server) StartBackgroundServices() error {
	if s.frpsManager != nil {
		if err := s.frpsManager.Start(); err != nil {
			return err
		}
	}

	if s.frpManager != nil && s.cfg.ForceFRP {
		if s.cfg.FRPExposeAPI {
			localPort, err := portFromListenAddr(s.cfg.APIListenAddr)
			if err != nil {
				return err
			}

			if err := s.frpManager.StartProxy(FRPProxySpec{
				ShareID:    apiTunnelShareID,
				ProxyName:  "nonav-api-tunnel",
				LocalHost:  "127.0.0.1",
				LocalPort:  localPort,
				RemotePort: s.cfg.FRPAPIRemotePort,
			}); err != nil {
				return fmt.Errorf("start api frp tunnel failed: %w", err)
			}
		}

		if s.cfg.FRPRecoverOnStart {
			if err := s.RecoverFRPProxies(context.Background()); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *Server) StopBackgroundServices() error {
	if s.frpManager != nil {
		s.frpManager.StopAll()
	}

	if s.frpsManager != nil {
		return s.frpsManager.Stop()
	}

	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqID := s.newRequestID()
	r = r.WithContext(context.WithValue(r.Context(), requestIDContextKey{}, reqID))
	w.Header().Set("X-Nonav-Req-Id", reqID)
	w.Header().Set("X-Nonav-Gateway-Rev", gatewayRevisionHeaderValue)
	routingHost := requestRoutingHost(r)
	w.Header().Set("X-Nonav-Routing-Host", routingHost)
	s.logRoute(r, "request", map[string]any{
		"host":         r.Host,
		"routing_host": routingHost,
		"method":       r.Method,
		"path":         r.URL.Path,
		"query":        r.URL.RawQuery,
	})
	s.mux.ServeHTTP(w, r)
}

type requestIDContextKey struct{}

func (s *Server) newRequestID() string {
	value, err := generateRandomToken(6)
	if err != nil || strings.TrimSpace(value) == "" {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return value
}

func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return "-"
	}
	value, _ := ctx.Value(requestIDContextKey{}).(string)
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func (s *Server) logRoute(r *http.Request, event string, extra map[string]any) {
	if !s.cfg.LogRouteTrace {
		return
	}

	payload := map[string]any{
		"event": event,
		"req":   requestIDFromContext(r.Context()),
	}
	for key, value := range extra {
		payload[key] = value
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("route-log-marshal-failed req=%s event=%s", requestIDFromContext(r.Context()), event)
		return
	}

	log.Printf("gateway_route %s", string(body))
}

func (s *Server) routesAPI() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/api/sites", s.withCORS(s.handleSites))
	s.mux.HandleFunc("/api/sites/", s.withCORS(s.handleSiteActions))
	s.mux.HandleFunc("/api/shares", s.withCORS(s.handleShares))
	s.mux.HandleFunc("/api/shares/", s.withCORS(s.handleShareActions))
	s.mux.HandleFunc("/api/internal/shares/token/", s.handleInternalShareByToken)
	s.mux.HandleFunc("/api/internal/shares/subdomain/", s.handleInternalShareBySubdomain)
	s.mux.HandleFunc("/", s.handleFrontend)
}

func (s *Server) routesGateway() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/api/", s.handleAPIProxy)
	s.mux.HandleFunc("/api", s.handleAPIProxy)
	s.mux.HandleFunc("/s/", s.handleGateway)
	s.mux.HandleFunc("/x/", s.handleGatewayContext)
	s.mux.HandleFunc("/", s.handleFrontend)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSites(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sites, err := s.store.ListSites(r.Context())
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		respondJSON(w, http.StatusOK, map[string]any{"sites": sites})
	case http.MethodPost:
		var payload struct {
			Name      string `json:"name"`
			URL       string `json:"url"`
			GroupName string `json:"groupName"`
			Icon      string `json:"icon"`
		}

		if err := decodeJSON(r, &payload); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}

		if payload.Name == "" || payload.URL == "" {
			respondError(w, http.StatusBadRequest, "name and url are required")
			return
		}

		if _, err := url.ParseRequestURI(payload.URL); err != nil {
			respondError(w, http.StatusBadRequest, "url format is invalid")
			return
		}

		site, err := s.store.CreateSite(r.Context(), core.Site{
			Name:      strings.TrimSpace(payload.Name),
			URL:       strings.TrimSpace(payload.URL),
			GroupName: strings.TrimSpace(payload.GroupName),
			Icon:      strings.TrimSpace(payload.Icon),
		})
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		respondJSON(w, http.StatusCreated, site)
	default:
		respondMethodNotAllowed(w)
	}
}

func (s *Server) handleSiteActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/sites/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		respondError(w, http.StatusBadRequest, "site id missing")
		return
	}

	siteID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "site id is invalid")
		return
	}

	if len(parts) == 2 && parts[1] == "click" {
		if r.Method != http.MethodPost {
			respondMethodNotAllowed(w)
			return
		}

		if err := s.store.IncrementSiteClick(r.Context(), siteID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "site not found")
				return
			}
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	if r.Method == http.MethodPut {
		var payload struct {
			Name      string `json:"name"`
			URL       string `json:"url"`
			GroupName string `json:"groupName"`
			Icon      string `json:"icon"`
		}

		if err := decodeJSON(r, &payload); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}

		if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.URL) == "" {
			respondError(w, http.StatusBadRequest, "name and url are required")
			return
		}

		if _, err := url.ParseRequestURI(payload.URL); err != nil {
			respondError(w, http.StatusBadRequest, "url format is invalid")
			return
		}

		updated, err := s.store.UpdateSite(r.Context(), core.Site{
			ID:        siteID,
			Name:      strings.TrimSpace(payload.Name),
			URL:       strings.TrimSpace(payload.URL),
			GroupName: strings.TrimSpace(payload.GroupName),
			Icon:      strings.TrimSpace(payload.Icon),
		})
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "site not found")
				return
			}

			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		respondJSON(w, http.StatusOK, updated)
		return
	}

	if r.Method != http.MethodDelete {
		respondMethodNotAllowed(w)
		return
	}

	unlock := s.lockSite(siteID)
	defer unlock()

	if err := s.store.DeleteSharesBySite(r.Context(), siteID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.cfg.ForceFRP {
		_ = s.frpManager.StopProxy(siteID)
	}

	if err := s.store.DeleteSite(r.Context(), siteID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			respondError(w, http.StatusNotFound, "site not found")
			return
		}

		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleShares(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		shares, err := s.store.ListShares(r.Context())
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		items := make([]map[string]any, 0, len(shares))
		for _, share := range shares {
			shareURL := s.buildShareURL(share)
			items = append(items, map[string]any{
				"id":            share.ID,
				"siteId":        share.SiteID,
				"siteName":      share.SiteName,
				"shareMode":     share.ShareMode,
				"subdomainSlug": share.Subdomain,
				"frpPort":       share.FRPPort,
				"token":         share.Token,
				"status":        share.Status,
				"expiresAt":     share.ExpiresAt,
				"createdAt":     share.CreatedAt,
				"updatedAt":     share.UpdatedAt,
				"stoppedAt":     share.StoppedAt,
				"accessHits":    share.AccessHits,
				"shareUrl":      shareURL,
			})
		}

		respondJSON(w, http.StatusOK, map[string]any{"shares": items})
	case http.MethodPost:
		var payload struct {
			SiteID         int64  `json:"siteId"`
			ExpiresInHours int    `json:"expiresInHours"`
			Password       string `json:"password"`
			ShareMode      string `json:"shareMode"`
			SubdomainSlug  string `json:"subdomainSlug"`
		}

		if err := decodeJSON(r, &payload); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}

		site, err := s.store.GetSiteByID(r.Context(), payload.SiteID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "site not found")
				return
			}
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		unlock := s.lockSite(site.ID)
		defer unlock()

		expires := s.cfg.DefaultShareTTL
		if payload.ExpiresInHours > 0 {
			expires = time.Duration(payload.ExpiresInHours) * time.Hour
		}

		shareToken, err := generateRandomToken(18)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		shareMode := strings.TrimSpace(payload.ShareMode)
		if shareMode == "" {
			shareMode = "path_ctx"
		}
		if shareMode != "path_ctx" && shareMode != "subdomain" {
			respondError(w, http.StatusBadRequest, "shareMode must be path_ctx or subdomain")
			return
		}

		subdomainSlug := ""
		autoSubdomainSlug := false
		if shareMode == "subdomain" {
			if !s.cfg.ShareSubdomainOn || strings.TrimSpace(s.cfg.ShareSubdomainBase) == "" {
				respondError(w, http.StatusBadRequest, "subdomain sharing is not enabled")
				return
			}

			subdomainSlug = normalizeSubdomainSlug(payload.SubdomainSlug)
			if subdomainSlug == "" {
				randomSlug, slugErr := generateRandomSubdomainSlug(10)
				if slugErr != nil {
					respondError(w, http.StatusInternalServerError, "failed to generate subdomain slug")
					return
				}
				subdomainSlug = randomSlug
				autoSubdomainSlug = true
			}
		}

		password := strings.TrimSpace(payload.Password)
		if password != "" && len(password) < 6 {
			respondError(w, http.StatusBadRequest, "password must be at least 6 characters")
			return
		}

		passwordHash := ""
		if password != "" {
			hashed, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if hashErr != nil {
				respondError(w, http.StatusInternalServerError, "failed to hash password")
				return
			}
			passwordHash = string(hashed)
		}

		if err := s.store.DeleteSharesBySite(r.Context(), site.ID); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		if s.cfg.ForceFRP {
			_ = s.frpManager.StopProxy(site.ID)
		}

		targetURL := site.URL
		frpPort := 0
		if s.cfg.ForceFRP {
			var selectErr error
			frpPort, targetURL, selectErr = s.selectFRPUpstreamForShare(r.Context())
			if selectErr != nil {
				respondError(w, http.StatusBadGateway, selectErr.Error())
				return
			}

			if targetURL == "" {
				respondError(w, http.StatusInternalServerError, "frp upstream url is empty")
				return
			}
		}

		if s.cfg.ForceFRP {
			localHost, localPort, parseErr := parseSiteUpstream(site.URL)
			if parseErr != nil {
				respondError(w, http.StatusBadRequest, "site url is not valid for frp tcp proxy")
				return
			}

			proxyName := fmt.Sprintf("share-%d-%d", site.ID, frpPort)
			if err := s.frpManager.StartProxy(FRPProxySpec{
				ShareID:    site.ID,
				ProxyName:  proxyName,
				LocalHost:  localHost,
				LocalPort:  localPort,
				RemotePort: frpPort,
			}); err != nil {
				respondError(w, http.StatusBadGateway, "frp proxy start failed: "+err.Error())
				return
			}
		}

		var created core.Share
		for attempt := 0; attempt < 5; attempt++ {
			created, err = s.store.CreateShare(r.Context(), core.Share{
				SiteID:    site.ID,
				SiteName:  site.Name,
				TargetURL: targetURL,
				ShareMode: shareMode,
				Subdomain: subdomainSlug,
				FRPPort:   frpPort,
				Token:     shareToken,
				Status:    "active",
				ExpiresAt: time.Now().UTC().Add(expires),
			}, passwordHash)
			if err == nil {
				break
			}

			if !isSubdomainConflictError(err) {
				break
			}

			if !autoSubdomainSlug {
				break
			}

			newSlug, slugErr := generateRandomSubdomainSlug(10)
			if slugErr != nil {
				err = slugErr
				break
			}
			subdomainSlug = newSlug
		}
		if err != nil {
			if s.cfg.ForceFRP {
				_ = s.frpManager.StopProxy(site.ID)
			}
			if isSubdomainConflictError(err) {
				respondError(w, http.StatusBadRequest, "subdomain slug already in use")
				return
			}
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		respondJSON(w, http.StatusCreated, core.ShareWithPassword{
			Share:         created,
			ShareURL:      s.buildShareURL(created),
			PlainPassword: password,
		})
	default:
		respondMethodNotAllowed(w)
	}
}

func (s *Server) handleShareActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/shares/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		respondError(w, http.StatusBadRequest, "invalid share action")
		return
	}

	shareID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "share id is invalid")
		return
	}

	if parts[1] == "stop" {
		if r.Method != http.MethodPost {
			respondMethodNotAllowed(w)
			return
		}

		share, _, err := s.store.GetShareByID(r.Context(), shareID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "share not found")
				return
			}
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		unlock := s.lockSite(share.SiteID)
		defer unlock()

		share, _, err = s.store.GetShareByID(r.Context(), shareID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "share not found")
				return
			}
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		if err := s.store.StopShare(r.Context(), shareID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "share not found")
				return
			}
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		if s.cfg.ForceFRP {
			_ = s.frpManager.StopProxy(share.SiteID)
		}

		respondJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
		return
	}

	respondError(w, http.StatusNotFound, "unknown share action")
}

func (s *Server) handleInternalShareByToken(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/internal/shares/token/")
	path = strings.Trim(path, "/")
	if path == "" {
		respondError(w, http.StatusBadRequest, "share token missing")
		return
	}

	parts := strings.Split(path, "/")
	token := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	share, passwordHash, err := s.store.GetShareByToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			respondError(w, http.StatusNotFound, "share not found")
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if action == "" {
		if r.Method != http.MethodGet {
			respondMethodNotAllowed(w)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"share":       share,
			"hasPassword": strings.TrimSpace(passwordHash) != "",
		})
		return
	}

	if action == "auth" {
		s.handleInternalShareAuth(w, r, share, passwordHash)
		return
	}

	if action == "session" {
		s.handleInternalShareSessionValidate(w, r, share)
		return
	}

	if action == "access-log" {
		s.handleInternalShareAccessLog(w, r, share)
		return
	}

	respondError(w, http.StatusNotFound, "unknown internal share action")
}

func (s *Server) handleInternalShareBySubdomain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondMethodNotAllowed(w)
		return
	}

	slug := strings.TrimSpace(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/internal/shares/subdomain/"), "/"))
	if slug == "" {
		respondError(w, http.StatusBadRequest, "share subdomain missing")
		return
	}

	share, passwordHash, err := s.store.GetShareBySubdomain(r.Context(), slug)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			respondError(w, http.StatusNotFound, "share not found")
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"share":       share,
		"hasPassword": strings.TrimSpace(passwordHash) != "",
	})
}

func (s *Server) handleInternalShareAuth(w http.ResponseWriter, r *http.Request, share core.Share, passwordHash string) {
	if r.Method != http.MethodPost {
		respondMethodNotAllowed(w)
		return
	}

	if strings.TrimSpace(passwordHash) == "" {
		respondJSON(w, http.StatusOK, map[string]any{"sessionToken": "", "expiresAt": share.ExpiresAt})
		return
	}

	var payload struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(payload.Password)); err != nil {
		respondError(w, http.StatusUnauthorized, "password invalid")
		return
	}

	sessionToken, err := generateRandomToken(24)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	expiresAt := time.Now().UTC().Add(8 * time.Hour)
	if share.ExpiresAt.Before(expiresAt) {
		expiresAt = share.ExpiresAt
	}

	if err := s.store.CreateShareSession(r.Context(), share.ID, sessionToken, expiresAt); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save session")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"sessionToken": sessionToken, "expiresAt": expiresAt})
}

func (s *Server) handleInternalShareSessionValidate(w http.ResponseWriter, r *http.Request, share core.Share) {
	if r.Method != http.MethodPost {
		respondMethodNotAllowed(w)
		return
	}

	var payload struct {
		SessionToken string `json:"sessionToken"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	valid, err := s.store.ValidateShareSession(r.Context(), share.ID, strings.TrimSpace(payload.SessionToken))
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]bool{"valid": valid})
}

func (s *Server) handleInternalShareAccessLog(w http.ResponseWriter, r *http.Request, share core.Share) {
	if r.Method != http.MethodPost {
		respondMethodNotAllowed(w)
		return
	}

	var payload struct {
		Method     string `json:"method"`
		Path       string `json:"path"`
		RemoteIP   string `json:"remoteIp"`
		StatusCode int    `json:"statusCode"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.store.LogShareAccess(
		r.Context(),
		share.ID,
		strings.TrimSpace(payload.Method),
		strings.TrimSpace(payload.Path),
		strings.TrimSpace(payload.RemoteIP),
		payload.StatusCode,
	); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGateway(w http.ResponseWriter, r *http.Request) {
	s.logRoute(r, "share_token_entry", map[string]any{"host": r.Host, "path": r.URL.Path})
	w.Header().Set("X-Nonav-Route", "share_token")

	shareToken, restPath, ok := parseSharePath(r.URL.Path)
	if !ok {
		w.Header().Set("X-Nonav-Reason", "share_path_not_found")
		s.logRoute(r, "share_token_path_invalid", map[string]any{"path": r.URL.Path})
		respondError(w, http.StatusNotFound, "share path not found")
		return
	}

	record, err := s.apiClient.GetShareByToken(r.Context(), shareToken)
	if err != nil {
		if errors.Is(err, errNotFound) {
			w.Header().Set("X-Nonav-Reason", "share_not_found")
			s.logRoute(r, "share_token_not_found", map[string]any{"token": shareToken})
			respondError(w, http.StatusNotFound, "share not found")
			return
		}
		w.Header().Set("X-Nonav-Reason", "internal_api_unavailable")
		s.logRoute(r, "share_token_internal_api_failed", map[string]any{"token": shareToken, "error": err.Error()})
		respondError(w, http.StatusBadGateway, "internal api unavailable")
		return
	}
	share := record.Share

	if share.Status != "active" || time.Now().UTC().After(share.ExpiresAt) {
		w.Header().Set("X-Nonav-Reason", "share_inactive")
		s.logRoute(r, "share_token_inactive", map[string]any{"token": shareToken, "status": share.Status})
		respondError(w, http.StatusGone, "share is no longer active")
		return
	}

	if !record.HasPassword {
		if s.tryRedirectToSubdomainShare(w, r, share, restPath) {
			return
		}
		s.redirectToShareContext(w, r, share, record.HasPassword, restPath)
		return
	}

	if restPath == "/auth" && r.Method == http.MethodPost {
		s.handleShareAuthByAPI(w, r, share)
		return
	}

	sessionToken, ok := getShareSessionTokenFromRequest(r, share.Token)
	if !ok {
		s.renderPasswordPage(w, r, share)
		return
	}

	valid, err := s.apiClient.ValidateShareSession(r.Context(), share.Token, sessionToken)
	if err != nil || !valid {
		s.renderPasswordPage(w, r, share)
		return
	}

	if s.tryRedirectToSubdomainShare(w, r, share, restPath) {
		return
	}

	s.redirectToShareContext(w, r, share, record.HasPassword, restPath)
}

func (s *Server) tryRedirectToSubdomainShare(w http.ResponseWriter, r *http.Request, share core.Share, restPath string) bool {
	if strings.TrimSpace(share.ShareMode) != "subdomain" {
		return false
	}
	if !s.cfg.ShareSubdomainOn || strings.TrimSpace(s.cfg.ShareSubdomainBase) == "" {
		return false
	}

	absolute := s.buildSubdomainURL(share, restPath, r.URL.RawQuery)
	if absolute == "" {
		return false
	}

	http.Redirect(w, r, absolute, http.StatusTemporaryRedirect)
	return true
}

func (s *Server) redirectToShareContext(w http.ResponseWriter, r *http.Request, share core.Share, hasPassword bool, restPath string) {
	ctxID, err := s.createOrGetShareContext(share, hasPassword)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "create share context failed")
		return
	}

	targetURL := ensureContextPathPrefix(restPath, ctxID)
	if strings.TrimSpace(r.URL.RawQuery) != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	http.Redirect(w, r, targetURL, http.StatusTemporaryRedirect)
}

func (s *Server) handleGatewayContext(w http.ResponseWriter, r *http.Request) {
	s.logRoute(r, "share_ctx_entry", map[string]any{"host": r.Host, "path": r.URL.Path})
	w.Header().Set("X-Nonav-Route", "path_ctx")

	ctxID, restPath, ok := parseShareContextPath(r.URL.Path)
	if !ok {
		w.Header().Set("X-Nonav-Reason", "ctx_path_not_found")
		s.logRoute(r, "share_ctx_path_invalid", map[string]any{"path": r.URL.Path})
		respondError(w, http.StatusNotFound, "share context path not found")
		return
	}

	shareCtx, ok := s.getShareContext(ctxID)
	if !ok {
		w.Header().Set("X-Nonav-Reason", "ctx_not_found")
		s.logRoute(r, "share_ctx_not_found", map[string]any{"ctx_id": ctxID})
		respondError(w, http.StatusNotFound, "share context not found")
		return
	}

	if shareCtx.Share.Status != "active" || time.Now().UTC().After(shareCtx.Share.ExpiresAt) {
		s.deleteShareContext(ctxID)
		w.Header().Set("X-Nonav-Reason", "share_inactive")
		s.logRoute(r, "share_ctx_inactive", map[string]any{"ctx_id": ctxID, "status": shareCtx.Share.Status})
		respondError(w, http.StatusGone, "share is no longer active")
		return
	}

	if shareCtx.HasPassword {
		sessionToken, hasSession := getShareSessionTokenFromRequest(r, shareCtx.Share.Token)
		if !hasSession {
			respondError(w, http.StatusUnauthorized, "share session required")
			return
		}

		valid, err := s.apiClient.ValidateShareSession(r.Context(), shareCtx.Share.Token, sessionToken)
		if err != nil || !valid {
			respondError(w, http.StatusUnauthorized, "share session invalid")
			return
		}
	}

	s.proxyShareTarget(w, r, shareCtx, restPath)
}

func (s *Server) proxyShareTarget(w http.ResponseWriter, r *http.Request, shareCtx shareProxyContext, restPath string) {
	share := shareCtx.Share
	targetRaw := share.TargetURL
	if s.cfg.ForceFRP {
		if !isShareAllowedInForceFRP(share, s.cfg) {
			respondError(w, http.StatusBadGateway, "share upstream is not valid in force frp mode")
			return
		}

		targetRaw = share.TargetURL
	}

	targetURL, err := url.Parse(targetRaw)
	if err != nil {
		respondError(w, http.StatusBadGateway, "target url malformed")
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = joinURLPath(targetURL.Path, restPath)
		req.URL.RawQuery = sanitizeContextQuery(req.URL.Query())
		req.Host = targetURL.Host
		req.Header.Set("X-Forwarded-Host", r.Host)
		req.Header.Set("X-Forwarded-Proto", forwardedProto(r))
		rewriteProxyOriginHeaders(req, targetURL, share.Token, shareCtx.ID)
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		s.logRoute(r, "proxy_response", map[string]any{"status": resp.StatusCode, "upstream": targetURL.Host, "path": restPath})
		rewriteShareLocationHeader(resp, shareCtx.ID, r.Host)
		if err := injectShareContextScript(resp, shareCtx.ID); err != nil {
			return err
		}
		if err := rewriteJavaScriptModuleResponse(resp, shareCtx.ID); err != nil {
			return err
		}
		if err := rewriteVueStyleModuleResponse(resp, shareCtx.ID); err != nil {
			return err
		}

		if s.apiClient != nil {
			s.apiClient.LogShareAccess(context.Background(), share.Token, r.Method, r.URL.Path, clientIP(r), resp.StatusCode)
		}
		return nil
	}

	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
		s.logRoute(req, "proxy_error", map[string]any{"error": proxyErr.Error(), "upstream": targetURL.Host, "path": req.URL.Path})
		rw.Header().Set("X-Nonav-Reason", "proxy_failed")
		if s.apiClient != nil {
			s.apiClient.LogShareAccess(context.Background(), share.Token, req.Method, req.URL.Path, clientIP(req), http.StatusBadGateway)
		}
		respondError(rw, http.StatusBadGateway, "gateway proxy failed")
	}

	proxy.ServeHTTP(w, r)
}

func (s *Server) handleShareAuthByAPI(w http.ResponseWriter, r *http.Request, share core.Share) {
	if err := r.ParseForm(); err != nil {
		respondError(w, http.StatusBadRequest, "invalid form payload")
		return
	}

	password := r.Form.Get("password")
	if strings.TrimSpace(password) == "" {
		s.renderPasswordPageWithMessage(w, r, share, "请输入访问密码")
		return
	}

	sessionToken, expiresAt, err := s.apiClient.AuthorizeShare(r.Context(), share.Token, password)
	if err != nil {
		if errors.Is(err, errUnauthorized) {
			s.renderPasswordPageWithMessage(w, r, share, "密码错误，请重试")
			return
		}
		respondError(w, http.StatusBadGateway, "internal api authorize failed")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     shareSessionCookieName(share.Token),
		Value:    sessionToken,
		Path:     shareSessionCookiePath(share.Token),
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/s/"+share.Token+"/", http.StatusSeeOther)
}

func (s *Server) renderPasswordPage(w http.ResponseWriter, r *http.Request, share core.Share) {
	s.renderPasswordPageWithMessage(w, r, share, "")
}

func (s *Server) renderPasswordPageWithMessage(w http.ResponseWriter, _ *http.Request, share core.Share, message string) {
	html := `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>访问分享：` + share.SiteName + `</title>
  <style>
    body{font-family:"HarmonyOS Sans SC","Noto Sans SC","PingFang SC","Microsoft YaHei",sans-serif;background:#f3f6fb;color:#13202f;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0;padding:16px}
    .panel{width:100%;max-width:420px;background:#fff;border-radius:18px;padding:28px;box-shadow:0 20px 50px rgba(18,30,55,.12)}
    h1{margin:0 0 10px;font-size:22px}
    p{margin:0 0 18px;color:#536176}
    input{width:100%;padding:12px 14px;border:1px solid #d8dfec;border-radius:12px;font-size:15px;box-sizing:border-box;margin-bottom:12px}
    button{width:100%;padding:12px;border:none;border-radius:12px;background:#2f6bff;color:#fff;font-size:15px;cursor:pointer}
    .err{margin:0 0 12px;color:#d14343;font-size:14px}
  </style>
</head>
<body>
  <form class="panel" method="post" action="/s/` + share.Token + `/auth">
    <h1>输入访问密码</h1>
    <p>分享站点：` + share.SiteName + `</p>
    ` + conditionalMessage(message) + `
    <input type="password" name="password" placeholder="请输入分享密码" required autofocus />
    <button type="submit">继续访问</button>
  </form>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func (s *Server) withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", s.cfg.AllowedCORSOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next(w, r)
	}
}

func respondJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func respondError(w http.ResponseWriter, statusCode int, message string) {
	respondJSON(w, statusCode, map[string]string{"error": message})
}

func respondMethodNotAllowed(w http.ResponseWriter) {
	respondError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func decodeJSON(r *http.Request, target any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read request body failed")
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("invalid json payload")
	}

	return nil
}

func parseSharePath(path string) (string, string, bool) {
	trimmed := strings.TrimPrefix(path, "/s/")
	if trimmed == path || trimmed == "" {
		return "", "", false
	}

	parts := strings.SplitN(trimmed, "/", 2)
	token := strings.TrimSpace(parts[0])
	if token == "" {
		return "", "", false
	}

	rest := "/"
	if len(parts) == 2 {
		rest = "/" + parts[1]
	}

	return token, rest, true
}

func parseShareContextPath(path string) (string, string, bool) {
	trimmed := strings.TrimPrefix(path, "/x/")
	if trimmed == path || trimmed == "" {
		return "", "", false
	}

	parts := strings.SplitN(trimmed, "/", 2)
	ctxID := strings.TrimSpace(parts[0])
	if ctxID == "" {
		return "", "", false
	}

	rest := "/"
	if len(parts) == 2 {
		rest = "/" + parts[1]
	}

	return ctxID, rest, true
}

func ensureContextPathPrefix(path string, ctxID string) string {
	prefix := "/x/" + strings.TrimSpace(ctxID)
	if strings.HasPrefix(path, prefix) {
		return path
	}

	if path == "" || path == "/" {
		return prefix + "/"
	}

	if strings.HasPrefix(path, "/") {
		return prefix + path
	}

	return prefix + "/" + path
}

func sanitizeContextQuery(query url.Values) string {
	clean := query
	clean.Del("__nonav_share")
	return clean.Encode()
}

func joinURLPath(basePath string, childPath string) string {
	basePath = strings.TrimSuffix(basePath, "/")
	if childPath == "/" {
		if basePath == "" {
			return "/"
		}
		return basePath + "/"
	}

	return basePath + childPath
}

func normalizeSubdomainSlug(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}

	var builder strings.Builder
	prevDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			prevDash = false
			continue
		}

		if r == '-' || r == '_' || r == ' ' {
			if !prevDash {
				builder.WriteRune('-')
				prevDash = true
			}
		}
	}

	slug := strings.Trim(builder.String(), "-")
	if len(slug) > 63 {
		slug = strings.Trim(slug[:63], "-")
	}

	return slug
}

func isSubdomainConflictError(err error) bool {
	if err == nil {
		return false
	}

	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "idx_shares_subdomain_slug") || strings.Contains(lower, "subdomain_slug")
}

func (s *Server) buildShareURL(share core.Share) string {
	baseURL := strings.TrimRight(s.cfg.PublicBaseURL, "/")
	mode := strings.TrimSpace(share.ShareMode)
	if mode == "" {
		mode = "path_ctx"
	}

	if mode == "subdomain" && s.cfg.ShareSubdomainOn {
		if absolute := s.buildSubdomainURL(share, "/", ""); absolute != "" {
			return absolute
		}
	}

	return fmt.Sprintf("%s/s/%s", baseURL, share.Token)
}

func (s *Server) buildSubdomainURL(share core.Share, path string, rawQuery string) string {
	slug := normalizeSubdomainSlug(share.Subdomain)
	baseDomain := strings.TrimSpace(strings.ToLower(s.cfg.ShareSubdomainBase))
	if slug == "" || baseDomain == "" {
		return ""
	}

	baseURL := strings.TrimRight(s.cfg.PublicBaseURL, "/")
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}

	parsed.Host = slug + "." + baseDomain
	if parsed.Scheme == "" {
		parsed.Scheme = "https"
	}

	if strings.TrimSpace(path) == "" {
		path = "/"
	}
	parsed.Path = path
	parsed.RawQuery = strings.TrimSpace(rawQuery)
	parsed.Fragment = ""

	return parsed.String()
}

func extractShareSubdomainSlug(hostWithPort string, baseDomain string) (string, bool) {
	base := strings.TrimSpace(strings.ToLower(baseDomain))
	if base == "" {
		return "", false
	}

	host := strings.ToLower(strings.TrimSpace(hostWithPort))
	if strings.Contains(host, ":") {
		h, _, err := net.SplitHostPort(host)
		if err == nil {
			host = h
		}
	}

	if host == base {
		return "", false
	}

	suffix := "." + base
	if !strings.HasSuffix(host, suffix) {
		return "", false
	}

	slug := strings.TrimSuffix(host, suffix)
	if slug == "" || strings.Contains(slug, ".") {
		return "", false
	}

	return slug, true
}

func requestRoutingHost(r *http.Request) string {
	forwardedHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if forwardedHost != "" {
		parts := strings.Split(forwardedHost, ",")
		if len(parts) > 0 {
			candidate := strings.TrimSpace(parts[0])
			if candidate != "" {
				return candidate
			}
		}
	}

	originalHost := strings.TrimSpace(r.Header.Get("X-Original-Host"))
	if originalHost != "" {
		return originalHost
	}

	return strings.TrimSpace(r.Host)
}

func conditionalMessage(msg string) string {
	if msg == "" {
		return ""
	}
	return `<p class="err">` + msg + `</p>`
}

func clientIP(r *http.Request) string {
	forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}

	return r.RemoteAddr
}

func forwardedProto(r *http.Request) string {
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if proto != "" {
		parts := strings.Split(proto, ",")
		if len(parts) > 0 {
			p := strings.TrimSpace(parts[0])
			if p != "" {
				return p
			}
		}
	}

	if r.TLS != nil {
		return "https"
	}

	return "http"
}

func rewriteProxyOriginHeaders(req *http.Request, targetURL *url.URL, shareToken string, ctxID string) {
	targetOrigin := targetURL.Scheme + "://" + targetURL.Host

	if shouldStripOriginForStatic(req.URL.Path, req.Method) {
		req.Header.Del("Origin")
		req.Header.Del("Referer")
		return
	}

	origin := strings.TrimSpace(req.Header.Get("Origin"))
	if origin != "" {
		req.Header.Set("Origin", targetOrigin)
	}

	referer := strings.TrimSpace(req.Header.Get("Referer"))
	if referer == "" {
		return
	}

	parsedReferer, err := url.Parse(referer)
	if err != nil {
		req.Header.Del("Referer")
		return
	}

	ctxToken, restPath, ok := parseShareContextPath(parsedReferer.Path)
	if ok && ctxToken == ctxID {
		parsedReferer.Scheme = targetURL.Scheme
		parsedReferer.Host = targetURL.Host
		parsedReferer.Path = joinURLPath(targetURL.Path, restPath)
		parsedReferer.RawQuery = sanitizeContextQuery(parsedReferer.Query())
		req.Header.Set("Referer", parsedReferer.String())
		return
	}

	token, shareRestPath, shareMatched := parseSharePath(parsedReferer.Path)
	if shareMatched && token == shareToken {
		parsedReferer.Scheme = targetURL.Scheme
		parsedReferer.Host = targetURL.Host
		parsedReferer.Path = joinURLPath(targetURL.Path, shareRestPath)
		req.Header.Set("Referer", parsedReferer.String())
		return
	}

	req.Header.Set("Referer", targetOrigin+"/")
}

func shouldStripOriginForStatic(path string, method string) bool {
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}

	cleanPath := strings.ToLower(strings.TrimSpace(path))
	if strings.Contains(cleanPath, "?") {
		cleanPath = strings.SplitN(cleanPath, "?", 2)[0]
	}

	if strings.Contains(cleanPath, "/assets/") {
		return true
	}

	staticExts := []string{
		".js", ".mjs", ".css", ".map", ".json", ".ico", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".woff", ".woff2", ".ttf", ".eot", ".wasm",
	}

	for _, ext := range staticExts {
		if strings.HasSuffix(cleanPath, ext) {
			return true
		}
	}

	return false
}

func isWebSocketRequest(r *http.Request) bool {
	connection := strings.ToLower(strings.TrimSpace(r.Header.Get("Connection")))
	if connection == "" || !strings.Contains(connection, "upgrade") {
		return false
	}

	upgrade := strings.TrimSpace(r.Header.Get("Upgrade"))
	return strings.EqualFold(upgrade, "websocket")
}

func isLikelyDocumentRequest(r *http.Request) bool {
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Dest")), "document") {
		return true
	}

	accept := strings.ToLower(strings.TrimSpace(r.Header.Get("Accept")))
	return strings.Contains(accept, "text/html")
}

func (s *Server) handleFrontend(w http.ResponseWriter, r *http.Request) {
	if !s.serveFrontend {
		if s.tryHandleSubdomainShareRequest(w, r) {
			return
		}
		w.Header().Set("X-Nonav-Route", "fallback")
		w.Header().Set("X-Nonav-Reason", "gateway_non_share_path")
		s.logRoute(r, "fallback_not_found", map[string]any{"host": r.Host, "path": r.URL.Path})

		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		respondMethodNotAllowed(w)
		return
	}

	if s.frontendDevProxy != nil {
		s.frontendDevProxy.ServeHTTP(w, r)
		return
	}

	requestPath := filepath.Clean(r.URL.Path)
	if requestPath == "." {
		requestPath = "/"
	}

	if requestPath == "/" {
		s.serveIndexHTML(w, r)
		return
	}

	if s.tryServeEmbeddedFrontendFile(w, r, requestPath) {
		return
	}

	trimmed := strings.TrimPrefix(requestPath, "/")
	filePath := filepath.Join(s.cfg.WebDistDir, trimmed)

	if stat, err := os.Stat(filePath); err == nil && !stat.IsDir() {
		if strings.HasPrefix(requestPath, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeFile(w, r, filePath)
		return
	}

	if hasFileExtension(requestPath) {
		http.NotFound(w, r)
		return
	}

	s.serveIndexHTML(w, r)
}

func (s *Server) serveIndexHTML(w http.ResponseWriter, r *http.Request) {
	if s.tryServeEmbeddedIndex(w, r) {
		return
	}

	indexPath := filepath.Join(s.cfg.WebDistDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		respondError(w, http.StatusNotFound, "frontend build not found, run make build")
		return
	}

	w.Header().Set("Cache-Control", "no-cache")

	http.ServeFile(w, r, indexPath)
}

func (s *Server) handleAPIProxy(w http.ResponseWriter, r *http.Request) {
	if s.tryHandleSubdomainShareRequest(w, r) {
		return
	}
	w.Header().Set("X-Nonav-Route", "fallback")
	w.Header().Set("X-Nonav-Reason", "api_proxy_not_matched")
	s.logRoute(r, "api_proxy_not_matched", map[string]any{"path": r.URL.Path})

	http.NotFound(w, r)
}

func (s *Server) tryHandleSubdomainShareRequest(w http.ResponseWriter, r *http.Request) bool {
	if s.apiClient == nil {
		return false
	}

	routingHost := requestRoutingHost(r)
	slug, ok := extractShareSubdomainSlug(routingHost, s.cfg.ShareSubdomainBase)
	if !ok {
		s.logRoute(r, "subdomain_not_matched", map[string]any{"host": r.Host, "routing_host": routingHost, "base": s.cfg.ShareSubdomainBase})
		if strings.EqualFold(strings.TrimSpace(routingHost), "localhost") || strings.HasPrefix(strings.TrimSpace(routingHost), "localhost:") {
			s.logRoute(r, "subdomain_proxy_host_warning", map[string]any{"hint": "proxy should preserve Host or set X-Forwarded-Host", "host": r.Host, "routing_host": routingHost})
		}
		return false
	}

	w.Header().Set("X-Nonav-Route", "subdomain")
	w.Header().Set("X-Nonav-Subdomain-Slug", slug)
	s.logRoute(r, "subdomain_matched", map[string]any{"host": r.Host, "routing_host": routingHost, "slug": slug, "base": s.cfg.ShareSubdomainBase})

	if !s.cfg.ShareSubdomainOn {
		w.Header().Set("X-Nonav-Reason", "subdomain_disabled")
		s.logRoute(r, "subdomain_disabled", map[string]any{"slug": slug})
		respondError(w, http.StatusServiceUnavailable, "subdomain sharing is disabled")
		return true
	}

	startedAt := time.Now()
	record, err := s.apiClient.GetShareBySubdomain(r.Context(), slug)
	if err != nil {
		if errors.Is(err, errNotFound) {
			w.Header().Set("X-Nonav-Reason", "subdomain_share_not_found")
			s.logRoute(r, "subdomain_share_not_found", map[string]any{"slug": slug, "latency_ms": time.Since(startedAt).Milliseconds()})
			respondError(w, http.StatusNotFound, "subdomain share not found")
			return true
		}

		w.Header().Set("X-Nonav-Reason", "internal_api_unavailable")
		s.logRoute(r, "subdomain_internal_api_failed", map[string]any{"slug": slug, "error": err.Error(), "latency_ms": time.Since(startedAt).Milliseconds()})
		respondError(w, http.StatusBadGateway, "internal api unavailable")
		return true
	}
	s.logRoute(r, "subdomain_share_loaded", map[string]any{"slug": slug, "token": record.Share.Token, "latency_ms": time.Since(startedAt).Milliseconds()})

	shareCtx := shareProxyContext{
		Share:       record.Share,
		HasPassword: record.HasPassword,
	}

	if shareCtx.Share.Status != "active" || time.Now().UTC().After(shareCtx.Share.ExpiresAt) {
		w.Header().Set("X-Nonav-Reason", "share_inactive")
		s.logRoute(r, "subdomain_share_inactive", map[string]any{"slug": slug, "status": shareCtx.Share.Status})
		respondError(w, http.StatusGone, "share is no longer active")
		return true
	}

	if shareCtx.HasPassword {
		sessionToken, hasSession := getShareSessionTokenFromRequest(r, shareCtx.Share.Token)
		if !hasSession {
			s.logRoute(r, "subdomain_password_required", map[string]any{"slug": slug})
			s.renderPasswordPage(w, r, shareCtx.Share)
			return true
		}

		valid, validateErr := s.apiClient.ValidateShareSession(r.Context(), shareCtx.Share.Token, sessionToken)
		if validateErr != nil || !valid {
			w.Header().Set("X-Nonav-Reason", "share_session_invalid")
			s.logRoute(r, "subdomain_session_invalid", map[string]any{"slug": slug})
			s.renderPasswordPage(w, r, shareCtx.Share)
			return true
		}
	}

	s.logRoute(r, "subdomain_proxy_start", map[string]any{"slug": slug, "path": r.URL.Path})
	s.proxyShareTarget(w, r, shareCtx, r.URL.Path)
	return true
}

func hasFileExtension(path string) bool {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return ext != ""
}

func shareSessionCookieName(shareToken string) string {
	cleanToken := strings.TrimSpace(shareToken)
	if cleanToken == "" {
		return "nonav_share_session"
	}

	return "nonav_share_session_" + cleanToken
}

func shareSessionCookiePath(shareToken string) string {
	_ = shareToken
	return "/"
}

func getShareSessionTokenFromRequest(r *http.Request, shareToken string) (string, bool) {
	specificCookie, specificErr := r.Cookie(shareSessionCookieName(shareToken))
	if specificErr == nil {
		value := strings.TrimSpace(specificCookie.Value)
		if value != "" {
			return value, true
		}
	}

	legacyCookie, legacyErr := r.Cookie(shareSessionCookieName(""))
	if legacyErr == nil {
		value := strings.TrimSpace(legacyCookie.Value)
		if value != "" {
			return value, true
		}
	}

	return "", false
}

func (s *Server) localFrontendAssetExists(requestPath string) bool {
	if s.embeddedFrontendFileExists(requestPath) {
		return true
	}

	if strings.TrimSpace(s.cfg.WebDistDir) == "" {
		return false
	}

	trimmed := strings.TrimPrefix(filepath.Clean(requestPath), "/")
	if trimmed == "" || trimmed == "." {
		return false
	}

	filePath := filepath.Join(s.cfg.WebDistDir, trimmed)
	if stat, err := os.Stat(filePath); err == nil && !stat.IsDir() {
		return true
	}

	return false
}

func (s *Server) embeddedFrontendFileExists(requestPath string) bool {
	frontendFS := getEmbeddedFrontendFS()
	if frontendFS == nil {
		return false
	}

	clean := strings.TrimPrefix(filepath.Clean(requestPath), "/")
	if clean == "" || clean == "." {
		return false
	}

	stat, err := fs.Stat(frontendFS, clean)
	if err != nil {
		return false
	}

	return !stat.IsDir()
}

func (s *Server) tryServeEmbeddedIndex(w http.ResponseWriter, r *http.Request) bool {
	frontendFS := getEmbeddedFrontendFS()
	if frontendFS == nil {
		return false
	}

	data, err := fs.ReadFile(frontendFS, "index.html")
	if err != nil {
		return false
	}

	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
	return true
}

func (s *Server) tryServeEmbeddedFrontendFile(w http.ResponseWriter, r *http.Request, requestPath string) bool {
	frontendFS := getEmbeddedFrontendFS()
	if frontendFS == nil {
		return false
	}

	clean := strings.TrimPrefix(filepath.Clean(requestPath), "/")
	if clean == "" || clean == "." {
		return false
	}

	stat, err := fs.Stat(frontendFS, clean)
	if err != nil || stat.IsDir() {
		return false
	}

	if strings.HasPrefix(requestPath, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}

	http.ServeFileFS(w, r, frontendFS, clean)
	return true
}

func ensureUpstreamReachable(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid upstream url")
	}

	hostPort := parsed.Host
	if hostPort == "" {
		return fmt.Errorf("missing upstream host")
	}

	if !strings.Contains(hostPort, ":") {
		if strings.EqualFold(parsed.Scheme, "https") {
			hostPort += ":443"
		} else {
			hostPort += ":80"
		}
	}

	conn, dialErr := net.DialTimeout("tcp", hostPort, 2*time.Second)
	if dialErr != nil {
		return fmt.Errorf("dial %s failed", hostPort)
	}
	_ = conn.Close()
	return nil
}

func ensureUpstreamReachableWithRetry(rawURL string, attempts int, interval time.Duration) error {
	if attempts <= 1 {
		return ensureUpstreamReachable(rawURL)
	}

	var lastErr error
	for idx := 0; idx < attempts; idx++ {
		if err := ensureUpstreamReachable(rawURL); err == nil {
			return nil
		} else {
			lastErr = err
		}

		time.Sleep(interval)
	}

	if lastErr != nil {
		return lastErr
	}

	return fmt.Errorf("upstream unreachable")
}

func (s *Server) selectFRPUpstreamForShare(ctx context.Context) (int, string, error) {
	if strings.TrimSpace(s.cfg.FRPUpstreamURL) == "" {
		return 0, "", fmt.Errorf("frp upstream url is empty")
	}

	if s.cfg.FRPPortMin <= 0 || s.cfg.FRPPortMax <= 0 || s.cfg.FRPPortMin > s.cfg.FRPPortMax {
		return 0, "", fmt.Errorf("frp port range is invalid")
	}

	usedPorts, err := s.store.GetUsedFRPPorts(ctx)
	if err != nil {
		return 0, "", err
	}

	for port := s.cfg.FRPPortMin; port <= s.cfg.FRPPortMax; port++ {
		if _, exists := usedPorts[port]; exists {
			continue
		}

		upstreamURL, buildErr := buildFRPUpstreamURL(s.cfg.FRPUpstreamURL, port)
		if buildErr != nil {
			return 0, "", buildErr
		}

		return port, upstreamURL, nil
	}

	return 0, "", fmt.Errorf("no available frp upstream port in range %d-%d", s.cfg.FRPPortMin, s.cfg.FRPPortMax)
}

func buildFRPUpstreamURL(baseURL string, port int) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("invalid frp upstream url")
	}

	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("frp upstream host is empty")
	}

	parsed.Host = net.JoinHostPort(host, strconv.Itoa(port))
	if parsed.Path == "" {
		parsed.Path = "/"
	}

	return parsed.String(), nil
}

func parseSiteUpstream(rawURL string) (string, int, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", 0, fmt.Errorf("invalid site url")
	}

	host := parsed.Hostname()
	if host == "" {
		return "", 0, fmt.Errorf("site host is empty")
	}

	portText := parsed.Port()
	if portText == "" {
		if strings.EqualFold(parsed.Scheme, "https") {
			portText = "443"
		} else {
			portText = "80"
		}
	}

	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 {
		return "", 0, fmt.Errorf("site port is invalid")
	}

	return host, port, nil
}

func isShareAllowedInForceFRP(share core.Share, cfg config.Config) bool {
	if share.FRPPort < cfg.FRPPortMin || share.FRPPort > cfg.FRPPortMax {
		return false
	}

	parsedShare, err := url.Parse(strings.TrimSpace(share.TargetURL))
	if err != nil {
		return false
	}

	parsedBase, err := url.Parse(strings.TrimSpace(cfg.FRPUpstreamURL))
	if err != nil {
		return false
	}

	if !strings.EqualFold(parsedShare.Hostname(), parsedBase.Hostname()) {
		return false
	}

	sharePort := parsedShare.Port()
	if sharePort == "" {
		return false
	}

	parsedPort, err := strconv.Atoi(sharePort)
	if err != nil {
		return false
	}

	return parsedPort == share.FRPPort
}

func portFromListenAddr(addr string) (int, error) {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return 0, fmt.Errorf("listen addr is empty")
	}

	if strings.HasPrefix(trimmed, ":") {
		port, err := strconv.Atoi(strings.TrimPrefix(trimmed, ":"))
		if err != nil || port <= 0 {
			return 0, fmt.Errorf("invalid listen addr: %s", addr)
		}
		return port, nil
	}

	_, portText, err := net.SplitHostPort(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid listen addr: %s", addr)
	}

	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 {
		return 0, fmt.Errorf("invalid listen addr port: %s", addr)
	}

	return port, nil
}

func (s *Server) lockSite(siteID int64) func() {
	lockValue, _ := s.siteLocks.LoadOrStore(siteID, &sync.Mutex{})
	mu, _ := lockValue.(*sync.Mutex)
	mu.Lock()
	return func() {
		mu.Unlock()
	}
}

func (s *Server) createOrGetShareContext(share core.Share, hasPassword bool) (string, error) {
	now := time.Now().UTC()

	s.shareCtxMu.Lock()
	defer s.shareCtxMu.Unlock()

	s.cleanupExpiredShareContextsLocked(now)

	if existingID, ok := s.shareCtxIDByToken[share.Token]; ok {
		if existingCtx, ok := s.shareCtxByID[existingID]; ok {
			if now.Before(existingCtx.ExpiresAt) {
				return existingID, nil
			}
		}
	}

	ctxID, err := generateRandomToken(18)
	if err != nil {
		return "", err
	}

	expiresAt := now.Add(8 * time.Hour)
	if share.ExpiresAt.Before(expiresAt) {
		expiresAt = share.ExpiresAt
	}

	shareCtx := shareProxyContext{
		ID:          ctxID,
		Share:       share,
		HasPassword: hasPassword,
		ExpiresAt:   expiresAt,
	}

	s.shareCtxByID[ctxID] = shareCtx
	s.shareCtxIDByToken[share.Token] = ctxID

	return ctxID, nil
}

func (s *Server) getShareContext(ctxID string) (shareProxyContext, bool) {
	now := time.Now().UTC()

	s.shareCtxMu.Lock()
	defer s.shareCtxMu.Unlock()

	shareCtx, ok := s.shareCtxByID[ctxID]
	if !ok {
		return shareProxyContext{}, false
	}

	if now.After(shareCtx.ExpiresAt) {
		delete(s.shareCtxByID, ctxID)
		delete(s.shareCtxIDByToken, shareCtx.Share.Token)
		return shareProxyContext{}, false
	}

	return shareCtx, true
}

func (s *Server) deleteShareContext(ctxID string) {
	s.shareCtxMu.Lock()
	defer s.shareCtxMu.Unlock()

	shareCtx, ok := s.shareCtxByID[ctxID]
	if !ok {
		return
	}

	delete(s.shareCtxByID, ctxID)
	delete(s.shareCtxIDByToken, shareCtx.Share.Token)
}

func (s *Server) cleanupExpiredShareContextsLocked(now time.Time) {
	for ctxID, shareCtx := range s.shareCtxByID {
		if now.After(shareCtx.ExpiresAt) {
			delete(s.shareCtxByID, ctxID)
			delete(s.shareCtxIDByToken, shareCtx.Share.Token)
		}
	}
}

func (s *Server) RecoverFRPProxies(ctx context.Context) error {
	if !s.cfg.ForceFRP {
		return nil
	}

	shares, err := s.store.ListShares(ctx)
	if err != nil {
		return err
	}

	for _, share := range shares {
		if share.Status != "active" || share.FRPPort <= 0 {
			continue
		}

		site, siteErr := s.store.GetSiteByID(ctx, share.SiteID)
		if siteErr != nil {
			continue
		}

		localHost, localPort, parseErr := parseSiteUpstream(site.URL)
		if parseErr != nil {
			continue
		}

		proxyName := fmt.Sprintf("share-%d-%d", share.SiteID, share.FRPPort)
		if startErr := s.frpManager.StartProxy(FRPProxySpec{
			ShareID:    share.SiteID,
			ProxyName:  proxyName,
			LocalHost:  localHost,
			LocalPort:  localPort,
			RemotePort: share.FRPPort,
		}); startErr != nil {
			continue
		}

		_ = ensureUpstreamReachableWithRetry(share.TargetURL, 4, 200*time.Millisecond)
	}

	return nil
}

func (s *Server) CleanupExpiredShares(ctx context.Context) error {
	if s.cfg.ForceFRP {
		siteIDs, err := s.store.ListExpiredActiveShareSiteIDs(ctx)
		if err != nil {
			return err
		}

		for _, siteID := range siteIDs {
			_ = s.frpManager.StopProxy(siteID)
		}
	}

	return s.store.PurgeExpiredShares(ctx)
}

func rewriteShareLocationHeader(resp *http.Response, shareToken string, requestHost string) {
	location := strings.TrimSpace(resp.Header.Get("Location"))
	if location == "" {
		return
	}

	rewritten, ok := rewriteShareLocation(location, shareToken, requestHost)
	if !ok {
		return
	}

	resp.Header.Set("Location", rewritten)
}

func injectShareContextScript(resp *http.Response, ctxID string) error {
	if resp == nil || resp.Request == nil {
		return nil
	}

	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if !strings.Contains(contentType, "text/html") {
		return nil
	}

	if strings.TrimSpace(resp.Header.Get("Content-Encoding")) != "" {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	htmlText := string(body)
	if strings.Contains(htmlText, "id=\"__nonav_ctx_prefix_script\"") {
		resp.Body = io.NopCloser(strings.NewReader(htmlText))
		resp.ContentLength = int64(len(htmlText))
		resp.Header.Set("Content-Length", strconv.Itoa(len(htmlText)))
		return nil
	}

	cleanCtxID := strings.TrimSpace(ctxID)
	if cleanCtxID == "" {
		resp.Body = io.NopCloser(strings.NewReader(htmlText))
		resp.ContentLength = int64(len(htmlText))
		resp.Header.Set("Content-Length", strconv.Itoa(len(htmlText)))
		return nil
	}

	htmlText = strings.ReplaceAll(htmlText, "src=\"/", "src=\"/x/"+cleanCtxID+"/")
	htmlText = strings.ReplaceAll(htmlText, "href=\"/", "href=\"/x/"+cleanCtxID+"/")
	htmlText = strings.ReplaceAll(htmlText, "action=\"/", "action=\"/x/"+cleanCtxID+"/")

	script := buildShareContextScript(cleanCtxID)
	injectAt := strings.LastIndex(strings.ToLower(htmlText), "</head>")
	if injectAt >= 0 {
		htmlText = htmlText[:injectAt] + script + htmlText[injectAt:]
	} else {
		htmlText = script + htmlText
	}

	resp.Body = io.NopCloser(strings.NewReader(htmlText))
	resp.ContentLength = int64(len(htmlText))
	resp.Header.Set("Content-Length", strconv.Itoa(len(htmlText)))
	resp.Header.Del("ETag")

	return nil
}

func buildShareContextScript(ctxID string) string {
	prefixLiteral := strconv.Quote("/x/" + strings.TrimSpace(ctxID))

	return "<script id=\"__nonav_ctx_prefix_script\">(function(){" +
		"var PREFIX=" + prefixLiteral + ";" +
		"function withCtx(input){try{if(typeof input!=='string'){return input;}var abs=new URL(input,window.location.href);if(abs.origin!==window.location.origin){return input;}" +
		"if(abs.pathname===PREFIX||abs.pathname.indexOf(PREFIX+'/')===0){return abs.pathname+abs.search+abs.hash;}" +
		"abs.pathname=PREFIX+(abs.pathname==='/'?'':abs.pathname);return abs.pathname+abs.search+abs.hash;}catch(_e){return input;}}" +
		"var push=history.pushState;history.pushState=function(s,t,url){if(typeof url==='string'){url=withCtx(url);}return push.call(this,s,t,url);};" +
		"var replace=history.replaceState;history.replaceState=function(s,t,url){if(typeof url==='string'){url=withCtx(url);}return replace.call(this,s,t,url);};" +
		"if(window.fetch){var oldFetch=window.fetch;window.fetch=function(input,init){if(typeof input==='string'){input=withCtx(input);}return oldFetch.call(this,input,init);};}" +
		"if(window.XMLHttpRequest&&window.XMLHttpRequest.prototype&&window.XMLHttpRequest.prototype.open){var oldOpen=window.XMLHttpRequest.prototype.open;window.XMLHttpRequest.prototype.open=function(method,url){if(typeof url==='string'){url=withCtx(url);}return oldOpen.apply(this,[method,url].concat([].slice.call(arguments,2)));};}" +
		"})();</script>"
}

func rewriteVueStyleModuleResponse(resp *http.Response, ctxID string) error {
	if resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return nil
	}

	query := resp.Request.URL.Query()
	if _, hasVue := query["vue"]; !hasVue || query.Get("type") != "style" {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	cssText := string(body)
	trimmed := strings.TrimSpace(cssText)
	if trimmed == "" {
		resp.Body = io.NopCloser(strings.NewReader(cssText))
		resp.ContentLength = int64(len(cssText))
		resp.Header.Set("Content-Length", strconv.Itoa(len(cssText)))
		return nil
	}

	if strings.HasPrefix(trimmed, "import ") || strings.Contains(trimmed, "__vite__updateStyle") {
		resp.Body = io.NopCloser(strings.NewReader(cssText))
		resp.ContentLength = int64(len(cssText))
		resp.Header.Set("Content-Length", strconv.Itoa(len(cssText)))
		return nil
	}

	moduleID := withContextPathPrefix(resp.Request.URL.Path, ctxID)
	if resp.Request.URL.RawQuery != "" {
		moduleID += "?" + sanitizeContextQuery(resp.Request.URL.Query())
	}

	wrapped := "import { updateStyle as __vite__updateStyle, removeStyle as __vite__removeStyle } from \"" + withContextPathPrefix("/@vite/client", ctxID) + "\"\n" +
		"const __vite__id = " + strconv.Quote(moduleID) + "\n" +
		"const __vite__css = " + strconv.Quote(cssText) + "\n" +
		"__vite__updateStyle(__vite__id, __vite__css)\n" +
		"if (import.meta.hot) {\n" +
		"  import.meta.hot.accept()\n" +
		"  import.meta.hot.prune(() => __vite__removeStyle(__vite__id))\n" +
		"}\n"

	resp.Body = io.NopCloser(strings.NewReader(wrapped))
	resp.ContentLength = int64(len(wrapped))
	resp.Header.Set("Content-Type", "text/javascript")
	resp.Header.Set("Content-Length", strconv.Itoa(len(wrapped)))
	resp.Header.Del("ETag")
	resp.Header.Del("Content-Encoding")

	return nil
}

var (
	reModuleImportFromDouble = regexp.MustCompile(`from\s+"(/[^"\n]+)"`)
	reModuleImportFromSingle = regexp.MustCompile(`from\s+'(/[^'\n]+)'`)
	reModuleImportBareDouble = regexp.MustCompile(`import\s+"(/[^"\n]+)"`)
	reModuleImportBareSingle = regexp.MustCompile(`import\s+'(/[^'\n]+)'`)
	reModuleImportDynDouble  = regexp.MustCompile(`import\(\s*"(/[^"\n]+)"\s*\)`)
	reModuleImportDynSingle  = regexp.MustCompile(`import\(\s*'(/[^'\n]+)'\s*\)`)
)

func rewriteJavaScriptModuleResponse(resp *http.Response, ctxID string) error {
	if resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return nil
	}

	if strings.TrimSpace(resp.Header.Get("Content-Encoding")) != "" {
		return nil
	}

	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if !strings.Contains(contentType, "javascript") && !strings.Contains(contentType, "ecmascript") {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	original := string(body)
	rewritten := rewriteModuleImportSpecifiers(original, ctxID)
	if rewritten == original {
		resp.Body = io.NopCloser(strings.NewReader(original))
		resp.ContentLength = int64(len(original))
		resp.Header.Set("Content-Length", strconv.Itoa(len(original)))
		return nil
	}

	resp.Body = io.NopCloser(strings.NewReader(rewritten))
	resp.ContentLength = int64(len(rewritten))
	resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	resp.Header.Del("ETag")

	return nil
}

func rewriteModuleImportSpecifiers(code string, ctxID string) string {
	if strings.TrimSpace(ctxID) == "" || code == "" {
		return code
	}

	patterns := []*regexp.Regexp{
		reModuleImportFromDouble,
		reModuleImportFromSingle,
		reModuleImportBareDouble,
		reModuleImportBareSingle,
		reModuleImportDynDouble,
		reModuleImportDynSingle,
	}

	out := code
	for _, re := range patterns {
		out = re.ReplaceAllStringFunc(out, func(m string) string {
			sub := re.FindStringSubmatch(m)
			if len(sub) < 2 {
				return m
			}

			originalPath := sub[1]
			rewrittenPath := withContextPathPrefix(originalPath, ctxID)
			if rewrittenPath == originalPath {
				return m
			}

			return strings.Replace(m, originalPath, rewrittenPath, 1)
		})
	}

	return out
}

func withContextPathPrefix(rawPath string, ctxID string) string {
	if strings.TrimSpace(ctxID) == "" {
		return rawPath
	}

	if strings.TrimSpace(rawPath) == "" {
		return rawPath
	}
	if !strings.HasPrefix(rawPath, "/") || strings.HasPrefix(rawPath, "//") {
		return rawPath
	}

	if strings.HasPrefix(rawPath, "/x/") {
		return rawPath
	}

	return ensureContextPathPrefix(rawPath, ctxID)
}

func rewriteShareLocation(rawLocation string, ctxID string, requestHost string) (string, bool) {
	if strings.TrimSpace(ctxID) == "" {
		return "", false
	}

	parsed, err := url.Parse(rawLocation)
	if err != nil {
		return "", false
	}

	if parsed.IsAbs() {
		if !strings.EqualFold(parsed.Host, requestHost) {
			return "", false
		}

		parsed.Path = ensureContextPathPrefix(parsed.Path, ctxID)
		return parsed.String(), true
	}

	if strings.HasPrefix(rawLocation, "/") {
		return ensureContextPathPrefix(rawLocation, ctxID), true
	}

	return "", false
}
