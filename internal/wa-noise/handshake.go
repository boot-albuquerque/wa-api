// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	"wa-api/internal/wa-noise/security/handshake"
	"wa-api/internal/wa-noise/protocol/proto/waWa6"
	"wa-api/internal/wa-noise/socket"
	"wa-api/internal/wa-noise/security/keys"
)

// NoiseHandshakeResponseTimeout e' reexportado de handshake.ResponseTimeout para
// que o nome historico da raiz continue existindo. Sao constantes: nao ha' como
// os dois valores divergirem em tempo de execucao.
const NoiseHandshakeResponseTimeout = handshake.ResponseTimeout

// doHandshake roda o handshake Noise e guarda o socket resultante.
//
// **Este metodo so' pode ser chamado com socketLock ja' segurado em modo
// escrita.** O unico chamador de producao e' unlockedConnect
// (client_connection.go), que roda sob `cli.socketLock.Lock()` tomado por
// ConnectContext ou connect. A atribuicao `cli.socket = ns` abaixo e' a razao —
// e e' exatamente por isso que ela ficou na raiz em vez de ir para o subpacote:
// o pacote handshake nao conhece socketLock e nao deve conhecer.
//
// Ver PATCHES.md, "Fase F/G — lote 10".
func (cli *Client) doHandshake(ctx context.Context, fs *socket.FrameSocket, ephemeralKP keys.KeyPair) error {
	ns, err := handshake.Do(ctx, fs, handshake.Config{
		NoiseKey:          cli.Store.NoiseKey,
		EphemeralKP:       ephemeralKP,
		ClientPayload:     cli.clientPayload,
		FrameHandler:      cli.handleFrame,
		DisconnectHandler: cli.onDisconnect,
	})
	if err != nil {
		return err
	}
	cli.socket = ns
	return nil
}

// clientPayload devolve o payload de login desta conexao: o do consumidor
// quando GetClientPayload esta' preenchido, o do device store caso contrario.
// A escolha fica na raiz porque e' leitura de campo do Client.
//
// E' passada como funcao (nao como valor ja' resolvido) para que handshake.Do a
// chame no mesmo ponto da sequencia em que o codigo original lia o campo — ver
// handshake.Config.ClientPayload.
func (cli *Client) clientPayload() *waWa6.ClientPayload {
	if cli.GetClientPayload != nil {
		return cli.GetClientPayload()
	}
	return cli.Store.GetClientPayload()
}
