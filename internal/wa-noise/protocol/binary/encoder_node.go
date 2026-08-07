package binary

import (
	"fmt"
	"strconv"

	"wa-api/internal/wa-noise/protocol/binary/token"
	"wa-api/internal/wa-noise/protocol/types"
)

// Camada de estrutura da codificacao: Node, atributos e o despacho por tipo
// Go. Escrito em cima dos primitivos de encoder.go.

// writeNode serializa um elemento como uma lista: tag, um par chave/valor por
// atributo e, se houver, o conteudo.
func (w *binaryEncoder) writeNode(n Node) {
	if n.Tag == emptyNodeTag {
		w.pushByte(token.List8)
		w.pushByte(token.ListEmpty)
		return
	}

	hasContent := 0
	if n.Content != nil {
		hasContent = 1
	}

	w.writeListStart(attrEntrySize*w.countAttributes(n.Attrs) + tagSize + hasContent)
	w.writeString(n.Tag)
	w.writeAttributes(n.Attrs)
	if n.Content != nil {
		w.write(n.Content)
	}
}

// write despacha pelo tipo Go do valor. Tudo que nao for JID, bytes ou lista
// de Node vira string — o XML binario so' tem esses tres tipos alem de texto.
func (w *binaryEncoder) write(data interface{}) {
	switch typedData := data.(type) {
	case nil:
		w.pushByte(token.ListEmpty)
	case types.JID:
		w.writeJID(typedData)
	case string:
		w.writeString(typedData)
	case int:
		w.writeString(strconv.Itoa(typedData))
	case int32:
		w.writeString(strconv.FormatInt(int64(typedData), 10))
	case uint:
		w.writeString(strconv.FormatUint(uint64(typedData), 10))
	case uint32:
		w.writeString(strconv.FormatUint(uint64(typedData), 10))
	case int64:
		w.writeString(strconv.FormatInt(typedData, 10))
	case uint64:
		w.writeString(strconv.FormatUint(typedData, 10))
	case bool:
		w.writeString(strconv.FormatBool(typedData))
	case []byte:
		w.writeBytes(typedData)
	case []Node:
		w.writeListStart(len(typedData))
		for _, n := range typedData {
			w.writeNode(n)
		}
	default:
		panic(fmt.Errorf("%w: %T", ErrInvalidType, typedData))
	}
}

// writeString escolhe a representacao mais curta disponivel para a string:
// token de 1 byte, token de 2 bytes, bloco nibble8, bloco hex8 ou, por fim,
// os bytes crus com prefixo de tamanho.
func (w *binaryEncoder) writeString(data string) {
	var dictIndex byte
	if tokenIndex, ok := token.IndexOfSingleToken(data); ok {
		w.pushByte(tokenIndex)
	} else if dictIndex, tokenIndex, ok = token.IndexOfDoubleByteToken(data); ok {
		w.pushByte(token.Dictionary0 + dictIndex)
		w.pushByte(tokenIndex)
	} else if validateNibble(data) {
		w.writePackedBytes(data, token.Nibble8)
	} else if validateHex(data) {
		w.writePackedBytes(data, token.Hex8)
	} else {
		w.writeStringRaw(data)
	}
}

// writeAttributes e countAttributes tem que concordar sobre quais atributos
// entram no frame: countAttributes alimenta o tamanho da lista que
// writeListStart ja' escreveu, e um par a mais ou a menos aqui desalinha todo
// o resto do frame. Por isso o filtro de vazio e' identico nos dois.
func (w *binaryEncoder) writeAttributes(attributes Attrs) {
	for key, val := range attributes {
		if val == "" || val == nil {
			continue
		}

		w.writeString(key)
		w.write(val)
	}
}

func (w *binaryEncoder) countAttributes(attributes Attrs) (count int) {
	for _, val := range attributes {
		if val == "" || val == nil {
			continue
		}
		count += 1
	}
	return
}
