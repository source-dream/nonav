package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"nonav/server/internal/config"
	"nonav/server/internal/core"
	"nonav/server/internal/store"
)

type Server struct {
	cfg              config.Config
	store            *store.SQLiteStore
	mux              *http.ServeMux
	apiProxy         *httputil.ReverseProxy
	frontendDevProxy *httputil.ReverseProxy
}

func NewAPI(cfg config.Config, st *store.SQLiteStore) (*Server, error) {
	s := &Server{
		cfg:   cfg,
		store: st,
		mux:   http.NewServeMux(),
	}

	s.routesAPI()
	return s, nil
}

func NewGateway(cfg config.Config, st *store.SQLiteStore) (*Server, error) {
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
		cfg:              cfg,
		store:            st,
		mux:              http.NewServeMux(),
		apiProxy:         httputil.NewSingleHostReverseProxy(apiURL),
		frontendDevProxy: frontendProxy,
	}

	s.routesGateway()
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routesAPI() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/api/sites", s.withCORS(s.handleSites))
	s.mux.HandleFunc("/api/sites/", s.withCORS(s.handleSiteActions))
	s.mux.HandleFunc("/api/shares", s.withCORS(s.handleShares))
	s.mux.HandleFunc("/api/shares/", s.withCORS(s.handleShareActions))
}

func (s *Server) routesGateway() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/api/", s.handleAPIProxy)
	s.mux.HandleFunc("/api", s.handleAPIProxy)
	s.mux.HandleFunc("/s/", s.handleGateway)
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

	if err := s.store.DeleteSharesBySite(r.Context(), siteID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
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

		baseURL := strings.TrimRight(s.cfg.PublicBaseURL, "/")
		items := make([]map[string]any, 0, len(shares))
		for _, share := range shares {
			items = append(items, map[string]any{
				"id":         share.ID,
				"siteId":     share.SiteID,
				"siteName":   share.SiteName,
				"token":      share.Token,
				"status":     share.Status,
				"expiresAt":  share.ExpiresAt,
				"createdAt":  share.CreatedAt,
				"updatedAt":  share.UpdatedAt,
				"stoppedAt":  share.StoppedAt,
				"accessHits": share.AccessHits,
				"shareUrl":   fmt.Sprintf("%s/s/%s", baseURL, share.Token),
			})
		}

		respondJSON(w, http.StatusOK, map[string]any{"shares": items})
	case http.MethodPost:
		var payload struct {
			SiteID         int64  `json:"siteId"`
			ExpiresInHours int    `json:"expiresInHours"`
			Password       string `json:"password"`
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

		expires := s.cfg.DefaultShareTTL
		if payload.ExpiresInHours > 0 {
			expires = time.Duration(payload.ExpiresInHours) * time.Hour
		}

		shareToken, err := generateRandomToken(18)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
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

		targetURL := site.URL
		if s.cfg.ForceFRP {
			targetURL = strings.TrimSpace(s.cfg.FRPUpstreamURL)
			if targetURL == "" {
				respondError(w, http.StatusInternalServerError, "frp upstream url is empty")
				return
			}

			if err := ensureUpstreamReachable(targetURL); err != nil {
				respondError(w, http.StatusBadGateway, "frp upstream unavailable: "+err.Error())
				return
			}
		}

		created, err := s.store.CreateShare(r.Context(), core.Share{
			SiteID:    site.ID,
			SiteName:  site.Name,
			TargetURL: targetURL,
			Token:     shareToken,
			Status:    "active",
			ExpiresAt: time.Now().UTC().Add(expires),
		}, passwordHash)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		baseURL := strings.TrimRight(s.cfg.PublicBaseURL, "/")
		respondJSON(w, http.StatusCreated, core.ShareWithPassword{
			Share:         created,
			ShareURL:      fmt.Sprintf("%s/s/%s", baseURL, created.Token),
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

		if err := s.store.StopShare(r.Context(), shareID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				respondError(w, http.StatusNotFound, "share not found")
				return
			}
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
		return
	}

	respondError(w, http.StatusNotFound, "unknown share action")
}

func (s *Server) handleGateway(w http.ResponseWriter, r *http.Request) {
	shareToken, restPath, ok := parseSharePath(r.URL.Path)
	if !ok {
		respondError(w, http.StatusNotFound, "share path not found")
		return
	}

	share, passwordHash, err := s.store.GetShareByToken(r.Context(), shareToken)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			respondError(w, http.StatusNotFound, "share not found")
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if share.Status != "active" || time.Now().UTC().After(share.ExpiresAt) {
		respondError(w, http.StatusGone, "share is no longer active")
		return
	}

	if strings.TrimSpace(passwordHash) == "" {
		s.setShareRouteCookie(w, share)
		s.proxyShareTarget(w, r, share, restPath)
		return
	}

	if restPath == "/auth" && r.Method == http.MethodPost {
		s.handleShareAuth(w, r, share, passwordHash)
		return
	}

	sessionCookieName := shareSessionCookieName()
	sessionCookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		s.renderPasswordPage(w, r, share)
		return
	}

	valid, err := s.store.ValidateShareSession(r.Context(), share.ID, sessionCookie.Value)
	if err != nil || !valid {
		s.renderPasswordPage(w, r, share)
		return
	}

	s.setShareRouteCookie(w, share)

	s.proxyShareTarget(w, r, share, restPath)
}

func (s *Server) proxyShareTarget(w http.ResponseWriter, r *http.Request, share core.Share, restPath string) {
	targetRaw := share.TargetURL
	if s.cfg.ForceFRP {
		targetRaw = strings.TrimSpace(s.cfg.FRPUpstreamURL)
		if targetRaw == "" {
			respondError(w, http.StatusBadGateway, "frp upstream url is empty")
			return
		}
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
		req.Host = targetURL.Host
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		rewriteShareLocationHeader(resp, share.Token, r.Host)

		remoteIP := clientIP(r)
		if err := s.store.LogShareAccess(context.Background(), share.ID, r.Method, r.URL.Path, remoteIP, resp.StatusCode); err != nil {
			return err
		}
		return nil
	}

	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
		_ = s.store.LogShareAccess(context.Background(), share.ID, req.Method, req.URL.Path, clientIP(req), http.StatusBadGateway)
		respondError(rw, http.StatusBadGateway, "gateway proxy failed")
	}

	proxy.ServeHTTP(w, r)
}

func (s *Server) handleShareAuth(w http.ResponseWriter, r *http.Request, share core.Share, passwordHash string) {
	if err := r.ParseForm(); err != nil {
		respondError(w, http.StatusBadRequest, "invalid form payload")
		return
	}

	password := r.Form.Get("password")
	if password == "" {
		respondError(w, http.StatusBadRequest, "password required")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		s.renderPasswordPageWithMessage(w, r, share, "密码错误，请重试")
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

	s.setShareRouteCookie(w, share)

	http.SetCookie(w, &http.Cookie{
		Name:     shareSessionCookieName(),
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

func (s *Server) handleFrontend(w http.ResponseWriter, r *http.Request) {
	if s.tryRedirectToSharePrefixedPath(w, r) {
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
	indexPath := filepath.Join(s.cfg.WebDistDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		respondError(w, http.StatusNotFound, "frontend build not found, run make build")
		return
	}

	w.Header().Set("Cache-Control", "no-cache")

	http.ServeFile(w, r, indexPath)
}

func (s *Server) handleAPIProxy(w http.ResponseWriter, r *http.Request) {
	if s.tryProxyShareAPIRequest(w, r) {
		return
	}

	if s.apiProxy == nil {
		respondError(w, http.StatusBadGateway, "api upstream is unavailable")
		return
	}

	s.apiProxy.ServeHTTP(w, r)
}

func (s *Server) tryProxyShareAPIRequest(w http.ResponseWriter, r *http.Request) bool {
	referer := strings.TrimSpace(r.Header.Get("Referer"))
	if referer == "" {
		return false
	}

	token, ok := shareTokenFromReferer(referer)
	if !ok {
		return false
	}

	share, passwordHash, err := s.store.GetShareByToken(r.Context(), token)
	if err != nil {
		return false
	}

	if share.Status != "active" || time.Now().UTC().After(share.ExpiresAt) {
		return false
	}

	if strings.TrimSpace(passwordHash) != "" {
		sessionCookie, cookieErr := r.Cookie(shareSessionCookieName())
		if cookieErr != nil {
			return false
		}

		valid, validErr := s.store.ValidateShareSession(r.Context(), share.ID, sessionCookie.Value)
		if validErr != nil || !valid {
			return false
		}
	}

	s.proxyShareTarget(w, r, share, r.URL.Path)
	return true
}

func hasFileExtension(path string) bool {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return ext != ""
}

func shareSessionCookieName() string {
	return "nonav_share_session"
}

func shareRouteCookieName() string {
	return "nonav_share_route"
}

func shareSessionCookiePath(shareToken string) string {
	_ = shareToken
	return "/"
}

func (s *Server) setShareRouteCookie(w http.ResponseWriter, share core.Share) {
	expiresAt := time.Now().UTC().Add(8 * time.Hour)
	if share.ExpiresAt.Before(expiresAt) {
		expiresAt = share.ExpiresAt
	}

	http.SetCookie(w, &http.Cookie{
		Name:     shareRouteCookieName(),
		Value:    share.Token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) tryRedirectToSharePrefixedPath(w http.ResponseWriter, r *http.Request) bool {
	if strings.HasPrefix(r.URL.Path, "/s/") || strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" || r.URL.Path == "/healthz" {
		return false
	}

	if s.localFrontendAssetExists(r.URL.Path) {
		return false
	}

	token := resolveShareTokenFromRequest(r)
	if token == "" {
		return false
	}

	targetPath := ensureSharePathPrefix(r.URL.Path, token)
	targetURL := targetPath
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	http.Redirect(w, r, targetURL, http.StatusTemporaryRedirect)
	return true
}

func resolveShareTokenFromRequest(r *http.Request) string {
	referer := strings.TrimSpace(r.Header.Get("Referer"))
	if referer != "" {
		refererToken, ok := shareTokenFromReferer(referer)
		if ok {
			return refererToken
		}
	}

	routeCookie, cookieErr := r.Cookie(shareRouteCookieName())
	if cookieErr == nil {
		return strings.TrimSpace(routeCookie.Value)
	}

	return ""
}

func (s *Server) localFrontendAssetExists(requestPath string) bool {
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

func shareTokenFromReferer(referer string) (string, bool) {
	parsed, err := url.Parse(referer)
	if err != nil {
		return "", false
	}

	token, _, ok := parseSharePath(parsed.Path)
	if !ok {
		return "", false
	}

	return token, true
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

func rewriteShareLocation(rawLocation string, shareToken string, requestHost string) (string, bool) {
	parsed, err := url.Parse(rawLocation)
	if err != nil {
		return "", false
	}

	if parsed.IsAbs() {
		if !strings.EqualFold(parsed.Host, requestHost) {
			return "", false
		}

		parsed.Path = ensureSharePathPrefix(parsed.Path, shareToken)
		return parsed.String(), true
	}

	if strings.HasPrefix(rawLocation, "/") {
		return ensureSharePathPrefix(rawLocation, shareToken), true
	}

	return "", false
}

func ensureSharePathPrefix(path string, shareToken string) string {
	prefix := "/s/" + shareToken
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
