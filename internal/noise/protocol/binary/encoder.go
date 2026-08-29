package binary

import (
	"fmt"
	"math"

	"wa-api/internal/noise/protocol/binary/token"
)

// binaryEncoder acumula o frame em construcao. Este arquivo tem so' a camada
// de bytes: empilhar bytes/inteiros, escrever prefixos de tamanho e empacotar
// blocos nibble8/hex8. A escrita da estrutura (Node, atributos, despacho por
// tipo) esta' em encoder_node.go.
//
// O primeiro byte e' sempre 0: e' o flag de compressao que Unpack le' na
// ponta oposta, e Marshal nunca comprime.
type binaryEncoder struct {
	data []byte
}

func newEncoder() *binaryEncoder {
	return &binaryEncoder{[]byte{0}}
}

func (w *binaryEncoder) getData() []byte {
	return w.data
}

func (w *binaryEncoder) pushByte(b byte) {
	w.data = append(w.data, b)
}

func (w *binaryEncoder) pushBytes(bytes []byte) {
	w.data = append(w.data, bytes...)
}

func (w *binaryEncoder) pushIntN(value, n int, littleEndian bool) {
	for i := 0; i < n; i++ {
		var curShift int
		if littleEndian {
			curShift = i
		} else {
			curShift = n - i - 1
		}
		w.pushByte(byte((value >> uint(curShift*bitsPerByte)) & byteMask))
	}
}

func (w *binaryEncoder) pushInt20(value int) {
	w.pushBytes([]byte{
		byte((value >> int20HighShift) & int20HighNibbleMask),
		byte((value >> int20MiddleShift) & byteMask),
		byte(value & byteMask),
	})
}

func (w *binaryEncoder) pushInt8(value int) {
	w.pushIntN(value, int8Size, false)
}

func (w *binaryEncoder) pushInt16(value int) {
	w.pushIntN(value, int16Size, false)
}

func (w *binaryEncoder) pushInt32(value int) {
	w.pushIntN(value, int32Size, false)
}

func (w *binaryEncoder) pushString(value string) {
	w.pushBytes([]byte(value))
}

// writeByteLength escolhe a menor das tres tags de bloco binario que comporta
// o tamanho dado.
func (w *binaryEncoder) writeByteLength(length int) {
	if length < token.SingleByteMax {
		w.pushByte(token.Binary8)
		w.pushInt8(length)
	} else if length < int20Max {
		w.pushByte(token.Binary20)
		w.pushInt20(length)
	} else if length < math.MaxInt32 {
		w.pushByte(token.Binary32)
		w.pushInt32(length)
	} else {
		panic(fmt.Errorf("length is too large: %d", length))
	}
}

// writeListStart escolhe entre lista vazia, lista de tamanho em 1 byte e
// lista de tamanho em 2 bytes.
func (w *binaryEncoder) writeListStart(listSize int) {
	if listSize == 0 {
		w.pushByte(byte(token.ListEmpty))
	} else if listSize < token.SingleByteMax {
		w.pushByte(byte(token.List8))
		w.pushInt8(listSize)
	} else {
		w.pushByte(byte(token.List16))
		w.pushInt16(listSize)
	}
}

func (w *binaryEncoder) writeBytes(value []byte) {
	w.writeByteLength(len(value))
	w.pushBytes(value)
}

func (w *binaryEncoder) writeStringRaw(value string) {
	w.writeByteLength(len(value))
	w.pushString(value)
}

// writePackedBytes escreve a string com dois caracteres por byte. Quando o
// numero de caracteres e' impar, o tamanho vai com packedOddLengthFlag ligado
// e o ultimo nibble leva padding — e' isso que readPacked8 desfaz.
func (w *binaryEncoder) writePackedBytes(value string, dataType int) {
	if len(value) > token.PackedMax {
		panic(fmt.Errorf("too many bytes to pack: %d", len(value)))
	}

	w.pushByte(byte(dataType))

	roundedLength := byte(math.Ceil(float64(len(value)) / float64(charsPerPackedByte)))
	if len(value)%charsPerPackedByte != 0 {
		roundedLength |= packedOddLengthFlag
	}
	w.pushByte(roundedLength)
	var packer func(byte) byte
	if dataType == token.Nibble8 {
		packer = packNibble
	} else if dataType == token.Hex8 {
		packer = packHex
	} else {
		// This should only be called with the correct values
		panic(fmt.Errorf("invalid packed byte data type %v", dataType))
	}
	for i, l := 0, len(value)/charsPerPackedByte; i < l; i++ {
		w.pushByte(w.packBytePair(packer, value[charsPerPackedByte*i], value[charsPerPackedByte*i+1]))
	}
	if len(value)%charsPerPackedByte != 0 {
		w.pushByte(w.packBytePair(packer, value[len(value)-1], '\x00'))
	}
}

func (w *binaryEncoder) packBytePair(packer func(byte) byte, part1, part2 byte) byte {
	return (packer(part1) << nibbleShift) | packer(part2)
}
