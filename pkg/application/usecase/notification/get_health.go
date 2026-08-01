package notification

import (
	"context"
	"database/sql"
	"fmt"
	"runtime"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetHealthUseCase retrieves health information about the server
type GetHealthUseCase struct {
	db             *sql.DB
	sessionCounter appport.SessionCounter
	logger         appport.Logger
	startTime      time.Time
	version        string
}

// NewGetHealthUseCase creates a new instance
func NewGetHealthUseCase(db *sql.DB, sc appport.SessionCounter, logger appport.Logger, version string) *GetHealthUseCase {
	return &GetHealthUseCase{
		db:             db,
		sessionCounter: sc,
		logger:         logger,
		startTime:      time.Now(),
		version:        version,
	}
}

// Execute retrieves health information
func (uc *GetHealthUseCase) Execute(ctx context.Context) (*domain.HealthResponse, error) {
	uptime := time.Since(uc.startTime)

	var totalUsers int
	rows, err := uc.db.QueryContext(ctx, "SELECT COUNT(*) FROM users")
	if err == nil {
		defer func() { _ = rows.Close() }()
		if rows.Next() {
			_ = rows.Scan(&totalUsers)
		}
	}

	counts, err := uc.sessionCounter.CountSessions(ctx)
	if err != nil {
		uc.logger.Error(ctx, "failed to count sessions", "error", err)
		return nil, fmt.Errorf("failed to count sessions: %w", err)
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	memoryStats := map[string]interface{}{
		"alloc_mb":       memStats.Alloc / 1024 / 1024,
		"total_alloc_mb": memStats.TotalAlloc / 1024 / 1024,
		"sys_mb":         memStats.Sys / 1024 / 1024,
		"num_gc":         memStats.NumGC,
	}

	response := &domain.HealthResponse{
		Status:            "ok",
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
		Uptime:            uptime.String(),
		ActiveConnections: counts.Total,
		TotalUsers:        totalUsers,
		ConnectedUsers:    counts.Connected,
		LoggedInUsers:     counts.LoggedIn,
		MemoryStats:       memoryStats,
		GoRoutines:        runtime.NumGoroutine(),
		Version:           uc.version,
	}

	return response, nil
}
