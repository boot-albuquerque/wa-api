package testkit

import (
	"context"
	"testing"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/protocol/types"
)

func jidVazio() types.JID { return types.NewJID("55119", types.DefaultUserServer) }

func wanoiseCreateParams() wanoise.CreateNewsletterParams {
	return wanoise.CreateNewsletterParams{Name: "n"}
}

// O dublê tem, em cada método, um ramo com função injetada e um ramo por
// omissão. Os testes que usam o dublê injetam sempre a função, então o ramo
// por omissão nunca era exercitado — e um dublê cujo comportamento padrão
// ninguém testou é um dublê que MAY estar errado sem que se saiba.
//
// O que se assere é o CONTRATO do padrão: devolver zero-value e NIL, nunca
// entrar em pânico. Um dublê que rebenta sem função injetada obriga todo teste
// futuro a injetar tudo, mesmo o que não lhe interessa.
func TestFakeNewsletter_SemFuncaoInjetadaDevolveZeroSemPanico(t *testing.T) {
	f := &Fake{}
	ctx := context.Background()

	casos := map[string]func() error{
		"CreateNewsletter":               func() error { _, err := f.CreateNewsletter(ctx, wanoiseCreateParams()); return err },
		"GetNewsletterInfo":              func() error { _, err := f.GetNewsletterInfo(ctx, jidVazio()); return err },
		"GetNewsletterInfoWithInvite":    func() error { _, err := f.GetNewsletterInfoWithInvite(ctx, "k"); return err },
		"FollowNewsletter":               func() error { return f.FollowNewsletter(ctx, jidVazio()) },
		"UnfollowNewsletter":             func() error { return f.UnfollowNewsletter(ctx, jidVazio()) },
		"NewsletterToggleMute":           func() error { return f.NewsletterToggleMute(ctx, jidVazio(), true) },
		"GetNewsletterMessages":          func() error { _, err := f.GetNewsletterMessages(ctx, jidVazio(), nil); return err },
		"GetNewsletterMessageUpdates":    func() error { _, err := f.GetNewsletterMessageUpdates(ctx, jidVazio(), nil); return err },
		"NewsletterMarkViewed":           func() error { return f.NewsletterMarkViewed(ctx, jidVazio(), nil) },
		"NewsletterSendReaction":         func() error { return f.NewsletterSendReaction(ctx, jidVazio(), 1, "x", "m") },
		"NewsletterSubscribeLiveUpdates": func() error { _, err := f.NewsletterSubscribeLiveUpdates(ctx, jidVazio()); return err },
		"NewsletterDemoteAdmin":          func() error { return f.NewsletterDemoteAdmin(ctx, jidVazio(), jidVazio()) },
		"NewsletterChangeOwner":          func() error { return f.NewsletterChangeOwner(ctx, jidVazio(), jidVazio()) },
		"NewsletterDelete":               func() error { return f.NewsletterDelete(ctx, jidVazio()) },
		"SetStatusMessage":               func() error { return f.SetStatusMessage(ctx, "olá") },
		"SendPeerMessage":                func() error { _, err := f.SendPeerMessage(ctx, nil); return err },
	}
	if len(casos) != 16 {
		t.Fatalf("a família são 16 métodos, a tabela tem %d", len(casos))
	}

	for nome, chamar := range casos {
		t.Run(nome, func(t *testing.T) {
			if err := chamar(); err != nil {
				t.Fatalf("%s sem função injetada devolveu erro %v, quero nil", nome, err)
			}
		})
	}
}

// TestFakeNewsletter_BuildHistorySyncRequestNaoDevolveNil: este é o único que
// devolve um ponteiro em vez de erro, e devolver nil aqui faria o adaptador
// recusar com "could not build" num teste que só queria um dublê inerte.
func TestFakeNewsletter_BuildHistorySyncRequestNaoDevolveNil(t *testing.T) {
	if got := (&Fake{}).BuildHistorySyncRequest(nil, 10); got == nil {
		t.Fatal("BuildHistorySyncRequest sem função injetada devolveu nil")
	}
}
