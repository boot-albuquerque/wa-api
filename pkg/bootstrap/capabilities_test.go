package bootstrap

import (
	"context"
	"encoding/json"
	"testing"
)

// D7 / F104. O que estes testes travam não é o formato do relatório — é a
// distinção que ele existe para carregar: "não dá" e "dá, mas não está ligado"
// são estados diferentes, e colapsá-los num booleano foi o que deixou a F104
// invisível por dias.

// limparCapacidades evita que uma publicação de um teste vaze para o seguinte.
// `currentCapabilities` é estado de pacote.
func limparCapacidades(t *testing.T) {
	t.Helper()
	anterior := currentCapabilities.Load()
	t.Cleanup(func() { currentCapabilities.Store(anterior) })
}

func TestCapacidades_SQLiteEmSingleNaoSuportaMultiPod(t *testing.T) {
	limparCapacidades(t)
	r := publishCapabilities(clusterModeSingle, databaseTypeSQLite)

	if r.MultiPod != multiPodUnsupported {
		t.Errorf("multi_pod = %q, want %q — com SQLite nao e' 'ainda nao', e' impossivel",
			r.MultiPod, multiPodUnsupported)
	}
	if r.SessionOwnership != ownershipInactive {
		t.Errorf("session_ownership = %q, want %q — em single o lease e' inerte por desenho",
			r.SessionOwnership, ownershipInactive)
	}
	// A durabilidade NAO depende de Postgres nem de broker (ADR-0005 D3), e
	// este campo existe justamente para matar essa suposicao recorrente.
	if r.DurableRetry != durableRetryOutbox {
		t.Errorf("durable_retry = %q, want %q mesmo no cenario mais pobre",
			r.DurableRetry, durableRetryOutbox)
	}
}

// TestCapacidades_PostgresEmSingleFicaDisponivel é a distinção que motivou usar
// string em vez de booleano. Este pod está tão "não-multi" quanto o de SQLite,
// e a ação de quem lê é oposta: aqui basta declarar o modo.
func TestCapacidades_PostgresEmSingleFicaDisponivel(t *testing.T) {
	limparCapacidades(t)
	r := publishCapabilities(clusterModeSingle, databaseTypePostgres)

	if r.MultiPod != multiPodAvailable {
		t.Errorf("multi_pod = %q, want %q", r.MultiPod, multiPodAvailable)
	}
	if r.MultiPod == multiPodUnsupported {
		t.Error("Postgres em single reportado como impossivel; a distincao entre 'nao da' e 'nao esta ligado' se perdeu, que e' exatamente o defeito da F104")
	}
	if r.SessionOwnership != ownershipInactive {
		t.Errorf("session_ownership = %q, want %q — Postgres nao liga a posse sozinho",
			r.SessionOwnership, ownershipInactive)
	}
}

func TestCapacidades_MultiAtivaPosseEMultiPod(t *testing.T) {
	limparCapacidades(t)
	r := publishCapabilities(clusterModeMulti, databaseTypePostgres)

	if r.MultiPod != multiPodActive {
		t.Errorf("multi_pod = %q, want %q", r.MultiPod, multiPodActive)
	}
	if r.SessionOwnership != ownershipEnforced {
		t.Errorf("session_ownership = %q, want %q", r.SessionOwnership, ownershipEnforced)
	}
}

// TestCapacidades_RelatorioSaiNoLogComTodosOsCampos é o teste do D7 em si.
//
// Ele checa campo a campo em vez de comparar a linha inteira: com uma
// comparação de string, retirar um campo do log faria o teste falhar por
// "linha diferente", sem dizer QUAL informação sumiu — e é a informação que
// importa, não a linha.
func TestCapacidades_RelatorioSaiNoLogComTodosOsCampos(t *testing.T) {
	limparCapacidades(t)

	buf := captureLogInto(t)

	publishCapabilities(clusterModeSingle, databaseTypeSQLite)

	if buf.Len() == 0 {
		t.Fatal("nada foi registrado; sem a linha de arranque o estado volta a ser inferido, que e' a F104")
	}

	var linha map[string]any
	if err := json.Unmarshal(buf.Bytes(), &linha); err != nil {
		t.Fatalf("a linha nao e' JSON valido: %v (%s)", err, buf.String())
	}

	if linha["message"] != capabilityReportMessage {
		t.Errorf("message = %v, want %q", linha["message"], capabilityReportMessage)
	}

	esperado := map[string]string{
		"cluster_mode":      clusterModeSingle,
		"database":          databaseTypeSQLite,
		"multi_pod":         multiPodUnsupported,
		"session_ownership": ownershipInactive,
		"durable_retry":     durableRetryOutbox,
	}
	for campo, valor := range esperado {
		if linha[campo] != valor {
			t.Errorf("campo %q = %v, want %q", campo, linha[campo], valor)
		}
	}
}

func TestCapacidades_PublicarTornaORelatorioLegivel(t *testing.T) {
	limparCapacidades(t)

	publishCapabilities(clusterModeMulti, databaseTypePostgres)

	lido := currentCapabilityReport()
	if lido.ClusterMode != clusterModeMulti || lido.Database != databaseTypePostgres {
		t.Errorf("relatorio lido = %+v; nao corresponde ao publicado", lido)
	}
}

// TestCapacidades_SemPublicacaoNaoInventaPadrao fixa a decisão de o zero-value
// ter strings vazias em vez de padrões plausíveis.
//
// Um relatório que respondesse `cluster_mode: single` sem ninguém ter declarado
// nada seria a mesma classe de mentira que este arquivo existe para remover: o
// operador leria uma afirmação onde não houve afirmação nenhuma.
func TestCapacidades_SemPublicacaoNaoInventaPadrao(t *testing.T) {
	limparCapacidades(t)
	currentCapabilities.Store(nil)

	r := currentCapabilityReport()
	if r.ClusterMode != "" || r.Database != "" || r.MultiPod != "" {
		t.Errorf("relatorio sem publicacao = %+v; deveria vir vazio, nao com padroes plausiveis", r)
	}
}

// TestProntidao_CorpoCarregaAsCapacidades liga as duas metades: publicar não
// serve de nada se o corpo do /health/ready não carregar.
func TestProntidao_CorpoCarregaAsCapacidades(t *testing.T) {
	limparCapacidades(t)
	publishCapabilities(clusterModeSingle, databaseTypeSQLite)

	probe := buildReadinessProbe(fakePinger{}, nil)
	report := probe(context.Background())

	if report.Capabilities.Database != databaseTypeSQLite {
		t.Errorf("capabilities.database = %q, want %q", report.Capabilities.Database, databaseTypeSQLite)
	}
	if report.Capabilities.MultiPod != multiPodUnsupported {
		t.Errorf("capabilities.multi_pod = %q, want %q", report.Capabilities.MultiPod, multiPodUnsupported)
	}
	if !report.Ready() {
		t.Error("o pod ficou nao-pronto por causa das capacidades; elas sao FATO, nao veredito — SQLite em single esta perfeitamente pronto")
	}
}

// TestProntidao_CapacidadesSaoLidasEmTempoDeRequisicao é o controle da
// armadilha que o lease já pagou uma vez: capturar o valor no momento da
// montagem congela o que estivesse lá, e o campo nunca mais muda.
//
// A sonda é montada ANTES da publicação, de propósito.
func TestProntidao_CapacidadesSaoLidasEmTempoDeRequisicao(t *testing.T) {
	limparCapacidades(t)
	currentCapabilities.Store(nil)

	probe := buildReadinessProbe(fakePinger{}, nil) // montada com o relatorio VAZIO

	publishCapabilities(clusterModeMulti, databaseTypePostgres)

	report := probe(context.Background())
	if report.Capabilities.ClusterMode != clusterModeMulti {
		t.Errorf("cluster_mode = %q, want %q — a sonda congelou o relatorio no momento da montagem, que foi o defeito medido com o lease",
			report.Capabilities.ClusterMode, clusterModeMulti)
	}
}
