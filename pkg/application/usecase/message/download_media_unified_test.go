package message

import (
	"context"
	"errors"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

func newDownloadMediaUseCase(md *contractsfake.MediaDownloader) *DownloadMediaUseCase {
	l := &contractsfake.Logger{}
	return NewDownloadMediaUseCase(
		NewDownloadImageUseCase(md, l),
		NewDownloadVideoUseCase(md, l),
		NewDownloadAudioUseCase(md, l),
		NewDownloadDocumentUseCase(md, l),
		NewDownloadStickerUseCase(md, l),
	)
}

// TestDownloadMediaUseCase_DispatchesByKind prova que cada um dos cinco
// kinds chega à porta com o MediaKind correspondente — o mapeamento kind ->
// use case que NewDownloadMediaUseCase monta.
func TestDownloadMediaUseCase_DispatchesByKind(t *testing.T) {
	kinds := []domain.MediaKind{
		domain.MediaKindImage, domain.MediaKindVideo, domain.MediaKindAudio,
		domain.MediaKindDocument, domain.MediaKindSticker,
	}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			var gotKind domain.MediaKind
			md := &contractsfake.MediaDownloader{
				DownloadFunc: func(_ context.Context, _ string, desc domain.MediaDescriptor) ([]byte, error) {
					gotKind = desc.Kind
					return []byte{0x01}, nil
				},
			}
			uc := newDownloadMediaUseCase(md)
			req := domain.DownloadRequest{Kind: kind, URL: "https://example.invalid/m", Mimetype: "application/octet-stream"}

			if _, err := uc.Execute(context.Background(), "user-1", req); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if gotKind != kind {
				t.Errorf("Kind: got %q, want %q", gotKind, kind)
			}
		})
	}
}

// TestDownloadMediaUseCase_UnknownKind_RejectsBeforeDownloader: kind fora do
// mapa é 400 unknown_media_kind, e a porta de download NUNCA é alcançada —
// nem para validar Url/DirectPath, que é o que os cinco use cases delegados
// fariam a seguir.
func TestDownloadMediaUseCase_UnknownKind_RejectsBeforeDownloader(t *testing.T) {
	md := &contractsfake.MediaDownloader{}
	uc := newDownloadMediaUseCase(md)

	_, err := uc.Execute(context.Background(), "user-1", domain.DownloadRequest{Kind: "carrierpigeon"})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("erro nao e' *apperr.AppError: %v", err)
	}
	if appErr.Code != CodeUnknownMediaKind {
		t.Errorf("Code: got %q, want %q", appErr.Code, CodeUnknownMediaKind)
	}
	if appErr.Category != apperr.CategoryValidation {
		t.Errorf("Category: got %q, want %q", appErr.Category, apperr.CategoryValidation)
	}
	if n := len(md.EnsureSessionCalls); n != 0 {
		t.Errorf("kind desconhecido alcancou EnsureSession %d vez(es)", n)
	}
	if n := len(md.DownloadCalls); n != 0 {
		t.Errorf("kind desconhecido alcancou Download %d vez(es)", n)
	}
}

// TestDownloadMediaUseCase_EmptyKind_Rejects: Kind vazio (zero value) e' o
// mesmo estado que um kind desconhecido — nao ha' entrada "" no mapa.
func TestDownloadMediaUseCase_EmptyKind_Rejects(t *testing.T) {
	md := &contractsfake.MediaDownloader{}
	uc := newDownloadMediaUseCase(md)

	_, err := uc.Execute(context.Background(), "user-1", domain.DownloadRequest{})

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Code != CodeUnknownMediaKind {
		t.Fatalf("Kind vazio: got %v, want %q", err, CodeUnknownMediaKind)
	}
}
