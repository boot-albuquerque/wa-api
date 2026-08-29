package socket

// Every WhatsApp websocket frame is prefixed by its payload length encoded as a
// big-endian 24-bit unsigned integer, i.e. exactly [FrameLengthSize] bytes.
// That is also why [FrameMaxSize] is 1<<24: it is the first length this prefix
// cannot represent.
const (
	frameLengthShiftHigh = 16
	frameLengthShiftMid  = 8
)

// encodeFrameLength writes length into the first [FrameLengthSize] bytes of dst
// as a big-endian 24-bit integer. dst must have at least [FrameLengthSize]
// bytes; length must be below [FrameMaxSize] (callers check that and return
// [ErrFrameTooLarge] otherwise) — a larger value silently loses its high bits,
// exactly as the original inline shifts did.
func encodeFrameLength(dst []byte, length int) {
	dst[0] = byte(length >> frameLengthShiftHigh)
	dst[1] = byte(length >> frameLengthShiftMid)
	dst[2] = byte(length)
}

// decodeFrameLength reads the big-endian 24-bit length prefix from the first
// [FrameLengthSize] bytes of src. src must have at least [FrameLengthSize]
// bytes; the caller checks that before calling (a shorter buffer is a partial
// header, which the read loop buffers instead of decoding).
func decodeFrameLength(src []byte) int {
	return (int(src[0]) << frameLengthShiftHigh) + (int(src[1]) << frameLengthShiftMid) + int(src[2])
}
