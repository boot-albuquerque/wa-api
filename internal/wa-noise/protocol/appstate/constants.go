// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package appstate

// Tamanhos fixos do protocolo de app state sync.
//
// Os valores sao ditados pelo protocolo do WhatsApp e pelos algoritmos usados
// (HMAC-SHA256 truncado, AES-CBC, LTHash); nao sao parametros ajustaveis.
const (
	// macLength e' o tamanho em bytes de todo MAC de app state (index MAC,
	// value MAC, snapshot MAC, patch MAC). Os MACs derivados de SHA-512 sao
	// truncados a este tamanho.
	macLength = 32
	// cbcIVLength e' o tamanho do IV do AES-CBC que prefixa o conteudo
	// cifrado de cada mutacao.
	cbcIVLength = 16
	// lthashLength e' o tamanho em bytes do estado do LTHash de integridade
	// de patches (`lthash.WAPatchIntegrity`, HKDFSize 128).
	lthashLength = 128
	// versionByteLength e' o tamanho do inteiro big-endian de 64 bits usado
	// nos MACs para carregar a versao do estado.
	versionByteLength = 8
	// contentMACOperationOffset e' somado ao valor numerico da operacao
	// (SET/REMOVE) antes de entrar no MAC de conteudo — o protocolo indexa as
	// operacoes a partir de 1, o enum protobuf a partir de 0.
	contentMACOperationOffset = 1
	// contentMACKeyIDLengthOffset e' somado ao tamanho do key ID no sufixo de
	// comprimento do MAC de conteudo, pelo mesmo motivo do offset acima (o
	// byte da operacao conta junto com o key ID).
	contentMACKeyIDLengthOffset = 1
)

// Tags e atributos dos nos XML que carregam patches de app state.
const (
	snapshotNodeTag      = "snapshot"
	patchesNodeTag       = "patches"
	patchNodeTag         = "patch"
	patchListAttrName    = "name"
	patchListAttrHasMore = "has_more_patches"
)

// Valores booleanos serializados como string dentro dos indices de mutacao.
// O protocolo de app state representa flags como "0"/"1" dentro do array JSON
// do indice, nao como booleano.
const (
	indexBoolFalse = "0"
	indexBoolTrue  = "1"
)

// selfSenderIndexValue e' o sender JID usado no indice de `star` quando o
// remetente e' o proprio dono do chat — o protocolo usa "0" como sentinela em
// vez de repetir o JID.
const selfSenderIndexValue = indexBoolFalse

// Versoes estaticas de cada tipo de mutacao. O campo `Version` de
// `MutationInfo` nao e' a versao do estado: e' um numero fixo por tipo de
// coisa mutada, definido pelo protocolo.
const (
	mutationVersionMute            int32 = 2
	mutationVersionPin             int32 = 5
	mutationVersionArchive         int32 = 3
	mutationVersionMarkChatAsRead  int32 = 3
	mutationVersionLabelAssocChat  int32 = 3
	mutationVersionLabelAssocMsg   int32 = 3
	mutationVersionLabelEdit       int32 = 3
	mutationVersionSettingPushName int32 = 1
	mutationVersionStar            int32 = 2
	mutationVersionDeleteChat      int32 = 6
)

// muteForeverEndTimestamp e' o valor de `MuteEndTimestamp` que o protocolo
// interpreta como "mutado para sempre" (nenhum fim).
const muteForeverEndTimestamp int64 = -1
