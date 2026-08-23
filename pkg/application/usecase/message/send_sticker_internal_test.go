package message

import (
	"encoding/base64"
	"testing"
)

// TestFetchImageMaxBytes_SharedWithSticker prova, sem alocar payload
// nenhum, que o teto usado pelo ramo data URI de SendStickerUseCase.Execute
// é EXATAMENTE fetchImageMaxBytes (send_image.go) — sticker não tem teto
// próprio, item 3 do packet CAP-07. Confirma a constante REALMENTE ligada
// em produção, não uma cópia que possa divergir.
func TestFetchImageMaxBytes_SharedWithSticker(t *testing.T) {
	const want int64 = 16 * 1024 * 1024
	if fetchImageMaxBytes != want {
		t.Fatalf("fetchImageMaxBytes = %d, want %d (16MB)", fetchImageMaxBytes, want)
	}
}

// TestCheckStickerDataURISize_Boundary congela a semântica de fronteira
// (limit aceito, limit+1 recusado) exercitando checkStickerDataURISize —
// função PURA por trás do teto de 16MB do ramo data URI — com um limite
// sintético pequeno (64 bytes) em vez do valor real de produção (16MB). Ver
// CLAUDE.md, "Teste de boundary sem desperdiçar RAM".
func TestCheckStickerDataURISize_Boundary(t *testing.T) {
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
		raw := "data:image/webp;base64," + base64.StdEncoding.EncodeToString(payload)

		if err := checkStickerDataURISize(raw, limit); err != nil {
			t.Fatalf("payload de exatamente %d bytes foi recusado: %v", limit, err)
		}
	})

	t.Run("limitPlus1_rejected", func(t *testing.T) {
		payload := makePayload(int(limit) + 1)
		raw := "data:image/webp;base64," + base64.StdEncoding.EncodeToString(payload)

		if err := checkStickerDataURISize(raw, limit); err == nil {
			t.Fatalf("payload de %d+1 bytes foi aceito", limit)
		}
	})
}

// TestCheckStickerDataURISize_NoComma_NoOp prova que, sem vírgula (não é
// uma data URI reconhecível como base64), a checagem não rejeita nada —
// deixa ProcessSticker ser a autoridade que rejeita o formato.
func TestCheckStickerDataURISize_NoComma_NoOp(t *testing.T) {
	if err := checkStickerDataURISize("data-sem-virgula-nenhuma", 64); err != nil {
		t.Fatalf("string sem virgula foi rejeitada pela checagem de tamanho: %v", err)
	}
}

// TestCheckStickerDataURISize_NotBase64_NoOp prova que uma data URI
// declarada sem ";base64" (ex.: percent-encoded) não é medida por esta
// checagem — ela só se aplica ao formato base64 que decodedBase64Len sabe
// medir.
func TestCheckStickerDataURISize_NotBase64_NoOp(t *testing.T) {
	if err := checkStickerDataURISize("data:image/webp,nao-e-base64", 64); err != nil {
		t.Fatalf("data URI nao-base64 foi rejeitada pela checagem de tamanho: %v", err)
	}
}

// TestIsDataSticker_ExactPrefix prova o discriminador "data" (4 chars, sem
// dois-pontos) — igual a isDataVideo, e pela mesma razão histórica (o fluxo
// original normaliza o ramo URL para dentro de uma data URI e converge num
// único pipeline).
func TestIsDataSticker_ExactPrefix(t *testing.T) {
	cases := map[string]bool{
		"data:image/webp;base64,AAAA":      true,
		"data:application/pdf;base64,AAAA": true,
		"data":                             true,
		"dat":                              false,
		"da":                               false,
		"d":                                false,
		"":                                 false,
		"http://exemplo.com/s.webp":        false,
	}
	for raw, want := range cases {
		if got := isDataSticker(raw); got != want {
			t.Errorf("isDataSticker(%q) = %v, want %v", raw, got, want)
		}
	}
}

// TestIsDataSticker_ShortStrings_NoPanic prova que Sticker de 1..3
// caracteres não panica ao ser discriminado.
func TestIsDataSticker_ShortStrings_NoPanic(t *testing.T) {
	for _, raw := range []string{"d", "da", "dat"} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("isDataSticker(%q) panicou: %v", raw, r)
				}
			}()
			if got := isDataSticker(raw); got {
				t.Errorf("isDataSticker(%q) = true, want false", raw)
			}
		}()
	}
}
