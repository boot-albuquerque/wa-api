package sqlstore

import (
	"context"
	"fmt"
	"testing"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
)

func contactJID(user string) types.JID {
	return types.JID{User: user, Server: types.DefaultUserServer}
}

func TestGetContactUnknownIsNotFound(t *testing.T) {
	info, err := newTestStore(t).GetContact(context.Background(), contactJID("1"))
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	if info.Found {
		t.Fatal("contato inexistente nao pode vir com Found=true")
	}
	if info.FullName != "" || info.PushName != "" {
		t.Fatalf("contato inexistente deveria vir zerado: %+v", info)
	}
}

func TestPutContactNameRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	jid := contactJID("1")
	if err := s.PutContactName(ctx, jid, "Ana", "Ana Silva"); err != nil {
		t.Fatalf("PutContactName: %v", err)
	}
	info, err := s.GetContact(ctx, jid)
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	if !info.Found || info.FirstName != "Ana" || info.FullName != "Ana Silva" {
		t.Fatalf("GetContact = %+v", info)
	}
}

// PutPushName devolve (mudou, nomeAnterior). Quem chama usa isso para emitir o
// evento de mudanca de push name — se devolvesse true sempre, o cliente
// dispararia evento a cada mensagem recebida.
func TestPutPushNameReportsChangeAndPreviousName(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	jid := contactJID("1")

	changed, previous, err := s.PutPushName(ctx, jid, "Ana")
	if err != nil {
		t.Fatalf("PutPushName 1: %v", err)
	}
	if !changed {
		t.Fatal("o primeiro push name deveria contar como mudanca")
	}
	if previous != "" {
		t.Fatalf("nome anterior deveria ser vazio, veio %q", previous)
	}

	changed, previous, err = s.PutPushName(ctx, jid, "Ana")
	if err != nil {
		t.Fatalf("PutPushName 2: %v", err)
	}
	if changed {
		t.Fatal("gravar o mesmo push name nao e' mudanca")
	}

	changed, previous, err = s.PutPushName(ctx, jid, "Ana Maria")
	if err != nil {
		t.Fatalf("PutPushName 3: %v", err)
	}
	if !changed || previous != "Ana" {
		t.Fatalf("esperava (true, \"Ana\"), veio (%v, %q)", changed, previous)
	}
}

func TestPutBusinessNameReportsChangeAndPreviousName(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	jid := contactJID("1")

	changed, _, err := s.PutBusinessName(ctx, jid, "Loja")
	if err != nil {
		t.Fatalf("PutBusinessName 1: %v", err)
	}
	if !changed {
		t.Fatal("o primeiro business name deveria contar como mudanca")
	}

	changed, previous, err := s.PutBusinessName(ctx, jid, "Loja Nova")
	if err != nil {
		t.Fatalf("PutBusinessName 2: %v", err)
	}
	if !changed || previous != "Loja" {
		t.Fatalf("esperava (true, \"Loja\"), veio (%v, %q)", changed, previous)
	}
}

// Os quatro nomes vivem em colunas distintas da MESMA linha: gravar um nao pode
// zerar os outros.
func TestContactNameFieldsCoexist(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	jid := contactJID("1")
	if err := s.PutContactName(ctx, jid, "Ana", "Ana Silva"); err != nil {
		t.Fatalf("PutContactName: %v", err)
	}
	if _, _, err := s.PutPushName(ctx, jid, "Aninha"); err != nil {
		t.Fatalf("PutPushName: %v", err)
	}
	if _, _, err := s.PutBusinessName(ctx, jid, "Loja da Ana"); err != nil {
		t.Fatalf("PutBusinessName: %v", err)
	}
	info, err := s.GetContact(ctx, jid)
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	if info.FirstName != "Ana" || info.FullName != "Ana Silva" ||
		info.PushName != "Aninha" || info.BusinessName != "Loja da Ana" {
		t.Fatalf("os campos se sobrescreveram: %+v", info)
	}
}

func TestPutAllContactNamesEmptyIsNoOp(t *testing.T) {
	if err := newTestStore(t).PutAllContactNames(context.Background(), nil); err != nil {
		t.Fatalf("PutAllContactNames vazio: %v", err)
	}
}

func TestPutAllContactNamesAndGetAllContacts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	entries := []store.ContactEntry{
		{JID: contactJID("1"), FirstName: "Ana", FullName: "Ana Silva"},
		{JID: contactJID("2"), FirstName: "Bruno", FullName: "Bruno Costa"},
	}
	if err := s.PutAllContactNames(ctx, entries); err != nil {
		t.Fatalf("PutAllContactNames: %v", err)
	}
	all, err := s.GetAllContacts(ctx)
	if err != nil {
		t.Fatalf("GetAllContacts: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("esperava 2 contatos, veio %d", len(all))
	}
	if all[contactJID("1")].FullName != "Ana Silva" || all[contactJID("2")].FirstName != "Bruno" {
		t.Fatalf("GetAllContacts = %+v", all)
	}
	for jid, info := range all {
		if !info.Found {
			t.Fatalf("%s deveria vir com Found=true", jid)
		}
	}
}

// Um lote maior que contactBatchSize e' quebrado em varios INSERTs dentro da
// mesma transacao — todos precisam ser gravados.
func TestPutAllContactNamesChunksLargeBatches(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	total := contactBatchSize + 10
	entries := make([]store.ContactEntry, total)
	for i := range entries {
		entries[i] = store.ContactEntry{
			JID:      contactJID(fmt.Sprintf("55119%08d", i)),
			FullName: fmt.Sprintf("Contato %d", i),
		}
	}
	if err := s.PutAllContactNames(ctx, entries); err != nil {
		t.Fatalf("PutAllContactNames: %v", err)
	}
	all, err := s.GetAllContacts(ctx)
	if err != nil {
		t.Fatalf("GetAllContacts: %v", err)
	}
	if len(all) != total {
		t.Fatalf("esperava %d contatos, veio %d", total, len(all))
	}
}

// Duplicatas no mesmo lote quebrariam o ON CONFLICT ("row cannot be updated
// twice"), entao exslices.DeduplicateUnsortedOverwriteFunc as remove antes do
// INSERT. Qual ocorrencia sobrevive e' detalhe da dependencia; o teste trava o
// que importa (o lote passa sem erro e a linha final e' uma so') e registra a
// observada hoje: a PRIMEIRA.
func TestPutAllContactNamesDeduplicates(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	jid := contactJID("1")
	err := s.PutAllContactNames(ctx, []store.ContactEntry{
		{JID: jid, FullName: "Primeiro"},
		{JID: jid, FullName: "Ultimo"},
	})
	if err != nil {
		t.Fatalf("PutAllContactNames: %v", err)
	}
	info, err := s.GetContact(ctx, jid)
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	if info.FullName != "Primeiro" && info.FullName != "Ultimo" {
		t.Fatalf("esperava uma das duas ocorrencias, veio %q", info.FullName)
	}
	all, err := s.GetAllContacts(ctx)
	if err != nil {
		t.Fatalf("GetAllContacts: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("as duas entradas duplicadas viraram %d linhas", len(all))
	}
}

// PutAllContactNames limpa o cache inteiro (o comentario no codigo diz que
// buscar push/business names de volta seria caro demais). Provamos que uma
// leitura anterior nao fica servindo dado velho.
func TestPutAllContactNamesInvalidatesCache(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	jid := contactJID("1")
	if err := s.PutContactName(ctx, jid, "Antigo", "Nome Antigo"); err != nil {
		t.Fatalf("PutContactName: %v", err)
	}
	if _, err := s.GetContact(ctx, jid); err != nil { // popula o cache
		t.Fatalf("GetContact: %v", err)
	}
	err := s.PutAllContactNames(ctx, []store.ContactEntry{
		{JID: jid, FirstName: "Novo", FullName: "Nome Novo"},
	})
	if err != nil {
		t.Fatalf("PutAllContactNames: %v", err)
	}
	info, err := s.GetContact(ctx, jid)
	if err != nil {
		t.Fatalf("GetContact depois: %v", err)
	}
	if info.FullName != "Nome Novo" {
		t.Fatalf("o cache serviu dado velho: %q", info.FullName)
	}
}

func TestPutManyRedactedPhonesEmptyIsNoOp(t *testing.T) {
	if err := newTestStore(t).PutManyRedactedPhones(context.Background(), nil); err != nil {
		t.Fatalf("PutManyRedactedPhones vazio: %v", err)
	}
}

func TestPutManyRedactedPhonesRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	jid := contactJID("1")
	err := s.PutManyRedactedPhones(ctx, []store.RedactedPhoneEntry{
		{JID: jid, RedactedPhone: "+55 11 ****-9999"},
	})
	if err != nil {
		t.Fatalf("PutManyRedactedPhones: %v", err)
	}
	info, err := s.GetContact(ctx, jid)
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	if info.RedactedPhone != "+55 11 ****-9999" {
		t.Fatalf("RedactedPhone = %q", info.RedactedPhone)
	}
}

// Ao contrario de PutAllContactNames, aqui o cache e' invalidado
// SELETIVAMENTE: so' as entradas cujo telefone redigido mudou saem do cache.
func TestPutManyRedactedPhonesInvalidatesOnlyChangedEntries(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	inalterado, alterado := contactJID("1"), contactJID("2")

	err := s.PutManyRedactedPhones(ctx, []store.RedactedPhoneEntry{
		{JID: inalterado, RedactedPhone: "A"},
		{JID: alterado, RedactedPhone: "B"},
	})
	if err != nil {
		t.Fatalf("PutManyRedactedPhones 1: %v", err)
	}
	// Popula o cache das duas entradas.
	for _, jid := range []types.JID{inalterado, alterado} {
		if _, gErr := s.GetContact(ctx, jid); gErr != nil {
			t.Fatalf("GetContact %s: %v", jid, gErr)
		}
	}

	err = s.PutManyRedactedPhones(ctx, []store.RedactedPhoneEntry{
		{JID: inalterado, RedactedPhone: "A"},
		{JID: alterado, RedactedPhone: "B2"},
	})
	if err != nil {
		t.Fatalf("PutManyRedactedPhones 2: %v", err)
	}

	info, err := s.GetContact(ctx, alterado)
	if err != nil {
		t.Fatalf("GetContact alterado: %v", err)
	}
	if info.RedactedPhone != "B2" {
		t.Fatalf("a entrada alterada deveria ter saido do cache, veio %q", info.RedactedPhone)
	}
	info, err = s.GetContact(ctx, inalterado)
	if err != nil {
		t.Fatalf("GetContact inalterado: %v", err)
	}
	if info.RedactedPhone != "A" {
		t.Fatalf("a entrada inalterada mudou: %q", info.RedactedPhone)
	}
}

func TestGetAllContactsPopulatesCache(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	jid := contactJID("1")
	if err := s.PutContactName(ctx, jid, "Ana", "Ana Silva"); err != nil {
		t.Fatalf("PutContactName: %v", err)
	}
	if _, err := s.GetAllContacts(ctx); err != nil {
		t.Fatalf("GetAllContacts: %v", err)
	}
	s.contactCacheLock.Lock()
	_, cached := s.contactCache[jid]
	s.contactCacheLock.Unlock()
	if !cached {
		t.Fatal("GetAllContacts deveria ter populado o cache")
	}
}

func TestGetContactCachesResult(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	jid := contactJID("1")
	if _, err := s.GetContact(ctx, jid); err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	s.contactCacheLock.Lock()
	_, cached := s.contactCache[jid]
	s.contactCacheLock.Unlock()
	if !cached {
		t.Fatal("mesmo um contato ausente deve ser cacheado (evita reconsultar a cada mensagem)")
	}
}

func TestContactsAreScopedPerOurJID(t *testing.T) {
	ctx := context.Background()
	container, _, s := newTestDevice(t)
	if err := s.PutContactName(ctx, contactJID("1"), "Ana", "Ana Silva"); err != nil {
		t.Fatalf("PutContactName: %v", err)
	}
	other := NewSQLStore(container, mustJID(t, "5511888888888", "s.whatsapp.net"))
	info, err := other.GetContact(ctx, contactJID("1"))
	if err != nil {
		t.Fatalf("GetContact outro our_jid: %v", err)
	}
	if info.Found {
		t.Fatalf("o contato de um our_jid vazou para outro: %+v", info)
	}
}
