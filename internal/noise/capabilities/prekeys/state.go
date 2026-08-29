package prekeys

import (
	"sync"
	"time"
)

// State e' o estado mutavel do dominio de prekeys. Reune os dois campos que
// viviam soltos em *Client antes desta extracao:
//
//	uploadPreKeysLock sync.Mutex -> uploadLock
//	lastPreKeyUpload  time.Time  -> lastUpload
//
// A estrutura de aquisicao e liberacao do lock NAO mudou: o corpo inteiro de
// Upload roda sob ele, do primeiro statement ao ultimo, exatamente como o
// corpo de uploadPreKeys rodava. lastUpload continua sendo lido e escrito
// apenas dentro dessa secao critica — por isso LastUpload e SetLastUpload nao
// tomam lock nenhum: tomar um segundo lock aqui mudaria o desenho.
//
// O zero value e' usavel. Por conter um mutex, State NUNCA pode ser copiado
// por valor depois de usado — sempre passe *State.
type State struct {
	// uploadLock serializa uploads de prekey entre si.
	uploadLock sync.Mutex
	// lastUpload e' quando o ultimo upload bem-sucedido terminou. Protegido
	// por uploadLock; ver o doc do tipo.
	lastUpload time.Time
}

// LockUpload/UnlockUpload serializam o upload de prekeys. Substituem o par
// `cli.uploadPreKeysLock.Lock()` / `defer cli.uploadPreKeysLock.Unlock()` que
// abria o corpo de uploadPreKeys; o lock e' segurado pelo upload inteiro,
// incluindo as consultas ao servidor, exatamente como antes.
func (s *State) LockUpload() { s.uploadLock.Lock() }

// UnlockUpload libera o lock tomado por LockUpload.
func (s *State) UnlockUpload() { s.uploadLock.Unlock() }

// LastUpload devolve o horario do ultimo upload concluido.
//
// So' pode ser chamado com o lock de LockUpload segurado. Nao toma lock por
// conta propria de proposito: no codigo original a leitura de lastPreKeyUpload
// acontecia dentro da mesma secao critica que a escrita, e um lock proprio
// aqui criaria um segundo ponto de sincronizacao que nao existia.
func (s *State) LastUpload() time.Time { return s.lastUpload }

// SetLastUpload registra o horario do ultimo upload concluido.
//
// Mesma condicao de LastUpload: exige o lock de LockUpload segurado.
func (s *State) SetLastUpload(t time.Time) { s.lastUpload = t }
