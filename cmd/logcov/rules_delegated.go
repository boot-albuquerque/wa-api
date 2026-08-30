package main

// Numerador L1-e — DELEGAÇÃO OBSERVÁVEL, decisão 91 (2026-08-23).
//
// # O defeito que esta regra corrige, medido e não suposto
//
// Elegibilidade exige ALCANCE DE PRODUÇÃO: uma capability de
// `internal/headless` só entra no denominador quando `pkg/` a liga. Como as
// capabilities foram escritas antes de serem ligadas, cada port novo acordava
// de uma vez a dívida inteira de uma capability — e `min_func_coverage`, que o
// baseline declara ratchet-UP, DESCEU nos seis commits anteriores a este:
//
//	564 -> 560 -> 558 -> 554 -> 551 -> 545 -> 543
//
// Cada queda tinha justificativa própria e correta. O padrão não tinha
// ninguém, porque cada commit só vê o próprio delta. Com 20 das 35
// capabilities ainda por ligar, o fim da fase estava projetado perto de 45%.
//
// # Por que o instrumento é que estava errado
//
// Um método como `groupreq.Manager.List` não chama `runner.Do` diretamente:
// chama `m.parked(ctx, script, key, label+"/list")`, e é `parked` quem rastreia.
// A operação É observável — o rastro existe e nomeia a operação de quem chamou.
// A métrica é que creditava o ajudante e cobrava do método.
//
// Reestruturar a produção para satisfazer a métrica seria escrever código para
// agradar o instrumento. A decisão 91 mandou o contrário: corrigir o
// instrumento, preservando denominador e piso.
//
// # O que faz a delegação ser COMPROVADA e não presumida
//
// Duas condições, e a segunda é a que impede isto de virar carimbo:
//
//  1. o destino da chamada está no MESMO pacote e ele próprio satisfaz L1 por
//     operação rastreada (forma `op`, a regra L1-d); e
//  2. quem chama passa um RÓTULO com pedaço constante para o destino.
//
// A (2) é o coração. Se o ajudante rastreasse sob nome próprio, o rastro diria
// que o ajudante rodou e não QUAL chamador o accionou — e aí não haveria
// observabilidade daquela função a creditar. Exigir o rótulo repassado é exigir
// que a operação de quem chama apareça no rastro, que é exatamente o que L1
// promete.
//
// Um salto só, deliberadamente. Cadeia de delegação mais funda credita cada vez
// mais longe do sítio observável, e o valor da prova cai com a distância.

import (
	"go/ast"
	"go/types"
)

// delegation é uma chamada candidata a creditar L1 por delegação: o alvo, e o
// sítio onde a chamada acontece.
type delegation struct {
	calleeKey string
	site      logSite
}

// collectDelegations acha, num corpo, as chamadas a funções do MESMO pacote que
// levam rótulo constante. Não decide nada: quem decide é applyDelegatedL1,
// depois de todas as entradas existirem, porque o alvo pode ser analisado
// depois de quem o chama.
func (a *analysis) collectDelegations(p *pkgCtx, pkgRel string, body *ast.BlockStmt, promoted map[*ast.FuncLit]bool) []delegation {
	var out []delegation
	ast.Inspect(body, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok && promoted[lit] {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		key, ok := a.samePackageCallee(p, pkgRel, call)
		if !ok {
			return true
		}
		// O rótulo repassado é a prova. Sem ele, o rastro do ajudante não
		// nomeia a operação de quem chamou.
		if !anyConstantLabel(call.Args) {
			return true
		}
		out = append(out, delegation{
			calleeKey: key,
			site: logSite{
				form:   "delegated",
				level:  "Info",
				fields: 2,
				pos:    a.fset.Position(call.Lparen),
				node:   call,
			},
		})
		return true
	})
	return out
}

// samePackageCallee devolve a chave da função chamada quando ela é declarada no
// mesmo pacote, na mesma grafia que declKey produz.
func (a *analysis) samePackageCallee(p *pkgCtx, pkgRel string, call *ast.CallExpr) (string, bool) {
	var id *ast.Ident
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		id = fun
	case *ast.SelectorExpr:
		id = fun.Sel
	default:
		return "", false
	}
	obj, ok := p.info.Uses[id].(*types.Func)
	if !ok || obj.Pkg() == nil || p.pkg.Types == nil || obj.Pkg() != p.pkg.Types {
		return "", false
	}
	sig, ok := obj.Type().(*types.Signature)
	if !ok {
		return "", false
	}
	if recv := sig.Recv(); recv != nil {
		return pkgRel + "." + recvTypeName(recv.Type()) + "." + obj.Name(), true
	}
	return pkgRel + "." + obj.Name(), true
}

// recvTypeName reduz o tipo do receptor ao nome que declKey escreve.
func recvTypeName(t types.Type) string {
	for {
		switch v := t.(type) {
		case *types.Pointer:
			t = v.Elem()
		case *types.Named:
			if v.Obj() == nil {
				return "?"
			}
			return v.Obj().Name()
		default:
			return "?"
		}
	}
}

// anyConstantLabel reporta se algum argumento carrega rótulo com pedaço
// constante, reusando a mesma leitura que L1-d faz do rótulo de runner.Do.
func anyConstantLabel(args []ast.Expr) bool {
	for _, arg := range args {
		if hasConstantLabel(arg) {
			return true
		}
	}
	return false
}

// applyDelegatedL1 corre DEPOIS de todas as entradas existirem e credita L1 às
// que só têm delegação observável.
//
// Só credita quem ainda não tem L1: a regra não inventa cobertura para quem já
// a tem, e não muda o denominador. E só credita se o alvo satisfizer L1 por
// operação RASTREADA — herdar de um alvo que também só delega faria a prova
// afastar-se do sítio observável a cada salto.
func applyDelegatedL1(rep *report) {
	traced := map[string]bool{}
	for _, e := range rep.Entries {
		if e.Excluded != "" {
			continue
		}
		for _, s := range e.Sites {
			if s.form == "op" {
				traced[e.Key] = true
				break
			}
		}
	}
	for i := range rep.Entries {
		e := &rep.Entries[i]
		if e.Excluded != "" || e.L1 {
			continue
		}
		for _, d := range e.Delegations {
			if !traced[d.calleeKey] {
				continue
			}
			site := d.site
			site.valid = ruleL3Structured(site)
			e.Sites = append(e.Sites, site)
			e.L1 = true
			e.L3 = site.valid
			break
		}
	}
}
