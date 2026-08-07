package wanoise

import (
	"context"
	"fmt"

	"wa-api/internal/wa-noise/capabilities/media"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waMmsRetry"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// A cripto do retry de midia (derivacao da chave, cifragem e decifragem do
// receipt) vive em internal/wa-noise/media/retry.go. O que ficou aqui e' o que
// depende do pacote raiz: o envio do stanza (sendNode/getOwnID) e a leitura do
// <notification> (que usa ElementMissingError e o dispatch de eventos).

// SendMediaRetryReceipt sends a request to the phone to re-upload the media in a message.
//
// This is mostly relevant when handling history syncs and getting a 404 or 410 error downloading media.
// Rough example on how to use it (will not work out of the box, you must adjust it depending on what you need exactly):
//
//	var mediaRetryCache map[types.MessageID]*waE2E.ImageMessage
//
//	evt, err := cli.ParseWebMessage(chatJID, historyMsg.GetMessage())
//	imageMsg := evt.Message.GetImageMessage() // replace this with the part of the message you want to download
//	data, err := cli.Download(imageMsg)
//	if errors.Is(err, wa-noise.ErrMediaDownloadFailedWith404) || errors.Is(err, wa-noise.ErrMediaDownloadFailedWith410) {
//	  err = cli.SendMediaRetryReceipt(&evt.Info, imageMsg.GetMediaKey())
//	  // You need to store the event data somewhere as it's necessary for handling the retry response.
//	  mediaRetryCache[evt.Info.ID] = imageMsg
//	}
//
// The response will come as an *events.MediaRetry. The response will then have to be decrypted
// using DecryptMediaRetryNotification and the same media key passed here. If the media retry was successful,
// the decrypted notification should contain an updated DirectPath, which can be used to download the file.
//
//	func eventHandler(rawEvt interface{}) {
//	  switch evt := rawEvt.(type) {
//	  case *events.MediaRetry:
//	    imageMsg := mediaRetryCache[evt.MessageID]
//	    retryData, err := wa-noise.DecryptMediaRetryNotification(evt, imageMsg.GetMediaKey())
//	    if err != nil || retryData.GetResult != waMmsRetry.MediaRetryNotification_SUCCESS {
//	      return
//	    }
//	    // Use the new path to download the attachment
//	    imageMsg.DirectPath = retryData.DirectPath
//	    data, err := cli.Download(imageMsg)
//	    // Alternatively, you can use cli.DownloadMediaWithPath and provide the individual fields manually.
//	  }
//	}
func (cli *Client) SendMediaRetryReceipt(ctx context.Context, message *types.MessageInfo, mediaKey []byte) error {
	if cli == nil {
		return ErrClientIsNil
	}
	ciphertext, iv, err := media.EncryptRetryReceipt(message.ID, mediaKey)
	if err != nil {
		return fmt.Errorf("failed to prepare encrypted retry receipt: %w", err)
	}
	ownID := cli.getOwnID().ToNonAD()
	if ownID.IsEmpty() {
		return ErrNotLoggedIn
	}

	rmrAttrs := waBinary.Attrs{
		"jid":     message.Chat,
		"from_me": message.IsFromMe,
	}
	if message.IsGroup {
		rmrAttrs["participant"] = message.Sender
	}

	encryptedRequest := []waBinary.Node{
		{Tag: "enc_p", Content: ciphertext},
		{Tag: "enc_iv", Content: iv},
	}

	return cli.sendNode(ctx, waBinary.Node{
		Tag: "receipt",
		Attrs: waBinary.Attrs{
			"id":   message.ID,
			"to":   ownID,
			"type": "server-error",
		},
		Content: []waBinary.Node{
			{Tag: "encrypt", Content: encryptedRequest},
			{Tag: "rmr", Attrs: rmrAttrs},
		},
	})
}

// DecryptMediaRetryNotification decrypts a media retry notification using the media key.
// See Client.SendMediaRetryReceipt for more info on how to use this.
func DecryptMediaRetryNotification(evt *events.MediaRetry, mediaKey []byte) (*waMmsRetry.MediaRetryNotification, error) {
	return media.DecryptRetryNotification(evt, mediaKey)
}

func parseMediaRetryNotification(node *waBinary.Node) (*events.MediaRetry, error) {
	ag := node.AttrGetter()
	var evt events.MediaRetry
	evt.Timestamp = ag.UnixTime("t")
	evt.MessageID = types.MessageID(ag.String("id"))
	if !ag.OK() {
		return nil, ag.Error()
	}
	rmr, ok := node.GetOptionalChildByTag("rmr")
	if !ok {
		return nil, &ElementMissingError{Tag: "rmr", In: "retry notification"}
	}
	rmrAG := rmr.AttrGetter()
	evt.ChatID = rmrAG.JID("jid")
	evt.FromMe = rmrAG.Bool("from_me")
	evt.SenderID = rmrAG.OptionalJIDOrEmpty("participant")
	if !rmrAG.OK() {
		return nil, fmt.Errorf("missing attributes in <rmr> tag: %w", rmrAG.Error())
	}

	errNode, ok := node.GetOptionalChildByTag("error")
	if ok {
		evt.Error = &events.MediaRetryError{
			Code: errNode.AttrGetter().Int("code"),
		}
		return &evt, nil
	}

	evt.Ciphertext, ok = node.GetChildByTag("encrypt", "enc_p").Content.([]byte)
	if !ok {
		return nil, &ElementMissingError{Tag: "enc_p", In: fmt.Sprintf("retry notification %s", evt.MessageID)}
	}
	evt.IV, ok = node.GetChildByTag("encrypt", "enc_iv").Content.([]byte)
	if !ok {
		return nil, &ElementMissingError{Tag: "enc_iv", In: fmt.Sprintf("retry notification %s", evt.MessageID)}
	}
	return &evt, nil
}

func (cli *Client) handleMediaRetryNotification(ctx context.Context, node *waBinary.Node) {
	evt, err := parseMediaRetryNotification(node)
	if err != nil {
		cli.Log.Warnf("Failed to parse media retry notification: %v", err)
		return
	}
	cli.dispatchEvent(evt)
}
