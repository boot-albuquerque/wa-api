package message

import (
	"context"

	"github.com/vincent-petithory/dataurl"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// mediaDownloadFlow é o corpo único das cinco capabilities de download
// (Download Image, Download Video, Download Audio, Download Document e
// Download Sticker). Os cinco use cases exportados guardam um destes e
// delegam: a diferença entre eles é o domain.MediaKind e o rótulo de log,
// nada mais — copiar o corpo cinco vezes foi o que permitiu que os cinco
// stubs divergissem em silêncio antes de CAP-09B.
type mediaDownloadFlow struct {
	downloader appport.MediaDownloader
	logger     appport.Logger
	kind       domain.MediaKind
}

// execute valida, garante sessão, baixa os bytes pela porta e devolve o
// resultado já em Data URL.
//
// A ORDEM importa e é testada: validação de payload ANTES de EnsureSession
// (payload inválido é 400 do cliente e não deve depender de haver sessão), e
// EnsureSession ANTES de Download (download é operação protocolar).
//
// O MIME devolvido é o do REQUEST, preservando o contrato histórico
// (`git show 41bc8e2^:handlers.go`, DownloadImage na linha 3836): lá o
// protobuf era montado com `Mimetype: proto.String(t.Mimetype)` e a resposta
// usava `img.GetMimetype()` — ou seja, o mesmo valor que entrou. O prefixo da
// Data URL vem do MESMO valor, por construção (dataurl.New), então
// `Mimetype` e o prefixo de `Data` não podem divergir.
func (f mediaDownloadFlow) execute(ctx context.Context, txtID string, req domain.DownloadRequest) (*domain.DownloadResult, error) {
	if req.URL == "" && req.DirectPath == "" {
		return nil, apperr.New("missing_url_or_direct_path", apperr.CategoryValidation, "missing Url and DirectPath in payload", false, nil)
	}

	if err := f.downloader.EnsureSession(ctx, txtID); err != nil {
		f.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	data, err := f.downloader.Download(ctx, txtID, req.Descriptor(f.kind))
	if err != nil {
		f.logger.Error(ctx, "failed to download media", "txtID", txtID, "kind", string(f.kind), "error", err)
		return nil, apperr.New("media_download_failed", apperr.CategoryInternal, "failed to download media", false, err)
	}

	// Bytes vazios com erro nil NÃO viram sucesso — divergência CONSCIENTE do
	// histórico, registrada em HOUSEKEEP.md (F126).
	//
	// O histórico respondia 200 com `"Data":"data:image/jpeg;base64,"` nesse
	// caso, porque `dataurl.New(nil, mime).String()` é uma Data URL válida e
	// vazia. Investigando a primitive, o caso é alcançável mas sempre
	// patológico: em internal/wa-noise/capabilities/media/download_transport.go
	// (DownloadAndDecrypt) o retorno (data, nil) só ocorre depois de
	// ValidateMedia + Decrypt, e o único caminho que produz zero byte sem erro
	// é o ramo de mídia NÃO cifrada (mediaKey/fileEncSHA256/mac todos nil) com
	// corpo de resposta vazio — nenhum download real de /chat/download* passa
	// por ele, já que todos carregam MediaKey. Entregar 200 com corpo vazio
	// nomeia como sucesso algo que o cliente não consegue distinguir de mídia
	// legítima de zero byte; o contrato público promete conteúdo, então isto é
	// erro de servidor.
	if len(data) == 0 {
		f.logger.Error(ctx, "media download returned no bytes", "txtID", txtID, "kind", string(f.kind))
		return nil, apperr.New("empty_media", apperr.CategoryInternal, "media download returned no content", false, nil)
	}

	result := &domain.DownloadResult{
		Mimetype: req.Mimetype,
		Data:     dataurl.New(data, req.Mimetype).String(),
	}

	f.logger.Info(ctx, "media downloaded", "txtID", txtID, "kind", string(f.kind), "bytes", len(data))
	return result, nil
}
