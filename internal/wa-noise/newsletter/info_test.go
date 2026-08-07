// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package newsletter

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

func TestJIDInput(t *testing.T) {
	jid := testJID()
	input := JIDInput(jid)

	if input["key"] != jid.String() {
		t.Errorf("key = %v, esperava %q", input["key"], jid.String())
	}
	if input["type"] != types.NewsletterKeyTypeJID {
		t.Errorf("type = %v, esperava %v", input["type"], types.NewsletterKeyTypeJID)
	}
}

// O convite aceita o link completo ou so' o codigo. Mandar o link inteiro para
// o servidor devolveria "newsletter nao encontrada".
func TestInviteInputCortaPrefixoDoLink(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"link completo", LinkPrefix + "AbCdEf123", "AbCdEf123"},
		{"so o codigo", "AbCdEf123", "AbCdEf123"},
		{"prefixo so' no meio nao e' cortado", "x" + LinkPrefix + "AbCdEf123", "x" + LinkPrefix + "AbCdEf123"},
		{"vazio", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := InviteInput(tt.in)
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

// GetInfo manda as quatro chaves de "variables" e propaga fetchViewerMeta.
func TestGetInfoMontaAsVariaveis(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexJSON(`{"data":{"xwa2_newsletter":{"id":"1234567890@newsletter"}}}`)

	meta, err := GetInfo(context.Background(), f, JIDInput(testJID()), true)
	if err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	if meta == nil || meta.ID.User != "1234567890" {
		t.Fatalf("meta = %+v", meta)
	}

	nodes := f.iqs[0].Content.([]waBinary.Node)
	var payload struct {
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(nodes[0].Content.([]byte), &payload); err != nil {
		t.Fatalf("unmarshal do payload: %v", err)
	}
	for _, key := range []string{"fetch_creation_time", "fetch_full_image", "fetch_viewer_metadata", "input"} {
		if _, ok := payload.Variables[key]; !ok {
			t.Errorf("variables sem a chave %q: %v", key, payload.Variables)
		}
	}
	if payload.Variables["fetch_viewer_metadata"] != true {
		t.Errorf("fetch_viewer_metadata = %v, esperava true", payload.Variables["fetch_viewer_metadata"])
	}
}

func TestGetInfoFetchViewerMetaFalso(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexJSON(`{"data":{}}`)

	if _, err := GetInfo(context.Background(), f, InviteInput("abc"), false); err != nil {
		t.Fatalf("GetInfo: %v", err)
	}
	nodes := f.iqs[0].Content.([]waBinary.Node)
	if !strings.Contains(string(nodes[0].Content.([]byte)), `"fetch_viewer_metadata":false`) {
		t.Errorf("payload = %s", nodes[0].Content)
	}
}

// Erro de rede sem data: nada a decodificar, o erro passa direto.
func TestGetInfoPropagaErroSemData(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("sem rede")
	f.iqErr = sentinel

	meta, err := GetInfo(context.Background(), f, JIDInput(testJID()), true)
	if meta != nil {
		t.Errorf("meta = %+v, esperava nil", meta)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, esperava %v", err, sentinel)
	}
}

// Erro GraphQL vem com data parcial. O erro do servidor tem prioridade sobre
// qualquer erro de unmarshal — trocar essa ordem esconderia a causa real.
func TestGetInfoErroDoServidorTemPrioridadeSobreUnmarshal(t *testing.T) {
	f := newFakeTransport()
	// data parcial invalido para respGetNewsletterInfo (xwa2_newsletter e' um
	// numero, nao um objeto), garantindo que o unmarshal tambem falharia.
	f.iqResp = mexJSON(`{"data":{"xwa2_newsletter":7},"errors":[{"message":"nope","extensions":{"error_code":1,"severity":"CRITICAL"}}]}`)

	_, err := GetInfo(context.Background(), f, JIDInput(testJID()), true)
	if err == nil || !strings.Contains(err.Error(), "graphql error") {
		t.Fatalf("err = %v, esperava o erro graphql", err)
	}
}

// Sem erro de rede, um data mal formado vira erro de unmarshal.
func TestGetInfoErroDeUnmarshalQuandoNaoHaErroDeRede(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexJSON(`{"data":{"xwa2_newsletter":7}}`)

	if _, err := GetInfo(context.Background(), f, JIDInput(testJID()), true); err == nil {
		t.Fatal("esperava erro de unmarshal")
	}
}

func TestGetSubscribedSucesso(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexJSON(`{"data":{"xwa2_newsletter_subscribed":[{"id":"1@newsletter"},{"id":"2@newsletter"}]}}`)

	list, err := GetSubscribed(context.Background(), f)
	if err != nil {
		t.Fatalf("GetSubscribed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("esperava 2, veio %d", len(list))
	}
	// A consulta nao leva variaveis, mas o campo "variables" precisa existir.
	nodes := f.iqs[0].Content.([]waBinary.Node)
	if string(nodes[0].Content.([]byte)) != `{"variables":{}}` {
		t.Errorf("payload = %s", nodes[0].Content)
	}
}

func TestGetSubscribedPropagaErroSemData(t *testing.T) {
	f := newFakeTransport()
	sentinel := errors.New("sem rede")
	f.iqErr = sentinel

	list, err := GetSubscribed(context.Background(), f)
	if list != nil {
		t.Errorf("list = %v, esperava nil", list)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, esperava %v", err, sentinel)
	}
}

func TestGetSubscribedErroDeUnmarshal(t *testing.T) {
	f := newFakeTransport()
	f.iqResp = mexJSON(`{"data":{"xwa2_newsletter_subscribed":7}}`)

	if _, err := GetSubscribed(context.Background(), f); err == nil {
		t.Fatal("esperava erro de unmarshal")
	}
}
