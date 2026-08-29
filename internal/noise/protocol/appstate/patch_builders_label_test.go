package appstate

import (
	"slices"
	"testing"
)

func TestBuildLabelChat(t *testing.T) {
	info := BuildLabelChat(testChat, "42", true)
	if info.Type != WAPatchRegular {
		t.Errorf("Type = %q, esperado %q", info.Type, WAPatchRegular)
	}
	mut := info.Mutations[0]
	want := []string{IndexLabelAssociationChat, "42", testChat.String()}
	if !slices.Equal(mut.Index, want) {
		t.Errorf("Index = %v, esperado %v", mut.Index, want)
	}
	if mut.Version != mutationVersionLabelAssocChat {
		t.Errorf("Version = %d, esperado %d", mut.Version, mutationVersionLabelAssocChat)
	}
	if !mut.Value.GetLabelAssociationAction().GetLabeled() {
		t.Error("Labeled deveria ser true")
	}
}

func TestBuildLabelChatUnlabel(t *testing.T) {
	info := BuildLabelChat(testChat, "42", false)
	if info.Mutations[0].Value.GetLabelAssociationAction().GetLabeled() {
		t.Error("Labeled deveria ser false")
	}
}

func TestBuildLabelMessageIndexShape(t *testing.T) {
	info := BuildLabelMessage(testChat, "7", "MSGID", true)
	if info.Type != WAPatchRegular {
		t.Errorf("Type = %q", info.Type)
	}
	mut := info.Mutations[0]
	want := []string{IndexLabelAssociationMessage, "7", testChat.String(), "MSGID", indexBoolFalse, indexBoolFalse}
	if !slices.Equal(mut.Index, want) {
		t.Errorf("Index = %v, esperado %v", mut.Index, want)
	}
	if mut.Version != mutationVersionLabelAssocMsg {
		t.Errorf("Version = %d, esperado %d", mut.Version, mutationVersionLabelAssocMsg)
	}
}

func TestBuildLabelEdit(t *testing.T) {
	info := BuildLabelEdit("9", "urgente", 5, false)
	if info.Type != WAPatchRegular {
		t.Errorf("Type = %q", info.Type)
	}
	mut := info.Mutations[0]
	want := []string{IndexLabelEdit, "9"}
	if !slices.Equal(mut.Index, want) {
		t.Errorf("Index = %v, esperado %v", mut.Index, want)
	}
	if mut.Version != mutationVersionLabelEdit {
		t.Errorf("Version = %d, esperado %d", mut.Version, mutationVersionLabelEdit)
	}
	action := mut.Value.GetLabelEditAction()
	if action.GetName() != "urgente" || action.GetColor() != 5 || action.GetDeleted() {
		t.Errorf("LabelEditAction = %+v", action)
	}
}

func TestBuildLabelEditDeleted(t *testing.T) {
	info := BuildLabelEdit("9", "urgente", 5, true)
	if !info.Mutations[0].Value.GetLabelEditAction().GetDeleted() {
		t.Error("Deleted deveria ser true")
	}
}
