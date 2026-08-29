package pairing

import (
	"sync/atomic"

	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/security/keys"
)

// LinkingCache guarda a sessao de pareamento por codigo de telefone que esta'
// pendente: o par de chaves efemero gerado por PairPhone, o codigo mostrado ao
// usuario e a referencia devolvida pelo servidor. HandleCodeNotification
// precisa dos tres para fechar o pareamento.
//
// Era o tipo nao exportado phoneLinkingCache, em pair-code.go na raiz.
type LinkingCache struct {
	JID         types.JID
	KeyPair     *keys.KeyPair
	LinkingCode string
	PairingRef  string
}

// State e' o estado mutavel do pareamento. Substitui o campo solto
// `phoneLinkingCache *phoneLinkingCache` de *Client.
//
// O campo e' atomic.Pointer porque as duas pontas rodam em goroutines
// diferentes: PairPhone escreve (chamado pela aplicacao) e
// HandleCodeNotification le' (chamado de um handler de notificacao). Como
// ponteiro comum — que e' como o upstream deixava — isso era corrida de dados
// pelo modelo de memoria do Go, e a goroutine de notificacao podia enxergar
// nil ou um LinkingCache parcialmente publicado (F50 em HOUSEKEEP.md).
//
// atomic.Pointer, e nao mutex, porque a semantica que o codigo quer e'
// exatamente "ultimo escritor ganha", sem secao critica nenhuma em volta.
//
// O zero value e' usavel.
type State struct {
	linking atomic.Pointer[LinkingCache]
}

// Linking devolve a sessao de pareamento por codigo pendente, ou nil.
func (s *State) Linking() *LinkingCache { return s.linking.Load() }

// SetLinking registra a sessao de pareamento por codigo pendente.
func (s *State) SetLinking(c *LinkingCache) { s.linking.Store(c) }
