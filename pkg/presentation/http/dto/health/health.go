// Package health holds the PUBLIC wire types for GET /health, plus the
// hand-written presenter that builds them from domain values.
//
// See docs/HTTP-DTO-CONVENTIONS.md.
package health

import "wa-api/pkg/domain"

// MemoryStatsResponse is the runtime memory summary served by /health.
//
// It replaces a map[string]interface{} built in the use case. The values are
// runtime.MemStats counters already divided down to megabytes there, so the
// units live in the NAMES and not in a comment nobody reads at 3am.
type MemoryStatsResponse struct {
	AllocMB      uint64 `json:"alloc_mb"`
	TotalAllocMB uint64 `json:"total_alloc_mb"`
	SysMB        uint64 `json:"sys_mb"`
	NumGC        uint32 `json:"num_gc"`
}

// HealthResponse is the body of `data` for GET /health.
//
// `version` lost its omitempty: a build with no version stamped in has to
// answer "" rather than drop the key, or a deploy check cannot tell an
// unstamped binary from an old one that never had the field.
type HealthResponse struct {
	Status            string              `json:"status"`
	Timestamp         string              `json:"timestamp"`
	Uptime            string              `json:"uptime"`
	ActiveConnections int                 `json:"active_connections"`
	TotalUsers        int                 `json:"total_users"`
	ConnectedUsers    int                 `json:"connected_users"`
	LoggedInUsers     int                 `json:"logged_in_users"`
	MemoryStats       MemoryStatsResponse `json:"memory_stats"`
	GoRoutines        int                 `json:"goroutines"`
	Version           string              `json:"version"`
}

// PresentMemoryStats maps the memory summary.
func PresentMemoryStats(m domain.MemoryStats) MemoryStatsResponse {
	return MemoryStatsResponse{
		AllocMB:      m.AllocMB,
		TotalAllocMB: m.TotalAllocMB,
		SysMB:        m.SysMB,
		NumGC:        m.NumGC,
	}
}

// PresentHealth maps the health snapshot. A nil input presents as the zero
// response, so the caller does not have to branch before calling.
func PresentHealth(h *domain.HealthResponse) HealthResponse {
	if h == nil {
		return HealthResponse{}
	}
	return HealthResponse{
		Status:            h.Status,
		Timestamp:         h.Timestamp,
		Uptime:            h.Uptime,
		ActiveConnections: h.ActiveConnections,
		TotalUsers:        h.TotalUsers,
		ConnectedUsers:    h.ConnectedUsers,
		LoggedInUsers:     h.LoggedInUsers,
		MemoryStats:       PresentMemoryStats(h.MemoryStats),
		GoRoutines:        h.GoRoutines,
		Version:           h.Version,
	}
}
