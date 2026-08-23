package main

// Trava o requisito nº 1 da Fase 4C: nenhum modo que carrega a credencial pode
// desligar o Chromium por sinal.
//
// O defeito que este teste existe para impedir já aconteceu, e passou por uma
// fase inteira sem ser visto: a 4C mediu que SIGTERM corrompe o estado de
// sessão, escreveu o requisito no relatório, e o requisito ficou só dentro do
// experimento que o mediu. `waopen`, `waprep`, `wacap`, `wasession` e `watabs`
// seguiram em `gracefulStop`. O sintoma reapareceu na Fase 5: dois `waopen`
// deixaram os 3 arquivos `Singleton` no perfil, que é a assinatura de saída
// suja estabelecida pela 4C.
//
// Por isso o teste é ESTÁTICO e não comportamental. O defeito não é "o
// desligamento não funciona" — `closeBrowserViaCDP` sempre funcionou. O defeito
// é "o call site chama a função errada", e isso só se vê olhando os call sites.
// Um teste de comportamento sobre `cleanStop` passaria com os cinco modos ainda
// quebrados.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// A isenção é POR CHAMADA, marcada com `//ablation:stop-form` na própria linha,
// e não por nome de função.
//
// A primeira versão deste teste isentava funções inteiras, e isso quase custou
// três defeitos: em `oneRecoveryTrial` apenas UMA das quatro chamadas a
// `gracefulStop` era a variável sob ablação — as outras três eram limpeza de
// caminho de erro sobre o perfil pareado, e uma delas roda justamente quando o
// baseline falha, ou seja, suja o perfil quando a credencial já está frágil.
//
// Isenção por nome de função é larga demais: ela cobre também o código que
// ainda vai ser escrito ali dentro.
const ablationMarker = "ablation:stop-form"

// cleanStopVia implementa o próprio caminho rotulado, então é a única isenção
// nominal que resta.
const shutdownImplFunc = "cleanStopVia"

func TestCredentialModesNeverStopBySignal(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	fset := token.NewFileSet()
	var offenders []string

	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		// Linhas que carregam a isenção explícita.
		exemptLines := map[int]bool{}
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				if strings.Contains(c.Text, ablationMarker) {
					exemptLines[fset.Position(c.Pos()).Line] = true
				}
			}
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name.Name == shutdownImplFunc {
				continue
			}
			var touchesProfile bool
			var signalStops []token.Pos

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.AssignStmt:
					// PersistentProfileDir = dir  -> a função monta o perfil
					// persistente, logo o browser dela carrega a credencial.
					for _, lhs := range v.Lhs {
						if id, ok := lhs.(*ast.Ident); ok && id.Name == "PersistentProfileDir" {
							if len(v.Rhs) == 1 {
								if s, ok := v.Rhs[0].(*ast.BasicLit); ok && s.Value == `""` {
									continue // a limpeza logo após o launch
								}
							}
							touchesProfile = true
						}
					}
				case *ast.CallExpr:
					if id, ok := v.Fun.(*ast.Ident); ok && id.Name == "gracefulStop" {
						if !exemptLines[fset.Position(v.Pos()).Line] {
							signalStops = append(signalStops, v.Pos())
						}
					}
				}
				return true
			})

			if touchesProfile && len(signalStops) > 0 {
				for _, p := range signalStops {
					offenders = append(offenders,
						fset.Position(p).String()+" em "+fn.Name.Name)
				}
			}
		}
	}

	if len(offenders) > 0 {
		t.Fatalf("modo que carrega a credencial desliga por sinal — SIGTERM corrompe o "+
			"estado de sessao (Fase 4C §7 req. 1). Use cleanStop.\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

// Contraprova do teste acima: ele só vale se `cleanStop` de fato existir e
// passar por `Browser.close`. Sem isto, apagar o corpo de cleanStop deixaria a
// suíte verde com todos os modos desligando por sinal por dentro.
func TestCleanStopGoesThroughBrowserClose(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p4c_lifecycle.go", nil, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var found bool
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "cleanStopVia" || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "closeBrowserViaCDP" {
					found = true
				}
			}
			return true
		})
	}
	if !found {
		t.Fatal("cleanStopVia nao chama closeBrowserViaCDP: o desligamento limpo sumiu")
	}
}
