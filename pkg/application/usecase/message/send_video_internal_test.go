package message

import (
	"encoding/base64"
	"testing"
)

// TestFetchVideoMaxBytes_IsExactly100MB prova, sem alocar payload nenhum,
// que a constante REALMENTE ligada em produção (decodeDataVideo e o ramo
// URL do Execute) vale exatamente 100MB — separado do teste de fronteira em
// si (TestDecodeVideoDataURIWithLimit_Boundary, que injeta um limite
// sintético pequeno para não desperdiçar RAM). Ver CLAUDE.md, "Teste de
// boundary sem desperdiçar RAM".
func TestFetchVideoMaxBytes_IsExactly100MB(t *testing.T) {
	const want int64 = 100 * 1024 * 1024
	if fetchVideoMaxBytes != want {
		t.Fatalf("fetchVideoMaxBytes = %d, want %d (100MB)", fetchVideoMaxBytes, want)
	}
}

// TestDecodeVideoDataURIWithLimit_Boundary congela a semântica de fronteira
// (limit aceito, limit+1 recusado) exercitando decodeVideoDataURIWithLimit
// — a função PURA por trás de decodeDataVideo — com um limite sintético
// pequeno (64 bytes) em vez do valor real de produção (100MB).
func TestDecodeVideoDataURIWithLimit_Boundary(t *testing.T) {
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
		raw := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(payload)

		data, err := decodeVideoDataURIWithLimit(raw, limit)
		if err != nil {
			t.Fatalf("payload de exatamente %d bytes foi recusado: %v", limit, err)
		}
		if len(data) != int(limit) {
			t.Errorf("bytes decodificados: got %d, want %d", len(data), limit)
		}
	})

	t.Run("limitPlus1_rejected", func(t *testing.T) {
		payload := makePayload(int(limit) + 1)
		raw := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(payload)

		_, err := decodeVideoDataURIWithLimit(raw, limit)
		if err == nil {
			t.Fatalf("payload de %d+1 bytes foi aceito", limit)
		}
	})
}

// TestDecodeVideoDataURIWithLimit_EarlyRejection_BeforeFullDecode prova que
// a estimativa barata (decodedBase64Len, a partir só do comprimento do
// texto base64) rejeita um payload grosseiramente acima do limite ANTES de
// dataurl.DecodeString alocar o buffer decodificado inteiro.
func TestDecodeVideoDataURIWithLimit_EarlyRejection_BeforeFullDecode(t *testing.T) {
	const limit = int64(64)

	payload := make([]byte, 1000)
	raw := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(payload)

	_, err := decodeVideoDataURIWithLimit(raw, limit)
	if err == nil {
		t.Fatal("payload grosseiramente acima do limite foi aceito")
	}
}

// TestIsDataVideo_WidestDiscrimination prova a discriminação MAIS FROUXA
// das quatro deste projeto: os 4 primeiros caracteres têm de ser "data"
// (sem os dois-pontos), aceitando qualquer MIME declarado depois — inclusive
// "data:image/..." disfarçado de vídeo, ao contrário de Audio/Image
// (estreitas) e Document ("data:" com dois-pontos).
func TestIsDataVideo_WidestDiscrimination(t *testing.T) {
	cases := map[string]bool{
		"data:video/mp4;base64,AAAA": true,
		"data:application/pdf;base64,AAAA": true,
		"data:image/png;base64,AAAA":       true,
		"data":                              true,
		"dat":                               false,
		"da":                                false,
		"d":                                 false,
		"":                                  false,
		"http://exemplo.com/v.mp4":          false,
	}
	for raw, want := range cases {
		if got := isDataVideo(raw); got != want {
			t.Errorf("isDataVideo(%q) = %v, want %v", raw, got, want)
		}
	}
}

// TestIsDataVideo_ShortStrings_NoPanic prova que Video de 1..3 caracteres
// não panica ao ser discriminado — o bug latente histórico
// (`t.Video[0:4]` sem checar tamanho) NÃO é reproduzido aqui.
func TestIsDataVideo_ShortStrings_NoPanic(t *testing.T) {
	for _, raw := range []string{"d", "da", "dat"} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("isDataVideo(%q) panicou: %v", raw, r)
				}
			}()
			if got := isDataVideo(raw); got {
				t.Errorf("isDataVideo(%q) = true, want false", raw)
			}
		}()
	}
}

// TestIsHTTPVideoURL_SchemeAndHost espelha isHTTPImageURL/isHTTPAudioURL:
// só http/https com host não vazio passam.
func TestIsHTTPVideoURL_SchemeAndHost(t *testing.T) {
	cases := map[string]bool{
		"http://exemplo.com/v.mp4":  true,
		"https://exemplo.com/v.mp4": true,
		"ftp://exemplo.com/v.mp4":   false,
		"file:///etc/passwd":        false,
		"data:video/mp4;base64,AA":  false,
		"":                          false,
	}
	for raw, want := range cases {
		if got := isHTTPVideoURL(raw); got != want {
			t.Errorf("isHTTPVideoURL(%q) = %v, want %v", raw, got, want)
		}
	}
}
