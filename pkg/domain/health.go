package domain

// MemoryStats agrega os contadores de runtime.MemStats que o health check
// reporta, já convertidos para megabytes.
//
// Era um map[string]interface{} montado no use case, o que fazia dele um
// formato de fio que nenhuma etiqueta `json` descrevia — e portanto invisível
// a qualquer auditoria de contrato.
type MemoryStats struct {
	AllocMB      uint64
	TotalAllocMB uint64
	SysMB        uint64
	NumGC        uint32
}

// HealthResponse represents the health check response.
//
// Já NÃO é o formato de fio: quem serve /health passa por
// pkg/presentation/http/dto/health (docs/HTTP-DTO-CONVENTIONS.md).
type HealthResponse struct {
	Status            string
	Timestamp         string
	Uptime            string
	ActiveConnections int
	TotalUsers        int
	ConnectedUsers    int
	LoggedInUsers     int
	MemoryStats       MemoryStats
	GoRoutines        int
	Version           string
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
