package user_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// hmacKey32 tem exatamente o comprimento mínimo aceito por AddUser.
const hmacKey32 = "0123456789abcdef0123456789abcdef"

func TestAddUserUseCase_Execute_Rejections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  domain.AddUserInput
		want error // identidade esperada, quando há uma
	}{
		{
			name: "nome vazio",
			req:  domain.AddUserInput{Token: "tok"},
		},
		{
			name: "token vazio",
			req:  domain.AddUserInput{Name: "alice"},
		},
		{
			name: "hmac curto demais",
			req:  domain.AddUserInput{Name: "alice", Token: "tok", HmacKey: "curto"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{}
			uc := user.NewAddUserUseCase(repo, &contractsfake.HmacKeyEncryptor{}, &contractsfake.S3SecretCipher{}, &contractsfake.Logger{}, true)

			resp, err := uc.Execute(context.Background(), tt.req)
			if err == nil {
				t.Fatal("esperava erro de validação")
			}
			if resp != nil {
				t.Errorf("resposta = %+v, queria nil", resp)
			}
			if len(repo.CreateUserCalls) != 0 {
				t.Errorf("CreateUser chamado %d vezes, queria 0", len(repo.CreateUserCalls))
			}
		})
	}
}

func TestAddUserUseCase_Execute_DuplicateToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		fn   func(ctx context.Context, rec domain.UserRecord) (bool, error)
	}{
		{
			name: "adapter devolve ErrDuplicateToken",
			fn: func(context.Context, domain.UserRecord) (bool, error) {
				return false, user.ErrDuplicateToken
			},
		},
		{
			name: "adapter recusa sem erro",
			fn: func(context.Context, domain.UserRecord) (bool, error) {
				return false, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{CreateUserFunc: tt.fn}
			uc := user.NewAddUserUseCase(repo, &contractsfake.HmacKeyEncryptor{}, &contractsfake.S3SecretCipher{}, &contractsfake.Logger{}, true)

			_, err := uc.Execute(context.Background(), domain.AddUserInput{Name: "alice", Token: "tok", Engine: "noise"})
			if !errors.Is(err, user.ErrDuplicateToken) {
				t.Fatalf("err = %v, queria user.ErrDuplicateToken", err)
			}
		})
	}
}

func TestAddUserUseCase_Execute_RepositoryError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	repo := &contractsfake.UserRepository{
		CreateUserFunc: func(context.Context, domain.UserRecord) (bool, error) { return false, boom },
	}
	logger := &contractsfake.Logger{}
	uc := user.NewAddUserUseCase(repo, &contractsfake.HmacKeyEncryptor{}, &contractsfake.S3SecretCipher{}, logger, true)

	_, err := uc.Execute(context.Background(), domain.AddUserInput{Name: "alice", Token: "tok", Engine: "noise"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, queria embrulhar boom", err)
	}
	rec, ok := logger.FindLevel(contractsfake.LevelError, "Failed to insert user")
	if !ok {
		t.Fatal("esperava log de erro da inserção")
	}
	if !rec.IsStructured() {
		t.Errorf("keyvals = %v, queria pares estruturados", rec.Keyvals)
	}
}

func TestAddUserUseCase_Execute_Success(t *testing.T) {
	t.Parallel()

	useProxy := false
	tests := []struct {
		name           string
		req            domain.AddUserInput
		wantProxy      bool
		wantS3Enabled  bool
		wantHmacConfig bool
	}{
		{
			name: "mínimo, com defaults",
			req:  domain.AddUserInput{Name: "alice", Token: "tok", Engine: "noise"},
			// sem ProxyConfig, webhookUseProxy vira true por default
			wantProxy: true,
		},
		{
			name: "webhookUseProxy explícito em false",
			req: domain.AddUserInput{
				Name:        "bob",
				Token:       "tok2",
				Engine:      "noise",
				ProxyConfig: &domain.ProxyConfig{ProxyURL: "http://proxy:8080", WebhookUseProxy: &useProxy},
			},
			wantProxy: false,
		},
		{
			name: "eventos com entradas vazias são ignoradas",
			req:  domain.AddUserInput{Name: "carol", Token: "tok3", Events: "Message,, ,ReadReceipt", Engine: "noise"},
			// a entrada vazia entra no `continue`, não vira erro
			wantProxy: true,
		},
		{
			name:           "hmac no comprimento mínimo",
			req:            domain.AddUserInput{Name: "dave", Token: "tok4", HmacKey: hmacKey32, Engine: "noise"},
			wantProxy:      true,
			wantHmacConfig: true,
		},
		{
			name: "s3 habilitado inicializa o cliente",
			req: domain.AddUserInput{
				Name:   "erin",
				Token:  "tok5",
				Engine: "noise",
				S3Config: &domain.S3Config{
					Enabled: true, Endpoint: "http://s3:9000", Region: "us-east-1",
					Bucket: "b", AccessKey: "ak", SecretKey: "sk", PathStyle: true,
					PublicURL: "http://cdn", MediaDelivery: "base64", RetentionDays: 7,
				},
			},
			wantProxy:     true,
			wantS3Enabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{}
			uc := user.NewAddUserUseCase(repo, &contractsfake.HmacKeyEncryptor{}, &contractsfake.S3SecretCipher{}, &contractsfake.Logger{}, true)

			resp, err := uc.Execute(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(repo.CreateUserCalls) != 1 {
				t.Fatalf("CreateUser chamado %d vezes, queria 1", len(repo.CreateUserCalls))
			}
			rec := repo.CreateUserCalls[0].Rec
			if rec.ID == "" {
				t.Error("ID gerado vazio")
			}
			if rec.Name != tt.req.Name || rec.Token != tt.req.Token {
				t.Errorf("registro gravado = %+v, queria nome/token do request", rec)
			}
			if rec.WebhookUseProxy != tt.wantProxy {
				t.Errorf("WebhookUseProxy = %v, queria %v", rec.WebhookUseProxy, tt.wantProxy)
			}
			if resp.ID != rec.ID {
				t.Errorf("resp.ID = %q, queria %q", resp.ID, rec.ID)
			}
			if resp.HmacConfigured != tt.wantHmacConfig {
				t.Errorf("HmacConfigured = %v, queria %v", resp.HmacConfigured, tt.wantHmacConfig)
			}
			if got := resp.S3.Enabled; got != tt.wantS3Enabled {
				t.Errorf("s3 enabled = %v, queria %v", got, tt.wantS3Enabled)
			}
			// A chave de acesso não volta no resultado em forma nenhuma. A
			// máscara "***" que a resposta devolvia era a mesma com e sem
			// chave configurada, e nenhum campo a substitui: o resultado não
			// tem onde a guardar.
			if got := resp.Proxy.WebhookUseProxy; got != tt.wantProxy {
				t.Errorf("proxy WebhookUseProxy = %v, queria %v", got, tt.wantProxy)
			}
			// A asserção é da PROPRIEDADE, não de "gravou algo": a versão
			// anterior só exigia len != 0, e por isso não viu a chave sendo
			// gravada em CLARO (F158). A prova de ida e volta contra o
			// AES-GCM real está em pkg/bootstrap/add_user_hmac_route_test.go.
			// S3 secret key must be enveloped, not plaintext (F163).
			if tt.wantS3Enabled && tt.req.S3Config != nil && tt.req.S3Config.SecretKey != "" {
				stored := rec.S3.SecretKey
				if stored == tt.req.S3Config.SecretKey {
					t.Errorf("S3 SecretKey stored as PLAINTEXT — got %q", stored)
				}
				want := contractsfake.FakeS3EnvelopePrefix + tt.req.S3Config.SecretKey
				if stored != want {
					t.Errorf("S3 SecretKey = %q, want %q (fake cipher output)", stored, want)
				}
			}
			if tt.wantHmacConfig {
				if len(rec.HmacKey) == 0 {
					t.Error("HmacKey gravada vazia")
				}
				if string(rec.HmacKey) == tt.req.HmacKey {
					t.Errorf("HmacKey gravada = texto plano do request, queria cifrada")
				}
				if want := contractsfake.FakeCipherPrefix + tt.req.HmacKey; string(rec.HmacKey) != want {
					t.Errorf("HmacKey gravada = %q, queria %q (a saída do cifrador)", rec.HmacKey, want)
				}
			}
		})
	}
}

func TestAddUserUseCase_Execute_CifraFalhaNaoCriaUsuario(t *testing.T) {
	t.Parallel()

	boom := errors.New("encryption key not configured")
	repo := &contractsfake.UserRepository{}
	encryptor := &contractsfake.HmacKeyEncryptor{
		EncryptHmacKeyFunc: func(string) ([]byte, error) { return nil, boom },
	}
	logger := &contractsfake.Logger{}
	uc := user.NewAddUserUseCase(repo, encryptor, &contractsfake.S3SecretCipher{}, logger, true)

	resp, err := uc.Execute(context.Background(),
		domain.AddUserInput{Name: "alice", Token: "tok", HmacKey: hmacKey32, Engine: "noise"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, queria embrulhar boom", err)
	}
	if resp != nil {
		t.Errorf("resposta = %+v, queria nil", resp)
	}
	// Falha FECHADA: a ORDEM é o contrato. Se CreateUser fosse chamado, o
	// usuário nasceria sem chave nenhuma e com 200 no lugar do erro.
	if len(repo.CreateUserCalls) != 0 {
		t.Errorf("CreateUser chamado %d vezes, queria 0", len(repo.CreateUserCalls))
	}
	// A chave em claro não entra no log do erro: só o tamanho.
	for _, rec := range logger.Records() {
		for _, kv := range rec.Keyvals {
			if v, ok := kv.(string); ok && v == hmacKey32 {
				t.Errorf("a chave em claro apareceu no log: %v", rec.Keyvals)
			}
		}
	}
}

func TestAddUserUseCase_Execute_ChaveCurtaNaoChegaAoCifrador(t *testing.T) {
	t.Parallel()

	repo := &contractsfake.UserRepository{}
	encryptor := &contractsfake.HmacKeyEncryptor{}
	uc := user.NewAddUserUseCase(repo, encryptor, &contractsfake.S3SecretCipher{}, &contractsfake.Logger{}, true)

	_, err := uc.Execute(context.Background(),
		domain.AddUserInput{Name: "alice", Token: "tok", HmacKey: hmacKey32[:len(hmacKey32)-1], Engine: "noise"})
	if err == nil {
		t.Fatal("esperava recusa por chave curta")
	}
	// Validar ANTES de cifrar: uma chave curta nunca chega ao cifrador.
	if len(encryptor.EncryptHmacKeyCalls) != 0 {
		t.Errorf("EncryptHmacKey chamado %d vezes, queria 0", len(encryptor.EncryptHmacKeyCalls))
	}
	if len(repo.CreateUserCalls) != 0 {
		t.Errorf("CreateUser chamado %d vezes, queria 0", len(repo.CreateUserCalls))
	}
}

// TestAddUserUseCase_Execute_EventoInvalidoNaoChegaAoRepositorio trava a
// ORDEM da recusa da F159: validar ANTES de gravar.
//
// O eixo de fronteira (status 400 e mensagem do envelope) fica em
// pkg/bootstrap/add_user_events_route_test.go, pela rota registrada. Aqui a
// asserção é a que só o dublê enxerga: CreateUser não é chamado NENHUMA vez.
// A lista mista é o caso que separa RECUSAR de FILTRAR — um filtro
// silencioso chamaria CreateUser com o evento válido remanescente.
func TestAddUserUseCase_Execute_EventoInvalidoNaoChegaAoRepositorio(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		events string
	}{
		{name: "evento com typo", events: "Mesage"},
		{name: "lista mista, um válido e um inválido", events: "Message,Mesage"},
		{name: "lista mista, o inválido primeiro", events: "Mesage,Message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{}
			uc := user.NewAddUserUseCase(repo, &contractsfake.HmacKeyEncryptor{}, &contractsfake.S3SecretCipher{}, &contractsfake.Logger{}, true)

			resp, err := uc.Execute(context.Background(),
				domain.AddUserInput{Name: "alice", Token: "tok", Events: tt.events, Engine: "noise"})
			if err == nil {
				t.Fatal("esperava recusa por tipo de evento desconhecido")
			}
			if resp != nil {
				t.Errorf("resposta = %+v, queria nil", resp)
			}
			if len(repo.CreateUserCalls) != 0 {
				t.Errorf("CreateUser chamado %d vezes, queria 0", len(repo.CreateUserCalls))
			}
		})
	}
}

// TestAddUserUseCase_Execute_EventosValidosChegamIntactos assegura o caminho
// de SUCESSO: o campo gravado é o do request, sem reescrita. A validação faz
// TrimSpace só para decidir, e não normaliza o valor.
func TestAddUserUseCase_Execute_EventosValidosChegamIntactos(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		events string
	}{
		{name: "um evento válido", events: "Message"},
		{name: "vários eventos válidos", events: "Message,ReadReceipt,Presence"},
		{name: "All", events: "All"},
		{name: "espaços em volta", events: "  Message  "},
		{name: "events vazio", events: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{}
			uc := user.NewAddUserUseCase(repo, &contractsfake.HmacKeyEncryptor{}, &contractsfake.S3SecretCipher{}, &contractsfake.Logger{}, true)

			resp, err := uc.Execute(context.Background(),
				domain.AddUserInput{Name: "alice", Token: "tok", Events: tt.events, Engine: "noise"})
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(repo.CreateUserCalls) != 1 {
				t.Fatalf("CreateUser chamado %d vezes, queria 1", len(repo.CreateUserCalls))
			}
			if got := repo.CreateUserCalls[0].Rec.Events; got != tt.events {
				t.Errorf("Events gravado = %q, queria %q", got, tt.events)
			}
			if resp.Events != tt.events {
				t.Errorf("resp.Events = %q, queria %q", resp.Events, tt.events)
			}
		})
	}
}

// TestAddUserUseCase_Execute_S3CifraFalhaNaoCriaUsuario locks the ORDER
// contract for the S3 secret key (F163): encrypt BEFORE write; a cipher
// failure must not create the user row.
func TestAddUserUseCase_Execute_S3CifraFalhaNaoCriaUsuario(t *testing.T) {
	t.Parallel()

	boom := errors.New("s3 encryption key not configured")
	repo := &contractsfake.UserRepository{}
	s3Cipher := &contractsfake.S3SecretCipher{
		EncryptS3SecretFunc: func(string) (string, error) { return "", boom },
	}
	logger := &contractsfake.Logger{}
	uc := user.NewAddUserUseCase(repo, &contractsfake.HmacKeyEncryptor{}, s3Cipher, logger, true)

	resp, err := uc.Execute(context.Background(),
		domain.AddUserInput{
			Name:   "alice",
			Token:  "tok",
			Engine: "noise",
			S3Config: &domain.S3Config{
				Enabled: true, SecretKey: "my-s3-secret",
			},
		})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want to wrap boom", err)
	}
	if resp != nil {
		t.Errorf("resp = %+v, want nil", resp)
	}
	if len(repo.CreateUserCalls) != 0 {
		t.Errorf("CreateUser called %d times, want 0", len(repo.CreateUserCalls))
	}
}

// TestAddUserUseCase_Execute_EngineObrigatorio trava os itens 4-5 do prompt
// arquitetural: engine ausente, nulo (zero value da string), vazio ou fora de
// {noise, headless} é recusado com invalid_engine ANTES de qualquer
// escrita — nunca um default silencioso (era o comportamento antigo do
// UserRecord.Engine zero-value, documentado em pkg/domain/user_record.go, e
// que esta mudança fecha do lado HTTP).
func TestAddUserUseCase_Execute_EngineObrigatorio(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		engine string
	}{
		{name: "ausente (zero value)", engine: ""},
		{name: "legacy_unknown não é escolha de criação", engine: "legacy_unknown"},
		{name: "valor arbitrário", engine: "postgres"},
		{name: "case errado não é normalizado", engine: "NOISE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{}
			uc := user.NewAddUserUseCase(repo, &contractsfake.HmacKeyEncryptor{}, &contractsfake.S3SecretCipher{}, &contractsfake.Logger{}, true)

			resp, err := uc.Execute(context.Background(),
				domain.AddUserInput{Name: "alice", Token: "tok", Engine: tt.engine})

			var appErr *apperr.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("err = %v, queria *apperr.AppError", err)
			}
			if appErr.Code != "invalid_engine" {
				t.Errorf("code = %q, queria %q", appErr.Code, "invalid_engine")
			}
			if appErr.Category != apperr.CategoryValidation {
				t.Errorf("category = %q, queria %q (400)", appErr.Category, apperr.CategoryValidation)
			}
			if resp != nil {
				t.Errorf("resposta = %+v, queria nil", resp)
			}
			if len(repo.CreateUserCalls) != 0 {
				t.Errorf("CreateUser chamado %d vezes, queria 0 — engine inválido não pode chegar à escrita", len(repo.CreateUserCalls))
			}
		})
	}
}

// TestAddUserUseCase_Execute_EngineValidoEhPersistido é o controle POSITIVO
// da TestAddUserUseCase_Execute_EngineObrigatorio: os dois únicos valores
// aceitos criam o usuário e o motor gravado é exatamente o que veio no
// request — sem reescrita — e a resposta o reflete (item 3: leitura
// administrativa mostra o motor).
func TestAddUserUseCase_Execute_EngineValidoEhPersistido(t *testing.T) {
	t.Parallel()

	for _, engine := range []string{"noise", "headless"} {
		t.Run(engine, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{}
			uc := user.NewAddUserUseCase(repo, &contractsfake.HmacKeyEncryptor{}, &contractsfake.S3SecretCipher{}, &contractsfake.Logger{}, true)

			resp, err := uc.Execute(context.Background(),
				domain.AddUserInput{Name: "alice", Token: "tok", Engine: engine})
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(repo.CreateUserCalls) != 1 {
				t.Fatalf("CreateUser chamado %d vezes, queria 1", len(repo.CreateUserCalls))
			}
			if got := repo.CreateUserCalls[0].Rec.Engine.String(); got != engine {
				t.Errorf("engine gravado = %q, queria %q", got, engine)
			}
			if resp.Engine != engine {
				t.Errorf("resp.Engine = %q, queria %q", resp.Engine, engine)
			}
		})
	}
}
