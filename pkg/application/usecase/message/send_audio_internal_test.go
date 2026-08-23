package message

import (
	"encoding/base64"
	"testing"
)

// TestFetchAudioMaxBytes_IsExactly16MB prova, sem alocar payload nenhum,
// que a constante REALMENTE ligada em produção (decodeDataURIAudio e o ramo
// URL do Execute) vale exatamente 16MB — separado do teste de fronteira em
// si (TestDecodeAudioDataURIWithLimit_Boundary, que injeta um limite
// sintético pequeno para não desperdiçar RAM). Ver CLAUDE.md, "Teste de
// boundary sem desperdiçar RAM". 16MB, não 100MB (Document): áudio tem
// regra própria.
func TestFetchAudioMaxBytes_IsExactly16MB(t *testing.T) {
	const want int64 = 16 * 1024 * 1024
	if fetchAudioMaxBytes != want {
		t.Fatalf("fetchAudioMaxBytes = %d, want %d (16MB)", fetchAudioMaxBytes, want)
	}
}

// TestDecodeAudioDataURIWithLimit_Boundary congela a semântica de fronteira
// (limit aceito, limit+1 recusado) exercitando decodeAudioDataURIWithLimit
// — a função PURA por trás de decodeDataURIAudio — com um limite sintético
// pequeno (64 bytes) em vez do valor real de produção (16MB).
func TestDecodeAudioDataURIWithLimit_Boundary(t *testing.T) {
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
		raw := "data:audio/ogg;base64," + base64.StdEncoding.EncodeToString(payload)

		data, mime, err := decodeAudioDataURIWithLimit(raw, limit)
		if err != nil {
			t.Fatalf("payload de exatamente %d bytes foi recusado: %v", limit, err)
		}
		if len(data) != int(limit) {
			t.Errorf("bytes decodificados: got %d, want %d", len(data), limit)
		}
		if mime != "audio/ogg" {
			t.Errorf("mime label: got %q, want %q", mime, "audio/ogg")
		}
	})

	t.Run("limitPlus1_rejected", func(t *testing.T) {
		payload := makePayload(int(limit) + 1)
		raw := "data:audio/ogg;base64," + base64.StdEncoding.EncodeToString(payload)

		_, _, err := decodeAudioDataURIWithLimit(raw, limit)
		if err == nil {
			t.Fatalf("payload de %d+1 bytes foi aceito", limit)
		}
	})
}

// TestDecodeAudioDataURIWithLimit_EarlyRejection_BeforeFullDecode prova que
// a estimativa barata (decodedBase64Len, a partir só do comprimento do
// texto base64) rejeita um payload grosseiramente acima do limite ANTES de
// dataurl.DecodeString alocar o buffer decodificado inteiro.
func TestDecodeAudioDataURIWithLimit_EarlyRejection_BeforeFullDecode(t *testing.T) {
	const limit = int64(64)

	payload := make([]byte, 1000)
	raw := "data:audio/ogg;base64," + base64.StdEncoding.EncodeToString(payload)

	_, _, err := decodeAudioDataURIWithLimit(raw, limit)
	if err == nil {
		t.Fatal("payload grosseiramente acima do limite foi aceito")
	}
}

// TestResolveAudioMimeType_PrecedenceLevels prova, diretamente na função
// pura, os 4 níveis de precedência sem depender de fetch/decode reais.
func TestResolveAudioMimeType_PrecedenceLevels(t *testing.T) {
	recognizableBytes := []byte("plain text, sniffavel")
	unrecognizableBytes := []byte{0x00, 0x01, 0x02, 0x03}

	cases := []struct {
		name         string
		reqMimeType  string
		detectedMime string
		data         []byte
		ptt          bool
		want         string
	}{
		{"level1_reqMimeType_wins_over_everything", "audio/x-custom", "audio/ogg", recognizableBytes, true, "audio/x-custom"},
		{"level2_detectedMime_wins_over_sniff_and_ptt", "", "audio/mp4", recognizableBytes, true, "audio/mp4"},
		{"level3_sniff_wins_when_recognized", "", "", recognizableBytes, true, "text/plain; charset=utf-8"},
		{"level4_fallback_ptt_true", "", "", unrecognizableBytes, true, "audio/ogg; codecs=opus"},
		{"level4_fallback_ptt_false", "", "", unrecognizableBytes, false, "audio/mpeg"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveAudioMimeType(tc.reqMimeType, tc.detectedMime, tc.data, tc.ptt)
			if got != tc.want {
				t.Errorf("resolveAudioMimeType(%q, %q, _, %v) = %q, want %q", tc.reqMimeType, tc.detectedMime, tc.ptt, got, tc.want)
			}
		})
	}
}

// TestIsDataURIAudio_NarrowDiscrimination prova que a discriminação é
// ESTREITA ("data:audio/" literal), não "data:" genérico.
func TestIsDataURIAudio_NarrowDiscrimination(t *testing.T) {
	cases := map[string]bool{
		"data:audio/ogg;base64,AAAA":       true,
		"data:audio/mpeg;base64,AAAA":      true,
		"data:application/pdf;base64,AAAA": false,
		"data:image/png;base64,AAAA":       false,
		"data:audio":                       false,
		"":                                 false,
	}
	for raw, want := range cases {
		if got := isDataURIAudio(raw); got != want {
			t.Errorf("isDataURIAudio(%q) = %v, want %v", raw, got, want)
		}
	}
}
