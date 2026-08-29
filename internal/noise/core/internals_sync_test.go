package core

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// O F29 registrou que nenhum gate detectava internals.go desatualizado:
// `go generate` não roda em `make check`, e não havia teste comparando o
// arquivo commitado com o que o gerador produziria. Foi assim que ele ficou
// dessincronizado da árvore sem ninguém perceber.
//
// Este teste fecha a lacuna sem rodar o gerador (que escreveria no diretório
// do pacote durante o teste). Ele afere a INVARIANTE que o gerador existe para
// manter: todo método não exportado de *Client com receptor `cli` tem um
// wrapper correspondente em DangerousInternalClient.
//
// Se alguém acrescentar um método e esquecer o `go generate`, este teste
// aponta exatamente qual ficou de fora.

// eligibleMethods replica o critério de seleção de internals_generate.go:
// FuncDecl com receptor nomeado `cli` e nome não exportado.
func eligibleMethods(t *testing.T) []string {
	t.Helper()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	fset := token.NewFileSet()
	var out []string
	for _, name := range names {
		if name == "internals.go" || name == "internals_generate.go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(f, func(node ast.Node) bool {
			fn, ok := node.(*ast.FuncDecl)
			if !ok || fn.Name.IsExported() {
				return true
			}
			if fn.Recv == nil || len(fn.Recv.List) == 0 || len(fn.Recv.List[0].Names) == 0 ||
				fn.Recv.List[0].Names[0].Name != "cli" {
				return true
			}
			out = append(out, fn.Name.Name)
			return true
		})
	}
	return out
}

// generatedWrappers lê os nomes de método já expostos em internals.go.
func generatedWrappers(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile("internals.go")
	if err != nil {
		t.Fatalf("ler internals.go: %v", err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "internals.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse internals.go: %v", err)
	}
	out := map[string]bool{}
	ast.Inspect(f, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			return true
		}
		star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			return true
		}
		ident, ok := star.X.(*ast.Ident)
		if !ok || ident.Name != "DangerousInternalClient" {
			return true
		}
		out[fn.Name.Name] = true
		return true
	})
	return out
}

// exported devolve o nome do wrapper que o gerador criaria para um método.
func exported(name string) string {
	return strings.ToUpper(name[0:1]) + name[1:]
}

func TestInternalsGeradoCobreTodoMetodoElegivel(t *testing.T) {
	wrappers := generatedWrappers(t)
	if len(wrappers) == 0 {
		t.Fatal("nenhum wrapper encontrado em internals.go — o parse falhou silenciosamente")
	}

	var faltando []string
	for _, method := range eligibleMethods(t) {
		if !wrappers[exported(method)] {
			faltando = append(faltando, method)
		}
	}
	if len(faltando) > 0 {
		t.Errorf("%d metodo(s) elegivel(is) sem wrapper em internals.go: %v\n"+
			"Rode `go generate ./internal/wa-noise/core/` e commite o resultado.\n"+
			"Ver F29 em HOUSEKEEP.md para o historico deste gate.",
			len(faltando), faltando)
	}
}

// A contrapartida: um wrapper que sobrou depois de o método ser removido ou
// renomeado também é sinal de internals.go desatualizado — e é a forma mais
// provável de o arquivo quebrar o build, já que o wrapper chama um método que
// não existe mais.
func TestInternalsGeradoNaoTemWrapperOrfao(t *testing.T) {
	elegiveis := map[string]bool{}
	for _, method := range eligibleMethods(t) {
		elegiveis[exported(method)] = true
	}

	var orfaos []string
	for wrapper := range generatedWrappers(t) {
		if !elegiveis[wrapper] {
			orfaos = append(orfaos, wrapper)
		}
	}
	if len(orfaos) > 0 {
		t.Errorf("%d wrapper(s) em internals.go sem metodo correspondente: %v\n"+
			"Rode `go generate ./internal/wa-noise/core/` e commite o resultado.",
			len(orfaos), orfaos)
	}
}
