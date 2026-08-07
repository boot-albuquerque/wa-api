package notification

import (
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// HandlePicture traduz um <notification type="picture"> em um events.Picture
// por filho reconhecido.
//
// Um evento e' despachado por filho, e nao um por notificacao: o servidor pode
// mandar varias mudancas no mesmo no. Filhos com tag desconhecida e filhos com
// atributos invalidos sao pulados — os primeiros em silencio (o `continue`
// antes da checagem de ag.OK()), os segundos com log de debug. A ordem das
// duas checagens e' do upstream e importa: uma tag desconhecida nunca chega a
// gerar aviso de atributo.
func HandlePicture(t Transport, node *waBinary.Node) {
	ts := node.AttrGetter().UnixTime("t")
	for _, child := range node.GetChildren() {
		ag := child.AttrGetter()
		var evt events.Picture
		evt.Timestamp = ts
		evt.JID = ag.JID("jid")
		evt.Author = ag.OptionalJIDOrEmpty("author")
		if child.Tag == "delete" {
			evt.Remove = true
		} else if child.Tag == "add" {
			evt.PictureID = ag.String("id")
		} else if child.Tag == "set" {
			// TODO sometimes there's a hash and no ID?
			evt.PictureID = ag.String("id")
		} else {
			continue
		}
		if !ag.OK() {
			t.Log().Debugf("Ignoring picture change notification with unexpected attributes: %v", ag.Error())
			continue
		}
		t.DispatchEvent(&evt)
	}
}
