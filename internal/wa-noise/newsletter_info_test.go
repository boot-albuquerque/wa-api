// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"encoding/json"
	"testing"

	"wa-api/internal/wa-noise/types"
)

func TestNewsletterJIDInput(t *testing.T) {
	jid := types.NewJID("1234567890", types.NewsletterServer)
	input := newsletterJIDInput(jid)

	if input["key"] != jid.String() {
		t.Errorf("key = %v, esperava %q", input["key"], jid.String())
	}
	if input["type"] != types.NewsletterKeyTypeJID {
		t.Errorf("type = %v, esperava %v", input["type"], types.NewsletterKeyTypeJID)
	}
}

// O convite aceita o link completo ou so' o codigo. Mandar o link inteiro para
// o servidor devolveria "newsletter nao encontrada".
func TestNewsletterInviteInputCortaPrefixoDoLink(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"link completo", NewsletterLinkPrefix + "AbCdEf123", "AbCdEf123"},
		{"so o codigo", "AbCdEf123", "AbCdEf123"},
		{"prefixo so' no meio nao e' cortado", "x" + NewsletterLinkPrefix + "AbCdEf123", "x" + NewsletterLinkPrefix + "AbCdEf123"},
		{"vazio", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := newsletterInviteInput(tt.in)
			if input["key"] != tt.want {
				t.Errorf("key = %v, esperava %q", input["key"], tt.want)
			}
			if input["type"] != types.NewsletterKeyTypeInvite {
				t.Errorf("type = %v, esperava %v", input["type"], types.NewsletterKeyTypeInvite)
			}
		})
	}
}

// Os nomes dos campos GraphQL sao contrato de wire: renomear a tag faz o
// Unmarshal devolver nil silenciosamente, sem erro.
func TestRespGetNewsletterInfoLeCampoXwa2Newsletter(t *testing.T) {
	var resp respGetNewsletterInfo
	if err := json.Unmarshal([]byte(`{"xwa2_newsletter":{"id":"1234567890@newsletter"}}`), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Newsletter == nil {
		t.Fatal("xwa2_newsletter deveria ter sido decodificado")
	}
	if resp.Newsletter.ID.User != "1234567890" {
		t.Errorf("ID.User = %q", resp.Newsletter.ID.User)
	}
}

func TestRespGetSubscribedNewslettersLeCampoXwa2NewsletterSubscribed(t *testing.T) {
	var resp respGetSubscribedNewsletters
	if err := json.Unmarshal([]byte(`{"xwa2_newsletter_subscribed":[{"id":"1@newsletter"},{"id":"2@newsletter"}]}`), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Newsletters) != 2 {
		t.Fatalf("esperava 2 newsletters, veio %d", len(resp.Newsletters))
	}
}

func TestRespCreateNewsletterLeCampoXwa2NewsletterCreate(t *testing.T) {
	var resp respCreateNewsletter
	if err := json.Unmarshal([]byte(`{"xwa2_newsletter_create":{"id":"1234567890@newsletter"}}`), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Newsletter == nil {
		t.Fatal("xwa2_newsletter_create deveria ter sido decodificado")
	}
}

// CreateNewsletterParams vai no corpo da mutation. Description e Picture sao
// omitempty; Name nao — o servidor exige o campo presente.
func TestCreateNewsletterParamsOmiteOpcionais(t *testing.T) {
	b, err := json.Marshal(CreateNewsletterParams{Name: "Canal"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != `{"name":"Canal"}` {
		t.Errorf("payload = %s", b)
	}
}
