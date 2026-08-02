// contractsfake_test.go — o pacote de dublês é ele próprio julgado.
//
// Três perguntas por fake, e são as únicas que importam para quem o consome:
// (1) o zero-value é utilizável e devolve o que a doc promete; (2) a Func
// configurada substitui esse padrão; (3) a chamada foi gravada com os
// argumentos que chegaram.
package contractsfake_test

import (
	"context"
	"errors"
	"testing"
	"time"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/domain"
)

var errBoom = errors.New("boom")

// --- Logger ------------------------------------------------------------

func TestLoggerCapturaNivelMensagemEKeyvals(t *testing.T) {
	f := &contractsfake.Logger{}
	type ctxKey string
	ctx := context.WithValue(context.Background(), ctxKey("k"), "v")

	f.Info(ctx, "criado", "user_id", "u1")
	f.Warn(ctx, "sem sessao", "user_id", "u1", "reason", "offline")
	f.Error(ctx, "falhou", "err", errBoom)

	recs := f.Records()
	if len(recs) != 3 {
		t.Fatalf("Records() = %d, quero 3", len(recs))
	}
	if f.Len() != 3 {
		t.Errorf("Len() = %d, quero 3", f.Len())
	}

	want := []struct {
		level string
		msg   string
		n     int
	}{
		{contractsfake.LevelInfo, "criado", 2},
		{contractsfake.LevelWarn, "sem sessao", 4},
		{contractsfake.LevelError, "falhou", 2},
	}
	for i, w := range want {
		if recs[i].Level != w.level {
			t.Errorf("recs[%d].Level = %q, quero %q", i, recs[i].Level, w.level)
		}
		if recs[i].Msg != w.msg {
			t.Errorf("recs[%d].Msg = %q, quero %q", i, recs[i].Msg, w.msg)
		}
		if len(recs[i].Keyvals) != w.n {
			t.Errorf("recs[%d].Keyvals = %d, quero %d", i, len(recs[i].Keyvals), w.n)
		}
		if recs[i].Ctx != ctx {
			t.Errorf("recs[%d].Ctx nao e' o ctx passado", i)
		}
	}
}

func TestLogRecordIsStructuredSegueL3(t *testing.T) {
	cases := []struct {
		name    string
		keyvals []any
		want    bool
	}{
		{"vazio", nil, false},
		{"um so", []any{"k"}, false},
		{"par minimo", []any{"k", "v"}, true},
		{"impar com tres", []any{"a", 1, "b"}, false},
		{"dois pares", []any{"a", 1, "b", 2}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := contractsfake.LogRecord{Keyvals: tc.keyvals}
			if got := r.IsStructured(); got != tc.want {
				t.Errorf("IsStructured() = %v, quero %v", got, tc.want)
			}
		})
	}
}

func TestLogRecordKeyval(t *testing.T) {
	f := &contractsfake.Logger{}
	f.Warn(context.Background(), "m", "user_id", "u1", 42, "chave-nao-string", "user_id", "duplicata")

	r, ok := f.Find("m")
	if !ok {
		t.Fatal("Find(m) nao achou")
	}
	v, ok := r.Keyval("user_id")
	if !ok || v != "u1" {
		t.Errorf(`Keyval("user_id") = %v, %v; quero "u1", true (primeira ocorrencia)`, v, ok)
	}
	if !r.HasKey("user_id") {
		t.Error("HasKey(user_id) = false")
	}
	if r.HasKey("ausente") {
		t.Error("HasKey(ausente) = true")
	}
	if _, ok := r.Keyval("chave-nao-string"); ok {
		t.Error("valor de chave nao-string nao devia casar como chave")
	}
}

func TestLoggerBuscaENiveis(t *testing.T) {
	f := &contractsfake.Logger{}
	f.Info(context.Background(), "a", "k", 1)
	f.Warn(context.Background(), "b", "k", 2)
	f.Warn(context.Background(), "a", "k", 3)

	if got := len(f.ByLevel(contractsfake.LevelWarn)); got != 2 {
		t.Errorf("ByLevel(warn) = %d, quero 2", got)
	}
	if got := len(f.ByLevel(contractsfake.LevelError)); got != 0 {
		t.Errorf("ByLevel(error) = %d, quero 0", got)
	}

	msgs := f.Messages()
	if len(msgs) != 3 || msgs[0] != "a" || msgs[2] != "a" {
		t.Errorf("Messages() = %v", msgs)
	}

	// Find devolve o PRIMEIRO "a", que e' info.
	if r, _ := f.Find("a"); r.Level != contractsfake.LevelInfo {
		t.Errorf("Find(a).Level = %q, quero info", r.Level)
	}
	// FindLevel discrimina.
	r, ok := f.FindLevel(contractsfake.LevelWarn, "a")
	if !ok {
		t.Fatal("FindLevel(warn, a) nao achou")
	}
	if v, _ := r.Keyval("k"); v != 3 {
		t.Errorf("FindLevel(warn, a) trouxe o registro errado: k = %v", v)
	}
	if _, ok := f.FindLevel(contractsfake.LevelError, "a"); ok {
		t.Error("FindLevel(error, a) achou o que nao existe")
	}
	if _, ok := f.Find("inexistente"); ok {
		t.Error("Find(inexistente) achou")
	}
	if !f.Logged("b") || f.Logged("z") {
		t.Error("Logged mentiu")
	}

	last, ok := f.Last()
	if !ok || last.Msg != "a" || last.Level != contractsfake.LevelWarn {
		t.Errorf("Last() = %+v, %v", last, ok)
	}
}

func TestLoggerVazio(t *testing.T) {
	f := &contractsfake.Logger{}
	if _, ok := f.Last(); ok {
		t.Error("Last() em logger vazio devolveu ok")
	}
	if f.Len() != 0 || len(f.Messages()) != 0 {
		t.Error("logger vazio nao esta' vazio")
	}
}

func TestLoggerResetEIsolamentoDeKeyvals(t *testing.T) {
	f := &contractsfake.Logger{}

	// O slice do chamador e' reaproveitado — o registro nao pode aliasa-lo.
	kv := []any{"k", "antes"}
	f.Info(context.Background(), "m", kv...)
	kv[1] = "depois"
	if r, _ := f.Find("m"); r.Keyvals[1] != "antes" {
		t.Errorf("keyvals aliasaram o slice do chamador: %v", r.Keyvals[1])
	}

	f.Reset()
	if f.Len() != 0 {
		t.Errorf("apos Reset, Len() = %d", f.Len())
	}
	f.Info(context.Background(), "depois-do-reset")
	if f.Len() != 1 {
		t.Errorf("Reset quebrou a gravacao seguinte: Len() = %d", f.Len())
	}
}

func TestLoggerFuncsRodamDepoisDaGravacao(t *testing.T) {
	f := &contractsfake.Logger{}
	var seen []string
	hook := func(name string) func(context.Context, string, ...any) {
		return func(_ context.Context, msg string, _ ...any) {
			// A gravacao ja' aconteceu quando o hook roda.
			if f.Len() == 0 {
				t.Errorf("%s: hook rodou ANTES da gravacao", name)
			}
			seen = append(seen, name+":"+msg)
		}
	}
	f.InfoFunc, f.WarnFunc, f.ErrorFunc = hook("info"), hook("warn"), hook("error")

	f.Info(context.Background(), "i")
	f.Warn(context.Background(), "w")
	f.Error(context.Background(), "e")

	want := []string{"info:i", "warn:w", "error:e"}
	if len(seen) != len(want) {
		t.Fatalf("seen = %v", seen)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("seen[%d] = %q, quero %q", i, seen[i], want[i])
		}
	}
}

func TestLoggerConcorrente(t *testing.T) {
	f := &contractsfake.Logger{}
	const n = 50
	done := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() {
			f.Warn(context.Background(), "concorrente", "k", "v")
			done <- struct{}{}
		}()
	}
	for i := 0; i < n; i++ {
		<-done
	}
	if f.Len() != n {
		t.Errorf("Len() = %d, quero %d", f.Len(), n)
	}
}

// --- SessionGuard e derivadas ------------------------------------------

func TestSessionGuard(t *testing.T) {
	f := &contractsfake.SessionGuard{}
	if err := f.EnsureSession(context.Background(), "u1"); err != nil {
		t.Errorf("zero-value devia aceitar a sessao, deu %v", err)
	}

	f.EnsureSessionFunc = func(_ context.Context, txtID string) error {
		if txtID == "u2" {
			return errBoom
		}
		return nil
	}
	if err := f.EnsureSession(context.Background(), "u2"); !errors.Is(err, errBoom) {
		t.Errorf("EnsureSession(u2) = %v, quero errBoom", err)
	}

	if len(f.EnsureSessionCalls) != 2 {
		t.Fatalf("EnsureSessionCalls = %d, quero 2", len(f.EnsureSessionCalls))
	}
	if f.EnsureSessionCalls[0].TxtID != "u1" || f.EnsureSessionCalls[1].TxtID != "u2" {
		t.Errorf("argumentos gravados errados: %+v", f.EnsureSessionCalls)
	}
}

func TestFailSession(t *testing.T) {
	g := contractsfake.FailSession(errBoom)
	if err := g.EnsureSession(context.Background(), "u1"); !errors.Is(err, errBoom) {
		t.Errorf("FailSession nao recusou: %v", err)
	}
}

// TestSessionGuardEmbutidoEPromovido prova a forma que os 8 pacotes de use
// case vao usar: configurar a sessao de uma porta de capacidade.
func TestSessionGuardEmbutidoEPromovido(t *testing.T) {
	f := &contractsfake.GroupDirectory{SessionGuard: contractsfake.FailSession(errBoom)}
	if err := f.EnsureSession(context.Background(), "u1"); !errors.Is(err, errBoom) {
		t.Errorf("literal aninhado nao propagou: %v", err)
	}

	g := &contractsfake.MessageComposer{}
	g.EnsureSessionFunc = func(context.Context, string) error { return errBoom }
	if err := g.EnsureSession(context.Background(), "u1"); !errors.Is(err, errBoom) {
		t.Errorf("campo promovido nao propagou: %v", err)
	}
	if len(g.EnsureSessionCalls) != 1 {
		t.Errorf("EnsureSessionCalls promovido = %d, quero 1", len(g.EnsureSessionCalls))
	}
}

func TestSessionStatusReader(t *testing.T) {
	f := &contractsfake.SessionStatusReader{}
	if c, l := f.SessionStatus(context.Background(), "u1"); c || l {
		t.Errorf("zero-value = %v, %v; quero false, false", c, l)
	}
	f.SessionStatusFunc = func(context.Context, string) (bool, bool) { return true, true }
	if c, l := f.SessionStatus(context.Background(), "u2"); !c || !l {
		t.Errorf("Func = %v, %v; quero true, true", c, l)
	}
	if len(f.SessionStatusCalls) != 2 || f.SessionStatusCalls[1].UserID != "u2" {
		t.Errorf("calls = %+v", f.SessionStatusCalls)
	}
}

func TestSessionController(t *testing.T) {
	f := &contractsfake.SessionController{}
	if err := f.Logout(context.Background(), "u1"); err != nil {
		t.Errorf("Logout zero-value = %v", err)
	}
	if err := f.Disconnect(context.Background(), "u1"); err != nil {
		t.Errorf("Disconnect zero-value = %v", err)
	}

	f.LogoutFunc = func(context.Context, string) error { return errBoom }
	f.DisconnectFunc = func(context.Context, string) error { return errBoom }
	if err := f.Logout(context.Background(), "u2"); !errors.Is(err, errBoom) {
		t.Errorf("LogoutFunc = %v", err)
	}
	if err := f.Disconnect(context.Background(), "u2"); !errors.Is(err, errBoom) {
		t.Errorf("DisconnectFunc = %v", err)
	}
	if len(f.LogoutCalls) != 2 || len(f.DisconnectCalls) != 2 {
		t.Errorf("calls = %d, %d", len(f.LogoutCalls), len(f.DisconnectCalls))
	}

	// As duas portas embutidas continuam funcionais.
	if err := f.EnsureSession(context.Background(), "u1"); err != nil {
		t.Errorf("EnsureSession embutido = %v", err)
	}
	if c, _ := f.SessionStatus(context.Background(), "u1"); c {
		t.Error("SessionStatus embutido devia ser false")
	}
}

func TestSessionCounter(t *testing.T) {
	f := &contractsfake.SessionCounter{}
	got, err := f.CountSessions(context.Background())
	if err != nil || got != (domain.SessionCounts{}) {
		t.Errorf("zero-value = %+v, %v", got, err)
	}
	f.CountSessionsFunc = func(context.Context) (domain.SessionCounts, error) {
		return domain.SessionCounts{}, errBoom
	}
	if _, err := f.CountSessions(context.Background()); !errors.Is(err, errBoom) {
		t.Errorf("Func = %v", err)
	}
	if len(f.CountSessionsCalls) != 2 {
		t.Errorf("calls = %d", len(f.CountSessionsCalls))
	}
}

// --- Chat --------------------------------------------------------------

func TestJIDResolverPadraoEhIdentidade(t *testing.T) {
	f := &contractsfake.JIDResolver{}
	j, err := f.ResolveJID(context.Background(), "5511999@s.whatsapp.net")
	if err != nil || j != domain.JID("5511999@s.whatsapp.net") {
		t.Errorf("ResolveJID = %q, %v", j, err)
	}
	q, err := f.ResolveQualifiedJID(context.Background(), "x@g.us")
	if err != nil || q != domain.JID("x@g.us") {
		t.Errorf("ResolveQualifiedJID = %q, %v", q, err)
	}

	f.ResolveJIDFunc = func(context.Context, string) (domain.JID, error) { return "", errBoom }
	f.ResolveQualifiedJIDFunc = func(context.Context, string) (domain.JID, error) { return "", errBoom }
	if _, err := f.ResolveJID(context.Background(), "lixo"); !errors.Is(err, errBoom) {
		t.Errorf("ResolveJIDFunc = %v", err)
	}
	if _, err := f.ResolveQualifiedJID(context.Background(), "lixo"); !errors.Is(err, errBoom) {
		t.Errorf("ResolveQualifiedJIDFunc = %v", err)
	}
	if len(f.ResolveJIDCalls) != 2 || f.ResolveJIDCalls[1].Raw != "lixo" {
		t.Errorf("ResolveJIDCalls = %+v", f.ResolveJIDCalls)
	}
	if len(f.ResolveQualifiedJIDCalls) != 2 || f.ResolveQualifiedJIDCalls[1].Raw != "lixo" {
		t.Errorf("ResolveQualifiedJIDCalls = %+v", f.ResolveQualifiedJIDCalls)
	}
}

func TestPresenceController(t *testing.T) {
	f := &contractsfake.PresenceController{}
	ctx := context.Background()
	if err := f.SendPresence(ctx, "u1", domain.PresenceAvailable); err != nil {
		t.Errorf("SendPresence = %v", err)
	}
	if err := f.SendChatPresence(ctx, "u1", "c@s", "composing", "text"); err != nil {
		t.Errorf("SendChatPresence = %v", err)
	}
	if err := f.SubscribePresence(ctx, "u1", "t@s"); err != nil {
		t.Errorf("SubscribePresence = %v", err)
	}

	if c := f.SendPresenceCalls[0]; c.TxtID != "u1" || c.Presence != domain.PresenceAvailable {
		t.Errorf("SendPresenceCalls[0] = %+v", c)
	}
	if c := f.SendChatPresenceCalls[0]; c.Chat != "c@s" || c.State != "composing" || c.Media != "text" {
		t.Errorf("SendChatPresenceCalls[0] = %+v", c)
	}
	if c := f.SubscribePresenceCalls[0]; c.Target != "t@s" {
		t.Errorf("SubscribePresenceCalls[0] = %+v", c)
	}

	f.SendPresenceFunc = func(context.Context, string, domain.PresenceType) error { return errBoom }
	f.SendChatPresenceFunc = func(context.Context, string, domain.JID, string, string) error { return errBoom }
	f.SubscribePresenceFunc = func(context.Context, string, domain.JID) error { return errBoom }
	if err := f.SendPresence(ctx, "u1", domain.PresenceUnavailable); !errors.Is(err, errBoom) {
		t.Errorf("SendPresenceFunc = %v", err)
	}
	if err := f.SendChatPresence(ctx, "u1", "c@s", "", ""); !errors.Is(err, errBoom) {
		t.Errorf("SendChatPresenceFunc = %v", err)
	}
	if err := f.SubscribePresence(ctx, "u1", "t@s"); !errors.Is(err, errBoom) {
		t.Errorf("SubscribePresenceFunc = %v", err)
	}
}

func TestChatMessenger(t *testing.T) {
	f := &contractsfake.ChatMessenger{}
	ctx := context.Background()
	at := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

	if err := f.MarkRead(ctx, "u1", []string{"m1", "m2"}, at, "c@s", "s@s"); err != nil {
		t.Errorf("MarkRead = %v", err)
	}
	res, err := f.SendReaction(ctx, "u1", "t@s", domain.Reaction{})
	if err != nil || res != (domain.MessageSendResult{}) {
		t.Errorf("SendReaction zero-value = %+v, %v", res, err)
	}

	c := f.MarkReadCalls[0]
	if len(c.IDs) != 2 || c.At != at || c.Chat != "c@s" || c.Sender != "s@s" {
		t.Errorf("MarkReadCalls[0] = %+v", c)
	}
	if f.SendReactionCalls[0].Target != "t@s" {
		t.Errorf("SendReactionCalls[0] = %+v", f.SendReactionCalls[0])
	}

	f.MarkReadFunc = func(context.Context, string, []string, time.Time, domain.JID, domain.JID) error {
		return errBoom
	}
	f.SendReactionFunc = func(context.Context, string, domain.JID, domain.Reaction) (domain.MessageSendResult, error) {
		return domain.MessageSendResult{}, errBoom
	}
	if err := f.MarkRead(ctx, "u1", nil, at, "", ""); !errors.Is(err, errBoom) {
		t.Errorf("MarkReadFunc = %v", err)
	}
	if _, err := f.SendReaction(ctx, "u1", "", domain.Reaction{}); !errors.Is(err, errBoom) {
		t.Errorf("SendReactionFunc = %v", err)
	}
}

func TestChatOperations(t *testing.T) {
	f := &contractsfake.ChatOperations{}
	ctx := context.Background()

	if err := f.ArchiveChat(ctx, "u1", "c@s", true); err != nil {
		t.Errorf("ArchiveChat = %v", err)
	}
	if err := f.RejectCall(ctx, "u1", "f@s", "call-1"); err != nil {
		t.Errorf("RejectCall = %v", err)
	}
	ack, err := f.RequestUnavailableMessage(ctx, "u1", "c@s", "s@s", "m1")
	if err != nil || ack != (domain.UnavailableMessageAck{}) {
		t.Errorf("RequestUnavailableMessage zero-value = %+v, %v", ack, err)
	}

	if c := f.ArchiveChatCalls[0]; c.Chat != "c@s" || !c.Archive {
		t.Errorf("ArchiveChatCalls[0] = %+v", c)
	}
	if c := f.RejectCallCalls[0]; c.From != "f@s" || c.CallID != "call-1" {
		t.Errorf("RejectCallCalls[0] = %+v", c)
	}
	if c := f.RequestUnavailableMessageCalls[0]; c.Sender != "s@s" || c.MessageID != "m1" {
		t.Errorf("RequestUnavailableMessageCalls[0] = %+v", c)
	}

	f.ArchiveChatFunc = func(context.Context, string, domain.JID, bool) error { return errBoom }
	f.RejectCallFunc = func(context.Context, string, domain.JID, string) error { return errBoom }
	f.RequestUnavailableMessageFunc = func(context.Context, string, domain.JID, domain.JID, string) (domain.UnavailableMessageAck, error) {
		return domain.UnavailableMessageAck{}, errBoom
	}
	if err := f.ArchiveChat(ctx, "u1", "", false); !errors.Is(err, errBoom) {
		t.Errorf("ArchiveChatFunc = %v", err)
	}
	if err := f.RejectCall(ctx, "u1", "", ""); !errors.Is(err, errBoom) {
		t.Errorf("RejectCallFunc = %v", err)
	}
	if _, err := f.RequestUnavailableMessage(ctx, "u1", "", "", ""); !errors.Is(err, errBoom) {
		t.Errorf("RequestUnavailableMessageFunc = %v", err)
	}
}

func TestNewsletterReader(t *testing.T) {
	f := &contractsfake.NewsletterReader{}
	got, err := f.ListSubscribed(context.Background(), "u1")
	if err != nil || got != nil {
		t.Errorf("zero-value = %v, %v", got, err)
	}
	f.ListSubscribedFunc = func(context.Context, string) (any, error) { return []string{"n1"}, nil }
	got, err = f.ListSubscribed(context.Background(), "u2")
	if err != nil {
		t.Fatalf("Func = %v", err)
	}
	if list, ok := got.([]string); !ok || len(list) != 1 {
		t.Errorf("Func devolveu %#v", got)
	}
	if len(f.ListSubscribedCalls) != 2 || f.ListSubscribedCalls[1].TxtID != "u2" {
		t.Errorf("calls = %+v", f.ListSubscribedCalls)
	}
}

// --- Group -------------------------------------------------------------

func TestGroupDirectory(t *testing.T) {
	f := &contractsfake.GroupDirectory{}
	ctx := context.Background()

	if v, err := f.GetGroupInfo(ctx, "u1", "g@g.us"); v != nil || err != nil {
		t.Errorf("GetGroupInfo zero-value = %v, %v", v, err)
	}
	if v, err := f.GetGroupInfoFromLink(ctx, "u1", "code"); v != nil || err != nil {
		t.Errorf("GetGroupInfoFromLink zero-value = %v, %v", v, err)
	}
	if s, err := f.GetGroupInviteLink(ctx, "u1", "g@g.us"); s != "" || err != nil {
		t.Errorf("GetGroupInviteLink zero-value = %q, %v", s, err)
	}
	if v, n, err := f.ListJoinedGroups(ctx, "u1"); v != nil || n != 0 || err != nil {
		t.Errorf("ListJoinedGroups zero-value = %v, %d, %v", v, n, err)
	}

	if f.GetGroupInfoCalls[0].Group != "g@g.us" {
		t.Errorf("GetGroupInfoCalls = %+v", f.GetGroupInfoCalls)
	}
	if f.GetGroupInfoFromLinkCalls[0].Code != "code" {
		t.Errorf("GetGroupInfoFromLinkCalls = %+v", f.GetGroupInfoFromLinkCalls)
	}
	if f.GetGroupInviteLinkCalls[0].TxtID != "u1" {
		t.Errorf("GetGroupInviteLinkCalls = %+v", f.GetGroupInviteLinkCalls)
	}
	if f.ListJoinedGroupsCalls[0].TxtID != "u1" {
		t.Errorf("ListJoinedGroupsCalls = %+v", f.ListJoinedGroupsCalls)
	}

	f.GetGroupInfoFunc = func(context.Context, string, domain.JID) (any, error) { return "info", nil }
	f.GetGroupInfoFromLinkFunc = func(context.Context, string, string) (any, error) { return nil, errBoom }
	f.GetGroupInviteLinkFunc = func(context.Context, string, domain.JID) (string, error) { return "https://l", nil }
	f.ListJoinedGroupsFunc = func(context.Context, string) (any, int, error) { return []int{1, 2}, 2, nil }

	if v, _ := f.GetGroupInfo(ctx, "u1", ""); v != "info" {
		t.Errorf("GetGroupInfoFunc = %v", v)
	}
	if _, err := f.GetGroupInfoFromLink(ctx, "u1", ""); !errors.Is(err, errBoom) {
		t.Errorf("GetGroupInfoFromLinkFunc = %v", err)
	}
	if s, _ := f.GetGroupInviteLink(ctx, "u1", ""); s != "https://l" {
		t.Errorf("GetGroupInviteLinkFunc = %q", s)
	}
	if _, n, _ := f.ListJoinedGroups(ctx, "u1"); n != 2 {
		t.Errorf("ListJoinedGroupsFunc n = %d", n)
	}
}

func TestGroupLifecycle(t *testing.T) {
	f := &contractsfake.GroupLifecycle{}
	ctx := context.Background()
	parts := []domain.JID{"a@s", "b@s"}

	if v, err := f.CreateGroup(ctx, "u1", "meu grupo", parts); v != nil || err != nil {
		t.Errorf("CreateGroup zero-value = %v, %v", v, err)
	}
	if v, err := f.JoinGroup(ctx, "u1", "code"); v != nil || err != nil {
		t.Errorf("JoinGroup zero-value = %v, %v", v, err)
	}
	if err := f.LeaveGroup(ctx, "u1", "g@g.us"); err != nil {
		t.Errorf("LeaveGroup zero-value = %v", err)
	}

	if c := f.CreateGroupCalls[0]; c.Name != "meu grupo" || len(c.Participants) != 2 {
		t.Errorf("CreateGroupCalls[0] = %+v", c)
	}
	if f.JoinGroupCalls[0].Code != "code" {
		t.Errorf("JoinGroupCalls = %+v", f.JoinGroupCalls)
	}
	if f.LeaveGroupCalls[0].Group != "g@g.us" {
		t.Errorf("LeaveGroupCalls = %+v", f.LeaveGroupCalls)
	}

	f.CreateGroupFunc = func(context.Context, string, string, []domain.JID) (any, error) { return nil, errBoom }
	f.JoinGroupFunc = func(context.Context, string, string) (any, error) { return nil, errBoom }
	f.LeaveGroupFunc = func(context.Context, string, domain.JID) error { return errBoom }
	if _, err := f.CreateGroup(ctx, "u1", "", nil); !errors.Is(err, errBoom) {
		t.Errorf("CreateGroupFunc = %v", err)
	}
	if _, err := f.JoinGroup(ctx, "u1", ""); !errors.Is(err, errBoom) {
		t.Errorf("JoinGroupFunc = %v", err)
	}
	if err := f.LeaveGroup(ctx, "u1", ""); !errors.Is(err, errBoom) {
		t.Errorf("LeaveGroupFunc = %v", err)
	}
}

func TestGroupSettings(t *testing.T) {
	f := &contractsfake.GroupSettings{}
	ctx := context.Background()
	at := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	parts := []domain.JID{"a@s"}

	if err := f.SetGroupName(ctx, "u1", "g@g.us", "nome"); err != nil {
		t.Errorf("SetGroupName = %v", err)
	}
	if err := f.SetGroupTopic(ctx, "u1", "g@g.us", "topico"); err != nil {
		t.Errorf("SetGroupTopic = %v", err)
	}
	if err := f.SetGroupPhoto(ctx, "u1", "g@g.us", []byte{1, 2}); err != nil {
		t.Errorf("SetGroupPhoto = %v", err)
	}
	if err := f.SetGroupAnnounce(ctx, "u1", "g@g.us", true); err != nil {
		t.Errorf("SetGroupAnnounce = %v", err)
	}
	if err := f.SetGroupLocked(ctx, "u1", "g@g.us", true); err != nil {
		t.Errorf("SetGroupLocked = %v", err)
	}
	if err := f.SetDisappearingTimer(ctx, "u1", "g@g.us", 7*24*time.Hour, at); err != nil {
		t.Errorf("SetDisappearingTimer = %v", err)
	}
	if v, err := f.UpdateGroupParticipants(ctx, "u1", "g@g.us", parts, domain.ParticipantAdd); v != nil || err != nil {
		t.Errorf("UpdateGroupParticipants zero-value = %v, %v", v, err)
	}

	if f.SetGroupNameCalls[0].Name != "nome" {
		t.Errorf("SetGroupNameCalls = %+v", f.SetGroupNameCalls)
	}
	if f.SetGroupTopicCalls[0].Topic != "topico" {
		t.Errorf("SetGroupTopicCalls = %+v", f.SetGroupTopicCalls)
	}
	if len(f.SetGroupPhotoCalls[0].Photo) != 2 {
		t.Errorf("SetGroupPhotoCalls = %+v", f.SetGroupPhotoCalls)
	}
	if !f.SetGroupAnnounceCalls[0].Announce {
		t.Errorf("SetGroupAnnounceCalls = %+v", f.SetGroupAnnounceCalls)
	}
	if !f.SetGroupLockedCalls[0].Locked {
		t.Errorf("SetGroupLockedCalls = %+v", f.SetGroupLockedCalls)
	}
	if c := f.SetDisappearingTimerCalls[0]; c.Duration != 7*24*time.Hour || c.At != at {
		t.Errorf("SetDisappearingTimerCalls[0] = %+v", c)
	}
	if c := f.UpdateGroupParticipantsCalls[0]; c.Action != domain.ParticipantAdd || len(c.Participants) != 1 {
		t.Errorf("UpdateGroupParticipantsCalls[0] = %+v", c)
	}

	f.SetGroupNameFunc = func(context.Context, string, domain.JID, string) error { return errBoom }
	f.SetGroupTopicFunc = func(context.Context, string, domain.JID, string) error { return errBoom }
	f.SetGroupPhotoFunc = func(context.Context, string, domain.JID, []byte) error { return errBoom }
	f.SetGroupAnnounceFunc = func(context.Context, string, domain.JID, bool) error { return errBoom }
	f.SetGroupLockedFunc = func(context.Context, string, domain.JID, bool) error { return errBoom }
	f.SetDisappearingTimerFunc = func(context.Context, string, domain.JID, time.Duration, time.Time) error { return errBoom }
	f.UpdateGroupParticipantsFunc = func(context.Context, string, domain.JID, []domain.JID, domain.ParticipantAction) (any, error) {
		return nil, errBoom
	}

	for name, err := range map[string]error{
		"SetGroupName":         f.SetGroupName(ctx, "u1", "", ""),
		"SetGroupTopic":        f.SetGroupTopic(ctx, "u1", "", ""),
		"SetGroupPhoto":        f.SetGroupPhoto(ctx, "u1", "", nil),
		"SetGroupAnnounce":     f.SetGroupAnnounce(ctx, "u1", "", false),
		"SetGroupLocked":       f.SetGroupLocked(ctx, "u1", "", false),
		"SetDisappearingTimer": f.SetDisappearingTimer(ctx, "u1", "", 0, at),
	} {
		if !errors.Is(err, errBoom) {
			t.Errorf("%sFunc = %v, quero errBoom", name, err)
		}
	}
	if _, err := f.UpdateGroupParticipants(ctx, "u1", "", nil, domain.ParticipantRemove); !errors.Is(err, errBoom) {
		t.Errorf("UpdateGroupParticipantsFunc = %v", err)
	}
}

func TestGroupRequests(t *testing.T) {
	f := &contractsfake.GroupRequests{}
	ctx := context.Background()
	parts := []domain.JID{"a@s"}

	if v, err := f.GetRequestParticipants(ctx, "u1", "g@g.us"); v != nil || err != nil {
		t.Errorf("GetRequestParticipants zero-value = %v, %v", v, err)
	}
	if err := f.UpdateRequestParticipants(ctx, "u1", "g@g.us", parts, domain.RequestApprove); err != nil {
		t.Errorf("UpdateRequestParticipants = %v", err)
	}
	if err := f.SetJoinApprovalMode(ctx, "u1", "g@g.us", true); err != nil {
		t.Errorf("SetJoinApprovalMode = %v", err)
	}

	if f.GetRequestParticipantsCalls[0].Group != "g@g.us" {
		t.Errorf("GetRequestParticipantsCalls = %+v", f.GetRequestParticipantsCalls)
	}
	if c := f.UpdateRequestParticipantsCalls[0]; c.Action != domain.RequestApprove || len(c.Participants) != 1 {
		t.Errorf("UpdateRequestParticipantsCalls[0] = %+v", c)
	}
	if !f.SetJoinApprovalModeCalls[0].Mode {
		t.Errorf("SetJoinApprovalModeCalls = %+v", f.SetJoinApprovalModeCalls)
	}

	f.GetRequestParticipantsFunc = func(context.Context, string, domain.JID) (any, error) { return nil, errBoom }
	f.UpdateRequestParticipantsFunc = func(context.Context, string, domain.JID, []domain.JID, domain.RequestAction) error {
		return errBoom
	}
	f.SetJoinApprovalModeFunc = func(context.Context, string, domain.JID, bool) error { return errBoom }
	if _, err := f.GetRequestParticipants(ctx, "u1", ""); !errors.Is(err, errBoom) {
		t.Errorf("GetRequestParticipantsFunc = %v", err)
	}
	if err := f.UpdateRequestParticipants(ctx, "u1", "", nil, domain.RequestReject); !errors.Is(err, errBoom) {
		t.Errorf("UpdateRequestParticipantsFunc = %v", err)
	}
	if err := f.SetJoinApprovalMode(ctx, "u1", "", false); !errors.Is(err, errBoom) {
		t.Errorf("SetJoinApprovalModeFunc = %v", err)
	}
}

// --- User --------------------------------------------------------------

func TestContactDirectory(t *testing.T) {
	f := &contractsfake.ContactDirectory{}
	ctx := context.Background()

	if v, err := f.IsOnWhatsApp(ctx, "u1", []string{"5511"}); v != nil || err != nil {
		t.Errorf("IsOnWhatsApp zero-value = %v, %v", v, err)
	}
	if v, err := f.GetUserInfo(ctx, "u1", []domain.JID{"a@s"}); v != nil || err != nil {
		t.Errorf("GetUserInfo zero-value = %v, %v", v, err)
	}
	if v, n, err := f.GetAllContacts(ctx, "u1"); v != nil || n != 0 || err != nil {
		t.Errorf("GetAllContacts zero-value = %v, %d, %v", v, n, err)
	}
	if v, err := f.GetProfilePicture(ctx, "u1", "a@s", true); v != nil || err != nil {
		t.Errorf("GetProfilePicture zero-value = %v, %v", v, err)
	}
	if v, err := f.GetLIDForPN(ctx, "u1", "a@s"); v != "" || err != nil {
		t.Errorf("GetLIDForPN zero-value = %q, %v", v, err)
	}

	if len(f.IsOnWhatsAppCalls[0].Phones) != 1 {
		t.Errorf("IsOnWhatsAppCalls = %+v", f.IsOnWhatsAppCalls)
	}
	if len(f.GetUserInfoCalls[0].JIDs) != 1 {
		t.Errorf("GetUserInfoCalls = %+v", f.GetUserInfoCalls)
	}
	if f.GetAllContactsCalls[0].TxtID != "u1" {
		t.Errorf("GetAllContactsCalls = %+v", f.GetAllContactsCalls)
	}
	if c := f.GetProfilePictureCalls[0]; c.Target != "a@s" || !c.Preview {
		t.Errorf("GetProfilePictureCalls[0] = %+v", c)
	}
	if f.GetLIDForPNCalls[0].JID != "a@s" {
		t.Errorf("GetLIDForPNCalls = %+v", f.GetLIDForPNCalls)
	}

	f.IsOnWhatsAppFunc = func(context.Context, string, []string) ([]domain.WhatsAppCheck, error) {
		return []domain.WhatsAppCheck{{}}, nil
	}
	f.GetUserInfoFunc = func(context.Context, string, []domain.JID) (any, error) { return nil, errBoom }
	f.GetAllContactsFunc = func(context.Context, string) (any, int, error) { return nil, 7, nil }
	f.GetProfilePictureFunc = func(context.Context, string, domain.JID, bool) (*domain.AvatarInfo, error) {
		return &domain.AvatarInfo{}, nil
	}
	f.GetLIDForPNFunc = func(context.Context, string, domain.JID) (domain.JID, error) { return "lid@lid", nil }

	if v, _ := f.IsOnWhatsApp(ctx, "u1", nil); len(v) != 1 {
		t.Errorf("IsOnWhatsAppFunc = %v", v)
	}
	if _, err := f.GetUserInfo(ctx, "u1", nil); !errors.Is(err, errBoom) {
		t.Errorf("GetUserInfoFunc = %v", err)
	}
	if _, n, _ := f.GetAllContacts(ctx, "u1"); n != 7 {
		t.Errorf("GetAllContactsFunc n = %d", n)
	}
	if v, _ := f.GetProfilePicture(ctx, "u1", "", false); v == nil {
		t.Error("GetProfilePictureFunc devolveu nil")
	}
	if v, _ := f.GetLIDForPN(ctx, "u1", ""); v != "lid@lid" {
		t.Errorf("GetLIDForPNFunc = %q", v)
	}
}

func TestBlocklistManager(t *testing.T) {
	f := &contractsfake.BlocklistManager{}
	ctx := context.Background()

	if v, err := f.GetBlocklist(ctx, "u1"); err != nil || len(v.JIDs) != 0 {
		t.Errorf("GetBlocklist zero-value = %+v, %v", v, err)
	}
	if _, err := f.UpdateBlocklist(ctx, "u1", "a@s", true); err != nil {
		t.Errorf("UpdateBlocklist zero-value = %v", err)
	}
	if c := f.UpdateBlocklistCalls[0]; c.Target != "a@s" || !c.Block {
		t.Errorf("UpdateBlocklistCalls[0] = %+v", c)
	}
	if f.GetBlocklistCalls[0].TxtID != "u1" {
		t.Errorf("GetBlocklistCalls = %+v", f.GetBlocklistCalls)
	}

	f.GetBlocklistFunc = func(context.Context, string) (domain.Blocklist, error) {
		return domain.Blocklist{}, errBoom
	}
	f.UpdateBlocklistFunc = func(context.Context, string, domain.JID, bool) (domain.BlocklistUpdate, error) {
		return domain.BlocklistUpdate{}, errBoom
	}
	if _, err := f.GetBlocklist(ctx, "u1"); !errors.Is(err, errBoom) {
		t.Errorf("GetBlocklistFunc = %v", err)
	}
	if _, err := f.UpdateBlocklist(ctx, "u1", "", false); !errors.Is(err, errBoom) {
		t.Errorf("UpdateBlocklistFunc = %v", err)
	}
}

func TestPrivacyManager(t *testing.T) {
	f := &contractsfake.PrivacyManager{}
	ctx := context.Background()

	if v, err := f.GetPrivacySettings(ctx, "u1"); v != nil || err != nil {
		t.Errorf("GetPrivacySettings zero-value = %v, %v", v, err)
	}
	if v, err := f.SetPrivacySetting(ctx, "u1", "last", "contacts"); v != nil || err != nil {
		t.Errorf("SetPrivacySetting zero-value = %v, %v", v, err)
	}
	if c := f.SetPrivacySettingCalls[0]; c.Name != "last" || c.Value != "contacts" {
		t.Errorf("SetPrivacySettingCalls[0] = %+v", c)
	}
	if f.GetPrivacySettingsCalls[0].TxtID != "u1" {
		t.Errorf("GetPrivacySettingsCalls = %+v", f.GetPrivacySettingsCalls)
	}

	f.GetPrivacySettingsFunc = func(context.Context, string) (any, error) { return "cfg", nil }
	f.SetPrivacySettingFunc = func(context.Context, string, string, string) (any, error) { return nil, errBoom }
	if v, _ := f.GetPrivacySettings(ctx, "u1"); v != "cfg" {
		t.Errorf("GetPrivacySettingsFunc = %v", v)
	}
	if _, err := f.SetPrivacySetting(ctx, "u1", "", ""); !errors.Is(err, errBoom) {
		t.Errorf("SetPrivacySettingFunc = %v", err)
	}
}

func TestUserRepositoryZeroValueEhCaminhoFeliz(t *testing.T) {
	f := &contractsfake.UserRepository{}
	ctx := context.Background()

	if created, err := f.CreateUser(ctx, domain.UserRecord{ID: "u1"}); !created || err != nil {
		t.Errorf("CreateUser zero-value = %v, %v; quero true, nil", created, err)
	}
	if exists, err := f.UserExists(ctx, "u1"); exists || err != nil {
		t.Errorf("UserExists zero-value = %v, %v; quero false, nil", exists, err)
	}
	if err := f.UpdateUser(ctx, "u1", domain.UserUpdate{}); err != nil {
		t.Errorf("UpdateUser zero-value = %v", err)
	}
	if list, err := f.ListUsers(ctx, ""); list != nil || err != nil {
		t.Errorf("ListUsers zero-value = %v, %v", list, err)
	}
	if deleted, err := f.DeleteUser(ctx, "u1"); !deleted || err != nil {
		t.Errorf("DeleteUser zero-value = %v, %v; quero true, nil", deleted, err)
	}

	if f.CreateUserCalls[0].Rec.ID != "u1" {
		t.Errorf("CreateUserCalls = %+v", f.CreateUserCalls)
	}
	if f.UserExistsCalls[0].ID != "u1" {
		t.Errorf("UserExistsCalls = %+v", f.UserExistsCalls)
	}
	if f.UpdateUserCalls[0].ID != "u1" {
		t.Errorf("UpdateUserCalls = %+v", f.UpdateUserCalls)
	}
	if f.ListUsersCalls[0].ID != "" {
		t.Errorf("ListUsersCalls = %+v", f.ListUsersCalls)
	}
	if f.DeleteUserCalls[0].ID != "u1" {
		t.Errorf("DeleteUserCalls = %+v", f.DeleteUserCalls)
	}
}

func TestUserRepositoryFuncs(t *testing.T) {
	f := &contractsfake.UserRepository{
		CreateUserFunc: func(context.Context, domain.UserRecord) (bool, error) { return false, nil },
		UserExistsFunc: func(context.Context, string) (bool, error) { return true, nil },
		UpdateUserFunc: func(context.Context, string, domain.UserUpdate) error { return errBoom },
		ListUsersFunc: func(context.Context, string) ([]domain.UserListEntry, error) {
			return []domain.UserListEntry{{}}, nil
		},
		DeleteUserFunc: func(context.Context, string) (bool, error) { return false, errBoom },
	}
	ctx := context.Background()

	if created, _ := f.CreateUser(ctx, domain.UserRecord{}); created {
		t.Error("CreateUserFunc ignorada: created = true")
	}
	if exists, _ := f.UserExists(ctx, "u1"); !exists {
		t.Error("UserExistsFunc ignorada")
	}
	if err := f.UpdateUser(ctx, "u1", domain.UserUpdate{}); !errors.Is(err, errBoom) {
		t.Errorf("UpdateUserFunc = %v", err)
	}
	if list, _ := f.ListUsers(ctx, "u1"); len(list) != 1 {
		t.Errorf("ListUsersFunc = %v", list)
	}
	deleted, err := f.DeleteUser(ctx, "u1")
	if deleted || !errors.Is(err, errBoom) {
		t.Errorf("DeleteUserFunc = %v, %v", deleted, err)
	}
}

func TestDeleteUserProvider(t *testing.T) {
	f := &contractsfake.DeleteUserProvider{}

	if exists, err := f.CheckUserExists("u1"); !exists || err != nil {
		t.Errorf("CheckUserExists zero-value = %v, %v; quero true, nil", exists, err)
	}
	name, jid, token, err := f.GetUserInfo("u1")
	if name != "" || jid != "" || token != "" || err != nil {
		t.Errorf("GetUserInfo zero-value = %q, %q, %q, %v", name, jid, token, err)
	}
	if err := f.DeleteUserFromDB("u1"); err != nil {
		t.Errorf("DeleteUserFromDB zero-value = %v", err)
	}
	if on, err := f.GetS3Enabled("u1"); on || err != nil {
		t.Errorf("GetS3Enabled zero-value = %v, %v; quero false, nil", on, err)
	}
	if err := f.DeleteLocalUserFiles("u1", "/tmp/x"); err != nil {
		t.Errorf("DeleteLocalUserFiles zero-value = %v", err)
	}

	if f.CheckUserExistsCalls[0].UserID != "u1" {
		t.Errorf("CheckUserExistsCalls = %+v", f.CheckUserExistsCalls)
	}
	if f.GetUserInfoCalls[0].UserID != "u1" {
		t.Errorf("GetUserInfoCalls = %+v", f.GetUserInfoCalls)
	}
	if f.DeleteUserFromDBCalls[0].UserID != "u1" {
		t.Errorf("DeleteUserFromDBCalls = %+v", f.DeleteUserFromDBCalls)
	}
	if f.GetS3EnabledCalls[0].UserID != "u1" {
		t.Errorf("GetS3EnabledCalls = %+v", f.GetS3EnabledCalls)
	}
	if c := f.DeleteLocalUserFilesCalls[0]; c.ExPath != "/tmp/x" {
		t.Errorf("DeleteLocalUserFilesCalls[0] = %+v", c)
	}

	f.CheckUserExistsFunc = func(string) (bool, error) { return false, nil }
	f.GetUserInfoFunc = func(string) (string, string, string, error) { return "n", "j", "t", nil }
	f.DeleteUserFromDBFunc = func(string) error { return errBoom }
	f.GetS3EnabledFunc = func(string) (bool, error) { return true, nil }
	f.DeleteLocalUserFilesFunc = func(string, string) error { return errBoom }

	if exists, _ := f.CheckUserExists("u2"); exists {
		t.Error("CheckUserExistsFunc ignorada")
	}
	if n, j, tk, _ := f.GetUserInfo("u2"); n != "n" || j != "j" || tk != "t" {
		t.Errorf("GetUserInfoFunc = %q, %q, %q", n, j, tk)
	}
	if err := f.DeleteUserFromDB("u2"); !errors.Is(err, errBoom) {
		t.Errorf("DeleteUserFromDBFunc = %v", err)
	}
	if on, _ := f.GetS3Enabled("u2"); !on {
		t.Error("GetS3EnabledFunc ignorada")
	}
	if err := f.DeleteLocalUserFiles("u2", ""); !errors.Is(err, errBoom) {
		t.Errorf("DeleteLocalUserFilesFunc = %v", err)
	}
}

// --- Misc --------------------------------------------------------------

func TestMessageComposerDevolveIDNaoVazio(t *testing.T) {
	f := &contractsfake.MessageComposer{}
	id, err := f.NewMessageID(context.Background(), "u1")
	if err != nil {
		t.Fatalf("NewMessageID = %v", err)
	}
	if id != contractsfake.DefaultMessageID || id == "" {
		t.Errorf("NewMessageID zero-value = %q, quero %q", id, contractsfake.DefaultMessageID)
	}

	f.NewMessageIDFunc = func(context.Context, string) (string, error) { return "", errBoom }
	if _, err := f.NewMessageID(context.Background(), "u2"); !errors.Is(err, errBoom) {
		t.Errorf("NewMessageIDFunc = %v", err)
	}
	if len(f.NewMessageIDCalls) != 2 || f.NewMessageIDCalls[1].TxtID != "u2" {
		t.Errorf("calls = %+v", f.NewMessageIDCalls)
	}
}

func TestMessagingPort(t *testing.T) {
	f := &contractsfake.MessagingPort{}
	ctx := context.Background()

	if err := f.PublishMessage(ctx, "rk", []byte("b")); err != nil {
		t.Errorf("PublishMessage = %v", err)
	}
	if err := f.PublishEvent(ctx, domain.Webhook{}, []byte("p")); err != nil {
		t.Errorf("PublishEvent = %v", err)
	}
	if err := f.PublishError(ctx, "tipo", []byte("p")); err != nil {
		t.Errorf("PublishError = %v", err)
	}

	if c := f.PublishMessageCalls[0]; c.RoutingKey != "rk" || string(c.Body) != "b" {
		t.Errorf("PublishMessageCalls[0] = %+v", c)
	}
	if string(f.PublishEventCalls[0].Payload) != "p" {
		t.Errorf("PublishEventCalls = %+v", f.PublishEventCalls)
	}
	if f.PublishErrorCalls[0].ErrType != "tipo" {
		t.Errorf("PublishErrorCalls = %+v", f.PublishErrorCalls)
	}

	f.PublishMessageFunc = func(context.Context, string, []byte) error { return errBoom }
	f.PublishEventFunc = func(context.Context, domain.Webhook, []byte) error { return errBoom }
	f.PublishErrorFunc = func(context.Context, string, []byte) error { return errBoom }
	if err := f.PublishMessage(ctx, "", nil); !errors.Is(err, errBoom) {
		t.Errorf("PublishMessageFunc = %v", err)
	}
	if err := f.PublishEvent(ctx, domain.Webhook{}, nil); !errors.Is(err, errBoom) {
		t.Errorf("PublishEventFunc = %v", err)
	}
	if err := f.PublishError(ctx, "", nil); !errors.Is(err, errBoom) {
		t.Errorf("PublishErrorFunc = %v", err)
	}
}

func TestStoragePort(t *testing.T) {
	f := &contractsfake.StoragePort{}
	ctx := context.Background()

	if url, err := f.UploadFile(ctx, "a@s", "f.png", []byte{1}, "image/png"); url != "" || err != nil {
		t.Errorf("UploadFile zero-value = %q, %v", url, err)
	}
	if err := f.ProcessMedia(ctx, &domain.Message{}); err != nil {
		t.Errorf("ProcessMedia zero-value = %v", err)
	}
	if cfg := f.GetS3Config("u1"); cfg != nil {
		t.Errorf("GetS3Config zero-value = %+v, quero nil", cfg)
	}

	if c := f.UploadFileCalls[0]; c.FileName != "f.png" || c.MimeType != "image/png" || c.JID != "a@s" {
		t.Errorf("UploadFileCalls[0] = %+v", c)
	}
	if f.ProcessMediaCalls[0].Msg == nil {
		t.Error("ProcessMediaCalls nao gravou a msg")
	}
	if f.GetS3ConfigCalls[0].UserID != "u1" {
		t.Errorf("GetS3ConfigCalls = %+v", f.GetS3ConfigCalls)
	}

	f.UploadFileFunc = func(context.Context, domain.JID, string, []byte, string) (string, error) {
		return "", errBoom
	}
	f.ProcessMediaFunc = func(context.Context, *domain.Message) error { return errBoom }
	f.GetS3ConfigFunc = func(string) *port.S3Config { return &port.S3Config{Bucket: "b"} }

	if _, err := f.UploadFile(ctx, "", "", nil, ""); !errors.Is(err, errBoom) {
		t.Errorf("UploadFileFunc = %v", err)
	}
	if err := f.ProcessMedia(ctx, nil); !errors.Is(err, errBoom) {
		t.Errorf("ProcessMediaFunc = %v", err)
	}
	if cfg := f.GetS3Config("u2"); cfg == nil || cfg.Bucket != "b" {
		t.Errorf("GetS3ConfigFunc = %+v", cfg)
	}
}

func TestProfileDataAccess(t *testing.T) {
	f := &contractsfake.ProfileDataAccess{}
	ctx := context.Background()

	if f.PushName() != "" {
		t.Errorf("PushName zero-value = %q", f.PushName())
	}
	if j, ok := f.OwnJID(); j != "" || ok {
		t.Errorf("OwnJID zero-value = %q, %v", j, ok)
	}
	if u, t2, err := f.ProfilePictureURL(ctx, "a@s"); u != "" || t2 != "" || err != nil {
		t.Errorf("ProfilePictureURL zero-value = %q, %q, %v", u, t2, err)
	}
	if a, b, err := f.ContactInfo(ctx, "a@s"); a != "" || b != "" || err != nil {
		t.Errorf("ContactInfo zero-value = %q, %q, %v", a, b, err)
	}
	if f.ProfilePictureURLCalls[0].JID != "a@s" || f.ContactInfoCalls[0].JID != "a@s" {
		t.Error("chamadas nao gravaram o JID")
	}

	f.PushNameValue = "Lucas"
	f.OwnJIDValue, f.OwnJIDOK = "me@s", true
	f.ProfilePictureURLFunc = func(context.Context, domain.JID) (string, string, error) {
		return "https://p", "id", nil
	}
	f.ContactInfoFunc = func(context.Context, domain.JID) (string, string, error) { return "", "", errBoom }

	if f.PushName() != "Lucas" {
		t.Errorf("PushNameValue ignorado: %q", f.PushName())
	}
	if j, ok := f.OwnJID(); j != "me@s" || !ok {
		t.Errorf("OwnJID = %q, %v", j, ok)
	}
	if u, id, _ := f.ProfilePictureURL(ctx, ""); u != "https://p" || id != "id" {
		t.Errorf("ProfilePictureURLFunc = %q, %q", u, id)
	}
	if _, _, err := f.ContactInfo(ctx, ""); !errors.Is(err, errBoom) {
		t.Errorf("ContactInfoFunc = %v", err)
	}
}

func TestProfileAccessProvider(t *testing.T) {
	f := &contractsfake.ProfileAccessProvider{}
	ctx := context.Background()

	if acc, err := f.ProfileAccess(ctx, "u1"); acc != nil || err != nil {
		t.Errorf("zero-value = %v, %v; quero nil, nil", acc, err)
	}

	// O caso comum: esta sessao tem ESTE perfil.
	f.Access = &contractsfake.ProfileDataAccess{PushNameValue: "Lucas"}
	acc, err := f.ProfileAccess(ctx, "u1")
	if err != nil || acc == nil {
		t.Fatalf("com Access = %v, %v", acc, err)
	}
	if acc.PushName() != "Lucas" {
		t.Errorf("PushName = %q", acc.PushName())
	}

	f.ProfileAccessFunc = func(context.Context, string) (port.ProfileDataAccess, error) {
		return nil, errBoom
	}
	if _, err := f.ProfileAccess(ctx, "u2"); !errors.Is(err, errBoom) {
		t.Errorf("ProfileAccessFunc = %v", err)
	}
	if len(f.ProfileAccessCalls) != 3 || f.ProfileAccessCalls[2].TxtID != "u2" {
		t.Errorf("calls = %+v", f.ProfileAccessCalls)
	}
}
