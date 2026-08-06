// Package walog adapta o logger do whatsmeow vendorizado
// (internal/wa-noise/util/log) para o zerolog da aplicação.
//
// É o único código de produção fora de internal/wa-noise/ que implementa uma
// interface *do* vendored: concentrá-lo aqui limita o raio de explosão de um
// drift do upstream (ADR-0002/0003) e mantém pkg/infra/wa-noise livre de
// dependência da interface waLog.Logger.
package walog

// Chaves e valores de campo emitidos pelo bridge. Nomes de módulo espelham
// os que o whatsmeow usa nos seus próprios subloggers.
const (
	// FieldModule é a chave sob a qual o módulo do SDK aparece no JSON.
	// Prefixo wa_ para não colidir com campos da aplicação.
	FieldModule = "wa_module"

	// ModuleDatabase é o módulo do container sqlstore.
	ModuleDatabase = "Database"

	// ModuleClient é o módulo do *whatsmeow.Client, do qual saem os
	// subloggers de Recv/Send/Pair.
	ModuleClient = "Client"

	// ModuleRoot é o módulo de um bridge sem dono definido.
	ModuleRoot = "whatsmeow"

	// SubSeparator separa os níveis de módulo em Sub, como em Client/Recv.
	SubSeparator = "/"
)
