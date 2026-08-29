package core

import (
	"crypto/hmac"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"sort"
	"sync"

	"go.mau.fi/util/exerrors"
	"go.mau.fi/util/exstrings"

	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types"
)

//go:embed reportingfields.json
var reportingFieldsJSON string
var getReportingFields = sync.OnceValue(func() (output []reportingField) {
	exerrors.PanicIfNotNil(json.Unmarshal(exstrings.UnsafeBytes(reportingFieldsJSON), &output))
	return
})

type reportingField struct {
	FieldNumber int              `json:"f"`
	IsMessage   bool             `json:"m,omitempty"`
	Subfields   []reportingField `json:"s,omitempty"`
}

func (cli *Client) shouldIncludeReportingToken(message *waE2E.Message) bool {
	if !cli.SendReportingTokens {
		return false
	}
	return message.ReactionMessage == nil &&
		message.EncReactionMessage == nil &&
		message.EncEventResponseMessage == nil &&
		message.PollUpdateMessage == nil
}

func (cli *Client) getMessageReportingToken(
	msgProtobuf []byte,
	msg *waE2E.Message,
	senderJID, remoteJID types.JID,
	messageID types.MessageID,
) waBinary.Node {
	reportingSecret, _ := generateMsgSecretKey(
		EncSecretReportToken, remoteJID, messageID, senderJID,
		msg.GetMessageContextInfo().GetMessageSecret(),
	)
	hasher := hmac.New(sha256.New, reportingSecret)
	hasher.Write(getReportingToken(msgProtobuf))
	return waBinary.Node{
		Tag: reportingTokenNodeTag,
		Content: []waBinary.Node{{
			Tag:     reportingTokenChildTag,
			Attrs:   waBinary.Attrs{reportingTokenVersionAttr: reportingTokenVersion},
			Content: hasher.Sum(nil)[:reportingTokenLength],
		}},
	}
}

func getReportingToken(messageProtobuf []byte) []byte {
	return extractReportingTokenContent(messageProtobuf, getReportingFields())
}

// Helper to find config for a field number
func getConfigForField(fields []reportingField, fieldNum int) *reportingField {
	for i := range fields {
		if fields[i].FieldNumber == fieldNum {
			return &fields[i]
		}
	}
	return nil
}

// Extracts the reporting token content recursively.
//
// Toda leitura abaixo confere os limites antes de fatiar `data`. Hoje a entrada
// e sempre um protobuf que nos mesmos serializamos, entao malformado e
// inalcancavel — mas sem as guardas qualquer descasamento futuro entre o
// serializador e este extrator viraria panico de slice, e esta funcao roda no
// caminho de envio de mensagem. Ver PATCHES.md, Fase E lote 4.
func extractReportingTokenContent(data []byte, config []reportingField) []byte {
	type field struct {
		Num   int
		Bytes []byte
	}
	var fields []field
	i := 0
	for i < len(data) {
		// Read tag (varint)
		tag, tagLen := binary.Uvarint(data[i:])
		if tagLen <= 0 {
			break // malformed
		}
		fieldNum := int(tag >> wireFieldNumShift)
		wireType := int(tag & wireTypeMask)
		fieldCfg := getConfigForField(config, fieldNum)
		fieldStart := i
		i += tagLen
		if fieldCfg == nil {
			// Skip field
			switch wireType {
			case wireVarint:
				_, n := binary.Uvarint(data[i:])
				if n <= 0 {
					return nil
				}
				i += n
			case wire64bit:
				i += wire64bitLength
			case wireBytes:
				l, n := binary.Uvarint(data[i:])
				if n <= 0 {
					return nil
				}
				i += n + int(l)
			case wire32bit:
				i += wire32bitLength
			default:
				return nil
			}
			if i > len(data) || i < 0 {
				return nil
			}
			continue
		}
		switch wireType {
		case wireVarint:
			_, n := binary.Uvarint(data[i:])
			if n <= 0 {
				return nil
			}
			i += n
			fields = append(fields, field{Num: fieldNum, Bytes: data[fieldStart:i]})
		case wire64bit:
			i += wire64bitLength
			if i > len(data) {
				return nil
			}
			fields = append(fields, field{Num: fieldNum, Bytes: data[fieldStart:i]})
		case wireBytes:
			l, n := binary.Uvarint(data[i:])
			if n <= 0 {
				return nil
			}
			valStart := i + n
			valEnd := valStart + int(l)
			if valEnd > len(data) || valEnd < valStart {
				return nil
			}
			if fieldCfg.IsMessage || len(fieldCfg.Subfields) > 0 {
				// Recursively extract subfields
				sub := extractReportingTokenContent(data[valStart:valEnd], fieldCfg.Subfields)
				if len(sub) > 0 {
					// Re-encode tag and length
					buf := make([]byte, 0, tagLen+n+len(sub))
					tagBuf := make([]byte, binary.MaxVarintLen64)
					tagN := binary.PutUvarint(tagBuf, tag)
					lenBuf := make([]byte, binary.MaxVarintLen64)
					lenN := binary.PutUvarint(lenBuf, uint64(len(sub)))
					buf = append(buf, tagBuf[:tagN]...)
					buf = append(buf, lenBuf[:lenN]...)
					buf = append(buf, sub...)
					fields = append(fields, field{Num: fieldNum, Bytes: buf})
				}
			} else {
				fields = append(fields, field{Num: fieldNum, Bytes: data[fieldStart:valEnd]})
			}
			i = valEnd
		case wire32bit:
			i += wire32bitLength
			if i > len(data) {
				return nil
			}
			fields = append(fields, field{Num: fieldNum, Bytes: data[fieldStart:i]})
		default:
			return nil
		}
	}
	// Sort by field number
	sort.Slice(fields, func(i, j int) bool { return fields[i].Num < fields[j].Num })
	// Concatenate
	var out []byte
	for _, f := range fields {
		out = append(out, f.Bytes...)
	}
	return out
}
