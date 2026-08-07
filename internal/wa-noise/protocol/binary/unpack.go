package binary

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
)

// Unpack unpacks the given decrypted data from the WhatsApp web API.
//
// It checks the first byte to decide whether to uncompress the data with zlib or just return as-is
// (without the first byte). There's currently no corresponding Pack function because Marshal
// already returns the data with a leading zero (i.e. not compressed).
func Unpack(data []byte) ([]byte, error) {
	// data vem decifrado do socket: e' entrada nao confiavel, e um frame de
	// zero bytes chegava aqui e dava panic no data[0] (F25 em HOUSEKEEP.md).
	if len(data) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	dataType, data := data[0], data[1:]
	if zlibCompressedFlag&dataType > 0 {
		if decompressor, err := zlib.NewReader(bytes.NewReader(data)); err != nil {
			return nil, fmt.Errorf("failed to create zlib reader: %w", err)
		} else if data, err = io.ReadAll(decompressor); err != nil {
			return nil, err
		}
	}
	return data, nil
}
