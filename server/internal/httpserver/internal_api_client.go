package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nonav/server/internal/core"
)

type InternalAPIClient struct {
	baseURL string
	http    *http.Client
}

func NewInternalAPIClient(baseURL string) *InternalAPIClient {
	return &InternalAPIClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

type ShareInternalRecord struct {
	Share       core.Share `json:"share"`
	HasPassword bool       `json:"hasPassword"`
}

func (c *InternalAPIClient) GetShareByToken(ctx context.Context, token string) (ShareInternalRecord, error) {
	endpoint := c.baseURL + "/api/internal/shares/token/" + url.PathEscape(token)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)

	resp, err := c.http.Do(req)
	if err != nil {
		return ShareInternalRecord{}, fmt.Errorf("internal api get share: %w", err)
	}
	defer resp.Body.Close()

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

	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("internal api validate session: %w", err)
	}
	defer resp.Body.Close()

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
