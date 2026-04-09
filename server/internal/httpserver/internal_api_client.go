package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nonav/server/internal/core"
)

type InternalAPIClient struct {
	baseURL string
	http    *http.Client
	service string
	logs    *SystemLogBuffer
}

func NewInternalAPIClient(baseURL string, logs *SystemLogBuffer, service string) *InternalAPIClient {
	return &InternalAPIClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout: 8 * time.Second,
		},
		service: service,
		logs:    logs,
	}
}

type ShareInternalRecord struct {
	Share       core.Share `json:"share"`
	HasPassword bool       `json:"hasPassword"`
}

func (c *InternalAPIClient) GetShareByToken(ctx context.Context, token string) (ShareInternalRecord, error) {
	endpoint := c.baseURL + "/api/internal/shares/token/" + url.PathEscape(token)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	startedAt := time.Now()

	resp, err := c.http.Do(req)
	if err != nil {
		log.Printf("internal_api event=get_share_by_token token=%s status=error latency_ms=%d error=%v", token, time.Since(startedAt).Milliseconds(), err)
		c.record("error", "get_share_by_token", "get share by token failed", "token="+token, fmt.Sprintf("latency_ms=%d", time.Since(startedAt).Milliseconds()), fmt.Sprintf("error=%v", err))
		return ShareInternalRecord{}, fmt.Errorf("internal api get share: %w", err)
	}
	defer resp.Body.Close()
	log.Printf("internal_api event=get_share_by_token token=%s status=%d latency_ms=%d", token, resp.StatusCode, time.Since(startedAt).Milliseconds())
	c.record(statusLevel(resp.StatusCode), "get_share_by_token", "get share by token completed", "token="+token, fmt.Sprintf("status=%d", resp.StatusCode), fmt.Sprintf("latency_ms=%d", time.Since(startedAt).Milliseconds()))

	if resp.StatusCode == http.StatusNotFound {
		return ShareInternalRecord{}, errNotFound
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ShareInternalRecord{}, fmt.Errorf("internal api get share status %d", resp.StatusCode)
	}

	var payload ShareInternalRecord
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ShareInternalRecord{}, fmt.Errorf("decode internal share payload: %w", err)
	}

	return payload, nil
}

func (c *InternalAPIClient) GetShareBySubdomain(ctx context.Context, slug string) (ShareInternalRecord, error) {
	endpoint := c.baseURL + "/api/internal/shares/subdomain/" + url.PathEscape(slug)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	startedAt := time.Now()

	resp, err := c.http.Do(req)
	if err != nil {
		log.Printf("internal_api event=get_share_by_subdomain slug=%s status=error latency_ms=%d error=%v", slug, time.Since(startedAt).Milliseconds(), err)
		c.record("error", "get_share_by_subdomain", "get share by subdomain failed", "slug="+slug, fmt.Sprintf("latency_ms=%d", time.Since(startedAt).Milliseconds()), fmt.Sprintf("error=%v", err))
		return ShareInternalRecord{}, fmt.Errorf("internal api get share by subdomain: %w", err)
	}
	defer resp.Body.Close()
	log.Printf("internal_api event=get_share_by_subdomain slug=%s status=%d latency_ms=%d", slug, resp.StatusCode, time.Since(startedAt).Milliseconds())
	c.record(statusLevel(resp.StatusCode), "get_share_by_subdomain", "get share by subdomain completed", "slug="+slug, fmt.Sprintf("status=%d", resp.StatusCode), fmt.Sprintf("latency_ms=%d", time.Since(startedAt).Milliseconds()))

	if resp.StatusCode == http.StatusNotFound {
		return ShareInternalRecord{}, errNotFound
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ShareInternalRecord{}, fmt.Errorf("internal api get share by subdomain status %d", resp.StatusCode)
	}

	var payload ShareInternalRecord
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ShareInternalRecord{}, fmt.Errorf("decode internal share by subdomain payload: %w", err)
	}

	return payload, nil
}

func (c *InternalAPIClient) AuthorizeShare(ctx context.Context, token string, password string) (string, time.Time, error) {
	body, _ := json.Marshal(map[string]string{"password": password})
	endpoint := c.baseURL + "/api/internal/shares/token/" + url.PathEscape(token) + "/auth"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("internal api authorize share: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", time.Time{}, errUnauthorized
	}

	if resp.StatusCode == http.StatusNotFound {
		return "", time.Time{}, errNotFound
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", time.Time{}, fmt.Errorf("internal api authorize status %d", resp.StatusCode)
	}

	var payload struct {
		SessionToken string    `json:"sessionToken"`
		ExpiresAt    time.Time `json:"expiresAt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", time.Time{}, fmt.Errorf("decode authorize payload: %w", err)
	}

	return payload.SessionToken, payload.ExpiresAt, nil
}

func (c *InternalAPIClient) ValidateShareSession(ctx context.Context, token string, sessionToken string) (bool, error) {
	body, _ := json.Marshal(map[string]string{"sessionToken": sessionToken})
	endpoint := c.baseURL + "/api/internal/shares/token/" + url.PathEscape(token) + "/session"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	startedAt := time.Now()

	resp, err := c.http.Do(req)
	if err != nil {
		log.Printf("internal_api event=validate_session token=%s status=error latency_ms=%d error=%v", token, time.Since(startedAt).Milliseconds(), err)
		c.record("error", "validate_session", "validate share session failed", "token="+token, fmt.Sprintf("latency_ms=%d", time.Since(startedAt).Milliseconds()), fmt.Sprintf("error=%v", err))
		return false, fmt.Errorf("internal api validate session: %w", err)
	}
	defer resp.Body.Close()
	log.Printf("internal_api event=validate_session token=%s status=%d latency_ms=%d", token, resp.StatusCode, time.Since(startedAt).Milliseconds())
	c.record(statusLevel(resp.StatusCode), "validate_session", "validate share session completed", "token="+token, fmt.Sprintf("status=%d", resp.StatusCode), fmt.Sprintf("latency_ms=%d", time.Since(startedAt).Milliseconds()))

	if resp.StatusCode == http.StatusNotFound {
		return false, errNotFound
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("internal api validate session status %d", resp.StatusCode)
	}

	var payload struct {
		Valid bool `json:"valid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false, fmt.Errorf("decode validate session payload: %w", err)
	}

	return payload.Valid, nil
}

func (c *InternalAPIClient) LogShareAccess(ctx context.Context, token string, method string, path string, remoteIP string, statusCode int) {
	body, _ := json.Marshal(map[string]any{
		"method":     method,
		"path":       path,
		"remoteIp":   remoteIP,
		"statusCode": statusCode,
	})

	endpoint := c.baseURL + "/api/internal/shares/token/" + url.PathEscape(token) + "/access-log"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err == nil && resp != nil {
		resp.Body.Close()
	}
}

func (c *InternalAPIClient) GetSystemStatus(ctx context.Context) (core.SystemStatusResponse, error) {
	endpoint := c.baseURL + "/api/internal/system/status"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	startedAt := time.Now()

	resp, err := c.http.Do(req)
	if err != nil {
		c.record("error", "get_system_status", "get internal system status failed", fmt.Sprintf("latency_ms=%d", time.Since(startedAt).Milliseconds()), fmt.Sprintf("error=%v", err))
		return core.SystemStatusResponse{}, fmt.Errorf("internal api get system status: %w", err)
	}
	defer resp.Body.Close()

	c.record(statusLevel(resp.StatusCode), "get_system_status", "get internal system status completed", fmt.Sprintf("status=%d", resp.StatusCode), fmt.Sprintf("latency_ms=%d", time.Since(startedAt).Milliseconds()))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return core.SystemStatusResponse{}, fmt.Errorf("internal api get system status status %d", resp.StatusCode)
	}

	var payload core.SystemStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return core.SystemStatusResponse{}, fmt.Errorf("decode internal system status payload: %w", err)
	}

	return payload, nil
}

func (c *InternalAPIClient) GetSystemLogs(ctx context.Context, level string, limit int) ([]core.SystemLogEntry, error) {
	params := url.Values{}
	params.Set("source", "nonav")
	if strings.TrimSpace(level) != "" {
		params.Set("level", strings.TrimSpace(level))
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}

	endpoint := c.baseURL + "/api/internal/system/logs"
	if encoded := params.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	startedAt := time.Now()

	resp, err := c.http.Do(req)
	if err != nil {
		c.record("error", "get_system_logs", "get internal system logs failed", fmt.Sprintf("latency_ms=%d", time.Since(startedAt).Milliseconds()), fmt.Sprintf("error=%v", err))
		return nil, fmt.Errorf("internal api get system logs: %w", err)
	}
	defer resp.Body.Close()

	c.record(statusLevel(resp.StatusCode), "get_system_logs", "get internal system logs completed", fmt.Sprintf("status=%d", resp.StatusCode), fmt.Sprintf("latency_ms=%d", time.Since(startedAt).Milliseconds()))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("internal api get system logs status %d", resp.StatusCode)
	}

	var payload core.SystemLogsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode internal system logs payload: %w", err)
	}

	return payload.Logs, nil
}

func (c *InternalAPIClient) record(level string, event string, message string, details ...string) {
	if c == nil || c.logs == nil {
		return
	}
	c.logs.Add(c.service, "internal_api", level, event, "-", message, details...)
}

func statusLevel(statusCode int) string {
	if statusCode >= 500 {
		return "error"
	}
	if statusCode >= 400 {
		return "warn"
	}
	return "info"
}
