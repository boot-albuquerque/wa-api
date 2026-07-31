package domain

// HealthResponse represents the health check response
type HealthResponse struct {
	Status            string                 `json:"status"`
	Timestamp         string                 `json:"timestamp"`
	Uptime            string                 `json:"uptime"`
	ActiveConnections int                    `json:"active_connections"`
	TotalUsers        int                    `json:"total_users"`
	ConnectedUsers    int                    `json:"connected_users"`
	LoggedInUsers     int                    `json:"logged_in_users"`
	MemoryStats       map[string]interface{} `json:"memory_stats"`
	GoRoutines        int                    `json:"goroutines"`
	Version           string                 `json:"version,omitempty"`
}
