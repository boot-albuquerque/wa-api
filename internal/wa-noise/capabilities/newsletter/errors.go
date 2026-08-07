// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package newsletter

import "errors"

// ErrArgoDecodingBroken e' devolvido enquanto o caminho de decodificacao Argo
// estiver desabilitado no fork. Ver PATCHES.md (Fase E, lote 2).
//
// O pacote raiz referencia ESTE valor (errArgoDecodingBroken = newsletter.
// ErrArgoDecodingBroken), nao uma copia: se virasse um errors.New proprio,
// errors.Is contra o nome da raiz falharia para erros produzidos aqui.
var ErrArgoDecodingBroken = errors.New("argo decoding is currently broken")
