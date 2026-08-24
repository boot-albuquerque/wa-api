package group

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Reading a group's own facts: who owns it, when it was made, who is in it.
//
// These are PROPERTIES in the reference, not calls, populated from metadata this
// module already fetches for other reasons. The only real question was where the
// values live on this build — which has hidden them behind getters and mixins
// four times (H83, H94, H103, H104). Measured, and one of them is hidden a fifth
// way; see DescriptionSource.

var (
	// ErrMetadata is the page refusing or failing the metadata read.
	ErrMetadata = fmt.Errorf("group: the page refused the metadata read")
)

// Participant is one member.
type Participant struct {
	// JID is the member's identity. On this build it arrives as a LID.
	JID string
	// Admin and SuperAdmin are the page's own booleans. SuperAdmin is the
	// creator; Admin includes them.
	Admin, SuperAdmin bool
	// JoinedAt is when they came in. Zero when the page does not say.
	JoinedAt time.Time
}

func (p Participant) String() string {
	return fmt.Sprintf("group.Participant(jid=%t admin=%t superAdmin=%t joined=%t)",
		p.JID != "", p.Admin, p.SuperAdmin, !p.JoinedAt.IsZero())
}

// Metadata is what a group says about itself.
type Metadata struct {
	JID     string
	Subject string
	// Owner is who created it, serialized. Empty when the page does not say —
	// which happens for groups this account joined rather than made.
	Owner string
	// CreatedAt is when it was made. Zero when absent.
	CreatedAt time.Time
	// Description is the group's text, and DescriptionSource says WHERE it was
	// found — see that field before trusting an empty string.
	Description string
	// DescriptionSource is the field the description came from, or "none".
	//
	// IT EXISTS BECAUSE AN EMPTY DESCRIPTION IS AMBIGUOUS HERE. Measured on the
	// lab group: both `desc` and `displayedDesc` read undefined, while
	// `descTime` is a number and `__x_displayedDesc` exists in raw storage. That
	// is consistent with a group that has no description AND with a reader
	// looking at the wrong field, and one group cannot separate them.
	//
	// Rather than pick one and let every caller receive "" forever — the defect
	// class this repository has met five times — both are read and the winner is
	// named. A caller seeing source "none" knows it got nothing, not that there
	// is nothing.
	DescriptionSource string
	// DescriptionAt is when the description last changed, which the page
	// reports even when the text itself does not come through.
	DescriptionAt time.Time
	Participants  []Participant
}

func (m Metadata) String() string {
	admins := 0
	for _, p := range m.Participants {
		if p.Admin {
			admins++
		}
	}
	return fmt.Sprintf("group.Metadata(jid=%t subject=%t owner=%t createdAt=%t desc=%t descFrom=%s participants=%d admins=%d)",
		m.JID != "", m.Subject != "", m.Owner != "", !m.CreatedAt.IsZero(),
		m.Description != "", m.DescriptionSource, len(m.Participants), admins)
}

// Metadata reads a group's own facts.
//
// IT REFRESHES FROM THE SERVER FIRST, for the same reason the membership-request
// read does: a session holds what it was given at boot, and anything that
// changed since is not in it.
func (m *Manager) Metadata(ctx context.Context, groupJID, label string) (Metadata, error) {
	if !strings.HasSuffix(strings.TrimSpace(groupJID), "@g.us") {
		return Metadata{}, ErrNotGroup
	}
	raw, err := m.parkedJoin(ctx, metadataScript(groupJID), label+"/metadata")
	if err != nil {
		return Metadata{}, fmt.Errorf("%w: %v", ErrMetadata, err)
	}
	var out struct {
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		JID     string `json:"jid"`
		Subject string `json:"subject"`
		Owner   string `json:"owner"`
		Created int64  `json:"created"`
		Desc    string `json:"desc"`
		DescSrc string `json:"descSource"`
		DescT   int64  `json:"descTime"`
		People  []struct {
			JID        string `json:"jid"`
			Admin      bool   `json:"admin"`
			SuperAdmin bool   `json:"superAdmin"`
			Joined     int64  `json:"joined"`
		} `json:"participants"`
	}
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		return Metadata{}, fmt.Errorf("group: unexpected metadata answer: %w", e)
	}
	if !out.OK {
		if out.Why == "NO_CHAT" || out.Why == "NO_METADATA" {
			return Metadata{}, fmt.Errorf("%w (%s)", ErrNotGroup, out.Why)
		}
		return Metadata{}, fmt.Errorf("%w (%s)", ErrMetadata, out.Why)
	}
	md := Metadata{
		JID: out.JID, Subject: out.Subject, Owner: out.Owner,
		Description: out.Desc, DescriptionSource: out.DescSrc,
	}
	if out.Created > 0 {
		md.CreatedAt = time.Unix(out.Created, 0)
	}
	if out.DescT > 0 {
		md.DescriptionAt = time.Unix(out.DescT, 0)
	}
	for _, p := range out.People {
		part := Participant{JID: p.JID, Admin: p.Admin, SuperAdmin: p.SuperAdmin}
		if p.Joined > 0 {
			part.JoinedAt = time.Unix(p.Joined, 0)
		}
		md.Participants = append(md.Participants, part)
	}
	return md, nil
}

func metadataScript(groupJID string) string {
	return `(() => {` + joinPrelude + `
	(async () => {
		try {
			// REFRESCA ANTES DE LER. Uma sessao guarda o que recebeu no boot.
			await window.require("WAWebGroupQueryJob")
				.queryAndUpdateGroupMetadataById({ id: ` + strconv.Quote(groupJID) + ` });
			const CC = window.require("WAWebChatCollection").ChatCollection;
			const chat = CC.get(` + strconv.Quote(groupJID) + `);
			if (!chat) { park({ ok: false, why: "NO_CHAT" }); return; }
			const md = chat.groupMetadata;
			if (!md) { park({ ok: false, why: "NO_METADATA" }); return; }

			// A DESCRICAO E PROCURADA EM DOIS LUGARES E A FONTE E REPORTADA.
			// Nao e fallback que engole: o nome do campo vencedor viaja junto,
			// para que "" nunca signifique as duas coisas ao mesmo tempo.
			let desc = "", descSource = "none";
			if (typeof md.displayedDesc === "string" && md.displayedDesc !== "") {
				desc = md.displayedDesc; descSource = "displayedDesc";
			} else if (typeof md.desc === "string" && md.desc !== "") {
				desc = md.desc; descSource = "desc";
			}

			const people = [];
			try {
				const p = md.participants;
				const arr = p && (typeof p.getModelsArray === "function" ? p.getModelsArray()
					: (typeof p.toArray === "function" ? p.toArray() : []));
				for (const one of (arr || [])) {
					try {
						people.push({
							jid: jidOf(one.id),
							admin: !!one.isAdmin,
							superAdmin: !!one.isSuperAdmin,
							joined: (typeof one.joinTime === "number") ? one.joinTime : 0,
						});
					} catch (e) {}
				}
			} catch (e) {}

			park({
				ok: true,
				jid: jidOf(md.id) || ` + strconv.Quote(groupJID) + `,
				subject: (typeof md.subject === "string") ? md.subject : "",
				owner: jidOf(md.owner),
				created: (typeof md.creation === "number") ? md.creation : 0,
				desc: desc, descSource: descSource,
				descTime: (typeof md.descTime === "number") ? md.descTime : 0,
				participants: people,
			});
		} catch (e) {
			park({ ok: false, why: describe(e) });
		}
	})();
	return "kicked";
	})()`
}
