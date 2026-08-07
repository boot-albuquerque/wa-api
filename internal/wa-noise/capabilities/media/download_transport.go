package media

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"go.mau.fi/util/retryafter"

	"wa-api/internal/wa-noise/protocol/socket"
	cbcutil "wa-api/internal/wa-noise/security/cbc"
	hkdfutil "wa-api/internal/wa-noise/security/hkdf"
)

// DownloadAndDecrypt baixa a URL dada, valida o HMAC e devolve o plaintext.
func DownloadAndDecrypt(
	ctx context.Context,
	t HTTPTransport,
	url string,
	mediaKey []byte,
	appInfo Type,
	fileLength int,
	fileEncSHA256,
	fileSHA256 []byte,
) (data []byte, err error) {
	iv, cipherKey, macKey, _ := GetKeys(mediaKey, appInfo)
	var ciphertext, mac []byte
	if ciphertext, mac, err = DownloadPossiblyEncryptedWithRetries(ctx, t, url, fileEncSHA256); err != nil {

	} else if mediaKey == nil && fileEncSHA256 == nil && mac == nil {
		// Unencrypted media, just return the downloaded data
		data = ciphertext
	} else if err = ValidateMedia(iv, ciphertext, macKey, mac); err != nil {

	} else if data, err = cbcutil.Decrypt(cipherKey, iv, ciphertext); err != nil {
		err = fmt.Errorf("failed to decrypt file: %w", err)
	} else if t.ReturnDownloadWarnings() {
		if fileLength >= 0 && len(data) != fileLength {
			err = fmt.Errorf("%w: expected %d, got %d", ErrFileLengthMismatch, fileLength, len(data))
		} else if len(fileSHA256) == sha256HashLength && sha256.Sum256(data) != *(*[sha256HashLength]byte)(fileSHA256) {
			err = ErrInvalidMediaSHA256
		}
	}
	return
}

// GetKeys expande a mediaKey via HKDF-SHA256 nas quatro fatias do protocolo.
func GetKeys(mediaKey []byte, appInfo Type) (iv, cipherKey, macKey, refKey []byte) {
	mediaKeyExpanded := hkdfutil.SHA256(mediaKey, nil, []byte(appInfo), mediaKeyExpandedLength)
	return mediaKeyExpanded[:mediaIVEnd],
		mediaKeyExpanded[mediaIVEnd:mediaCipherKeyEnd],
		mediaKeyExpanded[mediaCipherKeyEnd:mediaMACKeyEnd],
		mediaKeyExpanded[mediaMACKeyEnd:]
}

// ShouldRetryDownload diz se vale a pena repetir o download apos o erro dado.
func ShouldRetryDownload(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var netErr net.Error
	var httpErr DownloadHTTPError
	return errors.As(err, &netErr) ||
		strings.HasPrefix(err.Error(), "stream error:") || // hacky check for http2 errors
		(errors.As(err, &httpErr) && retryafter.Should(httpErr.StatusCode, true))
}

// retryDelay devolve quanto esperar antes da tentativa retryNum+1, honrando o
// header Retry-After quando o erro for um DownloadHTTPError.
func retryDelay(err error, retryNum int) time.Duration {
	d := time.Duration(retryNum+1) * mediaDownloadRetryStep
	var httpErr DownloadHTTPError
	if errors.As(err, &httpErr) {
		d = retryafter.Parse(httpErr.Response.Header.Get("Retry-After"), d)
	}
	return d
}

// DownloadPossiblyEncryptedWithRetries baixa a URL, repetindo em erros de rede
// e status retentaveis ate' mediaDownloadMaxRetries tentativas.
func DownloadPossiblyEncryptedWithRetries(
	ctx context.Context, t HTTPTransport, url string, checksum []byte,
) (file, mac []byte, err error) {
	for retryNum := 0; retryNum < mediaDownloadMaxRetries; retryNum++ {
		if checksum == nil {
			file, err = DownloadRaw(ctx, t, url)
		} else {
			file, mac, err = DownloadEncrypted(ctx, t, url, checksum)
		}
		if err == nil || !ShouldRetryDownload(err) {
			return
		}
		retryDuration := retryDelay(err, retryNum)
		t.Log().Warnf("Failed to download media due to network error: %v, retrying in %s...", err, retryDuration)
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(retryDuration):
		}
	}
	return
}

// DoDownloadRequest faz o GET de midia com os cabecalhos do fork e devolve a
// resposta ainda aberta; status != 200 vira DownloadHTTPError.
func DoDownloadRequest(ctx context.Context, t HTTPTransport, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare request: %w", err)
	}
	req.Header.Set("Origin", socket.Origin)
	req.Header.Set("Referer", socket.Origin+"/")
	if t.IsMessenger() {
		req.Header.Set("User-Agent", t.MessengerUserAgent())
	}
	// TODO user agent for whatsapp downloads?
	resp, err := t.HTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, DownloadHTTPError{Response: resp}
	}
	return resp, nil
}

// DownloadRaw baixa a URL inteira para memoria, sem tratar cifragem.
func DownloadRaw(ctx context.Context, t HTTPTransport, url string) ([]byte, error) {
	resp, err := DoDownloadRequest(ctx, t, url)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return data, err
}

// DownloadEncrypted baixa a URL e separa o MAC truncado do fim do ciphertext,
// conferindo o hash do ciphertext contra checksum.
func DownloadEncrypted(ctx context.Context, t HTTPTransport, url string, checksum []byte) (file, mac []byte, err error) {
	data, err := DownloadRaw(ctx, t, url)
	if err != nil {
		return
	} else if len(data) <= mediaHMACLength {
		err = ErrTooShortFile
		return
	}
	file, mac = data[:len(data)-mediaHMACLength], data[len(data)-mediaHMACLength:]
	if len(checksum) == sha256HashLength && sha256.Sum256(data) != *(*[sha256HashLength]byte)(checksum) {
		err = ErrInvalidMediaEncSHA256
	}
	return
}

// ValidateMedia confere o HMAC truncado sobre iv||file.
func ValidateMedia(iv, file, macKey, mac []byte) error {
	h := hmac.New(sha256.New, macKey)
	h.Write(iv)
	h.Write(file)
	if !hmac.Equal(h.Sum(nil)[:mediaHMACLength], mac) {
		return ErrInvalidMediaHMAC
	}
	return nil
}
