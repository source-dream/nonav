package httpserver

import (
	"strings"
	"sync"
	"time"

	"nonav/server/internal/core"
)

type SystemLogBuffer struct {
	mu      sync.Mutex
	nextID  int64
	maxSize int
	entries []core.SystemLogEntry
}

func NewSystemLogBuffer(maxSize int) *SystemLogBuffer {
	if maxSize <= 0 {
		maxSize = 600
	}

	return &SystemLogBuffer{maxSize: maxSize}
}

func (b *SystemLogBuffer) Add(source string, component string, level string, event string, req string, message string, details ...string) {
	if b == nil {
		return
	}

	entry := core.SystemLogEntry{
		Timestamp: time.Now().UTC(),
		Source:    strings.TrimSpace(source),
		Component: strings.TrimSpace(component),
		Level:     normalizeLogLevel(level),
		Event:     strings.TrimSpace(event),
		Req:       strings.TrimSpace(req),
		Message:   strings.TrimSpace(message),
		Details:   cloneNonEmptyStrings(details),
	}

	b.mu.Lock()
	b.nextID++
	entry.ID = b.nextID
	b.entries = append(b.entries, entry)
	if len(b.entries) > b.maxSize {
		b.entries = append([]core.SystemLogEntry(nil), b.entries[len(b.entries)-b.maxSize:]...)
	}
	b.mu.Unlock()
}

func (b *SystemLogBuffer) List(source string, level string, limit int) []core.SystemLogEntry {
	if b == nil {
		return nil
	}

	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	source = strings.TrimSpace(source)
	level = normalizeLogLevel(level)

	b.mu.Lock()
	defer b.mu.Unlock()

	items := make([]core.SystemLogEntry, 0, limit)
	for idx := len(b.entries) - 1; idx >= 0; idx-- {
		entry := b.entries[idx]
		if source != "" && source != "all" && entry.Source != source {
			continue
		}
		if level != "all" && level != "" && entry.Level != level {
			continue
		}

		items = append(items, cloneSystemLogEntry(entry))
		if len(items) >= limit {
			break
		}
	}

	return items
}

func normalizeLogLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "error":
		return "error"
	case "warn", "warning":
		return "warn"
	case "all":
		return "all"
	default:
		return "info"
	}
}

func cloneSystemLogEntry(entry core.SystemLogEntry) core.SystemLogEntry {
	entry.Details = cloneNonEmptyStrings(entry.Details)
	return entry
}

func cloneNonEmptyStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}

	cloned := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		cloned = append(cloned, trimmed)
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}
