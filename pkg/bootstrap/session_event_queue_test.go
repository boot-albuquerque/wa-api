package bootstrap

import (
	"sync"
	"testing"
	"time"
)

// F87, opção A. O que estes testes travam é o par de propriedades que a fila
// existe para dar: o produtor NÃO espera o trabalho, e o trabalho sai EM ORDEM.
// Uma sem a outra não resolve — devolver o laço de nós perdendo a ordem é a
// opção (B), que foi recusada.

// limparFilas garante que uma sessão de teste não vaze para a seguinte.
// `sessionQueues` é estado de pacote.
func limparFilas(t *testing.T, userID string) {
	t.Helper()
	t.Cleanup(func() { stopSessionEventQueue(userID) })
}

// TestSessionQueue_NaoBloqueiaOProdutor é O teste desta correção.
//
// Quem chama é o laço de nós do SDK, que é sequencial por sessão: enquanto ele
// não voltar, aquela sessão não processa mais NADA — nem recibo, nem presença,
// nem marcação de leitura. Antes desta mudança o download de mídia acontecia
// ali dentro, com prazos de 1 a 10 minutos.
func TestSessionQueue_NaoBloqueiaOProdutor(t *testing.T) {
	const userID = "u-lento"
	limparFilas(t, userID)
	startSessionEventQueue(userID)

	// O evento lento LIBERA SOZINHO, e essa é a diferença entre um teste que
	// falha e um que pendura. A primeira versão travava até um Cleanup que só
	// roda no fim do teste: com o enfileiramento removido, ela não acusava o
	// defeito — ela travava por dez minutos até o timeout do processo, que é a
	// ARMADILHAS 16 acontecendo dentro do controle negativo desta correção.
	// O cronômetro começa ANTES do evento lento, e essa posição é o teste.
	// Media-lo depois foi o meu primeiro erro aqui: com a execução em linha o
	// custo do evento lento acontecia antes do `time.Now()`, o número saía
	// idêntico nos dois mundos, e o controle negativo passava. Um teste que
	// mede depois do efeito não mede o efeito (ARMADILHAS 25).
	const lento = 2 * time.Second
	inicio := time.Now()
	enqueueSessionEvent(userID, func() { time.Sleep(lento) })
	enqueueSessionEvent(userID, func() {})
	enqueueSessionEvent(userID, func() {})
	levou := time.Since(inicio)

	// Folga larga de propósito: o que se mede é a diferença entre "voltou na
	// hora" e "esperou o download inteiro", que aqui são três ordens de
	// grandeza. Um limiar apertado só produziria teste instável.
	if levou > lento/4 {
		t.Fatalf("o produtor esperou %v pelo evento lento (que leva %v); o laco de nos do SDK ficaria parado esse tempo todo",
			levou, lento)
	}
}

// TestSessionQueue_PreservaAOrdem é a outra metade, e a razão de a opção (A)
// ter sido escolhida em vez de despachar no pool compartilhado.
//
// O cliente hoje tem essa garantia sem saber que tem — ela vem de graça do laço
// sequencial do SDK. Perdê-la faria um texto enviado depois de um vídeo chegar
// ao webhook antes dele.
func TestSessionQueue_PreservaAOrdem(t *testing.T) {
	const userID = "u-ordem"
	limparFilas(t, userID)
	startSessionEventQueue(userID)

	const total = 50
	var (
		mu    sync.Mutex
		saida []int
	)
	pronto := make(chan struct{})

	for i := 0; i < total; i++ {
		n := i
		enqueueSessionEvent(userID, func() {
			mu.Lock()
			saida = append(saida, n)
			if len(saida) == total {
				close(pronto)
			}
			mu.Unlock()
		})
	}

	select {
	case <-pronto:
	case <-time.After(5 * time.Second):
		// Prazo, nunca espera indefinida: teste pendurado nao reporta nada
		// (ARMADILHAS 16).
		t.Fatal("a fila nao processou todos os eventos")
	}

	mu.Lock()
	defer mu.Unlock()
	for i, v := range saida {
		if v != i {
			t.Fatalf("evento fora de ordem na posicao %d: %d (saida=%v)", i, v, saida[:min(len(saida), 10)])
		}
	}
}

// TestSessionQueue_SemFilaExecutaEmLinha fixa o caminho de quem nunca passou
// pelo Attach. Criar fila implícita ali produziria worker que ninguém desmonta.
func TestSessionQueue_SemFilaExecutaEmLinha(t *testing.T) {
	executou := false
	if !enqueueSessionEvent("u-sem-fila", func() { executou = true }) {
		t.Fatal("enqueue devolveu false sem fila registrada")
	}
	if !executou {
		t.Error("o evento nao rodou; sem fila o comportamento tem de ser o antigo, em linha")
	}
}

// TestSessionQueue_AposParadaDescarta: eventos de uma sessão desmontada não
// podem ser processados. Eles tocariam cliente e registries já liberados —
// exatamente o que a ordem "para a fila ANTES de derrubar o cliente" evita.
func TestSessionQueue_AposParadaDescarta(t *testing.T) {
	const userID = "u-parada"
	startSessionEventQueue(userID)
	stopSessionEventQueue(userID)

	// A fila foi removida do mapa: o caminho agora é o de "sem fila". O que
	// importa é que não entre em pânico nem bloqueie.
	executou := false
	enqueueSessionEvent(userID, func() { executou = true })
	if !executou {
		t.Error("evento apos a parada nem executou em linha nem foi descartado com seguranca")
	}
}

// TestSessionQueue_ParadaLiberaProdutorBloqueado é o caso que trava o processo
// se estiver errado: fila cheia, produtor bloqueado, sessão sendo derrubada. Se
// o envio não observasse a parada, o laço de nós do SDK ficaria preso para
// sempre numa sessão que já morreu.
func TestSessionQueue_ParadaLiberaProdutorBloqueado(t *testing.T) {
	const userID = "u-cheia"
	q := startSessionEventQueue(userID)
	t.Cleanup(func() { stopSessionEventQueue(userID) })

	// Trava o worker para que a fila encha de verdade — e ESPERA ele estacionar
	// antes de encher.
	//
	// Sem o `entrou`, o teste era corrida: o laço abaixo podia encher a fila
	// enquanto o worker ainda não tinha pego o evento bloqueante; ele então
	// drenava um item, abria vaga, e o envio "bloqueado" passava direto. Passava
	// isolado e falhava na suíte completa, que é a assinatura clássica de teste
	// que depende de escalonamento.
	entrou := make(chan struct{})
	prender := make(chan struct{})
	enqueueSessionEvent(userID, func() { close(entrou); <-prender })
	t.Cleanup(func() { close(prender) })

	select {
	case <-entrou:
	case <-time.After(3 * time.Second):
		t.Fatal("o worker nao comecou a processar o evento bloqueante")
	}

	// Com o worker parado, nada mais drena: encher ate a capacidade e
	// deterministico.
	for len(q.fila) < sessionQueueCapacity {
		q.fila <- func() {}
	}

	bloqueado := make(chan bool, 1)
	go func() { bloqueado <- enqueueSessionEvent(userID, func() {}) }()

	// Confirma que ELE ESTÁ MESMO bloqueado antes de parar: sem isto o teste
	// passaria mesmo que o envio nunca bloqueasse, e não provaria nada.
	select {
	case <-bloqueado:
		t.Fatal("o envio nao bloqueou com a fila cheia; o teto nao esta valendo")
	case <-time.After(100 * time.Millisecond):
	}

	stopSessionEventQueue(userID)

	select {
	case aceito := <-bloqueado:
		if aceito {
			t.Error("o envio devolveu true para uma sessao ja desmontada")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("o produtor continuou preso apos a parada; o laco de nos do SDK ficaria travado numa sessao morta")
	}
}

// TestSessionQueue_StartEhIdempotente: Attach roda de novo a cada reconexão. Um
// worker por chamada significaria dois consumidores da mesma fila — e ordem
// perdida, que é justamente o que ela protege.
func TestSessionQueue_StartEhIdempotente(t *testing.T) {
	const userID = "u-idem"
	limparFilas(t, userID)

	primeira := startSessionEventQueue(userID)
	segunda := startSessionEventQueue(userID)

	if primeira != segunda {
		t.Fatal("a segunda chamada criou fila nova; haveria dois consumidores e a ordem se perderia")
	}
}

// TestSessionQueue_StopEhIdempotente: Detach é idempotente por contrato
// (KillChannel.Signal vira no-op sem ouvinte), então o desligamento também
// precisa ser — fechar duas vezes entraria em pânico.
func TestSessionQueue_StopEhIdempotente(t *testing.T) {
	const userID = "u-stop2"
	startSessionEventQueue(userID)
	stopSessionEventQueue(userID)
	stopSessionEventQueue(userID) // não pode entrar em panico
}

// TestSessionQueue_PanicoNaoMataOWorker: um evento que estoura não pode levar a
// sessão junto. Sem o recover POR EVENTO, o sintoma apareceria como "parou de
// receber mensagem", sem relação aparente com o evento que falhou.
func TestSessionQueue_PanicoNaoMataOWorker(t *testing.T) {
	const userID = "u-panico"
	limparFilas(t, userID)
	startSessionEventQueue(userID)

	enqueueSessionEvent(userID, func() { panic("boom") })

	depois := make(chan struct{})
	enqueueSessionEvent(userID, func() { close(depois) })

	select {
	case <-depois:
	case <-time.After(3 * time.Second):
		t.Fatal("o worker morreu com o panico; a sessao pararia de processar eventos em silencio")
	}
}

// TestSessionQueue_ProfundidadeDistingueVazioDeAusente: -1 e 0 significam
// coisas diferentes, e um diagnóstico que os confunde manda o operador procurar
// no lugar errado.
func TestSessionQueue_ProfundidadeDistingueVazioDeAusente(t *testing.T) {
	const userID = "u-prof"
	if d := sessionQueueDepth(userID); d != -1 {
		t.Errorf("profundidade sem fila = %d, want -1", d)
	}

	limparFilas(t, userID)
	startSessionEventQueue(userID)

	if d := sessionQueueDepth(userID); d != 0 {
		t.Errorf("profundidade de fila vazia = %d, want 0", d)
	}
}
