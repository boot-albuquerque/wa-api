// storage_test.go — os 10 use cases de configuração de armazenamento.
//
// Os três eixos que valem asserção aqui, e nenhum outro:
//
//  1. a guarda de sessão. NOVE dos 10 abrem com EnsureSession — SetProxy é a
//     exceção, e o comentário em guardCases diz por quê —, e desde a
//     migração da F11 propagam a causa (return err), não mais um
//     fmt.Errorf de texto fixo que apagava o erro tipado da porta. O teste
//     assere errors.Is contra a sentinela injetada — se alguém reintroduzir
//     a tradução, o errors.Is falha. Nenhuma asserção sobre texto de erro.
//  2. as validações próprias de cada use case (media_delivery, history < 0,
//     campos obrigatórios de S3, URL de proxy/endpoint), medidas pelo
//     resultado observável: erro não-nil e resultado nil.
//  3. o log: caminho de recusa emite "no wa-noise session" em nível error
//     carregando txtID e error; caminho feliz emite um Info.
package storage_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/storage"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// errNoSession é a sentinela que a porta devolve quando não há cliente. É o
// erro tipado que a migração da F11 passou a propagar intacto.
var errNoSession = errors.New("porta: sem sessao wanoise")

const txtID = "user-1"

// execFn adapta os 10 Execute com assinaturas distintas a um denominador
// comum: (ok, err). ok reporta se o ponteiro de resultado veio não-nil.
type execFn func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (ok bool, err error)

// guardCases enumera os nove use cases que têm a guarda de sessão em comum,
// com os argumentos do caminho feliz de cada um.
func guardCases() []struct {
	name string
	run  execFn
} {
	return []struct {
		name string
		run  execFn
	}{
		{"ConfigureHmac", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := newConfigureHmac(sg, log).Execute(ctx, txtID, domain.HmacConfigRequest{HmacKey: validHmacKey})
			return r != nil, err
		}},
		{"ConfigureS3", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := newConfigureS3(sg, log).Execute(ctx, txtID, domain.S3ConfigRequest{Enabled: true})
			return r != nil, err
		}},
		{"DeleteHmacConfig", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := storage.NewDeleteHmacConfigUseCase(sg, &contractsfake.HmacKeyStore{}, &contractsfake.UserInfoHmacCache{}, log).Execute(ctx, txtID)
			return r != nil, err
		}},
		{"DeleteS3Config", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := newDeleteS3Config(sg, log).Execute(ctx, txtID)
			return r != nil, err
		}},
		{"GetHistory", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := storage.NewGetHistoryUseCase(sg, &contractsfake.HistoryConfigStore{}, log).Execute(ctx, txtID)
			return r != nil, err
		}},
		{"GetHmacConfig", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := storage.NewGetHmacConfigUseCase(sg, &contractsfake.HmacKeyStore{}, log).Execute(ctx, txtID)
			return r != nil, err
		}},
		{"GetS3Config", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := storage.NewGetS3ConfigUseCase(sg, &contractsfake.S3ConfigStore{}, log).Execute(ctx, txtID)
			return r != nil, err
		}},
		{"SetHistory", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := newSetHistory(sg, log, &contractsfake.HistoryConfigStore{}, &contractsfake.UserInfoSessionCache{}).
				Execute(ctx, txtID, domain.WebhookHistoryRequest{History: 10})
			return r != nil, err
		}},
		// SetProxy NAO entra nesta tabela, e a ausencia e' contrato e nao
		// esquecimento: ele e' o unico dos dez que NAO abre com EnsureSession.
		// A guarda dele e' a oposta — recusa a sessao CONECTADA
		// (`41bc8e2^:handlers.go:6099`) —, e quem nao tem sessao nenhuma tem de
		// conseguir configurar o proxy, que e' exatamente o estado de quem vai
		// conectar atraves dele. Os eixos dele estao em session_config_test.go.
		{"TestS3Connection", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := newTestS3Connection(sg, log, enabledS3Store(txtID)).Execute(ctx, txtID)
			return r != nil, err
		}},
	}
}

func TestUseCases_SemSessao_PropagamACausaEnaoATraduzem(t *testing.T) {
	for _, tc := range guardCases() {
		t.Run(tc.name, func(t *testing.T) {
			sg := contractsfake.FailSession(errNoSession)
			log := &contractsfake.Logger{}

			ok, err := tc.run(context.Background(), &sg, log)

			if err == nil {
				t.Fatal("sessao recusada devia produzir erro")
			}
			if !errors.Is(err, errNoSession) {
				t.Fatalf("a causa da porta se perdeu no caminho: %v — a traducao fmt.Errorf(\"no session\") voltou?", err)
			}
			if ok {
				t.Error("resultado devia ser nil quando a sessao e' recusada")
			}
			if len(sg.EnsureSessionCalls) != 1 {
				t.Fatalf("EnsureSession chamada %d vez(es), quero 1", len(sg.EnsureSessionCalls))
			}
			if sg.EnsureSessionCalls[0].TxtID != txtID {
				t.Errorf("EnsureSession recebeu txtID %q, quero %q", sg.EnsureSessionCalls[0].TxtID, txtID)
			}

			rec, found := log.FindLevel(contractsfake.LevelWarn, "no wanoise session")
			if !found {
				t.Fatalf("recusa de sessao nao foi logada em nivel warn (F72): %v", log.Records())
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

func TestUseCases_ComSessao_LogamOSucessoEDevolvemResultado(t *testing.T) {
	for _, tc := range guardCases() {
		t.Run(tc.name, func(t *testing.T) {
			sg := &contractsfake.SessionGuard{}
			log := &contractsfake.Logger{}

			ok, err := tc.run(context.Background(), sg, log)

			if err != nil {
				t.Fatalf("caminho feliz devolveu erro: %v", err)
			}
			if !ok {
				t.Fatal("caminho feliz devolveu resultado nil")
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

// --- Resultados nomeados do caminho feliz ------------------------------

func TestResultadosDoCaminhoFeliz(t *testing.T) {
	ctx := context.Background()
	sg := &contractsfake.SessionGuard{}
	log := &contractsfake.Logger{}

	// Enabled deixou de espelhar o pedido: ele reporta o ESTADO depois da
	// operacao. Gravar uma chave habilita; revogar desabilita. O request nao
	// tem mais campo `enabled` — nunca existiu no fio (41bc8e2^:handlers.go:6767).
	t.Run("ConfigureHmac habilita apos gravar", func(t *testing.T) {
		r, err := newConfigureHmac(sg, log).Execute(ctx, txtID, domain.HmacConfigRequest{HmacKey: validHmacKey})
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if !r.Enabled {
			t.Error("Enabled = false apos gravar a chave, quero true")
		}
		if r.Details == "" {
			t.Error("Details vazio")
		}
	})

	t.Run("DeleteHmacConfig zera Enabled", func(t *testing.T) {
		r, err := storage.NewDeleteHmacConfigUseCase(sg, &contractsfake.HmacKeyStore{}, &contractsfake.UserInfoHmacCache{}, log).Execute(ctx, txtID)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if r.Enabled {
			t.Error("delete devia devolver Enabled=false")
		}
	})

	t.Run("DeleteS3Config zera Enabled", func(t *testing.T) {
		r, err := newDeleteS3Config(sg, log).Execute(ctx, txtID)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if r.Enabled {
			t.Error("delete devia devolver Enabled=false")
		}
	})

	t.Run("SetHistory espelha History", func(t *testing.T) {
		r, err := newSetHistory(sg, log, &contractsfake.HistoryConfigStore{}, &contractsfake.UserInfoSessionCache{}).
			Execute(ctx, txtID, domain.WebhookHistoryRequest{History: 42})
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if r.History != 42 {
			t.Errorf("History = %d, quero 42", r.History)
		}
	})

	t.Run("TestS3Connection marca Connected", func(t *testing.T) {
		r, err := newTestS3Connection(sg, log, enabledS3Store(txtID)).Execute(ctx, txtID)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if !r.Connected {
			t.Error("Connected = false, quero true")
		}
	})

	// GetHmacConfig ficou de fora: ele devolve HmacConfigView (`hmac_key`
	// mascarado), nao HmacConfigResult. O contrato dele esta' em
	// hmac_config_test.go.
	// GetHistory nao entra na lista de "devolvem Details": desde o CAP-32 ele
	// nao devolve Details nenhum. O `Details` que ele respondia
	// ("History configuration retrieved") era ficcao do stub da migracao, e
	// nao contrato historico — `41bc8e2^:handlers.go:6497` lia historico de
	// MENSAGENS (HOUSEKEEP F166). O que a leitura devolve agora e' o limite
	// gravado, e e' isso que este subteste assere.
	t.Run("GetHistory devolve o limite gravado", func(t *testing.T) {
		const gravado = 50
		store := &contractsfake.HistoryConfigStore{Stored: map[string]int{txtID: gravado}}

		r, err := storage.NewGetHistoryUseCase(sg, store, log).Execute(ctx, txtID)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if r.History != gravado {
			t.Errorf("History = %d, quero %d", r.History, gravado)
		}
		if r.Details != "" {
			t.Errorf("Details = %q, quero vazio — a leitura nao fabrica texto", r.Details)
		}
	})
}

// --- Validações próprias ------------------------------------------------

func TestConfigureS3_MediaDelivery(t *testing.T) {
	cases := []struct {
		name     string
		delivery string
		wantErr  bool
		// wantStored é o valor que o caminho feliz deve ter normalizado.
		// "" no request vira "base64" — a única normalização do use case.
		wantEnabled bool
	}{
		{name: "vazio vira base64", delivery: "", wantEnabled: true},
		{name: "base64", delivery: "base64", wantEnabled: true},
		{name: "s3", delivery: "s3", wantEnabled: true},
		{name: "both", delivery: "both", wantEnabled: true},
		{name: "desconhecido e' recusado", delivery: "carrier-pigeon", wantErr: true},
		{name: "maiuscula nao vale", delivery: "S3", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log := &contractsfake.Logger{}
			r, err := newConfigureS3(&contractsfake.SessionGuard{}, log).
				Execute(context.Background(), txtID, domain.S3ConfigRequest{Enabled: true, MediaDelivery: tc.delivery})

			if tc.wantErr {
				if err == nil {
					t.Fatalf("media_delivery %q devia ser recusado", tc.delivery)
				}
				if r != nil {
					t.Error("resultado devia ser nil na recusa")
				}
				if len(log.ByLevel(contractsfake.LevelInfo)) != 0 {
					t.Error("recusa nao devia logar sucesso")
				}
				return
			}
			if err != nil {
				t.Fatalf("media_delivery %q devia ser aceito: %v", tc.delivery, err)
			}
			if r.Enabled != tc.wantEnabled {
				t.Errorf("Enabled = %v, quero %v", r.Enabled, tc.wantEnabled)
			}
		})
	}
}

func TestConfigureS3_Endpoint(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{name: "vazio e' o default da AWS e passa", endpoint: ""},
		{name: "IP publico literal passa sem DNS", endpoint: "https://93.184.216.34:9000"},
		{name: "loopback e' recusado", endpoint: "https://127.0.0.1:9000", wantErr: true},
		{name: "malformado e' recusado", endpoint: "nao-e-uma-url", wantErr: true},
		{name: "esquema nao-http e' recusado", endpoint: "ftp://93.184.216.34", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := newConfigureS3(&contractsfake.SessionGuard{}, &contractsfake.Logger{}).
				Execute(context.Background(), txtID, domain.S3ConfigRequest{Enabled: true, Endpoint: tc.endpoint})

			if tc.wantErr {
				if err == nil {
					t.Fatalf("endpoint %q devia ser recusado", tc.endpoint)
				}
				if r != nil {
					t.Error("resultado devia ser nil na recusa")
				}
				return
			}
			if err != nil {
				t.Fatalf("endpoint %q devia ser aceito: %v", tc.endpoint, err)
			}
		})
	}
}

// TestSetProxy_URL saiu daqui para session_config_test.go, junto com os
// outros eixos de SetProxy: a validacao de URL deixou de ser
// egress.ValidateOutboundURL e passou a ser a historica (so' `http` e
// `socks5`), o que muda cada um dos cinco casos que existiam aqui.

func TestSetHistory_ValorNegativo(t *testing.T) {
	cases := []struct {
		name    string
		history int
		wantErr bool
	}{
		{name: "zero e' valido", history: 0},
		{name: "positivo e' valido", history: 1},
		{name: "negativo e' recusado", history: -1, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := newSetHistory(&contractsfake.SessionGuard{}, &contractsfake.Logger{},
				&contractsfake.HistoryConfigStore{}, &contractsfake.UserInfoSessionCache{}).
				Execute(context.Background(), txtID, domain.WebhookHistoryRequest{History: tc.history})

			if tc.wantErr {
				if err == nil {
					t.Fatalf("history %d devia ser recusado", tc.history)
				}
				if r != nil {
					t.Error("resultado devia ser nil na recusa")
				}
				return
			}
			if err != nil {
				t.Fatalf("history %d devia ser aceito: %v", tc.history, err)
			}
			if r.History != tc.history {
				t.Errorf("History = %d, quero %d", r.History, tc.history)
			}
		})
	}
}

// TestTestS3Connection_SemConfiguracaoHabilitada — o use case deixou de ler o
// corpo (ele testa a configuracao GRAVADA, `41bc8e2^:handlers.go:6383`), entao
// a recusa que existe e' a do estado: sem linha, ou com S3 desabilitado.
func TestTestS3Connection_SemConfiguracaoHabilitada(t *testing.T) {
	cases := []struct {
		name  string
		store *contractsfake.S3ConfigStore
	}{
		{"nunca configurado", &contractsfake.S3ConfigStore{}},
		{"configurado e desabilitado", &contractsfake.S3ConfigStore{Stored: map[string]port.S3ConfigRecord{
			txtID: {Enabled: false, Bucket: "b", Region: "us-east-1"},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log := &contractsfake.Logger{}
			r, err := newTestS3Connection(&contractsfake.SessionGuard{}, log, tc.store).
				Execute(context.Background(), txtID)

			if err == nil {
				t.Fatal("S3 desabilitado devia ser recusado")
			}
			if r != nil {
				t.Error("resultado devia ser nil na recusa")
			}
			if len(log.ByLevel(contractsfake.LevelInfo)) != 0 {
				t.Error("recusa nao devia logar sucesso")
			}
		})
	}
}

// TestTestS3Connection_RecusaDoUpstreamVira422TipadoENaoTextoSolto é o teste
// do defeito da F276: uma recusa da AWS (credenciais inválidas, bucket
// errado — qualquer coisa que TestConnection devolva) subia crua até a
// fronteira HTTP, que a servia como 500 com `error` em string solta, em vez
// do envelope canônico {code,error:{code,message}} com o 422 que a categoria
// de recusa upstream já define para casos irmãos (/users/block).
func TestTestS3Connection_RecusaDoUpstreamVira422TipadoENaoTextoSolto(t *testing.T) {
	upstreamErr := errors.New("operation error S3: ListObjectsV2, https response error StatusCode: 403, " +
		"api error InvalidAccessKeyId: The AWS Access Key Id you provided does not exist in our records.")
	clients := &contractsfake.S3ClientManager{
		TestConnectionFunc: func(context.Context, string) error { return upstreamErr },
	}
	log := &contractsfake.Logger{}
	uc := storage.NewTestS3ConnectionUseCase(&contractsfake.SessionGuard{}, enabledS3Store(txtID),
		&contractsfake.S3SecretCipher{}, clients, log)

	r, err := uc.Execute(context.Background(), txtID)

	if err == nil {
		t.Fatal("recusa do upstream devia ser recusada, não sucesso")
	}
	if r != nil {
		t.Error("resultado devia ser nil na recusa")
	}

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("erro não é *apperr.AppError (ficaria 500 opaco na fronteira HTTP): %v (%T)", err, err)
	}
	if appErr.Code != "upstream_rejected" {
		t.Errorf("code = %q, quero upstream_rejected", appErr.Code)
	}
	if appErr.Category != apperr.CategoryUpstreamRejected {
		t.Errorf("category = %q, quero %q", appErr.Category, apperr.CategoryUpstreamRejected)
	}
	if status := appErr.Category.HTTPStatus(); status != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, quero 422", status)
	}
	// A mensagem tem de trazer o diagnóstico do upstream — é a própria
	// razão de existir da rota — mas nunca o segredo armazenado.
	if !strings.Contains(appErr.Message, "InvalidAccessKeyId") {
		t.Errorf("mensagem não traz o diagnóstico do upstream: %q", appErr.Message)
	}
	if strings.Contains(appErr.Message, "sk") {
		t.Errorf("mensagem vazou o segredo armazenado: %q", appErr.Message)
	}
	if !errors.Is(err, upstreamErr) {
		t.Error("a cadeia de causa perdeu o erro original do SDK")
	}
}

// --- construtores dos quatro use cases de S3 ------------------------------
//
// Os dubles das quatro portas novas vem de contractsfake, que imita as regras
// REAIS dos adapters de producao (pkg/infra/db/s3_config_repository.go,
// pkg/infra/auth/s3_secret.go, pkg/infra/storage/s3.go).

func newConfigureS3(sg port.SessionGuard, log port.Logger) *storage.ConfigureS3UseCase {
	return storage.NewConfigureS3UseCase(sg, &contractsfake.S3ConfigStore{}, &contractsfake.S3SecretCipher{},
		&contractsfake.S3ClientManager{}, &contractsfake.UserInfoS3Cache{}, log)
}

func newDeleteS3Config(sg port.SessionGuard, log port.Logger) *storage.DeleteS3ConfigUseCase {
	return storage.NewDeleteS3ConfigUseCase(sg, &contractsfake.S3ConfigStore{}, &contractsfake.S3ClientManager{},
		&contractsfake.UserInfoS3Cache{}, log)
}

func newTestS3Connection(sg port.SessionGuard, log port.Logger, store *contractsfake.S3ConfigStore) *storage.TestS3ConnectionUseCase {
	return storage.NewTestS3ConnectionUseCase(sg, store, &contractsfake.S3SecretCipher{},
		&contractsfake.S3ClientManager{}, log)
}

// enabledS3Store e' o store de quem TEM S3 habilitado, com o segredo no
// envelope do dublê — a forma que o cifrador real produziria (ADR-0009). Um
// store vazio transformaria o caminho de sucesso na recusa 400 sem que o teste
// percebesse.
func enabledS3Store(userID string) *contractsfake.S3ConfigStore {
	return &contractsfake.S3ConfigStore{Stored: map[string]port.S3ConfigRecord{
		userID: {
			Enabled:       true,
			Region:        "us-east-1",
			Bucket:        "b",
			AccessKey:     "ak",
			SecretKey:     contractsfake.FakeS3EnvelopePrefix + "sk",
			MediaDelivery: domain.MediaDeliveryBase64,
		},
	}}
}
