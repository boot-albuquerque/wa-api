package binary

import (
	"fmt"
	"io"
	"strings"

	"wa-api/internal/noise/protocol/binary/token"
)

// binaryDecoder e' um cursor sobre o frame recebido. Este arquivo tem so' a
// camada de bytes: avancar o indice, ler inteiros de largura fixa, ler blocos
// crus e desempacotar blocos nibble8/hex8. A leitura da estrutura (tokens,
// atributos, listas, Node) esta' em decoder_node.go.
type binaryDecoder struct {
	data  []byte
	index int
}

func newDecoder(data []byte) *binaryDecoder {
	return &binaryDecoder{data, 0}
}

func (r *binaryDecoder) checkEOS(length int) error {
	if r.index+length > len(r.data) {
		return io.EOF
	}

	return nil
}

func (r *binaryDecoder) readByte() (byte, error) {
	if err := r.checkEOS(int8Size); err != nil {
		return 0, err
	}

	b := r.data[r.index]
	r.index++

	return b, nil
}

func (r *binaryDecoder) readIntN(n int, littleEndian bool) (int, error) {
	if err := r.checkEOS(n); err != nil {
		return 0, err
	}

	var ret int

	for i := 0; i < n; i++ {
		var curShift int
		if littleEndian {
			curShift = i
		} else {
			curShift = n - i - 1
		}
		ret |= int(r.data[r.index+i]) << uint(curShift*bitsPerByte)
	}

	r.index += n
	return ret, nil
}

func (r *binaryDecoder) readInt8(littleEndian bool) (int, error) {
	return r.readIntN(int8Size, littleEndian)
}

func (r *binaryDecoder) readInt16(littleEndian bool) (int, error) {
	return r.readIntN(int16Size, littleEndian)
}

func (r *binaryDecoder) readInt20() (int, error) {
	if err := r.checkEOS(int20Size); err != nil {
		return 0, err
	}

	ret := ((int(r.data[r.index]) & int20HighNibbleMask) << int20HighShift) +
		(int(r.data[r.index+1]) << int20MiddleShift) +
		int(r.data[r.index+2])
	r.index += int20Size
	return ret, nil
}

func (r *binaryDecoder) readInt32(littleEndian bool) (int, error) {
	return r.readIntN(int32Size, littleEndian)
}

// readPacked8 le' um bloco nibble8/hex8: um byte de tamanho (cujo bit alto
// marca que o ultimo caractere e' padding) seguido dos bytes com dois
// caracteres cada.
func (r *binaryDecoder) readPacked8(tag int) (string, error) {
	startByte, err := r.readByte()
	if err != nil {
		return "", err
	}

	var build strings.Builder

	for i := 0; i < int(startByte&packedLengthMask); i++ {
		currByte, err := r.readByte()
		if err != nil {
			return "", err
		}

		lower, err := unpackByte(tag, currByte&nibbleHighMask>>nibbleShift)
		if err != nil {
			return "", err
		}

		upper, err := unpackByte(tag, currByte&nibbleLowMask)
		if err != nil {
			return "", err
		}

		build.WriteByte(lower)
		build.WriteByte(upper)
	}

	ret := build.String()
	if startByte&packedOddLengthFlag != 0 {
		ret = ret[:len(ret)-1]
	}
	return ret, nil
}

func (r *binaryDecoder) readListSize(tag int) (int, error) {
	switch tag {
	case token.ListEmpty:
		return 0, nil
	case token.List8:
		return r.readInt8(false)
	case token.List16:
		return r.readInt16(false)
	default:
		return 0, fmt.Errorf("readListSize with unknown tag %d at position %d", tag, r.index)
	}
}

func (r *binaryDecoder) readBytesOrString(length int, asString bool) (interface{}, error) {
	data, err := r.readRaw(length)
	if err != nil {
		return nil, err
	}
	if asString {
		return string(data), nil
	}
	return data, nil
}

func (r *binaryDecoder) readRaw(length int) ([]byte, error) {
	if err := r.checkEOS(length); err != nil {
		return nil, err
	}

	ret := r.data[r.index : r.index+length]
	r.index += length

	return ret, nil
}
