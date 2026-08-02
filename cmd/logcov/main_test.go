package main

// main_test.go — flags, formatos de saida, determinismo, golden e o dedup de
// blocos do relatorio de cobertura de teste.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
		gotLines := strings.Split(strings.TrimRight(got, "\n"), "\n")
		wantLines := strings.Split(strings.TrimRight(string(want), "\n"), "\n")
		t.Fatalf("golden divergiu: %d linhas geradas, %d versionadas; regenere com\n"+
			"  go run ./cmd/logcov -golden > cmd/logcov/testdata/eligible.golden",
			len(gotLines), len(wantLines))
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
	total := jr.ByForm["port"].CallSites + jr.ByForm["zerolog"].CallSites + jr.ByForm["hlog"].CallSites
	if jr.CallSitesTotal != total {
		t.Fatalf("call_sites_total = %d, soma das formas = %d", jr.CallSitesTotal, total)
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
	for _, line := range strings.Split(string(data), "\n") {
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
