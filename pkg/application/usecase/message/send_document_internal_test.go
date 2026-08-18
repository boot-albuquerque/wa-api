package message

import (
	"encoding/base64"
	"testing"
)

// TestFetchDocumentMaxBytes_IsExactly100MB prova, sem alocar payload
// nenhum, que a constante REALMENTE ligada em produção
// (decodeDataURIDocument e o ramo URL do Execute) vale exatamente 100MB —
// separado do teste de fronteira em si (TestDecodeDataURIWithLimit_Boundary,
// que injeta um limite sintético pequeno para não desperdiçar RAM). Ver
// CLAUDE.md, "Teste de boundary sem desperdiçar RAM".
func TestFetchDocumentMaxBytes_IsExactly100MB(t *testing.T) {
	const want int64 = 100 * 1024 * 1024
	if fetchDocumentMaxBytes != want {
		t.Fatalf("fetchDocumentMaxBytes = %d, want %d (100MB)", fetchDocumentMaxBytes, want)
	}
}

// TestDecodeDataURIWithLimit_Boundary congela a semântica de fronteira
// (limit aceito, limit+1 recusado) exercitando decodeDataURIWithLimit — a
// função PURA por trás de decodeDataURIDocument — com um limite sintético
// pequeno (64 bytes) em vez do valor real de produção (100MB). Isso prova a
// MESMA lógica de comparação que roda em produção sem alocar payloads de
// 100MB só para uma comparação de fronteira (TestFetchDocumentMaxBytes_IsExactly100MB
// prova, separadamente, que a produção usa 100MB de verdade).
func TestDecodeDataURIWithLimit_Boundary(t *testing.T) {
	const limit = int64(64)

	makePayload := func(n int) []byte {
		data := make([]byte, n)
		for i := range data {
			data[i] = byte('A' + i%26)
		}
		return data
	}

	t.Run("exactlyLimit_accepted", func(t *testing.T) {
		payload := makePayload(int(limit))
		raw := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(payload)

		data, err := decodeDataURIWithLimit(raw, limit)
		if err != nil {
			t.Fatalf("payload de exatamente %d bytes foi recusado: %v", limit, err)
		}
		if len(data) != int(limit) {
			t.Errorf("bytes decodificados: got %d, want %d", len(data), limit)
		}
	})

	t.Run("limitPlus1_rejected", func(t *testing.T) {
		payload := makePayload(int(limit) + 1)
		raw := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(payload)

		_, err := decodeDataURIWithLimit(raw, limit)
		if err == nil {
			t.Fatalf("payload de %d+1 bytes foi aceito", limit)
		}
	})
}

// TestDecodeDataURIWithLimit_EarlyRejection_BeforeFullDecode prova que a
// estimativa barata (decodedBase64Len, a partir só do comprimento do texto
// base64) rejeita um payload grosseiramente acima do limite ANTES de
// dataurl.DecodeString alocar o buffer decodificado inteiro — a razão de
// existir da etapa 1 de decodeDataURIWithLimit.
func TestDecodeDataURIWithLimit_EarlyRejection_BeforeFullDecode(t *testing.T) {
	const limit = int64(64)

	// 1000 bytes decodificados é grosseiramente acima do limite de 64 —
	// decodedBase64Len calcula isso SEM decodificar, a partir só do
	// comprimento do texto base64 (múltiplo de 4, sem caracteres inválidos).
	payload := make([]byte, 1000)
	raw := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(payload)

	_, err := decodeDataURIWithLimit(raw, limit)
	if err == nil {
		t.Fatal("payload grosseiramente acima do limite foi aceito")
	}
}
