package wanoise

import (
	"wa-api/internal/wa-noise/capabilities/group"
	"wa-api/internal/wa-noise/persistence/store"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// Fachada; ver o cabecalho de group.go. Os quatro metodos abaixo sao citados
// por internals.go (gerado) e o ultimo e' chamado por notification.go.

func (cli *Client) parseGroupCreate(parentNode, node *waBinary.Node) (*events.JoinedGroup, []store.LIDMapping, []store.RedactedPhoneEntry, error) {
	return group.ParseCreate(cli.groupT(), parentNode, node)
}

func (cli *Client) parseGroupChange(node *waBinary.Node) (*events.GroupInfo, []store.LIDMapping, error) {
	return group.ParseChange(cli.groupT(), node)
}

func (cli *Client) updateGroupParticipantCache(evt *events.GroupInfo) {
	group.UpdateParticipantCache(cli.groupT(), evt)
}

func (cli *Client) parseGroupNotification(node *waBinary.Node) (any, []store.LIDMapping, []store.RedactedPhoneEntry, error) {
	return group.ParseNotification(cli.groupT(), node)
}
