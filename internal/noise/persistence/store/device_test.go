package store

import (
	"context"
	"errors"
	"testing"

	"wa-api/internal/noise/protocol/types"
)

// recordingContainer conta as chamadas de PutDevice/DeleteDevice e permite
// injetar erro — DeviceContainer e' interface, entao Device pode ser exercitado
// sem banco nenhum.
type recordingContainer struct {
	puts    int
	deletes int
	putErr  error
	delErr  error
}

func (c *recordingContainer) PutDevice(ctx context.Context, device *Device) error {
	c.puts++
	return c.putErr
}

func (c *recordingContainer) DeleteDevice(ctx context.Context, device *Device) error {
	c.deletes++
	return c.delErr
}

func newDeviceWithContainer() (*Device, *recordingContainer) {
	container := &recordingContainer{}
	jid := types.JID{User: "5511999999999", Server: types.DefaultUserServer}
	return &Device{
		ID:        &jid,
		LID:       types.JID{User: "111111111111111", Server: types.HiddenUserServer},
		Container: container,
	}, container
}

func TestGetJIDOnNilDevice(t *testing.T) {
	var device *Device
	if got := device.GetJID(); got != types.EmptyJID {
		t.Fatalf("GetJID em device nil = %s, esperava EmptyJID", got)
	}
	if got := device.GetLID(); got != types.EmptyJID {
		t.Fatalf("GetLID em device nil = %s, esperava EmptyJID", got)
	}
}

func TestGetJIDWithNilID(t *testing.T) {
	device := &Device{}
	if got := device.GetJID(); got != types.EmptyJID {
		t.Fatalf("GetJID com ID nil = %s, esperava EmptyJID", got)
	}
}

func TestGetJIDAndGetLID(t *testing.T) {
	device, _ := newDeviceWithContainer()
	if got := device.GetJID(); got != *device.ID {
		t.Fatalf("GetJID = %s", got)
	}
	if got := device.GetLID(); got != device.LID {
		t.Fatalf("GetLID = %s", got)
	}
}

func TestSaveDelegatesToContainer(t *testing.T) {
	device, container := newDeviceWithContainer()
	if err := device.Save(context.Background()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if container.puts != 1 {
		t.Fatalf("PutDevice chamado %d vezes", container.puts)
	}
}

func TestSavePropagatesContainerError(t *testing.T) {
	device, container := newDeviceWithContainer()
	boom := errors.New("boom")
	container.putErr = boom
	if err := device.Save(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("Save deveria propagar o erro do container, veio %v", err)
	}
}

func TestSaveOnDeletedDeviceFails(t *testing.T) {
	device, container := newDeviceWithContainer()
	device.Deleted = true
	if err := device.Save(context.Background()); !errors.Is(err, ErrDeviceDeleted) {
		t.Fatalf("esperava ErrDeviceDeleted, veio %v", err)
	}
	if container.puts != 0 {
		t.Fatal("Save em device deletado nao pode chegar no container")
	}
}

// Delete tem que fazer TRES coisas alem de apagar: zerar o JID/LID, marcar
// Deleted e trocar todos os stores por um NoopStore que devolve
// ErrDeviceDeleted. E' o que impede uso posterior de uma sessao encerrada.
func TestDeleteResetsIdentityAndStores(t *testing.T) {
	ctx := context.Background()
	device, container := newDeviceWithContainer()
	device.SetAllStores(&NoopStore{errors.New("original")})

	if err := device.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if container.deletes != 1 {
		t.Fatalf("DeleteDevice chamado %d vezes", container.deletes)
	}
	if device.ID != nil {
		t.Fatal("Delete deveria zerar o ID")
	}
	if device.LID != types.EmptyJID {
		t.Fatalf("Delete deveria zerar o LID, veio %s", device.LID)
	}
	if !device.Deleted {
		t.Fatal("Delete deveria marcar Deleted")
	}
	if err := device.Sessions.PutSession(ctx, "alice:0", nil); !errors.Is(err, ErrDeviceDeleted) {
		t.Fatalf("os stores deveriam ter virado NoopStore{ErrDeviceDeleted}, veio %v", err)
	}
}

func TestDeleteTwiceIsNoOp(t *testing.T) {
	ctx := context.Background()
	device, container := newDeviceWithContainer()
	if err := device.Delete(ctx); err != nil {
		t.Fatalf("Delete 1: %v", err)
	}
	if err := device.Delete(ctx); err != nil {
		t.Fatalf("Delete 2: %v", err)
	}
	if container.deletes != 1 {
		t.Fatalf("o segundo Delete nao deveria chegar no container (deletes=%d)", container.deletes)
	}
}

// Se o container falhar, o device NAO pode ser marcado como deletado — senao
// ficaria inutilizavel em memoria enquanto continua existindo no banco.
func TestDeleteDoesNotMutateOnContainerError(t *testing.T) {
	device, container := newDeviceWithContainer()
	boom := errors.New("boom")
	container.delErr = boom
	originalID := device.ID

	if err := device.Delete(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("Delete deveria propagar o erro, veio %v", err)
	}
	if device.Deleted {
		t.Fatal("o device nao pode ser marcado como deletado se o container falhou")
	}
	if device.ID != originalID {
		t.Fatal("o ID nao pode ser zerado se o container falhou")
	}
}

func TestSetAllStoresAssignsEverySessionSpecificField(t *testing.T) {
	device := &Device{}
	noop := &NoopStore{errors.New("x")}
	device.SetAllStores(noop)

	if device.Identities == nil || device.Sessions == nil || device.PreKeys == nil ||
		device.SenderKeys == nil || device.AppStateKeys == nil || device.AppState == nil ||
		device.Contacts == nil || device.ChatSettings == nil || device.MsgSecrets == nil ||
		device.PrivacyTokens == nil || device.NCTSalt == nil || device.EventBuffer == nil {
		t.Fatalf("SetAllStores deixou campo nil: %+v", device)
	}
	// SetAllStores cobre so' os stores POR SESSAO: LIDs e' global e vem do
	// container, entao tem que continuar intocado.
	if device.LIDs != nil {
		t.Fatal("SetAllStores nao deveria mexer em LIDs (store global)")
	}
}

// recordingLIDStore devolve JIDs distinguiveis para provar qual direcao de
// traducao GetAltJID escolheu.
type recordingLIDStore struct {
	NoopStore
	lidForPN types.JID
	pnForLID types.JID
}

func (s *recordingLIDStore) GetLIDForPN(ctx context.Context, pn types.JID) (types.JID, error) {
	return s.lidForPN, nil
}

func (s *recordingLIDStore) GetPNForLID(ctx context.Context, lid types.JID) (types.JID, error) {
	return s.pnForLID, nil
}

func TestGetAltJIDTranslatesInBothDirections(t *testing.T) {
	ctx := context.Background()
	lid := types.JID{User: "111111111111111", Server: types.HiddenUserServer}
	pn := types.JID{User: "5511999999999", Server: types.DefaultUserServer}
	device := &Device{LIDs: &recordingLIDStore{lidForPN: lid, pnForLID: pn}}

	got, err := device.GetAltJID(ctx, pn)
	if err != nil {
		t.Fatalf("GetAltJID(PN): %v", err)
	}
	if got != lid {
		t.Fatalf("GetAltJID(PN) = %s, esperava o LID", got)
	}

	got, err = device.GetAltJID(ctx, lid)
	if err != nil {
		t.Fatalf("GetAltJID(LID): %v", err)
	}
	if got != pn {
		t.Fatalf("GetAltJID(LID) = %s, esperava o PN", got)
	}
}

func TestGetAltJIDOnOtherServersReturnsEmpty(t *testing.T) {
	device := &Device{LIDs: &recordingLIDStore{}}
	group := types.JID{User: "g1", Server: types.GroupServer}
	got, err := device.GetAltJID(context.Background(), group)
	if err != nil {
		t.Fatalf("GetAltJID: %v", err)
	}
	if got != types.EmptyJID {
		t.Fatalf("JID de grupo nao tem equivalente, esperava EmptyJID, veio %s", got)
	}
}

func TestGetAltJIDOnNilDeviceIsSafe(t *testing.T) {
	var device *Device
	got, err := device.GetAltJID(context.Background(), types.JID{User: "1", Server: types.DefaultUserServer})
	if err != nil {
		t.Fatalf("GetAltJID em device nil: %v", err)
	}
	if got != types.EmptyJID {
		t.Fatalf("esperava EmptyJID, veio %s", got)
	}
}
