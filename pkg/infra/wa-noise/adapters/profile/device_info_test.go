package profile

import (
	"testing"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/pkg/domain"
)

// DeviceInfo lê seis campos do store e dois do cliente. Não faz rede e não
// devolve erro: store ausente vira zero-value, que é a resposta honesta para
// "o store ainda não tem isso" — e o mesmo contrato das demais funções deste
// adapter.

// TestDeviceInfo_ClienteNil: o adapter aceita client nil por desenho
// (NewProfileDataAccessFromInterface devolve um assim quando o tipo não é o
// esperado). Tem de devolver zero-value, e não entrar em pânico.
func TestDeviceInfo_ClienteNil(t *testing.T) {
	da := &ProfileDataAccess{}

	got := da.DeviceInfo()

	if got != (domain.SessionDeviceInfo{}) {
		t.Errorf("client nil devolveu %+v, quero zero-value", got)
	}
}

// TestDeviceInfo_StoreNil: com cliente mas sem store, Connected e LoggedIn
// ainda respondem — descrevem a conexão, não o store. É a diferença que faz
// esta função útil numa sessão conectada e ainda não autenticada.
func TestDeviceInfo_StoreNil(t *testing.T) {
	da := &ProfileDataAccess{client: &wanoise.Client{}}

	got := da.DeviceInfo()

	if got.Platform != "" || got.LID != "" || got.RegistrationID != 0 {
		t.Errorf("campos de store preenchidos sem store: %+v", got)
	}
	// Um cliente zerado não está conectado nem logado; o que importa aqui é
	// que a leitura acontece sem pânico e sem depender do store.
	if got.Connected || got.LoggedIn {
		t.Errorf("cliente zerado reportou Connected=%v LoggedIn=%v", got.Connected, got.LoggedIn)
	}
}

func TestDeviceInfo_StorePreenchido(t *testing.T) {
	lid := types.NewJID("90000000000002", types.HiddenUserServer)
	da := &ProfileDataAccess{client: &wanoise.Client{Store: &store.Device{
		LID:                   lid,
		Platform:              "iphone",
		RegistrationID:        803190740,
		LIDMigrationTimestamp: 1762864519,
		Initialized:           true,
	}}}

	got := da.DeviceInfo()

	if got.LID != lid.String() {
		t.Errorf("LID = %q, quero %q", got.LID, lid.String())
	}
	if got.Platform != "iphone" {
		t.Errorf("Platform = %q, quero iphone", got.Platform)
	}
	if got.RegistrationID != 803190740 {
		t.Errorf("RegistrationID = %d, quero 803190740", got.RegistrationID)
	}
	if got.LIDMigrationTimestamp != 1762864519 {
		t.Errorf("LIDMigrationTimestamp = %d, quero 1762864519", got.LIDMigrationTimestamp)
	}
	if !got.Initialized {
		t.Error("Initialized = false, quero true")
	}
}

// TestDeviceInfo_LIDVazioSaiVazio fixa o contrato observável: store sem LID
// produz string vazia, e não um JID degenerado que um cliente testando
// `if lid` interpretaria como "há LID".
//
// A garantia hoje vem de types.JID.String(), que devolve "" para o JID
// zerado — medido, não suposto: a guarda IsEmpty que existia aqui foi
// removida justamente porque nenhum teste conseguia distinguir sua presença
// da sua ausência. O teste fica porque trava a PROPRIEDADE, não a
// implementação: se String() mudar, é aqui que aparece.
func TestDeviceInfo_LIDVazioSaiVazio(t *testing.T) {
	da := &ProfileDataAccess{client: &wanoise.Client{Store: &store.Device{}}}

	if got := da.DeviceInfo().LID; got != "" {
		t.Errorf("LID = %q para store sem LID, quero string vazia", got)
	}
}
