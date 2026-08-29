package newsletter

import "errors"

// ErrArgoDecodingBroken e' devolvido enquanto o caminho de decodificacao Argo
// estiver desabilitado no fork. Ver PATCHES.md (Fase E, lote 2).
//
// O pacote raiz referencia ESTE valor (errArgoDecodingBroken = newsletter.
// ErrArgoDecodingBroken), nao uma copia: se virasse um errors.New proprio,
// errors.Is contra o nome da raiz falharia para erros produzidos aqui.
var ErrArgoDecodingBroken = errors.New("argo decoding is currently broken")
