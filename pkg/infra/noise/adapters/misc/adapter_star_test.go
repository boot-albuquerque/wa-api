package misc

import (
	"context"
	"errors"
	"testing"

	"wa-api/internal/noise/protocol/appstate"
	"wa-api/internal/noise/protocol/types"
	"wa-api/pkg/infra/noise/client"
	"wa-api/pkg/infra/noise/client/testkit"
)

func TestMiscAdapter_StarMessage_NoSession(t *testing.T) {
	a := NewMiscAdapter(testkit.GetterWith(nil))
	err := a.StarMessage(context.Background(), "u1", "c@s.whatsapp.net", "s@s.whatsapp.net", "msg1", false, true)
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("StarMessage code = %q, want no_session", testkit.AppErrCode(err))
	}
}

func TestMiscAdapter_StarMessage_PropagatesError(t *testing.T) {
	sdkErr := errors.New("boom")
	fake := &testkit.Fake{SendAppStateFn: func(_ context.Context, _ appstate.PatchInfo) error { return sdkErr }}
	a := NewMiscAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
	err := a.StarMessage(context.Background(), "u1", "c@s.whatsapp.net", "s@s.whatsapp.net", "msg1", false, true)
	if err == nil {
		t.Fatal("StarMessage should propagate error")
	}
}

func TestMiscAdapter_StarMessage_FromMeFalse(t *testing.T) {
	var got appstate.PatchInfo
	fake := &testkit.Fake{SendAppStateFn: func(_ context.Context, p appstate.PatchInfo) error {
		got = p
		return nil
	}}
	a := NewMiscAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
	if err := a.StarMessage(context.Background(), "u1", "120363111111111111@g.us", "5511999999999@s.whatsapp.net", "ABCDE12345", false, true); err != nil {
		t.Fatalf("StarMessage = %v", err)
	}

	// BuildStar produces a PatchInfo with exactly one mutation whose index
	// encodes fromMe as "0" for false — the production implementation in
	// patch_builders_message.go:53 uses indexBoolFalse.
	want := appstate.BuildStar(
		types.JID{User: "120363111111111111", Server: "g.us"},
		types.JID{User: "5511999999999", Server: "s.whatsapp.net"},
		"ABCDE12345", false, true,
	)
	if len(got.Mutations) != 1 {
		t.Fatalf("mutations = %d, want 1", len(got.Mutations))
	}
	if len(want.Mutations) != 1 {
		t.Fatalf("want mutations = %d, want 1", len(want.Mutations))
	}
	gotIdx := got.Mutations[0].Index
	wantIdx := want.Mutations[0].Index
	if len(gotIdx) != len(wantIdx) {
		t.Fatalf("index len = %d, want %d", len(gotIdx), len(wantIdx))
	}
	for i := range gotIdx {
		if gotIdx[i] != wantIdx[i] {
			t.Errorf("index[%d] = %q, want %q", i, gotIdx[i], wantIdx[i])
		}
	}
}

func TestMiscAdapter_StarMessage_FromMeTrue(t *testing.T) {
	var got appstate.PatchInfo
	fake := &testkit.Fake{SendAppStateFn: func(_ context.Context, p appstate.PatchInfo) error {
		got = p
		return nil
	}}
	a := NewMiscAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
	if err := a.StarMessage(context.Background(), "u1", "120363111111111111@g.us", "5511999999999@s.whatsapp.net", "ABCDE12345", true, true); err != nil {
		t.Fatalf("StarMessage = %v", err)
	}

	want := appstate.BuildStar(
		types.JID{User: "120363111111111111", Server: "g.us"},
		types.JID{User: "5511999999999", Server: "s.whatsapp.net"},
		"ABCDE12345", true, true,
	)
	if len(got.Mutations) != 1 {
		t.Fatalf("mutations = %d, want 1", len(got.Mutations))
	}
	gotIdx := got.Mutations[0].Index
	wantIdx := want.Mutations[0].Index
	if len(gotIdx) != len(wantIdx) {
		t.Fatalf("index len = %d, want %d", len(gotIdx), len(wantIdx))
	}
	for i := range gotIdx {
		if gotIdx[i] != wantIdx[i] {
			t.Errorf("index[%d] = %q, want %q", i, gotIdx[i], wantIdx[i])
		}
	}
}

// TestMiscAdapter_StarMessage_FromMeChangesKey verifies that fromMe=true and
// fromMe=false produce DIFFERENT mutation index keys — the core invariant
// this capability tests.
func TestMiscAdapter_StarMessage_FromMeChangesKey(t *testing.T) {
	var patches []appstate.PatchInfo
	fake := &testkit.Fake{SendAppStateFn: func(_ context.Context, p appstate.PatchInfo) error {
		patches = append(patches, p)
		return nil
	}}
	a := NewMiscAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))

	if err := a.StarMessage(context.Background(), "u1", "120363111111111111@g.us", "5511999999999@s.whatsapp.net", "M1", false, true); err != nil {
		t.Fatalf("star fromMe=false: %v", err)
	}
	if err := a.StarMessage(context.Background(), "u1", "120363111111111111@g.us", "5511999999999@s.whatsapp.net", "M1", true, true); err != nil {
		t.Fatalf("star fromMe=true: %v", err)
	}

	if len(patches) != 2 {
		t.Fatalf("expected 2 patches, got %d", len(patches))
	}
	idx0 := patches[0].Mutations[0].Index
	idx1 := patches[1].Mutations[0].Index

	same := true
	if len(idx0) != len(idx1) {
		same = false
	} else {
		for i := range idx0 {
			if idx0[i] != idx1[i] {
				same = false
				break
			}
		}
	}
	if same {
		t.Errorf("fromMe=false and fromMe=true produced the SAME index key %v — they must differ", idx0)
	}
}

func TestMiscAdapter_StarMessage_Unstar(t *testing.T) {
	var got appstate.PatchInfo
	fake := &testkit.Fake{SendAppStateFn: func(_ context.Context, p appstate.PatchInfo) error {
		got = p
		return nil
	}}
	a := NewMiscAdapter(testkit.GetterWith(map[string]client.Client{"u1": fake}))
	if err := a.StarMessage(context.Background(), "u1", "c@s.whatsapp.net", "s@s.whatsapp.net", "msg1", false, false); err != nil {
		t.Fatalf("StarMessage unstar = %v", err)
	}

	want := appstate.BuildStar(
		types.JID{User: "c", Server: "s.whatsapp.net"},
		types.JID{User: "s", Server: "s.whatsapp.net"},
		"msg1", false, false,
	)
	gotVal := got.Mutations[0].Value.GetStarAction().GetStarred()
	wantVal := want.Mutations[0].Value.GetStarAction().GetStarred()
	if gotVal != wantVal {
		t.Errorf("starred = %v, want %v", gotVal, wantVal)
	}
}
