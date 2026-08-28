package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/application/usecase/profile"
	"wa-api/pkg/domain"
	"wa-api/pkg/presentation/http/contracttest"

	"github.com/gorilla/mux"
)

// F302 — GET /session/profile/full served `user_info` and `privacy` in
// PascalCase: domain.UserInfo and domain.PrivacySettings carry no `json`
// tags (deliberately — see their doc comments), and the handler used to hand
// them straight to RespondJSON. The fix routes both fields through the
// ALREADY-EXISTING user-family presenters (dtouser.PresentUserInfo /
// PresentPrivacySettings) via presentProfileFull in profile_handler.go.
//
// This file follows the route-level contract pattern from
// handler_group_info_contract_test.go: the request goes through the REAL
// registered mux router, not a raw handler call (ARMADILHAS #2).

// perfilCompletoDeReferencia is the aggregated result with every
// network-derived field populated, so the mapping is measured in full and
// not just on the zero value.
func perfilCompletoDeReferencia() *profile.ProfileFullResult {
	return &profile.ProfileFullResult{
		ProfileResult: profile.ProfileResult{
			Pushname:     "Lucas",
			AvatarURL:    "https://example.com/a.jpg",
			AvatarID:     "avatar-1",
			JID:          "5511999999999@s.whatsapp.net",
			FullName:     "Lucas Albuquerque",
			BusinessName: "",
			SessionDeviceInfo: domain.SessionDeviceInfo{
				LID:                   "111111111111111@lid",
				Platform:              "android",
				RegistrationID:        42,
				LIDMigrationTimestamp: 1700000000,
			},
		},
		UserInfo: []domain.UserInfo{
			{
				JID:          "5511999999999@s.whatsapp.net",
				LID:          "111111111111111@lid",
				Status:       "disponivel",
				PictureID:    "pic-1",
				VerifiedName: "Lucas",
				PushName:     "Lucas",
				BusinessName: "",
			},
		},
		Privacy: domain.PrivacySettings{
			GroupAdd:     "contacts",
			LastSeen:     "everyone",
			Status:       "everyone",
			Profile:      "everyone",
			ReadReceipts: "all",
			CallAdd:      "everyone",
			Online:       "all",
			Messages:     "everyone",
			Defense:      "off",
			Stickers:     "everyone",
		},
	}
}

type profileFullFakeUseCase struct {
	result *profile.ProfileFullResult
}

func (f *profileFullFakeUseCase) Execute(context.Context, string) (*profile.ProfileFullResult, error) {
	return f.result, nil
}

// profileFullRouter mounts the route EXACTLY as wiring_routes.go mounts it:
// registry.Register("/session/profile/full", customChain.Then(ch.ProfileFull),
// "GET") applied to a mux.Router — see pkg/bootstrap/wiring_routes.go:48.
func profileFullRouter(t *testing.T, uc ProfileFullUseCase) *mux.Router {
	t.Helper()
	registry := NewHandlerRegistry()
	registry.Register("/session/profile/full", NewProfileFullHandler(uc), http.MethodGet)
	router := mux.NewRouter()
	registry.Apply(router)
	return router
}

func doGetProfileFull(t *testing.T, router *mux.Router) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/session/profile/full", nil)
	ctx := context.WithValue(req.Context(), appport.UserInfoKey, &mockUserInfo{id: "user-1"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestProfileFull_ContratoPublico_NomesCanonicos is the affirmative case:
// every key in the body, recursively, is lowercase snake_case — asserted by
// the shared helper, not a hand-written list that goes stale.
func TestProfileFull_ContratoPublico_NomesCanonicos(t *testing.T) {
	router := profileFullRouter(t, &profileFullFakeUseCase{result: perfilCompletoDeReferencia()})

	rec := doGetProfileFull(t, router)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200; corpo: %s", rec.Code, rec.Body.String())
	}
	contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
}

// TestProfileFull_ContratoPublico_ChavesAntigasSumiram proves the OLD keys
// are gone. Before the fix, domain.UserInfo and domain.PrivacySettings were
// serialized without json tags, which made the encoder emit the Go field
// names below verbatim.
func TestProfileFull_ContratoPublico_ChavesAntigasSumiram(t *testing.T) {
	router := profileFullRouter(t, &profileFullFakeUseCase{result: perfilCompletoDeReferencia()})

	rec := doGetProfileFull(t, router)

	contracttest.AssertNoKeys(t, rec.Body.Bytes(),
		"JID", "LID", "PictureID", "VerifiedName", "PushName", "BusinessName",
		"Devices", "Status",
		"GroupAdd", "LastSeen", "Profile", "ReadReceipts", "CallAdd", "Online",
		"Messages", "Defense", "Stickers",
	)
}

// TestProfileFull_ContratoPublico_ValoresMapeados proves the presenter put
// the right VALUES in the right keys — a swap between two neighboring
// fields would still pass the two tests above.
func TestProfileFull_ContratoPublico_ValoresMapeados(t *testing.T) {
	router := profileFullRouter(t, &profileFullFakeUseCase{result: perfilCompletoDeReferencia()})

	rec := doGetProfileFull(t, router)

	var envelope struct {
		Success bool `json:"success"`
		Code    int  `json:"code"`
		Data    struct {
			Pushname string `json:"pushname"`
			JID      string `json:"jid"`
			UserInfo []struct {
				JID       string `json:"jid"`
				LID       string `json:"lid"`
				Status    string `json:"status"`
				PictureID string `json:"picture_id"`
			} `json:"user_info"`
			Privacy struct {
				GroupAdd string `json:"group_add"`
				LastSeen string `json:"last_seen"`
			} `json:"privacy"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("corpo não é o envelope canónico: %v\n%s", err, rec.Body.String())
	}
	if !envelope.Success || envelope.Code != 200 {
		t.Fatalf("envelope = %+v, quero success=true code=200", envelope)
	}
	if envelope.Data.Pushname != "Lucas" || envelope.Data.JID != "5511999999999@s.whatsapp.net" {
		t.Errorf("campos herdados de ProfileResult errados: %+v", envelope.Data)
	}
	if len(envelope.Data.UserInfo) != 1 {
		t.Fatalf("user_info = %#v, quero 1 elemento", envelope.Data.UserInfo)
	}
	ui := envelope.Data.UserInfo[0]
	if ui.JID != "5511999999999@s.whatsapp.net" || ui.LID != "111111111111111@lid" ||
		ui.Status != "disponivel" || ui.PictureID != "pic-1" {
		t.Errorf("user_info[0] = %+v", ui)
	}
	if envelope.Data.Privacy.GroupAdd != "contacts" || envelope.Data.Privacy.LastSeen != "everyone" {
		t.Errorf("privacy = %+v", envelope.Data.Privacy)
	}
}
