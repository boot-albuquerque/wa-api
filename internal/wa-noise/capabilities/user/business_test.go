package user

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waVnameCert"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// Relocado de user_business_test.go (Fase E lote 7), adaptado aos dubles.

// --- ParseBusinessProfile ---

func TestParseBusinessProfileFullNode(t *testing.T) {
	node := waBinary.Node{
		Tag: businessProfileNodeTag,
		Content: []waBinary.Node{{
			Tag:   profileNodeTag,
			Attrs: waBinary.Attrs{"jid": userTestPNJID},
			Content: []waBinary.Node{
				{Tag: "address", Content: []byte("Rua 1")},
				{Tag: "email", Content: []byte("a@b.com")},
				{
					Tag:   "business_hours",
					Attrs: waBinary.Attrs{"timezone": "America/Sao_Paulo"},
					Content: []waBinary.Node{
						{Tag: businessHoursConfigTag, Attrs: waBinary.Attrs{
							"day_of_week": "mon", "mode": "open_time",
							"open_time": "0800", "close_time": "1800",
						}},
						{Tag: "outra-tag"}, // ignorado
					},
				},
				{Tag: "categories", Content: []waBinary.Node{
					{Tag: businessCategoryTag, Attrs: waBinary.Attrs{"id": "1"}, Content: []byte("Loja")},
					{Tag: "nao-categoria", Content: []byte("x")}, // ignorado
				}},
				{Tag: "profile_options", Content: []waBinary.Node{
					{Tag: "cart_enabled", Content: []byte("true")},
				}},
			},
		}},
	}
	got, err := ParseBusinessProfile(&node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.JID != userTestPNJID {
		t.Errorf("got JID %s, want %s", got.JID, userTestPNJID)
	}
	if got.Address != "Rua 1" || got.Email != "a@b.com" {
		t.Errorf("got address %q / email %q", got.Address, got.Email)
	}
	if got.BusinessHoursTimeZone != "America/Sao_Paulo" {
		t.Errorf("got timezone %q", got.BusinessHoursTimeZone)
	}
	if len(got.BusinessHours) != 1 || got.BusinessHours[0].DayOfWeek != "mon" ||
		got.BusinessHours[0].OpenTime != "0800" || got.BusinessHours[0].CloseTime != "1800" ||
		got.BusinessHours[0].Mode != "open_time" {
		t.Errorf("got business hours %+v", got.BusinessHours)
	}
	if len(got.Categories) != 1 || got.Categories[0].ID != "1" || got.Categories[0].Name != "Loja" {
		t.Errorf("got categories %+v", got.Categories)
	}
	if got.ProfileOptions["cart_enabled"] != "true" || len(got.ProfileOptions) != 1 {
		t.Errorf("got profile options %v", got.ProfileOptions)
	}
}

func TestParseBusinessProfileMissingJID(t *testing.T) {
	node := waBinary.Node{
		Tag:     businessProfileNodeTag,
		Content: []waBinary.Node{{Tag: profileNodeTag}},
	}
	got, err := ParseBusinessProfile(&node)
	if err == nil {
		t.Fatalf("expected an error, got profile %+v", got)
	}
	if got != nil {
		t.Errorf("expected nil profile on error, got %+v", got)
	}
}

// Conteudo textual em campo que deveria ser opcional nao pode virar panic:
// todos os campos ausentes ou com conteudo de tipo inesperado saem como vazio.
func TestParseBusinessProfileTolerantesToMissingAndNonByteContent(t *testing.T) {
	node := waBinary.Node{
		Tag: businessProfileNodeTag,
		Content: []waBinary.Node{{
			Tag:   profileNodeTag,
			Attrs: waBinary.Attrs{"jid": userTestPNJID},
			Content: []waBinary.Node{
				// <address> com filhos em vez de bytes
				{Tag: "address", Content: []waBinary.Node{{Tag: "x"}}},
				// <email> ausente por completo
				{Tag: "categories", Content: []waBinary.Node{
					{Tag: businessCategoryTag, Attrs: waBinary.Attrs{"id": "9"}}, // sem nome
				}},
				{Tag: "profile_options", Content: []waBinary.Node{
					{Tag: "opt", Content: []waBinary.Node{{Tag: "y"}}},
				}},
			},
		}},
	}
	got, err := ParseBusinessProfile(&node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Address != "" || got.Email != "" {
		t.Errorf("got address %q / email %q, want both empty", got.Address, got.Email)
	}
	if len(got.Categories) != 1 || got.Categories[0].Name != "" {
		t.Errorf("got categories %+v", got.Categories)
	}
	if got.ProfileOptions["opt"] != "" {
		t.Errorf("got profile options %v", got.ProfileOptions)
	}
	// Campos de lista sao sempre nao-nil, mesmo sem os nos correspondentes.
	if got.BusinessHours == nil || got.ProfileOptions == nil {
		t.Error("expected non-nil slices/maps")
	}
}

// --- GetBusinessProfile ---

func TestGetBusinessProfileBuildsTheIQAndParses(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{{
		Tag: "iq",
		Content: []waBinary.Node{{
			Tag: businessProfileNodeTag,
			Content: []waBinary.Node{{
				Tag:   profileNodeTag,
				Attrs: waBinary.Attrs{"jid": userTestPNJID},
			}},
		}},
	}}

	got, err := GetBusinessProfile(t.Context(), f, userTestPNJID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.JID != userTestPNJID {
		t.Errorf("got %+v", got)
	}
	iq := f.sent[0]
	if iq.Namespace != businessIQNamespace || iq.Type != IQGet || iq.To != types.ServerJID {
		t.Errorf("envelope = %+v", iq)
	}
	bp := iq.Content.([]waBinary.Node)[0]
	if bp.Tag != businessProfileNodeTag || bp.Attrs["v"] != businessProfileVersion {
		t.Errorf("<business_profile> = %+v", bp)
	}
	profile := bp.Content.([]waBinary.Node)[0]
	if profile.Tag != profileNodeTag || profile.Attrs["jid"] != userTestPNJID {
		t.Errorf("<profile> = %+v", profile)
	}
}

func TestGetBusinessProfilePropagatesError(t *testing.T) {
	boom := errors.New("nope")
	f := newFakeTransport()
	f.err = []error{boom}
	if _, err := GetBusinessProfile(t.Context(), f, userTestPNJID); !errors.Is(err, boom) {
		t.Errorf("got %v", err)
	}
}

func TestGetBusinessProfileMissingNodeIsAnError(t *testing.T) {
	f := newFakeTransport()
	f.resp = []*waBinary.Node{{Tag: "iq"}}
	_, err := GetBusinessProfile(t.Context(), f, userTestPNJID)
	var missing *testElementMissing
	if !errors.As(err, &missing) || missing.Tag != businessProfileNodeTag {
		t.Errorf("got %v", err)
	}
}

// --- UpdateBusinessName ---

func TestUpdateBusinessNameStoresBothJIDsAndDispatches(t *testing.T) {
	f := newFakeTransport()
	st := f.withStores()

	UpdateBusinessName(t.Context(), f, userTestPNJID, userTestLIDJID, nil, "Loja")

	if st.businessNames[userTestPNJID] != "Loja" || st.businessNames[userTestLIDJID] != "Loja" {
		t.Errorf("gravados = %v", st.businessNames)
	}
	if len(f.events) != 1 {
		t.Fatalf("eventos = %v", f.events)
	}
	evt, ok := f.events[0].(*events.BusinessName)
	if !ok || evt.JID != userTestPNJID || evt.NewBusinessName != "Loja" {
		t.Errorf("evento = %+v", f.events[0])
	}
}

func TestUpdateBusinessNameResolvesAltJIDFromStore(t *testing.T) {
	f := newFakeTransport()
	st := f.withStores()
	st.altJID = userTestLIDJID

	UpdateBusinessName(t.Context(), f, userTestPNJID, types.EmptyJID, nil, "Loja")

	if st.businessNames[userTestLIDJID] != "Loja" {
		t.Errorf("gravados = %v", st.businessNames)
	}
}

func TestUpdateBusinessNameWithoutAltJID(t *testing.T) {
	f := newFakeTransport()
	st := f.withStores()

	UpdateBusinessName(t.Context(), f, userTestPNJID, types.EmptyJID, nil, "Loja")

	if len(st.businessNames) != 1 || len(f.events) != 1 {
		t.Errorf("gravados = %v, eventos = %v", st.businessNames, f.events)
	}
}

func TestUpdateBusinessNameNoOpPaths(t *testing.T) {
	t.Run("sem contact store", func(t *testing.T) {
		f := newFakeTransport()
		f.withStores()
		f.store.Contacts = nil
		UpdateBusinessName(t.Context(), f, userTestPNJID, types.EmptyJID, nil, "Loja")
		if len(f.events) != 0 {
			t.Errorf("eventos = %v", f.events)
		}
	})
	t.Run("nome nao mudou", func(t *testing.T) {
		f := newFakeTransport()
		st := f.withStores()
		st.noChange = true
		UpdateBusinessName(t.Context(), f, userTestPNJID, types.EmptyJID, nil, "Loja")
		if len(f.events) != 0 {
			t.Errorf("eventos = %v", f.events)
		}
	})
	t.Run("erro ao gravar", func(t *testing.T) {
		f := newFakeTransport()
		st := f.withStores()
		st.putErr = errors.New("disco cheio")
		UpdateBusinessName(t.Context(), f, userTestPNJID, types.EmptyJID, nil, "Loja")
		if len(f.events) != 0 {
			t.Errorf("eventos = %v", f.events)
		}
	})
}

func TestUpdateBusinessNameAltStoreErrorStillDispatches(t *testing.T) {
	f := newFakeTransport()
	st := f.withStores()
	st.altJID = userTestLIDJID
	f.store.Contacts = &failOnSecondPut{fakeStores: st}

	UpdateBusinessName(t.Context(), f, userTestPNJID, types.EmptyJID, nil, "Loja")

	if len(f.events) != 1 {
		t.Errorf("eventos = %v", f.events)
	}
}

// --- ParseVerifiedName ---

func TestParseVerifiedNameRoundTrip(t *testing.T) {
	node := waBinary.Node{
		Tag: businessNodeTag,
		Content: []waBinary.Node{{
			Tag:     verifiedNameNodeTag,
			Content: verifiedNameCertBytes(t, "Loja Teste"),
		}},
	}
	got, err := ParseVerifiedName(node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a verified name")
	}
	if got.Details.GetVerifiedName() != "Loja Teste" {
		t.Errorf("got %q, want %q", got.Details.GetVerifiedName(), "Loja Teste")
	}
}

// Os tres "nao e' erro, so' nao tem nome verificado" — importantes porque
// IsOnWhatsApp/GetInfo logam Warn quando o erro nao e' nil, e um usuario comum
// (sem conta business) cai exatamente aqui.
func TestParseVerifiedNameAbsentIsNotAnError(t *testing.T) {
	cases := map[string]waBinary.Node{
		"tag nao e' business": {Tag: "not-business", Content: []waBinary.Node{
			{Tag: verifiedNameNodeTag, Content: verifiedNameCertBytes(t, "x")},
		}},
		"sem <verified_name>": {Tag: businessNodeTag},
		"no zerado":           {},
		"<verified_name> sem bytes": {Tag: businessNodeTag, Content: []waBinary.Node{
			{Tag: verifiedNameNodeTag, Content: []waBinary.Node{{Tag: "x"}}},
		}},
	}
	for name, node := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseVerifiedName(node)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if got != nil {
				t.Errorf("expected nil verified name, got %+v", got)
			}
		})
	}
}

func TestParseVerifiedNameInvalidProtobuf(t *testing.T) {
	node := waBinary.Node{
		Tag: businessNodeTag,
		Content: []waBinary.Node{{
			Tag:     verifiedNameNodeTag,
			Content: []byte{0xff, 0xff, 0xff, 0xff},
		}},
	}
	got, err := ParseVerifiedName(node)
	if err == nil {
		t.Fatalf("expected an error, got %+v", got)
	}
	if got != nil {
		t.Errorf("expected nil on error, got %+v", got)
	}
}

// Certificado valido cujo `details` nao e' um protobuf de Details valido: o erro
// vem do segundo Unmarshal, nao do primeiro.
func TestParseVerifiedNameInvalidDetails(t *testing.T) {
	cert, err := proto.Marshal(&waVnameCert.VerifiedNameCertificate{
		Details: []byte{0xff, 0xff, 0xff, 0xff},
	})
	if err != nil {
		t.Fatalf("failed to marshal cert: %v", err)
	}
	got, err := ParseVerifiedNameContent(waBinary.Node{
		Tag:     verifiedNameNodeTag,
		Content: cert,
	})
	if err == nil {
		t.Fatalf("expected an error, got %+v", got)
	}
}

// Certificado vazio: details ausente desserializa para Details zerado, sem erro.
func TestParseVerifiedNameEmptyCertificate(t *testing.T) {
	got, err := ParseVerifiedNameContent(waBinary.Node{
		Tag:     verifiedNameNodeTag,
		Content: []byte{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Details.GetVerifiedName() != "" {
		t.Errorf("got %+v, want an empty verified name", got)
	}
}
