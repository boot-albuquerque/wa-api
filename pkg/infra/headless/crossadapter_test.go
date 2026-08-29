package headless_test

import (
	"testing"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/headless"
	headlesschat "wa-api/pkg/infra/headless/chat"
	"wa-api/pkg/infra/headless/registry"
	noisemisc "wa-api/pkg/infra/noise/adapters/misc"
)

// Verificação CROSS-ADAPTER (decisão 81).
//
// Este arquivo é o único lugar do repositório que enxerga os DOIS adaptadores ao
// mesmo tempo, e é de propósito: a premissa da decisão 71 é que eles são
// transportes alternativos para a mesma intenção de produto, e uma premissa que
// ninguém verifica é uma esperança.

// TestOsDoisAdaptadoresSatisfazemAMesmaPortaDeArquivar é a composição no sentido
// que importa: um caso de uso que peça ChatArchiver aceita QUALQUER um dos dois
// sem saber qual está atrás.
func TestOsDoisAdaptadoresSatisfazemAMesmaPortaDeArquivar(t *testing.T) {
	var doSocket appport.ChatArchiver = noisemisc.NewMiscAdapter(nil)
	var daPagina appport.ChatArchiver = headlesschat.NewArchiver(adapter.NewSessions(registry.New(1), nil))

	if doSocket == nil || daPagina == nil {
		t.Fatal("um dos adaptadores não satisfaz ChatArchiver")
	}
}

// TestOSocketSatisfazAComposicaoEAPaginaNAO trava a assimetria REAL, e não uma
// versão dela escrita à mão.
//
// O socket decifra, então RequestUnavailableMessage faz sentido lá e ele
// satisfaz ChatOperations inteira. A página não decifra, então não satisfaz — e
// é exatamente isso que a decisão 80 tornou representável no tipo em vez de num
// erro em tempo de execução.
func TestOSocketSatisfazAComposicaoEAPaginaNAO(t *testing.T) {
	var doSocket any = noisemisc.NewMiscAdapter(nil)
	var daPagina any = headlesschat.NewArchiver(adapter.NewSessions(registry.New(1), nil))

	if _, ok := doSocket.(appport.ChatOperations); !ok {
		t.Error("o adaptador do socket deixou de satisfazer ChatOperations; a " +
			"composição quebrou para o transporte que a satisfazia")
	}
	if _, ok := daPagina.(appport.ChatOperations); ok {
		t.Error("o adaptador da página passou a satisfazer ChatOperations: alguém " +
			"implementou capacidade que este transporte não tem")
	}
}

// TestOsDoisTransportesDiscordamDaGrafiaEConcordamDoSignificado é a razão pela
// qual eles NÃO são intercambiáveis sem conversão, e por que ToPageJID existe.
//
// Um domain.JID atravessa a fronteira entre casos de uso; se os dois adaptadores
// interpretassem a mesma string do mesmo modo, a conversão seria supérflua. Eles
// não interpretam — e este teste falha se algum dia passarem a interpretar, o
// que tornaria ToPageJID código morto que ninguém sabe remover.
func TestOsDoisTransportesDiscordamDaGrafiaEConcordamDoSignificado(t *testing.T) {
	doSocket := domain.JID("5511999999999@s.whatsapp.net")

	daPagina, err := adapter.ToPageJID(doSocket)
	if err != nil {
		t.Fatalf("ToPageJID: %v", err)
	}
	if daPagina == string(doSocket) {
		t.Fatal("as duas grafias convergiram; ou o build mudou, ou ToPageJID " +
			"virou código morto — nos dois casos isto precisa de ser revisto")
	}
	if domain.JID(daPagina).Namespace() != doSocket.Namespace() {
		t.Fatalf("as grafias deixaram de significar o mesmo: %q contra %q",
			domain.JID(daPagina).Namespace(), doSocket.Namespace())
	}
}
