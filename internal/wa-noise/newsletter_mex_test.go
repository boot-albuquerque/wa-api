// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"testing"

	"wa-api/internal/wa-noise/argo"
	"wa-api/internal/wa-noise/proto/waWa6"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/util/keys"
)

// newMexTestClient devolve um Client com o minimo que convertQueryID precisa:
// um Store capaz de montar o payload de registro (sem JID, sem socket).
func newMexTestClient() *Client {
	device := &store.Device{
		IdentityKey:    keys.NewKeyPair(),
		NoiseKey:       keys.NewKeyPair(),
		RegistrationID: 0x01020304,
	}
	device.SignedPreKey = device.IdentityKey.CreateSignedPreKey(0x00AABBCC)
	return &Client{Store: device}
}

// withoutWebInfo zera o WebInfo do payload base (restaurando no fim), que e' o
// que de fato faz convertQueryID escolher as query IDs de desktop.
func withoutWebInfo(t *testing.T) {
	t.Helper()
	original := store.BaseClientPayload.WebInfo
	store.BaseClientPayload.WebInfo = nil
	t.Cleanup(func() {
		store.BaseClientPayload.WebInfo = original
	})
}

// Com WebInfo presente (cliente web), as query IDs passam intactas.
func TestConvertQueryIDWebMantemIDs(t *testing.T) {
	cli := newMexTestClient()
	for _, id := range []string{
		queryFetchNewsletter,
		queryRecommendedNewsletters,
		querySubscribedNewsletters,
		queryNewsletterSubscribers,
		mutationMuteNewsletter,
		mutationUnmuteNewsletter,
		mutationUpdateNewsletter,
		mutationCreateNewsletter,
		mutationUnfollowNewsletter,
		mutationFollowNewsletter,
	} {
		if got := convertQueryID(cli, id); got != id {
			t.Errorf("convertQueryID(%q) = %q, esperava o mesmo ID", id, got)
		}
	}
}

// Sem WebInfo (desktop/companion), cada ID web vira o ID desktop equivalente.
// Um mapeamento errado aqui faz o servidor recusar a consulta inteira.
func TestConvertQueryIDDesktopMapeiaTodasAsIDs(t *testing.T) {
	withoutWebInfo(t)
	cli := newMexTestClient()

	for web, desktop := range map[string]string{
		queryFetchNewsletter:        queryFetchNewsletterDesktop,
		queryRecommendedNewsletters: queryRecommendedNewslettersDesktop,
		querySubscribedNewsletters:  querySubscribedNewslettersDesktop,
		queryNewsletterSubscribers:  queryNewsletterSubscribersDesktop,
		mutationMuteNewsletter:      mutationMuteNewsletterDesktop,
		mutationUnmuteNewsletter:    mutationUnmuteNewsletterDesktop,
		mutationUpdateNewsletter:    mutationUpdateNewsletterDesktop,
		mutationCreateNewsletter:    mutationCreateNewsletterDesktop,
		mutationUnfollowNewsletter:  mutationUnfollowNewsletterDesktop,
		mutationFollowNewsletter:    mutationFollowNewsletterDesktop,
	} {
		if got := convertQueryID(cli, web); got != desktop {
			t.Errorf("convertQueryID(%q) = %q, esperava %q", web, got, desktop)
		}
	}
}

// IDs fora da tabela (e as proprias IDs de desktop) passam sem traducao.
func TestConvertQueryIDDesktopPassaDesconhecidas(t *testing.T) {
	withoutWebInfo(t)
	cli := newMexTestClient()

	for _, id := range []string{
		queryFetchNewsletterDehydrated,
		queryNewslettersDirectory,
		queryFetchNewsletterDesktop,
		"0000000000000000",
	} {
		if got := convertQueryID(cli, id); got != id {
			t.Errorf("convertQueryID(%q) = %q, esperava o mesmo ID", id, got)
		}
	}
}

// Documenta um bug herdado do upstream: convertQueryID compara
// `payload.GetUserAgent().Platform == waWa6...MACOS.Enum()`, ou seja, dois
// PONTEIROS diferentes — a comparacao e' sempre falsa. Na pratica so' o
// `GetWebInfo() == nil` decide. Ver HOUSEKEEP.md (F31).
func TestConvertQueryIDPlatformMacOSNaoDecideSozinho(t *testing.T) {
	original := store.BaseClientPayload.UserAgent.Platform
	store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_MACOS.Enum()
	t.Cleanup(func() {
		store.BaseClientPayload.UserAgent.Platform = original
	})

	cli := newMexTestClient()
	// WebInfo continua presente, entao o ramo desktop nao e' escolhido, apesar
	// da plataforma MACOS.
	if got := convertQueryID(cli, queryFetchNewsletter); got != queryFetchNewsletter {
		t.Errorf("convertQueryID = %q, esperava %q (a comparacao de ponteiro e' inerte)", got, queryFetchNewsletter)
	}
}

// Toda ID de desktop emitida por convertQueryID precisa ter um wire type Argo
// correspondente; sem isso a decodificacao Argo nao teria como montar a
// resposta quando o caminho for reabilitado.
func TestQueryIDsDesktopTemWireTypeArgo(t *testing.T) {
	queryIDMap, err := argo.GetQueryIDToMessageName()
	if err != nil {
		t.Fatalf("argo.GetQueryIDToMessageName: %v", err)
	}
	wireStore, err := argo.GetStore()
	if err != nil {
		t.Fatalf("argo.GetStore: %v", err)
	}

	for _, id := range []string{
		queryFetchNewsletterDesktop,
		queryRecommendedNewslettersDesktop,
		querySubscribedNewslettersDesktop,
		queryNewsletterSubscribersDesktop,
		mutationMuteNewsletterDesktop,
		mutationUnmuteNewsletterDesktop,
		mutationUpdateNewsletterDesktop,
		mutationCreateNewsletterDesktop,
	} {
		name, ok := queryIDMap[id]
		if !ok {
			t.Errorf("query ID de desktop %q nao esta' em name-to-queryids.json", id)
			continue
		}
		if _, ok := wireStore[name]; !ok {
			t.Errorf("query ID %q mapeia para %q, que nao esta' no wire type store", id, name)
		}
	}
}

// mutationUnfollowNewsletterDesktop e' a unica ID de desktop que nao tem wire
// type Argo: ela mapeia para "WamoSubCancelSubscription" (cancelamento de
// assinatura paga, nao "deixar de seguir canal") e esse nome nao existe no
// wire type store. Registrado em HOUSEKEEP.md (F32); o teste trava o estado
// atual para que a correcao seja deliberada.
func TestQueryIDUnfollowDesktopSemWireTypeArgo(t *testing.T) {
	queryIDMap, err := argo.GetQueryIDToMessageName()
	if err != nil {
		t.Fatalf("argo.GetQueryIDToMessageName: %v", err)
	}
	wireStore, err := argo.GetStore()
	if err != nil {
		t.Fatalf("argo.GetStore: %v", err)
	}
	name := queryIDMap[mutationUnfollowNewsletterDesktop]
	if name != "WamoSubCancelSubscription" {
		t.Fatalf("mapeamento mudou para %q — atualize HOUSEKEEP.md (F32) e este teste", name)
	}
	if _, ok := wireStore[name]; ok {
		t.Fatal("o wire type apareceu — atualize HOUSEKEEP.md (F32) e este teste")
	}
}

// mutationFollowNewsletterDesktop e querySubscribedNewslettersDesktop tem o
// MESMO valor no upstream, e por isso "seguir canal" no desktop dispara a
// consulta de canais assinados. Registrado em HOUSEKEEP.md (F32); o teste
// trava o estado atual para que a correcao seja deliberada.
func TestQueryIDsDesktopDuplicadaConhecida(t *testing.T) {
	if mutationFollowNewsletterDesktop != querySubscribedNewslettersDesktop {
		t.Fatal("a duplicata conhecida sumiu — atualize HOUSEKEEP.md (F32) e este teste")
	}
}

// O guard de MACOS aborta sendMexIQ antes de tocar no socket: e' o unico
// caminho de sendMexIQ exercitavel sem sessao Noise aberta.
func TestSendMexIQRecusaEmMacOS(t *testing.T) {
	original := store.BaseClientPayload.UserAgent.Platform
	store.BaseClientPayload.UserAgent.Platform = waWa6.ClientPayload_UserAgent_MACOS.Enum()
	t.Cleanup(func() {
		store.BaseClientPayload.UserAgent.Platform = original
	})

	data, err := (&Client{}).sendMexIQ(context.Background(), queryFetchNewsletter, nil)
	if data != nil {
		t.Errorf("esperava data nil, veio %s", data)
	}
	if !errors.Is(err, errArgoDecodingBroken) {
		t.Fatalf("err = %v, esperava errArgoDecodingBroken", err)
	}
}
