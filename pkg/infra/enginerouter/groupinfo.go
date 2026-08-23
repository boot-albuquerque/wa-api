// Package enginerouter escolhe, POR SESSÃO, qual transporte serve cada chamada.
//
// Existe porque os dois transportes não servem o mesmo conjunto de ports
// (medido: o headless satisfaz 17 dos 25), e a decisão 94 exige que a escolha
// seja explícita e que a falta seja uma RECUSA — nunca uma queda calada para o
// outro lado.
//
// # Por que um roteador por port, e não um genérico
//
// Um roteador tem de IMPLEMENTAR a interface do port, método a método, e Go não
// gera isso. A alternativa seria um despachante por reflexão, que trocaria um
// erro de compilação por um de execução — exatamente a troca que este
// repositório recusa em todo lado.
//
// O que NÃO se repete é a regra: ela vive uma vez, em Selector/rotaDeEngine, e
// cada roteador só a consulta. Uma regra copiada em vinte e cinco `if` é vinte
// e cinco oportunidades de divergir (lição da decisão 82).
package enginerouter

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// Selector diz qual engine serve uma sessão, e como recusar quando o engine
// escolhido não serve o port.
//
// É uma interface, e não o tipo do bootstrap, porque isto é infraestrutura e
// não pode importar o pacote que a monta.
type Selector interface {
	// UsaHeadless diz se ESTA sessão está no transporte headless.
	UsaHeadless(txtID string) bool
	// Recusar produz o erro de "engine não serve este port".
	Recusar(port, txtID string) error
}

// GroupInfo roteia appport.GroupInfoSettings entre os dois transportes.
//
// `headless` nil significa que o transporte headless NÃO serve este port, e
// nesse caso uma sessão headless é recusada. Nunca cai no socket: cair
// devolveria uma resposta correta para uma pergunta que ninguém fez.
type GroupInfo struct {
	nome     string
	sel      Selector
	socket   appport.GroupInfoSettings
	headless appport.GroupInfoSettings
}

// NewGroupInfo monta o roteador. `headless` pode ser nil.
func NewGroupInfo(sel Selector, socket, headless appport.GroupInfoSettings) *GroupInfo {
	return &GroupInfo{nome: "GroupInfoSettings", sel: sel, socket: socket, headless: headless}
}

// alvo devolve o transporte desta sessão, ou o erro que diz por que não há um.
func (r *GroupInfo) alvo(txtID string) (appport.GroupInfoSettings, error) {
	if !r.sel.UsaHeadless(txtID) {
		return r.socket, nil
	}
	if r.headless == nil {
		return nil, r.sel.Recusar(r.nome, txtID)
	}
	return r.headless, nil
}

// EnsureSession pergunta ao transporte DESTA sessão.
//
// A guarda de posse tem de ir ao mesmo lado que a operação iria: perguntar ao
// socket se o headless é que serve responderia sobre a sessão errada, e o
// chamador leria "pronta" para uma sessão que nunca foi restaurada.
func (r *GroupInfo) EnsureSession(ctx context.Context, txtID string) error {
	alvo, err := r.alvo(txtID)
	if err != nil {
		return err
	}
	return alvo.EnsureSession(ctx, txtID)
}

func (r *GroupInfo) SetGroupName(ctx context.Context, txtID string, group domain.JID, name string) error {
	alvo, err := r.alvo(txtID)
	if err != nil {
		return err
	}
	return alvo.SetGroupName(ctx, txtID, group, name)
}

func (r *GroupInfo) SetGroupTopic(ctx context.Context, txtID string, group domain.JID, topic string) error {
	alvo, err := r.alvo(txtID)
	if err != nil {
		return err
	}
	return alvo.SetGroupTopic(ctx, txtID, group, topic)
}

func (r *GroupInfo) SetGroupAnnounce(ctx context.Context, txtID string, group domain.JID, announce bool) error {
	alvo, err := r.alvo(txtID)
	if err != nil {
		return err
	}
	return alvo.SetGroupAnnounce(ctx, txtID, group, announce)
}

func (r *GroupInfo) SetGroupLocked(ctx context.Context, txtID string, group domain.JID, locked bool) error {
	alvo, err := r.alvo(txtID)
	if err != nil {
		return err
	}
	return alvo.SetGroupLocked(ctx, txtID, group, locked)
}

// Prova em tempo de compilação de que o roteador é substituível pelo port.
var _ appport.GroupInfoSettings = (*GroupInfo)(nil)
