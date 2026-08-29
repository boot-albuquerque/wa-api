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
	"fmt"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

var (
	errNoSession = errors.New("porta: sem sessao noise")
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

// ctlDe adapta um SessionGuard fake a port.SessionController — a porta que
// Disconnect e Logout passaram a exigir na F79, por precisarem AGIR sobre a
// sessão e não só verificá-la.
//
// EnsureSession delega ao ponteiro ORIGINAL de propósito: copiar o struct
// para dentro do controller registraria as chamadas na cópia, e a asserção
// sobre sg.EnsureSessionCalls passaria a medir nada — falha que o teste não
// acusaria, porque continuaria compilando e passando.
func ctlDe(sg *contractsfake.SessionGuard) *contractsfake.SessionController {
	return &contractsfake.SessionController{
		SessionGuard: contractsfake.SessionGuard{EnsureSessionFunc: sg.EnsureSession},
	}
}

// pairerDe segue o mesmo idioma de ctlDe: delega EnsureSession ao guard
// ORIGINAL, para que as chamadas continuem sendo registradas nele. Entrou com
// o CAP-26, quando PairPhoneUseCase passou a consumir port.PhonePairer.
func qrDe(sg *contractsfake.SessionGuard, users *contractsfake.UserRepository) *contractsfake.PairingQRReader {
	return &contractsfake.PairingQRReader{
		SessionGuard: contractsfake.SessionGuard{EnsureSessionFunc: sg.EnsureSession},
		PairingQRFunc: func(ctx context.Context, txtID string) (string, error) {
			entries, err := users.ListUsers(ctx, txtID)
			if err != nil {
				return "", fmt.Errorf("database error: %w", err)
			}
			if len(entries) == 0 {
				return "", apperr.New("no_session", apperr.CategoryValidation, "no session", false, nil)
			}
			return entries[0].QRCode, nil
		},
	}
}

// pairerDe segue o mesmo idioma de ctlDe.
func pairerDe(sg *contractsfake.SessionGuard) *contractsfake.PhonePairer {
	return &contractsfake.PhonePairer{
		SessionGuard: contractsfake.SessionGuard{EnsureSessionFunc: sg.EnsureSession},
	}
}

// statusDe embrulha o guarda num StatusMessageSetter, como pairerDe faz para
// o pareamento. O use case passou a consumir a porta nova com a F198.
func histDe(sg *contractsfake.SessionGuard) *contractsfake.HistorySyncRequester {
	return &contractsfake.HistorySyncRequester{
		SessionGuard: contractsfake.SessionGuard{EnsureSessionFunc: sg.EnsureSession},
	}
}

func statusDe(sg *contractsfake.SessionGuard) *contractsfake.StatusMessageSetter {
	return &contractsfake.StatusMessageSetter{
		SessionGuard: contractsfake.SessionGuard{EnsureSessionFunc: sg.EnsureSession},
	}
}

func guardCases() []guardCase {
	users := &contractsfake.UserRepository{}
	return []guardCase{
		{"Disconnect", func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error) {
			return session.NewDisconnectUseCase(ctlDe(sg), log).Execute(context.Background(), txtID, domain.DisconnectRequest{})
		}},
		{"Logout", func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error) {
			return session.NewLogoutUseCase(ctlDe(sg), &contractsfake.SessionDetacher{}, log).Execute(context.Background(), txtID, domain.LogoutRequest{})
		}},
		{"RequestHistorySync", func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error) {
			return session.NewRequestHistorySyncUseCase(histDe(sg), log).Execute(context.Background(), txtID, domain.RequestHistorySyncRequest{ChatJid: "c@s.whatsapp.net", OldestMsgID: "M1"})
		}},
		{"PairPhone", func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error) {
			return session.NewPairPhoneUseCase(pairerDe(sg), log).
				Execute(context.Background(), txtID, domain.PairPhoneRequest{Phone: "5511987654321"})
		}},
		{"SetStatusMessage", func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error) {
			return session.NewSetStatusMessageUseCase(statusDe(sg), log).Execute(context.Background(), txtID, domain.SetStatusMessageRequest{Body: "ola"})
		}},
		{"GetQR", func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error) {
			return session.NewGetQRUseCase(qrDe(sg, users), log).Execute(context.Background(), txtID)
		}},
		// GetStatus NÃO entra: desde a F196 ele não consulta o SessionGuard, e
		// é essa a correção. Ver TestGetStatus_DesconectadaDevolveEstado.
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

			// GetQR deixou de nomear um engine nesta mensagem quando a
			// leitura passou para trás de port.PairingQRReader: o use case já
			// não sabe qual transporte serve a sessão, e continuar a escrever
			// "noise" seria uma linha de log que mente para metade dos
			// pedidos. Os outros seis ainda consomem portas só do noise.
			wantRefusalMsg := "no noise session"
			if tc.name == "GetQR" {
				wantRefusalMsg = "no session for QR read"
			}
			rec, found := log.FindLevel(contractsfake.LevelWarn, wantRefusalMsg)
			if !found {
				t.Fatalf("recusa de sessao nao foi logada em nivel warn (F72): %v", log.Messages())
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

			var err error
			switch tc.name {
			case "GetQR":
				_, err = session.NewGetQRUseCase(qrDe(sg, users), log).Execute(context.Background(), txtID)
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
			return session.NewPairPhoneUseCase(pairerDe(sg), log).
				Execute(context.Background(), txtID, domain.PairPhoneRequest{})
		}},
		{"SetStatusMessage sem Body", func(sg *contractsfake.SessionGuard, log *contractsfake.Logger) (any, error) {
			return session.NewSetStatusMessageUseCase(statusDe(sg), log).Execute(context.Background(), txtID, domain.SetStatusMessageRequest{})
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
	// O valor semeado é um data URI, e não "2@abc", porque é isso que a
	// coluna users.qrcode REALMENTE guarda: o listener de QR do noise
	// escreve o PNG já codificado (pkg/application/session/orchestrator.go,
	// onPairingQR — "A coluna guarda a IMAGEM"). O dublê dizia "2@abc" e por
	// isso divergia da produção na exata regra em causa; foi a F373 que o
	// mediu em campo (GET /session/pair/qr respondeu 1858 caracteres de
	// data URI contra um servidor vivo).
	t.Run("devolve o QR persistido", func(t *testing.T) {
		users := &contractsfake.UserRepository{ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{{ID: txtID, QRCode: qrNoisePersistido}}, nil
		}}
		log := &contractsfake.Logger{}

		r, err := session.NewGetQRUseCase(qrDe(&contractsfake.SessionGuard{}, users), log).
			Execute(context.Background(), txtID)

		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if r.QRCode != qrNoisePersistido {
			t.Errorf("QRCode = %.40q…, quero o data URI persistido tal e qual — recodificá-lo "+
				"produz um QR que desenha o próprio data URI (F373)", r.QRCode)
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

		r, err := session.NewGetQRUseCase(qrDe(&contractsfake.SessionGuard{}, users), log).
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

		r, err := session.NewGetQRUseCase(qrDe(&contractsfake.SessionGuard{}, users), log).
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

		r, err := session.NewGetQRUseCase(qrDe(&contractsfake.SessionGuard{}, users), log).
			Execute(context.Background(), txtID)

		assertNoSessionAppErr(t, err)
		if r != nil {
			t.Error("resultado devia ser nil")
		}
		// A ausência de registro passou a ser detectada pelo adaptador de
		// engine (pkg/infra/noise/adapters/pairing/qr.go) e não pelo use
		// case, então o registro que sobra aqui é o da falha da leitura.
		if !log.Logged("failed to read QR code") {
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

				r, err := session.NewGetStatusUseCase(status, users, log).
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

		r, err := session.NewGetStatusUseCase(&contractsfake.SessionStatusReader{}, users, &contractsfake.Logger{}).
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

		// Os dois resumos deixaram de ser map[string]any: hoje são
		// domain.ProxySummary e domain.S3Summary, e o nome de fio deles vive
		// em pkg/presentation/http/dto/session. Comparar a struct inteira
		// (e não campo a campo) é o que faz um campo NOVO não preenchido
		// aparecer aqui.
		wantProxy := domain.ProxySummary{Enabled: entry.HasProxyURL, ProxyURL: entry.ProxyURL}
		if r.ProxyConfig != wantProxy {
			t.Errorf("ProxyConfig = %+v, quero %+v", r.ProxyConfig, wantProxy)
		}

		wantS3 := domain.S3Summary{
			Enabled:       entry.S3.Enabled,
			Endpoint:      entry.S3.Endpoint,
			Region:        entry.S3.Region,
			Bucket:        entry.S3.Bucket,
			PathStyle:     entry.S3.PathStyle,
			PublicURL:     entry.S3.PublicURL,
			MediaDelivery: entry.S3.MediaDelivery,
			RetentionDays: entry.S3.RetentionDays,
		}
		if r.S3Config != wantS3 {
			t.Errorf("S3Config = %+v, quero %+v", r.S3Config, wantS3)
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

		r, err := session.NewGetStatusUseCase(status, users, log).
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

		r, err := session.NewGetStatusUseCase(&contractsfake.SessionStatusReader{}, users, log).
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
