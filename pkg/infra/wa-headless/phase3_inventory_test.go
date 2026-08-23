package waheadless

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// O inventário da Fase 3 (decisão 81).
//
// O critério de encerramento é: cada port de TRANSPORTE satisfeito, ou recusado
// com motivo escrito. Este teste é o que torna isso verificável em vez de
// narrado — e ele DESCOBRE os ports no código em vez de os repetir, porque uma
// lista escrita à mão só prova que alguém a escreveu à mão.
//
// Um port de transporte é o que carrega `txtID string`: esse é o endereçamento
// de sessão, e é o que distingue "operar sobre uma sessão" de infraestrutura
// como Storage, Logger ou UserRepository.

type portStatus struct {
	// satisfeito diz que existe adaptador headless com asserção em tempo de
	// compilação. O compilador é a prova; esta tabela é só o índice.
	satisfeito bool
	// motivo é obrigatório quando NÃO satisfeito. Recusa sem motivo escrito é
	// exatamente a "diferença silenciosa entre dois adaptadores" que o doc do
	// pacote proíbe.
	motivo string
}

var inventarioFase3 = map[string]portStatus{
	"ChatArchiver":      {satisfeito: true},
	"PresenceAnnouncer": {satisfeito: true},
	"BlocklistManager":  {satisfeito: true},

	// Os tres nascidos da divisao do ContactDirectory (decisao 82). Todos
	// satisfaziveis por este transporte, sem assimetria: lookup.NumberID e
	// lookup.LidAndPhone estao PROVEN na H127, avatar.Fetch existe, e o roster
	// do contacts.Lister foi provado na H129 com as duas linhas fundidas.
	"IdentityResolver":      {satisfeito: true},
	"AvatarReader":          {satisfeito: true},
	"ContactRoster":         {satisfeito: true},
	"NewsletterReader":      {satisfeito: true},
	"SessionDisconnector":   {satisfeito: true},
	"ProfileAccessProvider": {satisfeito: true},
	"AppStateSyncer":        {satisfeito: true},

	"SessionLogouter": {motivo: "RECUSADO POR POLÍTICA, e não por incapacidade. A H122 " +
		"mediu que Socket.logout EXISTE e funciona neste build — mas chamá-lo " +
		"DESEMPAREIA a conta, e restaurar exige um humano com o telefone. É uma " +
		"quinta categoria: a capacidade FUNCIONA, e exercitá-la custa algo que só " +
		"uma pessoa pode repor. Implementar para satisfazer o compilador seria " +
		"chamar a operação que funciona, apagando um pareamento que ninguém pediu."},

	"PresenceSubscriber": {motivo: "RECUSADO POR DEPENDÊNCIA HUMANA, medida na H144: " +
		"com as duas contas acordadas ao mesmo tempo, a assinatura nunca chega a " +
		"`subscribed` em 45s, e os sinalizadores da agenda leem `isMyContact:false " +
		"isAddressBookContact:false`. A assinatura de presença exige o vínculo de " +
		"agenda, que se cria NO TELEFONE. Não é código por fazer — chamar isto de " +
		"pendente convidaria alguém a gastar uma sessão a tentar contorná-lo."},
	"SessionGuard": {satisfeito: true,
		motivo: "embutido em todos os demais; o adaptador de chat o satisfaz"},

	"UnavailableMessageRequester": {motivo: "RECUSADO POR NATUREZA: pede ao par o reenvio " +
		"de uma mensagem que não pôde ser DECIFRADA, e quem dirige a SPA não decifra " +
		"nada — a página já entrega texto. Não é lacuna, é ausência de sentido."},

	"CallRejecter":    {motivo: "pendente; o LEDGER regista reject como PARTIAL neste build"},
	"ChatMessenger":   {motivo: "pendente"},
	"GroupDirectory":  {motivo: "pendente"},
	"GroupLifecycle":  {motivo: "pendente"},
	"GroupRequests":   {motivo: "pendente"},
	"GroupSettings":   {motivo: "pendente"},
	"MessageComposer": {motivo: "pendente"},
	"PrivacyManager":  {motivo: "pendente"},
}

// portsDeTransporte lê os contratos e devolve os que carregam txtID.
func portsDeTransporte(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join("..", "..", "application", "contracts")
	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("lendo contratos: %v", err)
	}

	var out []string
	fset := token.NewFileSet()
	for _, e := range entradas {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		caminho := filepath.Join(dir, e.Name())
		arq, err := parser.ParseFile(fset, caminho, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", caminho, err)
		}
		ast.Inspect(arq, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || !ts.Name.IsExported() {
				return true
			}
			iface, ok := ts.Type.(*ast.InterfaceType)
			if !ok {
				return true
			}
			for _, m := range iface.Methods.List {
				fn, ok := m.Type.(*ast.FuncType)
				if !ok || fn.Params == nil {
					continue
				}
				for _, p := range fn.Params.List {
					id, ok := p.Type.(*ast.Ident)
					if !ok || id.Name != "string" {
						continue
					}
					for _, nome := range p.Names {
						if nome.Name == "txtID" {
							out = append(out, ts.Name.Name)
							return false
						}
					}
				}
			}
			return true
		})
	}
	sort.Strings(out)
	return out
}

// TestOInventarioCobreTodosOsPortsDeTransporte trava a cláusula "sem capability
// órfã" da decisão 71, na forma que a 81 tornou mensurável.
//
// Um port de transporte NOVO em pkg/application/contracts falha aqui até ser
// classificado. É o oposto de descobrir a lacuna quando o adaptador não compila
// — ou pior, quando ninguém descobre.
func TestOInventarioCobreTodosOsPortsDeTransporte(t *testing.T) {
	encontrados := portsDeTransporte(t)
	if len(encontrados) == 0 {
		t.Fatal("nenhum port de transporte encontrado: o instrumento está quebrado, " +
			"não o repositório")
	}

	for _, p := range encontrados {
		if _, ok := inventarioFase3[p]; !ok {
			t.Errorf("port de transporte %q não está no inventário da Fase 3: "+
				"classifique-o como satisfeito ou recusado com motivo", p)
		}
	}
	for p := range inventarioFase3 {
		if !contem(encontrados, p) {
			t.Errorf("o inventário classifica %q, que já não é um port de transporte: "+
				"a tabela ficou para trás do código", p)
		}
	}
}

// TestRecusaExigeMotivoEscrito: recusa sem motivo é a diferença silenciosa entre
// dois adaptadores que o doc do pacote proíbe.
func TestRecusaExigeMotivoEscrito(t *testing.T) {
	for nome, st := range inventarioFase3 {
		if !st.satisfeito && strings.TrimSpace(st.motivo) == "" {
			t.Errorf("port %q não satisfeito e SEM motivo escrito", nome)
		}
	}
}

func contem(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
