package bootstrap

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"wa-api/pkg/presentation/http/apidocs"
)

// Este ficheiro é o GATE da documentação OpenAPI.
//
// POR QUE UM GATE E NÃO UMA REVISÃO. A cobertura de documentação é
// exactamente o tipo de coisa que fica a 100% no dia em que é escrita e a 80%
// três funcionalidades depois, porque acrescentar uma rota e esquecer o YAML
// não parte nada. Aqui parte.
//
// O que se afirma:
//   1. toda rota registada existe na especificação, com o método certo;
//   2. a especificação não inventa rotas que o router não serve;
//   3. toda operação tem etiqueta, título, descrição e respostas;
//   4. toda etiqueta usada está declarada, com descrição;
//   5. o documento embutido no binário está em dia com as fontes.

const (
	openapiSpecRoot   = "../../api/openapi"
	openapiCmdPackage = "../../cmd/openapidoc"
)

// operacao é uma operação da especificação, achatada para asserção.
type operacao struct {
	metodo    string
	caminho   string
	tags      []string
	resumo    string
	descricao string
	respostas []string
}

// carregarEspecificacao lê o documento EMBUTIDO, e não o ficheiro do disco.
//
// A distinção importa: é o embutido que o servidor entrega, e um teste que
// leia o ficheiro fonte passaria mesmo que o binário levasse uma versão
// antiga. TestOpenAPIGeradoEstaAtualizado é que garante que os dois coincidem.
func carregarEspecificacao(t *testing.T) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal(apidocs.Specification(), &doc); err != nil {
		t.Fatalf("especificação embutida não é YAML válido: %v", err)
	}
	return doc
}

func operacoesDaEspecificacao(t *testing.T, doc map[string]any) map[string]operacao {
	t.Helper()
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("especificação sem secção paths")
	}
	out := map[string]operacao{}
	for caminho, item := range paths {
		metodos, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("caminho %q não é um mapa de métodos", caminho)
		}
		for metodo, corpo := range metodos {
			op, ok := corpo.(map[string]any)
			if !ok {
				continue
			}
			registo := operacao{metodo: strings.ToUpper(metodo), caminho: caminho}
			if tags, ok := op["tags"].([]any); ok {
				for _, tag := range tags {
					registo.tags = append(registo.tags, toString(tag))
				}
			}
			registo.resumo = toString(op["summary"])
			registo.descricao = toString(op["description"])
			if respostas, ok := op["responses"].(map[string]any); ok {
				for codigo := range respostas {
					registo.respostas = append(registo.respostas, codigo)
				}
			}
			out[registo.metodo+" "+caminho] = registo
		}
	}
	return out
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

// TestOpenAPICobreTodasAsRotasRegistadas é o critério de aceite da tarefa,
// escrito como asserção em vez de como afirmação num relatório.
func TestOpenAPICobreTodasAsRotasRegistadas(t *testing.T) {
	registadas := rotasRegistadas(t)
	documentadas := operacoesDaEspecificacao(t, carregarEspecificacao(t))

	// Uma rota LEGADA não é documentada de propósito: o contrato descreve um
	// nome por operação, e o nome documentado é o canónico. Ela conta como
	// coberta quando a gémea canónica está no documento — o que também
	// significa que apagar a canónica faz o gate acusar as DUAS.
	canonicaDe := map[string]string{}
	for _, linha := range CaminhosCanonicos() {
		canonicaDe[linha.LegacyMethod+" "+linha.LegacyPath] =
			linha.CanonicalMethod + " " + linha.CanonicalPath
	}

	// A consolidação CAP-10 das cinco rotas de download por-kind numa única
	// canónica (/chats/download/{kind}) foi REVERTIDA em 2026-08-27
	// (HOUSEKEEP.md F297, corte-limpo explícito): as cinco rotas legadas
	// deixaram de existir, não apenas de ser documentadas, então já não
	// precisam de exceção aqui — `registadas` nunca mais as inclui.

	var faltam []string
	for _, chave := range registadas {
		if _, ok := documentadas[chave]; ok {
			continue
		}
		if canonica, ehLegada := canonicaDe[chave]; ehLegada {
			if _, ok := documentadas[canonica]; ok {
				continue
			}
			faltam = append(faltam, chave+" (legada) e "+canonica+" (canónica): NENHUMA documentada")
			continue
		}
		faltam = append(faltam, chave)
	}
	sort.Strings(faltam)
	if len(faltam) > 0 {
		t.Errorf("%d rotas registadas SEM entrada na especificação OpenAPI.\n"+
			"Acrescente-as em api/openapi/paths/<grupo>.yaml e corra `go run ./cmd/openapidoc`:\n  %s",
			len(faltam), strings.Join(faltam, "\n  "))
	}
}

// TestOpenAPINaoInventaRotas é o simétrico do teste acima, e existe porque a
// documentação errada por excesso é mais perigosa que a errada por falta: quem
// lê uma rota que não existe escreve um cliente que falha em produção, e não
// tem como saber que a culpa é do documento.
func TestOpenAPINaoInventaRotas(t *testing.T) {
	registadas := map[string]bool{}
	for _, chave := range rotasRegistadas(t) {
		registadas[chave] = true
	}
	var sobram []string
	for chave := range operacoesDaEspecificacao(t, carregarEspecificacao(t)) {
		if !registadas[chave] {
			sobram = append(sobram, chave)
		}
	}
	sort.Strings(sobram)
	if len(sobram) > 0 {
		t.Errorf("%d operações documentadas que o router NÃO serve:\n  %s",
			len(sobram), strings.Join(sobram, "\n  "))
	}
}

// TestOpenAPIOperacoesEstaoCompletas afirma o mínimo que torna a página útil a
// quem nunca viu o código.
func TestOpenAPIOperacoesEstaoCompletas(t *testing.T) {
	doc := carregarEspecificacao(t)
	declaradas := map[string]bool{}
	if tags, ok := doc["tags"].([]any); ok {
		for _, raw := range tags {
			tag, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			nome := toString(tag["name"])
			if strings.TrimSpace(toString(tag["description"])) == "" {
				t.Errorf("etiqueta %q sem descrição: um grupo sem explicação não orienta ninguém", nome)
			}
			declaradas[nome] = true
		}
	}

	for chave, op := range operacoesDaEspecificacao(t, doc) {
		switch {
		case len(op.tags) == 0:
			t.Errorf("%s: sem etiqueta — ficaria fora de todos os grupos da página", chave)
		default:
			for _, tag := range op.tags {
				if !declaradas[tag] {
					t.Errorf("%s: usa a etiqueta %q, que não está declarada em tags", chave, tag)
				}
			}
		}
		if strings.TrimSpace(op.resumo) == "" {
			t.Errorf("%s: sem summary", chave)
		}
		if len(strings.TrimSpace(op.descricao)) < minimoDescricao {
			t.Errorf("%s: description com menos de %d caracteres (%d) — "+
				"repetir o nome da rota não é descrever",
				chave, minimoDescricao, len(strings.TrimSpace(op.descricao)))
		}
		if len(op.respostas) == 0 {
			t.Errorf("%s: sem responses", chave)
		}
	}
}

// rotasRegistadas enumera as rotas como "MÉTODO /caminho", a partir da MESMA
// função que serve o processo — Routes(Deps{}) — e não de uma lista à parte.
//
// É de propósito: a lista à parte é a que fica desatualizada em silêncio, e
// então o gate afirma cobertura sobre um universo que já não é o real. Também
// é a fonte que `cmd/listroutes` usa, logo o número que este teste conta é o
// mesmo que um operador obtém na linha de comandos.
//
// As rotas do painel de desenvolvimento (devui) e a própria página de
// documentação ficam de fora: a primeira só existe quando o modo de
// desenvolvimento está ligado, e a segunda é a página que serve a
// especificação — documentar-se a si mesma não ajudaria ninguém.
func rotasRegistadas(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, rota := range Routes(Deps{}) {
		if strings.HasPrefix(rota.Path, apidocs.BasePath) || strings.Contains(rota.Path, "{rest:") {
			continue
		}
		for _, metodo := range rota.Methods {
			out = append(out, metodo+" "+rota.Path)
		}
	}
	sort.Strings(out)
	if len(out) == 0 {
		t.Fatal("nenhuma rota registada: o gate estaria a afirmar cobertura sobre o vazio")
	}
	return out
}

// segredosReaisDesteAmbiente são valores que existem MESMO, e que por isso
// nunca podem aparecer na especificação.
//
// POR QUE ESTA LISTA EXISTE. Ao documentar `POST /admin/users` alguém colou o
// token de sessão real do ambiente de testes no exemplo — funcionava, era
// realista, e ficou. A especificação é servida sem autenticação a quem abrir
// `/docs`: um segredo ali é um segredo publicado. O exemplo tem de ser
// obviamente falso, e é isso que este teste obriga.
var segredosReaisDesteAmbiente = []string{
	"tok_fila",
	"tok_lucas",
}

// TestOpenAPINaoTrazSegredoReal recusa a especificação que carregue uma
// credencial verdadeira.
func TestOpenAPINaoTrazSegredoReal(t *testing.T) {
	documento := string(apidocs.Specification())
	for _, segredo := range segredosReaisDesteAmbiente {
		if strings.Contains(documento, segredo) {
			t.Errorf("a especificação contém %q, que é uma credencial real deste ambiente. "+
				"A página /docs é servida sem autenticação — use um valor obviamente falso.", segredo)
		}
	}
}

// marcasDeEvidencia são os quatro símbolos que podem abrir um summary.
//
// Estão aqui, e não só na tabela, porque um símbolo novo tem de ser uma
// decisão: acrescentar um quinto valor sem o declarar faria a página mostrar
// uma classificação que ninguém definiu.
var marcasDeEvidencia = []string{"✅", "🟡", "❌", "⬜"}

// TestOpenAPISummariesTrazemMarcaDeEvidencia trava a propriedade que torna a
// página honesta à primeira vista: o leitor vê, no título, quanta prova existe
// de que aquela rota funciona.
//
// Sem este teste, uma rota nova entraria sem marca e leria-se como as outras —
// e "não medida" passaria por "medida" só por não se distinguir.
func TestOpenAPISummariesTrazemMarcaDeEvidencia(t *testing.T) {
	contagem := map[string]int{}
	for chave, op := range operacoesDaEspecificacao(t, carregarEspecificacao(t)) {
		marca := ""
		for _, candidata := range marcasDeEvidencia {
			if strings.HasPrefix(op.resumo, candidata) {
				marca = candidata
				break
			}
		}
		if marca == "" {
			t.Errorf("%s: summary %q não começa por marca de evidência (%s) — "+
				"o gerador aplica-a a partir de api/openapi/evidencias.tsv",
				chave, op.resumo, strings.Join(marcasDeEvidencia, " "))
			continue
		}
		contagem[marca]++
		if resto := strings.TrimSpace(strings.TrimPrefix(op.resumo, marca)); resto == "" {
			t.Errorf("%s: summary é só a marca, sem título", chave)
		}
	}
	// O total tem de fechar com o número de operações: uma marca contada a
	// menos significaria uma operação sem classificação a passar despercebida.
	total := 0
	for _, n := range contagem {
		total += n
	}
	if esperado := len(operacoesDaEspecificacao(t, carregarEspecificacao(t))); total != esperado {
		t.Errorf("marcas contadas = %d, operações = %d", total, esperado)
	}
	t.Logf("evidência: ✅%d 🟡%d ❌%d ⬜%d",
		contagem["✅"], contagem["🟡"], contagem["❌"], contagem["⬜"])
}

// minimoDescricao é o piso abaixo do qual uma descrição é, na prática, o nome
// da rota outra vez. Não mede qualidade — mede que alguém escreveu.
const minimoDescricao = 80

// TestOpenAPIGeradoEstaAtualizado recusa um documento embutido que já não
// corresponde às fontes.
func TestOpenAPIGeradoEstaAtualizado(t *testing.T) {
	if _, err := os.Stat(openapiSpecRoot); err != nil {
		t.Skipf("fontes da especificação ausentes: %v", err)
	}
	raiz, err := filepath.Abs(openapiSpecRoot)
	if err != nil {
		t.Fatal(err)
	}
	saida, err := filepath.Abs("../presentation/http/apidocs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", openapiCmdPackage, "-root", raiz, "-out", saida, "-check")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("o documento embutido está desatualizado face às fontes.\n"+
			"Corra `go run ./cmd/openapidoc` e commite o resultado.\n%s", out)
	}
}
