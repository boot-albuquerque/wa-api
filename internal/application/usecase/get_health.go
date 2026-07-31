package usecase

import (
	"context"
	"database/sql"
	"runtime"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	appport "disparazap/internal/contracts"
	"disparazap/internal/shared/domain"
)

// GetHealthUseCase retrieves health information about the server
type GetHealthUseCase struct {
	db                 *sql.DB
	healthProvider     appport.HealthClientProvider
	logger             zerolog.Logger
	startTime          time.Time
	version            string
}

// NewGetHealthUseCase creates a new instance
func NewGetHealthUseCase(db *sql.DB, hp appport.HealthClientProvider, logger zerolog.Logger, version string) *GetHealthUseCase {
	return &GetHealthUseCase{
		db:             db,
		healthProvider: hp,
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

	activeConnections := uc.healthProvider.GetWhatsmeowClientsCount()
	connectedUsers := 0
	loggedInUsers := 0

	uc.healthProvider.IterateWhatsmeowClients(func(client *whatsmeow.Client) bool {
		if client != nil {
			if client.IsConnected() {
				connectedUsers++
			}
			if client.IsLoggedIn() {
				loggedInUsers++
			}
		}
		return true
	})

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
		ActiveConnections: activeConnections,
		TotalUsers:        totalUsers,
		ConnectedUsers:    connectedUsers,
		LoggedInUsers:     loggedInUsers,
		MemoryStats:       memoryStats,
		GoRoutines:        runtime.NumGoroutine(),
		Version:           uc.version,
	}

	return response, nil
}
