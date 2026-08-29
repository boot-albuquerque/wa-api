package retry

import (
	"fmt"
	"testing"
	"time"
)

// O contador precisa continuar contando igual dentro da janela — o despejo nao
// pode custar corretude no caminho normal.
func TestCounterMapContaNormalmenteDentroDaJanela(t *testing.T) {
	var m counterMap[string]
	base := time.Unix(1700000000, 0)

	for i := 1; i <= 5; i++ {
		if got := m.increment("MSG1", base.Add(time.Duration(i)*time.Second)); got != i {
			t.Fatalf("incremento %d = %d, esperado %d", i, got, i)
		}
	}
	if m.len() != 1 {
		t.Errorf("len = %d, esperado 1", m.len())
	}
}

// Entrada parada ha' mais de counterTTL sai na varredura, e a contagem daquela
// chave recomeca. E' a consequencia aceita da politica.
func TestCounterMapDespejaEntradaVelha(t *testing.T) {
	var m counterMap[string]
	base := time.Unix(1700000000, 0)

	m.increment("VELHA", base)
	m.increment("VELHA", base)

	// Uma chave nova depois do TTL dispara a varredura.
	depois := base.Add(counterTTL + time.Minute)
	m.increment("NOVA", depois)

	if _, ok := m.entries["VELHA"]; ok {
		t.Error("a entrada velha deveria ter sido despejada")
	}
	if got := m.increment("VELHA", depois); got != 1 {
		t.Errorf("apos o despejo a contagem = %d, esperado recomecar em 1", got)
	}
}

// Dentro do TTL nada e' despejado, mesmo com a varredura rodando.
func TestCounterMapNaoDespejaDentroDoTTL(t *testing.T) {
	var m counterMap[string]
	base := time.Unix(1700000000, 0)

	m.increment("A", base)
	// Passa do intervalo de varredura, mas nao do TTL.
	m.increment("B", base.Add(counterSweepInterval+time.Second))

	if m.len() != 2 {
		t.Fatalf("len = %d, esperado 2", m.len())
	}
	if got := m.increment("A", base.Add(counterSweepInterval+2*time.Second)); got != 2 {
		t.Errorf("A = %d, esperado 2 — nao deveria ter sido despejada", got)
	}
}

// O caso que o TTL sozinho nao cobre: inundacao de chaves distintas DENTRO da
// janela. O teto rigido tem que segurar.
func TestCounterMapRespeitaOTetoSobInundacao(t *testing.T) {
	var m counterMap[string]
	base := time.Unix(1700000000, 0)

	// Todas dentro do TTL, entao nenhuma sai por idade.
	for i := 0; i < counterMaxEntries*2; i++ {
		m.increment(fmt.Sprintf("FLOOD-%d", i), base.Add(time.Duration(i)*time.Millisecond))
	}

	if m.len() > counterMaxEntries {
		t.Fatalf("len = %d, esperado no maximo %d", m.len(), counterMaxEntries)
	}
	if m.len() < counterEvictTarget {
		t.Errorf("len = %d, esperado nao descer abaixo da marca d'agua %d", m.len(), counterEvictTarget)
	}
	// As mais RECENTES sao as que sobrevivem.
	ultima := fmt.Sprintf("FLOOD-%d", counterMaxEntries*2-1)
	if _, ok := m.entries[ultima]; !ok {
		t.Error("a entrada mais recente deveria ter sobrevivido ao despejo")
	}
	primeira := "FLOOD-0"
	if _, ok := m.entries[primeira]; ok {
		t.Error("a entrada mais antiga deveria ter saido primeiro")
	}
}

// A varredura e' O(n) e roda sob o lock do contador; o intervalo existe para
// ela nao rodar a cada incremento.
func TestCounterMapVarreNoMaximoUmaVezPorIntervalo(t *testing.T) {
	var m counterMap[string]
	base := time.Unix(1700000000, 0)

	m.increment("A", base)
	primeira := m.lastSweep

	m.increment("B", base.Add(time.Second))
	if !m.lastSweep.Equal(primeira) {
		t.Error("varreu de novo antes de o intervalo passar")
	}

	m.increment("C", base.Add(counterSweepInterval+time.Second))
	if m.lastSweep.Equal(primeira) {
		t.Error("nao varreu depois de o intervalo passar")
	}
}

// BumpMessageRetries reinicia a contagem a partir do valor do servidor quando
// este e' o nosso primeiro recibo. A regra tem que sobreviver ao counterMap.
func TestBumpMessageRetriesReiniciaDoValorDoServidor(t *testing.T) {
	var s State
	if got := s.BumpMessageRetries("MSG1", 4); got != 5 {
		t.Errorf("= %d, esperado 5 (countInMsg + 1)", got)
	}
	if got := s.BumpMessageRetries("MSG1", 4); got != 6 {
		t.Errorf("= %d, esperado 6 — o segundo recibo so' incrementa", got)
	}
}

// A defesa contra inundacao nao pode virar o vetor: descer so' ate' o teto
// faria a proxima insercao estourar de novo e ordenar de novo, cobrando
// O(n log n) POR MENSAGEM de quem inunda. A marca d'agua amortiza isso, e este
// teste trava o custo em tempo de parede.
func TestCounterMapInundacaoNaoEhQuadratica(t *testing.T) {
	var m counterMap[string]
	base := time.Unix(1700000000, 0)

	inicio := time.Now()
	for i := 0; i < counterMaxEntries*4; i++ {
		m.increment(fmt.Sprintf("FLOOD-%d", i), base.Add(time.Duration(i)*time.Millisecond))
	}
	decorrido := time.Since(inicio)

	// Sem a marca d'agua isto levava mais de um minuto. O teto e' generoso de
	// proposito: o que se trava e' a ORDEM de grandeza, nao o numero exato.
	if decorrido > 5*time.Second {
		t.Errorf("%d insercoes levaram %v — o despejo por teto voltou a ser quadratico",
			counterMaxEntries*4, decorrido)
	}
}
