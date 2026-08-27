package handlers

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// O GATE ARQUITECTURAL da migração para DTO.
//
// # O que ele afirma
//
// Cada chamada a RespondJSON neste pacote está inscrita num LIVRO-RAZÃO
// (testdata/respondjson_ledger.tsv) com a expressão que ela serializa e a
// classificação dessa expressão: `dto` (já passa por apresentador), `nil`
// (não serializa nada — ramo de erro), ou `pendente` (ainda entrega um valor
// de domínio ou de protocolo directamente ao codificador).
//
// Duas coisas fazem o teste falhar:
//
//  1. o livro-razão não descreve mais o código — alguém acrescentou, removeu
//     ou mudou uma chamada sem o actualizar;
//  2. o número de `pendente` SUBIU. Esta é a catraca: as seis famílias que vão
//     migrar só podem baixar esse número.
//
// # O compromisso escolhido, e o que ele NÃO apanha
//
// Um gate preciso exigiria go/types sobre a árvore inteira para saber o TIPO
// ESTÁTICO do terceiro argumento, e distinguir `rsp` (domain) de `rsp` (dto)
// pelo nome não é possível. Medido: 404 sítios de chamada, e resolver tipos em
// apresentadas), e resolver tipos em todos custa packages.Load dentro do gate.
//
// A aproximação por AST classifica pela FORMA da expressão. Ela NÃO apanha um
// apresentador que devolva o tipo errado, nem uma variável chamada `dtoAlgo`
// que não seja DTO nenhum. O que ela apanha, e que é o modo de falha real
// desta migração, é a chamada NOVA que ninguém classificou: a que aparece num
// handler acrescentado depois e serializa `rsp` do use case porque foi copiada
// do vizinho. Essa falha aqui, com o nome do ficheiro e da função.
//
// O que fica coberto pelo outro lado é o VALOR: o teste de contrato por rota
// (handler_group_info_contract_test.go) afirma os nomes das chaves servidas, e
// nenhum tipo errado sobrevive a ele.

// maxPendingRespondJSONSites é a CATRACA. Baixe-o quando uma família migrar;
// nunca o suba.
//
//	108 — 2026-08-27, fundação da migração DTO (/group/info)
//	 81 — família sessão + configuração de webhook + configuração de armazenamento
//	 69 — integração com a família de utilizadores, contactos e blocklist
//	 67 — integração com a família de canais/newsletters
const maxPendingRespondJSONSites = 67

// updateLedger reescreve o livro-razão em vez de o comparar.
var updateLedger = flag.Bool("update-ledger", false,
	"reescreve testdata/respondjson_ledger.tsv a partir do código actual")

const ledgerPath = "testdata/respondjson_ledger.tsv"

// classificacaoDTO, classificacaoNil e classificacaoPendente são os três
// veredictos possíveis sobre o que uma chamada serializa.
const (
	classificacaoDTO      = "dto"
	classificacaoNil      = "nil"
	classificacaoPendente = "pendente"
)

// entradaLivro é uma linha do livro-razão.
type entradaLivro struct {
	Ficheiro      string
	Funcao        string
	Expressao     string
	Classificacao string
	Vezes         int
}

func (e entradaLivro) linha() string {
	return strings.Join([]string{e.Ficheiro, e.Funcao, e.Expressao, e.Classificacao, fmt.Sprint(e.Vezes)}, "\t")
}

func TestRespondJSONLedger(t *testing.T) {
	atual := recolheChamadasRespondJSON(t)

	if *updateLedger {
		escreveLivro(t, atual)
		t.Logf("livro-razão reescrito com %d entradas", len(atual))
		return
	}

	esperado := leLivro(t)
	compara(t, esperado, atual)

	pendentes := 0
	for _, e := range atual {
		if e.Classificacao == classificacaoPendente {
			pendentes += e.Vezes
		}
	}
	if pendentes > maxPendingRespondJSONSites {
		t.Errorf("%d sítios RespondJSON ainda entregam valor não-DTO ao codificador, "+
			"acima da catraca de %d.\n"+
			"Uma chamada NOVA que serializa domínio directamente é exactamente o que esta "+
			"migração existe para impedir: apresente o valor em "+
			"pkg/presentation/http/dto/<família> antes de o devolver.",
			pendentes, maxPendingRespondJSONSites)
	}
	t.Logf("sítios RespondJSON: %d entradas, %d pendentes (catraca %d)",
		len(atual), pendentes, maxPendingRespondJSONSites)
}

// recolheChamadasRespondJSON lê os ficheiros não-teste do pacote por AST.
func recolheChamadasRespondJSON(t *testing.T) []entradaLivro {
	t.Helper()

	nomes, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	sort.Strings(nomes)

	contagem := map[entradaLivro]int{}
	fset := token.NewFileSet()
	for _, nome := range nomes {
		if strings.HasSuffix(nome, "_test.go") {
			continue
		}
		ficheiro, err := parser.ParseFile(fset, nome, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", nome, err)
		}
		for _, decl := range ficheiro.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn.Body, func(no ast.Node) bool {
				chamada, ok := no.(*ast.CallExpr)
				if !ok || !ehRespondJSON(chamada.Fun) || len(chamada.Args) < 3 {
					return true
				}
				expr := renderExpr(chamada.Args[2])
				chave := entradaLivro{
					Ficheiro:      nome,
					Funcao:        nomeDaFuncao(fn),
					Expressao:     expr,
					Classificacao: classifica(expr),
				}
				contagem[chave]++
				return true
			})
		}
	}

	saida := make([]entradaLivro, 0, len(contagem))
	for chave, vezes := range contagem {
		chave.Vezes = vezes
		saida = append(saida, chave)
	}
	sort.Slice(saida, func(i, j int) bool { return saida[i].linha() < saida[j].linha() })
	return saida
}

// ehRespondJSON reconhece `customhttp.RespondJSON` e `RespondJSON`.
func ehRespondJSON(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		return f.Sel.Name == "RespondJSON"
	case *ast.Ident:
		return f.Name == "RespondJSON"
	}
	return false
}

// nomeDaFuncao devolve `Tipo.Metodo` ou `Funcao`.
func nomeDaFuncao(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return renderExpr(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

// renderExpr devolve uma forma textual estável da expressão. Não é código Go
// completo de propósito: o que interessa é a FORMA, e o corpo de um literal
// composto grande só faria o livro-razão churnar a cada edição.
func renderExpr(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return renderExpr(v.X) + "." + v.Sel.Name
	case *ast.StarExpr:
		return "*" + renderExpr(v.X)
	case *ast.UnaryExpr:
		return v.Op.String() + renderExpr(v.X)
	case *ast.CallExpr:
		return renderExpr(v.Fun) + "(…)"
	case *ast.IndexExpr:
		return renderExpr(v.X) + "[…]"
	case *ast.CompositeLit:
		if v.Type == nil {
			return "{…}"
		}
		return renderExpr(v.Type) + "{…}"
	case *ast.MapType:
		return "map[" + renderExpr(v.Key) + "]" + renderExpr(v.Value)
	case *ast.ArrayType:
		return "[]" + renderExpr(v.Elt)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.BasicLit:
		return v.Kind.String() + "-literal"
	}
	return "?"
}

// classifica dá o veredicto sobre uma expressão serializada.
func classifica(expr string) string {
	if expr == "nil" {
		return classificacaoNil
	}
	// Um apresentador vive num pacote importado com o prefixo `dto`, e a
	// convenção de docs/HTTP-DTO-CONVENTIONS.md exige-o: `dtogroup.Present…`,
	// `dtosession.Present…`. É por isso que o prefixo é verificável.
	if strings.HasPrefix(expr, "dto") {
		return classificacaoDTO
	}
	return classificacaoPendente
}

func escreveLivro(t *testing.T, entradas []entradaLivro) {
	t.Helper()
	var b strings.Builder
	b.WriteString("# GERADO: go test ./pkg/presentation/http/handlers/ -run TestRespondJSONLedger -update-ledger\n")
	b.WriteString("# ficheiro\tfuncao\texpressao_serializada\tclassificacao\tvezes\n")
	for _, e := range entradas {
		b.WriteString(e.linha())
		b.WriteString("\n")
	}
	if err := os.MkdirAll(filepath.Dir(ledgerPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(ledgerPath, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("escrever livro-razão: %v", err)
	}
}

func leLivro(t *testing.T) []entradaLivro {
	t.Helper()
	bruto, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatalf("livro-razão ausente (corra: go test ./pkg/presentation/http/handlers/ -run TestRespondJSONLedger -update-ledger): %v", err)
	}
	var saida []entradaLivro
	for _, linha := range strings.Split(string(bruto), "\n") {
		if linha == "" || strings.HasPrefix(linha, "#") {
			continue
		}
		campos := strings.Split(linha, "\t")
		if len(campos) != 5 {
			t.Fatalf("linha malformada no livro-razão (%d campos): %q", len(campos), linha)
		}
		var vezes int
		if _, err := fmt.Sscanf(campos[4], "%d", &vezes); err != nil {
			t.Fatalf("contagem inválida em %q: %v", linha, err)
		}
		saida = append(saida, entradaLivro{campos[0], campos[1], campos[2], campos[3], vezes})
	}
	return saida
}

func compara(t *testing.T, esperado, atual []entradaLivro) {
	t.Helper()
	indexa := func(entradas []entradaLivro) map[string]entradaLivro {
		m := make(map[string]entradaLivro, len(entradas))
		for _, e := range entradas {
			m[e.Ficheiro+"\t"+e.Funcao+"\t"+e.Expressao] = e
		}
		return m
	}
	mEsperado, mAtual := indexa(esperado), indexa(atual)

	var novas, sumidas, mudadas []string
	for chave, e := range mAtual {
		anterior, existe := mEsperado[chave]
		switch {
		case !existe:
			novas = append(novas, chave+"  ->  "+e.Classificacao)
		case anterior.Vezes != e.Vezes || anterior.Classificacao != e.Classificacao:
			mudadas = append(mudadas, fmt.Sprintf("%s: %s x%d -> %s x%d",
				chave, anterior.Classificacao, anterior.Vezes, e.Classificacao, e.Vezes))
		}
	}
	for chave := range mEsperado {
		if _, existe := mAtual[chave]; !existe {
			sumidas = append(sumidas, chave)
		}
	}
	sort.Strings(novas)
	sort.Strings(sumidas)
	sort.Strings(mudadas)

	relata := func(rotulo string, linhas []string) {
		if len(linhas) == 0 {
			return
		}
		t.Errorf("%d chamada(s) a RespondJSON %s:\n  %s\n\n"+
			"Se a mudança é intencional, actualize o livro-razão:\n"+
			"  go test ./pkg/presentation/http/handlers/ -run TestRespondJSONLedger -update-ledger",
			len(linhas), rotulo, strings.Join(linhas, "\n  "))
	}
	relata("NOVAS, ausentes do livro-razão", novas)
	relata("no livro-razão mas já não no código", sumidas)
	relata("com forma ou contagem diferente da registada", mudadas)
}
