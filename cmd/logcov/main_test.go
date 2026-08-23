package main

// main_test.go — flags, formatos de saida, determinismo, golden e o dedup de
// blocos do relatorio de cobertura de teste.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const profileFixture = "testdata/profile/duplicated.out"

func runTool(t *testing.T, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := run(append([]string{"-root", "../.."}, args...), &buf); err != nil {
		t.Fatalf("run(%v): %v", args, err)
	}
	return buf.String()
}

// TestSaidaDeterministica — duas execucoes produzem bytes identicos (9.5).
func TestSaidaDeterministica(t *testing.T) {
	a := runTool(t, "./pkg")
	b := runTool(t, "./pkg")
	if a != b {
		t.Fatalf("saida nao determinista:\n--- 1 ---\n%s\n--- 2 ---\n%s", a, b)
	}
}

// TestGoldenBate — o golden versionado descreve exatamente o universo atual.
func TestGoldenBate(t *testing.T) {
	got := runTool(t, "-golden")
	want, err := os.ReadFile("testdata/eligible.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		diagnosis := diagnoseGoldenDivergence(string(want), got)
		t.Fatal(diagnosis)
	}
}

// TestFormaPortaResolvida — se `port` vier 0, L1-a esta' quebrada e a fase nao
// fecha (criterio 9.3).
func TestFormaPortaResolvida(t *testing.T) {
	var jr jsonReport
	if err := json.Unmarshal([]byte(runTool(t, "-json")), &jr); err != nil {
		t.Fatal(err)
	}
	if jr.ByForm["port"].CallSites == 0 {
		t.Fatal("port com 0 call sites: L1-a esta' quebrada")
	}
	if jr.ByForm["port"].Files == 0 {
		t.Fatal("port sem arquivos")
	}
	// MUNDO FECHADO: o conjunto de formas reconhecidas e' EXATAMENTE este.
	//
	// A versao anterior somava tres formas a mao e comparava com
	// call_sites_total, e foi assim que ela apanhou as formas novas da decisao
	// 78. Mas isso era efeito colateral, nao asserção: call_sites_total E' a
	// soma das formas (main.go:240), entao somar TODAS dos dois lados nao pode
	// falhar nunca — tautologia. Descobri isso porque o controle negativo nao
	// mordeu.
	//
	// O que o teste vale de facto e' isto: uma forma nova nao entra em silencio.
	// Acrescentar uma exige documenta-la em METRIC.md e reconciliar
	// .log-coverage-baseline, e esta linha e' o que obriga a passagem por aqui.
	formasConhecidas := map[string]bool{
		"port": true, "zerolog": true, "hlog": true, // L1-a, L1-b, L1-c
		"stage": true, "op": true, // L1-d, decisao 78
	}
	for forma := range jr.ByForm {
		if !formasConhecidas[forma] {
			t.Errorf("forma %q nao declarada: documente-a em METRIC.md e "+
				"reconcilie .log-coverage-baseline antes de a acrescentar", forma)
		}
	}
	// E' preciso exigir SITIOS, e nao presenca da chave: as chaves de ByForm
	// sao pre-criadas, entao uma regra que deixasse de casar produziria uma
	// forma com zero sitios e a chave continuaria la'. O controle negativo
	// mostrou isso — desligar L1-d1 passava neste teste.
	for forma := range formasConhecidas {
		if jr.ByForm[forma].CallSites == 0 {
			t.Errorf("forma %q com zero call sites: a regra que a produz "+
				"deixou de casar", forma)
		}
	}

	if jr.Eligible == 0 || jr.Covered == 0 || len(jr.ByPackage) == 0 {
		t.Fatalf("relatorio JSON incompleto: %+v", jr)
	}
	if jr.FuncCovTenths != tenths(jr.Covered, jr.Eligible) {
		t.Fatalf("decimos inconsistentes: %d", jr.FuncCovTenths)
	}
}

// TestBaselineBateComAMedicao — os quatro numeros de .log-coverage-baseline
// sao os MEDIDOS, nao aspiracionais.
func TestBaselineBateComAMedicao(t *testing.T) {
	var jr jsonReport
	if err := json.Unmarshal([]byte(runTool(t, "-json")), &jr); err != nil {
		t.Fatal(err)
	}
	base := readBaselineForTest(t, "../../.log-coverage-baseline")
	checks := map[string]int{
		"min_func_coverage":      jr.FuncCovTenths,
		"min_errpath_coverage":   jr.ErrPathCovTenths,
		"min_eligible":           jr.Eligible,
		"max_exempt_annotations": jr.ExemptAnnotations,
	}
	for k, want := range checks {
		if base[k] != want {
			t.Errorf("%s = %d no baseline, medido %d", k, base[k], want)
		}
	}
	// stage evolui advisory -> ratchet -> floor ao longo das fases F9-F16
	// (ADR-008); o que este teste garante e' que o valor declarado seja um
	// dos tres validos, nao que fique fixo em advisory para sempre.
	switch base["stage_raw"] {
	case stageAdvisory, stageRatchet, stageFloor:
	default:
		t.Errorf("stage invalido ou ausente no baseline: %d", base["stage_raw"])
	}
}

const (
	stageAdvisory = 1
	stageRatchet  = 2
	stageFloor    = 3
)

func readBaselineForTest(t *testing.T, path string) map[string]int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	// A duplicata de uma chave e' ERRO, nao "vale a ultima". Antes do F129
	// este parser sobrescrevia em silencio, e por isso nao viu as duas linhas
	// min_func_coverage= que desativaram o piso do gate em 6fa6270.
	seen := map[string]int{}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch line {
		case "stage=advisory":
			out["stage_raw"] = stageAdvisory
		case "stage=ratchet":
			out["stage_raw"] = stageRatchet
		case "stage=floor":
			out["stage_raw"] = stageFloor
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(k, "#") {
			continue
		}
		if n, err := strconv.Atoi(strings.Fields(v)[0]); err == nil {
			if prev, dup := seen[k]; dup {
				t.Fatalf("chave %q duplicada em %s (linhas %d e %d): chave ambigua desativa o gate em silencio (F129)", k, path, prev, i+1)
			}
			seen[k] = i + 1
			out[k] = n
		}
	}
	return out
}

func TestFormatosDeSaida(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"resumo", nil, "func_coverage"},
		{"por pacote", []string{"-by-package"}, "PACOTE"},
		{"lista descobertos", []string{"-list-uncovered"}, "uncovered:"},
		{"orcamento", []string{"-list-uncovered", "-by-package"}, "# Orcamento de cobertura de log"},
		{"golden", []string{"-golden"}, "\tEXCLUDED\t"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runTool(t, tc.args...); !strings.Contains(got, tc.want) {
				t.Fatalf("saida nao contem %q", tc.want)
			}
		})
	}
}

func TestNormalizePatterns(t *testing.T) {
	for in, want := range map[string]string{
		"":            "./pkg/...",
		"pkg":         "./pkg/...",
		"./pkg":       "./pkg/...",
		"./pkg/...":   "./pkg/...",
		"./pkg/infra": "./pkg/infra/...",
	} {
		var args []string
		if in != "" {
			args = []string{in}
		}
		got := normalizePatterns(args)
		if len(got) != 1 || got[0] != want {
			t.Errorf("normalizePatterns(%q) = %v, quero [%s]", in, got, want)
		}
	}
}

func TestParseFlagsInvalida(t *testing.T) {
	if _, err := parseFlags([]string{"-nao-existe"}); err == nil {
		t.Fatal("flag desconhecida deveria falhar")
	}
	opt, err := parseFlags(nil)
	if err != nil {
		t.Fatal(err)
	}
	if opt.root == "" {
		t.Fatal("root deveria ser resolvido a partir do go.mod")
	}
}

func TestRunErroDePadrao(t *testing.T) {
	var buf bytes.Buffer
	if err := run([]string{"-root", "../..", "./nao/existe"}, &buf); err == nil {
		t.Fatal("padrao inexistente deveria falhar")
	}
}

func TestFindModuleRootForaDoModulo(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := findModuleRoot(); err == nil {
		t.Skip("o diretorio temporario esta' dentro de um modulo Go")
	}
}

// ---------------------------------------------------------------------------
// Dedup de blocos do perfil de cobertura (§0.3a do plano)
// ---------------------------------------------------------------------------

// TestDedupBateComGoToolCover — a prova exigida pelo entregavel 9.3.7: o total
// agregado com dedup e' identico ao de `go tool cover -func | tail -1`.
func TestDedupBateComGoToolCover(t *testing.T) {
	f, err := os.Open(profileFixture)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	blocks, err := parseCoverProfile(f)
	if err != nil {
		t.Fatal(err)
	}
	_, total, covered := aggregateCoverage(restrictToFuncDecls("../..", blocks))
	if got, want := totalFormatado(covered, total), totalDoGoToolCover(t, profileFixture); got != want {
		t.Fatalf("total com dedup = %s, go tool cover -func = %s", got, want)
	}
}

// TestDedupEvitaInflacaoDoDenominador — sem dedup, blocos repetidos por
// -coverpkg=./... multiplicam o denominador.
func TestDedupEvitaInflacaoDoDenominador(t *testing.T) {
	data, err := os.ReadFile(profileFixture)
	if err != nil {
		t.Fatal(err)
	}
	naive := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n")[1:] {
		_, _, blk, ok := parseCoverLine(strings.TrimSpace(line))
		if !ok {
			t.Fatalf("linha invalida: %q", line)
		}
		naive += blk.numStmt
	}
	blocks, err := parseCoverProfile(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	_, total, _ := aggregateCoverage(restrictToFuncDecls("../..", blocks))
	if naive <= total {
		t.Fatalf("a fixture nao tem blocos duplicados: naive=%d dedup=%d", naive, total)
	}
}

func TestWriteCoverageReport(t *testing.T) {
	var buf bytes.Buffer
	if err := writeCoverageReport(profileFixture, &buf); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "PACOTE") || !strings.Contains(got, "total:") {
		t.Fatalf("relatorio incompleto:\n%s", got)
	}
	if err := writeCoverageReport("testdata/profile/nao-existe.out", &buf); err == nil {
		t.Fatal("perfil inexistente deveria falhar")
	}
}

func TestRunCoverProfileViaFlag(t *testing.T) {
	var buf bytes.Buffer
	if err := run([]string{"-root", "../..", "-coverprofile", profileFixture}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "total:") {
		t.Fatal("saida de -coverprofile sem total")
	}
}

func TestParseCoverProfileInvalido(t *testing.T) {
	if _, err := parseCoverProfile(strings.NewReader("mode: set\nlixo\n")); err == nil {
		t.Fatal("linha invalida deveria falhar")
	}
	for _, bad := range []string{"a b", "sem-dois-pontos 1 1", "f.go:1.2,3.4 x 1", "f.go:1.2,3.4 1 x"} {
		if _, _, _, ok := parseCoverLine(bad); ok {
			t.Errorf("parseCoverLine(%q) deveria falhar", bad)
		}
	}
}

func TestMainNaoPanica(t *testing.T) {
	// `main` so' delega para run; o teste garante que o caminho de erro
	// escreve em stderr sem panico.
	var buf bytes.Buffer
	err := run([]string{"-root", "../..", "-coverprofile", "/dev/null/impossivel"}, &buf)
	if err == nil {
		t.Fatal("caminho invalido deveria falhar")
	}
}

// TestBlocosForaDeFuncDeclSaoDescartados — a unica fonte de divergencia entre
// o nosso total e o de `go tool cover -func` sao os blocos que nao caem dentro
// de nenhum *ast.FuncDecl (literais atribuidos a variaveis de pacote). O teste
// monta um perfil sintetico sobre uma fixture que tem um desses e prova que os
// dois totais coincidem.
func TestBlocosForaDeFuncDeclSaoDescartados(t *testing.T) {
	const src = "cmd/logcov/testdata/cases/extra/extra.go"
	lazy := linhaComPrefixo(t, "../../"+src, "var Lazy = func")
	group := linhaComPrefixo(t, "../../"+src, "func Group(")

	profile := "mode: set\n" +
		fmt.Sprintf("wa-api/%s:%d.33,%d.3 4 1\n", src, lazy, lazy+2) +
		fmt.Sprintf("wa-api/%s:%d.24,%d.2 2 1\n", src, group, group+1)
	path := t.TempDir() + "/sintetico.out"
	if err := os.WriteFile(path, []byte(profile), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	blocks, err := parseCoverProfile(f)
	if err != nil {
		t.Fatal(err)
	}
	_, cruTotal, _ := aggregateCoverage(blocks)
	_, total, covered := aggregateCoverage(restrictToFuncDecls("../..", blocks))
	if cruTotal != 6 || total != 2 {
		t.Fatalf("recorte por FuncDecl: cru=%d restrito=%d, quero 6 e 2", cruTotal, total)
	}

	if got, want := totalFormatado(covered, total), totalDoGoToolCover(t, path); got != want {
		t.Fatalf("total restrito = %s, go tool cover -func = %s", got, want)
	}
}

// linhaComPrefixo devolve a linha (1-based) da primeira linha do arquivo que
// comeca com o prefixo dado.
func linhaComPrefixo(t *testing.T, path, prefixo string) int {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(l, prefixo) {
			return i + 1
		}
	}
	t.Fatalf("fixture mudou: %q nao encontrado em %s", prefixo, path)
	return 0
}

// totalDoGoToolCover devolve o percentual da ultima linha de `go tool cover
// -func`, que e' a referencia do criterio 9.10.
func totalDoGoToolCover(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command("go", "tool", "cover", "-func="+path).Output()
	if err != nil {
		t.Fatalf("go tool cover: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	last := lines[len(lines)-1]
	return last[strings.LastIndex(last, "\t")+1:]
}

func totalFormatado(covered, total int) string {
	return strconv.FormatFloat(pct(covered, total), 'f', 1, 64) + "%"
}

func TestRestrictToFuncDeclsArquivoAusente(t *testing.T) {
	in := map[string]map[string]coverBlock{
		"wa-api/nao/existe.go": {"wa-api/nao/existe.go:1.1,2.1": {numStmt: 3, count: 1}},
	}
	out := restrictToFuncDecls("../..", in)
	if len(out) != 1 {
		t.Fatalf("arquivo ilegivel deveria passar intacto, veio %v", out)
	}
	if blockLine("sem-formato") != 0 {
		t.Error("chave malformada deveria dar linha 0")
	}
}

func TestBlockLineMalformada(t *testing.T) {
	for _, bad := range []string{"sem-dois-pontos", "f.go:semponto", "f.go:x.1,2.3"} {
		if got := blockLine(bad); got != 0 {
			t.Errorf("blockLine(%q) = %d, quero 0", bad, got)
		}
	}
	if got := blockLine("f.go:12.1,15.2"); got != 12 {
		t.Errorf("blockLine = %d, quero 12", got)
	}
}

// ---------------------------------------------------------------------------
// Golden set comparison (F154)
// ---------------------------------------------------------------------------

const (
	goldenFieldSep  = "\t"
	goldenRegenCmd  = "go run ./cmd/logcov -golden > cmd/logcov/testdata/eligible.golden"
	diagPositionFmt = "golden diverged: set is IDENTICAL (same %d entries with same status) " +
		"but line positions changed; regeneration is safe:\n  %s"
	diagSetChangedHeader = "golden diverged: the eligible SET changed"
)

type goldenEntry struct {
	name   string
	status string
}

func parseGoldenIdentities(raw string) map[goldenEntry]struct{} {
	set := map[goldenEntry]struct{}{}
	for _, line := range strings.Split(strings.TrimRight(raw, "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, goldenFieldSep, 3)
		if len(parts) < 2 {
			continue
		}
		set[goldenEntry{name: parts[0], status: parts[1]}] = struct{}{}
	}
	return set
}

func diagnoseGoldenDivergence(versioned, generated string) string {
	wantSet := parseGoldenIdentities(versioned)
	gotSet := parseGoldenIdentities(generated)

	var added, removed []string
	var statusChanged []string

	wantByName := map[string]string{}
	for e := range wantSet {
		wantByName[e.name] = e.status
	}
	gotByName := map[string]string{}
	for e := range gotSet {
		gotByName[e.name] = e.status
	}

	for e := range gotSet {
		if _, ok := wantSet[e]; !ok {
			if oldStatus, existed := wantByName[e.name]; existed {
				statusChanged = append(statusChanged, e.name+": "+oldStatus+" -> "+e.status)
			} else {
				added = append(added, e.name+" ("+e.status+")")
			}
		}
	}
	for e := range wantSet {
		if _, ok := gotSet[e]; !ok {
			if _, stillExists := gotByName[e.name]; !stillExists {
				removed = append(removed, e.name+" ("+e.status+")")
			}
		}
	}

	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(statusChanged)

	if len(added) == 0 && len(removed) == 0 && len(statusChanged) == 0 {
		return fmt.Sprintf(diagPositionFmt, len(wantSet), goldenRegenCmd)
	}

	var b strings.Builder
	b.WriteString(diagSetChangedHeader)
	if len(added) > 0 {
		b.WriteString("\n\nADDED:\n")
		for _, a := range added {
			b.WriteString("  + ")
			b.WriteString(a)
			b.WriteByte('\n')
		}
	}
	if len(removed) > 0 {
		b.WriteString("\nREMOVED:\n")
		for _, r := range removed {
			b.WriteString("  - ")
			b.WriteString(r)
			b.WriteByte('\n')
		}
	}
	if len(statusChanged) > 0 {
		b.WriteString("\nSTATUS CHANGED:\n")
		for _, s := range statusChanged {
			b.WriteString("  ~ ")
			b.WriteString(s)
			b.WriteByte('\n')
		}
	}
	b.WriteString("\nregenerate ONLY after verifying these changes are intentional:\n  ")
	b.WriteString(goldenRegenCmd)
	return b.String()
}

func TestDiagnoseGoldenPositionOnly(t *testing.T) {
	versioned := "foo.Bar\tELIGIBLE\tuncovered:L42\nfoo.Baz\tEXCLUDED\tX5\n"
	generated := "foo.Bar\tELIGIBLE\tuncovered:L99\nfoo.Baz\tEXCLUDED\tX5\n"

	diag := diagnoseGoldenDivergence(versioned, generated)
	if !strings.Contains(diag, "IDENTICAL") {
		t.Fatalf("expected position-only diagnosis, got:\n%s", diag)
	}
	if !strings.Contains(diag, goldenRegenCmd) {
		t.Fatalf("missing regen command in diagnosis:\n%s", diag)
	}
	for i := 0; i < 5; i++ {
		d := diagnoseGoldenDivergence(versioned, generated)
		if d != diag {
			t.Fatalf("non-deterministic output on iteration %d", i)
		}
	}
}

func TestDiagnoseGoldenEntryAdded(t *testing.T) {
	versioned := "foo.Bar\tELIGIBLE\tuncovered:L42\n"
	generated := "foo.Bar\tELIGIBLE\tuncovered:L42\nfoo.New\tELIGIBLE\tuncovered:L10\n"

	diag := diagnoseGoldenDivergence(versioned, generated)
	if !strings.Contains(diag, diagSetChangedHeader) {
		t.Fatalf("expected set-changed diagnosis, got:\n%s", diag)
	}
	if !strings.Contains(diag, "foo.New") {
		t.Fatalf("added entry not named in diagnosis:\n%s", diag)
	}
	if !strings.Contains(diag, "ADDED") {
		t.Fatalf("missing ADDED section:\n%s", diag)
	}
	for i := 0; i < 5; i++ {
		d := diagnoseGoldenDivergence(versioned, generated)
		if d != diag {
			t.Fatalf("non-deterministic output on iteration %d", i)
		}
	}
}

func TestDiagnoseGoldenEntryRemoved(t *testing.T) {
	versioned := "foo.Bar\tELIGIBLE\tuncovered:L42\nfoo.Old\tEXCLUDED\tX5\n"
	generated := "foo.Bar\tELIGIBLE\tuncovered:L42\n"

	diag := diagnoseGoldenDivergence(versioned, generated)
	if !strings.Contains(diag, diagSetChangedHeader) {
		t.Fatalf("expected set-changed diagnosis, got:\n%s", diag)
	}
	if !strings.Contains(diag, "foo.Old") {
		t.Fatalf("removed entry not named in diagnosis:\n%s", diag)
	}
	if !strings.Contains(diag, "REMOVED") {
		t.Fatalf("missing REMOVED section:\n%s", diag)
	}
	for i := 0; i < 5; i++ {
		d := diagnoseGoldenDivergence(versioned, generated)
		if d != diag {
			t.Fatalf("non-deterministic output on iteration %d", i)
		}
	}
}

func TestDiagnoseGoldenStatusChanged(t *testing.T) {
	versioned := "foo.Bar\tELIGIBLE\tuncovered:L42\n"
	generated := "foo.Bar\tEXCLUDED\tX5\n"

	diag := diagnoseGoldenDivergence(versioned, generated)
	if !strings.Contains(diag, diagSetChangedHeader) {
		t.Fatalf("expected set-changed diagnosis, got:\n%s", diag)
	}
	if !strings.Contains(diag, "STATUS CHANGED") {
		t.Fatalf("missing STATUS CHANGED section:\n%s", diag)
	}
	if !strings.Contains(diag, "ELIGIBLE -> EXCLUDED") {
		t.Fatalf("status transition not shown:\n%s", diag)
	}
}

func TestDiagnoseGoldenMixedChanges(t *testing.T) {
	versioned := "a.A\tELIGIBLE\tL1\nb.B\tEXCLUDED\tX1\nc.C\tELIGIBLE\tL3\n"
	generated := "a.A\tELIGIBLE\tL1\nc.C\tEXCLUDED\tX9\nd.D\tELIGIBLE\tL5\n"

	diag := diagnoseGoldenDivergence(versioned, generated)
	if !strings.Contains(diag, "ADDED") || !strings.Contains(diag, "d.D") {
		t.Fatalf("missing ADDED d.D:\n%s", diag)
	}
	if !strings.Contains(diag, "REMOVED") || !strings.Contains(diag, "b.B") {
		t.Fatalf("missing REMOVED b.B:\n%s", diag)
	}
	if !strings.Contains(diag, "STATUS CHANGED") || !strings.Contains(diag, "c.C") {
		t.Fatalf("missing STATUS CHANGED c.C:\n%s", diag)
	}
	for i := 0; i < 10; i++ {
		d := diagnoseGoldenDivergence(versioned, generated)
		if d != diag {
			t.Fatalf("non-deterministic output on iteration %d", i)
		}
	}
}
