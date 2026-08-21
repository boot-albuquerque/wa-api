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
