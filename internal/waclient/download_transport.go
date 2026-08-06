// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

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

	"wa-api/internal/waclient/socket"
	"wa-api/internal/waclient/util/cbcutil"
	"wa-api/internal/waclient/util/hkdfutil"
)

func (cli *Client) downloadAndDecrypt(
	ctx context.Context,
	url string,
	mediaKey []byte,
	appInfo MediaType,
	fileLength int,
	fileEncSHA256,
	fileSHA256 []byte,
) (data []byte, err error) {
	iv, cipherKey, macKey, _ := getMediaKeys(mediaKey, appInfo)
	var ciphertext, mac []byte
	if ciphertext, mac, err = cli.downloadPossiblyEncryptedMediaWithRetries(ctx, url, fileEncSHA256); err != nil {

	} else if mediaKey == nil && fileEncSHA256 == nil && mac == nil {
		// Unencrypted media, just return the downloaded data
		data = ciphertext
	} else if err = validateMedia(iv, ciphertext, macKey, mac); err != nil {

	} else if data, err = cbcutil.Decrypt(cipherKey, iv, ciphertext); err != nil {
		err = fmt.Errorf("failed to decrypt file: %w", err)
	} else if ReturnDownloadWarnings {
		if fileLength >= 0 && len(data) != fileLength {
			err = fmt.Errorf("%w: expected %d, got %d", ErrFileLengthMismatch, fileLength, len(data))
		} else if len(fileSHA256) == 32 && sha256.Sum256(data) != *(*[32]byte)(fileSHA256) {
			err = ErrInvalidMediaSHA256
		}
	}
	return
}

func getMediaKeys(mediaKey []byte, appInfo MediaType) (iv, cipherKey, macKey, refKey []byte) {
	mediaKeyExpanded := hkdfutil.SHA256(mediaKey, nil, []byte(appInfo), 112)
	return mediaKeyExpanded[:16], mediaKeyExpanded[16:48], mediaKeyExpanded[48:80], mediaKeyExpanded[80:]
}

func shouldRetryMediaDownload(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var netErr net.Error
	var httpErr DownloadHTTPError
	return errors.As(err, &netErr) ||
		strings.HasPrefix(err.Error(), "stream error:") || // hacky check for http2 errors
		(errors.As(err, &httpErr) && retryafter.Should(httpErr.StatusCode, true))
}

func (cli *Client) downloadPossiblyEncryptedMediaWithRetries(ctx context.Context, url string, checksum []byte) (file, mac []byte, err error) {
	for retryNum := 0; retryNum < 5; retryNum++ {
		if checksum == nil {
			file, err = cli.downloadMedia(ctx, url)
		} else {
			file, mac, err = cli.downloadEncryptedMedia(ctx, url, checksum)
		}
		if err == nil || !shouldRetryMediaDownload(err) {
			return
		}
		retryDuration := time.Duration(retryNum+1) * time.Second
		var httpErr DownloadHTTPError
		if errors.As(err, &httpErr) {
			retryDuration = retryafter.Parse(httpErr.Response.Header.Get("Retry-After"), retryDuration)
		}
		cli.Log.Warnf("Failed to download media due to network error: %v, retrying in %s...", err, retryDuration)
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(retryDuration):
		}
	}
	return
}

func (cli *Client) doMediaDownloadRequest(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare request: %w", err)
	}
	req.Header.Set("Origin", socket.Origin)
	req.Header.Set("Referer", socket.Origin+"/")
	if cli.MessengerConfig != nil {
		req.Header.Set("User-Agent", cli.MessengerConfig.UserAgent)
	}
	// TODO user agent for whatsapp downloads?
	resp, err := cli.mediaHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, DownloadHTTPError{Response: resp}
	}
	return resp, nil
}

func (cli *Client) downloadMedia(ctx context.Context, url string) ([]byte, error) {
	resp, err := cli.doMediaDownloadRequest(ctx, url)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return data, err
}

const mediaHMACLength = 10

func (cli *Client) downloadEncryptedMedia(ctx context.Context, url string, checksum []byte) (file, mac []byte, err error) {
	data, err := cli.downloadMedia(ctx, url)
	if err != nil {
		return
	} else if len(data) <= mediaHMACLength {
		err = ErrTooShortFile
		return
	}
	file, mac = data[:len(data)-mediaHMACLength], data[len(data)-mediaHMACLength:]
	if len(checksum) == 32 && sha256.Sum256(data) != *(*[32]byte)(checksum) {
		err = ErrInvalidMediaEncSHA256
	}
	return
}

func validateMedia(iv, file, macKey, mac []byte) error {
	h := hmac.New(sha256.New, macKey)
	h.Write(iv)
	h.Write(file)
	if !hmac.Equal(h.Sum(nil)[:mediaHMACLength], mac) {
		return ErrInvalidMediaHMAC
	}
	return nil
}
