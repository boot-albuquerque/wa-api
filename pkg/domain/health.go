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

// SessionCounts agrega as contagens de sessões WhatsApp usadas pelo health
// check. Expressa em tipos de domínio para que o use case não precise
// conhecer o tipo concreto do cliente do SDK (ADR-001).
type SessionCounts struct {
	// Total é o número de sessões registradas no gerenciador de clientes,
	// conectadas ou não.
	Total int
	// Connected é o número de sessões com transporte ativo.
	Connected int
	// LoggedIn é o número de sessões autenticadas no WhatsApp.
	LoggedIn int
}
