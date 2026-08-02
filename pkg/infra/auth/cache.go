package auth

import (
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
)

const (
	// DefaultTTL é o tempo de expiração padrão para entradas de cache de tokens.
	DefaultTTL = 5 * time.Minute
	// CleanupInterval é o intervalo de limpeza de entradas expiradas.
	CleanupInterval = 10 * time.Minute
)

// NewTokenCache cria um cache com TTL configurável.
// Substitui o uso de cache.NoExpiration para evitar vazamento de memória.
// defaultTTL: tempo de expiração padrão (recomendado: 5 min).
// cleanupInterval: intervalo de limpeza de itens expirados (recomendado: 10 min).
func NewTokenCache(defaultTTL, cleanupInterval time.Duration) *cache.Cache {
	if defaultTTL <= 0 {
		// go-cache trata `0` como NoExpiration: sem esta normalização o cache
		// de tokens nunca expiraria e uma credencial revogada continuaria
		// autenticando. O caso é anômalo o bastante para virar evidência.
		log.Warn().
			Str("component", "auth.NewTokenCache").
			Dur("requested_ttl", defaultTTL).
			Dur("applied_ttl", DefaultTTL).
			Msg("TTL nao-positivo recusado; caira para o padrao para manter entradas expiraveis")
		defaultTTL = DefaultTTL
	}
	if cleanupInterval <= 0 {
		log.Warn().
			Str("component", "auth.NewTokenCache").
			Dur("requested_cleanup_interval", cleanupInterval).
			Dur("applied_cleanup_interval", CleanupInterval).
			Msg("intervalo de limpeza nao-positivo recusado; caira para o padrao para manter o janitor ativo")
		cleanupInterval = CleanupInterval
	}
	return cache.New(defaultTTL, cleanupInterval)
}
