package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"
)

// Estes testes existem porque os controles negativos sobre o REPOSITÓRIO não
// morderam: não há hoje nenhum literal com `Stage` sem `Cause`, nem nenhum
// `.Do(ctx, X, "texto")` cujo segundo argumento não seja `OpKind`. Afrouxar a
// regra nesses dois pontos passava com o golden intacto — e uma propriedade
// que nenhum teste segura não está travada, por mais que o código a expresse.

func litComCampos(campos ...string) *ast.CompositeLit {
	lit := &ast.CompositeLit{Lbrace: token.Pos(1)}
	for _, c := range campos {
		lit.Elts = append(lit.Elts, &ast.KeyValueExpr{
			Key:   ast.NewIdent(c),
			Value: ast.NewIdent("x"),
		})
	}
	return lit
}

func ctxComFset() *pkgCtx {
	p := novoCtx(nil)
	p.a.fset = token.NewFileSet()
	return p
}

// Stage E Cause, juntos: um sem o outro não é falha com estágio. `Stage`
// sozinho classifica sem dizer a causa; `Cause` sozinho embrulha sem dizer onde.
func TestL1dStagedExigeOsDoisCampos(t *testing.T) {
	casos := []struct {
		nome   string
		campos []string
		quer   bool
	}{
		{"Stage e Cause", []string{"Stage", "Cause"}, true},
		{"so Stage", []string{"Stage"}, false},
		{"so Cause", []string{"Cause"}, false},
		{"nenhum dos dois", []string{"Motivo", "Erro"}, false},
		{"vazio", nil, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			_, ok := ruleL1dStagedFailure(ctxComFset(), litComCampos(c.campos...))
			if ok != c.quer {
				t.Fatalf("got %v, want %v", ok, c.quer)
			}
		})
	}
}

// A falha com estágio vale como Error, e isso é o que a faz cobrir também L2:
// construir uma dessas é, por definição, relatar uma falha.
func TestL1dStagedContaComoError(t *testing.T) {
	s, ok := ruleL1dStagedFailure(ctxComFset(), litComCampos("Stage", "Cause"))
	if !ok {
		t.Fatal("literal com Stage e Cause não foi reconhecido")
	}
	if s.level != "Error" {
		t.Fatalf("level=%q, quero Error — senão a falha com estágio não cobre L2", s.level)
	}
	if s.form != "stage" {
		t.Fatalf("form=%q, quero stage", s.form)
	}
}

// tipoNomeado devolve um *types.Named chamado nome, para exercitar a checagem
// por TIPO que distingue uma operação rastreada de um `.Do` qualquer.
func tipoNomeado(nome string) types.Type {
	obj := types.NewTypeName(token.NoPos, nil, nome, nil)
	return types.NewNamed(obj, types.Typ[types.String].Underlying(), nil)
}

type ctxFalso struct{}

func ctxParaOp(tipoSegundoArg types.Type, rotulo ast.Expr) (*pkgCtx, *ast.CallExpr) {
	ctxArg := ast.NewIdent("ctx")
	kindArg := ast.NewIdent("k")
	call := &ast.CallExpr{
		Fun: &ast.SelectorExpr{X: ast.NewIdent("r"), Sel: ast.NewIdent("Do")},
		Args: []ast.Expr{ctxArg, kindArg, rotulo,
			&ast.FuncLit{Type: &ast.FuncType{}, Body: &ast.BlockStmt{}}},
		Lparen: token.Pos(1),
	}
	iface := types.NewInterfaceType(nil, nil)
	iface.Complete()
	p := novoCtx(map[ast.Expr]types.TypeAndValue{
		ctxArg:  {Type: types.NewInterfaceType(nil, nil)},
		kindArg: {Type: tipoSegundoArg},
	})
	p.a.fset = token.NewFileSet()
	p.a.ctxIface = iface // interface vazia: tudo a satisfaz, isolando a checagem do KIND
	return p, call
}

// A distinção é por TIPO, e não por nome de variável: um `.Do` cujo segundo
// argumento seja um `string` qualquer não é operação rastreada, e contá-lo
// transformaria qualquer chamada de três argumentos em observabilidade.
func TestL1dTracedOpExigeOTipoOpKind(t *testing.T) {
	rotulo := &ast.BasicLit{Kind: token.STRING, Value: `"launch/await"`}

	p, call := ctxParaOp(tipoNomeado(opKindTypeName), rotulo)
	if _, ok := ruleL1dTracedOp(p, call); !ok {
		t.Fatal("Do com OpKind não foi reconhecido como operação rastreada")
	}

	p, call = ctxParaOp(tipoNomeado("Etiqueta"), rotulo)
	if _, ok := ruleL1dTracedOp(p, call); ok {
		t.Fatal("Do com um tipo nomeado QUALQUER passou por operação rastreada; " +
			"a regra tem de olhar o tipo, não a forma da chamada")
	}
}

// O rótulo tem de carregar algo constante. A concatenação `label + "/kick"` é o
// idioma real da árvore e conta; um rótulo inteiramente dinâmico não, porque
// pode ser vazio e um rastro sem rótulo não localiza nada.
func TestL1dTracedOpExigeRotuloComParteConstante(t *testing.T) {
	casos := []struct {
		nome   string
		rotulo ast.Expr
		quer   bool
	}{
		{"literal puro", &ast.BasicLit{Kind: token.STRING, Value: `"boot"`}, true},
		{"concatenacao com literal", &ast.BinaryExpr{
			X:  ast.NewIdent("label"),
			Op: token.ADD,
			Y:  &ast.BasicLit{Kind: token.STRING, Value: `"/kick"`},
		}, true},
		{"identificador nu", ast.NewIdent("label"), false},
		{"literal vazio", &ast.BasicLit{Kind: token.STRING, Value: `""`}, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			p, call := ctxParaOp(tipoNomeado(opKindTypeName), c.rotulo)
			_, ok := ruleL1dTracedOp(p, call)
			if ok != c.quer {
				t.Fatalf("got %v, want %v", ok, c.quer)
			}
		})
	}
}

// Rastrear prova que a operação ACONTECEU, não que alguém classificou uma
// falha. Por isso Info: tratá-la como Warn faria toda função que chama o
// rastreador parecer ter coberto os seus erros.
func TestL1dTracedOpNaoCobreCaminhoDeSaida(t *testing.T) {
	p, call := ctxParaOp(tipoNomeado(opKindTypeName),
		&ast.BasicLit{Kind: token.STRING, Value: `"boot"`})
	s, ok := ruleL1dTracedOp(p, call)
	if !ok {
		t.Fatal("não reconhecido")
	}
	if logLevelRank[s.level] >= logLevelRank["Warn"] {
		t.Fatalf("level=%q tem posto de Warn ou mais: o rastro passaria a cobrir "+
			"caminhos de saída, que é a confiança falsa que a métrica evita", s.level)
	}
}

var _ = ctxFalso{}
