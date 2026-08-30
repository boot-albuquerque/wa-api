package user

import (
	"context"
	"fmt"

	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
)

// AccountKind is this package's own answer to "is this account Business or
// personal", kept separate from pkg/domain.AccountType so that internal/
// never imports pkg/ (the fork stays a library that does not know about the
// application built on top of it — see internal/noise/PATCHES.md).
//
// # THE SIGNAL, AND WHY IT IS RELIABLE
//
// A usync query against ONE'S OWN jid, asking for <business><verified_name>,
// answers with a VerifiedNameCertificate node when-and-only-when the account
// is a WhatsApp Business account. This is the SAME field ParseVerifiedName
// already reads for other people's contacts (see business.go's own comment:
// "Ausencia nao e' erro: um usuario comum, sem conta business, cai aqui"),
// and it is a structured protocol certificate signed by WhatsApp — not an
// inference from the absence of an error. GetBusinessProfile, by contrast, was
// considered and REJECTED as the detection mechanism: it returns
// ElementMissing whenever the <business_profile> node is absent, and that
// error is indistinguishable from a transport/parse failure — exactly the
// "frágil" pattern CLAUDE.md warns against building account-type detection on.
type AccountKind int

const (
	// AccountKindUnknown is returned whenever the query itself failed — the
	// signal was never observed, so no classification is honest here.
	AccountKindUnknown AccountKind = iota
	AccountKindPersonal
	AccountKindBusiness
)

// DetectOwnAccountKind asks the server, via usync, whether ownJID carries a
// verified-name certificate — the same field the usync path already reads for
// other people's contacts. It never returns AccountKindPersonal/Business
// alongside a non-nil error: a caller checks the error first.
func DetectOwnAccountKind(ctx context.Context, t Transport, ownJID types.JID) (AccountKind, error) {
	list, err := USync(ctx, t, []types.JID{ownJID}, ModeQuery, ContextInteractive, []waBinary.Node{
		{Tag: businessNodeTag, Content: []waBinary.Node{{Tag: verifiedNameNodeTag}}},
	})
	if err != nil {
		return AccountKindUnknown, fmt.Errorf("account type: usync query failed: %w", err)
	}

	target := ownJID.ToNonAD()
	for _, child := range list.GetChildren() {
		jid, jidOK := child.Attrs["jid"].(types.JID)
		if child.Tag != usyncUserTag || !jidOK {
			continue
		}
		if jid.ToNonAD() != target {
			continue
		}
		verifiedName, err := ParseVerifiedName(child.GetChildByTag(businessNodeTag))
		if err != nil {
			// The node was present but did not parse. That is a measurement
			// failure, not evidence of "personal" — reporting Personal here
			// would be inventing a classification the data does not support.
			return AccountKindUnknown, fmt.Errorf("account type: parsing verified name: %w", err)
		}
		if verifiedName != nil {
			return AccountKindBusiness, nil
		}
		return AccountKindPersonal, nil
	}

	// The server answered but said nothing about ownJID at all. That is the
	// same shape as a transport failure from this caller's point of view:
	// nothing was measured, so nothing is known.
	return AccountKindUnknown, fmt.Errorf("account type: usync response did not include %s", ownJID)
}
