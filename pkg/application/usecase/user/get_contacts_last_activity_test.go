package user_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/user"
	"wa-api/pkg/domain"
)

func newLastActivityUC(
	activityFn func(ctx context.Context, userID string) (map[string]time.Time, error),
	resolveFn func(ctx context.Context, txtID string, jids []domain.JID) (map[domain.JID]domain.JID, error),
) *user.GetContactsLastActivityUseCase {
	activity := &contractsfake.ChatActivityReader{GetLastActivityByUserFunc: activityFn}
	contacts := &contractsfake.ContactDirectory{GetManyLIDsForPNsFunc: resolveFn}
	return user.NewGetContactsLastActivityUseCase(activity, contacts, &contractsfake.Logger{})
}

// TestGetContactsLastActivityUseCase_ResolvePNToLID é o cenário que motivou
// a correção: whatsmeow_contacts (roster) devolve majoritariamente @lid,
// message_history mistura @lid e @s.whatsapp.net — sem normalizar, os dois
// nunca casam pelo mesmo JID/telefone (medido contra dados reais: 0/1141
// contatos casaram por telefone, só 5/284 por JID bruto).
func TestGetContactsLastActivityUseCase_ResolvePNToLID(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	uc := newLastActivityUC(
		func(context.Context, string) (map[string]time.Time, error) {
			return map[string]time.Time{"5511999@s.whatsapp.net": ts}, nil
		},
		func(_ context.Context, _ string, jids []domain.JID) (map[domain.JID]domain.JID, error) {
			if len(jids) != 1 || jids[0] != "5511999@s.whatsapp.net" {
				t.Fatalf("GetManyLIDsForPNs recebeu %v, queria só o PN não-resolvido", jids)
			}
			return map[domain.JID]domain.JID{"5511999@s.whatsapp.net": "123456@lid"}, nil
		},
	)
	got, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %+v, want 1 entrada", got)
	}
	if !got["123456@lid"].Equal(ts) {
		t.Fatalf("chave 123456@lid ausente/errada: %+v", got)
	}
	if _, stillPN := got["5511999@s.whatsapp.net"]; stillPN {
		t.Fatalf("chave PN não deveria sobreviver após resolução: %+v", got)
	}
}

// TestGetContactsLastActivityUseCase_PNSemLIDMantemOriginal — telefone sem
// mapeamento LID conhecido (ainda não sincronizado pelo app-state) não é
// descartado, fica com a chave PN original.
func TestGetContactsLastActivityUseCase_PNSemLIDMantemOriginal(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	uc := newLastActivityUC(
		func(context.Context, string) (map[string]time.Time, error) {
			return map[string]time.Time{"5511999@s.whatsapp.net": ts}, nil
		},
		func(context.Context, string, []domain.JID) (map[domain.JID]domain.JID, error) {
			return map[domain.JID]domain.JID{}, nil // nenhum mapeamento conhecido
		},
	)
	got, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !got["5511999@s.whatsapp.net"].Equal(ts) {
		t.Fatalf("PN sem LID deveria manter a chave original: %+v", got)
	}
}

// TestGetContactsLastActivityUseCase_LIDDiretoNaoPrecisaResolver — quando
// raw já não tem nenhum @s.whatsapp.net, GetManyLIDsForPNs nem é chamado
// (grupos/@lid direto passam sem round-trip extra).
func TestGetContactsLastActivityUseCase_LIDDiretoNaoPrecisaResolver(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	called := false
	uc := newLastActivityUC(
		func(context.Context, string) (map[string]time.Time, error) {
			return map[string]time.Time{"123456@lid": ts, "grupo@g.us": ts}, nil
		},
		func(context.Context, string, []domain.JID) (map[domain.JID]domain.JID, error) {
			called = true
			return nil, nil
		},
	)
	got, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if called {
		t.Fatal("GetManyLIDsForPNs não deveria ser chamado sem PN no resultado cru")
	}
	if len(got) != 2 {
		t.Fatalf("got %+v, want 2 entradas passthrough", got)
	}
}

// TestGetContactsLastActivityUseCase_GreatestNoConflitoLIDePN — mesmo
// contato aparece tanto por @lid direto (HistorySync já trouxe nesse
// espaço) quanto por PN resolvido pro MESMO LID: fica o timestamp mais
// recente dos dois, não uma sobrescrita arbitrária (a ordem de iteração de
// um map em Go não é determinística — é exatamente esse não-determinismo
// que o teste força a cobrir, repetindo a checagem N vezes).
func TestGetContactsLastActivityUseCase_GreatestNoConflitoLIDePN(t *testing.T) {
	older := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)

	for i := 0; i < 20; i++ {
		uc := newLastActivityUC(
			func(context.Context, string) (map[string]time.Time, error) {
				return map[string]time.Time{
					"123456@lid":              older,
					"5511999@s.whatsapp.net": newer,
				}, nil
			},
			func(context.Context, string, []domain.JID) (map[domain.JID]domain.JID, error) {
				return map[domain.JID]domain.JID{"5511999@s.whatsapp.net": "123456@lid"}, nil
			},
		)
		got, err := uc.Execute(context.Background(), "u1")
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("iteração %d: got %+v, want 1 entrada mesclada", i, got)
		}
		if !got["123456@lid"].Equal(newer) {
			t.Fatalf("iteração %d: 123456@lid = %v, want o mais recente (%v)", i, got["123456@lid"], newer)
		}
	}
}

// TestGetContactsLastActivityUseCase_ResolverFalhaDegradaSemNormalizar —
// erro do GetManyLIDsForPNs (ex.: sessão em standby) não derruba a chamada
// inteira, devolve o dado cru.
func TestGetContactsLastActivityUseCase_ResolverFalhaDegradaSemNormalizar(t *testing.T) {
	t.Parallel()
	ts := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	boom := errors.New("session standby")
	uc := newLastActivityUC(
		func(context.Context, string) (map[string]time.Time, error) {
			return map[string]time.Time{"5511999@s.whatsapp.net": ts}, nil
		},
		func(context.Context, string, []domain.JID) (map[domain.JID]domain.JID, error) {
			return nil, boom
		},
	)
	got, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatalf("Execute não deveria propagar erro do resolver: %v", err)
	}
	if !got["5511999@s.whatsapp.net"].Equal(ts) {
		t.Fatalf("deveria degradar pro dado cru: %+v", got)
	}
}

// TestGetContactsLastActivityUseCase_ReaderFalhaPropaga — erro do READ
// (message_history inacessível) é real e sobe, diferente do resolver.
func TestGetContactsLastActivityUseCase_ReaderFalhaPropaga(t *testing.T) {
	t.Parallel()
	boom := errors.New("db down")
	uc := newLastActivityUC(
		func(context.Context, string) (map[string]time.Time, error) {
			return nil, boom
		},
		nil,
	)
	_, err := uc.Execute(context.Background(), "u1")
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}
