// hmac_config_test.go — o contrato dos tres use cases de HMAC por usuario.
//
// Ate' o CAP-27 os tres respondiam 200 e nao tocavam em nada (HOUSEKEEP
// F151/F157). O que estes testes travam nao e' o status: e' o EFEITO —
// a chave chega cifrada ao store, a chave curta nao chega, a falha de cifra
// nao grava, a falha de gravacao nao publica no cache, e a revogacao limpa as
// DUAS metades.
//
// A assercao de CACHE nao e' zelo: appCtx.UserInfoCache e' cache.NoExpiration
// e e' dele que lifecycle_webhook.go:178 tira a chave para assinar cada
// webhook por usuario. Um DELETE que limpasse so' o banco devolveria o MESMO
// 200 e deixaria a chave revogada assinando ate' o processo reiniciar.
package storage_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/storage"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// validHmacKey tem exatamente MinHmacKeyLength caracteres — o menor valor
// ACEITO. Um caso de teste com chave folgada nao mediria a fronteira.
const validHmacKey = "0123456789abcdef0123456789abcdef"

// shortHmacKey tem MinHmacKeyLength-1: o maior valor RECUSADO.
const shortHmacKey = "0123456789abcdef0123456789abcde"

func init() {
	if len(validHmacKey) != domain.MinHmacKeyLength || len(shortHmacKey) != domain.MinHmacKeyLength-1 {
		panic("as fixtures de chave deixaram de cercar domain.MinHmacKeyLength")
	}
}

// newConfigureHmac monta o use case de escrita com dubles zero-value: store em
// memoria, cifrador que prefixa FakeCipherPrefix, cache que grava as chamadas.
func newConfigureHmac(sg *contractsfake.SessionGuard, log *contractsfake.Logger) *storage.ConfigureHmacUseCase {
	return storage.NewConfigureHmacUseCase(sg, &contractsfake.HmacKeyStore{}, &contractsfake.HmacKeyEncryptor{}, &contractsfake.UserInfoHmacCache{}, log)
}

// hmacFixture reune os quatro dubles para que cada teste possa inspecionar as
// tres fronteiras (store, cifrador, cache) depois da chamada.
type hmacFixture struct {
	sessions  *contractsfake.SessionGuard
	store     *contractsfake.HmacKeyStore
	encryptor *contractsfake.HmacKeyEncryptor
	cache     *contractsfake.UserInfoHmacCache
	log       *contractsfake.Logger
}

func newHmacFixture() *hmacFixture {
	return &hmacFixture{
		sessions:  &contractsfake.SessionGuard{},
		store:     &contractsfake.HmacKeyStore{},
		encryptor: &contractsfake.HmacKeyEncryptor{},
		cache:     &contractsfake.UserInfoHmacCache{},
		log:       &contractsfake.Logger{},
	}
}

func (f *hmacFixture) configure() *storage.ConfigureHmacUseCase {
	return storage.NewConfigureHmacUseCase(f.sessions, f.store, f.encryptor, f.cache, f.log)
}

func (f *hmacFixture) get() *storage.GetHmacConfigUseCase {
	return storage.NewGetHmacConfigUseCase(f.sessions, f.store, f.log)
}

func (f *hmacFixture) del() *storage.DeleteHmacConfigUseCase {
	return storage.NewDeleteHmacConfigUseCase(f.sessions, f.store, f.cache, f.log)
}

// logMentionsSecret varre TODOS os registros — mensagem e keyvals — atras da
// chave em claro. Buscar so' num campo deixaria passar o vazamento por outro.
func logMentionsSecret(t *testing.T, log *contractsfake.Logger, secret string) bool {
	t.Helper()
	for _, rec := range log.Records() {
		if strings.Contains(rec.Msg, secret) {
			return true
		}
		for _, kv := range rec.Keyvals {
			if strings.Contains(fmt.Sprintf("%v", kv), secret) {
				return true
			}
		}
	}
	return false
}

// stringify serializa a resposta como ela sai no fio, para que a busca pelo
// segredo cubra a estrutura INTEIRA — inclusive um campo que alguem venha a
// acrescentar depois.
func stringify(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}
	return string(b)
}

// TESTE 1 — o caminho de SUCESSO da escrita: a chave chega ao store CIFRADA
// (o que foi gravado nao e' o texto plano) e o cache recebe o MESMO valor.
func TestConfigureHmac_GravaCifradoEPublicaNoCache(t *testing.T) {
	f := newHmacFixture()

	rsp, err := f.configure().Execute(context.Background(), txtID, domain.HmacConfigRequest{HmacKey: validHmacKey})
	if err != nil {
		t.Fatalf("caminho feliz devolveu erro: %v", err)
	}
	if !rsp.Enabled {
		t.Error("Enabled = false apos gravar, quero true")
	}

	if len(f.store.SaveHmacKeyCalls) != 1 {
		t.Fatalf("SaveHmacKey chamado %d vez(es), quero 1", len(f.store.SaveHmacKeyCalls))
	}
	gravado := f.store.SaveHmacKeyCalls[0].EncryptedKey
	if f.store.SaveHmacKeyCalls[0].UserID != txtID {
		t.Errorf("gravou sob o usuario %q, quero %q", f.store.SaveHmacKeyCalls[0].UserID, txtID)
	}
	if string(gravado) == validHmacKey {
		t.Fatal("a chave foi gravada EM CLARO: o que chegou ao store e' o texto plano do request")
	}
	if !strings.HasPrefix(string(gravado), contractsfake.FakeCipherPrefix) {
		t.Fatalf("o valor gravado nao passou pelo cifrador: %q", gravado)
	}
	if len(f.encryptor.EncryptHmacKeyCalls) != 1 || f.encryptor.EncryptHmacKeyCalls[0].PlainKey != validHmacKey {
		t.Errorf("o cifrador recebeu %+v, quero uma chamada com a chave em claro", f.encryptor.EncryptHmacKeyCalls)
	}

	if len(f.cache.SetHmacKeyCalls) != 1 {
		t.Fatalf("SetHmacKey no cache chamado %d vez(es), quero 1", len(f.cache.SetHmacKeyCalls))
	}
	if got := f.cache.SetHmacKeyCalls[0]; got.UserID != txtID || string(got.EncryptedKey) != string(gravado) {
		t.Errorf("o cache recebeu (%q, %q); quero (%q, %q) — o MESMO valor que foi ao banco",
			got.UserID, got.EncryptedKey, txtID, gravado)
	}
}

// TESTE 2 — chave curta: 400 (CategoryValidation) e NADA gravado. A guarda
// tem de vir ANTES do cifrador, senao o segredo curto atravessa a fronteira
// de cifra a' toa.
func TestConfigureHmac_ChaveCurta_RecusaSemGravar(t *testing.T) {
	f := newHmacFixture()

	rsp, err := f.configure().Execute(context.Background(), txtID, domain.HmacConfigRequest{HmacKey: shortHmacKey})
	if err == nil {
		t.Fatal("chave com menos de 32 caracteres devia ser recusada")
	}
	if rsp != nil {
		t.Error("resultado devia ser nil na recusa")
	}

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Category != apperr.CategoryValidation {
		t.Fatalf("erro = %v; quero apperr com CategoryValidation (400), nao 500", err)
	}
	if !strings.Contains(err.Error(), "at least 32 characters") {
		t.Errorf("mensagem = %q; quero o texto historico do 400", err.Error())
	}
	if len(f.encryptor.EncryptHmacKeyCalls) != 0 {
		t.Error("a chave curta chegou ao cifrador — a validacao esta' depois da cifra")
	}
	if len(f.store.SaveHmacKeyCalls) != 0 {
		t.Fatalf("a recusa gravou no banco: %+v", f.store.SaveHmacKeyCalls)
	}
	if len(f.cache.SetHmacKeyCalls) != 0 {
		t.Fatalf("a recusa tocou o cache: %+v", f.cache.SetHmacKeyCalls)
	}
}

// TESTE 3 — falha de cifra: 500 e NADA gravado. Gravar aqui poria texto claro
// na coluna, que e' o pior resultado possivel desta rota.
func TestConfigureHmac_FalhaDeCifra_NaoGrava(t *testing.T) {
	f := newHmacFixture()
	cifraErr := errors.New("encryption key not configured")
	f.encryptor.EncryptHmacKeyFunc = func(string) ([]byte, error) { return nil, cifraErr }

	rsp, err := f.configure().Execute(context.Background(), txtID, domain.HmacConfigRequest{HmacKey: validHmacKey})
	if err == nil {
		t.Fatal("falha de cifra devia produzir erro")
	}
	if rsp != nil {
		t.Error("resultado devia ser nil na falha")
	}
	if !errors.Is(err, cifraErr) {
		t.Errorf("a causa do cifrador se perdeu: %v", err)
	}
	var appErr *apperr.AppError
	if errors.As(err, &appErr) && appErr.Category == apperr.CategoryValidation {
		t.Error("falha de cifra e' 500, nao 400: o cliente nao tem o que corrigir no payload")
	}
	if len(f.store.SaveHmacKeyCalls) != 0 {
		t.Fatalf("gravou apos a cifra falhar — a coluna receberia texto claro: %+v", f.store.SaveHmacKeyCalls)
	}
	if len(f.cache.SetHmacKeyCalls) != 0 {
		t.Fatalf("publicou no cache apos a cifra falhar: %+v", f.cache.SetHmacKeyCalls)
	}
}

// TESTE 4 — falha de GRAVACAO: 500 e cache NAO TOCADO. E' um teste de ORDEM:
// inverter as duas ultimas chamadas passa em todos os outros testes deste
// arquivo e deixa o cache assinando com uma chave que nao esta' no banco.
func TestConfigureHmac_FalhaDeGravacao_NaoTocaOCache(t *testing.T) {
	f := newHmacFixture()
	gravacaoErr := errors.New("database is read-only")
	f.store.SaveHmacKeyFunc = func(context.Context, string, []byte) error { return gravacaoErr }

	rsp, err := f.configure().Execute(context.Background(), txtID, domain.HmacConfigRequest{HmacKey: validHmacKey})
	if err == nil {
		t.Fatal("falha de gravacao devia produzir erro")
	}
	if rsp != nil {
		t.Error("resultado devia ser nil na falha")
	}
	if !errors.Is(err, gravacaoErr) {
		t.Errorf("a causa do store se perdeu: %v", err)
	}
	if len(f.cache.SetHmacKeyCalls) != 0 {
		t.Fatalf("o cache foi publicado com a gravacao FALHADA: %+v — a chave assinaria sem existir no banco", f.cache.SetHmacKeyCalls)
	}
}

// TESTE 5 e 6 — a leitura devolve PRESENCA, nunca o valor.
func TestGetHmacConfig_MascaraEnaoVazaOSegredo(t *testing.T) {
	t.Run("sem chave devolve vazio", func(t *testing.T) {
		f := newHmacFixture()

		view, err := f.get().Execute(context.Background(), txtID)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if view.HmacKey != "" {
			t.Errorf("hmac_key = %q, quero \"\" quando nao ha' chave", view.HmacKey)
		}
	})

	t.Run("com chave devolve mascara e nao o valor", func(t *testing.T) {
		f := newHmacFixture()
		cifrada := []byte(contractsfake.FakeCipherPrefix + validHmacKey)
		f.store.Stored = map[string][]byte{txtID: cifrada}

		view, err := f.get().Execute(context.Background(), txtID)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if view.HmacKey != domain.MaskedHmacKey {
			t.Fatalf("hmac_key = %q, quero %q", view.HmacKey, domain.MaskedHmacKey)
		}
		// Teste NEGATIVO de vazamento: a busca e' na estrutura INTEIRA, nao
		// so' no campo, para que um campo novo que carregue o segredo tambem
		// caia aqui.
		corpo := stringify(view)
		if strings.Contains(corpo, validHmacKey) {
			t.Fatalf("a chave EM CLARO vazou na resposta: %s", corpo)
		}
		if strings.Contains(corpo, string(cifrada)) {
			t.Fatalf("a forma CIFRADA vazou na resposta: %s", corpo)
		}
	})

	t.Run("falha de leitura propaga a causa", func(t *testing.T) {
		f := newHmacFixture()
		leituraErr := errors.New("connection refused")
		f.store.LoadHmacKeyFunc = func(context.Context, string) ([]byte, error) { return nil, leituraErr }

		view, err := f.get().Execute(context.Background(), txtID)
		if err == nil {
			t.Fatal("falha de leitura devia produzir erro")
		}
		if view != nil {
			t.Error("resultado devia ser nil na falha")
		}
		if !errors.Is(err, leituraErr) {
			t.Errorf("a causa do store se perdeu: %v", err)
		}
	})
}

// TESTE 7 — a revogacao limpa banco E cache. A metade do cache e' a que o
// sintoma esconde: sem ela o 200 e' identico e a chave continua assinando.
func TestDeleteHmacConfig_RevogaBancoECache(t *testing.T) {
	f := newHmacFixture()
	f.store.Stored = map[string][]byte{txtID: []byte(contractsfake.FakeCipherPrefix + validHmacKey)}

	rsp, err := f.del().Execute(context.Background(), txtID)
	if err != nil {
		t.Fatalf("revogacao devolveu erro: %v", err)
	}
	if rsp.Enabled {
		t.Error("Enabled = true apos revogar")
	}

	if len(f.store.DeleteHmacKeyCalls) != 1 || f.store.DeleteHmacKeyCalls[0].UserID != txtID {
		t.Fatalf("DeleteHmacKey = %+v, quero uma chamada para %q", f.store.DeleteHmacKeyCalls, txtID)
	}
	if got := f.store.Stored[txtID]; len(got) != 0 {
		t.Fatalf("a chave continua no store apos a revogacao: %q", got)
	}

	if len(f.cache.SetHmacKeyCalls) != 1 {
		t.Fatalf("o CACHE nao foi limpo (%d chamadas): a chave revogada continua assinando os webhooks do usuario ate' o processo reiniciar",
			len(f.cache.SetHmacKeyCalls))
	}
	if got := f.cache.SetHmacKeyCalls[0]; got.UserID != txtID || len(got.EncryptedKey) != 0 {
		t.Fatalf("o cache recebeu (%q, %q); quero (%q, vazio) — chave nao-vazia aqui e' revogacao que nao revoga",
			got.UserID, got.EncryptedKey, txtID)
	}
}

// TESTE 8 — falha de banco na revogacao: 500 e cache NAO limpo. Limpar o
// cache aqui seria revogacao pela metade: pareceria funcionar ate' o restart
// trazer a chave de volta do banco, sem nenhum sinal de que voltou.
func TestDeleteHmacConfig_FalhaDeBanco_NaoLimpaOCache(t *testing.T) {
	f := newHmacFixture()
	delErr := errors.New("database is locked")
	f.store.DeleteHmacKeyFunc = func(context.Context, string) error { return delErr }

	rsp, err := f.del().Execute(context.Background(), txtID)
	if err == nil {
		t.Fatal("falha de banco devia produzir erro")
	}
	if rsp != nil {
		t.Error("resultado devia ser nil na falha")
	}
	if !errors.Is(err, delErr) {
		t.Errorf("a causa do store se perdeu: %v", err)
	}
	if len(f.cache.SetHmacKeyCalls) != 0 {
		t.Fatalf("o cache foi limpo com a revogacao FALHADA: %+v — o restart traria a chave de volta em silencio", f.cache.SetHmacKeyCalls)
	}
}

// TESTE 9 — o segredo nunca entra em log, em nenhum dos tres caminhos, nem
// quando ha' erro. Os caminhos de ERRO sao os que mais logam, entao sao os
// que mais podem vazar.
func TestHmacUseCases_SegredoNuncaVaiParaOLog(t *testing.T) {
	cases := []struct {
		name   string
		secret string
		run    func(f *hmacFixture) error
	}{
		{"configure/sucesso", validHmacKey, func(f *hmacFixture) error {
			_, err := f.configure().Execute(context.Background(), txtID, domain.HmacConfigRequest{HmacKey: validHmacKey})
			return err
		}},
		{"configure/chave curta", shortHmacKey, func(f *hmacFixture) error {
			_, err := f.configure().Execute(context.Background(), txtID, domain.HmacConfigRequest{HmacKey: shortHmacKey})
			return err
		}},
		{"configure/falha de cifra", validHmacKey, func(f *hmacFixture) error {
			f.encryptor.EncryptHmacKeyFunc = func(k string) ([]byte, error) {
				// O cifrador real NUNCA poe o texto plano no erro
				// (pkg/infra/auth/hmac.go:38-67 loga so' tamanhos); o duble
				// imita a regra REAL.
				return nil, errors.New("failed to create cipher")
			}
			_, err := f.configure().Execute(context.Background(), txtID, domain.HmacConfigRequest{HmacKey: validHmacKey})
			return err
		}},
		{"configure/falha de gravacao", validHmacKey, func(f *hmacFixture) error {
			f.store.SaveHmacKeyFunc = func(context.Context, string, []byte) error { return errors.New("write failed") }
			_, err := f.configure().Execute(context.Background(), txtID, domain.HmacConfigRequest{HmacKey: validHmacKey})
			return err
		}},
		{"get/com chave", validHmacKey, func(f *hmacFixture) error {
			f.store.Stored = map[string][]byte{txtID: []byte(contractsfake.FakeCipherPrefix + validHmacKey)}
			_, err := f.get().Execute(context.Background(), txtID)
			return err
		}},
		{"get/falha de leitura", validHmacKey, func(f *hmacFixture) error {
			f.store.LoadHmacKeyFunc = func(context.Context, string) ([]byte, error) { return nil, errors.New("read failed") }
			_, err := f.get().Execute(context.Background(), txtID)
			return err
		}},
		{"delete/sucesso", validHmacKey, func(f *hmacFixture) error {
			f.store.Stored = map[string][]byte{txtID: []byte(contractsfake.FakeCipherPrefix + validHmacKey)}
			_, err := f.del().Execute(context.Background(), txtID)
			return err
		}},
		{"delete/falha de banco", validHmacKey, func(f *hmacFixture) error {
			f.store.DeleteHmacKeyFunc = func(context.Context, string) error { return errors.New("delete failed") }
			_, err := f.del().Execute(context.Background(), txtID)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newHmacFixture()

			err := tc.run(f)

			if len(f.log.Records()) == 0 {
				t.Fatal("o caminho nao logou nada — a checagem de vazamento fica vacua")
			}
			if logMentionsSecret(t, f.log, tc.secret) {
				t.Fatalf("a chave em claro foi para o log: %v", f.log.Records())
			}
			if err != nil && strings.Contains(err.Error(), tc.secret) {
				t.Fatalf("a chave em claro foi para a mensagem de erro: %v", err)
			}
		})
	}
}
