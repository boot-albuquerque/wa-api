package db_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"wa-api/pkg/infra/db"
)

// Os ramos de ERRO deste repositório nunca eram exercitados: os testes de
// handler usam um banco saudável, e um caminho de falha que ninguém percorre
// é um caminho que MAY estar errado — inclusive no log, que é a única coisa
// que o operador vê quando a escrita falha.
//
// A falha é provocada APAGANDO a tabela, e não com um dublê: um dublê de
// sqlx.DB imitaria a minha ideia do que o driver faz, e a armadilha nº1 do
// ARMADILHAS.md é exatamente isso. Tabela ausente é falha REAL do driver.

func labelDBSemTabelas(t *testing.T) *sqlx.DB {
	t.Helper()
	database, err := sqlx.Open("sqlite", t.TempDir()+"/vazio.db"+db.SQLitePragmas)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.InitializeSchema(database); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	for _, tbl := range []string{"wa_label_messages", "wa_label_chats", "wa_labels"} {
		if _, err := database.Exec("DROP TABLE IF EXISTS " + tbl); err != nil {
			t.Fatalf("drop %s: %v", tbl, err)
		}
	}
	return database
}

func TestLabelRepository_FalhaDeEscritaPropaga(t *testing.T) {
	repo := db.NewLabelRepository(labelDBSemTabelas(t))
	ctx := context.Background()
	agora := time.Now().UTC()

	casos := map[string]func() error{
		"UpsertLabel": func() error {
			return repo.UpsertLabel(ctx, "u1", db.Label{LabelID: "L1", UpdatedAt: agora})
		},
		"SetChatLabel": func() error {
			return repo.SetChatLabel(ctx, "u1", db.LabelChat{LabelID: "L1", ChatJID: "c", UpdatedAt: agora})
		},
		"SetMessageLabel": func() error {
			return repo.SetMessageLabel(ctx, "u1", "L1", "c", "m", true, agora)
		},
	}
	for nome, chamar := range casos {
		t.Run(nome, func(t *testing.T) {
			err := chamar()
			if err == nil {
				t.Fatalf("%s engoliu a falha de escrita: o chamador acharia que gravou", nome)
			}
			// A mensagem tem de nomear a tabela — é o que o operador procura
			// no log quando a escrita falha.
			if !strings.Contains(err.Error(), "wa_label") {
				t.Errorf("erro %q não nomeia a tabela", err)
			}
		})
	}
}

func TestLabelRepository_FalhaDeLeituraPropagaENaoDevolveListaVazia(t *testing.T) {
	repo := db.NewLabelRepository(labelDBSemTabelas(t))
	ctx := context.Background()

	// Devolver lista vazia numa falha de leitura seria pior que o erro: o
	// cliente concluiria "este utilizador não tem etiquetas" a partir de uma
	// avaria — e pararia de tentar.
	rotulos, err := repo.ListLabels(ctx, "u1")
	if err == nil {
		t.Fatal("ListLabels engoliu a falha: o cliente leria ausência onde há avaria")
	}
	if rotulos != nil {
		t.Fatalf("ListLabels devolveu %v além do erro", rotulos)
	}

	conversas, err := repo.ListChatsForLabel(ctx, "u1", "L1")
	if err == nil {
		t.Fatal("ListChatsForLabel engoliu a falha")
	}
	if conversas != nil {
		t.Fatalf("ListChatsForLabel devolveu %v além do erro", conversas)
	}
}

// --- F205: o comportamento CONHECIDO, travado para não ser redescoberto ------
//
// As três escritas sobrepõem por ORDEM DE CHEGADA: `updated_at` é gravado e
// nunca comparado. Isto NÃO é descuido — é o comportamento medido e decidido
// (decisão 39=c do canal), e este teste existe para que ninguém o encontre
// como surpresa daqui a seis meses.
//
// POR QUE NÃO COMPARAMOS, com a medição que sustenta a decisão:
//
//  1. O Baileys faz o mesmo. `processSyncAction` em Utils/chat-utils.ts trata
//     labelEditAction e labelAssociationAction emitindo o evento, sem
//     comparação de timestamp; a única lógica temporal é `messageRange` em
//     archive/unarchive, que não se aplica a etiquetas.
//
//  2. A biblioteca já descarta snapshot mais velho:
//     appstatesync/recovery.go:47 — `currentVersion >= version` → "Ignoring
//     app state recovery response ... as current version is newer".
//
//  3. Os patches de app-state têm versão monotónica e LTHash verificado
//     (protocol/appstate/decode.go): aplicar fora de ordem não passa em
//     silêncio, aborta com ErrMismatchingLTHash.
//
// O que NÃO foi medido, e por isso a F205 continua ABERTA em vez de refutada:
// não se reproduziu um FullSync tardio contra o servidor real — provocar
// reordenação de patches não está ao alcance da API. Ausência de prova não é
// prova de ausência.
//
// SE ALGUÉM VIER MUDAR ISTO: comparar `updated_at` no DO UPDATE cria um
// caminho de DESCARTE SILENCIOSO, e um descarte sem registo é o defeito da
// F184 noutra família. Acrescente o log junto com a comparação.
func TestLabelRepository_UltimaEscritaVenceIndependenteDoTimestamp(t *testing.T) {
	repo := db.NewLabelRepository(labelDBComTabelas(t))
	ctx := context.Background()
	novo := time.Now().UTC().Truncate(time.Second)
	velho := novo.Add(-1 * time.Hour)

	// Chega a mais NOVA primeiro, depois a mais VELHA. Se algum dia passarmos
	// a comparar timestamp, "Recente" sobrevive e este teste falha — que é
	// exatamente o sinal que se quer.
	if err := repo.UpsertLabel(ctx, "u1", db.Label{LabelID: "L1", Name: "Recente", UpdatedAt: novo}); err != nil {
		t.Fatalf("UpsertLabel novo: %v", err)
	}
	if err := repo.UpsertLabel(ctx, "u1", db.Label{LabelID: "L1", Name: "Antigo", UpdatedAt: velho}); err != nil {
		t.Fatalf("UpsertLabel velho: %v", err)
	}

	rotulos, err := repo.ListLabels(ctx, "u1")
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(rotulos) != 1 {
		t.Fatalf("etiquetas = %d, quero 1", len(rotulos))
	}
	if rotulos[0].Name != "Antigo" {
		t.Fatalf("nome = %q, quero \"Antigo\".\n"+
			"A última escrita deixou de vencer — alguém passou a comparar timestamp.\n"+
			"Se foi intencional, ATUALIZE a F205 no HOUSEKEEP e confirme que o\n"+
			"descarte da escrita mais velha DEIXA REGISTO: descarte silencioso é\n"+
			"o defeito da F184 noutra família.", rotulos[0].Name)
	}
}

// labelDBComTabelas é o banco COM as tabelas — o oposto de labelDBSemTabelas,
// que existe para exercitar os ramos de erro.
func labelDBComTabelas(t *testing.T) *sqlx.DB {
	t.Helper()
	database, err := sqlx.Open("sqlite", t.TempDir()+"/labels.db"+db.SQLitePragmas)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.InitializeSchema(database); err != nil {
		t.Fatalf("InitializeSchema: %v", err)
	}
	return database
}
