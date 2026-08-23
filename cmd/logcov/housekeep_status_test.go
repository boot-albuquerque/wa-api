package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Testes da F211: o HOUSEKEEP.md tinha deixado de ser consultável por máquina.
//
// Três heurísticas razoáveis davam três respostas incompatíveis para "quantos
// achados estão abertos": 26, 37 e 147. O veredito vivia numa linha
// `**Status**` cuja posição não era previsível, e um bloco podia ter várias —
// a F183 tem quatro cabeçalhos e o veredito dela não é o último `Status` do
// bloco final.
//
// Isso não é queixa de formatação: o ficheiro é a fonte de verdade do que
// falta fazer, e a instrução operacional ("resolver os abertos por gravidade")
// depende de o conseguir consultar.
//
// Este teste vive aqui, e não noutro pacote, pelo mesmo motivo que o
// `.logcov-exclude` está fixado em `rules_test.go`: é onde as convenções do
// repositório que ninguém compila ficam travadas.

const (
	caminhoHousekeep = "../../HOUSEKEEP.md"

	// Os TRÊS valores aceites.
	//
	// Nasceu com dois — `aberto` e `corrigido` — e o comentário dizia que um
	// terceiro "convidaria a um limbo permanente". A prática mostrou o custo
	// oposto: com duas gavetas para três coisas, o que foi DECIDIDO e não será
	// feito não cabe em nenhuma. A F107 ("limitação aceite, não será
	// implementado") e a F135 ("não corrigido, por desenho") ficavam `aberto` e
	// inflavam a contagem de trabalho por fazer; e um worker, sem onde as pôr,
	// empurrou duas outras para `corrigido` — perdendo a distinção entre
	// "consertei" e "decidi não consertar" no próprio registo criado para a
	// preservar.
	//
	// `nao-se-faz` NÃO é limbo: exige veredito explícito, tal como `corrigido`.
	// A diferença entre os dois é o que aconteceu ao código, não o grau de
	// certeza. O que continua a não existir é um valor para "não sei".
	marcaAberto    = "<!-- f-status: aberto -->"
	marcaCorrigido = "<!-- f-status: corrigido -->"
	marcaNaoSeFaz  = "<!-- f-status: nao-se-faz -->"
)

var (
	reCabecalho = regexp.MustCompile(`(?m)^## (F\d+)`)
	reMarca     = regexp.MustCompile(`<!-- f-status: ([\w-]+) -->`)
)

// TestHousekeepTemUmaMarcaPorAchado: cada número de achado tem exatamente uma
// marca, e ela é a última coisa da sua última entrada.
func TestHousekeepTemUmaMarcaPorAchado(t *testing.T) {
	dados, err := os.ReadFile(caminhoHousekeep)
	if err != nil {
		t.Fatalf("não consegui ler o HOUSEKEEP: %v", err)
	}
	texto := string(dados)

	cabecalhos := reCabecalho.FindAllStringSubmatchIndex(texto, -1)
	if len(cabecalhos) == 0 {
		t.Fatal("nenhum cabeçalho ## F<n>: o teste não está a medir nada")
	}

	// O ÚLTIMO bloco de cada número é o que carrega o veredito. Um achado pode
	// ter várias entradas — rediagnóstico é histórico legítimo (a F183 tem
	// quatro) — e o que conta é a última.
	ultimo := map[string][2]int{}
	for i, c := range cabecalhos {
		nome := texto[c[2]:c[3]]
		fim := len(texto)
		if i+1 < len(cabecalhos) {
			fim = cabecalhos[i+1][0]
		}
		ultimo[nome] = [2]int{c[0], fim}
	}

	semMarca, comVarias, valorMau := classificaMarcas(texto, ultimo)

	if len(semMarca) > 0 {
		t.Errorf("%d achado(s) sem marca f-status: %s.\n\nSem ela o veredito "+
			"volta a ser adivinhado a partir de linhas **Status** cuja posição "+
			"não é previsível — foi assim que a mesma pergunta deu 26, 37 e 147 "+
			"(F211).", len(semMarca), strings.Join(semMarca, ", "))
	}
	if len(comVarias) > 0 {
		t.Errorf("%d achado(s) com MAIS de uma marca: %s. Duas marcas são duas "+
			"fontes de verdade, que é o defeito original com outra roupa.",
			len(comVarias), strings.Join(comVarias, ", "))
	}
	if len(valorMau) > 0 {
		t.Errorf("valor(es) fora de {aberto, corrigido, nao-se-faz}: %s", strings.Join(valorMau, ", "))
	}
}

// TestHousekeepMarcaFechaOBloco: a marca é a ÚLTIMA coisa do bloco.
//
// Não é preciosismo de posição: se puder aparecer no meio, volta a haver texto
// de veredito DEPOIS dela — que é exatamente como a F204 ficou a dizer
// "parcialmente corrigido" um dia inteiro depois de estar fechada, porque a
// secção nova entrou por baixo da linha de estado.
func TestHousekeepMarcaFechaOBloco(t *testing.T) {
	dados, err := os.ReadFile(caminhoHousekeep)
	if err != nil {
		t.Fatalf("não consegui ler o HOUSEKEEP: %v", err)
	}
	texto := string(dados)
	cabecalhos := reCabecalho.FindAllStringSubmatchIndex(texto, -1)

	var forasDeSitio []string
	for i, c := range cabecalhos {
		nome := texto[c[2]:c[3]]
		fim := len(texto)
		if i+1 < len(cabecalhos) {
			fim = cabecalhos[i+1][0]
		}
		bloco := strings.TrimSpace(texto[c[0]:fim])
		if bloco == "" {
			continue
		}
		if !strings.HasSuffix(bloco, marcaAberto) && !strings.HasSuffix(bloco, marcaCorrigido) &&
			!strings.HasSuffix(bloco, marcaNaoSeFaz) {
			// Só reclama do último bloco de cada número: os anteriores são
			// histórico e não levam marca.
			if lim, ok := ultimoBlocoDe(texto, cabecalhos, nome); ok && lim == c[0] {
				forasDeSitio = append(forasDeSitio, nome)
			}
		}
	}
	if len(forasDeSitio) > 0 {
		t.Errorf("%d achado(s) cuja marca não fecha o bloco: %s.\n\nCom texto "+
			"DEPOIS da marca, um veredito novo pode entrar por baixo dela e a "+
			"marca fica a mentir — foi assim que a F204 ficou 'parcialmente "+
			"corrigido' um dia depois de fechada.",
			len(forasDeSitio), strings.Join(forasDeSitio, ", "))
	}
}

// classificaMarcas separa os três defeitos possíveis. Está fora do teste
// porque o `gocyclo` do gate conta os ramos do switch como complexidade do
// caso, e um teste que reprova o lint por classificar marcas é ruído — a mesma
// razão que tirou o switch de `TestTodoMetodoComErroTemWrapper`.
func classificaMarcas(texto string, ultimo map[string][2]int) (semMarca, comVarias, valorMau []string) {
	for nome, lim := range ultimo {
		achadas := reMarca.FindAllStringSubmatch(texto[lim[0]:lim[1]], -1)
		switch {
		case len(achadas) == 0:
			semMarca = append(semMarca, nome)
		case len(achadas) > 1:
			comVarias = append(comVarias, nome)
		default:
			if v := achadas[0][1]; v != "aberto" && v != "corrigido" && v != "nao-se-faz" {
				valorMau = append(valorMau, nome+"="+v)
			}
		}
	}
	return semMarca, comVarias, valorMau
}

func ultimoBlocoDe(texto string, cabecalhos [][]int, nome string) (int, bool) {
	inicio, ok := -1, false
	for _, c := range cabecalhos {
		if texto[c[2]:c[3]] == nome {
			inicio, ok = c[0], true
		}
	}
	return inicio, ok
}
