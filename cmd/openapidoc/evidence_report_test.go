package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildTabelaCompleta_OrdenaPeloCaminhoCru trava a causa, não o sintoma,
// de um defeito medido em 2026-08-28: a primeira versão ordenava `lines`
// DEPOIS de já ter embrulhado `caminho`/`metodo` em crases markdown
// ("`/admin/users/{id}`"). O caracter de fecho da crase (0x60) ordena DEPOIS
// de "/" (0x2F), então "/admin/users/{id}`" comparava como MAIOR que
// "/admin/users/{id}/full`" — o prefixo mais curto, que devia vir primeiro,
// saía depois do mais longo. Medido: a tabela gerada listava
// `/admin/users/{id}/full` antes de `/admin/users/{id}`.
func TestBuildTabelaCompleta_OrdenaPeloCaminhoCru(t *testing.T) {
	rows := []evidenceRow{
		{metodo: "DELETE", caminho: "/admin/users/{id}/full", marca: "✅"},
		{metodo: "DELETE", caminho: "/admin/users/{id}", marca: "✅"},
		{metodo: "GET", caminho: "/admin/users/{id}", marca: "✅"},
		{metodo: "GET", caminho: "/admin/users", marca: "✅"},
	}
	ops := map[string]reportOperation{
		"DELETE /admin/users/{id}/full": {grupo: "Administração", titulo: "full"},
		"DELETE /admin/users/{id}":      {grupo: "Administração", titulo: "delete"},
		"GET /admin/users/{id}":         {grupo: "Administração", titulo: "get-one"},
		"GET /admin/users":              {grupo: "Administração", titulo: "list"},
	}

	got, err := buildTabelaCompleta(rows, ops, map[string]string{})
	if err != nil {
		t.Fatalf("buildTabelaCompleta: %v", err)
	}

	idxList := strings.Index(got, "`/admin/users`")
	idxGetOne := strings.Index(got, "`/admin/users/{id}`")
	idxDelete := strings.Index(got, "| `DELETE` | `/admin/users/{id}` |")
	idxFull := strings.Index(got, "`/admin/users/{id}/full`")
	for nome, idx := range map[string]int{"list": idxList, "get-one": idxGetOne, "delete": idxDelete, "full": idxFull} {
		if idx < 0 {
			t.Fatalf("linha %q não encontrada na tabela gerada:\n%s", nome, got)
		}
	}
	if !(idxList < idxDelete && idxDelete < idxFull) {
		t.Fatalf("ordem errada — queria /admin/users < /admin/users/{id} < /admin/users/{id}/full, saiu:\n%s", got)
	}
}

// TestReadEvidenceRows_ColunasFinaisVaziasSaoPreservadas trava a segunda
// causa medida em 2026-08-28: TrimSpace, aplicado à linha ANTES do
// strings.Split(line, "\t"), trata tabulação como espaço e come colunas
// finais vazias. Uma linha "GET\t/x\t✅\t\t\t" (data/observador/evidência
// vazios, o caso comum enquanto a campanha F239/F282 não remediu a rota)
// virava "GET\t/x\t✅" e o Split devolvia 3 campos em vez de 6.
func TestReadEvidenceRows_ColunasFinaisVaziasSaoPreservadas(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evidencias.tsv")
	conteudo := "GET\t/x\t✅\t\t\t\n"
	if err := os.WriteFile(path, []byte(conteudo), 0o644); err != nil {
		t.Fatal(err)
	}

	rows, err := readEvidenceRows(path)
	if err != nil {
		t.Fatalf("readEvidenceRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, queria 1", len(rows))
	}
	if rows[0].marca != "✅" || rows[0].data != "" || rows[0].observador != "" || rows[0].evidencia != "" {
		t.Errorf("row = %+v, queria marca=✅ e os três campos finais vazios (não ausentes)", rows[0])
	}
}
