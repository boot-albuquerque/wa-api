package auth

import (
	"time"

	"github.com/patrickmn/go-cache"
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
		defaultTTL = DefaultTTL
	}
	if cleanupInterval <= 0 {
		cleanupInterval = CleanupInterval
	}
	return cache.New(defaultTTL, cleanupInterval)
}
