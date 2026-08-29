package client

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestTodoMetodoComErroTemWrapper trava o custo que a decisão 46=a assumiu.
//
// `RealClient` embute `*noise.Client`, então um método SEM wrapper continua a
// compilar, a satisfazer a interface e a passar em todos os outros testes — só
// que devolve o erro cru do SDK, e a recusa do servidor volta a chegar ao
// cliente como 500 (F204). É exatamente a falha silenciosa que a alternativa
// (b) do canal tinha, e que a (a) só evita se alguém a travar.
//
// Este teste é esse gate. Sem ele, um método novo na interface entra sem
// wrapper e o defeito reaparece numa rota só — o mais difícil de notar.
func TestTodoMetodoComErroTemWrapper(t *testing.T) {
	naInterface := map[string]bool{}
	comWrapper := map[string]bool{}

	// `parser.ParseFile` sobre os .go do diretório, e não `ParseDir`: esta está
	// deprecada desde Go 1.25 por não considerar build tags. Aqui isso seria
	// inócuo — não há build tags neste pacote —, mas a alternativa é uma linha.
	nomes, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("não consegui listar o pacote: %v", err)
	}
	fset := token.NewFileSet()
	for _, nome := range nomes {
		f, err := parser.ParseFile(fset, nome, nil, 0)
		if err != nil {
			t.Fatalf("não consegui ler %s: %v", nome, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			colher(n, naInterface, comWrapper)
			return true
		})
	}

	if len(naInterface) == 0 {
		t.Fatal("não encontrei métodos com erro na interface Client: o teste não está a medir nada")
	}

	var faltam []string
	for nome := range naInterface {
		if !comWrapper[nome] {
			faltam = append(faltam, nome)
		}
	}
	if len(faltam) > 0 {
		t.Errorf("%d método(s) da interface Client devolvem erro e NÃO têm wrapper que "+
			"traduza no RealClient: %s.\n\nSem wrapper, o método é PROMOVIDO de "+
			"*noise.Client e devolve o erro cru do SDK; com wrapper que não chama "+
			"errmap.ClassifyIQ, o mesmo, com uma camada a mais. Nos dois casos a recusa "+
			"do servidor do WhatsApp volta a chegar ao cliente como 500 nessa rota "+
			"(F204), e nada mais no repositório acusa.",
			len(faltam), strings.Join(faltam, ", "))
	}
}

// chamaClassifyIQ procura a tradução no corpo do wrapper. É a asserção que
// distingue "existe um método" de "o método faz a coisa".
func chamaClassifyIQ(fd *ast.FuncDecl) bool {
	achou := false
	ast.Inspect(fd, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "ClassifyIQ" {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == "errmap" {
			achou = true
		}
		return true
	})
	return achou
}

// colher separa o que é declaração da interface do que é wrapper. Está fora do
// teste porque o `gocyclo` do gate conta os ramos do switch como complexidade
// do caso de teste, e um teste que reprova o lint por ler AST é ruído.
func colher(n ast.Node, naInterface, comWrapper map[string]bool) {
	switch v := n.(type) {
	case *ast.TypeSpec:
		colherInterface(v, naInterface)
	case *ast.FuncDecl:
		// Existir não basta: o wrapper tem de TRADUZIR. A primeira versão deste
		// teste só registava a presença, e o controlo negativo que trocava
		// `errmap.ClassifyIQ(err)` por `err` passou verde — um wrapper que
		// delega sem traduzir é exatamente o defeito da F204, com uma camada a
		// mais.
		if v.Recv != nil && len(v.Recv.List) > 0 &&
			receptorEhRealClient(v.Recv.List[0].Type) && chamaClassifyIQ(v) {
			comWrapper[v.Name.Name] = true
		}
	}
}

func colherInterface(ts *ast.TypeSpec, naInterface map[string]bool) {
	it, ok := ts.Type.(*ast.InterfaceType)
	if !ok || ts.Name.Name != "Client" {
		return
	}
	for _, m := range it.Methods.List {
		ft, ok := m.Type.(*ast.FuncType)
		if !ok || len(m.Names) == 0 || !devolveErro(ft) {
			continue
		}
		naInterface[m.Names[0].Name] = true
	}
}

func devolveErro(ft *ast.FuncType) bool {
	if ft.Results == nil {
		return false
	}
	for _, r := range ft.Results.List {
		if id, ok := r.Type.(*ast.Ident); ok && id.Name == "error" {
			return true
		}
	}
	return false
}

func receptorEhRealClient(expr ast.Expr) bool {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == "RealClient"
}
