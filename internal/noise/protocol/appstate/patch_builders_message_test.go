package appstate

import (
	"slices"
	"testing"
)

func TestBuildSettingPushName(t *testing.T) {
	info := BuildSettingPushName("Fulano")
	if info.Type != WAPatchCriticalBlock {
		t.Errorf("Type = %q, esperado %q", info.Type, WAPatchCriticalBlock)
	}
	mut := info.Mutations[0]
	if !slices.Equal(mut.Index, []string{IndexSettingPushName}) {
		t.Errorf("Index = %v, esperado [%s]", mut.Index, IndexSettingPushName)
	}
	if mut.Version != mutationVersionSettingPushName {
		t.Errorf("Version = %d, esperado %d", mut.Version, mutationVersionSettingPushName)
	}
	if mut.Value.GetPushNameSetting().GetName() != "Fulano" {
		t.Errorf("Name = %q", mut.Value.GetPushNameSetting().GetName())
	}
}

func TestBuildStarFromOtherSender(t *testing.T) {
	info := BuildStar(testChat, testSender, "MSGID", false, true)
	if info.Type != WAPatchRegularHigh {
		t.Errorf("Type = %q, esperado %q", info.Type, WAPatchRegularHigh)
	}
	mut := info.Mutations[0]
	want := []string{IndexStar, testChat.String(), "MSGID", indexBoolFalse, testSender.String()}
	if !slices.Equal(mut.Index, want) {
		t.Errorf("Index = %v, esperado %v", mut.Index, want)
	}
	if mut.Version != mutationVersionStar {
		t.Errorf("Version = %d, esperado %d", mut.Version, mutationVersionStar)
	}
	if !mut.Value.GetStarAction().GetStarred() {
		t.Error("Starred deveria ser true")
	}
}

func TestBuildStarFromMeSetsFlag(t *testing.T) {
	info := BuildStar(testChat, testSender, "MSGID", true, false)
	mut := info.Mutations[0]
	if mut.Index[3] != indexBoolTrue {
		t.Errorf("flag fromMe = %q, esperado %q", mut.Index[3], indexBoolTrue)
	}
	if mut.Value.GetStarAction().GetStarred() {
		t.Error("Starred deveria ser false")
	}
}

func TestBuildStarSameUserUsesSentinelSender(t *testing.T) {
	info := BuildStar(testChat, testChat, "MSGID", false, true)
	if got := info.Mutations[0].Index[4]; got != selfSenderIndexValue {
		t.Errorf("sender = %q, esperado sentinela %q", got, selfSenderIndexValue)
	}
}
