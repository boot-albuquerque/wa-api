package notification

import (
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types/events"
)

// HandleBlocklist traduz o <blocklist> (que chega dentro de um
// <notification type="account_sync">) em um events.Blocklist.
//
// Um filho com atributos invalidos e' pulado com aviso, mas nao aborta o
// evento: o resto das mudancas ainda e' entregue. Comportamento do upstream,
// preservado verbatim.
func HandleBlocklist(t Transport, node *waBinary.Node) {
	ag := node.AttrGetter()
	evt := events.Blocklist{
		Action:    events.BlocklistAction(ag.OptionalString("action")),
		DHash:     ag.String("dhash"),
		PrevDHash: ag.OptionalString("prev_dhash"),
	}
	for _, child := range node.GetChildren() {
		ag := child.AttrGetter()
		change := events.BlocklistChange{
			JID:    ag.JID("jid"),
			Action: events.BlocklistChangeAction(ag.String("action")),
		}
		if !ag.OK() {
			t.Log().Warnf("Unexpected data in blocklist event child %v: %v", child.XMLString(), ag.Error())
			continue
		}
		evt.Changes = append(evt.Changes, change)
	}
	t.DispatchEvent(&evt)
}
