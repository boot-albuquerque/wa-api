package binary

import (
	"fmt"

	"wa-api/internal/wa-noise/binary/token"
)

// Este arquivo concentra o codec nibble8/hex8 nas DUAS direcoes. Encoder e
// decoder implementam metades opostas do mesmo alfabeto posicional, e manter
// packNibble ao lado de unpackNibble e' o que deixa a inversa verificavel de
// relance: se um lado ganhar um caractere novo e o outro nao, a assimetria
// fica na mesma tela.

// validateNibble diz se a string inteira cabe no alfabeto nibble8
// (digitos decimais mais '-' e '.').
func validateNibble(value string) bool {
	if len(value) > token.PackedMax {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9') && char != '-' && char != '.' {
			return false
		}
	}
	return true
}

// validateHex diz se a string inteira cabe no alfabeto hex8
// (digitos decimais mais 'A'-'F', maiusculo).
func validateHex(value string) bool {
	if len(value) > token.PackedMax {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9') && !(char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}

func packNibble(value byte) byte {
	switch value {
	case '-':
		return nibbleDash
	case '.':
		return nibbleDot
	case 0:
		return nibblePadding
	default:
		if value >= '0' && value <= '9' {
			return value - '0'
		}
		// This should be validated beforehand
		panic(fmt.Errorf("invalid string to pack as nibble: %d / '%s'", value, string(value)))
	}
}

func packHex(value byte) byte {
	switch {
	case value >= '0' && value <= '9':
		return value - '0'
	case value >= 'A' && value <= 'F':
		return decimalDigitCount + value - 'A'
	case value == 0:
		return nibblePadding
	default:
		// This should be validated beforehand
		panic(fmt.Errorf("invalid string to pack as hex: %d / '%s'", value, string(value)))
	}
}

func unpackByte(tag int, value byte) (byte, error) {
	switch tag {
	case token.Nibble8:
		return unpackNibble(value)
	case token.Hex8:
		return unpackHex(value)
	default:
		return 0, fmt.Errorf("unpackByte with unknown tag %d", tag)
	}
}

func unpackNibble(value byte) (byte, error) {
	switch {
	case value < decimalDigitCount:
		return '0' + value, nil
	case value == nibbleDash:
		return '-', nil
	case value == nibbleDot:
		return '.', nil
	case value == nibblePadding:
		return 0, nil
	default:
		return 0, fmt.Errorf("unpackNibble with value %d", value)
	}
}

func unpackHex(value byte) (byte, error) {
	switch {
	case value < decimalDigitCount:
		return '0' + value, nil
	case value < hexDigitCount:
		return 'A' + value - decimalDigitCount, nil
	default:
		return 0, fmt.Errorf("unpackHex with value %d", value)
	}
}
