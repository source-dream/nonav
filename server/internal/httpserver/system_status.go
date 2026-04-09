package httpserver

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"nonav/server/internal/core"
)

func (s *Server) recordSystemLog(component string, level string, event string, req string, message string, details ...string) {
	if s == nil || s.systemLogs == nil {
		return
	}
	s.systemLogs.Add(s.serviceName, component, level, event, req, message, details...)
}

func formatLogFields(fields map[string]any) []string {
	if len(fields) == 0 {
		return nil
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	details := make([]string, 0, len(keys))
	for _, key := range keys {
		details = append(details, fmt.Sprintf("%s=%v", key, fields[key]))
	}
	return details
}

func (s *Server) handleInternalSystemStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondMethodNotAllowed(w)
		return
	}

	respondJSON(w, http.StatusOK, s.buildAPISystemStatus())
}

func (s *Server) handleInternalSystemLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondMethodNotAllowed(w)
		return
	}

	source, level, limit := parseSystemLogQuery(r)
	respondJSON(w, http.StatusOK, core.SystemLogsResponse{Logs: s.systemLogs.List(source, level, limit)})
}

func (s *Server) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	if s.tryHandleSubdomainShareRequest(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		respondMethodNotAllowed(w)
		return
	}

	services := []core.SystemServiceStatus{
		buildGatewayServiceStatus(),
		buildFRPSServiceStatus(s.frpsManager.Snapshot()),
	}

	if s.apiClient == nil {
		services = append(services,
			core.SystemServiceStatus{Key: "nonav", Label: "导航状态", Health: "offline", Summary: "nonav 服务不可达"},
			core.SystemServiceStatus{Key: "frpc", Label: "frpc 状态", Health: "offline", Summary: "无法获取 frpc 状态"},
		)
	} else {
		payload, err := s.apiClient.GetSystemStatus(r.Context())
		if err != nil {
			s.recordSystemLog("core", "error", "system_status_upstream_failed", requestIDFromContext(r.Context()), "failed to fetch nonav system status", "error="+err.Error())
			services = append(services,
				core.SystemServiceStatus{Key: "nonav", Label: "导航状态", Health: "offline", Summary: "nonav 服务不可达"},
				core.SystemServiceStatus{Key: "frpc", Label: "frpc 状态", Health: "offline", Summary: "无法获取 frpc 状态"},
			)
		} else {
			services = append(services, payload.Services...)
		}
	}

	respondJSON(w, http.StatusOK, core.SystemStatusResponse{
		OverallHealth: overallSystemHealth(services),
		CheckedAt:     time.Now().UTC(),
		Services:      services,
	})
}

func (s *Server) handleSystemLogs(w http.ResponseWriter, r *http.Request) {
	if s.tryHandleSubdomainShareRequest(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		respondMethodNotAllowed(w)
		return
	}

	source, level, limit := parseSystemLogQuery(r)
	logs := make([]core.SystemLogEntry, 0, limit*2)

	if source == "" || source == "all" || source == s.serviceName {
		logs = append(logs, s.systemLogs.List(s.serviceName, level, limit)...)
	}

	if source == "" || source == "all" || source == "nonav" {
		if s.apiClient != nil {
			remoteLogs, err := s.apiClient.GetSystemLogs(r.Context(), level, limit)
			if err != nil {
				logs = append(logs, core.SystemLogEntry{
					ID:        0,
					Timestamp: time.Now().UTC(),
					Source:    s.serviceName,
					Component: "core",
					Level:     "error",
					Event:     "system_logs_upstream_failed",
					Req:       requestIDFromContext(r.Context()),
					Message:   "failed to fetch nonav logs",
					Details:   []string{"error=" + err.Error()},
				})
			} else {
				logs = append(logs, remoteLogs...)
			}
		}
	}

	sort.Slice(logs, func(i int, j int) bool {
		return logs[i].Timestamp.After(logs[j].Timestamp)
	})
	if len(logs) > limit {
		logs = logs[:limit]
	}

	respondJSON(w, http.StatusOK, core.SystemLogsResponse{Logs: logs})
}

func (s *Server) buildAPISystemStatus() core.SystemStatusResponse {
	services := []core.SystemServiceStatus{
		buildNonavServiceStatus(),
		buildFRPCServiceStatus(s.frpManager.Snapshot()),
	}

	return core.SystemStatusResponse{
		OverallHealth: overallSystemHealth(services),
		CheckedAt:     time.Now().UTC(),
		Services:      services,
	}
}

func buildGatewayServiceStatus() core.SystemServiceStatus {
	return core.SystemServiceStatus{
		Key:     "gateway",
		Label:   "网关状态",
		Health:  "online",
		Summary: "nonav-gateway 运行正常",
	}
}

func buildNonavServiceStatus() core.SystemServiceStatus {
	return core.SystemServiceStatus{
		Key:     "nonav",
		Label:   "导航状态",
		Health:  "online",
		Summary: "nonav 服务可访问",
	}
}

func buildFRPCServiceStatus(snapshot FRPProcessSnapshot) core.SystemServiceStatus {
	if !snapshot.Enabled {
		return core.SystemServiceStatus{Key: "frpc", Label: "frpc 状态", Health: "online", Summary: "未启用 FRP"}
	}
	if snapshot.DesiredCount == 0 {
		return core.SystemServiceStatus{Key: "frpc", Label: "frpc 状态", Health: "online", Summary: "当前没有活动代理"}
	}
	if snapshot.RestartingCount > 0 {
		return core.SystemServiceStatus{
			Key:     "frpc",
			Label:   "frpc 状态",
			Health:  "degraded",
			Summary: fmt.Sprintf("%d/%d 个代理运行中，%d 个重连中", snapshot.RunningCount, snapshot.DesiredCount, snapshot.RestartingCount),
		}
	}
	if snapshot.RunningCount == 0 {
		return core.SystemServiceStatus{Key: "frpc", Label: "frpc 状态", Health: "offline", Summary: "没有正在运行的代理"}
	}
	return core.SystemServiceStatus{
		Key:     "frpc",
		Label:   "frpc 状态",
		Health:  "online",
		Summary: fmt.Sprintf("%d 个代理运行中", snapshot.RunningCount),
	}
}

func buildFRPSServiceStatus(snapshot FRPServerSnapshot) core.SystemServiceStatus {
	if !snapshot.Enabled {
		return core.SystemServiceStatus{Key: "frps", Label: "frps 状态", Health: "online", Summary: "由外部进程提供"}
	}
	if snapshot.Running {
		if snapshot.Restarts > 0 {
			return core.SystemServiceStatus{Key: "frps", Label: "frps 状态", Health: "degraded", Summary: fmt.Sprintf("frps 已重启 %d 次，当前运行中", snapshot.Restarts)}
		}
		return core.SystemServiceStatus{Key: "frps", Label: "frps 状态", Health: "online", Summary: "frps 已启动"}
	}
	if snapshot.Desired {
		return core.SystemServiceStatus{Key: "frps", Label: "frps 状态", Health: "offline", Summary: "frps 未运行"}
	}
	return core.SystemServiceStatus{Key: "frps", Label: "frps 状态", Health: "offline", Summary: "frps 未启动"}
}

func overallSystemHealth(services []core.SystemServiceStatus) string {
	worst := "online"
	for _, service := range services {
		if healthPriority(service.Health) > healthPriority(worst) {
			worst = service.Health
		}
	}
	return worst
}

func healthPriority(health string) int {
	switch strings.ToLower(strings.TrimSpace(health)) {
	case "offline":
		return 3
	case "degraded":
		return 2
	default:
		return 1
	}
}

func parseSystemLogQuery(r *http.Request) (string, string, int) {
	query := r.URL.Query()
	source := strings.TrimSpace(query.Get("source"))
	level := strings.TrimSpace(query.Get("level"))
	limit := 100
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 500 {
		limit = 500
	}
	if level == "" {
		level = "all"
	}
	return source, level, limit
}
