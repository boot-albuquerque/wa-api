package send

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"time"

	"go.mau.fi/util/random"
	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/msgattrs"
	armadillo "wa-api/internal/wa-noise/protocol/proto"
	"wa-api/internal/wa-noise/protocol/proto/waArmadilloApplication"
	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/proto/waConsumerApplication"
	"wa-api/internal/wa-noise/protocol/proto/waMsgApplication"
	"wa-api/internal/wa-noise/protocol/types"
)

// Versoes de subprotocolo do caminho v3/FB. A raiz as reexporta com os nomes
// historicos (IGMessageApplicationVersion, FBConsumerMessageVersion,
// FBArmadilloMessageVersion) e o mesmo tipo (constante sem tipo).
const (
	IGApplicationVersion = 3
	ConsumerVersion      = 1
	ArmadilloVersion     = 1
)

// FBMessage e' o corpo de Client.SendFBMessage a partir do ponto em que o
// parametro variadico ja' foi resolvido. Como em Message, as guardas de
// receptor nil e de aridade do variadico ficaram na fachada da raiz.
//
// A ORDEM foi preservada literalmente, e ela e' diferente da de Message: aqui o
// marshal do subprotocolo e o calculo da tag de franking acontecem ANTES das
// guardas de `to.Device` e de login. Parece acidental no upstream, mas e'
// observavel (uma mensagem de tipo nao suportado devolve "unsupported message
// type" mesmo deslogado), entao nao foi mexido.
func FBMessage(
	ctx context.Context,
	t Transport,
	to types.JID,
	message armadillo.RealMessageApplicationSub,
	metadata *waMsgApplication.MessageApplication_Metadata,
	req RequestExtra,
) (resp Response, err error) {
	var subproto waMsgApplication.MessageApplication_SubProtocolPayload
	subproto.FutureProof = waCommon.FutureProofBehavior_PLACEHOLDER.Enum()
	switch typedMsg := message.(type) {
	case *waConsumerApplication.ConsumerApplication:
		var consumerMessage []byte
		consumerMessage, err = proto.Marshal(typedMsg)
		if err != nil {
			err = fmt.Errorf("failed to marshal consumer message: %w", err)
			return
		}
		subproto.SubProtocol = &waMsgApplication.MessageApplication_SubProtocolPayload_ConsumerMessage{
			ConsumerMessage: &waCommon.SubProtocol{
				Payload: consumerMessage,
				Version: proto.Int32(ConsumerVersion),
			},
		}
	case *waArmadilloApplication.Armadillo:
		var armadilloMessage []byte
		armadilloMessage, err = proto.Marshal(typedMsg)
		if err != nil {
			err = fmt.Errorf("failed to marshal armadillo message: %w", err)
			return
		}
		subproto.SubProtocol = &waMsgApplication.MessageApplication_SubProtocolPayload_Armadillo{
			Armadillo: &waCommon.SubProtocol{
				Payload: armadilloMessage,
				Version: proto.Int32(ArmadilloVersion),
			},
		}
	default:
		err = fmt.Errorf("unsupported message type %T", message)
		return
	}
	if metadata == nil {
		metadata = &waMsgApplication.MessageApplication_Metadata{}
	}
	metadata.FrankingVersion = proto.Int32(frankingVersion)
	metadata.FrankingKey = random.Bytes(frankingKeySize)
	msgAttrs := msgattrs.GetAttrsFromFBMessage(message)
	messageAppProto := &waMsgApplication.MessageApplication{
		Payload: &waMsgApplication.MessageApplication_Payload{
			Content: &waMsgApplication.MessageApplication_Payload_SubProtocol{
				SubProtocol: &subproto,
			},
		},
		Metadata: metadata,
	}
	messageApp, err := proto.Marshal(messageAppProto)
	if err != nil {
		return resp, fmt.Errorf("failed to marshal message application: %w", err)
	}
	frankingHash := hmac.New(sha256.New, metadata.FrankingKey)
	frankingHash.Write(messageApp)
	frankingTag := frankingHash.Sum(nil)
	if to.Device > 0 && !req.Peer {
		err = ErrRecipientADJID
		return
	}
	ownID := t.OwnID()
	if ownID.IsEmpty() {
		err = t.Errors().NotLoggedIn
		return
	}

	if req.Timeout == 0 {
		req.Timeout = t.DefaultRequestTimeout()
	}
	if len(req.ID) == 0 {
		req.ID = t.GenerateMessageID()
	}
	resp.ID = req.ID

	start := time.Now()
	// Sending multiple messages at a time can cause weird issues and makes it harder to retry safely
	lock := t.SendLock()
	lock.Lock()
	resp.DebugTimings.Queue = time.Since(start)
	defer lock.Unlock()

	if !req.Peer {
		err = t.AddRecentMessage(ctx, to, req.ID, nil, messageAppProto)
		if err != nil {
			return
		}
	}
	respChan := t.WaitResponse(req.ID)
	var phash string
	var data []byte
	switch to.Server {
	case types.GroupServer:
		phash, data, err = GroupV3(ctx, t, to, ownID, req.ID, messageApp, msgAttrs, frankingTag, &resp.DebugTimings)
	case types.DefaultUserServer, types.MessengerServer:
		if req.Peer {
			err = fmt.Errorf("peer messages to fb are not yet supported")
			//data, err = PeerMessage(ctx, t, to, req.ID, message, &resp.DebugTimings)
		} else {
			data, phash, err = DMV3(ctx, t, to, ownID, req.ID, messageApp, msgAttrs, frankingTag, &resp.DebugTimings)
		}
	default:
		err = fmt.Errorf("%w %s", ErrUnknownServer, to.Server)
	}
	start = time.Now()
	if err != nil {
		t.CancelResponse(req.ID, respChan)
		return
	}
	var respNode *waBinary.Node
	respNode, err = AwaitAck(ctx, t, &req, &resp, respChan, data, start)
	if err != nil {
		return
	}
	err = ApplyAck(t, respNode, to, phash, &resp)
	return
}
