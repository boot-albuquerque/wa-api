package auth_test

import (
	"testing"
	"time"

	"wa-api/pkg/infra/auth"

	"github.com/patrickmn/go-cache"
)

// TestNewTokenCache_ZeroTTLMustNotProduceNeverExpiringCache é o teste que mata
// o mutante sobrevivente da F8.
//
// O mutante: trocar `if defaultTTL <= 0` por `if defaultTTL < 0` em
// NewTokenCache. Com essa troca, `defaultTTL == 0` deixa de ser normalizado e
// chega intacto em cache.New. E go-cache (newCache, cache.go:1102) faz
// `if de == 0 { de = -1 }` — ou seja, -1 == NoExpiration.
//
// O resultado é um cache de tokens cujas entradas NUNCA expiram: o userinfo de
// um token revogado, deletado ou rotacionado fica autenticado para sempre,
// porque o caminho de auth resolve token -> userinfo pelo cache antes de bater
// no banco. É bypass de autenticação, e a suíte anterior não tinha um único
// teste tocando NewTokenCache (0% de cobertura), então o mutante passava batido.
//
// A asserção é sobre Item.Expiration: com TTL normalizado o item tem deadline
// (Expiration != 0); sob o mutante o item é NoExpiration (Expiration == 0).
func TestNewTokenCache_ZeroTTLMustNotProduceNeverExpiringCache(t *testing.T) {
	c := auth.NewTokenCache(0, 0)
	c.Set("tok", "userinfo", cache.DefaultExpiration)

	item, ok := c.Items()["tok"]
	if !ok {
		t.Fatal("entrada gravada nao esta no cache")
	}
	if item.Expiration == 0 {
		t.Fatal("BYPASS: NewTokenCache(0, 0) produziu cache NoExpiration — " +
			"um token revogado cacheado aqui continuaria autenticando para sempre")
	}

	remaining := time.Until(time.Unix(0, item.Expiration))
	if remaining <= 0 || remaining > auth.DefaultTTL {
		t.Fatalf("TTL aplicado fora do contrato: restante %v, DefaultTTL %v", remaining, auth.DefaultTTL)
	}
}

// TestNewTokenCache_NegativeTTLMustNotProduceNeverExpiringCache cobre o outro
// lado da mesma fronteira: NoExpiration é literalmente -1 em go-cache, então
// repassar um valor negativo é o mesmo bypass por outra porta.
func TestNewTokenCache_NegativeTTLMustNotProduceNeverExpiringCache(t *testing.T) {
	for _, ttl := range []time.Duration{-1, -time.Hour} {
		c := auth.NewTokenCache(ttl, -1)
		c.Set("tok", "userinfo", cache.DefaultExpiration)

		item, ok := c.Items()["tok"]
		if !ok {
			t.Fatalf("ttl=%v: entrada gravada nao esta no cache", ttl)
		}
		if item.Expiration == 0 {
			t.Fatalf("BYPASS: NewTokenCache(%v, -1) produziu cache NoExpiration", ttl)
		}
	}
}

// TestNewTokenCache_PositiveTTLIsHonored garante que a normalização só age no
// caso anômalo: um TTL positivo explícito não pode ser sobrescrito pelo padrão,
// senão a configuração de expiração do operador vira decorativa.
func TestNewTokenCache_PositiveTTLIsHonored(t *testing.T) {
	const ttl = 40 * time.Millisecond
	c := auth.NewTokenCache(ttl, time.Minute)
	c.Set("tok", "userinfo", cache.DefaultExpiration)

	if _, found := c.Get("tok"); !found {
		t.Fatal("entrada expirou antes do TTL configurado")
	}

	time.Sleep(2 * ttl)

	if _, found := c.Get("tok"); found {
		t.Fatalf("entrada ainda viva depois de %v com TTL de %v — o TTL positivo "+
			"foi substituido pelo padrao de %v", 2*ttl, ttl, auth.DefaultTTL)
	}
}

// TestNewTokenCache_DefaultsAreTheDocumentedOnes trava os valores que a
// normalização aplica. Se DefaultTTL virar NoExpiration num refactor, o bypass
// volta sem que nenhum outro teste perceba.
func TestNewTokenCache_DefaultsAreTheDocumentedOnes(t *testing.T) {
	if auth.DefaultTTL <= 0 {
		t.Fatalf("DefaultTTL nao-positivo (%v) reintroduz o cache sem expiracao", auth.DefaultTTL)
	}
	if auth.CleanupInterval <= 0 {
		t.Fatalf("CleanupInterval nao-positivo (%v) desliga o janitor", auth.CleanupInterval)
	}
}
