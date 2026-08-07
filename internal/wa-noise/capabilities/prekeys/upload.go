package prekeys

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"go.mau.fi/libsignal/ecc"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// GetServerCount pergunta ao servidor quantas prekeys deste dispositivo ele
// ainda tem em estoque.
func GetServerCount(ctx context.Context, t Transport) (int, error) {
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: "encrypt",
		Type:      IQGet,
		To:        types.ServerJID,
		Content: []waBinary.Node{
			{Tag: "count"},
		},
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get prekey count on server: %w", err)
	}
	count := resp.GetChildByTag("count")
	ag := count.AttrGetter()
	val := ag.Int("value")
	return val, ag.Error()
}

// Upload gera e envia um lote de prekeys ao servidor.
//
// O corpo inteiro roda sob o lock de upload, do primeiro statement ao ultimo —
// inclusive as duas idas ao servidor. E o mesmo desenho de uploadPreKeys antes
// da extracao; mudar isso seria mudanca de comportamento, e este lote e'
// extracao.
//
// Todos os erros sao logados e engolidos: o chamador nao tem o que fazer com
// eles, e a proxima reconexao (ou a proxima notificacao de estoque baixo) tenta
// de novo.
func Upload(ctx context.Context, t Transport, initialUpload bool) {
	t.State().LockUpload()
	defer t.State().UnlockUpload()
	if t.State().LastUpload().Add(UploadDebounce).After(time.Now()) {
		sc, _ := GetServerCount(ctx, t)
		if sc >= WantedCount {
			t.Log().Debugf("Canceling prekey upload request due to likely race condition")
			return
		}
	}
	var registrationIDBytes [RegistrationIDLength]byte
	binary.BigEndian.PutUint32(registrationIDBytes[:], t.Store().RegistrationID)
	wantedCount := WantedCount
	if initialUpload {
		wantedCount = InitialCount
	}
	preKeys, err := t.Store().PreKeys.GetOrGenPreKeys(ctx, uint32(wantedCount))
	if err != nil {
		t.Log().Errorf("Failed to get prekeys to upload: %v", err)
		return
	}
	t.Log().Infof("Uploading %d new prekeys to server", len(preKeys))
	_, err = t.SendIQ(ctx, IQ{
		Namespace: "encrypt",
		Type:      IQSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{
			{Tag: "registration", Content: registrationIDBytes[:]},
			{Tag: "type", Content: []byte{ecc.DjbType}},
			{Tag: "identity", Content: t.Store().IdentityKey.Pub[:]},
			{Tag: "list", Content: ToNodes(preKeys)},
			ToNode(t.Store().SignedPreKey),
		},
	})
	if err != nil {
		t.Log().Errorf("Failed to send request to upload prekeys: %v", err)
		return
	}
	t.Log().Debugf("Got response to uploading prekeys")
	err = t.Store().PreKeys.MarkPreKeysAsUploaded(ctx, preKeys[len(preKeys)-1].KeyID)
	if err != nil {
		t.Log().Warnf("Failed to mark prekeys as uploaded: %v", err)
		return
	}
	t.State().SetLastUpload(time.Now())
}
