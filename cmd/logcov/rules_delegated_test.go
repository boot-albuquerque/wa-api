package main

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

// Os testes da L1-e batem nas DUAS decisões que a sustentam, e as duas são
// pontos onde afrouxar passaria despercebido no golden:
//
//  1. só credita quem delega para um alvo que RASTREIA (forma `op`);
//  2. só credita quando um RÓTULO constante é repassado.
//
// Sem (1) a regra herdaria cobertura de quem não a tem. Sem (2) creditaria uma
// função cujo rastro não a nomeia — e um rastro que não diz qual chamador rodou
// não observa aquele chamador.

func entradaCom(key string, formas ...string) entry {
	e := entry{Key: key}
	for _, f := range formas {
		e.Sites = append(e.Sites, logSite{form: f, level: "Info", fields: 2})
	}
	e.L1 = len(e.Sites) > 0
	return e
}

func delegaPara(key, alvo string) entry {
	e := entry{Key: key}
	e.Delegations = []delegation{{
		calleeKey: alvo,
		site:      logSite{form: "delegated", level: "Info", fields: 2},
	}}
	return e
}

func creditadas(rep *report) map[string]bool {
	out := map[string]bool{}
	for _, e := range rep.Entries {
		out[e.Key] = e.L1
	}
	return out
}

func TestL1eCreditaDelegacaoParaAlvoRASTREADO(t *testing.T) {
	rep := &report{Entries: []entry{
		entradaCom("p.M.parked", "op"),
		delegaPara("p.M.List", "p.M.parked"),
	}}
	applyDelegatedL1(rep)
	if !creditadas(rep)["p.M.List"] {
		t.Fatal("delegação para alvo rastreado não foi creditada")
	}
}

func TestL1eNaoHerdaDeAlvoQueNaoRASTREIA(t *testing.T) {
	// O alvo tem log de outra forma, e não operação rastreada: não é o rastro
	// que nomeia a operação de quem chamou.
	rep := &report{Entries: []entry{
		entradaCom("p.M.ajudante", "zerolog"),
		delegaPara("p.M.List", "p.M.ajudante"),
	}}
	applyDelegatedL1(rep)
	if creditadas(rep)["p.M.List"] {
		t.Fatal("creditou delegação para alvo sem operação rastreada")
	}
}

func TestL1eNaoHerdaDeAlvoQueSoDELEGA(t *testing.T) {
	// Um salto só, deliberadamente: cadeia mais funda credita cada vez mais
	// longe do sítio observável.
	rep := &report{Entries: []entry{
		entradaCom("p.M.parked", "op"),
		delegaPara("p.M.meio", "p.M.parked"),
		delegaPara("p.M.ponta", "p.M.meio"),
	}}
	applyDelegatedL1(rep)
	c := creditadas(rep)
	if !c["p.M.meio"] {
		t.Fatal("o primeiro salto deixou de ser creditado")
	}
	if c["p.M.ponta"] {
		t.Fatal("creditou o SEGUNDO salto: a prova afastou-se do sítio observável")
	}
}

func TestL1eNaoCreditaFuncaoEXCLUIDA(t *testing.T) {
	rep := &report{Entries: []entry{
		entradaCom("p.M.parked", "op"),
		func() entry { e := delegaPara("p.M.trivial", "p.M.parked"); e.Excluded = "X1"; return e }(),
	}}
	applyDelegatedL1(rep)
	if creditadas(rep)["p.M.trivial"] {
		t.Fatal("creditou L1 a uma função fora do denominador")
	}
}

func TestL1eNaoMexeEmQuemJaTemL1(t *testing.T) {
	rep := &report{Entries: []entry{
		entradaCom("p.M.parked", "op"),
		func() entry {
			e := delegaPara("p.M.jaLoga", "p.M.parked")
			e.Sites = []logSite{{form: "zerolog"}}
			e.L1 = true
			return e
		}(),
	}}
	applyDelegatedL1(rep)
	for _, e := range rep.Entries {
		if e.Key == "p.M.jaLoga" && len(e.Sites) != 1 {
			t.Fatalf("acrescentou sítio a quem já tinha L1: %d sítios", len(e.Sites))
		}
	}
}

// O rótulo é a prova, e esta é a leitura que decide se ele existe.
func TestL1eExigeRotuloConstanteEmAlgumArgumento(t *testing.T) {
	rotulo := &ast.BasicLit{Kind: token.STRING, Value: `"cap/lista"`}
	sufixo := &ast.BinaryExpr{
		X:  ast.NewIdent("label"),
		Op: token.ADD,
		Y:  &ast.BasicLit{Kind: token.STRING, Value: `"/list"`},
	}
	casos := []struct {
		nome string
		args []ast.Expr
		quer bool
	}{
		{"literal puro", []ast.Expr{ast.NewIdent("ctx"), rotulo}, true},
		{"label + sufixo constante", []ast.Expr{ast.NewIdent("ctx"), sufixo}, true},
		{"tudo em tempo de execucao", []ast.Expr{ast.NewIdent("ctx"), ast.NewIdent("vindoDeFora")}, false},
		{"sem argumentos", nil, false},
	}
	for _, c := range casos {
		if got := anyConstantLabel(c.args); got != c.quer {
			t.Errorf("%s: anyConstantLabel = %v, queria %v", c.nome, got, c.quer)
		}
	}
}

// --- o caminho REAL, com informacao de tipos ---
//
// Os testes acima batem em applyDelegatedL1, que decide sobre entradas ja
// montadas. A exigencia do ROTULO, porem, vive em collectDelegations, e
// afrouxa-la passava por todos eles: o controle negativo NAO MORDEU. Um teste
// que nao morde e' pior que nenhum, entao esta parte carrega um modulo de
// verdade e exercita a regra com tipos resolvidos, que e' onde ela decide.

const corpusDelegacao = `package cap

import "context"

type OpKind string

type Runner struct{}

func (r *Runner) Do(ctx context.Context, k OpKind, label string, f func(context.Context) error) error {
	return f(ctx)
}

type M struct{ runner *Runner }

func (m *M) parked(ctx context.Context, label string) error {
	if label == "" {
		return context.Canceled
	}
	return m.runner.Do(ctx, OpKind("probe"), label+"/kick", func(context.Context) error { return nil })
}

func (m *M) ComRotulo(ctx context.Context) error {
	if ctx == nil {
		return context.Canceled
	}
	return m.parked(ctx, "cap/lista")
}

func (m *M) SemRotulo(ctx context.Context, vindoDeFora string) error {
	if ctx == nil {
		return context.Canceled
	}
	return m.parked(ctx, vindoDeFora)
}
`

func analisaCorpus(t *testing.T, src string) map[string]entry {
	t.Helper()
	dir := t.TempDir()
	// O corpus vive num SUBDIRETORIO: Analyze so' aceita pacotes cujo caminho
	// comece por modulePath + "/", e o pacote-raiz de um modulo nao tem barra.
	// Medido — a primeira versao pos o corpus na raiz e nenhuma funcao entrou
	// no universo, o que teria sido lido como "a regra nao creditou".
	if err := os.MkdirAll(filepath.Join(dir, "cap"), 0o750); err != nil {
		t.Fatalf("criando o subdiretorio: %v", err)
	}
	for nome, conteudo := range map[string]string{
		"go.mod":        "module sample\n\ngo 1.24\n",
		"cap/sample.go": src,
	} {
		if err := os.WriteFile(filepath.Join(dir, nome), []byte(conteudo), 0o600); err != nil {
			t.Fatalf("escrevendo %s: %v", nome, err)
		}
	}
	a := newAnalysis(dir, "sample", nil, nil)
	pkgs, err := a.load("./...")
	if err != nil {
		t.Fatalf("carregando o corpus: %v", err)
	}
	rep := a.Analyze(pkgs)
	out := map[string]entry{}
	for _, e := range rep.Entries {
		out[e.Key] = e
	}
	return out
}

func TestL1eNoCaminhoRealExigeORotulo(t *testing.T) {
	es := analisaCorpus(t, corpusDelegacao)

	comRotulo, ok := es["cap.M.ComRotulo"]
	if !ok {
		t.Fatal("ComRotulo nao entrou no universo — o corpus nao mediu o que devia")
	}
	if comRotulo.Excluded != "" {
		t.Fatalf("ComRotulo saiu por %s; o corpus precisa dela ELEGIVEL", comRotulo.Excluded)
	}
	if !comRotulo.L1 {
		t.Error("delegacao COM rotulo constante nao foi creditada pelo caminho real")
	}

	semRotulo, ok := es["cap.M.SemRotulo"]
	if !ok {
		t.Fatal("SemRotulo nao entrou no universo")
	}
	if semRotulo.Excluded != "" {
		t.Fatalf("SemRotulo saiu por %s; o corpus precisa dela ELEGIVEL", semRotulo.Excluded)
	}
	if semRotulo.L1 {
		t.Error("creditou delegacao cujo rotulo vem inteiro de fora: o rastro nao nomeia esta funcao")
	}
}
