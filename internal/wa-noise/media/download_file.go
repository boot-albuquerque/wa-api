// Copyright (c) 2024 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package media

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"time"

	"go.mau.fi/util/fallocate"

	"wa-api/internal/wa-noise/proto/waMediaTransport"
	"wa-api/internal/wa-noise/util/cbcutil"
)

// File e' o destino de um download para arquivo. *os.File o satisfaz.
type File interface {
	io.Reader
	io.Writer
	io.Seeker
	io.ReaderAt
	io.WriterAt
	Truncate(size int64) error
	Stat() (os.FileInfo, error)
}

// DownloadMessageToFile e' identico a [DownloadMessage], mas escreve o anexo
// no arquivo dado em vez de devolve-lo como slice.
func DownloadMessageToFile(ctx context.Context, t Transport, msg Downloadable, file File) error {
	mediaType := GetType(msg)
	if mediaType == "" {
		return fmt.Errorf("%w %T", ErrUnknownMediaType, msg)
	}
	url, isWebWhatsappNetURL := directURL(msg)
	if len(url) > 0 && !isWebWhatsappNetURL {
		return DownloadAndDecryptToFile(ctx, t, url, msg.GetMediaKey(), mediaType, getSize(msg), msg.GetFileEncSHA256(), msg.GetFileSHA256(), file)
	} else if len(msg.GetDirectPath()) > 0 {
		return DownloadWithPathToFile(
			ctx, t, msg.GetDirectPath(), msg.GetFileEncSHA256(), msg.GetFileSHA256(),
			msg.GetMediaKey(), getSize(msg), mediaType, mediaTypeToMMSType[mediaType], file,
		)
	} else {
		if isWebWhatsappNetURL {
			t.Log().Warnf("Got a media message with a web.whatsapp.net URL (%s) and no direct path", url)
		}
		return ErrNoURLPresent
	}
}

// DownloadFBToFile e' o equivalente de [DownloadFB] escrevendo em arquivo.
func DownloadFBToFile(
	ctx context.Context,
	t Transport,
	transport *waMediaTransport.WAMediaTransport_Integral,
	mediaType Type,
	file File,
) error {
	return DownloadWithPathToFile(
		ctx, t, transport.GetDirectPath(), transport.GetFileEncSHA256(), transport.GetFileSHA256(),
		transport.GetMediaKey(), UnknownFileLength, mediaType, mediaTypeToMMSType[mediaType], file,
	)
}

// DownloadWithPathToFile e' o equivalente de [DownloadWithPath] escrevendo em
// arquivo.
//
// Diferente de [DownloadWithPath], nao valida que directPath comeca com barra:
// esse assimetria vem do codigo de origem e foi preservada.
func DownloadWithPathToFile(
	ctx context.Context,
	t Transport,
	directPath string,
	encFileHash, fileHash, mediaKey []byte,
	fileLength int,
	mediaType Type,
	mmsType string,
	file File,
) error {
	mediaConn, err := RefreshConn(ctx, t, false)
	if err != nil {
		return fmt.Errorf("failed to refresh media connections: %w", err)
	}
	if len(mmsType) == 0 {
		mmsType = mediaTypeToMMSType[mediaType]
	}
	for i, host := range mediaConn.Hosts {
		// TODO omit hash for unencrypted media?
		mediaURL := buildDownloadURL(host.Hostname, directPath, encFileHash, mmsType)
		err = DownloadAndDecryptToFile(ctx, t, mediaURL, mediaKey, mediaType, fileLength, encFileHash, fileHash, file)
		if isTerminalDownloadResult(err) {
			return err
		} else if i >= len(mediaConn.Hosts)-1 {
			return fmt.Errorf("failed to download media from last host: %w", err)
		}
		t.Log().Warnf("Failed to download media: %s, trying with next host...", err)
	}
	return err
}

// DownloadAndDecryptToFile baixa a URL para o arquivo, valida o HMAC e decifra
// o conteudo no proprio arquivo.
func DownloadAndDecryptToFile(
	ctx context.Context,
	t HTTPTransport,
	url string,
	mediaKey []byte,
	appInfo Type,
	fileLength int,
	fileEncSHA256, fileSHA256 []byte,
	file File,
) error {
	iv, cipherKey, macKey, _ := GetKeys(mediaKey, appInfo)
	hasher := sha256.New()
	if mac, err := DownloadPossiblyEncryptedWithRetriesToFile(ctx, t, url, fileEncSHA256, file); err != nil {
		return err
	} else if mediaKey == nil && fileEncSHA256 == nil && mac == nil {
		// Unencrypted media, just return the downloaded data
		return nil
	} else if err = ValidateMediaFile(file, iv, macKey, mac); err != nil {
		return err
	} else if _, err = file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to start of file after validating mac: %w", err)
	} else if err = cbcutil.DecryptFile(cipherKey, iv, file); err != nil {
		return fmt.Errorf("failed to decrypt file: %w", err)
	} else if t.ReturnDownloadWarnings() {
		if info, err := file.Stat(); err != nil {
			return fmt.Errorf("failed to stat file: %w", err)
		} else if fileLength >= 0 && info.Size() != int64(fileLength) {
			return fmt.Errorf("%w: expected %d, got %d", ErrFileLengthMismatch, fileLength, info.Size())
		} else if _, err = file.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek to start of file after decrypting: %w", err)
		} else if _, err = io.Copy(hasher, file); err != nil {
			return fmt.Errorf("failed to hash file: %w", err)
		} else if !hmac.Equal(fileSHA256, hasher.Sum(nil)) {
			return ErrInvalidMediaSHA256
		}
	}
	return nil
}

// DownloadPossiblyEncryptedWithRetriesToFile e' o equivalente para arquivo de
// [DownloadPossiblyEncryptedWithRetries]; entre tentativas rebobina o arquivo.
func DownloadPossiblyEncryptedWithRetriesToFile(
	ctx context.Context, t HTTPTransport, url string, checksum []byte, file File,
) (mac []byte, err error) {
	for retryNum := 0; retryNum < mediaDownloadMaxRetries; retryNum++ {
		if checksum == nil {
			_, _, err = DownloadRawToFile(ctx, t, url, file)
		} else {
			mac, err = DownloadEncryptedToFile(ctx, t, url, checksum, file)
		}
		if err == nil || !ShouldRetryDownload(err) {
			return
		}
		retryDuration := retryDelay(err, retryNum)
		t.Log().Warnf("Failed to download media due to network error: %v, retrying in %s...", err, retryDuration)
		_, err = file.Seek(0, io.SeekStart)
		if err != nil {
			return nil, fmt.Errorf("failed to seek to start of file to retry download: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryDuration):
		}
	}
	return
}

// DownloadRawToFile baixa a URL para o writer dado, devolvendo quantos bytes
// foram escritos e o SHA-256 do que passou.
func DownloadRawToFile(ctx context.Context, t HTTPTransport, url string, file io.Writer) (int64, []byte, error) {
	resp, err := DoDownloadRequest(ctx, t, url)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	osFile, ok := file.(*os.File)
	if ok && resp.ContentLength > 0 {
		err = fallocate.Fallocate(osFile, int(resp.ContentLength))
		if err != nil {
			return 0, nil, fmt.Errorf("failed to preallocate file: %w", err)
		}
	}
	hasher := sha256.New()
	n, err := io.Copy(file, io.TeeReader(resp.Body, hasher))
	return n, hasher.Sum(nil), err
}

// DownloadEncryptedToFile baixa a URL para o arquivo, confere o hash do
// ciphertext e corta o MAC truncado do fim do arquivo, devolvendo-o.
func DownloadEncryptedToFile(ctx context.Context, t HTTPTransport, url string, checksum []byte, file File) ([]byte, error) {
	size, hash, err := DownloadRawToFile(ctx, t, url, file)
	if err != nil {
		return nil, err
	} else if size <= mediaHMACLength {
		return nil, ErrTooShortFile
	} else if len(checksum) == sha256HashLength && !hmac.Equal(checksum, hash) {
		return nil, ErrInvalidMediaEncSHA256
	}
	mac := make([]byte, mediaHMACLength)
	_, err = file.ReadAt(mac, size-mediaHMACLength)
	if err != nil {
		return nil, fmt.Errorf("failed to read MAC from file: %w", err)
	}
	err = file.Truncate(size - mediaHMACLength)
	if err != nil {
		return nil, fmt.Errorf("failed to truncate file to remove MAC: %w", err)
	}
	return mac, nil
}

// ValidateMediaFile confere o HMAC truncado sobre iv||conteudo do arquivo.
func ValidateMediaFile(file io.ReadSeeker, iv, macKey, mac []byte) error {
	h := hmac.New(sha256.New, macKey)
	h.Write(iv)
	_, err := file.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek to start of file: %w", err)
	}
	_, err = io.Copy(h, file)
	if err != nil {
		return fmt.Errorf("failed to hash file: %w", err)
	}
	if !hmac.Equal(h.Sum(nil)[:mediaHMACLength], mac) {
		return ErrInvalidMediaHMAC
	}
	return nil
}
