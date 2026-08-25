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
	NewsletterOpCreate     NewsletterOp = "create"
	NewsletterOpInfo       NewsletterOp = "info"
	NewsletterOpInfoInvite NewsletterOp = "info_invite"
	NewsletterOpFollow     NewsletterOp = "follow"
	NewsletterOpUnfollow   NewsletterOp = "unfollow"
	NewsletterOpMute       NewsletterOp = "mute"
	NewsletterOpMessages   NewsletterOp = "messages"
	NewsletterOpUpdates    NewsletterOp = "updates"
	NewsletterOpMarkViewed NewsletterOp = "mark_viewed"
	NewsletterOpReact      NewsletterOp = "react"
	NewsletterOpSubscribe  NewsletterOp = "subscribe"
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
}

// NewsletterResult carrega o que a operação devolveu.
//
// `Data` é `any` pela mesma razão da porta: NewsletterMetadata é tipo do
// vendor e traduzi-lo arrastaria a árvore do protocolo para o domínio.
// `Duration` existe separado porque só o subscribe devolve tempo, e enfiá-lo
// em Data faria o cliente ter de adivinhar quando olhar para lá.
type NewsletterResult struct {
	Data            any    `json:"data,omitempty"`
	DurationSeconds int64  `json:"duration_seconds,omitempty"`
	Status          string `json:"status"`
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

	data, dur, err := uc.dispatch(ctx, userID, req)
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
	return &NewsletterResult{Data: data, DurationSeconds: int64(dur.Seconds()), Status: domain.StatusSent}, nil
}

// dispatch chama a porta. Separado do Execute para que a guarda de sessão, a
// validação e a tradução de erro não fiquem enterradas num switch de onze
// ramos — e para que o switch seja legível como a tabela que ele é.
func (uc *NewsletterOpsUseCase) dispatch(ctx context.Context, userID string, req NewsletterRequest) (any, time.Duration, error) {
	n := uc.newsletters
	switch req.Op {
	case NewsletterOpCreate:
		d, err := n.CreateNewsletter(ctx, userID, req.Name, req.Description, req.Picture)
		return d, 0, err
	case NewsletterOpInfo:
		d, err := n.NewsletterInfo(ctx, userID, req.JID)
		return d, 0, err
	case NewsletterOpInfoInvite:
		d, err := n.NewsletterInfoWithInvite(ctx, userID, req.Invite)
		return d, 0, err
	case NewsletterOpFollow:
		err := n.FollowNewsletter(ctx, userID, req.JID)
		return nil, 0, err
	case NewsletterOpUnfollow:
		err := n.UnfollowNewsletter(ctx, userID, req.JID)
		return nil, 0, err
	case NewsletterOpMute:
		err := n.ToggleNewsletterMute(ctx, userID, req.JID, req.Mute)
		return nil, 0, err
	case NewsletterOpMessages:
		d, err := n.NewsletterMessages(ctx, userID, req.JID, req.Count, req.Before)
		return d, 0, err
	case NewsletterOpUpdates:
		d, err := n.NewsletterMessageUpdates(ctx, userID, req.JID, req.Count, req.Since, req.After)
		return d, 0, err
	case NewsletterOpMarkViewed:
		err := n.MarkNewsletterViewed(ctx, userID, req.JID, req.ServerIDs)
		return nil, 0, err
	case NewsletterOpReact:
		err := n.SendNewsletterReaction(ctx, userID, req.JID, req.ServerID, req.Reaction, req.MessageID)
		return nil, 0, err
	case NewsletterOpSubscribe:
		dur, err := n.SubscribeNewsletterLiveUpdates(ctx, userID, req.JID)
		return nil, dur, err
	}
	// Inalcançável enquanto validateNewsletter correr primeiro. Fica como erro
	// e não como panic porque uma operação nova acrescentada ao switch da
	// validação e esquecida aqui tem de virar 4xx, não derrubar o processo.
	return nil, 0, apperr.New("unknown_newsletter_op", apperr.CategoryValidation,
		"unknown newsletter operation", false, nil)
}

// newsletterRequirement é um campo obrigatório de uma operação.
type newsletterRequirement struct {
	code    string
	message string
	missing func(NewsletterRequest) bool
}

// requireJID é partilhado porque sete das onze operações pedem o mesmo canal:
// escrever a mesma verificação sete vezes seria sete sítios onde a mensagem de
// erro pode divergir.
var requireJID = newsletterRequirement{
	code:    "missing_jid",
	message: "jid do canal é obrigatório",
	missing: func(r NewsletterRequest) bool { return r.JID == "" },
}

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
		message: "nome do canal é obrigatório",
		missing: func(r NewsletterRequest) bool { return r.Name == "" },
	}},
	NewsletterOpInfoInvite: {{
		code:    "missing_invite",
		message: "código de convite é obrigatório",
		missing: func(r NewsletterRequest) bool { return r.Invite == "" },
	}},
	NewsletterOpMarkViewed: {requireJID, {
		code:    "missing_server_ids",
		message: "pelo menos um server_id é obrigatório",
		missing: func(r NewsletterRequest) bool { return len(r.ServerIDs) == 0 },
	}},
	// `reaction` NÃO entra na tabela: vazio REMOVE a reação, como no resto do
	// protocolo. Exigi-lo tornaria impossível desfazer pelo painel.
	NewsletterOpReact: {requireJID, {
		code:    "missing_server_id",
		message: "server_id é obrigatório",
		missing: func(r NewsletterRequest) bool { return r.ServerID == 0 },
	}},
	NewsletterOpInfo:      {requireJID},
	NewsletterOpFollow:    {requireJID},
	NewsletterOpUnfollow:  {requireJID},
	NewsletterOpMute:      {requireJID},
	NewsletterOpMessages:  {requireJID},
	NewsletterOpUpdates:   {requireJID},
	NewsletterOpSubscribe: {requireJID},
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
