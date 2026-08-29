package user

import (
	"context"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// ParseBlocklist le o <list> da resposta de blocklist.
//
// A tag dos filhos nao e' conferida: o criterio de aceitacao e' o `jid` ser
// lido com sucesso.
func ParseBlocklist(t Transport, node *waBinary.Node) *types.Blocklist {
	output := &types.Blocklist{
		DHash: node.AttrGetter().String("dhash"),
	}
	for _, child := range node.GetChildren() {
		ag := child.AttrGetter()
		blockedJID := ag.JID("jid")
		if !ag.OK() {
			t.Log().Debugf("Ignoring contact blocked data with unexpected attributes: %v", ag.Error())
			continue
		}

		output.JIDs = append(output.JIDs, blockedJID)
	}
	return output
}

// GetBlocklist gets the list of users that this user has blocked.
func GetBlocklist(ctx context.Context, t Transport) (*types.Blocklist, error) {
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: blocklistIQNamespace,
		Type:      IQGet,
		To:        types.ServerJID,
	})
	if err != nil {
		return nil, err
	}
	list, ok := resp.GetOptionalChildByTag("list")
	if !ok {
		return nil, t.ElementMissing("list", "response to blocklist query")
	}
	return ParseBlocklist(t, &list), nil
}

// UpdateBlocklist updates the user's block list and returns the updated list.
//
// pnJID is the phone-number JID counterpart of jid, required by the server
// alongside a LID `jid` when action is block (LIB-02, porting whatsmeow
// 8d023aa973 / Baileys 8ca9316a10: the blocklist write migrated to LID
// addressing, and a block additionally carries `pn_jid`; unblock does not).
// Pass a zero JID when the caller has no PN to offer — the attribute is
// then omitted, same as before this parameter existed.
func UpdateBlocklist(
	ctx context.Context, t Transport,
	jid types.JID, pnJID types.JID, action events.BlocklistChangeAction,
) (*types.Blocklist, error) {
	attrs := waBinary.Attrs{
		"jid":    jid,
		"action": string(action),
	}
	if action == events.BlocklistChangeActionBlock && !pnJID.IsEmpty() {
		attrs["pn_jid"] = pnJID
	}
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: blocklistIQNamespace,
		Type:      IQSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag:   "item",
			Attrs: attrs,
		}},
	})
	if err != nil {
		return nil, err
	}
	list, ok := resp.GetOptionalChildByTag("list")
	if !ok {
		return nil, t.ElementMissing("list", "response to blocklist update")
	}
	return ParseBlocklist(t, &list), err
}
