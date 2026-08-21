package waheadless

import (
	"context"
	"os"
	"testing"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeRemainingShapes reads the CALL SHAPES of the whole remaining
// whatsapp-web.js surface in one run, because ARMADILHAS.md now records that
// sibling functions in this build do not share signatures and that guessing one
// from the other costs a live failure (H58) while reading costs ten seconds
// (H59).
//
// It prints function sources and enum keys. No identity, no message content.
func TestProbeRemainingShapes(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_SHAPES") == "" {
		t.Skip("set WA_PROBE_SHAPES=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), nCycleReadyDeadline)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	want := []struct{ mod, fn string }{
		{"WAWebSetSubjectGroupAction", "setGroupSubject"},
		{"WAWebExitGroupAction", "sendExitGroup"},
		{"WAWebPollsSendPollCreationMsgAction", "sendPollCreation"},
		{"WAWebPollsSendPollCreationMsgAction", "createPollCreationMsgData"},
		{"WAWebFindCommonGroupsContactAction", "findCommonGroups"},
		{"WAWebMsgInfoAction", "updateMsgInfo"},
		{"WAWebMsgInfoAction", "updateMsgInfo"},
		{"WAWebMsgInfoGetters", "getMsgInfo"},
		{"WAWebTextStatusAction", "getTextStatus"},
		{"WAWebTextStatusAction", "setMyTextStatus"},
		{"WAWebContactTextStatusBridge", "getTextStatus"},
		{"WAWebSetAboutJob", "setAbout"},
		{"WAWebSetTextStatusJob", "setTextStatus"},
		{"WAWebSendClearChatAction", "sendClear"},
		{"WAWebDeleteChatAction", "sendDelete"},
		{"WAWebSetPushnameConnAction", "setPushname"},
		{"WAWebGroupParticipantsJob", "promoteParticipantsJob"},
		{"WAWebGroupParticipantsJob", "demoteParticipantsJob"},
		{"WAWebChatForwardMessage", "forwardMessages"},
		{"WAWebChatSendStarMsgsBridge", "sendStarMsgs"},
		{"WAWebChatSendStarMsgsBridge", "sendUnstarAll"},
		{"WAWebChatMuteBridge", "sendConversationMute"},
		{"WAWebMuteExpirations", "calculateMuteExpiration"},
		{"WAWebMuteGetters", "getIsMuted"},
		{"WAWebMuteUtils", "canMute"},
		{"WAWebSendMessageEditAction", "sendMessageEdit"},
		{"WAWebSendMessageEditAction", "addAndSendMessageEdit"},
		{"WAWebMessageEditUtils", "msgTypeSupportsEditing"},
	}
	for _, w := range want {
		var raw string
		script := `(() => {
			try {
				const m = window.require('` + w.mod + `');
				const f = m && m['` + w.fn + `'];
				if (typeof f !== 'function') { return 'NOT_A_FUNCTION:' + typeof f; }
				return String(f).slice(0, 700);
			} catch (e) { return 'THREW:' + String((e && e.message) || e).slice(0, 120); }
		})()`
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/shape", func(c context.Context) error {
			return sess.Tab().Evaluate(c, script, &raw)
		}); err != nil {
			t.Errorf("%s.%s: %v", w.mod, w.fn, err)
			continue
		}
		t.Logf("\n### %s.%s\n%s", w.mod, w.fn, raw)
	}
}
