package send

import "sync"

// State e' o estado mutavel do dominio de envio.
//
// Existe para que o mutex de envio pertenca a ESTE pacote, e nao a core. Ate' a
// correcao da F58 o mutex era o campo `messageSendLock sync.Mutex` de
// *core.Client, emprestado para ca' por ponteiro via `SendLock() *sync.Mutex`
// no Transport — a unica violacao de "estado e lock viajam juntos" em todo o
// modulo, e a unica capacidade que nao seguia o desenho de retry/, prekeys/ e
// tctoken/, que ja' tinham o proprio State.
//
// Emprestar o ponteiro nao era incorreto (a secao critica sempre foi a mesma),
// mas deixava o dono do lock a uma indirecao de distancia de quem o usa: quem
// lesse core/client.go via um mutex sem saber o que ele serializa, e quem
// lesse send/ via um lock sem dono aparente.
//
// Por conter um mutex, State NUNCA pode ser copiado por valor depois de usado
// — sempre passe *State.
//
// O zero value e' usavel.
type State struct {
	// sendLock serializa TODOS os envios do cliente.
	//
	// Nao e' so' uma protecao de estado: mandar duas mensagens ao mesmo tempo
	// para o mesmo usuario quebra o prefetch de sessao que torna o envio para
	// grupo rapido. A secao critica cobre da gravacao da mensagem recente ate'
	// o fim do envio, incluindo cifragem e escrita no socket.
	sendLock sync.Mutex
}

// SendLock devolve o mutex que serializa os envios.
//
// Devolve o ponteiro, e nao um par Lock/Unlock, porque os chamadores usam
// `defer lock.Unlock()` logo apos o Lock e medem o tempo de fila entre os dois.
func (s *State) SendLock() *sync.Mutex { return &s.sendLock }
