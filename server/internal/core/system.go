package core

import "time"

type SystemServiceStatus struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Health  string `json:"health"`
	Summary string `json:"summary"`
}

type SystemStatusResponse struct {
	OverallHealth string                `json:"overallHealth"`
	CheckedAt     time.Time             `json:"checkedAt"`
	Services      []SystemServiceStatus `json:"services"`
}

type SystemLogEntry struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`
	Component string    `json:"component"`
	Level     string    `json:"level"`
	Event     string    `json:"event"`
	Req       string    `json:"req"`
	Message   string    `json:"message"`
	Details   []string  `json:"details"`
}

type SystemLogsResponse struct {
	Logs []SystemLogEntry `json:"logs"`
}
