// Package groupreq adapts group membership requests — reading them, deciding
// them, and the policy that produces them — to the application's GroupRequests
// port.
package groupreq

import (
	"context"
	"fmt"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/wa-headless"
)

const (
	listLabel     = "adapter/list-group-requests"
	decideLabel   = "adapter/decide-group-requests"
	approvalLabel = "adapter/set-join-approval"
)

// requests is the slice of the request capability this adapter uses.
type requests interface {
	List(ctx context.Context, groupJID, label string) (waheadless.GroupRequestList, error)
	Approve(ctx context.Context, groupJID string, requesters []string, label string) ([]waheadless.GroupRequestAction, error)
	Reject(ctx context.Context, groupJID string, requesters []string, label string) ([]waheadless.GroupRequestAction, error)
}

// policy is the slice of the group capability this adapter uses. The approval
// MODE is a group policy, not a request operation — the port puts the two
// together because they are one product feature, but the page keeps them in
// separate modules and so does this adapter.
type policy interface {
	SetPolicy(ctx context.Context, groupJID string, p waheadless.GroupPolicy, on bool, label string) (waheadless.GroupPolicyChange, error)
}

// Manager implements appport.GroupRequests over a headless session.
type Manager struct {
	sessions *adapter.Sessions
	// newRequests and newPolicy are overridable in tests. Nil uses the real
	// page capabilities.
	newRequests func(ctx context.Context, txtID string) (requests, error)
	newPolicy   func(ctx context.Context, txtID string) (policy, error)
}

// NewManager builds the adapter.
func NewManager(sessions *adapter.Sessions) *Manager { return &Manager{sessions: sessions} }

// EnsureSession reports whether this process can serve txtID, without booting.
func (m *Manager) EnsureSession(ctx context.Context, txtID string) error {
	return m.sessions.EnsureSession(ctx, txtID)
}

// GetRequestParticipants reads the pending requests of one group.
//
// O resultado carrega Fields, os NOMES dos campos que os registros da página
// trouxeram — nunca os valores. A forma não pôde ser medida num grupo sem
// solicitações pendentes, então a primeira solicitação viva é que diz o que
// está mesmo lá. Achatar isso custaria a única via de descobrir.
func (m *Manager) GetRequestParticipants(ctx context.Context, txtID string, group domain.JID) ([]domain.GroupJoinRequest, error) {
	pageJID, err := adapter.ToPageJID(group)
	if err != nil {
		return nil, err
	}
	r, err := m.requests(ctx, txtID)
	if err != nil {
		return nil, err
	}
	lista, err := r.List(ctx, pageJID, listLabel)
	if err != nil {
		return nil, err
	}
	// Fields fica de fora: são os NOMES dos campos que os registros da página
	// trouxeram, informação de diagnóstico para quem lê o log, e não parte do
	// que um cliente pediu. Continua a sair no String() da capability.
	out := make([]domain.GroupJoinRequest, 0, len(lista.Requests))
	for _, req := range lista.Requests {
		out = append(out, domain.GroupJoinRequest{
			JID:         domain.JID(req.RequesterJID),
			RequestedAt: req.At,
			AddedByJID:  domain.JID(req.AddedByJID),
			Method:      req.Method,
		})
	}
	return out, nil
}

// UpdateRequestParticipants approves or rejects requesters.
//
// A capability faz UM RPC POR PARTICIPANTE e devolve um resultado por
// solicitante, porque o desfecho parcial é normal: três aprovações em que a
// segunda falha são três resultados, não um erro. F280 (2026-08-28): esse
// resultado por solicitante agora sai no domain.ParticipantsUpdate que o
// port devolve — mesma forma que o motor noise já usa para add/remove —
// em vez de ser só contado e descartado.
//
// O que não pode acontecer continua a não acontecer (invariante 14): se
// qualquer solicitante falhou, ou a página respondeu por menos gente do que
// foi pedido, isto continua erro — com a CONTAGEM dos que falharam e os
// códigos que a página devolveu. Os jids dos solicitantes que FALHARAM
// ficam de fora da mensagem de erro — são identidade, e a mensagem de erro
// é registro; os que TIVERAM SUCESSO aparecem no ParticipantsUpdate
// devolvido no caminho sem erro, que é dado do pedido, não do log.
func (m *Manager) UpdateRequestParticipants(ctx context.Context, txtID string, group domain.JID, participants []domain.JID, action domain.RequestAction) (domain.ParticipantsUpdate, error) {
	if len(participants) == 0 {
		return domain.ParticipantsUpdate{}, fmt.Errorf("waheadless: no requesters to %s", action)
	}
	pageJID, err := adapter.ToPageJID(group)
	if err != nil {
		return domain.ParticipantsUpdate{}, err
	}
	requesters := make([]string, 0, len(participants))
	for _, p := range participants {
		pageRequester, err := adapter.ToPageJID(p)
		if err != nil {
			return domain.ParticipantsUpdate{}, err
		}
		requesters = append(requesters, pageRequester)
	}

	r, err := m.requests(ctx, txtID)
	if err != nil {
		return domain.ParticipantsUpdate{}, err
	}

	var results []waheadless.GroupRequestAction
	switch action {
	case domain.RequestApprove:
		results, err = r.Approve(ctx, pageJID, requesters, decideLabel)
	case domain.RequestReject:
		results, err = r.Reject(ctx, pageJID, requesters, decideLabel)
	default:
		return domain.ParticipantsUpdate{}, fmt.Errorf("waheadless: unknown request action %q", action)
	}
	if err != nil {
		return domain.ParticipantsUpdate{}, err
	}
	if err := partialFailure(results, len(requesters), action); err != nil {
		return domain.ParticipantsUpdate{}, err
	}
	out := make([]domain.GroupParticipant, 0, len(results))
	for _, res := range results {
		out = append(out, domain.GroupParticipant{JID: domain.JID(res.RequesterJID), Error: res.Code})
	}
	return domain.ParticipantsUpdate{Participants: out, Confirmed: true}, nil
}

// partialFailure turns a per-requester outcome into the single error the port
// can carry. It is a free function so the rule is testable without a page.
//
// Two things make it an error rather than a silent success: a requester the
// page refused, and a requester the page never answered about — a shorter
// result list than the request list is not agreement, it is absence.
func partialFailure(results []waheadless.GroupRequestAction, asked int, action domain.RequestAction) error {
	failed := 0
	codes := make([]int, 0, len(results))
	for _, res := range results {
		if res.OK {
			continue
		}
		failed++
		codes = append(codes, res.Code)
	}
	missing := asked - len(results)
	if failed == 0 && missing <= 0 {
		return nil
	}
	return fmt.Errorf("waheadless: %s incompleto: %d de %d recusados pela pagina (codigos %v), %d sem resposta",
		action, failed, asked, codes, missing)
}

// SetJoinApprovalMode turns joining this group into a request, or back.
//
// Ao contrário de uma mudança de participantes, esta É verificável pela sessão
// que agiu: PolicyChange.Verified é verdadeiro para uma mudança real, e a
// diferença entre as duas foi MEDIDA, não suposta (H85). Este port devolve só
// error, então o que sobra desse fato é a recusa: mudança não verificada não
// pode passar por feita.
func (m *Manager) SetJoinApprovalMode(ctx context.Context, txtID string, group domain.JID, mode bool) error {
	pageJID, err := adapter.ToPageJID(group)
	if err != nil {
		return err
	}
	p, err := m.policy(ctx, txtID)
	if err != nil {
		return err
	}
	change, err := p.SetPolicy(ctx, pageJID, waheadless.PolicyJoinNeedsApproval, mode, approvalLabel)
	if err != nil {
		return err
	}
	return unverified(change)
}

// unverified refuses a policy change the page did not read back, unless there
// was nothing to change. A no-op has no new value to confirm.
func unverified(change waheadless.GroupPolicyChange) error {
	if change.NoOp || change.Verified {
		return nil
	}
	return fmt.Errorf("waheadless: modo de aprovacao pedido (%t) sem confirmacao lida de volta", change.Wanted)
}

func (m *Manager) requests(ctx context.Context, txtID string) (requests, error) {
	if m.newRequests != nil {
		return m.newRequests(ctx, txtID)
	}
	eval, err := m.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return waheadless.NewGroupRequestManager(m.sessions.Runner(), eval), nil
}

func (m *Manager) policy(ctx context.Context, txtID string) (policy, error) {
	if m.newPolicy != nil {
		return m.newPolicy(ctx, txtID)
	}
	eval, err := m.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return nil, err
	}
	return waheadless.NewGroupManager(m.sessions.Runner(), eval), nil
}

// Compile-time proof that this adapter satisfies the port.
var _ appport.GroupRequests = (*Manager)(nil)
