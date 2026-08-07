package prekeys

import (
	"context"

	waLog "wa-api/internal/wa-noise/observability/log"
	"wa-api/internal/wa-noise/persistence/store"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// IQType e' o atributo "type" de um <iq>. Espelha o infoQueryType do pacote
// raiz sem depender dele.
type IQType string

// Os dois tipos de <iq> usados por este dominio: get para contar/buscar
// prekeys, set para enviar o lote.
const (
	IQGet IQType = "get"
	IQSet IQType = "set"
)

// IQ e' a fatia de infoQuery (pacote raiz) que este dominio usa. Existe pelo
// mesmo motivo que newsletter.IQ (lote 2) e appstatesync.IQ (lote 3): expor o
// infoQuery da raiz na interface arrastaria a raiz para dentro deste pacote e
// refaria o ciclo de import. O adaptador da raiz traduz IQ -> infoQuery campo a
// campo; os campos que este dominio nunca preenche (Target, ID, SMaxID,
// Timeout, NoRetry) ficam no zero, exatamente como ficavam antes da extracao.
type IQ struct {
	Namespace string
	Type      IQType
	To        types.JID
	Content   any
}

// Transport e' a fatia do cliente de que o dominio de prekeys precisa.
//
// Deliberadamente nao expoe nada do *wa-noise.Client alem disso — e' o que
// permite que este pacote nao importe o pacote raiz e que os testes usem um
// duble em vez de um cliente real com socket e sessao Noise.
type Transport interface {
	// Store e' o device store da sessao. Aparece inteiro (e nao fatiado em um
	// metodo por sub-store) pelo mesmo racional do lote 3: `store` ja' e' um
	// subpacote folha do fork, entao expo-lo nao cria dependencia nova nem
	// ciclo, e fatia-lo custaria metodos de interface sem ganhar isolamento.
	Store() *store.Device
	// State e' o estado mutavel deste dominio (lock de upload e horario do
	// ultimo upload). Vive no cliente para durar entre chamadas; o ponteiro
	// precisa ser estavel (o mutex nunca pode ser copiado por valor).
	State() *State
	// Log e' o logger do cliente.
	Log() waLog.Logger
	// SendIQ envia um <iq> e espera a resposta.
	SendIQ(ctx context.Context, query IQ) (*waBinary.Node, error)
}
