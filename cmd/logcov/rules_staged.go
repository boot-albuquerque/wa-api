package main

import (
	"go/ast"
	"go/types"
)

// Numerador L1-d — observabilidade por ESTÁGIO TIPADO e por OPERAÇÃO RASTREADA
// (decisão 78).
//
// # Por que esta regra existe, e por que ela quebra rules_frozen
//
// A Fase 3 fiou `internal/wa-headless` em `pkg/`, e com isso a árvore inteira
// entrou no denominador: `eligible` 571 → 660, `func_coverage` 697 → 603,
// `errpath` 858 → 808. Foi MEDIDO com prova causal — removido o consumidor da
// fachada, os números voltam exatamente aos anteriores.
//
// A queda não é regressão de instrumentação: nenhum sítio deixou de logar. É
// que aquela árvore **não observa por linha de log**. Ela observa por duas
// formas que a régua da camada de aplicação não enxergava:
//
//	BootFailure{Stage: StageOwnership, Cause: err}   o erro CARREGA onde falhou
//	runner.Do(ctx, OpBoot, "launch/await-endpoint")  a operação fica no OpLog
//
// Um estágio tipado diz MAIS que uma linha de log: ele chega intacto a quem
// decide se aquilo é erro, e é essa camada que loga — o mesmo padrão que este
// repositório já aceitou dezenas de vezes ("adapter não loga, use case loga"),
// agora em escala de biblioteca.
//
// Medir essa árvore com a régua da aplicação faria uma de duas coisas, ambas
// ruins: excluí-la do denominador, que é encolher a base para embelezar o
// número (o que `min_eligible` existe para impedir), ou instrumentar 85 funções
// com logs que contradizem o desenho delas. A decisão 78 escolheu a terceira,
// que é cara e correta: ensinar a régua.
//
// `rules_frozen=true` vale desde a Fase 12, e o próprio texto diz que uma
// mudança depois disso é decisão deliberada e não efeito colateral. Esta é
// deliberada, e está escrita aqui em vez de num commit.

// stageFieldName e causeFieldName são os dois campos que, JUNTOS, fazem de um
// literal uma falha com estágio. Um sem o outro não basta: `Stage` sozinho
// classifica sem dizer a causa, e `Cause` sozinho embrulha sem dizer onde.
const (
	stageFieldName = "Stage"
	causeFieldName = "Cause"
)

// opKindTypeName é o tipo que marca uma operação rastreada. A verificação é
// por TIPO e não por nome de variável: um `string` chamado `op` não conta.
const opKindTypeName = "OpKind"

// ruleL1dStagedFailure reconhece a construção de uma falha que carrega o
// estágio em que ocorreu.
//
// Exige os DOIS campos preenchidos no literal. O nível é Error porque construir
// uma dessas é, por definição, relatar uma falha — o que a faz valer também
// para L2, que exige nível >= Warn no caminho de saída.
func ruleL1dStagedFailure(p *pkgCtx, lit *ast.CompositeLit) (logSite, bool) {
	if lit == nil || len(lit.Elts) == 0 {
		return logSite{}, false
	}
	var hasStage, hasCause bool
	fields := 0
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		fields++
		switch key.Name {
		case stageFieldName:
			hasStage = true
		case causeFieldName:
			hasCause = true
		}
	}
	if !hasStage || !hasCause {
		return logSite{}, false
	}
	return logSite{
		form:   "stage",
		level:  "Error",
		fields: fields,
		pos:    p.a.fset.Position(lit.Lbrace),
		node:   lit,
	}, true
}

// ruleL1dTracedOp reconhece uma operação executada através do rastreador, que
// a registra com a sua classe e o seu rótulo.
//
// O nível é Info, e isso é deliberado: rastrear uma operação prova que ela
// ACONTECEU, não que alguém classificou uma falha. Por isso satisfaz L1
// (presença) e NÃO satisfaz L2, que cobre caminhos de saída e exige >= Warn.
// Tratá-la como Warn faria toda função que chama o rastreador parecer ter
// coberto os seus erros, que é precisamente a confiança falsa que esta métrica
// existe para não dar.
func ruleL1dTracedOp(p *pkgCtx, call *ast.CallExpr) (logSite, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Do" || len(call.Args) < 3 {
		return logSite{}, false
	}
	if p.a.ctxIface == nil || !p.a.implements(p.info.TypeOf(call.Args[0]), p.a.ctxIface) {
		return logSite{}, false
	}
	if !isNamedType(p.info.TypeOf(call.Args[1]), opKindTypeName) {
		return logSite{}, false
	}
	// O rótulo tem de conter ao menos um pedaço CONSTANTE. O idioma real desta
	// árvore é `label + "/kick"`: a base vem de quem chamou e o sufixo é
	// literal, e o par localiza o sítio tão bem quanto um literal inteiro.
	//
	// A primeira versão desta regra exigia literal puro e, por isso, rejeitava
	// 163 dos 169 sítios — ela media a minha suposição sobre o código em vez do
	// código. Um rótulo inteiramente montado em tempo de execução continua
	// recusado: pode ser vazio, e um rastro sem rótulo não localiza nada.
	if !hasConstantLabel(call.Args[2]) {
		return logSite{}, false
	}
	return logSite{
		form:   "op",
		level:  "Info",
		fields: 2, // a classe e o rótulo, ambos estruturados por construção
		pos:    p.a.fset.Position(call.Lparen),
		node:   call,
	}, true
}

func isNamedType(t types.Type, name string) bool {
	named, ok := t.(*types.Named)
	if !ok || named.Obj() == nil {
		return false
	}
	return named.Obj().Name() == name
}

// hasConstantLabel reporta se a expressão carrega algum literal de texto não
// vazio, direto ou dentro de uma concatenação.
func hasConstantLabel(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.BasicLit:
		return v.Kind.String() == "STRING" && len(v.Value) > 2
	case *ast.BinaryExpr:
		return hasConstantLabel(v.X) || hasConstantLabel(v.Y)
	case *ast.ParenExpr:
		return hasConstantLabel(v.X)
	}
	return false
}
