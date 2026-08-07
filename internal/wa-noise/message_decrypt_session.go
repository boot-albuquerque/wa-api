// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"time"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/message"
	"wa-api/internal/wa-noise/types"
)

// Fachadas do nucleo de decifragem Signal, que vive em
// internal/wa-noise/message/ desde a Fase F/G lote 9. Existem porque
// internals.go (gerado) cita os tres nomes minusculos em
// DangerousInternalClient.

func (cli *Client) bufferedDecrypt(
	ctx context.Context,
	ciphertext []byte,
	serverTimestamp time.Time,
	decrypt func(context.Context) ([]byte, error),
	extraHashData ...string,
) (plaintext []byte, ciphertextHash [32]byte, err error) {
	return message.BufferedDecrypt(ctx, cli.msgT(), ciphertext, serverTimestamp, decrypt, extraHashData...)
}

func (cli *Client) decryptDM(ctx context.Context, child *waBinary.Node, from types.JID, isPreKey bool, serverTS time.Time) ([]byte, *[32]byte, error) {
	return message.DecryptDM(ctx, cli.msgT(), child, from, isPreKey, serverTS)
}

func (cli *Client) decryptGroupMsg(ctx context.Context, child *waBinary.Node, from types.JID, chat types.JID, serverTS time.Time) ([]byte, *[32]byte, error) {
	return message.DecryptGroupMsg(ctx, cli.msgT(), child, from, chat, serverTS)
}
