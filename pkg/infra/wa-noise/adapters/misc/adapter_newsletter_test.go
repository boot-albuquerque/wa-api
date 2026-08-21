package misc

import (
	"context"
	"errors"
	"testing"
	"time"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/protocol/types"
	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"
)

// As onze operações de newsletter do adapter (paridade de 2026-08-20).
//
// O que estes testes travam não é "o método devolve nil": é que o JID chega ao
// SDK JÁ CONVERTIDO e que a falha da sessão é recusada ANTES de tocar no
// cliente. O modo de falha destas onze é o identificador — passar o código de
// convite onde ia o JID, ou não converter — e isso responde sucesso com a
// operação feita no canal errado.

const canalJID = "120363000000000000@newsletter"

func comCliente(f *testkit.Fake) *MiscAdapter {
	return NewMiscAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": f}))
}

func semCliente() *MiscAdapter { return NewMiscAdapter(testkit.GetterWith(nil)) }

// TestNewsletterAdapter_SemSessaoRecusaAntesDoSDK cobre as onze de uma vez: sem
// sessão, NENHUMA pode alcançar o cliente.
func TestNewsletterAdapter_SemSessaoRecusaAntesDoSDK(t *testing.T) {
	a := semCliente()
	ctx := context.Background()

	casos := map[string]func() error{
		"CreateNewsletter": func() error { _, err := a.CreateNewsletter(ctx, "u1", "n", "d", nil); return err },
		"NewsletterInfo":   func() error { _, err := a.NewsletterInfo(ctx, "u1", canalJID); return err },
		"InfoWithInvite":   func() error { _, err := a.NewsletterInfoWithInvite(ctx, "u1", "abc"); return err },
		"Follow":           func() error { return a.FollowNewsletter(ctx, "u1", canalJID) },
		"Unfollow":         func() error { return a.UnfollowNewsletter(ctx, "u1", canalJID) },
		"ToggleMute":       func() error { return a.ToggleNewsletterMute(ctx, "u1", canalJID, true) },
		"Messages":         func() error { _, err := a.NewsletterMessages(ctx, "u1", canalJID, 5, ""); return err },
		"MessageUpdates": func() error {
			_, err := a.NewsletterMessageUpdates(ctx, "u1", canalJID, 5, time.Time{}, "")
			return err
		},
		"MarkViewed":    func() error { return a.MarkNewsletterViewed(ctx, "u1", canalJID, []int{1}) },
		"SendReaction":  func() error { return a.SendNewsletterReaction(ctx, "u1", canalJID, 1, "x", "m") },
		"SubscribeLive": func() error { _, err := a.SubscribeNewsletterLiveUpdates(ctx, "u1", canalJID); return err },
	}
	if len(casos) != 11 {
		t.Fatalf("a família tem 11 operações, a tabela tem %d", len(casos))
	}
	for nome, chamar := range casos {
		if code := testkit.AppErrCode(chamar()); code != "no_session" {
			t.Errorf("%s: code = %q, quero no_session", nome, code)
		}
	}
}

// TestNewsletterAdapter_JIDChegaConvertidoAoSDK é o teste da CAUSA: o texto do
// domínio tem de virar types.JID antes de chegar ao cliente.
func TestNewsletterAdapter_JIDChegaConvertidoAoSDK(t *testing.T) {
	var visto types.JID
	fake := &testkit.Fake{
		FollowNewsletterFn: func(_ context.Context, jid types.JID) error { visto = jid; return nil },
	}
	if err := comCliente(fake).FollowNewsletter(context.Background(), "u1", canalJID); err != nil {
		t.Fatalf("FollowNewsletter = %v", err)
	}
	if visto.String() != canalJID {
		t.Fatalf("JID no SDK = %q, quero %q", visto.String(), canalJID)
	}
	if visto.Server != types.NewsletterServer {
		t.Fatalf("servidor = %q, quero %q — o JID não foi parseado", visto.Server, types.NewsletterServer)
	}
}

// TestNewsletterAdapter_ConviteNaoPassaPorToJID trava a divergência
// deliberada: o código de convite NÃO é um JID e não pode ser convertido.
func TestNewsletterAdapter_ConviteNaoPassaPorToJID(t *testing.T) {
	var visto string
	fake := &testkit.Fake{
		GetNewsletterInfoWithInviteFn: func(_ context.Context, key string) (*types.NewsletterMetadata, error) {
			visto = key
			return nil, nil
		},
	}
	if _, err := comCliente(fake).NewsletterInfoWithInvite(context.Background(), "u1", "AbCd1234"); err != nil {
		t.Fatalf("NewsletterInfoWithInvite = %v", err)
	}
	if visto != "AbCd1234" {
		t.Fatalf("convite no SDK = %q, quero %q — foi transformado pelo caminho", visto, "AbCd1234")
	}
}

// TestNewsletterAdapter_PropagaFalhaDoSDK confirma que a falha não é engolida.
func TestNewsletterAdapter_PropagaFalhaDoSDK(t *testing.T) {
	boom := errors.New("sdk-boom")
	fake := &testkit.Fake{
		UnfollowNewsletterFn: func(_ context.Context, _ types.JID) error { return boom },
	}
	err := comCliente(fake).UnfollowNewsletter(context.Background(), "u1", canalJID)
	if !errors.Is(err, boom) {
		t.Fatalf("erro = %v, quero envolver %v", err, boom)
	}
}

// TestNewsletterAdapter_CreatePassaOsCamposDeCriacao cobre o único método cujo
// pedido é um struct de parâmetros e não um JID.
func TestNewsletterAdapter_CreatePassaOsCamposDeCriacao(t *testing.T) {
	var p wanoise.CreateNewsletterParams
	fake := &testkit.Fake{
		CreateNewsletterFn: func(_ context.Context, params wanoise.CreateNewsletterParams) (*types.NewsletterMetadata, error) {
			p = params
			return nil, nil
		},
	}
	pic := []byte{1, 2, 3}
	if _, err := comCliente(fake).CreateNewsletter(context.Background(), "u1", "Canal X", "desc", pic); err != nil {
		t.Fatalf("CreateNewsletter = %v", err)
	}
	if p.Name != "Canal X" || p.Description != "desc" || len(p.Picture) != 3 {
		t.Fatalf("params = %+v, quero Name/Description/Picture preenchidos", p)
	}
}

// TestNewsletterAdapter_ServerIDDeTextoInvalidoVaiZero trava a decisão
// documentada: id ilegível vira 0, que é "omitir o atributo", e NÃO um erro.
func TestNewsletterAdapter_ServerIDDeTextoInvalidoVaiZero(t *testing.T) {
	var got *wanoise.GetNewsletterMessagesParams
	fake := &testkit.Fake{
		GetNewsletterMessagesFn: func(_ context.Context, _ types.JID, params *wanoise.GetNewsletterMessagesParams) ([]*types.NewsletterMessage, error) {
			got = params
			return nil, nil
		},
	}
	if _, err := comCliente(fake).NewsletterMessages(context.Background(), "u1", canalJID, 7, "nao-e-numero"); err != nil {
		t.Fatalf("NewsletterMessages = %v", err)
	}
	if got == nil || got.Count != 7 {
		t.Fatalf("params = %+v, quero Count=7", got)
	}
	if got.Before != 0 {
		t.Fatalf("Before = %d, quero 0 — texto ilegível tem de virar 0", got.Before)
	}
}

// TestNewsletterAdapter_CaminhoDeSucessoDasOnze existe por causa da armadilha
// nº2 do ARMADILHAS.md: três defeitos deste repositório viviam atrás de suítes
// que só exercitavam a GUARDA. O teste acima cobre a recusa das onze; este
// cobre o SUCESSO das onze, e afirma que o SDK foi mesmo chamado.
func TestNewsletterAdapter_CaminhoDeSucessoDasOnze(t *testing.T) {
	ctx := context.Background()
	var chamado string

	fake := &testkit.Fake{
		CreateNewsletterFn: func(context.Context, wanoise.CreateNewsletterParams) (*types.NewsletterMetadata, error) {
			chamado = "create"
			return nil, nil
		},
		GetNewsletterInfoFn: func(context.Context, types.JID) (*types.NewsletterMetadata, error) {
			chamado = "info"
			return nil, nil
		},
		GetNewsletterInfoWithInviteFn: func(context.Context, string) (*types.NewsletterMetadata, error) {
			chamado = "invite"
			return nil, nil
		},
		FollowNewsletterFn:     func(context.Context, types.JID) error { chamado = "follow"; return nil },
		UnfollowNewsletterFn:   func(context.Context, types.JID) error { chamado = "unfollow"; return nil },
		NewsletterToggleMuteFn: func(context.Context, types.JID, bool) error { chamado = "mute"; return nil },
		GetNewsletterMessagesFn: func(context.Context, types.JID, *wanoise.GetNewsletterMessagesParams) ([]*types.NewsletterMessage, error) {
			chamado = "messages"
			return nil, nil
		},
		GetNewsletterMessageUpdatesFn: func(context.Context, types.JID, *wanoise.GetNewsletterUpdatesParams) ([]*types.NewsletterMessage, error) {
			chamado = "updates"
			return nil, nil
		},
		NewsletterMarkViewedFn: func(context.Context, types.JID, []types.MessageServerID) error {
			chamado = "markviewed"
			return nil
		},
		NewsletterSendReactionFn: func(context.Context, types.JID, types.MessageServerID, string, types.MessageID) error {
			chamado = "react"
			return nil
		},
		NewsletterSubscribeLiveUpdatesFn: func(context.Context, types.JID) (time.Duration, error) {
			chamado = "subscribe"
			return 42 * time.Second, nil
		},
	}
	a := comCliente(fake)

	casos := []struct {
		nome   string
		chamar func() error
		quero  string
	}{
		{"create", func() error { _, err := a.CreateNewsletter(ctx, "u1", "n", "d", nil); return err }, "create"},
		{"info", func() error { _, err := a.NewsletterInfo(ctx, "u1", canalJID); return err }, "info"},
		{"invite", func() error { _, err := a.NewsletterInfoWithInvite(ctx, "u1", "k"); return err }, "invite"},
		{"follow", func() error { return a.FollowNewsletter(ctx, "u1", canalJID) }, "follow"},
		{"unfollow", func() error { return a.UnfollowNewsletter(ctx, "u1", canalJID) }, "unfollow"},
		{"mute", func() error { return a.ToggleNewsletterMute(ctx, "u1", canalJID, true) }, "mute"},
		{"messages", func() error { _, err := a.NewsletterMessages(ctx, "u1", canalJID, 3, "1"); return err }, "messages"},
		{"updates", func() error {
			_, err := a.NewsletterMessageUpdates(ctx, "u1", canalJID, 3, time.Unix(0, 0), "2")
			return err
		}, "updates"},
		{"markviewed", func() error { return a.MarkNewsletterViewed(ctx, "u1", canalJID, []int{1, 2}) }, "markviewed"},
		{"react", func() error { return a.SendNewsletterReaction(ctx, "u1", canalJID, 3, "👍", "m1") }, "react"},
		{"subscribe", func() error { _, err := a.SubscribeNewsletterLiveUpdates(ctx, "u1", canalJID); return err }, "subscribe"},
	}
	if len(casos) != 11 {
		t.Fatalf("a família tem 11 operações, a tabela tem %d", len(casos))
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			chamado = ""
			if err := c.chamar(); err != nil {
				t.Fatalf("%s = %v, quero sucesso", c.nome, err)
			}
			if chamado != c.quero {
				t.Fatalf("SDK chamado = %q, quero %q — a operação não alcançou o cliente", chamado, c.quero)
			}
		})
	}
}

// TestNewsletterAdapter_SubscribeDevolveADuracao trava o único retorno que não
// é dado nem erro: a duração da subscrição. Perdê-la faria o chamador achar
// que a subscrição é instantânea.
func TestNewsletterAdapter_SubscribeDevolveADuracao(t *testing.T) {
	fake := &testkit.Fake{
		NewsletterSubscribeLiveUpdatesFn: func(context.Context, types.JID) (time.Duration, error) {
			return 90 * time.Second, nil
		},
	}
	got, err := comCliente(fake).SubscribeNewsletterLiveUpdates(context.Background(), "u1", canalJID)
	if err != nil {
		t.Fatalf("SubscribeNewsletterLiveUpdates = %v", err)
	}
	if got != 90*time.Second {
		t.Fatalf("duração = %v, quero 90s", got)
	}
}
