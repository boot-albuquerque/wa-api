package wuzapi

import "wuzapi/internal/infrastructure/db"

// Types delegated to internal/infrastructure/db
type DatabaseConfig = db.DatabaseConfig
type HistoryMessage = db.HistoryMessage

// Functions delegated to internal/infrastructure/db
var (
	InitializeDatabase   = db.InitializeDatabase
	getDatabaseConfig    = db.GetDatabaseConfig
	saveMessageToHistory = db.SaveMessageToHistory
	trimMessageHistory   = db.TrimMessageHistory
	setDisconnectedState = db.SetDisconnectedState
)
