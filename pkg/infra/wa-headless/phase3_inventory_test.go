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

// TestOTotalDePortsEOMedidoENaoOAnunciado trava uma correção de contagem que eu
// mesmo precisei de fazer: durante várias decisões reportei "X de 18", que era o
// número medido ANTES das divisões.
//
// Cada divisão de port CRIA ports. Dividir ChatOperations em três, e
// ContactDirectory em três, e PresenceController e SessionController em dois
// cada, levou o total de 18 para 22 — e o numerador e o denominador crescem
// JUNTOS. Reportar contra o denominador antigo fazia o progresso parecer melhor
// do que era.
//
// Este teste não fixa o número: ele obriga a que o número reportado venha da
// MEDIÇÃO. Se alguém dividir outro port, o total muda e quem reportar tem de
// olhar em vez de repetir.
func TestOTotalDePortsEOMedidoENaoOAnunciado(t *testing.T) {
	encontrados := portsDeTransporte(t)
	if len(encontrados) != len(inventarioFase3) {
		t.Fatalf("o código tem %d ports de transporte e o inventário classifica %d: "+
			"a contagem reportada tem de vir da medição, não de um número anotado "+
			"antes das divisões", len(encontrados), len(inventarioFase3))
	}
	satisfeitos := 0
	for _, st := range inventarioFase3 {
		if st.satisfeito {
			satisfeitos++
		}
	}
	t.Logf("Fase 3: %d de %d ports satisfeitos", satisfeitos, len(encontrados))
}

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
	"ChatArchiver": {satisfeito: true},
	"ChatPinner": {motivo: "PENDENTE com caminho PROVADO: mesmo mecanismo de ChatArchiver " +
		"(SendAppState com BuildPin), que já é satisfeito pela headless. Falta criar " +
		"o adaptador headless espelhando chat/archiver.go — uma sessão de trabalho."},
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
	"ChatMessenger":         {satisfeito: true},
	"GroupDirectory":        {satisfeito: true},
	"GroupLifecycle":        {satisfeito: true},

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

	"CallRejecter": {motivo: "RECUSADO POR CAPACIDADE BLOQUEADA, decisão 88: o LEDGER " +
		"regista reject como PARTIAL neste build, com INCOMING_CALL BLOCKED e seis " +
		"hipóteses eliminadas por medição. Recusar uma chamada exige receber o evento " +
		"dela, e é o evento que não chega — implementar a recusa daria um método que " +
		"nunca é chamado."},
	"GroupRequests": {satisfeito: true},

	// --- os NOVE ports que a fusão de feature/wa-noise trouxe (2026-08-23) ---
	//
	// A branch de socket partiu a mensageria em portas por TIPO de mensagem, e o
	// dispositivo desta tabela acusou as nove de uma vez. Classificação por
	// medição, não por expectativa.

	"LIDResolver": {satisfeito: true,
		motivo: "o adaptador de identidade já o satisfazia; a asserção foi acrescentada e compila"},

	"TextMessenger": {motivo: "PENDENTE com caminho PROVADO: send.Text envia texto, e o " +
		"LEDGER regista-o como funcionando. Falta o adaptador, não a capacidade."},

	"MediaMessenger": {motivo: "PENDENTE com caminho PROVADO: send.SendMedia cobre imagem, " +
		"vídeo, áudio, documento e figurinha, e o LEDGER regista os cinco como OK. " +
		"Falta o adaptador."},

	"MediaDownloader": {motivo: "PENDENTE com caminho: capabilities/media.Get baixa mídia " +
		"pela página. Falta o adaptador."},

	"InteractiveMessenger": {motivo: "PENDENTE, e agora com evidência em vez de silêncio " +
		"(H144, 2026-08-23). O build TEM os geradores — WAWebGenerateInteractiveMessageProto e " +
		"WAWebGenerateNativeFlowButtonsMessageProto, ambos carregáveis como função — e o " +
		"despacho genérico WAWebSendMsgChatAction.addAndSendMsgToChat, que trata " +
		"MSG_TYPE.INTERACTIVE num ramo explícito. O pipeline de saída é dirigido por tabela " +
		"type→generateProtobuf, com `interactive` lá dentro. O que NÃO está provado é ENTREGA: " +
		"a enquete tem gerador, ação dedicada, e o nosso código chama a ação certa — e o ack " +
		"fica em 0 (H98/H101). A pergunta que decide exige sessão pareada."},

	"SimpleMessenger": {motivo: "PENDENTE e HETEROGÉNEO — os cinco métodos não têm o mesmo " +
		"estado, e tratá-los como um só esconderia isso. SendPoll: BLOQUEADO e medido (ack 0, " +
		"H98/H101). SendList e SendTemplate: geradores presentes e `list`/`hsm` na tabela de " +
		"saída (H144). SendLocation e SendContact: a H75 dava-os como MISSING, e a H144 " +
		"reabriu — WAWebSendLocationChatAction é função carregável, não componente React. " +
		"Quando esta porta for servida, é candidata a divisão pela decisão 92."},

	"PhonePairer": {motivo: "RECUSADO POR DEPENDÊNCIA HUMANA: parear exige um humano com o " +
		"telefone, e o caminho de boot da headless é de RESTAURAÇÃO — recusa página não " +
		"pareada com `PAIRING_LOADING`, medido em 2026-08-23. Não é código por fazer."},

	"StatusMessageSetter": {motivo: "PENDENTE POR MEDIR: capabilities/profile expõe " +
		"SetDisplayName, que é o NOME e não o recado. Nenhuma capability põe o `about`, e o " +
		"LEDGER não o regista. Não medi se a página o expõe — dizer 'recusado' aqui seria " +
		"repetir o erro da H75."},

	"HistorySyncRequester": {motivo: "RECUSADO POR AUSÊNCIA DE SENTIDO: pedir sincronização " +
		"de histórico é operação de PROTOCOLO, e quem dirige a SPA não a pede — a página já " +
		"tem o histórico no próprio store, que é de onde a headless lê. Não é lacuna; é a " +
		"pergunta não existir deste lado. Zero ocorrências de history sync em capabilities/."},

	"GroupInfoSettings": {satisfeito: true},
	"GroupParticipants": {satisfeito: true},

	"GroupPhotoSetter": {motivo: "RECUSADO POR CAPACIDADE AUSENTE DO BUILD, medida em " +
		"H140/decisão 60: `WAWebSetPicture` e `WAWebProfilePicThumbBridge` estão " +
		"ausentes desta build, e o LEDGER regista `setPicture` e `deletePicture` como " +
		"BLOCKED. Não é lacuna nossa: é capacidade que a página não expõe aqui."},

	"GroupEphemeralSetter": {motivo: "PENDENTE, e agora MEDIDO: o build TEM os módulos. " +
		"A varredura de bundles de 2026-08-23 achou 105 nomes casando com " +
		"`Ephemeral|Disappear|Expir`, entre eles `WAWebChangeEphemeralDurationChatAction` " +
		"— exatamente a ação que este port precisaria — e `WAWebEphemeralIsDurationAllowed`. " +
		"A ausência no LEDGER dizia apenas que a REFERÊNCIA não expõe a operação, e a " +
		"cautela de não confundir isso com ausência no build valeu: valia investigar. " +
		"Falta o passo que exige sessão pareada (carregar o módulo e medir a forma), " +
		"e nome em bundle NÃO é prova de módulo carregável."},
	"ChatMuter": {motivo: "PENDENTE. Port NOVO, chegou com a CAP-52 (2026-08-24): silenciar conversa por app-state. Mesma familia do ChatPinner e do ArchiveChat — a stack headless teria de emitir a mutacao de app-state pela pagina, e isso ainda nao esta medido para nenhum dos tres. Classificar como recusa seria inventar uma ausencia que ninguem mediu."},

	"MessageStarrer": {motivo: "PENDENTE. Port NOVO, chegou com a CAP-54 (2026-08-24): favoritar mensagem por app-state. Herda o veredito do ChatMuter acima e pela mesma razao — e' app-state, nao mensagem, e a emissao pela pagina nao esta medida."},

	"DefaultDisappearingTimerSetter": {motivo: "PENDENTE. Port NOVO, chegou com a CAP-50 (2026-08-24): o temporizador PADRÃO de mensagens temporárias da conta, irmão do por-conversa. Herda o veredito do `GroupEphemeralSetter` acima e pela mesma evidência: a varredura de bundles de 2026-08-23 achou `WAWebChangeEphemeralDurationChatAction` e `WAWebEphemeralIsDurationAllowed`, logo o build TEM os módulos da família. O que falta é o mesmo passo: carregar o módulo com sessão pareada e medir a FORMA — o padrão de conta pode não usar a mesma ação que o de conversa, e nome em bundle NÃO é prova de módulo carregável. Classificar como recusa aqui seria inventar uma ausência que ninguém mediu."},

	"PrivacyManager": {motivo: "PENDENTE, e agora MEDIDO: o build TEM os módulos, o que " +
		"confirma a decisão 89 ao recusar classificá-lo como recusa. A varredura de " +
		"bundles de 2026-08-23 achou 124 nomes casando com `Privac`, entre eles " +
		"`WASmaxBizSettingsGetPrivacySettingRPC`, `WASmaxBizSettingsSetPrivacySettingRPC` " +
		"e `WASmaxPrivacyGetContactBlacklistRPC` — o par get/set que este port pede. " +
		"Falta o passo que exige sessão pareada, e nome em bundle NÃO é prova de módulo " +
		"carregável."},
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
