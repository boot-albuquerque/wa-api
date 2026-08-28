package notification

import (
	"context"
	"errors"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// As onze operações de newsletter que o levantamento de paridade de 2026-08-20
// encontrou em falta. A biblioteca expunha doze capacidades e nós expúnhamos
// uma (ListSubscribed).
//
// POR QUE UM USE CASE E NÃO ONZE. As onze têm a mesma forma: garantir sessão,
// validar o identificador, delegar, traduzir o erro. Onze ficheiros com essa
// forma repetida são onze sítios onde ela pode divergir — e a HOUSEKEEP F187
// custou um dia inteiro por exatamente isso, com DUAS cópias.
//
// O que varia entre elas é o método chamado e os campos do pedido, e isso vive
// no `switch` de Execute em vez de na estrutura do ficheiro.

// NewsletterOp identifica a operação pedida.
type NewsletterOp string

// As operações. São constantes e não strings soltas porque atravessam a
// fronteira HTTP: o handler recebe o nome da rota e escolhe uma destas
// (ADR-0004).
const (
	NewsletterOpCreate      NewsletterOp = "create"
	NewsletterOpInfo        NewsletterOp = "info"
	NewsletterOpInfoInvite  NewsletterOp = "info_invite"
	NewsletterOpFollow      NewsletterOp = "follow"
	NewsletterOpUnfollow    NewsletterOp = "unfollow"
	NewsletterOpMute        NewsletterOp = "mute"
	NewsletterOpMessages    NewsletterOp = "messages"
	NewsletterOpUpdates     NewsletterOp = "updates"
	NewsletterOpMarkViewed  NewsletterOp = "mark_viewed"
	NewsletterOpReact       NewsletterOp = "react"
	NewsletterOpSubscribe   NewsletterOp = "subscribe"
	NewsletterOpDemote      NewsletterOp = "demote"
	NewsletterOpChangeOwner NewsletterOp = "change_owner"
	NewsletterOpDelete      NewsletterOp = "delete"

	NewsletterOpAdminInvite       NewsletterOp = "admin_invite"
	NewsletterOpAdminInviteAccept NewsletterOp = "admin_invite_accept"
	NewsletterOpAdminInviteRevoke NewsletterOp = "admin_invite_revoke"
)

// NewsletterRequest é o pedido de qualquer uma das onze.
//
// Campos opcionais em vez de um tipo por operação: o custo de um struct largo é
// menor que o de onze DTOs que precisam de ser mantidos em paralelo com onze
// rotas — e o Execute valida o que CADA operação exige, que é onde a garantia
// tem de estar.
type NewsletterRequest struct {
	Op NewsletterOp

	JID    domain.JID // canal, para tudo menos create e info_invite
	Invite string     // código de convite, só para info_invite

	Name        string // create
	Description string // create
	Picture     []byte // create

	Mute bool // mute

	Count  int       // messages, updates
	Before string    // messages
	After  string    // updates
	Since  time.Time // updates

	ServerIDs []int  // mark_viewed
	ServerID  int    // react
	Reaction  string // react — vazio REMOVE a reação
	MessageID string // react

	UserJID    domain.JID // demote, change_owner — the target user
	ConfirmJID domain.JID // delete — must match JID as explicit confirmation
}

// NewsletterResult carrega o que a operação devolveu.
//
// SEM ETIQUETAS `json`, e a ausência é o ponto: este tipo já não é o formato de
// fio. Era — `Data any` ia direto para o codificador, o que fazia a forma da
// resposta de `/newsletter/info` ser decidida pelo motor da sessão. A forma
// pública vive agora em `pkg/presentation/http/dto/newsletter`.
//
// Os três campos de carga são exclusivos por operação, e são campos SEPARADOS
// em vez de um `any` porque é isso que dá erro de compilação quando o
// apresentador lê o campo errado: com `any`, ler `Messages` de um `info` seria
// uma asserção de tipo que falha em runtime e serve `null`.
type NewsletterResult struct {
	// Metadata é preenchido por create, info e info_invite.
	Metadata *domain.NewsletterMetadata
	// Messages é preenchido por messages e updates.
	Messages []domain.NewsletterMessage
	// Duration é preenchida só por subscribe: é por quanto tempo as
	// atualizações ao vivo valem, e vem do servidor.
	Duration time.Duration
	Status   string
}

// NewsletterOpsUseCase executa as onze operações.
type NewsletterOpsUseCase struct {
	newsletters appport.NewsletterReader
	logger      appport.Logger
}

// NewNewsletterOpsUseCase cria o use case.
func NewNewsletterOpsUseCase(nr appport.NewsletterReader, logger appport.Logger) *NewsletterOpsUseCase {
	return &NewsletterOpsUseCase{newsletters: nr, logger: logger}
}

// Execute corre a operação pedida.
func (uc *NewsletterOpsUseCase) Execute(ctx context.Context, userID string, req NewsletterRequest) (*NewsletterResult, error) {
	if err := uc.newsletters.EnsureSession(ctx, userID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "error", err, "user_id", userID, "op", string(req.Op))
		return nil, err
	}

	if err := validateNewsletter(req); err != nil {
		uc.logger.Warn(ctx, "newsletter request refused", "error", err, "user_id", userID, "op", string(req.Op))
		return nil, err
	}

	result, err := uc.dispatch(ctx, userID, req)
	if err != nil {
		var appErr *apperr.AppError
		if errors.As(err, &appErr) {
			uc.logger.Warn(ctx, "newsletter operation refused", "error", err,
				"user_id", userID, "op", string(req.Op), "code", appErr.Code)
			return nil, err
		}
		uc.logger.Error(ctx, "newsletter operation failed", "error", err, "user_id", userID, "op", string(req.Op))
		return nil, apperr.New("newsletter_failed", apperr.CategoryInternal,
			"newsletter operation failed", true, err)
	}

	uc.logger.Info(ctx, "newsletter operation done", "user_id", userID, "op", string(req.Op))
	result.Status = domain.StatusSent
	return &result, nil
}

// dispatch chama a porta. Separado do Execute para que a guarda de sessão, a
// validação e a tradução de erro não fiquem enterradas num switch de onze
// ramos — e para que o switch seja legível como a tabela que ele é.
//
// CADA RAMO ATRIBUI O ERRO ANTES DE O DEVOLVER, e a forma de duas linhas não é
// desleixo: condensá-la em `return NewsletterResult{}, n.X(...)` custou eleven
// caminhos de saída na medição de cobertura de log — 88,9% -> 48,1% neste
// pacote, medido a 2026-08-27 com `go run ./cmd/logcov -by-package ./pkg`
// antes e depois. O comportamento é idêntico; o que muda é o analisador deixar
// de reconhecer a propagação da causa. Não volte a encurtar.
func (uc *NewsletterOpsUseCase) dispatch(ctx context.Context, userID string, req NewsletterRequest) (NewsletterResult, error) {
	n := uc.newsletters
	switch req.Op {
	case NewsletterOpCreate:
		m, err := n.CreateNewsletter(ctx, userID, req.Name, req.Description, req.Picture)
		return NewsletterResult{Metadata: m}, err
	case NewsletterOpInfo:
		m, err := n.NewsletterInfo(ctx, userID, req.JID)
		return NewsletterResult{Metadata: m}, err
	case NewsletterOpInfoInvite:
		m, err := n.NewsletterInfoWithInvite(ctx, userID, req.Invite)
		return NewsletterResult{Metadata: m}, err
	case NewsletterOpFollow:
		err := n.FollowNewsletter(ctx, userID, req.JID)
		return NewsletterResult{}, err
	case NewsletterOpUnfollow:
		err := n.UnfollowNewsletter(ctx, userID, req.JID)
		return NewsletterResult{}, err
	case NewsletterOpMute:
		err := n.ToggleNewsletterMute(ctx, userID, req.JID, req.Mute)
		return NewsletterResult{}, err
	case NewsletterOpMessages:
		msgs, err := n.NewsletterMessages(ctx, userID, req.JID, req.Count, req.Before)
		return NewsletterResult{Messages: msgs}, err
	case NewsletterOpUpdates:
		msgs, err := n.NewsletterMessageUpdates(ctx, userID, req.JID, req.Count, req.Since, req.After)
		return NewsletterResult{Messages: msgs}, err
	case NewsletterOpMarkViewed:
		err := n.MarkNewsletterViewed(ctx, userID, req.JID, req.ServerIDs)
		return NewsletterResult{}, err
	case NewsletterOpReact:
		err := n.SendNewsletterReaction(ctx, userID, req.JID, req.ServerID, req.Reaction, req.MessageID)
		return NewsletterResult{}, err
	case NewsletterOpSubscribe:
		dur, err := n.SubscribeNewsletterLiveUpdates(ctx, userID, req.JID)
		return NewsletterResult{Duration: dur}, err
	case NewsletterOpDemote:
		err := n.DemoteNewsletterAdmin(ctx, userID, req.JID, req.UserJID)
		return NewsletterResult{}, err
	case NewsletterOpChangeOwner:
		err := n.ChangeNewsletterOwner(ctx, userID, req.JID, req.UserJID)
		return NewsletterResult{}, err
	case NewsletterOpDelete:
		err := n.DeleteNewsletter(ctx, userID, req.JID)
		return NewsletterResult{}, err
	case NewsletterOpAdminInvite:
		err := n.CreateNewsletterAdminInvite(ctx, userID, req.JID, req.UserJID)
		return NewsletterResult{}, err
	case NewsletterOpAdminInviteAccept:
		err := n.AcceptNewsletterAdminInvite(ctx, userID, req.JID)
		return NewsletterResult{}, err
	case NewsletterOpAdminInviteRevoke:
		err := n.RevokeNewsletterAdminInvite(ctx, userID, req.JID, req.UserJID)
		return NewsletterResult{}, err
	}
	// Inalcançável enquanto validateNewsletter correr primeiro. Fica como erro
	// e não como panic porque uma operação nova acrescentada ao switch da
	// validação e esquecida aqui tem de virar 4xx, não derrubar o processo.
	return NewsletterResult{}, apperr.New("unknown_newsletter_op", apperr.CategoryValidation,
		"unknown newsletter operation", false, nil)
}

// newsletterRequirement é um campo obrigatório de uma operação.
type newsletterRequirement struct {
	code    string
	message string
	missing func(NewsletterRequest) bool
}

// Error codes for the shared JID rules. They are constants and not literals
// because they cross the HTTP boundary — a client branches on them (ADR-0004).
const (
	codeMissingJID        = "missing_jid"
	codeInvalidNewsletter = "invalid_newsletter_jid"
)

// requireJID é partilhado porque sete das onze operações pedem o mesmo canal:
// escrever a mesma verificação sete vezes seria sete sítios onde a mensagem de
// erro pode divergir.
var requireJID = newsletterRequirement{
	code:    codeMissingJID,
	message: "channel jid is required",
	missing: func(r NewsletterRequest) bool { return r.JID == "" },
}

// requireNewsletterServer rejects a jid that is PRESENT but cannot name a
// channel — blanks, free text, or a jid from another server such as
// "@s.whatsapp.net".
//
// WHY IT IS A VALIDATION AND NOT A DISPATCH FAILURE (F271). Until this rule
// existed such a jid reached the adapter, failed there, and came back as
// `500 newsletter_failed`. A 5xx tells the caller "my fault, retry" — false
// here, since the same jid fails forever, so a client with automatic retry
// hammers a request that can never work; and it makes any dashboard counting
// 5xx count the consumer's typos as service failures.
//
// It sits immediately after requireJID so the ABSENCE keeps reporting
// `missing_jid`: the two answers say different things to the caller, and
// collapsing them would trade one wrong code for another.
var requireNewsletterServer = newsletterRequirement{
	code:    codeInvalidNewsletter,
	message: "jid must be a channel jid ending in " + domain.ServerNewsletter,
	missing: func(r NewsletterRequest) bool { return !r.JID.IsNewsletter() },
}

// The two rules travel TOGETHER in every row of the table below: an operation
// that listed only requireJID would go back to answering 500 for a malformed
// jid, which is the defect F271 recorded. A helper that built the pair would
// enforce that structurally, but it would also add an uncovered function to the
// log-coverage denominator (ADR-008 gate, stage=ratchet), so the invariant is
// enforced by TestNewsletter_JIDRulesAreNeverSplit instead — it reads the table
// and fails on any row that carries one rule without the other.

// newsletterRequirements diz, por operação, o que o pedido tem de trazer.
//
// TABELA E NÃO `switch`: o que varia entre as onze é DADO — qual campo é
// obrigatório —, e escrever dado como fluxo de controlo foi o que fez esta
// validação ter oito saídas em vez de uma. Com a tabela, acrescentar uma
// operação é acrescentar uma linha, e uma operação ausente da tabela é
// recusada em vez de passar em silêncio.
var newsletterRequirements = map[NewsletterOp][]newsletterRequirement{
	NewsletterOpCreate: {{
		code:    "missing_name",
		message: "channel name is required",
		missing: func(r NewsletterRequest) bool { return r.Name == "" },
	}},
	NewsletterOpInfoInvite: {{
		code:    "missing_invite",
		message: "invite code is required",
		missing: func(r NewsletterRequest) bool { return r.Invite == "" },
	}},
	NewsletterOpMarkViewed: {requireJID, requireNewsletterServer, {
		code:    "missing_server_ids",
		message: "at least one server_id is required",
		missing: func(r NewsletterRequest) bool { return len(r.ServerIDs) == 0 },
	}},
	// `reaction` NÃO entra na tabela: vazio REMOVE a reação, como no resto do
	// protocolo. Exigi-lo tornaria impossível desfazer pelo painel.
	NewsletterOpReact: {requireJID, requireNewsletterServer, {
		code:    "missing_server_id",
		message: "server_id is required",
		missing: func(r NewsletterRequest) bool { return r.ServerID == 0 },
	}},
	NewsletterOpInfo:      {requireJID, requireNewsletterServer},
	NewsletterOpFollow:    {requireJID, requireNewsletterServer},
	NewsletterOpUnfollow:  {requireJID, requireNewsletterServer},
	NewsletterOpMute:      {requireJID, requireNewsletterServer},
	NewsletterOpMessages:  {requireJID, requireNewsletterServer},
	NewsletterOpUpdates:   {requireJID, requireNewsletterServer},
	NewsletterOpSubscribe: {requireJID, requireNewsletterServer},
	NewsletterOpDemote: {requireJID, requireNewsletterServer, {
		code:    "missing_user_jid",
		message: "user jid is required for demote",
		missing: func(r NewsletterRequest) bool { return r.UserJID == "" },
	}},
	NewsletterOpChangeOwner: {requireJID, requireNewsletterServer, {
		code:    "missing_user_jid",
		message: "new owner jid is required",
		missing: func(r NewsletterRequest) bool { return r.UserJID == "" },
	}},
	NewsletterOpDelete: {requireJID, requireNewsletterServer, {
		code:    "missing_confirm_jid",
		message: "confirm_jid must match the channel jid",
		missing: func(r NewsletterRequest) bool { return r.ConfirmJID == "" || r.ConfirmJID != r.JID },
	}},
	NewsletterOpAdminInvite: {requireJID, requireNewsletterServer, {
		code:    "missing_user_jid",
		message: "invitee jid is required",
		missing: func(r NewsletterRequest) bool { return r.UserJID == "" },
	}},
	NewsletterOpAdminInviteAccept: {requireJID, requireNewsletterServer},
	NewsletterOpAdminInviteRevoke: {requireJID, requireNewsletterServer, {
		code:    "missing_user_jid",
		message: "invitee jid is required for revoke",
		missing: func(r NewsletterRequest) bool { return r.UserJID == "" },
	}},
}

// validateNewsletter exige o que CADA operação precisa.
//
// A validação é por operação e não global porque os identificadores são
// diferentes: quase todas querem um JID de canal, `info_invite` quer um CÓDIGO
// de convite, e `create` não quer identificador nenhum. Uma validação única
// teria de aceitar os três, e aí não validava nada.
func validateNewsletter(req NewsletterRequest) error {
	rules, known := newsletterRequirements[req.Op]
	if !known {
		return apperr.New("unknown_newsletter_op", apperr.CategoryValidation,
			"unknown newsletter operation", false, nil)
	}
	for _, rule := range rules {
		if rule.missing(req) {
			return apperr.New(rule.code, apperr.CategoryValidation, rule.message, false, nil)
		}
	}
	return nil
}
