package main

import (
	"os"
	"path/filepath"
	"testing"
)

// HOUSEKEEP F189 — o orcamento de isencoes contava MENCOES, nao anotacoes.
//
// Estes testes travam as duas metades do defeito, e sao dois porque as duas
// falham de formas opostas: uma faz o gate disparar sem motivo, a outra faz o
// gate NAO disparar quando devia. Um teste so' apanharia uma.

func escreverPacote(t *testing.T, arquivos map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for nome, conteudo := range arquivos {
		if err := os.WriteFile(filepath.Join(dir, nome), []byte(conteudo), 0o600); err != nil {
			t.Fatalf("escrever %s: %v", nome, err)
		}
	}
	return dir
}

// TestContagemDeIsencoes_MencaoEmProsaNaoConta e' o defeito medido: um comentario
// que EXPLICA a anotacao, sem a usar, somava ao orcamento.
//
// O caso e' o real, nao inventado: foi escrevendo uma frase a dizer por que NAO
// usei a isencao que eu derrubei o gate em 2026-08-20.
func TestContagemDeIsencoes_MencaoEmProsaNaoConta(t *testing.T) {
	dir := escreverPacote(t, map[string]string{
		"a.go": `package a

// F faz alguma coisa.
//
// Nao usei a anotacao //log:exempt aqui de proposito: preferi repetir o getter
// a pendurar uma excecao permanente no ficheiro.
func F() error { return nil }
`,
	})

	n, err := countExemptAnnotations(dir)
	if err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 0 {
		t.Fatalf("contagem = %d, quero 0: mencionar a anotacao em prosa NAO isenta funcao nenhuma, "+
			"e fazer o orcamento disparar por documentacao desencoraja explicar o mecanismo (F189)", n)
	}
}

// TestContagemDeIsencoes_AnotacaoDeVerdadeConta e' o controle na direcao oposta.
//
// Sem ele, um contador que devolvesse sempre zero passaria no teste acima — e um
// orcamento que nunca dispara e' pior que um que dispara de mais, porque parece
// que esta' a proteger.
func TestContagemDeIsencoes_AnotacaoDeVerdadeConta(t *testing.T) {
	dir := escreverPacote(t, map[string]string{
		"a.go": `package a

//log:exempt motivo escrito aqui
func F() error { return nil }

// G nao esta' isenta.
func G() error { return nil }
`,
	})

	n, err := countExemptAnnotations(dir)
	if err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 1 {
		t.Fatalf("contagem = %d, quero 1: a anotacao REAL, no bloco de doc da declaracao, tem de contar", n)
	}
}

// TestContagemDeIsencoes_IgnoraArquivosDeTeste trava a segunda metade da F189: o
// comentario da funcao dizia "ignorando arquivos de teste" e o codigo NAO os
// ignorava — o filtro era so' `.go`. Um //log:exempt num teste entrava na conta
// de producao.
func TestContagemDeIsencoes_IgnoraArquivosDeTeste(t *testing.T) {
	dir := escreverPacote(t, map[string]string{
		"a_test.go": `package a

//log:exempt isento num teste, nao devia contar para producao
func F() error { return nil }
`,
	})

	n, err := countExemptAnnotations(dir)
	if err != nil {
		t.Fatalf("contar: %v", err)
	}
	if n != 0 {
		t.Fatalf("contagem = %d, quero 0: isencao em ficheiro de teste nao e' divida de producao", n)
	}
}

// TestContagemDeIsencoes_ArquivoQueNaoParseiaFalhaAlto: um ficheiro invalido nao
// pode virar "zero isencoes" em silencio, que seria o gate a proteger menos
// precisamente quando o repositorio esta' partido.
func TestContagemDeIsencoes_ArquivoQueNaoParseiaFalhaAlto(t *testing.T) {
	dir := escreverPacote(t, map[string]string{"a.go": "package a\nfunc F( {"})

	if _, err := countExemptAnnotations(dir); err == nil {
		t.Fatal("ficheiro que nao parseia devolveu erro nil: o gate passaria a contar zero com o repo partido")
	}
}
