package broadcast

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Testes da instrumentação da F85 (decisão 47=a do canal).
//
// Três medições excluíram três mecanismos — painel lento por mensagem,
// separador em segundo plano, escrita em SQLite a esfomear — e NENHUMA
// reproduziu a queda de campo de 2026-08-10. A decisão foi parar de levantar
// hipóteses e registar o que acontece, para a próxima ocorrência trazer a sua
// própria prova.
//
// O que estes testes travam é isso: que uma escrita lenta que NÃO falha ainda
// assim deixe rasto, com os três números de que o diagnóstico precisa —
// duração, tamanho da carga e número de conexões.

// capturarLog troca o logger global por um que escreve num buffer, e repõe-no
// no fim. Devolve o buffer.
func capturarLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	anterior := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = anterior })
	return &buf
}

// servidorLento aceita o WebSocket e depois NÃO LÊ durante `atraso`.
//
// A lentidão é real, não simulada: sem ninguém a drenar, a janela TCP enche e
// a escrita do broadcast bloqueia de verdade. Um dublê que apenas dormisse
// dentro do Registry mediria o `time.Sleep`, não o caminho de escrita — e é
// exatamente a diferença que fez o primeiro harness da F86 medir a coisa
// errada (CLAUDE.md, "o instrumento precisa alocar, bloquear e falhar como o
// original").
func servidorLento(t *testing.T, atraso time.Duration) *websocket.Conn {
	t.Helper()
	pronto := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.Close(websocket.StatusNormalClosure, "") }()
		close(pronto)
		time.Sleep(atraso)
		// Lê tudo o que se acumulou, para a escrita destravar em vez de
		// estourar o prazo: o caso sob teste é LENTO, não morto.
		for {
			if _, _, err := c.Read(r.Context()); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	conn, _, err := websocket.Dial(context.Background(), "ws"+srv.URL[len("http"):], nil)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })
	<-pronto
	return conn
}

func TestBroadcast_EscritaLentaDeixaRasto(t *testing.T) {
	buf := capturarLog(t)
	r := New()

	// Um pouco acima do limiar, e bem abaixo do writeTimeout: o caso que
	// interessa é a escrita que DEMORA e ainda assim entrega. A que falha já
	// era registada antes desta mudança.
	conn := servidorLento(t, slowWriteThreshold+400*time.Millisecond)
	r.Add("u1", conn)

	// Carga grande o suficiente para encher a janela e fazer a escrita
	// bloquear de facto enquanto ninguém lê.
	grande := strings.Repeat("x", 2<<20)
	r.Broadcast("u1", map[string]string{"type": "Message", "corpo": grande})

	linhas := buf.String()
	if !strings.Contains(linhas, "websocket broadcast write was slow") {
		t.Fatalf("a escrita lenta não deixou rasto. Sem esta linha, a próxima "+
			"queda em campo gera a QUARTA hipótese em vez de trazer prova.\nlog: %s", linhas)
	}

	// Os três números são o ponto: sem eles a linha diz "foi lento" e não
	// ajuda a distinguir carga grande de consumidor parado de fan-out largo.
	var ev map[string]interface{}
	for _, l := range strings.Split(strings.TrimSpace(linhas), "\n") {
		if strings.Contains(l, "was slow") {
			if err := json.Unmarshal([]byte(l), &ev); err != nil {
				t.Fatalf("linha de log não é JSON: %v (%s)", err, l)
			}
		}
	}
	for _, campo := range []string{"writeDuration", "payloadBytes", "conns", "userID"} {
		if _, ok := ev[campo]; !ok {
			t.Errorf("falta %q na linha de escrita lenta: %v", campo, ev)
		}
	}
	if n, ok := ev["payloadBytes"].(float64); !ok || int(n) < 2<<20 {
		t.Errorf("payloadBytes = %v, esperava pelo menos o tamanho da carga", ev["payloadBytes"])
	}
}

// O limite: uma escrita RÁPIDA não pode registar nada. Uma rajada de
// HistorySync produz milhares de escritas legítimas, e um limiar que as
// registasse a todas tornaria o log inútil no exato momento em que ele serve.
func TestBroadcast_EscritaRapidaNaoRegista(t *testing.T) {
	buf := capturarLog(t)
	r := New()

	conn := servidorLento(t, 0) // lê imediatamente
	r.Add("u1", conn)
	r.Broadcast("u1", map[string]string{"type": "Message", "corpo": "curto"})

	if strings.Contains(buf.String(), "was slow") {
		t.Errorf("escrita rápida registada como lenta: numa rajada isto seria "+
			"uma linha por evento.\nlog: %s", buf.String())
	}
}

// A serialização acontece UMA VEZ, antes do fan-out, e uma carga que não
// serializa não é tentada N vezes.
func TestBroadcast_PayloadInvalidoFalhaUmaVezSo(t *testing.T) {
	buf := capturarLog(t)
	r := New()
	for i := 0; i < 3; i++ {
		r.Add("u1", servidorLento(t, 0))
	}

	// canais não serializam para JSON.
	r.Broadcast("u1", map[string]interface{}{"type": "Message", "mau": make(chan int)})

	linhas := strings.Count(buf.String(), "does not serialise")
	if linhas != 1 {
		t.Errorf("payload inválido produziu %d linhas com 3 conexões, quero 1: "+
			"o Marshal tem de acontecer antes do fan-out, senão o mesmo erro é "+
			"multiplicado por conexão.\nlog: %s", linhas, buf.String())
	}
}
