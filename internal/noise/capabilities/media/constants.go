// Package media implementa o caminho de midia do fork (download, upload,
// media connection e retry de midia) como funcoes livres sobre a interface
// estreita [Transport], sem depender do pacote raiz noise.
//
// Ver ADR-0004 (docs/adr/0004-refatorar-internal-waclient-em-fork-intencional.md)
// e a secao "Fase F/G — lote 1" de internal/noise/PATCHES.md.
package media

import "time"

// Constantes compartilhadas pelo caminho de midia do fork. Os valores sao
// exatamente os que estavam em internal/noise/media_constants.go (Fase E);
// a extracao para este pacote nao mudou nenhum deles.

// Layout do material de chave derivado de mediaKey via HKDF-SHA256.
// O protocolo do WhatsApp expande a mediaKey em 112 bytes e fatia esse buffer
// em IV | cipherKey | macKey | refKey, nessa ordem.
const (
	// mediaKeyLength e' o tamanho da mediaKey aleatoria gerada no upload.
	mediaKeyLength = 32
	// mediaKeyExpandedLength e' o total derivado por HKDF (soma das quatro fatias).
	mediaKeyExpandedLength = 112

	mediaIVLength        = 16
	mediaCipherKeyLength = 32
	mediaMACKeyLength    = 32

	mediaIVEnd        = mediaIVLength
	mediaCipherKeyEnd = mediaIVEnd + mediaCipherKeyLength
	mediaMACKeyEnd    = mediaCipherKeyEnd + mediaMACKeyLength
)

// mediaHMACLength e' quantos bytes do HMAC-SHA256 sao anexados ao ciphertext
// da midia (truncado) — tanto na leitura quanto na escrita.
const mediaHMACLength = 10

// sha256HashLength e' o tamanho de um digest SHA-256 completo, usado para
// decidir se um hash recebido do servidor tem tamanho comparavel.
const sha256HashLength = 32

// Politica de retry do download de midia.
const (
	mediaDownloadMaxRetries = 5
	// mediaDownloadRetryStep e' a base do backoff linear: a espera da tentativa
	// N e' (N+1) * mediaDownloadRetryStep, salvo header Retry-After do servidor.
	mediaDownloadRetryStep = time.Second
)

// UnknownFileLength e' o sentinela de "tamanho do arquivo desconhecido"
// passado a fileLength; qualquer valor negativo desliga a validacao de tamanho.
const UnknownFileLength = -1

// webWhatsappNetURLPrefix marca URLs de midia que o servidor as vezes devolve
// mas que nao sao baixaveis diretamente — nesse caso o directPath e' obrigatorio.
const webWhatsappNetURLPrefix = "https://web.whatsapp.net"

// mediaDownloadURLFormat monta a URL de download a partir do host da
// mediaConn, do directPath, do hash do arquivo cifrado e do mms-type.
const mediaDownloadURLFormat = "https://%s%s&hash=%s&mms-type=%s&__wa-mms="

// Nomes de mms-type e prefixos de caminho usados no upload.
const (
	mmsTypeAudio = "audio"
	// mmsTypePTT substitui mmsTypeAudio em uploads do Messenger, que so'
	// aceitam mensagem de voz e nao arquivo de audio.
	mmsTypePTT = "ptt"

	uploadPrefixDefault    = "mms"
	uploadPrefixMessenger  = "wa-msgr/mms"
	uploadPrefixNewsletter = "newsletter"

	newsletterMMSTypeFormat = "newsletter-%s"
	uploadPathFormat        = "/%s/%s/%s"
	deletePathFormat        = "/mms/%s/%s"
)

// stickerPackMetadataURLFormat e' o endpoint estatico (nao passa pela
// mediaConn) que devolve o JSON de metadados de um pacote de figurinhas.
const stickerPackMetadataURLFormat = "https://static.whatsapp.net/sticker?lottie=1&cat=sticker_pack_data&id=%s&lg=en"
