// Copyright (c) 2024 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
)

// UploadNewsletter sobe o anexo dado aos servidores do WhatsApp sem cifra-lo.
//
// Midia de newsletter funciona quase igual a midia normal, com duas
// diferencas: como nao e' cifrada, nao ha' MediaKey nem FileEncSHA256; e ha' um
// "media handle" que precisa ser passado em SendRequestExtra.
func UploadNewsletter(ctx context.Context, t Transport, data []byte, appInfo Type) (resp UploadResponse, err error) {
	resp.FileLength = uint64(len(data))
	hash := sha256.Sum256(data)
	resp.FileSHA256 = hash[:]
	err = RawUpload(ctx, t, bytes.NewReader(data), resp.FileLength, resp.FileSHA256, appInfo, true, &resp)
	return
}

// UploadNewsletterReader e' identico a [UploadNewsletter], mas le o conteudo de
// um io.ReadSeeker. Diferente de [UploadReader], nao precisa de arquivo
// temporario; o ReadSeeker e' necessario porque os dados sao lidos duas vezes
// (uma para o hash, outra para o envio).
func UploadNewsletterReader(ctx context.Context, t Transport, data io.ReadSeeker, appInfo Type) (resp UploadResponse, err error) {
	hasher := sha256.New()
	var fileLength int64
	fileLength, err = io.Copy(hasher, data)
	resp.FileLength = uint64(fileLength)
	resp.FileSHA256 = hasher.Sum(nil)
	_, err = data.Seek(0, io.SeekStart)
	if err != nil {
		err = fmt.Errorf("failed to seek to start of data: %w", err)
		return
	}
	err = RawUpload(ctx, t, data, resp.FileLength, resp.FileSHA256, appInfo, true, &resp)
	return
}
