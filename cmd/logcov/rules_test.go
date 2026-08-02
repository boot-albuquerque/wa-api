package main

// rules_test.go — a ferramenta que julga o repositorio e' ela propria julgada.
// Um par positivo/negativo por regra, table-driven contra testdata/cases.

import (
	"strings"
	"testing"
)

const casesPrefix = "cmd/logcov/testdata/cases/"

// analyzeCase carrega um pacote de fixture e devolve as entradas por chave.
func analyzeCase(t *testing.T, name string, excludes []string) map[string]entry {
	t.Helper()
	a := newAnalysis("../..", "wa-api", excludes, nil)
	pkgs, err := a.load("./" + casesPrefix + name)
	if err != nil {
		t.Fatalf("load(%s): %v", name, err)
	}
	rep := a.Analyze(pkgs)
	out := make(map[string]entry, len(rep.Entries))
	for _, e := range rep.Entries {
		out[e.Key] = e
	}
	return out
}

func lookup(t *testing.T, m map[string]entry, key string) entry {
	t.Helper()
	e, ok := m[key]
	if !ok {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		t.Fatalf("entrada %q nao encontrada; existentes: %v", key, keys)
	}
	return e
}

type ruleCase struct {
	name     string
	fixture  string
	fn       string // sufixo da chave, sem o prefixo do pacote
	excluded string // "" quando se espera ELIGIBLE
	detail   string // substring esperada do <detalhe>
	excludes []string
}

// TestRuleTable cobre os 20 pares minimos da secao 9.3.2.
func TestRuleTable(t *testing.T) {
	x5pkg := []string{casesPrefix + "x5excluded/"}

	cases := []ruleCase{
		// --- X1..X8: um par por regra ---
		{name: "X1 positivo", fixture: "rulesx", fn: "Box.X1Pos", excluded: "X1"},
		{name: "X1 negativo", fixture: "rulesx", fn: "Box.X1Neg"},
		{name: "X2 positivo", fixture: "rulesx", fn: "Box.X2Pos", excluded: "X2"},
		{name: "X2 negativo", fixture: "rulesx", fn: "Box.X2Neg"},
		{name: "X3 positivo", fixture: "rulesx", fn: "init", excluded: "X3"},
		{name: "X3 negativo", fixture: "rulesx", fn: "NotInit"},
		{name: "X4 positivo", fixture: "rulesx", fn: "Box.String", excluded: "X4"},
		{name: "X4 negativo", fixture: "rulesx", fn: "Box.Stringy"},
		{name: "X5 positivo", fixture: "x5excluded", fn: "Alfa", excluded: "X5", excludes: x5pkg},
		{name: "X5 negativo", fixture: "x5excluded", fn: "Alfa"},
		{name: "X6 positivo", fixture: "x6", fn: "SortSlice.func1", excluded: "X6"},
		{name: "X6 negativo", fixture: "x6", fn: "GoRoutine.func1"},
		{name: "X7 positivo", fixture: "rulesx", fn: "Box.X7Pos", excluded: "X7"},
		{name: "X7 negativo", fixture: "rulesx", fn: "Box.X7Neg"},
		{name: "X8 positivo", fixture: "rulesx", fn: "Box.X8Pos", excluded: "X8", detail: "fixture da regra X8"},
		{name: "X8 negativo", fixture: "rulesx", fn: "Box.X8Neg"},

		// --- L1-a/b/c: um par por forma ---
		{name: "L1-a positivo", fixture: "l1", fn: "Svc.PortPos", detail: "covered"},
		{name: "L1-a negativo", fixture: "l1", fn: "Svc.PortNeg", detail: "uncovered:L1"},
		{name: "L1-b positivo", fixture: "l1", fn: "ZerologPos", detail: "covered"},
		{name: "L1-b negativo", fixture: "l1", fn: "ZerologNeg", detail: "uncovered:L1"},
		{name: "L1-c positivo", fixture: "l1", fn: "HlogPos", detail: "covered"},
		{name: "L1-c negativo", fixture: "l1", fn: "HlogNeg", detail: "uncovered:L1"},

		// --- L2: S-ret, S-http, S-consume, (a), (b) valida, (b) descarte ---
		{name: "L2 S-ret descoberto", fixture: "l2", fn: "SRetUncovered", detail: "uncovered:L2(S-ret"},
		{name: "L2 S-ret coberto por (b)", fixture: "l2", fn: "SRetPropagateWrap", detail: "uncovered:L1"},
		{name: "L2 S-http descoberto", fixture: "l2", fn: "SHTTPUncovered", detail: "uncovered:L2(S-http"},
		{name: "L2 S-http negativo (<400)", fixture: "l2", fn: "SHTTPOK", detail: "uncovered:L1"},
		{name: "L2 S-consume descoberto", fixture: "l2", fn: "SConsumeUncovered", detail: "uncovered:L2(S-consume"},
		{name: "L2 S-consume negativo", fixture: "l2", fn: "SConsumeNeg", detail: "uncovered:L1"},
		{name: "L2 (a) cobre", fixture: "l2", fn: "SRetCoveredByLog", detail: "covered"},
		{name: "L2 (a) Info nao cobre", fixture: "l2", fn: "SRetInfoDoesNotCover", detail: "uncovered:L2(S-ret"},
		{name: "L2 (b) valida bare", fixture: "l2", fn: "SRetPropagateBare", detail: "uncovered:L1"},
		{name: "L2 (b) valida apperr", fixture: "l2", fn: "SRetPropagateAppErr", detail: "uncovered:L1"},
		{name: "L2 (b) descarte apperr", fixture: "l2", fn: "SRetDiscardAppErr", detail: "uncovered:L2(S-ret"},
		{name: "L2 S-http envelope", fixture: "l2", fn: "SHTTPEnvelope", detail: "uncovered:L2(S-http"},
		{name: "L2 S-consume coberto", fixture: "l2", fn: "SConsumeCovered", detail: "covered"},

		// --- L3: porta, zerolog, keyvals impar ---
		{name: "L3 porta positivo", fixture: "l3", fn: "Svc.PortPos", detail: "covered"},
		{name: "L3 porta negativo", fixture: "l3", fn: "Svc.PortNeg", detail: "uncovered:L3(port"},
		{name: "L3 zerolog positivo", fixture: "l3", fn: "ZerologPos", detail: "covered"},
		{name: "L3 zerolog negativo", fixture: "l3", fn: "ZerologNeg", detail: "uncovered:L3(zerolog"},
		{name: "L3 keyvals impar", fixture: "l3", fn: "Svc.PortOdd", detail: "uncovered:L3(port"},
		{name: "L3 uma violacao reprova a funcao", fixture: "l3", fn: "Svc.PortMixed", detail: "uncovered:L3(port"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entries := analyzeCase(t, tc.fixture, tc.excludes)
			e := lookup(t, entries, casesPrefix+tc.fixture+"."+tc.fn)
			if e.Excluded != tc.excluded {
				t.Fatalf("Excluded = %q, quero %q (detalhe: %s)", e.Excluded, tc.excluded, e.status())
			}
			if tc.detail != "" && !strings.Contains(e.status(), tc.detail) {
				t.Fatalf("status = %q, quero conter %q", e.status(), tc.detail)
			}
		})
	}
}

// TestX6QualifiedShapes — as demais formas qualificadas de X6.
func TestX6QualifiedShapes(t *testing.T) {
	entries := analyzeCase(t, "x6", nil)
	for _, key := range []string{"Deferred.func1", "Handler.func1", "Middleware.func1"} {
		e := lookup(t, entries, casesPrefix+"x6."+key)
		if e.Excluded != "" {
			t.Errorf("%s: Excluded = %q, quero elegivel", key, e.Excluded)
		}
	}
}

// TestX6FuncLitEmVarDePacote — R1: um *ast.FuncLit dentro de uma `var` de
// nivel de pacote e' visivel a metrica e recebe a MESMA promocao X6 usada
// dentro de corpos de funcao. Sem a correcao, nenhuma das duas entradas
// abaixo existiria: o laco de descoberta so' percorria *ast.FuncDecl.
func TestX6FuncLitEmVarDePacote(t *testing.T) {
	entries := analyzeCase(t, "x6pkgvar", nil)
	if e := lookup(t, entries, casesPrefix+"x6pkgvar.Once.func1"); e.Excluded != "" {
		t.Errorf("Once.func1: Excluded = %q, quero elegivel (sync.OnceValue promove por X6)", e.Excluded)
	}
	if e := lookup(t, entries, casesPrefix+"x6pkgvar.Plain.func1"); e.Excluded != "X6" {
		t.Errorf("Plain.func1: Excluded = %q, quero X6", e.Excluded)
	}
}

// TestL1bRestritaPorDiretorio — nos diretorios da regra depguard
// camada-de-aplicacao, L1-b nao e' forma valida (criterio 9.6).
func TestL1bRestritaPorDiretorio(t *testing.T) {
	dirs, err := readDepguardDirs("../../.golangci.yml")
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("nenhum diretorio lido da regra depguard/camada-de-aplicacao")
	}
	a := &analysis{depguardDirs: dirs}
	if ruleL1bValidHere(a, "pkg/application/usecase/chat/archive_chat.go") {
		t.Error("L1-b deveria ser invalida em pkg/application/usecase/chat")
	}
	if !ruleL1bValidHere(a, "pkg/infra/messaging/rabbitmq.go") {
		t.Error("L1-b deveria ser valida em pkg/infra/messaging")
	}
}

// TestZerologSuprimidoNaCamadaDeAplicacao — o cross-check do criterio 9.6
// exercitado sobre a fixture: a mesma cadeia zerolog conta fora da camada de
// aplicacao e nao conta dentro dela.
func TestZerologSuprimidoNaCamadaDeAplicacao(t *testing.T) {
	a := newAnalysis("../..", "wa-api", nil, []string{casesPrefix + "l1"})
	pkgs, err := a.load("./" + casesPrefix + "l1")
	if err != nil {
		t.Fatal(err)
	}
	rep := a.Analyze(pkgs)
	// packages.Load com NeedDeps traz o fecho transitivo de dependencias
	// junto com o pacote pedido — a asserção sobre call sites de zerolog
	// tem de ficar restrita as entradas do proprio fixture l1, senao
	// zerolog legitimo em qualquer dependencia real (ex.: pkg/domain) conta
	// aqui tambem, sem nenhuma relacao com a supressao por diretorio sendo
	// testada.
	zerologSitesEmL1 := 0
	for _, e := range rep.Entries {
		if !strings.HasPrefix(e.Key, casesPrefix+"l1.") {
			continue
		}
		if e.Key == casesPrefix+"l1.ZerologPos" && e.L1 {
			t.Fatal("ZerologPos deveria ficar sem cobertura: L1-b suprimida neste diretorio")
		}
		for _, s := range e.Sites {
			if s.form == "zerolog" {
				zerologSitesEmL1++
			}
		}
	}
	if zerologSitesEmL1 != 0 {
		t.Fatalf("call sites zerolog no fixture l1 = %d, quero 0 no diretorio restrito", zerologSitesEmL1)
	}
}

func TestRuleX8ExemptParsing(t *testing.T) {
	if ok, _ := ruleX8Exempt(nil); ok {
		t.Error("doc nil nao pode ser isencao")
	}
}

func TestReadExcludeFile(t *testing.T) {
	got, err := readExcludeFile("../../.logcov-exclude")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "cmd/" {
		t.Fatalf("excludes = %v, quero [cmd/]", got)
	}
	missing, err := readExcludeFile("../../.logcov-exclude-inexistente")
	if err != nil || missing != nil {
		t.Fatalf("arquivo ausente deveria dar (nil, nil), deu (%v, %v)", missing, err)
	}
}

func TestReadDepguardDirsAusente(t *testing.T) {
	got, err := readDepguardDirs("../../.golangci-inexistente.yml")
	if err != nil || got != nil {
		t.Fatalf("arquivo ausente deveria dar (nil, nil), deu (%v, %v)", got, err)
	}
}

func TestCountExemptAnnotations(t *testing.T) {
	n, err := countExemptAnnotations("../../pkg")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Logf("//log:exempt em pkg/: %d", n)
	}
	zero, err := countExemptAnnotations("../../diretorio-inexistente")
	if err != nil || zero != 0 {
		t.Fatalf("diretorio ausente deveria dar (0, nil), deu (%d, %v)", zero, err)
	}
}

func TestTenthsEPct(t *testing.T) {
	if got := tenths(0, 0); got != 0 {
		t.Errorf("tenths(0,0) = %d", got)
	}
	if got := tenths(1, 2); got != 500 {
		t.Errorf("tenths(1,2) = %d, quero 500", got)
	}
	if got := pct(0, 0); got != 0 {
		t.Errorf("pct(0,0) = %v", got)
	}
	if got := pct(1, 4); got != 25 {
		t.Errorf("pct(1,4) = %v, quero 25", got)
	}
}

func TestTypeExprNameFallback(t *testing.T) {
	if got := typeExprName(nil); got != "?" {
		t.Errorf("typeExprName(nil) = %q, quero ?", got)
	}
}
