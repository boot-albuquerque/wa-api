package binary

import (
	"fmt"

	"wa-api/internal/noise/protocol/binary/token"
)

// Camada de estrutura da decodificacao: despacho de tag, atributos, listas e
// Node. Tudo aqui e' escrito em cima dos primitivos de decoder.go e nao toca
// no indice do cursor diretamente.

// read despacha um valor pela tag que o precede. asString decide se um bloco
// binario volta como string ou como []byte.
func (r *binaryDecoder) read(string bool) (interface{}, error) {
	tagByte, err := r.readByte()
	if err != nil {
		return nil, err
	}
	tag := int(tagByte)
	switch tag {
	case token.ListEmpty:
		return nil, nil
	case token.List8, token.List16:
		return r.readList(tag)
	case token.Binary8:
		size, err := r.readInt8(false)
		if err != nil {
			return nil, err
		}

		return r.readBytesOrString(size, string)
	case token.Binary20:
		size, err := r.readInt20()
		if err != nil {
			return nil, err
		}

		return r.readBytesOrString(size, string)
	case token.Binary32:
		size, err := r.readInt32(false)
		if err != nil {
			return nil, err
		}

		return r.readBytesOrString(size, string)
	case token.Dictionary0, token.Dictionary1, token.Dictionary2, token.Dictionary3:
		i, err := r.readInt8(false)
		if err != nil {
			return "", err
		}

		return token.GetDoubleToken(tag-token.Dictionary0, i)
	case token.FBJID:
		return r.readFBJID()
	case token.InteropJID:
		return r.readInteropJID()
	case token.JIDPair:
		return r.readJIDPair()
	case token.ADJID:
		return r.readADJID()
	case token.Nibble8, token.Hex8:
		return r.readPacked8(tag)
	default:
		if tag >= 1 && tag < len(token.SingleByteTokens) {
			return token.SingleByteTokens[tag], nil
		}
		return "", fmt.Errorf("%w %d at position %d", ErrInvalidToken, tag, r.index)
	}
}

func (r *binaryDecoder) readAttributes(n int) (Attrs, error) {
	if n == 0 {
		return nil, nil
	}

	ret := make(Attrs)
	for i := 0; i < n; i++ {
		keyIfc, err := r.read(true)
		if err != nil {
			return nil, err
		}

		key, ok := keyIfc.(string)
		if !ok {
			return nil, fmt.Errorf("%[1]w at position %[3]d (%[2]T): %+[2]v", ErrNonStringKey, key, r.index)
		}

		ret[key], err = r.read(true)
		if err != nil {
			return nil, err
		}
	}

	return ret, nil
}

func (r *binaryDecoder) readList(tag int) ([]Node, error) {
	size, err := r.readListSize(tag)
	if err != nil {
		return nil, err
	}

	ret := make([]Node, size)
	for i := 0; i < size; i++ {
		n, err := r.readNode()

		if err != nil {
			return nil, err
		}

		ret[i] = *n
	}

	return ret, nil
}

// readNode le' um elemento. O elemento e' uma lista cuja primeira posicao e' a
// tag, seguida de um par chave/valor por atributo e, se o tamanho da lista for
// par, de mais uma posicao com o conteudo.
func (r *binaryDecoder) readNode() (*Node, error) {
	ret := &Node{}

	size, err := r.readInt8(false)
	if err != nil {
		return nil, err
	}
	listSize, err := r.readListSize(size)
	if err != nil {
		return nil, err
	}

	rawDesc, err := r.read(true)
	if err != nil {
		return nil, err
	}
	// read() devolve interface{} e pode legitimamente trazer nil (ListEmpty),
	// types.JID ou []Node. A assercao sem `ok` virava panic com bytes do
	// socket (F24 em HOUSEKEEP.md).
	tag, ok := rawDesc.(string)
	if !ok {
		return nil, fmt.Errorf("%w: expected string tag, got %T", ErrInvalidNode, rawDesc)
	}
	ret.Tag = tag
	if listSize == 0 || ret.Tag == "" {
		return nil, ErrInvalidNode
	}

	ret.Attrs, err = r.readAttributes((listSize - tagSize) / attrEntrySize)
	if err != nil {
		return nil, err
	}

	if listSize%attrEntrySize == 1 {
		return ret, nil
	}

	ret.Content, err = r.read(false)
	return ret, err
}
