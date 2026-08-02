// session_test.go — os 8 use cases de ciclo de vida da sessão.
//
// Dois deles, GetQR e GetStatus, deixaram de ser validadores vazios: leem o
// registro persistido via UserRepository.ListUsers e, no caso de GetStatus,
// o estado ao vivo via SessionStatusReader. O teste cobre isso pelo que se
// observa — os campos do resultado e as chamadas gravadas nos fakes —, não
// pelo texto de nenhum erro.
//
// A guarda de sessão dos 7 que a têm propaga a causa desde a F11
// (return err, não mais um fmt.Errorf de texto fixo). errors.Is é a trava.
// Os dois
// sítios que não têm causa a propagar — "nao ha registro de usuario" —
// constroem um *apperr.AppError com Code "no_session", e é o Code que o
// teste assere.
package session_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

var (
	errNoSession = errors.New("porta: sem sessao whatsmeow")
	errDB        = errors.New("repo: banco indisponivel")
)

const txtID = "user-1"

// --- Guarda de sessão ---------------------------------------------------

// guardCase é um use case que abre com EnsureSession. ConnectUseCase é o
// único que não entra nesta tabela — ele não tem guarda, por desenho.
type guardCase struct {
	name string
	run  func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error)
}

func guardCases() []guardCase {
	users := &contractsfake.UserRepository{}
	status := &contractsfake.SessionStatusReader{}
	return []guardCase{
		{"Disconnect", func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error) {
			return session.NewDisconnectUseCase(sg, log).Execute(context.Background(), txtID, domain.DisconnectRequest{})
		}},
		{"Logout", func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error) {
			return session.NewLogoutUseCase(sg, log).Execute(context.Background(), txtID, domain.LogoutRequest{})
		}},
		{"RequestHistorySync", func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error) {
			return session.NewRequestHistorySyncUseCase(sg, log).Execute(context.Background(), txtID, domain.RequestHistorySyncRequest{})
		}},
		{"PairPhone", func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error) {
			return session.NewPairPhoneUseCase(sg, log).Execute(context.Background(), txtID, domain.PairPhoneRequest{Phone: "5511987654321"})
		}},
		{"SetStatusMessage", func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error) {
			return session.NewSetStatusMessageUseCase(sg, log).Execute(context.Background(), txtID, domain.SetStatusMessageRequest{Body: "ola"})
		}},
		{"GetQR", func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error) {
			return session.NewGetQRUseCase(sg, users, log).Execute(context.Background(), txtID)
		}},
		{"GetStatus", func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error) {
			return session.NewGetStatusUseCase(sg, status, users, log).Execute(context.Background(), txtID)
		}},
	}
}

func TestUseCases_SemSessao_PropagamACausa(t *testing.T) {
	for _, tc := range guardCases() {
		t.Run(tc.name, func(t *testing.T) {
			sg := contractsfake.FailSession(errNoSession)
			log := &contractsfake.Logger{}

			_, err := tc.run(&sg, log)

			if !errors.Is(err, errNoSession) {
				t.Fatalf("a causa da porta se perdeu: %v — a traducao fmt.Errorf(\"no session\") voltou?", err)
			}
			if len(sg.EnsureSessionCalls) != 1 || sg.EnsureSessionCalls[0].TxtID != txtID {
				t.Errorf("EnsureSession: %+v", sg.EnsureSessionCalls)
			}

			rec, found := log.FindLevel(contractsfake.LevelError, "no whatsmeow session")
			if !found {
				t.Fatalf("recusa de sessao nao foi logada em nivel error: %v", log.Messages())
			}
			if !rec.IsStructured() {
				t.Errorf("registro nao e' estruturado: %v", rec.Keyvals)
			}
			if v, ok := rec.Keyval("txtID"); !ok || v != txtID {
				t.Errorf(`Keyval("txtID") = %v, %v; quero %q`, v, ok, txtID)
			}
			if v, ok := rec.Keyval("error"); !ok || !errors.Is(v.(error), errNoSession) {
				t.Errorf(`Keyval("error") = %v, %v; quero a causa da porta`, v, ok)
			}
		})
	}
}

func TestUseCases_ComSessao_LogamOSucesso(t *testing.T) {
	for _, tc := range guardCases() {
		t.Run(tc.name, func(t *testing.T) {
			log := &contractsfake.Logger{}
			// GetQR/GetStatus exigem um registro persistido; os demais
			// ignoram o repositório. O fake abaixo serve aos dois grupos.
			sg := &contractsfake.SessionGuard{}
			users := &contractsfake.UserRepository{ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
				return []domain.UserListEntry{{ID: txtID}}, nil
			}}
			status := &contractsfake.SessionStatusReader{}

			var err error
			switch tc.name {
			case "GetQR":
				_, err = session.NewGetQRUseCase(sg, users, log).Execute(context.Background(), txtID)
			case "GetStatus":
				_, err = session.NewGetStatusUseCase(sg, status, users, log).Execute(context.Background(), txtID)
			default:
				_, err = tc.run(sg, log)
			}

			if err != nil {
				t.Fatalf("caminho feliz devolveu erro: %v", err)
			}
			if got := len(log.ByLevel(contractsfake.LevelInfo)); got != 1 {
				t.Errorf("registros info = %d, quero 1: %v", got, log.Messages())
			}
			if got := len(log.ByLevel(contractsfake.LevelError)); got != 0 {
				t.Errorf("caminho feliz logou erro: %v", log.Messages())
			}
		})
	}
}

// --- Connect ------------------------------------------------------------

// Connect não tem porta nenhuma: só registra a intenção e devolve resultado
// vazio, porque o fluxo real (QR + WebSocket) é do handler. O teste trava
// justamente isso — se alguém acrescentar uma guarda aqui sem acrescentar o
// port, o construtor deixa de compilar e este teste é o primeiro a apontar.
func TestConnect_SempreAceitaELoga(t *testing.T) {
	log := &contractsfake.Logger{}

	r, err := session.NewConnectUseCase(log).
		Execute(context.Background(), txtID, domain.ConnectRequest{Subscribe: []string{"Message"}, Immediate: true})

	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if r == nil {
		t.Fatal("resultado nil")
	}
	rec, ok := log.FindLevel(contractsfake.LevelInfo, "connect requested")
	if !ok {
		t.Fatalf("connect nao foi logado: %v", log.Messages())
	}
	if v, ok := rec.Keyval("txtID"); !ok || v != txtID {
		t.Errorf(`Keyval("txtID") = %v, %v`, v, ok)
	}
}

// --- Validações que rodam ANTES da guarda -------------------------------

// PairPhone e SetStatusMessage validam o payload antes de tocar a sessão. A
// ordem importa: um payload vazio numa sessão morta deve ser recusado pelo
// payload, e EnsureSession nem chega a ser chamada.
func TestValidacaoDePayloadPrecedeAGuardaDeSessao(t *testing.T) {
	cases := []struct {
		name string
		run  func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error)
	}{
		{"PairPhone sem Phone", func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error) {
			return session.NewPairPhoneUseCase(sg, log).Execute(context.Background(), txtID, domain.PairPhoneRequest{})
		}},
		{"SetStatusMessage sem Body", func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error) {
			return session.NewSetStatusMessageUseCase(sg, log).Execute(context.Background(), txtID, domain.SetStatusMessageRequest{})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sg := contractsfake.FailSession(errNoSession)
			log := &contractsfake.Logger{}

			_, err := tc.run(&sg, log)

			if err == nil {
				t.Fatal("payload vazio devia ser recusado")
			}
			if errors.Is(err, errNoSession) {
				t.Fatal("a recusa veio da guarda de sessao — a validacao de payload deixou de vir antes")
			}
			if len(sg.EnsureSessionCalls) != 0 {
				t.Error("EnsureSession foi chamada apesar do payload invalido")
			}
			if log.Len() != 0 {
				t.Errorf("recusa por payload nao devia logar: %v", log.Messages())
			}
		})
	}
}

// --- GetQR --------------------------------------------------------------

func TestGetQR(t *testing.T) {
	t.Run("devolve o QR persistido", func(t *testing.T) {
		users := &contractsfake.UserRepository{ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{{ID: txtID, QRCode: "2@abc"}}, nil
		}}
		log := &contractsfake.Logger{}

		r, err := session.NewGetQRUseCase(&contractsfake.SessionGuard{}, users, log).
			Execute(context.Background(), txtID)

		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if r.QRCode != "2@abc" {
			t.Errorf("QRCode = %q, quero %q — o valor persistido nao chegou ao resultado", r.QRCode, "2@abc")
		}
		if len(users.ListUsersCalls) != 1 || users.ListUsersCalls[0].ID != txtID {
			t.Errorf("ListUsers: %+v", users.ListUsersCalls)
		}
		rec, ok := log.FindLevel(contractsfake.LevelInfo, "get QR validated")
		if !ok {
			t.Fatalf("sucesso nao foi logado: %v", log.Messages())
		}
		if v, _ := rec.Keyval("hasQR"); v != true {
			t.Errorf(`Keyval("hasQR") = %v, quero true`, v)
		}
	})

	t.Run("QR ainda vazio nao e' erro", func(t *testing.T) {
		users := &contractsfake.UserRepository{ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{{ID: txtID}}, nil
		}}
		log := &contractsfake.Logger{}

		r, err := session.NewGetQRUseCase(&contractsfake.SessionGuard{}, users, log).
			Execute(context.Background(), txtID)

		if err != nil {
			t.Fatalf("QR vazio nao devia ser erro: %v", err)
		}
		if r.QRCode != "" {
			t.Errorf("QRCode = %q, quero vazio", r.QRCode)
		}
		rec, _ := log.FindLevel(contractsfake.LevelInfo, "get QR validated")
		if v, _ := rec.Keyval("hasQR"); v != false {
			t.Errorf(`Keyval("hasQR") = %v, quero false`, v)
		}
	})

	t.Run("falha do repositorio e' logada e embrulhada com a causa", func(t *testing.T) {
		users := &contractsfake.UserRepository{ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return nil, errDB
		}}
		log := &contractsfake.Logger{}

		r, err := session.NewGetQRUseCase(&contractsfake.SessionGuard{}, users, log).
			Execute(context.Background(), txtID)

		if !errors.Is(err, errDB) {
			t.Fatalf("a causa do repositorio se perdeu: %v", err)
		}
		if r != nil {
			t.Error("resultado devia ser nil")
		}
		if !log.Logged("failed to read QR code") {
			t.Errorf("falha nao foi logada: %v", log.Messages())
		}
	})

	t.Run("sem registro de usuario produz apperr no_session", func(t *testing.T) {
		users := &contractsfake.UserRepository{ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return nil, nil
		}}
		log := &contractsfake.Logger{}

		r, err := session.NewGetQRUseCase(&contractsfake.SessionGuard{}, users, log).
			Execute(context.Background(), txtID)

		assertNoSessionAppErr(t, err)
		if r != nil {
			t.Error("resultado devia ser nil")
		}
		if !log.Logged("no user record for session") {
			t.Errorf("ausencia de registro nao foi logada: %v", log.Messages())
		}
	})
}

// --- GetStatus ----------------------------------------------------------

func TestGetStatus(t *testing.T) {
	entry := domain.UserListEntry{
		ID:          txtID,
		Name:        "Ana",
		JID:         "5511987654321@s.whatsapp.net",
		Webhook:     "https://hook.example",
		Events:      "Message,ReadReceipt",
		ProxyURL:    "http://proxy.example:8080",
		HasProxyURL: true,
		QRCode:      "2@abc",
		S3: domain.S3Config{
			Enabled: true, Endpoint: "https://s3.example", Region: "us-east-1",
			Bucket: "b", PathStyle: true, PublicURL: "https://cdn.example",
			MediaDelivery: "both", RetentionDays: 30,
		},
	}

	t.Run("soma o estado ao vivo ao registro persistido", func(t *testing.T) {
		liveCases := []struct {
			name                string
			connected, loggedIn bool
		}{
			{"desconectado", false, false},
			{"conectado mas nao logado", true, false},
			{"conectado e logado", true, true},
		}
		for _, lc := range liveCases {
			t.Run(lc.name, func(t *testing.T) {
				status := &contractsfake.SessionStatusReader{SessionStatusFunc: func(context.Context, string) (bool, bool) {
					return lc.connected, lc.loggedIn
				}}
				users := &contractsfake.UserRepository{ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
					return []domain.UserListEntry{entry}, nil
				}}
				log := &contractsfake.Logger{}

				r, err := session.NewGetStatusUseCase(&contractsfake.SessionGuard{}, status, users, log).
					Execute(context.Background(), txtID)

				if err != nil {
					t.Fatalf("erro inesperado: %v", err)
				}
				if r.Connected != lc.connected || r.LoggedIn != lc.loggedIn {
					t.Errorf("estado ao vivo = (%v, %v), quero (%v, %v)", r.Connected, r.LoggedIn, lc.connected, lc.loggedIn)
				}
				if len(status.SessionStatusCalls) != 1 || status.SessionStatusCalls[0].UserID != txtID {
					t.Errorf("SessionStatus: %+v", status.SessionStatusCalls)
				}
			})
		}
	})

	t.Run("copia cada campo do registro persistido", func(t *testing.T) {
		users := &contractsfake.UserRepository{ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{entry}, nil
		}}

		r, err := session.NewGetStatusUseCase(&contractsfake.SessionGuard{}, &contractsfake.SessionStatusReader{}, users, &contractsfake.Logger{}).
			Execute(context.Background(), txtID)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}

		escalares := []struct {
			campo     string
			got, want string
		}{
			{"ID", r.ID, entry.ID},
			{"Name", r.Name, entry.Name},
			{"Jid", r.Jid, entry.JID},
			{"Webhook", r.Webhook, entry.Webhook},
			{"Events", r.Events, entry.Events},
			{"ProxyURL", r.ProxyURL, entry.ProxyURL},
			{"Qrcode", r.Qrcode, entry.QRCode},
			{"History", r.History, "0"},
		}
		for _, c := range escalares {
			if c.got != c.want {
				t.Errorf("%s = %q, quero %q", c.campo, c.got, c.want)
			}
		}

		wantProxy := map[string]any{"enabled": entry.HasProxyURL, "proxyUrl": entry.ProxyURL}
		for k, want := range wantProxy {
			if got := r.ProxyConfig[k]; got != want {
				t.Errorf("ProxyConfig[%q] = %v, quero %v", k, got, want)
			}
		}

		wantS3 := map[string]any{
			"enabled":        entry.S3.Enabled,
			"endpoint":       entry.S3.Endpoint,
			"region":         entry.S3.Region,
			"bucket":         entry.S3.Bucket,
			"path_style":     entry.S3.PathStyle,
			"public_url":     entry.S3.PublicURL,
			"media_delivery": entry.S3.MediaDelivery,
			"retention_days": entry.S3.RetentionDays,
		}
		for k, want := range wantS3 {
			if got := r.S3Config[k]; got != want {
				t.Errorf("S3Config[%q] = %v, quero %v", k, got, want)
			}
		}
		if len(r.S3Config) != len(wantS3) {
			t.Errorf("S3Config tem %d chaves, quero %d", len(r.S3Config), len(wantS3))
		}
	})

	// A ordem é deliberada: o estado ao vivo é lido ANTES do registro, e uma
	// falha do repositório aborta mesmo com o SDK respondendo.
	t.Run("falha do repositorio e' logada e embrulhada com a causa", func(t *testing.T) {
		status := &contractsfake.SessionStatusReader{}
		users := &contractsfake.UserRepository{ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return nil, errDB
		}}
		log := &contractsfake.Logger{}

		r, err := session.NewGetStatusUseCase(&contractsfake.SessionGuard{}, status, users, log).
			Execute(context.Background(), txtID)

		if !errors.Is(err, errDB) {
			t.Fatalf("a causa do repositorio se perdeu: %v", err)
		}
		if r != nil {
			t.Error("resultado devia ser nil")
		}
		if len(status.SessionStatusCalls) != 1 {
			t.Error("o estado ao vivo devia ter sido lido antes do registro")
		}
		if !log.Logged("failed to read session record") {
			t.Errorf("falha nao foi logada: %v", log.Messages())
		}
	})

	t.Run("sem registro de usuario produz apperr no_session", func(t *testing.T) {
		users := &contractsfake.UserRepository{ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{}, nil
		}}
		log := &contractsfake.Logger{}

		r, err := session.NewGetStatusUseCase(&contractsfake.SessionGuard{}, &contractsfake.SessionStatusReader{}, users, log).
			Execute(context.Background(), txtID)

		assertNoSessionAppErr(t, err)
		if r != nil {
			t.Error("resultado devia ser nil")
		}
		if !log.Logged("no user record for session") {
			t.Errorf("ausencia de registro nao foi logada: %v", log.Messages())
		}
	})
}

// assertNoSessionAppErr trava o Code e a Category do erro tipado — não o
// texto. É o que a fronteira HTTP usa para decidir o status.
func assertNoSessionAppErr(t *testing.T, err error) {
	t.Helper()
	var ae *apperr.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("erro nao e' um *apperr.AppError: %v", err)
	}
	if ae.Code != "no_session" {
		t.Errorf("Code = %q, quero %q", ae.Code, "no_session")
	}
	if ae.Category != apperr.CategoryValidation {
		t.Errorf("Category = %q, quero %q", ae.Category, apperr.CategoryValidation)
	}
	if ae.Retryable {
		t.Error("Retryable = true; sessao ausente nao melhora com retry do mesmo pedido")
	}
}
