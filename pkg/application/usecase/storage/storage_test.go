// storage_test.go — os 10 use cases de configuração de armazenamento.
//
// Os três eixos que valem asserção aqui, e nenhum outro:
//
//  1. a guarda de sessão. Todos os 10 abrem com EnsureSession, e desde a
//     migração da F11 propagam a causa (return err), não mais um
//     fmt.Errorf de texto fixo que apagava o erro tipado da porta. O teste
//     assere errors.Is contra a sentinela injetada — se alguém reintroduzir
//     a tradução, o errors.Is falha. Nenhuma asserção sobre texto de erro.
//  2. as validações próprias de cada use case (media_delivery, history < 0,
//     campos obrigatórios de S3, URL de proxy/endpoint), medidas pelo
//     resultado observável: erro não-nil e resultado nil.
//  3. o log: caminho de recusa emite "no whatsmeow session" em nível error
//     carregando txtID e error; caminho feliz emite um Info.
package storage_test

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/storage"
	"wa-api/pkg/domain"
)

// errNoSession é a sentinela que a porta devolve quando não há cliente. É o
// erro tipado que a migração da F11 passou a propagar intacto.
var errNoSession = errors.New("porta: sem sessao whatsmeow")

const txtID = "user-1"

// execFn adapta os 10 Execute com assinaturas distintas a um denominador
// comum: (ok, err). ok reporta se o ponteiro de resultado veio não-nil.
type execFn func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (ok bool, err error)

// guardCases enumera os 10 use cases pelo que têm em comum — a guarda de
// sessão — com os argumentos do caminho feliz de cada um.
func guardCases() []struct {
	name string
	run  execFn
} {
	return []struct {
		name string
		run  execFn
	}{
		{"ConfigureHmac", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := storage.NewConfigureHmacUseCase(sg, log).Execute(ctx, txtID, domain.HmacConfigRequest{Enabled: true})
			return r != nil, err
		}},
		{"ConfigureS3", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := storage.NewConfigureS3UseCase(sg, log).Execute(ctx, txtID, domain.S3ConfigRequest{Enabled: true})
			return r != nil, err
		}},
		{"DeleteHmacConfig", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := storage.NewDeleteHmacConfigUseCase(sg, log).Execute(ctx, txtID)
			return r != nil, err
		}},
		{"DeleteS3Config", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := storage.NewDeleteS3ConfigUseCase(sg, log).Execute(ctx, txtID)
			return r != nil, err
		}},
		{"GetHistory", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := storage.NewGetHistoryUseCase(sg, log).Execute(ctx, txtID)
			return r != nil, err
		}},
		{"GetHmacConfig", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := storage.NewGetHmacConfigUseCase(sg, log).Execute(ctx, txtID)
			return r != nil, err
		}},
		{"GetS3Config", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := storage.NewGetS3ConfigUseCase(sg, log).Execute(ctx, txtID)
			return r != nil, err
		}},
		{"SetHistory", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := storage.NewSetHistoryUseCase(sg, log).Execute(ctx, txtID, domain.WebhookHistoryRequest{History: 10})
			return r != nil, err
		}},
		{"SetProxy", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := storage.NewSetProxyUseCase(sg, log).Execute(ctx, txtID, domain.ProxyConfigRequest{})
			return r != nil, err
		}},
		{"TestS3Connection", func(ctx context.Context, sg *contractsfake.SessionGuard, log *contractsfake.Logger) (bool, error) {
			r, err := storage.NewTestS3ConnectionUseCase(sg, log).Execute(ctx, txtID, domain.S3TestRequest{
				Endpoint: "https://s3.example", Region: "us-east-1", Bucket: "b", AccessKey: "ak", SecretKey: "sk",
			})
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

			rec, found := log.FindLevel(contractsfake.LevelError, "no whatsmeow session")
			if !found {
				t.Fatalf("recusa de sessao nao foi logada em nivel error: %v", log.Records())
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

	t.Run("ConfigureHmac espelha Enabled", func(t *testing.T) {
		for _, enabled := range []bool{true, false} {
			r, err := storage.NewConfigureHmacUseCase(sg, log).Execute(ctx, txtID, domain.HmacConfigRequest{Enabled: enabled})
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if r.Enabled != enabled {
				t.Errorf("Enabled = %v, quero %v", r.Enabled, enabled)
			}
			if r.Details == "" {
				t.Error("Details vazio")
			}
		}
	})

	t.Run("DeleteHmacConfig zera Enabled", func(t *testing.T) {
		r, err := storage.NewDeleteHmacConfigUseCase(sg, log).Execute(ctx, txtID)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if r.Enabled {
			t.Error("delete devia devolver Enabled=false")
		}
	})

	t.Run("DeleteS3Config zera Enabled", func(t *testing.T) {
		r, err := storage.NewDeleteS3ConfigUseCase(sg, log).Execute(ctx, txtID)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if r.Enabled {
			t.Error("delete devia devolver Enabled=false")
		}
	})

	t.Run("SetHistory espelha History", func(t *testing.T) {
		r, err := storage.NewSetHistoryUseCase(sg, log).Execute(ctx, txtID, domain.WebhookHistoryRequest{History: 42})
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if r.History != 42 {
			t.Errorf("History = %d, quero 42", r.History)
		}
	})

	t.Run("SetProxy marca Set", func(t *testing.T) {
		r, err := storage.NewSetProxyUseCase(sg, log).Execute(ctx, txtID, domain.ProxyConfigRequest{})
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if !r.Set {
			t.Error("Set = false, quero true")
		}
	})

	t.Run("TestS3Connection marca Connected", func(t *testing.T) {
		r, err := storage.NewTestS3ConnectionUseCase(sg, log).Execute(ctx, txtID, domain.S3TestRequest{
			Endpoint: "https://s3.example", Region: "us-east-1", Bucket: "b", AccessKey: "ak", SecretKey: "sk",
		})
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if !r.Connected {
			t.Error("Connected = false, quero true")
		}
	})

	t.Run("leituras devolvem Details", func(t *testing.T) {
		if r, err := storage.NewGetHmacConfigUseCase(sg, log).Execute(ctx, txtID); err != nil || r.Details == "" {
			t.Errorf("GetHmacConfig = %+v, %v", r, err)
		}
		if r, err := storage.NewGetS3ConfigUseCase(sg, log).Execute(ctx, txtID); err != nil || r.Details == "" {
			t.Errorf("GetS3Config = %+v, %v", r, err)
		}
		if r, err := storage.NewGetHistoryUseCase(sg, log).Execute(ctx, txtID); err != nil || r.Details == "" {
			t.Errorf("GetHistory = %+v, %v", r, err)
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
			r, err := storage.NewConfigureS3UseCase(&contractsfake.SessionGuard{}, log).
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
			r, err := storage.NewConfigureS3UseCase(&contractsfake.SessionGuard{}, &contractsfake.Logger{}).
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

func TestSetProxy_URL(t *testing.T) {
	cases := []struct {
		name    string
		req     domain.ProxyConfigRequest
		wantErr bool
	}{
		{name: "desabilitado ignora a URL invalida", req: domain.ProxyConfigRequest{Enabled: false, URL: "nao-e-uma-url"}},
		{name: "habilitado sem URL e' recusado", req: domain.ProxyConfigRequest{Enabled: true}, wantErr: true},
		{name: "habilitado com loopback e' recusado", req: domain.ProxyConfigRequest{Enabled: true, URL: "http://127.0.0.1:8080"}, wantErr: true},
		{name: "habilitado com malformado e' recusado", req: domain.ProxyConfigRequest{Enabled: true, URL: "://x"}, wantErr: true},
		{name: "habilitado com IP publico passa", req: domain.ProxyConfigRequest{Enabled: true, URL: "http://93.184.216.34:8080"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := storage.NewSetProxyUseCase(&contractsfake.SessionGuard{}, &contractsfake.Logger{}).
				Execute(context.Background(), txtID, tc.req)

			if tc.wantErr {
				if err == nil {
					t.Fatal("proxy invalido devia ser recusado")
				}
				if r != nil {
					t.Error("resultado devia ser nil na recusa")
				}
				return
			}
			if err != nil {
				t.Fatalf("proxy valido recusado: %v", err)
			}
			if !r.Set {
				t.Error("Set = false, quero true")
			}
		})
	}
}

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
			r, err := storage.NewSetHistoryUseCase(&contractsfake.SessionGuard{}, &contractsfake.Logger{}).
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

func TestTestS3Connection_CamposObrigatorios(t *testing.T) {
	completo := domain.S3TestRequest{
		Endpoint: "https://s3.example", Region: "us-east-1", Bucket: "b", AccessKey: "ak", SecretKey: "sk",
	}
	cases := []struct {
		name  string
		mutar func(r *domain.S3TestRequest)
	}{
		{"sem endpoint", func(r *domain.S3TestRequest) { r.Endpoint = "" }},
		{"sem region", func(r *domain.S3TestRequest) { r.Region = "" }},
		{"sem bucket", func(r *domain.S3TestRequest) { r.Bucket = "" }},
		{"sem access_key", func(r *domain.S3TestRequest) { r.AccessKey = "" }},
		{"sem secret_key", func(r *domain.S3TestRequest) { r.SecretKey = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := completo
			tc.mutar(&req)

			log := &contractsfake.Logger{}
			r, err := storage.NewTestS3ConnectionUseCase(&contractsfake.SessionGuard{}, log).
				Execute(context.Background(), txtID, req)

			if err == nil {
				t.Fatal("campo obrigatorio ausente devia ser recusado")
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
