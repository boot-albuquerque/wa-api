package message_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/message"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/media/opengraph"
)

// fetchAudioMaxBytesForTest espelha a constante interna fetchAudioMaxBytes
// de send_audio.go — 16MB, paridade com o wuzapi histórico (ver comentário
// naquele arquivo). Não exportada do pacote message de propósito; duplicada
// aqui só para a asserção do valor passado a MediaFetcher.FetchBytes.
const fetchAudioMaxBytesForTest int64 = 16 * 1024 * 1024

// oggBytes é um corpo binário que não é reconhecido pelo sniffer do Go
// (http.DetectContentType devolve "application/octet-stream" para ele,
// diferente de texto puro que sniffa como "text/plain") — usado para
// exercitar o nível 4 da precedência de MIME (fallback por PTT) e, nos
// demais testes, como corpo de áudio genérico.
var oggBytes = []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C}

const audioURL = "https://exemplo.com/nota-de-voz.ogg"

func audioDataURI(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func boolPtr(b bool) *bool { return &b }

// --- validação de campos obrigatórios ------------------------------------

func TestSendAudio_MissingRequiredField(t *testing.T) {
	cases := []struct {
		name string
		req  domain.SendAudioRequest
	}{
		{"Phone", domain.SendAudioRequest{Audio: audioURL}},
		{"Audio", domain.SendAudioRequest{Phone: "5511987654321"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			mf := &contractsfake.MediaFetcher{}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
				Execute(context.Background(), userID, tc.req)

			if err == nil {
				t.Fatalf("request sem %s foi aceito", tc.name)
			}
			if n := len(mm.EnsureSessionCalls); n != 0 {
				t.Errorf("validacao falhou mas a sessao foi consultada %d vez(es)", n)
			}
			if n := len(mf.FetchBytesCalls); n != 0 {
				t.Errorf("validacao falhou mas o fetch foi chamado %d vez(es)", n)
			}
			if n := len(mm.SendAudioCalls); n != 0 {
				t.Errorf("validacao falhou mas SendAudio foi chamado %d vez(es)", n)
			}
		})
	}
}

func TestSendAudio_SessionFailurePropagates(t *testing.T) {
	mm := &contractsfake.MediaMessenger{SessionGuard: contractsfake.FailSession(errSession)}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audioURL})

	if !errors.Is(err, errSession) {
		t.Fatalf("erro da porta nao chegou ao chamador: got %#v", err)
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Errorf("sem sessao, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendAudioCalls); n != 0 {
		t.Errorf("sem sessao, mas SendAudio foi chamado %d vez(es)", n)
	}
}

func TestSendAudio_InvalidPhoneNeverFetchesOrSends(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	jr := &contractsfake.JIDResolver{
		ResolveJIDFunc: func(context.Context, string) (domain.JID, error) { return "", errors.New("jid invalido") },
	}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendAudioUseCase(mm, jr, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "lixo", Audio: audioURL})

	if err == nil {
		t.Fatal("JID invalido foi aceito")
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Errorf("JID invalido, mas o fetch foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendAudioCalls); n != 0 {
		t.Errorf("JID invalido, mas SendAudio foi chamado %d vez(es)", n)
	}
}

// --- discriminação de transporte (ESTREITA: "data:audio/", não "data:") -

// TestSendAudio_UnsupportedSource_Rejected prova a discriminação estreita:
// nem base64 cru sem prefixo, nem scheme proibido, nem "data:" genérico
// (application/pdf, por exemplo, sem ser "data:audio/") são aceitos.
func TestSendAudio_UnsupportedSource_Rejected(t *testing.T) {
	cases := map[string]string{
		"raw_base64_no_prefix":     "AAAA",
		"unsupported_scheme":       "ftp://exemplo.com/nota.ogg",
		"data_uri_non_audio_mime":  "data:application/pdf;base64,AAAA",
		"data_uri_image_not_audio": "data:image/png;base64,AAAA",
	}
	for name, audio := range cases {
		t.Run(name, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			mf := &contractsfake.MediaFetcher{}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
				Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audio})

			if err == nil {
				t.Fatalf("fonte nao suportada %q foi aceita", audio)
			}
			if n := len(mf.FetchBytesCalls); n != 0 {
				t.Errorf("fonte nao suportada, mas o fetch foi chamado %d vez(es)", n)
			}
			if n := len(mm.SendAudioCalls); n != 0 {
				t.Errorf("fonte nao suportada, mas SendAudio foi chamado %d vez(es)", n)
			}
		})
	}
}

func TestSendAudio_FetchFailurePropagates(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return nil, "", errors.New("fetch: recusado")
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audioURL})

	if err == nil {
		t.Fatal("falha no fetch foi engolida")
	}
	if n := len(mm.SendAudioCalls); n != 0 {
		t.Errorf("fetch falhou, mas SendAudio foi chamado %d vez(es)", n)
	}
}

func TestSendAudio_EmptyBodyRejected(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return []byte{}, "audio/ogg", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audioURL})

	if err == nil {
		t.Fatal("corpo vazio foi aceito")
	}
	if n := len(mm.SendAudioCalls); n != 0 {
		t.Errorf("corpo vazio, mas SendAudio foi chamado %d vez(es)", n)
	}
}

// --- PTT: default TRUE quando ausente ------------------------------------

// TestSendAudio_PTT_DefaultTrueWhenAbsent é o teste que trava a semantica
// mais facil de inverter por engano: PTT ausente no request tem de chegar
// como true na AudioMessage — nao false.
func TestSendAudio_PTT_DefaultTrueWhenAbsent(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return oggBytes, "audio/ogg", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audioURL})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendAudioCalls[0].Payload.PTT; got != true {
		t.Errorf("PTT ausente: got %v, want true (default)", got)
	}
}

func TestSendAudio_PTT_ExplicitTrue(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return oggBytes, "audio/ogg", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audioURL, PTT: boolPtr(true)})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendAudioCalls[0].Payload.PTT; got != true {
		t.Errorf("PTT explicito true: got %v, want true", got)
	}
}

// TestSendAudio_PTT_ExplicitFalse prova que false explicito é respeitado —
// nil e false NAO sao equivalentes.
func TestSendAudio_PTT_ExplicitFalse(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return oggBytes, "audio/ogg", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audioURL, PTT: boolPtr(false)})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendAudioCalls[0].Payload.PTT; got != false {
		t.Errorf("PTT explicito false: got %v, want false", got)
	}
}

// --- precedência de MIME: 4 níveis ---------------------------------------

// TestSendAudio_MimeType_Level1_RequestFieldWins prova o nível 1: req.MimeType
// vence tudo, inclusive um Content-Type remoto de áudio.
func TestSendAudio_MimeType_Level1_RequestFieldWins(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return oggBytes, "audio/mpeg", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{
			Phone: "5511987654321", Audio: audioURL, MimeType: "audio/x-custom",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendAudioCalls[0].Payload.MimeType; got != "audio/x-custom" {
		t.Errorf("MimeType: got %q, want %q", got, "audio/x-custom")
	}
}

// TestSendAudio_MimeType_Level2_DataURILabelUsed prova o nível 2 no ramo
// data URI: o rótulo DECLARADO PELA PRÓPRIA data URI é usado quando
// req.MimeType está ausente — diferente de Image/Document, onde o rótulo
// nunca é confiável. Isso é FIEL ao histórico
// (`git show 41bc8e2^:handlers.go`, linha 1091: `detectedMime =
// dataURL.ContentType()`), não uma preferência deste projeto: mesmo que o
// rótulo minta sobre o conteúdo real, o resultado esperado é o rótulo.
func TestSendAudio_MimeType_Level2_DataURILabelUsed(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	// oggBytes nao sao realmente ogg; o rotulo "audio/ogg" da data URI
	// mente sobre o conteudo — e ainda assim tem de vencer, porque essa e'
	// a regra historica recuperada (nao uma preferencia nossa).
	uri := audioDataURI("audio/ogg", oggBytes)
	_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: uri})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendAudioCalls[0].Payload.MimeType; got != "audio/ogg" {
		t.Errorf("MimeType: got %q, want %q (rotulo da propria data URI, fiel ao historico)", got, "audio/ogg")
	}
}

// TestSendAudio_MimeType_Level2_RemoteContentTypeUsedOnlyWhenAudioPrefixed
// prova o nível 2 no ramo URL: o Content-Type remoto só alimenta o MIME
// quando começa com "audio/" — um Content-Type não-áudio é descartado e o
// fluxo cai para os níveis seguintes.
func TestSendAudio_MimeType_Level2_RemoteContentTypeUsedOnlyWhenAudioPrefixed(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return oggBytes, "audio/mp4", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audioURL})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendAudioCalls[0].Payload.MimeType; got != "audio/mp4" {
		t.Errorf("MimeType: got %q, want %q (Content-Type remoto de audio)", got, "audio/mp4")
	}
}

// TestSendAudio_MimeType_Level2_NonAudioRemoteContentTypeIgnored prova que
// um Content-Type remoto que NAO comeca com "audio/" e' descartado (nunca
// vira o MIME final) — o fluxo cai para o sniffing (nivel 3) ou o fallback
// por PTT (nivel 4).
func TestSendAudio_MimeType_Level2_NonAudioRemoteContentTypeIgnored(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return oggBytes, "text/html", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audioURL})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendAudioCalls[0].Payload.MimeType; got == "text/html" {
		t.Errorf("MimeType usou o Content-Type remoto nao-audio: %q", got)
	}
}

// TestSendAudio_MimeType_Level3_SniffedWhenRecognized prova o nível 3:
// sem req.MimeType e sem detectedMime, o sniffing dos bytes reais é usado
// quando o resultado NAO é "application/octet-stream".
func TestSendAudio_MimeType_Level3_SniffedWhenRecognized(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	plainText := []byte("isto e' texto puro, sniffavel como text/plain")
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return plainText, "", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audioURL})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	want := http.DetectContentType(plainText)
	if got := mm.SendAudioCalls[0].Payload.MimeType; got != want {
		t.Errorf("MimeType: got %q, want %q (sniffing real)", got, want)
	}
}

// TestSendAudio_MimeType_Level4_FallbackByPTT_WhenSniffFails prova o nível
// 4: quando req.MimeType, detectedMime estao ausentes E o sniffing devolve
// "application/octet-stream" (o caso comum para OGG/Opus/MP3/AAC/M4A, que o
// sniffer do Go nao reconhece), o MIME final vem do fallback por PTT.
func TestSendAudio_MimeType_Level4_FallbackByPTT_WhenSniffFails(t *testing.T) {
	// Confere a premissa do teste: http.DetectContentType(oggBytes) tem de
	// ser "application/octet-stream", senao o nivel 3 venceria e o teste
	// nao estaria exercitando o nivel 4.
	if got := http.DetectContentType(oggBytes); got != "application/octet-stream" {
		t.Fatalf("premissa do teste quebrada: oggBytes sniffa como %q, quero application/octet-stream", got)
	}

	cases := []struct {
		name string
		ptt  *bool
		want string
	}{
		{"ptt_true_default", nil, "audio/ogg; codecs=opus"},
		{"ptt_true_explicit", boolPtr(true), "audio/ogg; codecs=opus"},
		{"ptt_false", boolPtr(false), "audio/mpeg"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mm := &contractsfake.MediaMessenger{}
			mf := &contractsfake.MediaFetcher{
				FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
					return oggBytes, "", nil
				},
			}
			logger := &contractsfake.Logger{}

			_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
				Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audioURL, PTT: tc.ptt})

			if err != nil {
				t.Fatalf("caminho feliz falhou: %v", err)
			}
			if got := mm.SendAudioCalls[0].Payload.MimeType; got != tc.want {
				t.Errorf("MimeType: got %q, want %q", got, tc.want)
			}
		})
	}
}

// --- Seconds ---------------------------------------------------------------

func TestSendAudio_Seconds_ForwardedFromRequest(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return oggBytes, "audio/ogg", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audioURL, Seconds: 42})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if got := mm.SendAudioCalls[0].Payload.Seconds; got != 42 {
		t.Errorf("Seconds: got %d, want %d", got, 42)
	}
}

// --- upload: MediaAudio ----------------------------------------------------

// TestSendAudio_Upload_UsesMediaAudio prova que o payload chega a SendAudio
// (que o adapter usa com wanoise.MediaAudio no upload) — a garantia de que
// audio nunca sobe como wanoise.MediaDocument ou wanoise.MediaImage é
// verificada no controle negativo do adapter (ver messenger.go/HOUSEKEEP).
// Aqui, no nivel do use case, o que se prova e' que o Payload.Bytes chega
// intacto a domain.AudioPayload — a fronteira que o adapter consome.
func TestSendAudio_Upload_BytesForwardedIntact(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return oggBytes, "audio/ogg", nil
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audioURL})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if string(mm.SendAudioCalls[0].Payload.Bytes) != string(oggBytes) {
		t.Error("bytes repassados a SendAudio divergem dos buscados")
	}
}

// --- teste da causa: fluxo completo, metadata real, resultado ------------

// TestSendAudio_CausalSuccess é o teste da causa: um envio bem-sucedido
// OBRIGATORIAMENTE busca a URL com o limite correto, sobe exatamente os
// bytes buscados com destinatário e PTT corretos, e Status só vale
// StatusSent DEPOIS que a porta de envio devolveu sucesso — metadata do
// resultado (MessageID, Timestamp) vem do que a porta REALMENTE devolveu.
func TestSendAudio_CausalSuccess(t *testing.T) {
	sentAt := int64(1755500080)
	mm := &contractsfake.MediaMessenger{
		SendAudioFunc: func(_ context.Context, _ string, target domain.JID, payload domain.AudioPayload, id string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-id-audio", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(_ context.Context, resourceURL string, limit int64) ([]byte, string, error) {
			if resourceURL != audioURL {
				t.Errorf("URL buscada: got %q, want %q", resourceURL, audioURL)
			}
			if limit != fetchAudioMaxBytesForTest {
				t.Errorf("limite: got %d, want %d", limit, fetchAudioMaxBytesForTest)
			}
			return oggBytes, "audio/ogg", nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{
			Phone: "5511987654321", Audio: audioURL, Seconds: 7,
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(mm.SendAudioCalls); n != 1 {
		t.Fatalf("SendAudio chamado %d vez(es), quero exatamente 1", n)
	}
	call := mm.SendAudioCalls[0]
	if call.Target != domain.JID("5511987654321@s.whatsapp.net") {
		t.Errorf("destinatario: got %q, want %q", call.Target, "5511987654321")
	}
	if string(call.Payload.Bytes) != string(oggBytes) {
		t.Errorf("bytes repassados a SendAudio divergem dos buscados")
	}
	if call.Payload.PTT != true {
		t.Errorf("PTT: got %v, want true (default)", call.Payload.PTT)
	}
	if call.Payload.Seconds != 7 {
		t.Errorf("Seconds: got %d, want 7", call.Payload.Seconds)
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if result.MessageID != "wire-id-audio" {
		t.Errorf("MessageID: got %q, want o ID devolvido pela porta (%q)", result.MessageID, "wire-id-audio")
	}
	if result.Timestamp != sentAt {
		t.Errorf("Timestamp: got %v, want %v", result.Timestamp, sentAt)
	}
}

func TestSendAudio_ClientSuppliedIDIsForwardedButServerIDWins(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendAudioFunc: func(_ context.Context, _ string, _ domain.JID, _ domain.AudioPayload, id string) (domain.MessageSendResult, error) {
			if id != "id-do-cliente" {
				t.Errorf("id repassado a porta: got %q, want %q", id, "id-do-cliente")
			}
			return domain.MessageSendResult{ID: "id-que-o-sdk-usou"}, nil
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return oggBytes, "audio/ogg", nil },
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{
			Phone: "5511987654321", Audio: audioURL, ID: "id-do-cliente",
		})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if result.MessageID != "id-que-o-sdk-usou" {
		t.Errorf("MessageID: got %q, want %q", result.MessageID, "id-que-o-sdk-usou")
	}
}

// TestSendAudio_DownstreamFailureNeverProducesSent: SendAudio (upload e/ou
// envio) falhando nunca pode virar Status=StatusSent nem resultado não-nil
// — a garantia central anti-falso-sucesso, agora para áudio.
func TestSendAudio_DownstreamFailureNeverProducesSent(t *testing.T) {
	mm := &contractsfake.MediaMessenger{
		SendAudioFunc: func(context.Context, string, domain.JID, domain.AudioPayload, string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{}, errDownstream
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return oggBytes, "audio/ogg", nil },
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audioURL})

	if err == nil {
		t.Fatal("falha da porta foi engolida")
	}
	if !errors.Is(err, errDownstream) {
		t.Fatalf("causa nao propagada: got %#v", err)
	}
	if result != nil {
		t.Errorf("resultado nao-nil devolvido junto com erro: %+v", result)
	}
}

// TestSendAudio_AcquisitionOK_UploadOK_SendFail_NeverSent é o caso
// obrigatório: aquisição (fetch/decode) OK e upload OK (simulado dentro do
// SendAudioFunc, que representa a porta upload+envio) seguidos de falha de
// envio NÃO produzem Status=sent — sem rollback de upload inventado.
func TestSendAudio_AcquisitionOK_UploadOK_SendFail_NeverSent(t *testing.T) {
	sendErr := errors.New("sendmessage: boom apos upload bem-sucedido")
	uploadThenSendCalled := false
	mm := &contractsfake.MediaMessenger{
		SendAudioFunc: func(context.Context, string, domain.JID, domain.AudioPayload, string) (domain.MessageSendResult, error) {
			uploadThenSendCalled = true
			return domain.MessageSendResult{}, sendErr
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) { return oggBytes, "audio/ogg", nil },
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audioURL})

	if !uploadThenSendCalled {
		t.Fatal("upload+envio nunca foi chamado — teste nao exercita o caso aquisicao-ok-upload-ok-send-fail")
	}
	if !errors.Is(err, sendErr) {
		t.Fatalf("falha de envio nao propagada: got %#v", err)
	}
	if result != nil {
		t.Errorf("resultado nao-nil com envio falho: %+v", result)
	}
}

// --- ramo data URI ------------------------------------------------------

// TestSendAudio_DataURI_CausalSuccess é o teste da causa para o ramo data
// URI: prova que uma data URI válida é decodificada LOCALMENTE (o
// MediaFetcher nunca é tocado), os bytes decodificados batem com os
// originais, e o resultado só vira StatusSent depois que SendAudio confirma
// sucesso.
func TestSendAudio_DataURI_CausalSuccess(t *testing.T) {
	sentAt := int64(1755500090)
	mm := &contractsfake.MediaMessenger{
		SendAudioFunc: func(_ context.Context, _ string, target domain.JID, payload domain.AudioPayload, id string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-id-audio-datauri", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	uri := audioDataURI("audio/ogg", oggBytes)
	result, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: uri})

	if err != nil {
		t.Fatalf("caminho feliz falhou: %v", err)
	}
	if n := len(mf.FetchBytesCalls); n != 0 {
		t.Fatalf("data URI, mas MediaFetcher foi chamado %d vez(es)", n)
	}
	if n := len(mm.SendAudioCalls); n != 1 {
		t.Fatalf("SendAudio chamado %d vez(es), quero exatamente 1", n)
	}
	if string(mm.SendAudioCalls[0].Payload.Bytes) != string(oggBytes) {
		t.Error("bytes decodificados divergem dos originais")
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
}

func TestSendAudio_DataURI_MalformedBase64_Rejected(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{
			Phone: "5511987654321", Audio: "data:audio/ogg;base64,%%%nao-e-base64%%%",
		})

	if err == nil {
		t.Fatal("base64 malformado foi aceito")
	}
	if n := len(mm.SendAudioCalls); n != 0 {
		t.Fatalf("base64 malformado, mas SendAudio foi chamado %d vez(es)", n)
	}
}

// TestSendAudio_URLBranch_NotCapturedByDataURIDiscrimination é o teste de
// conservação: uma URL http(s) comum não pode ser capturada pela
// discriminação "data:audio/".
func TestSendAudio_URLBranch_NotCapturedByDataURIDiscrimination(t *testing.T) {
	sentAt := int64(1755500100)
	mm := &contractsfake.MediaMessenger{
		SendAudioFunc: func(_ context.Context, _ string, target domain.JID, payload domain.AudioPayload, id string) (domain.MessageSendResult, error) {
			return domain.MessageSendResult{ID: "wire-id-url-conservation", Timestamp: time.Unix(sentAt, 0)}, nil
		},
	}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return oggBytes, "audio/ogg", nil
		},
	}
	logger := &contractsfake.Logger{}

	result, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audioURL})

	if err != nil {
		t.Fatalf("URL comum foi recusada pela discriminacao: %v", err)
	}
	if n := len(mf.FetchBytesCalls); n != 1 {
		t.Fatalf("URL comum devia acionar MediaFetcher exatamente 1 vez, foi %d", n)
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
}

// --- boundary de 16MB, sem alocar 16MB ------------------------------------

// TestSendAudio_DataURI_Boundary_16MB_UsesPureLimitFunc prova a fronteira
// exata 16MB/16MB+1 injetando um LIMITE SINTETICO pequeno na funcao pura
// decodeAudioDataURIWithLimit (via um payload pequeno que so' e' testado
// contra um teto artificial) — sem alocar 16MB de verdade. A funcao pura
// nao e' exportada; este teste vive no MESMO pacote (message, nao
// message_test) para poder chama-la diretamente.
//
// Este teste fica em send_audio_internal_test.go (pacote message) — ver
// ali TestDecodeAudioDataURIWithLimit_Boundary e
// TestSendAudio_FetchAudioMaxBytesConstant.

// TestSendAudio_DataURI_TooLarge_ViaMediaFetcherLimit prova que o LIMITE DE
// PRODUCAO tambem se aplica ao ramo URL: FetchBytes recebe fetchAudioMaxBytes
// como teto (ja' coberto por TestSendAudio_CausalSuccess, que afirma o valor
// exato). Este teste cobre o caminho de ERRO: MediaFetcher que recusa por
// estourar o limite tem a falha propagada, nunca engolida.
func TestSendAudio_URL_FetchTooLarge_Rejected(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := &contractsfake.MediaFetcher{
		FetchBytesFunc: func(context.Context, string, int64) ([]byte, string, error) {
			return nil, "", errors.New("corpo excede o limite de bytes")
		},
	}
	logger := &contractsfake.Logger{}

	_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: audioURL})

	if err == nil {
		t.Fatal("fetch acima do limite foi aceito")
	}
	if n := len(mm.SendAudioCalls); n != 0 {
		t.Errorf("fetch acima do limite, mas SendAudio foi chamado %d vez(es)", n)
	}
}

// --- integração com fetch real / SSRF -------------------------------------

// TestSendAudio_RealFetchIntegration exercita o use case com um
// appport.MediaFetcher REAL (opengraph.URLFetcher) contra um
// httptest.Server — não um dublê do fetch. Cobre a integração completa,
// incluindo a passagem pela seam SSRF-safe já provada em CAP-02.
func TestSendAudio_RealFetchIntegration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/ogg")
		_, _ = w.Write(oggBytes)
	}))
	defer srv.Close()

	mm := &contractsfake.MediaMessenger{}
	mf := opengraph.NewURLFetcher(srv.Client())
	logger := &contractsfake.Logger{}

	result, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{Phone: "5511987654321", Audio: srv.URL})

	if err != nil {
		t.Fatalf("integracao com fetch real falhou: %v", err)
	}
	if result.Status != domain.StatusSent {
		t.Errorf("Status: got %q, want %q", result.Status, domain.StatusSent)
	}
	if n := len(mm.SendAudioCalls); n != 1 {
		t.Fatalf("SendAudio chamado %d vez(es), quero exatamente 1", n)
	}
	if string(mm.SendAudioCalls[0].Payload.Bytes) != string(oggBytes) {
		t.Error("bytes buscados pelo fetch real divergem do corpo servido")
	}
}

// TestSendAudio_SSRF_LoopbackBlocked prova que Audio passa pela MESMA seam
// SSRF-safe já provada em CAP-02 — não um caminho de fetch paralelo.
func TestSendAudio_SSRF_LoopbackBlocked(t *testing.T) {
	mm := &contractsfake.MediaMessenger{}
	mf := opengraph.NewURLFetcher(http.DefaultClient)
	logger := &contractsfake.Logger{}

	_, err := message.NewSendAudioUseCase(mm, &contractsfake.JIDResolver{}, mf, logger).
		Execute(context.Background(), userID, domain.SendAudioRequest{
			Phone: "5511987654321", Audio: "http://127.0.0.1:1/nota.ogg",
		})

	if err == nil {
		t.Fatal("URL loopback foi aceita")
	}
	if n := len(mm.SendAudioCalls); n != 0 {
		t.Errorf("loopback deveria ser bloqueado antes de SendAudio, mas foi chamado %d vez(es)", n)
	}
}
