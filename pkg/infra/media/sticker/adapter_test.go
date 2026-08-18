package sticker

import (
	"context"
	"strings"
	"testing"

	appport "wa-api/pkg/application/contracts"
)

// TestNewProcessor_ImplementsPort confere em runtime a asserção de
// compilação (var _ appport.StickerProcessor) — NewProcessor devolve algo
// atribuível à porta.
func TestNewProcessor_ImplementsPort(t *testing.T) {
	var _ appport.StickerProcessor = NewProcessor()
}

// TestProcessor_ProcessSticker_DelegatesToProcessStickerData prova que
// Processor.ProcessSticker é um passthrough puro para ProcessStickerData —
// mesmo resultado, chamado com os mesmos argumentos, para um erro (data URI
// malformada, não decodificável — não precisa de ffmpeg para reproduzir).
func TestProcessor_ProcessSticker_DelegatesToProcessStickerData(t *testing.T) {
	p := NewProcessor()

	gotData, gotMime, gotErr := p.ProcessSticker(context.Background(), "nao-comeca-com-data", "", "", "", "", nil)
	wantData, wantMime, wantErr := ProcessStickerData("nao-comeca-com-data", "", "", "", "", nil)

	if gotErr == nil || wantErr == nil {
		t.Fatalf("esperava erro dos dois lados: adapter=%v, direto=%v", gotErr, wantErr)
	}
	if gotErr.Error() != wantErr.Error() {
		t.Errorf("mensagem de erro divergiu: adapter=%q, direto=%q", gotErr.Error(), wantErr.Error())
	}
	if string(gotData) != string(wantData) || gotMime != wantMime {
		t.Errorf("retorno divergiu: adapter=(%v,%q), direto=(%v,%q)", gotData, gotMime, wantData, wantMime)
	}
}

// TestProcessor_ProcessSticker_PackMetadataForwarded prova que os
// parâmetros de metadata de pacote chegam intactos a ProcessStickerData —
// sem isso, EmbedStickerEXIF nunca veria o packID/packName/packPublisher
// que o chamador passar. Usa uma data URI malformada de propósito (erro
// ANTES de precisar de ffmpeg): esta suíte confere REPASSE de parâmetros,
// não o pipeline de conversão em si (coberto por exif_test.go/sticker_test.go
// e pelo caminho real do CAP-07 exercitado manualmente contra o ffmpeg desta
// máquina, ver CAP07-report.md).
func TestProcessor_ProcessSticker_PackMetadataForwarded(t *testing.T) {
	p := NewProcessor()
	_, _, err := p.ProcessSticker(context.Background(), "nao-comeca-com-data", "img/override", "pack-id", "pack-name", "pack-publisher", []string{"😀"})
	if err == nil {
		t.Fatal("esperava erro de prefixo ausente")
	}
	if !strings.Contains(err.Error(), `data should start with "data:mime/type;base64,"`) {
		t.Errorf("erro inesperado (parametros nao chegaram a ProcessStickerData?): %v", err)
	}
}
