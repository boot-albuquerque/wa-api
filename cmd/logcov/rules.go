package main

// rules.go — uma funcao por regra da secao 9.1 do plano (cmd/logcov/METRIC.md).
//
// Toda resolucao aqui e' TYPE-AWARE: nada e' decidido por nome de identificador.
// O nome de um metodo so' entra na decisao depois que o TIPO do receptor foi
// resolvido por go/types. E' o requisito no 1 da fase (§0.3b do plano).

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"
)

// ---------------------------------------------------------------------------
// Denominador — X1..X8 (secao 9.1.2)
// ---------------------------------------------------------------------------

// ruleX5PackageExcluded — funcoes em pacotes listados em .logcov-exclude.
func ruleX5PackageExcluded(a *analysis, pkgRel string) bool {
	for _, pref := range a.excludes {
		if pkgRel == strings.TrimSuffix(pref, "/") || strings.HasPrefix(pkgRel+"/", pref) {
			return true
		}
	}
	return false
}

// ruleX6FuncLitEligible — um *ast.FuncLit e' EXCLUIDO, exceto quando for
// operando de GoStmt/DeferStmt, argumento de (*errgroup.Group).Go /
// sync.OnceFunc / sync.OnceValue, ou de tipo http.Handler / http.HandlerFunc /
// func(http.Handler) http.Handler. Retorna true quando o FuncLit E' elegivel
// como entrada propria.
func ruleX6FuncLitEligible(p *pkgCtx, lit *ast.FuncLit) bool {
	// `go func(){}()` e `defer func(){}()` chegam aqui com o CallExpr como
	// pai — o GoStmt/DeferStmt e' o avo'. `go f(func(){})` idem, com o
	// literal como argumento.
	if call, ok := p.parent[lit].(*ast.CallExpr); ok {
		switch p.parent[call].(type) {
		case *ast.GoStmt, *ast.DeferStmt:
			return true
		}
		if isConcurrencyLauncher(p, call) {
			return true
		}
	}
	return isHandlerShapedFuncLit(p, lit)
}

// isConcurrencyLauncher — a chamada e' (*errgroup.Group).Go, sync.OnceFunc ou
// sync.OnceValue, resolvidas pelo objeto (pacote + nome), nunca pelo texto.
func isConcurrencyLauncher(p *pkgCtx, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	obj := p.info.Uses[sel.Sel]
	fn, ok := obj.(*types.Func)
	if !ok || fn.Pkg() == nil {
		return false
	}
	switch fn.Pkg().Path() {
	case "sync":
		return fn.Name() == "OnceFunc" || fn.Name() == "OnceValue"
	case "golang.org/x/sync/errgroup":
		return fn.Name() == "Go"
	}
	return false
}

// isHandlerShapedFuncLit — o literal tem a forma de um http.Handler
// (func(http.ResponseWriter, *http.Request)) ou de um middleware
// (func(http.Handler) http.Handler).
func isHandlerShapedFuncLit(p *pkgCtx, lit *ast.FuncLit) bool {
	sig, ok := p.info.TypeOf(lit).(*types.Signature)
	if !ok {
		return false
	}
	if p.a.httpHandler == nil {
		return false
	}
	return isHandlerFuncShape(p, sig) || isMiddlewareShape(p, sig)
}

// isHandlerFuncShape — func(http.ResponseWriter, *http.Request).
func isHandlerFuncShape(p *pkgCtx, sig *types.Signature) bool {
	ps, rs := sig.Params(), sig.Results()
	if ps.Len() != 2 || rs.Len() != 0 {
		return false
	}
	return p.a.implements(ps.At(0).Type(), p.a.httpResponseWriter) &&
		types.Identical(ps.At(1).Type(), p.a.httpRequestPtr)
}

// isMiddlewareShape — func(http.Handler) http.Handler.
func isMiddlewareShape(p *pkgCtx, sig *types.Signature) bool {
	ps, rs := sig.Params(), sig.Results()
	if ps.Len() != 1 || rs.Len() != 1 {
		return false
	}
	return p.a.implements(ps.At(0).Type(), p.a.httpHandler) &&
		p.a.implements(rs.At(0).Type(), p.a.httpHandler)
}

// ruleX3Init — func init().
func ruleX3Init(fd *ast.FuncDecl) bool {
	return fd != nil && fd.Recv == nil && fd.Name.Name == "init"
}

// ruleX4SelfReferential — String/Error/MarshalJSON/UnmarshalJSON/Format.
// Logar dentro deles e' recursao infinita via zerolog.
func ruleX4SelfReferential(p *pkgCtx, fd *ast.FuncDecl) bool {
	if fd == nil {
		return false
	}
	obj, _ := p.info.Defs[fd.Name].(*types.Func)
	if obj == nil {
		return false
	}
	want, ok := selfReferentialSignatures[fd.Name.Name]
	return ok && signatureMatches(obj.Signature(), want)
}

// selfReferentialSignature descreve a assinatura exata de um metodo de X4.
type selfReferentialSignature struct {
	params  []string
	results []string
}

// selfReferentialSignatures — a tabela de X4. A verificacao e' por assinatura,
// nao so' por nome: um `String() (string, error)` nao e' o String do fmt.
var selfReferentialSignatures = map[string]selfReferentialSignature{
	"String":        {nil, []string{"string"}},
	"Error":         {nil, []string{"string"}},
	"MarshalJSON":   {nil, []string{"[]byte", "error"}},
	"UnmarshalJSON": {[]string{"[]byte"}, []string{"error"}},
	"Format":        {[]string{"fmt.State", "rune"}, nil},
}

func signatureMatches(sig *types.Signature, want selfReferentialSignature) bool {
	return tupleMatches(sig.Params(), want.params) && tupleMatches(sig.Results(), want.results)
}

func tupleMatches(tup *types.Tuple, want []string) bool {
	if tup.Len() != len(want) {
		return false
	}
	for i, name := range want {
		if types.TypeString(tup.At(i).Type(), nil) != name {
			return false
		}
	}
	return true
}

// ruleX8Exempt — comentario //log:exempt <motivo> imediatamente acima da
// declaracao. Valvula orcada por max_exempt_annotations.
func ruleX8Exempt(doc *ast.CommentGroup) (bool, string) {
	if doc == nil {
		return false, ""
	}
	for _, c := range doc.List {
		txt := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		if strings.HasPrefix(txt, "log:exempt") {
			return true, strings.TrimSpace(strings.TrimPrefix(txt, "log:exempt"))
		}
	}
	return false, ""
}

// ruleX1Trivial — corpo com <=2 statements E nenhum caminho de saida.
func ruleX1Trivial(body *ast.BlockStmt, paths []exitPath) bool {
	return body != nil && len(body.List) <= 2 && len(paths) == 0
}

// ruleX2PureAccessor — corpo e' um unico ReturnStmt cujos operandos sao apenas
// identificadores, literais ou selectors sobre o receiver.
func ruleX2PureAccessor(p *pkgCtx, fd *ast.FuncDecl, body *ast.BlockStmt) bool {
	if body == nil || len(body.List) != 1 {
		return false
	}
	ret, ok := body.List[0].(*ast.ReturnStmt)
	if !ok {
		return false
	}
	recv := receiverName(fd)
	for _, r := range ret.Results {
		if !isAccessorOperand(p, r, recv) {
			return false
		}
	}
	return len(ret.Results) > 0
}

func isAccessorOperand(p *pkgCtx, e ast.Expr, recv string) bool {
	switch x := e.(type) {
	case *ast.BasicLit:
		return true
	case *ast.Ident:
		return true
	case *ast.SelectorExpr:
		base, ok := x.X.(*ast.Ident)
		return ok && recv != "" && base.Name == recv
	case *ast.StarExpr:
		return isAccessorOperand(p, x.X, recv)
	case *ast.UnaryExpr:
		return x.Op == token.AND && isAccessorOperand(p, x.X, recv)
	}
	return false
}

func receiverName(fd *ast.FuncDecl) string {
	if fd == nil || fd.Recv == nil || len(fd.Recv.List) == 0 || len(fd.Recv.List[0].Names) == 0 {
		return ""
	}
	return fd.Recv.List[0].Names[0].Name
}

// ruleX7PureDelegation — corpo e' exatamente uma chamada e um return, os
// argumentos da chamada sao exatamente os parametros da funcao (sem
// transformacao) e o retorno e' exatamente o resultado da chamada.
func ruleX7PureDelegation(p *pkgCtx, params []string, body *ast.BlockStmt) bool {
	if body == nil || len(body.List) != 1 {
		return false
	}
	ret, ok := body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	if len(call.Args) != len(params) {
		return false
	}
	for i, arg := range call.Args {
		id, ok := arg.(*ast.Ident)
		if !ok || id.Name != params[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Numerador L1 — presenca (secao 9.1.3)
// ---------------------------------------------------------------------------

// logSite e' uma chamada de log reconhecida.
type logSite struct {
	form   string // "port" | "zerolog" | "hlog"
	level  string // Trace|Debug|Info|Warn|Error|Fatal|Panic
	fields int    // L1-a: len(keyvals); L1-b/c: metodos de campo encadeados
	odd    bool   // L1-a: keyvals impar
	pos    token.Position
	node   ast.Node
	valid  bool // passa L3
}

var logLevelRank = map[string]int{
	"Trace": 0, "Debug": 1, "Info": 2, "Warn": 3, "Error": 4, "Fatal": 5, "Panic": 6,
}

// zerologFieldMethods — os metodos de campo que satisfazem L3 em L1-b/L1-c.
var zerologFieldMethods = map[string]bool{
	"Str": true, "Strs": true, "Int": true, "Int64": true, "Uint": true,
	"Float64": true, "Bool": true, "Dur": true, "Time": true, "Err": true,
	"AnErr": true, "Interface": true, "Any": true, "Bytes": true,
	"Stringer": true, "RawJSON": true, "Fields": true, "Ctx": true,
}

// ruleL1aPort — chamada a Info/Warn/Error sobre valor cujo tipo implementa
// wa-api/pkg/application/contracts.Logger, com context.Context como primeiro
// argumento. A porta e' resolvida por implementacao de interface via go/types.
func ruleL1aPort(p *pkgCtx, call *ast.CallExpr) (logSite, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return logSite{}, false
	}
	switch sel.Sel.Name {
	case "Info", "Warn", "Error":
	default:
		return logSite{}, false
	}
	if p.a.loggerIface == nil {
		return logSite{}, false
	}
	recvType := p.info.TypeOf(sel.X)
	if recvType == nil || !p.a.implements(recvType, p.a.loggerIface) {
		return logSite{}, false
	}
	if len(call.Args) < 1 {
		return logSite{}, false
	}
	if p.a.ctxIface == nil || !p.a.implements(p.info.TypeOf(call.Args[0]), p.a.ctxIface) {
		return logSite{}, false
	}
	// A assinatura da porta e' Info(ctx, msg string, keyvals ...any): o
	// compilador ja' garante os dois primeiros argumentos.
	keyvals := len(call.Args) - 2
	return logSite{
		form:   "port",
		level:  sel.Sel.Name,
		fields: keyvals,
		odd:    keyvals%2 != 0,
		pos:    p.a.fset.Position(call.Lparen),
		node:   call,
	}, true
}

// ruleL1bZerolog / ruleL1cHlog — cadeia terminando em .Msg/.Msgf/.Send.
// As duas compartilham o desenrolamento da cadeia; o que as distingue e' a
// raiz. Retorna o site com form "zerolog" ou "hlog".
func ruleL1bcChain(p *pkgCtx, call *ast.CallExpr) (logSite, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return logSite{}, false
	}
	switch sel.Sel.Name {
	case "Msg", "Msgf", "Send":
	default:
		return logSite{}, false
	}
	fields := 0
	cur := sel.X
	for {
		inner, ok := cur.(*ast.CallExpr)
		if !ok {
			return logSite{}, false
		}
		isel, ok := inner.Fun.(*ast.SelectorExpr)
		if !ok {
			return logSite{}, false
		}
		name := isel.Sel.Name
		if _, isLevel := logLevelRank[name]; isLevel {
			if form, ok := chainRootForm(p, isel.X); ok {
				return logSite{
					form:   form,
					level:  name,
					fields: fields,
					pos:    p.a.fset.Position(call.Lparen),
					node:   call,
				}, true
			}
		}
		if zerologFieldMethods[name] {
			fields++
		}
		cur = isel.X
	}
}

// chainRootForm classifica a raiz de uma cadeia zerolog.
//   - "zerolog": ident de pacote github.com/rs/zerolog/log, ou expressao de
//     tipo zerolog.Logger / *zerolog.Logger.
//   - "hlog": chamada hlog.FromRequest(...).
func chainRootForm(p *pkgCtx, root ast.Expr) (string, bool) {
	// hlog vem PRIMEIRO: hlog.FromRequest devolve *zerolog.Logger, e a
	// checagem por tipo classificaria a cadeia como L1-b. A distincao importa
	// porque L1-b tem restricao por diretorio e L1-c nao.
	if rootIsHlogFromRequest(p, root) {
		return "hlog", true
	}
	if rootIsZerologPkg(p, root) || rootIsZerologTyped(p, root) {
		return "zerolog", true
	}
	return "", false
}

// rootIsZerologPkg — a raiz e' o identificador do pacote
// github.com/rs/zerolog/log, resolvido pelo *types.PkgName do import.
func rootIsZerologPkg(p *pkgCtx, root ast.Expr) bool {
	id, ok := root.(*ast.Ident)
	if !ok {
		return false
	}
	pn, ok := p.info.Uses[id].(*types.PkgName)
	return ok && pn.Imported().Path() == "github.com/rs/zerolog/log"
}

// rootIsZerologTyped — a raiz e' uma expressao de tipo zerolog.Logger ou
// *zerolog.Logger.
func rootIsZerologTyped(p *pkgCtx, root ast.Expr) bool {
	if p.a.zerologLogger == nil {
		return false
	}
	t := p.info.TypeOf(root)
	if t == nil {
		return false
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	return types.Identical(t, p.a.zerologLogger)
}

// rootIsHlogFromRequest — a raiz e' hlog.FromRequest(...).
func rootIsHlogFromRequest(p *pkgCtx, root ast.Expr) bool {
	c, ok := root.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	fn, ok := p.info.Uses[sel.Sel].(*types.Func)
	return ok && fn.Pkg() != nil &&
		fn.Pkg().Path() == "github.com/rs/zerolog/hlog" && fn.Name() == "FromRequest"
}

// ruleL1bValidHere — restricao por diretorio (§0.3c): nos diretorios da regra
// depguard/camada-de-aplicacao, L1-b NAO e' forma valida. Aceitar zerolog ali
// seria premiar o que o lint reprova.
func ruleL1bValidHere(a *analysis, fileRel string) bool {
	for _, d := range a.depguardDirs {
		if strings.HasPrefix(fileRel, d+"/") {
			return false
		}
	}
	return true
}

// ruleL3Structured — estruturacao POR CALL SITE.
//   - L1-a: len(keyvals) >= 2 e par;
//   - L1-b/c: >=1 metodo de campo encadeado antes do .Msg.
//
// Uma unica violacao em qualquer call site reprova a funcao inteira.
func ruleL3Structured(s logSite) bool {
	if s.form == "port" {
		return s.fields >= 2 && !s.odd
	}
	return s.fields >= 1
}

// ---------------------------------------------------------------------------
// Numerador L2 — caminhos de saida (secao 9.1.4)
// ---------------------------------------------------------------------------

type exitPath struct {
	kind  string // "S-ret" | "S-http" | "S-consume"
	pos   token.Position
	block *ast.BlockStmt // menor BlockStmt que o contem
	node  ast.Node
	// satisfiedByB e' verdadeiro quando o caminho e' S-ret e a expressao
	// retornada propaga a causa sem descarte.
	satisfiedByB bool
	covered      bool
}

// ruleL2SRet — ReturnStmt que retorna, em qualquer posicao, valor de tipo
// error que nao seja o literal nil.
func ruleL2SRet(p *pkgCtx, ret *ast.ReturnStmt) (ast.Expr, bool) {
	for _, r := range ret.Results {
		if isNilLiteral(p, r) {
			continue
		}
		t := p.info.TypeOf(r)
		if t == nil {
			continue
		}
		// `return f(...)` com resultado multiplo devolve o error do callee
		// numa posicao da tupla: e' caminho de saida do mesmo jeito.
		if tup, ok := t.(*types.Tuple); ok {
			if tupleHasError(p, tup) {
				return r, true
			}
			continue
		}
		if p.a.implements(t, p.a.errorIface) {
			return r, true
		}
	}
	return nil, false
}

func tupleHasError(p *pkgCtx, tup *types.Tuple) bool {
	for i := 0; i < tup.Len(); i++ {
		if p.a.implements(tup.At(i).Type(), p.a.errorIface) {
			return true
		}
	}
	return false
}

func isNilLiteral(p *pkgCtx, e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	if !ok {
		return false
	}
	_, isNil := p.info.Uses[id].(*types.Nil)
	return isNil
}

// ruleL2SHTTP — escrita de resposta com status >= 400.
//   - WriteHeader com argumento constante >= 400 (http.Status* e' constante
//     tipada, entao a avaliacao constante resolve os dois casos);
//   - http.Error;
//   - helpers de envelope de pkg/presentation/http cuja assinatura receba
//     status.
func ruleL2SHTTP(p *pkgCtx, call *ast.CallExpr) bool {
	idx, ok := statusArgIndex(p, call)
	if !ok || idx >= len(call.Args) {
		return false
	}
	tv, ok := p.info.Types[call.Args[idx]]
	if !ok || tv.Value == nil {
		return false
	}
	n, ok := constant.Int64Val(constant.ToInt(tv.Value))
	return ok && n >= 400
}

// statusArgIndex devolve o indice do argumento de status, resolvido pelo
// objeto da funcao chamada (nunca por texto).
func statusArgIndex(p *pkgCtx, call *ast.CallExpr) (int, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return 0, false
	}
	fn, ok := p.info.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil {
		return 0, false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return 0, false
	}
	if sig.Recv() != nil {
		// WriteHeader sobre um http.ResponseWriter.
		ok := fn.Name() == "WriteHeader" &&
			p.a.implements(p.info.TypeOf(sel.X), p.a.httpResponseWriter)
		return 0, ok
	}
	return packageStatusArgIndex(p, fn, sig)
}

// packageStatusArgIndex trata http.Error e os writers de envelope do
// repositorio (funcoes de pacote, sem receiver).
func packageStatusArgIndex(p *pkgCtx, fn *types.Func, sig *types.Signature) (int, bool) {
	if fn.Pkg().Path() == "net/http" && fn.Name() == "Error" {
		return 2, true
	}
	if !strings.HasPrefix(fn.Pkg().Path(), p.a.modulePath+"/pkg/presentation/http") {
		return 0, false
	}
	return statusParamIndex(sig)
}

// statusParamIndex acha um parametro int de status numa assinatura que tambem
// recebe um http.ResponseWriter.
func statusParamIndex(sig *types.Signature) (int, bool) {
	params := sig.Params()
	hasWriter := false
	for i := 0; i < params.Len(); i++ {
		if strings.HasSuffix(types.TypeString(params.At(i).Type(), nil), "http.ResponseWriter") {
			hasWriter = true
		}
	}
	if !hasWriter {
		return 0, false
	}
	for i := 0; i < params.Len(); i++ {
		pv := params.At(i)
		if types.TypeString(pv.Type(), nil) != "int" {
			continue
		}
		switch strings.ToLower(pv.Name()) {
		case "status", "statuscode", "code", "httpstatus":
			return i, true
		}
	}
	return 0, false
}

// ruleL2SConsume — bloco de `if err != nil` (ou switch sobre err) que NAO
// propaga err inalterado: o bloco termina sem retornar err, ou retorna erro
// construido sem encadear a causa.
func ruleL2SConsumeIf(p *pkgCtx, ifs *ast.IfStmt) (*ast.Ident, bool) {
	bin, ok := ifs.Cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.NEQ {
		return nil, false
	}
	id, ok := bin.X.(*ast.Ident)
	if !ok || !isNilLiteral(p, bin.Y) {
		return nil, false
	}
	t := p.info.TypeOf(id)
	if t == nil || !p.a.implements(t, p.a.errorIface) {
		return nil, false
	}
	if blockPropagates(p, ifs.Body, id) {
		return nil, false
	}
	return id, true
}

// ruleL2SConsumeSwitch — switch cuja tag e' de tipo error e cujo nenhum corpo
// de case propaga a causa.
func ruleL2SConsumeSwitch(p *pkgCtx, sw *ast.SwitchStmt) (*ast.Ident, bool) {
	if sw.Tag == nil {
		return nil, false
	}
	id, ok := sw.Tag.(*ast.Ident)
	if !ok {
		return nil, false
	}
	t := p.info.TypeOf(id)
	if t == nil || !p.a.implements(t, p.a.errorIface) {
		return nil, false
	}
	if blockPropagates(p, sw.Body, id) {
		return nil, false
	}
	return id, true
}

// blockPropagates — algum ReturnStmt do bloco devolve o erro propagando a
// causa sem descarte.
func blockPropagates(p *pkgCtx, block *ast.BlockStmt, errID *ast.Ident) bool {
	found := false
	ast.Inspect(block, func(n ast.Node) bool {
		if found {
			return false
		}
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, r := range ret.Results {
			if ruleL2PropagatesCause(p, r, errID) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// ruleL2PropagatesCause — cobertura (b), RESTRITA. A expressao propaga a causa
// sem descarte quando e':
//   - o identificador do erro recebido inalterado (`return err`);
//   - apperr.Wrap(err, ...) / qualquer construtor de wa-api/pkg/domain/apperr
//     que receba um error nao-nil entre os argumentos;
//   - fmt.Errorf("...: %w", err).
//
// Construcao de erro NOVO que descarta a causa nunca satisfaz (b), mesmo
// usando apperr.New sem passar err.
func ruleL2PropagatesCause(p *pkgCtx, e ast.Expr, errID *ast.Ident) bool {
	switch x := e.(type) {
	case *ast.Ident:
		if isNilLiteral(p, x) {
			return false
		}
		t := p.info.TypeOf(x)
		if t == nil || !p.a.implements(t, p.a.errorIface) {
			return false
		}
		if errID != nil {
			return p.info.Uses[x] == p.info.Uses[errID] || x.Name == errID.Name
		}
		return true
	case *ast.CallExpr:
		// `return f(...)` de resultado multiplo devolve o error do callee
		// verbatim: e' a mesma propagacao sem descarte de `return err`.
		if tup, ok := p.info.TypeOf(x).(*types.Tuple); ok && tupleHasError(p, tup) {
			return true
		}
		return callChainsCause(p, x, errID)
	}
	return false
}

func callChainsCause(p *pkgCtx, call *ast.CallExpr, errID *ast.Ident) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	fn, ok := p.info.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil {
		return false
	}
	path, name := fn.Pkg().Path(), fn.Name()
	switch {
	case path == "fmt" && name == "Errorf":
		return wrapsWithVerb(call) && callHasErrArg(p, call, errID)
	case path == "errors" && name == "Join":
		return callHasErrArg(p, call, errID)
	case path == p.a.modulePath+"/pkg/domain/apperr":
		// apperr.Wrap(err, ...) e apperr.New(..., err) so' propagam quando um
		// error de verdade e' passado. apperr.New(..., nil) descarta a causa.
		return callHasErrArg(p, call, errID)
	}
	return false
}

// wrapsWithVerb — o formato de fmt.Errorf carrega %w.
func wrapsWithVerb(call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	return ok && strings.Contains(lit.Value, "%w")
}

// callHasErrArg — algum argumento da chamada e' um error nao-nil que
// corresponde ao erro em questao.
func callHasErrArg(p *pkgCtx, call *ast.CallExpr, errID *ast.Ident) bool {
	for _, a := range call.Args {
		if isNilLiteral(p, a) {
			continue
		}
		t := p.info.TypeOf(a)
		if t == nil || !p.a.implements(t, p.a.errorIface) {
			continue
		}
		id, ok := a.(*ast.Ident)
		if errID == nil || !ok {
			return true
		}
		return p.info.Uses[id] == p.info.Uses[errID] || id.Name == errID.Name
	}
	return false
}
