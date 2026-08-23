package bootstrap

import (
	"context"
	"testing"

	"github.com/patrickmn/go-cache"
)

// F200/F201, o ADAPTADOR. O use case já estava travado por três testes, mas
// quem realmente invalida as caches é este adaptador — e estava a 0% de
// cobertura. Um conserto cuja peça central não é exercitada é meio conserto.

func comCaches(t *testing.T) func() {
	t.Helper()
	origUserInfo, origToken := appCtx.UserInfoCache, userinfocache
	appCtx.UserInfoCache = cache.New(cache.NoExpiration, cache.NoExpiration)
	userinfocache = cache.New(cache.NoExpiration, cache.NoExpiration)
	return func() { appCtx.UserInfoCache, userinfocache = origUserInfo, origToken }
}

// TestRepublishUser_ApagaAEntradaPorUserID é o núcleo da F200: a entrada é
// escrita sob NoExpiration e, sem isto, um valor obsoleto ficava para toda a
// vida do processo.
func TestRepublishUser_ApagaAEntradaPorUserID(t *testing.T) {
	defer comCaches(t)()

	valores := Values{M: map[string]string{userInfoIDField: "u1", userInfoHistoryField: "0"}}
	appCtx.UserInfoCache.Set("u1", valores, cache.NoExpiration)

	userInfoRepublisher{db: discardTestDB(t)}.RepublishUser(context.Background(), "u1")

	if _, encontrado := appCtx.UserInfoCache.Get("u1"); encontrado {
		t.Fatal("a entrada por user id sobreviveu: o valor velho continuaria a valer para sempre (F200)")
	}
}

// TestRepublishUser_ApagaTODOSOsTokensDoUtilizador é a F201. A varredura por
// id existe precisamente para apanhar tokens que ninguém passou — incluindo os
// deixados por edições anteriores. Um token revogado que continue na cache
// continua a autenticar.
func TestRepublishUser_ApagaTODOSOsTokensDoUtilizador(t *testing.T) {
	defer comCaches(t)()

	nossos := []string{"tok-antigo", "tok-novo", "tok-mais-antigo-ainda"}
	for _, tok := range nossos {
		userinfocache.Set(tok, Values{M: map[string]string{userInfoIDField: "u1"}}, cache.NoExpiration)
	}
	// De OUTRO utilizador: não pode ser apagado. Uma varredura que apanhe
	// demais desloga terceiros.
	userinfocache.Set("tok-alheio", Values{M: map[string]string{userInfoIDField: "u2"}}, cache.NoExpiration)

	userInfoRepublisher{db: discardTestDB(t)}.RepublishUser(context.Background(), "u1")

	for _, tok := range nossos {
		if _, encontrado := userinfocache.Get(tok); encontrado {
			t.Errorf("token %q sobreviveu: continuaria a autenticar depois de revogado (F201)", tok)
		}
	}
	if _, encontrado := userinfocache.Get("tok-alheio"); !encontrado {
		t.Error("a varredura apagou o token de OUTRO utilizador: desloga terceiros")
	}
}

// TestRepublishUser_EntradaDeTipoInesperadoNaoDerruba: a cache é `any`, e uma
// entrada de outro tipo não pode fazer a varredura entrar em pânico no meio de
// uma edição que o banco já aceitou.
func TestRepublishUser_EntradaDeTipoInesperadoNaoDerruba(t *testing.T) {
	defer comCaches(t)()

	userinfocache.Set("lixo", "isto não é Values", cache.NoExpiration)
	userinfocache.Set("tok", Values{M: map[string]string{userInfoIDField: "u1"}}, cache.NoExpiration)

	userInfoRepublisher{db: discardTestDB(t)}.RepublishUser(context.Background(), "u1")

	if _, encontrado := userinfocache.Get("tok"); encontrado {
		t.Fatal("a entrada de tipo inesperado interrompeu a varredura antes do token real")
	}
}
