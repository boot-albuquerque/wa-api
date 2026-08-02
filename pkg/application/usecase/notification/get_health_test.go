package notification_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/notification"
	"wa-api/pkg/domain"

	_ "modernc.org/sqlite"
)

// openDB abre um SQLite em arquivo (não :memory:) porque o pool do
// database/sql pode abrir mais de uma conexão, e cada conexão para :memory:
// enxerga um banco diferente — o mesmo motivo documentado em
// pkg/infra/db/migrations_test.go.
func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "health.db"))
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("fechar db: %v", err)
		}
	})
	return db
}

// seedUsers cria a tabela users e insere n linhas.
func seedUsers(t *testing.T, db *sql.DB, n int) {
	t.Helper()
	if _, err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("criar tabela users: %v", err)
	}
	for i := 0; i < n; i++ {
		if _, err := db.Exec("INSERT INTO users (id) VALUES (?)", i+1); err != nil {
			t.Fatalf("inserir user %d: %v", i, err)
		}
	}
}

func TestGetHealthExecute(t *testing.T) {
	errCount := errors.New("gerenciador de clientes indisponível")

	tests := []struct {
		name string
		// setupDB prepara o banco; quando nil, o banco fica sem a tabela
		// users e o SELECT COUNT(*) falha.
		setupDB  func(t *testing.T, db *sql.DB)
		counts   domain.SessionCounts
		countErr error

		wantErr        error
		wantTotalUsers int
		wantActive     int
		wantConnected  int
		wantLoggedIn   int
		// wantErrorLog é a mensagem de log de erro esperada; vazio significa
		// que nenhum log de erro deve ter sido emitido.
		wantErrorLog string
	}{
		{
			name:    "tabela users ausente nao aborta o health check",
			setupDB: nil,
			counts:  domain.SessionCounts{Total: 2, Connected: 1, LoggedIn: 1},
			// O erro de query é deliberadamente ignorado pelo use case: o
			// health check ainda responde, só que com total_users zerado.
			wantTotalUsers: 0,
			wantActive:     2,
			wantConnected:  1,
			wantLoggedIn:   1,
		},
		{
			name:           "tabela users vazia conta zero",
			setupDB:        func(t *testing.T, db *sql.DB) { seedUsers(t, db, 0) },
			counts:         domain.SessionCounts{},
			wantTotalUsers: 0,
		},
		{
			name:           "tabela users populada conta as linhas",
			setupDB:        func(t *testing.T, db *sql.DB) { seedUsers(t, db, 3) },
			counts:         domain.SessionCounts{Total: 5, Connected: 4, LoggedIn: 2},
			wantTotalUsers: 3,
			wantActive:     5,
			wantConnected:  4,
			wantLoggedIn:   2,
		},
		{
			name:         "falha ao contar sessoes aborta e loga",
			setupDB:      func(t *testing.T, db *sql.DB) { seedUsers(t, db, 1) },
			countErr:     errCount,
			wantErr:      errCount,
			wantErrorLog: "failed to count sessions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openDB(t)
			if tt.setupDB != nil {
				tt.setupDB(t, db)
			}

			counter := &contractsfake.SessionCounter{
				CountSessionsFunc: func(context.Context) (domain.SessionCounts, error) {
					return tt.counts, tt.countErr
				},
			}
			logger := &contractsfake.Logger{}
			uc := notification.NewGetHealthUseCase(db, counter, logger, "v9.9.9")

			got, err := uc.Execute(context.Background())

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("erro = %v, queria envolver %v", err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("resposta = %+v, queria nil no caminho de erro", got)
				}
			} else {
				if err != nil {
					t.Fatalf("erro inesperado: %v", err)
				}
				if got == nil {
					t.Fatal("resposta nil sem erro")
				}
				if got.Status != "ok" {
					t.Errorf("status = %q, queria \"ok\"", got.Status)
				}
				if got.Version != "v9.9.9" {
					t.Errorf("version = %q, queria a versao injetada no construtor", got.Version)
				}
				if got.TotalUsers != tt.wantTotalUsers {
					t.Errorf("total_users = %d, queria %d", got.TotalUsers, tt.wantTotalUsers)
				}
				if got.ActiveConnections != tt.wantActive {
					t.Errorf("active_connections = %d, queria %d", got.ActiveConnections, tt.wantActive)
				}
				if got.ConnectedUsers != tt.wantConnected {
					t.Errorf("connected_users = %d, queria %d", got.ConnectedUsers, tt.wantConnected)
				}
				if got.LoggedInUsers != tt.wantLoggedIn {
					t.Errorf("logged_in_users = %d, queria %d", got.LoggedInUsers, tt.wantLoggedIn)
				}
				if got.Timestamp == "" {
					t.Error("timestamp vazio")
				}
				if got.Uptime == "" {
					t.Error("uptime vazio")
				}
				if got.GoRoutines <= 0 {
					t.Errorf("goroutines = %d, queria > 0", got.GoRoutines)
				}
				for _, key := range []string{"alloc_mb", "total_alloc_mb", "sys_mb", "num_gc"} {
					if _, ok := got.MemoryStats[key]; !ok {
						t.Errorf("memory_stats sem a chave %q: %v", key, got.MemoryStats)
					}
				}
			}

			if len(counter.CountSessionsCalls) != 1 {
				t.Errorf("CountSessions chamado %d vezes, queria 1", len(counter.CountSessionsCalls))
			}

			errLogs := logger.ByLevel("error")
			if tt.wantErrorLog == "" {
				if len(errLogs) != 0 {
					t.Errorf("logs de erro inesperados: %v", logger.Messages())
				}
				return
			}
			rec, ok := logger.FindLevel("error", tt.wantErrorLog)
			if !ok {
				t.Fatalf("faltou log de erro %q; registros: %v", tt.wantErrorLog, logger.Messages())
			}
			if !rec.HasKey("error") {
				t.Errorf("log %q sem a keyval \"error\" — a causa se perde", tt.wantErrorLog)
			}
			if !rec.IsStructured() {
				t.Errorf("log %q com keyvals desbalanceadas", tt.wantErrorLog)
			}
		})
	}
}

func TestGetHealthNewUseCaseUsaOConstrutor(t *testing.T) {
	db := openDB(t)
	seedUsers(t, db, 0)
	uc := notification.NewGetHealthUseCase(db, &contractsfake.SessionCounter{}, &contractsfake.Logger{}, "")
	if uc == nil {
		t.Fatal("construtor devolveu nil")
	}
	got, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	// Version tem `omitempty`: a string vazia do construtor tem de continuar
	// vazia na resposta, e nao virar um placeholder.
	if got.Version != "" {
		t.Errorf("version = %q, queria vazia", got.Version)
	}
}
