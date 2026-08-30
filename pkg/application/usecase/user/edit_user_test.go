package user_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

func intPtr(v int) *int { return &v }

func TestEditUserUseCase_Execute_Rejections(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	tests := []struct {
		name       string
		req        domain.EditUserInput
		existsFunc func(ctx context.Context, id string) (bool, error)
		wantIs     error
		wantUpdate bool
	}{
		{
			name: "id vazio",
			req:  domain.EditUserInput{},
		},
		{
			name:       "erro ao consultar existência",
			req:        domain.EditUserInput{UserID: "u1"},
			existsFunc: func(context.Context, string) (bool, error) { return false, boom },
			wantIs:     boom,
		},
		{
			name:       "usuário inexistente",
			req:        domain.EditUserInput{UserID: "u1"},
			existsFunc: func(context.Context, string) (bool, error) { return false, nil },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{UserExistsFunc: tt.existsFunc}
			uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{}, &contractsfake.UserInfoRepublisher{}, &contractsfake.Logger{})

			err := uc.Execute(context.Background(), tt.req)
			if err == nil {
				t.Fatal("esperava erro")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("err = %v, queria embrulhar %v", err, tt.wantIs)
			}
			if len(repo.UpdateUserCalls) != 0 {
				t.Errorf("UpdateUser chamado %d vezes, queria 0", len(repo.UpdateUserCalls))
			}
		})
	}
}

func TestEditUserUseCase_Execute_UpdateErrors(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	tests := []struct {
		name      string
		updateErr error
		wantIs    error
		wantLog   bool
	}{
		{
			name:      "token duplicado sobe com a identidade original",
			updateErr: user.ErrDuplicateToken,
			wantIs:    user.ErrDuplicateToken,
		},
		{
			// O nome era "sobe sem embrulho" e passou a mentir com a F206: o
			// erro AGORA é embrulhado em apperr, e é essa a correção. A causa
			// continua alcançável por errors.Is — é isso que este caso trava.
			name:      "nada a atualizar mantém a causa alcançável",
			updateErr: domain.ErrNoFieldsToUpdate,
			wantIs:    domain.ErrNoFieldsToUpdate,
		},
		{
			name:      "erro genérico vira erro de banco logado",
			updateErr: boom,
			wantIs:    boom,
			wantLog:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{
				UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
				UpdateUserFunc: func(context.Context, string, domain.UserUpdate) error { return tt.updateErr },
			}
			logger := &contractsfake.Logger{}
			uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{}, &contractsfake.UserInfoRepublisher{}, logger)

			err := uc.Execute(context.Background(), domain.EditUserInput{UserID: "u1", Name: "novo"})
			if !errors.Is(err, tt.wantIs) {
				t.Fatalf("err = %v, queria %v", err, tt.wantIs)
			}
			if got := logger.Logged("Failed to update user"); got != tt.wantLog {
				t.Errorf("log de falha = %v, queria %v", got, tt.wantLog)
			}
		})
	}
}

func TestEditUserUseCase_Execute_InvalidEventEntriesAreSkipped(t *testing.T) {
	t.Parallel()

	repo := &contractsfake.UserRepository{
		UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
	}
	uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{}, &contractsfake.UserInfoRepublisher{}, &contractsfake.Logger{})

	if err := uc.Execute(context.Background(), domain.EditUserInput{UserID: "u1", Events: "Message,, ,ReadReceipt"}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(repo.UpdateUserCalls) != 1 {
		t.Fatalf("UpdateUser chamado %d vezes, queria 1", len(repo.UpdateUserCalls))
	}
}

func TestEditUserUseCase_Execute_BuildsPartialUpdate(t *testing.T) {
	t.Parallel()

	useProxy := true
	tests := []struct {
		name   string
		req    domain.EditUserInput
		assert func(t *testing.T, upd domain.UserUpdate)
	}{
		{
			name: "todos os campos escalares",
			req: domain.EditUserInput{
				UserID: "u1", Name: "n", Token: "t", Webhook: "http://w",
				Expiration: 10, Events: "Message", History: intPtr(5),
			},
			assert: func(t *testing.T, upd domain.UserUpdate) {
				t.Helper()
				for name, p := range map[string]bool{
					"Name": upd.Name == nil, "Token": upd.Token == nil,
					"Webhook": upd.Webhook == nil, "Expiration": upd.Expiration == nil,
					"Events": upd.Events == nil, "History": upd.History == nil,
				} {
					if p {
						t.Errorf("%s = nil, queria informado", name)
					}
				}
				if upd.ProxyURL != nil || upd.S3 != nil {
					t.Error("ProxyURL/S3 informados sem estarem no request")
				}
			},
		},
		{
			name: "proxy habilitado propaga a URL",
			req: domain.EditUserInput{
				UserID:      "u1",
				ProxyConfig: &domain.ProxyConfig{Enabled: true, ProxyURL: "http://proxy:8080", WebhookUseProxy: &useProxy},
			},
			assert: func(t *testing.T, upd domain.UserUpdate) {
				t.Helper()
				if upd.ProxyURL == nil || *upd.ProxyURL != "http://proxy:8080" {
					t.Errorf("ProxyURL = %v, queria a URL do request", upd.ProxyURL)
				}
				if upd.WebhookUseProxy == nil || !*upd.WebhookUseProxy {
					t.Error("WebhookUseProxy não propagado")
				}
			},
		},
		{
			name: "proxy desabilitado zera a URL",
			req: domain.EditUserInput{
				UserID:      "u1",
				ProxyConfig: &domain.ProxyConfig{Enabled: false, ProxyURL: "http://proxy:8080"},
			},
			assert: func(t *testing.T, upd domain.UserUpdate) {
				t.Helper()
				if upd.ProxyURL == nil || *upd.ProxyURL != "" {
					t.Errorf("ProxyURL = %v, queria string vazia", upd.ProxyURL)
				}
			},
		},
		{
			name: "s3 habilitado with secret enveloped (F163)",
			req: domain.EditUserInput{
				UserID:   "u1",
				S3Config: &domain.S3Config{Enabled: true, Bucket: "b", Region: "r", SecretKey: "my-s3-secret"},
			},
			assert: func(t *testing.T, upd domain.UserUpdate) {
				t.Helper()
				if upd.S3 == nil || !upd.S3.Enabled {
					t.Errorf("S3 = %v, want enabled", upd.S3)
				}
				if upd.S3.SecretKey == "my-s3-secret" {
					t.Error("S3 SecretKey stored as PLAINTEXT")
				}
				want := contractsfake.FakeS3EnvelopePrefix + "my-s3-secret"
				if upd.S3.SecretKey != want {
					t.Errorf("S3 SecretKey = %q, want %q", upd.S3.SecretKey, want)
				}
			},
		},
		{
			name: "s3 desabilitado remove o cliente",
			req: domain.EditUserInput{
				UserID:   "u1",
				S3Config: &domain.S3Config{Enabled: false},
			},
			assert: func(t *testing.T, upd domain.UserUpdate) {
				t.Helper()
				if upd.S3 == nil || upd.S3.Enabled {
					t.Errorf("S3 = %v, queria desabilitado", upd.S3)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{
				UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
			}
			uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{}, &contractsfake.UserInfoRepublisher{}, &contractsfake.Logger{})

			if err := uc.Execute(context.Background(), tt.req); err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(repo.UpdateUserCalls) != 1 {
				t.Fatalf("UpdateUser chamado %d vezes, queria 1", len(repo.UpdateUserCalls))
			}
			call := repo.UpdateUserCalls[0]
			if call.ID != tt.req.UserID {
				t.Errorf("id = %q, queria %q", call.ID, tt.req.UserID)
			}
			tt.assert(t, call.Update)
		})
	}
}

// TestEditUserUseCase_Execute_EventoInvalidoNaoChegaAoRepositorio cobre o
// SEGUNDO chamador de isValidEvent (edit_user.go:49).
//
// A recusa da F159 não é só de POST /admin/users: `isValidEvent` é do pacote,
// e PUT /admin/users/{id} passa pela mesma guarda. Sem este teste, metade da
// mudança de contrato ficaria sem trava.
func TestEditUserUseCase_Execute_EventoInvalidoNaoChegaAoRepositorio(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		events string
	}{
		{name: "evento com typo", events: "Mesage"},
		{name: "lista mista, um válido e um inválido", events: "Message,Mesage"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{
				UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
			}
			uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{}, &contractsfake.UserInfoRepublisher{}, &contractsfake.Logger{})

			err := uc.Execute(context.Background(),
				domain.EditUserInput{UserID: "u1", Events: tt.events})
			if err == nil {
				t.Fatal("esperava recusa por tipo de evento desconhecido")
			}
			// Recusar, não filtrar: nada é atualizado. Um filtro silencioso
			// gravaria a lista podada e devolveria sucesso.
			if len(repo.UpdateUserCalls) != 0 {
				t.Errorf("UpdateUser chamado %d vezes, queria 0", len(repo.UpdateUserCalls))
			}
		})
	}
}

// TestEditUserUseCase_Execute_EventosValidosChegamIntactos é o caminho de
// SUCESSO da mesma guarda: o valor gravado é o do request, sem poda.
func TestEditUserUseCase_Execute_EventosValidosChegamIntactos(t *testing.T) {
	t.Parallel()

	for _, events := range []string{"Message", "Message,ReadReceipt,Presence", "All", "  Message  "} {
		t.Run(events, func(t *testing.T) {
			t.Parallel()
			repo := &contractsfake.UserRepository{
				UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
			}
			uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{}, &contractsfake.UserInfoRepublisher{}, &contractsfake.Logger{})

			if err := uc.Execute(context.Background(),
				domain.EditUserInput{UserID: "u1", Events: events}); err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if len(repo.UpdateUserCalls) != 1 {
				t.Fatalf("UpdateUser chamado %d vezes, queria 1", len(repo.UpdateUserCalls))
			}
			upd := repo.UpdateUserCalls[0].Update
			if upd.Events == nil {
				t.Fatal("Events = nil, queria informado")
			}
			if *upd.Events != events {
				t.Errorf("Events = %q, queria %q", *upd.Events, events)
			}
		})
	}
}

// TestEditUserUseCase_Execute_S3CifraFalhaNaoGravaUsuario locks the ORDER
// contract for the S3 secret key on the EDIT path (F163): encrypt BEFORE
// write; a cipher failure must not update the user row.
func TestEditUserUseCase_Execute_S3CifraFalhaNaoGravaUsuario(t *testing.T) {
	t.Parallel()

	boom := errors.New("s3 encryption key not configured")
	repo := &contractsfake.UserRepository{
		UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
	}
	s3Cipher := &contractsfake.S3SecretCipher{
		EncryptS3SecretFunc: func(string) (string, error) { return "", boom },
	}
	logger := &contractsfake.Logger{}
	uc := user.NewEditUserUseCase(repo, s3Cipher, &contractsfake.UserInfoRepublisher{}, logger)

	err := uc.Execute(context.Background(), domain.EditUserInput{
		UserID:   "u1",
		S3Config: &domain.S3Config{Enabled: true, SecretKey: "my-s3-secret"},
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want to wrap boom", err)
	}
	if len(repo.UpdateUserCalls) != 0 {
		t.Errorf("UpdateUser called %d times, want 0", len(repo.UpdateUserCalls))
	}
}

// --- F200 / F201: a edição tem de republicar a cache -------------------------

// TestEditUser_RepublicaAposEscritaBemSucedida trava a CAUSA da F200: a edição
// entrava no banco e o processo continuava a ler o valor velho da cache, que é
// escrita sob NoExpiration e nunca expira. Medido contra o servidor real: sete
// sondas ao longo de 413s com `history=30` no banco não gravaram nada, e um
// reinício resolveu — ver HOUSEKEEP F200.
func TestEditUser_RepublicaAposEscritaBemSucedida(t *testing.T) {
	repo := &contractsfake.UserRepository{
		UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
	}
	rep := &contractsfake.UserInfoRepublisher{}

	uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{}, rep, &contractsfake.Logger{})
	if err := uc.Execute(context.Background(), domain.EditUserInput{
		UserID: "u1", History: intPtr(30),
	}); err != nil {
		t.Fatalf("Execute = %v", err)
	}

	if len(rep.RepublishCalls) != 1 {
		t.Fatalf("RepublishUser chamado %d vez(es), quero 1 — a edição entra no banco e o processo nunca a vê", len(rep.RepublishCalls))
	}
	if got := rep.RepublishCalls[0].UserID; got != "u1" {
		t.Fatalf("republicou %q, quero \"u1\"", got)
	}
}

// TestEditUser_NaoRepublicaQuandoAEscritaFalha é o outro lado: republicar uma
// escrita que falhou publicaria na cache um valor que o banco NÃO tem — e,
// como a entrada por user id não expira, esse valor errado ficaria para sempre.
func TestEditUser_NaoRepublicaQuandoAEscritaFalha(t *testing.T) {
	repo := &contractsfake.UserRepository{
		UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
		UpdateUserFunc: func(context.Context, string, domain.UserUpdate) error {
			return errors.New("banco fora")
		},
	}
	rep := &contractsfake.UserInfoRepublisher{}

	uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{}, rep, &contractsfake.Logger{})
	if err := uc.Execute(context.Background(), domain.EditUserInput{UserID: "u1", History: intPtr(30)}); err == nil {
		t.Fatal("Execute devolveu nil apesar de a escrita ter falhado")
	}

	if len(rep.RepublishCalls) != 0 {
		t.Fatalf("republicou %d vez(es) depois de a escrita falhar — a cache passaria a ter um valor que o banco não tem", len(rep.RepublishCalls))
	}
}

// TestEditUser_RepublicaDEPOISDaEscritaENaoAntes trava a ORDEM. Inverter as
// duas chamadas passa em todos os outros testes deste ficheiro: ambos os
// caminhos continuariam a chamar as duas coisas uma vez. O que distingue é
// QUANDO — por isso o dublê conta as escritas já feitas no instante em que é
// chamado.
func TestEditUser_RepublicaDEPOISDaEscritaENaoAntes(t *testing.T) {
	repo := &contractsfake.UserRepository{
		UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
	}
	rep := &contractsfake.UserInfoRepublisher{
		ContadorDeEscritas: func() int { return len(repo.UpdateUserCalls) },
	}

	uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{}, rep, &contractsfake.Logger{})
	if err := uc.Execute(context.Background(), domain.EditUserInput{UserID: "u1", History: intPtr(30)}); err != nil {
		t.Fatalf("Execute = %v", err)
	}

	if len(rep.EscritasAoSerChamado) != 1 {
		t.Fatalf("republicador chamado %d vez(es)", len(rep.EscritasAoSerChamado))
	}
	if rep.EscritasAoSerChamado[0] != 1 {
		t.Fatalf("republicou com %d escritas feitas, quero 1 — a republicação está ANTES da escrita",
			rep.EscritasAoSerChamado[0])
	}
}

// TestEditUser_SemCamposEhErroDoCliente trava a F206 (decisão 48=a do canal):
// `token:""` significa CAMPO NÃO INFORMADO, e um pedido sem nenhum campo útil
// é inválido — não avaria nossa.
//
// Medido em campo a 2026-08-22, antes da correção:
//
//	PUT /admin/users/{id} {"token":""}  -> 500 {"error":"internal server error"}
//	PUT /admin/users/{id} {}           -> 500      <- o mesmo, pelo caminho mais banal
//
// 500 diz "avaria nossa, tente outra vez" para algo determinístico: por mais
// que o cliente repita, a resposta nunca muda. Mesma família da F182 e da F204.
func TestEditUser_SemCamposEhErroDoCliente(t *testing.T) {
	t.Parallel()

	for _, req := range []domain.EditUserInput{
		{UserID: "u1"},            // corpo vazio
		{UserID: "u1", Token: ""}, // o caso da entrada
		{UserID: "u1", Name: "", Webhook: ""},
	} {
		repo := &contractsfake.UserRepository{
			UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
			UpdateUserFunc: func(context.Context, string, domain.UserUpdate) error {
				return domain.ErrNoFieldsToUpdate
			},
		}
		uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{},
			&contractsfake.UserInfoRepublisher{}, &contractsfake.Logger{})

		err := uc.Execute(context.Background(), req)

		var app *apperr.AppError
		if !errors.As(err, &app) {
			t.Fatalf("%+v: erro sem taxonomia (%T) — sobe cru e o RespondJSON cai no "+
				"ramo genérico, devolvendo 500 para erro do cliente", req, err)
		}
		if app.Category != apperr.CategoryValidation {
			t.Errorf("%+v: categoria = %q, quero validation (400)", req, app.Category)
		}
		if app.Code != "no_fields_to_update" {
			t.Errorf("%+v: code = %q; o cliente precisa de um código legível por "+
				"máquina, não de \"internal server error\"", req, app.Code)
		}
		if app.Retryable {
			t.Errorf("%+v: marcado retryable; repetir o mesmo pedido dá o mesmo erro", req)
		}
		// A causa tem de continuar alcançável, senão o log perde o motivo.
		if !errors.Is(err, domain.ErrNoFieldsToUpdate) {
			t.Errorf("%+v: a causa deixou de ser alcançável por errors.Is", req)
		}
	}
}

// TestEditUser_S3ConfigEhPersistido trava comportamento que JÁ EXISTIA e não
// tinha teste nenhum.
//
// Nasceu de um diagnóstico ERRADO da F210: eu tinha escrito que o use case
// nunca preenchia `upd.S3`, e cheguei a acrescentar um bloco a fazer o que
// `edit_user.go:95-109` já fazia. Quem apanhou a redundância foi o controlo
// negativo — removi a minha atribuição e o teste ficou VERDE, porque a linha
// 108 já a fazia. O bloco duplicado foi revertido; o teste ficou, porque a
// propriedade é real e estava sem cobertura.
//
// O defeito verdadeiro da F210 é outro e não se vê daqui: o PUT LÊ `s3Config`
// (camelCase) e a resposta DEVOLVE `s3_config` (snake_case), então o ciclo
// ler-editar-reenviar é ignorado em silêncio com 200. Isso só aparece num
// teste de round-trip pela rota, não neste nível.
func TestEditUser_S3ConfigEhPersistido(t *testing.T) {
	t.Parallel()

	var recebido domain.UserUpdate
	repo := &contractsfake.UserRepository{
		UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
		UpdateUserFunc: func(_ context.Context, _ string, upd domain.UserUpdate) error {
			recebido = upd
			return nil
		},
	}
	uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{},
		&contractsfake.UserInfoRepublisher{}, &contractsfake.Logger{})

	err := uc.Execute(context.Background(), domain.EditUserInput{
		UserID: "u1",
		S3Config: &domain.S3Config{
			Enabled: true, Bucket: "meu-balde", Endpoint: "http://minio:9000",
			Region: "us-east-1", AccessKey: "AK", SecretKey: "segredo-em-claro",
		},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if recebido.S3 == nil {
		t.Fatal("o repositório não recebeu S3: a configuração não é persistida, " +
			"morre no reinício e nunca aparece na listagem (F210)")
	}
	if recebido.S3.Bucket != "meu-balde" || !recebido.S3.Enabled {
		t.Errorf("S3 persistido incompleto: %+v", *recebido.S3)
	}

	// A parte que um teste de persistência sozinho não pega, e que é onde a
	// correção podia dar errado em silêncio: o segredo tem de ir CIFRADO.
	// Gravar em claro passaria em todas as outras asserções deste teste.
	if recebido.S3.SecretKey == "segredo-em-claro" {
		t.Error("o segredo S3 foi persistido EM CLARO — o AddUser cifra (F163), " +
			"e o EditUser tem de cifrar também")
	}
	if !strings.HasPrefix(recebido.S3.SecretKey, contractsfake.FakeS3EnvelopePrefix) {
		t.Errorf("o segredo não passou pela cifra: %q", recebido.S3.SecretKey)
	}
}

// --- F218: history=0 tem de chegar ao repositório como zero ---------------------

// TestEditUser_HistoryZeroChegaAoRepositorio é a F218: com o tipo antigo (int
// + omitempty), `{"history":0}` era indistinguível de "não mencionou" e o
// pedido era recusado com no_fields_to_update. Com *int, 0 é valor válido.
func TestEditUser_HistoryZeroChegaAoRepositorio(t *testing.T) {
	t.Parallel()

	repo := &contractsfake.UserRepository{
		UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
	}
	uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{}, &contractsfake.UserInfoRepublisher{}, &contractsfake.Logger{})

	if err := uc.Execute(context.Background(), domain.EditUserInput{
		UserID: "u1", History: intPtr(0),
	}); err != nil {
		t.Fatalf("Execute = %v — history=0 was refused; the old int+omitempty bug is back", err)
	}

	if len(repo.UpdateUserCalls) != 1 {
		t.Fatalf("UpdateUser called %d times, want 1", len(repo.UpdateUserCalls))
	}
	upd := repo.UpdateUserCalls[0].Update
	if upd.History == nil {
		t.Fatal("History = nil — zero was treated as absent, the F218 bug")
	}
	if *upd.History != 0 {
		t.Fatalf("History = %d, want 0", *upd.History)
	}
}

// TestEditUser_HistoryOmitidoNaoToca is the other side of F218: a request that
// does NOT mention history must leave it alone (nil in the update).
func TestEditUser_HistoryOmitidoNaoToca(t *testing.T) {
	t.Parallel()

	repo := &contractsfake.UserRepository{
		UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
	}
	uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{}, &contractsfake.UserInfoRepublisher{}, &contractsfake.Logger{})

	if err := uc.Execute(context.Background(), domain.EditUserInput{
		UserID: "u1", Name: "only-name",
	}); err != nil {
		t.Fatalf("Execute = %v", err)
	}

	if len(repo.UpdateUserCalls) != 1 {
		t.Fatalf("UpdateUser called %d times, want 1", len(repo.UpdateUserCalls))
	}
	upd := repo.UpdateUserCalls[0].Update
	if upd.History != nil {
		t.Errorf("History = %d, want nil — the request did not mention history", *upd.History)
	}
}

// E o limite: um PUT SEM `s3_config` não pode inventar um, senão apagaria a
// configuração existente de quem só queria mudar o nome.
func TestEditUser_SemS3ConfigNaoTocaNoS3(t *testing.T) {
	t.Parallel()

	var recebido domain.UserUpdate
	repo := &contractsfake.UserRepository{
		UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
		UpdateUserFunc: func(_ context.Context, _ string, upd domain.UserUpdate) error {
			recebido = upd
			return nil
		},
	}
	uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{},
		&contractsfake.UserInfoRepublisher{}, &contractsfake.Logger{})

	if err := uc.Execute(context.Background(), domain.EditUserInput{
		UserID: "u1", Name: "so-o-nome",
	}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if recebido.S3 != nil {
		t.Errorf("S3 = %+v num pedido que não o traz: isto sobrescreveria a "+
			"configuração de quem só queria mudar o nome", *recebido.S3)
	}
}

// TestEditUser_EngineImutavel trava os itens 8/61: o engine não pode ser
// alterado depois da criação. Divergir do valor persistido é recusado com
// engine_immutable (409, CategoryConflict) e o UPDATE NUNCA chega ao
// repositório — não é só o código HTTP que importa, é a garantia de que o
// banco não muda.
func TestEditUser_EngineImutavel(t *testing.T) {
	t.Parallel()

	novoEngine := "headless"
	repo := &contractsfake.UserRepository{
		UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{{ID: "u1", Engine: domain.EngineNoise}}, nil
		},
	}
	uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{},
		&contractsfake.UserInfoRepublisher{}, &contractsfake.Logger{})

	err := uc.Execute(context.Background(), domain.EditUserInput{
		UserID: "u1", Engine: &novoEngine,
	})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("err = %v, queria *apperr.AppError", err)
	}
	if appErr.Code != "engine_immutable" {
		t.Errorf("code = %q, queria %q", appErr.Code, "engine_immutable")
	}
	if appErr.Category != apperr.CategoryConflict {
		t.Errorf("category = %q, queria %q (409)", appErr.Category, apperr.CategoryConflict)
	}
	if len(repo.UpdateUserCalls) != 0 {
		t.Errorf("UpdateUser chamado %d vezes, queria 0 — engine divergente não pode tocar o banco", len(repo.UpdateUserCalls))
	}
}

// TestEditUser_EngineIgualAoPersistidoEhNoop é o controle POSITIVO: reenviar
// o MESMO engine já gravado (edição idempotente que reenvia o próprio
// estado) não é "divergir" e não é recusado.
func TestEditUser_EngineIgualAoPersistidoEhNoop(t *testing.T) {
	t.Parallel()

	mesmoEngine := "noise"
	repo := &contractsfake.UserRepository{
		UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{{ID: "u1", Engine: domain.EngineNoise}}, nil
		},
		UpdateUserFunc: func(context.Context, string, domain.UserUpdate) error { return nil },
	}
	uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{},
		&contractsfake.UserInfoRepublisher{}, &contractsfake.Logger{})

	err := uc.Execute(context.Background(), domain.EditUserInput{
		UserID: "u1", Name: "novo-nome", Engine: &mesmoEngine,
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(repo.UpdateUserCalls) != 1 {
		t.Fatalf("UpdateUser chamado %d vezes, queria 1", len(repo.UpdateUserCalls))
	}
	if repo.UpdateUserCalls[0].Update.Engine != nil {
		t.Errorf("Update.Engine = %v, queria nil — reenviar o mesmo valor não deve virar um SET no adapter", *repo.UpdateUserCalls[0].Update.Engine)
	}
}

// TestEditUser_SemEngineNoBodyNaoConsultaEngine é o controle de que um PUT
// que não menciona engine (o caso comum) não paga o custo extra de
// ListUsers, e não pode ser recusado por causa de um campo que não veio.
func TestEditUser_SemEngineNoBodyNaoConsultaEngine(t *testing.T) {
	t.Parallel()

	repo := &contractsfake.UserRepository{
		UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			t.Fatal("ListUsers não deveria ser chamado quando engine não veio no corpo")
			return nil, nil
		},
	}
	uc := user.NewEditUserUseCase(repo, &contractsfake.S3SecretCipher{},
		&contractsfake.UserInfoRepublisher{}, &contractsfake.Logger{})

	if err := uc.Execute(context.Background(), domain.EditUserInput{
		UserID: "u1", Name: "novo-nome",
	}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
}
