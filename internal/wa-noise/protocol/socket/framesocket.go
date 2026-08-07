package socket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/coder/websocket"

	waLog "wa-api/internal/wa-noise/observability/log"
)

type FrameSocket struct {
	parentCtx context.Context
	cancelCtx context.Context
	cancel    context.CancelFunc
	conn      *websocket.Conn
	log       waLog.Logger
	lock      sync.Mutex

	URL         string
	HTTPHeaders http.Header
	HTTPClient  *http.Client

	Frames chan []byte

	// onDisconnect e' lido por Close, que roda no goroutine do read pump, e
	// escrito por newNoiseSocket/NoiseSocket.Stop, que rodam em outro. Como
	// campo exportado sem sincronizacao isso era data race: Connect ja'
	// disparou o read pump ANTES de newNoiseSocket escrever aqui, e a janela
	// entre os dois cobre o handshake Noise inteiro (F56 em HOUSEKEEP.md).
	//
	// Por isso o campo e' privado e so' se mexe nele por SetOnDisconnect, sob o
	// mesmo fs.lock que Close ja' toma.
	onDisconnect func(ctx context.Context, remote bool)

	Header []byte

	closed bool

	incomingLength int
	receivedLength int
	incoming       []byte
	partialHeader  []byte
}

func NewFrameSocket(log waLog.Logger, client *http.Client) *FrameSocket {
	return &FrameSocket{
		log:    log,
		Header: WAConnHeader,
		Frames: make(chan []byte),

		URL:         URL,
		HTTPHeaders: http.Header{originHeaderName: {Origin}},
		HTTPClient:  client,
	}
}

func (fs *FrameSocket) IsConnected() bool {
	return fs.conn != nil
}

func (fs *FrameSocket) Close(code websocket.StatusCode) {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	if fs.conn == nil {
		return
	}

	fs.closed = true
	if code > statusForceClose {
		err := fs.conn.Close(code, "")
		if err != nil {
			fs.log.Warnf("Error sending close to websocket: %v", err)
		}
	} else {
		err := fs.conn.CloseNow()
		if err != nil {
			fs.log.Debugf("Error force closing websocket: %v", err)
		}
	}
	fs.conn = nil
	fs.cancel()
	fs.cancel = nil
	if fs.onDisconnect != nil {
		go fs.onDisconnect(fs.parentCtx, code == statusForceClose)
	}
}

// SetOnDisconnect registra (ou limpa, com nil) o callback de desconexao.
//
// Toma o mesmo lock de Close: e' isso que da' happens-before entre esta escrita
// e a leitura feita pelo read pump.
func (fs *FrameSocket) SetOnDisconnect(fn func(ctx context.Context, remote bool)) {
	fs.lock.Lock()
	defer fs.lock.Unlock()
	fs.onDisconnect = fn
}

func (fs *FrameSocket) Connect(ctx context.Context) error {
	fs.lock.Lock()
	defer fs.lock.Unlock()
	if fs.conn != nil {
		return ErrSocketAlreadyOpen
	}
	fs.parentCtx = ctx
	fs.cancelCtx, fs.cancel = context.WithCancel(ctx)

	fs.log.Debugf("Dialing %s", fs.URL)
	conn, resp, err := websocket.Dial(ctx, fs.URL, fs.makeDialOptions())
	if err != nil {
		if resp != nil {
			err = ErrWithStatusCode{err, resp.StatusCode}
		}
		fs.cancel()
		return fmt.Errorf("%w: %w", ErrDialFailed, err)
	}
	conn.SetReadLimit(FrameMaxSize)

	fs.conn = conn

	go fs.readPump(conn, ctx)
	return nil
}

func (fs *FrameSocket) Context() context.Context {
	return fs.cancelCtx
}

func (fs *FrameSocket) SendFrame(data []byte) error {
	conn := fs.conn
	if conn == nil {
		return ErrSocketClosed
	}
	dataLength := len(data)
	if dataLength >= FrameMaxSize {
		return fmt.Errorf("%w (got %d bytes, max %d bytes)", ErrFrameTooLarge, len(data), FrameMaxSize)
	}

	headerLength := len(fs.Header)
	// Whole frame is header + 3 bytes for length + data
	wholeFrame := make([]byte, headerLength+FrameLengthSize+dataLength)

	// Copy the header if it's there
	if fs.Header != nil {
		copy(wholeFrame[:headerLength], fs.Header)
		// We only want to send the header once
		fs.Header = nil
	}

	// Encode length of frame
	encodeFrameLength(wholeFrame[headerLength:], dataLength)

	// Copy actual frame data
	copy(wholeFrame[headerLength+FrameLengthSize:], data)

	return conn.Write(fs.cancelCtx, websocket.MessageBinary, wholeFrame)
}

func (fs *FrameSocket) frameComplete() {
	data := fs.incoming
	fs.incoming = nil
	fs.partialHeader = nil
	fs.incomingLength = 0
	fs.receivedLength = 0
	fs.Frames <- data
}

func (fs *FrameSocket) processData(msg []byte) {
	for len(msg) > 0 {
		// This probably doesn't happen a lot (if at all), so the code is unoptimized
		if fs.partialHeader != nil {
			msg = append(fs.partialHeader, msg...)
			fs.partialHeader = nil
		}
		if fs.incoming == nil {
			if len(msg) >= FrameLengthSize {
				length := decodeFrameLength(msg)
				fs.incomingLength = length
				// receivedLength e' contado DEPOIS de descartar o cabecalho.
				// Era `len(msg)` antes do corte, ou seja, incluia os
				// FrameLengthSize bytes de cabecalho — enquanto
				// incomingLength e todos os copy(fs.incoming[receivedLength:])
				// do ramo de continuacao sao relativos ao PAYLOAD. Com o
				// payload chegando picado em mais de um websocket message, o
				// segundo pedaco era escrito FrameLengthSize bytes adiante do
				// lugar certo, deixando bytes zerados no meio do frame
				// remontado e truncando o fim (F18 em HOUSEKEEP.md).
				msg = msg[FrameLengthSize:]
				fs.receivedLength = len(msg)
				if len(msg) >= length {
					fs.incoming = msg[:length]
					msg = msg[length:]
					fs.frameComplete()
				} else {
					fs.incoming = make([]byte, length)
					copy(fs.incoming, msg)
					msg = nil
				}
			} else {
				fs.log.Warnf("Received partial header (report if this happens often)")
				fs.partialHeader = msg
				msg = nil
			}
		} else {
			if fs.receivedLength+len(msg) >= fs.incomingLength {
				copy(fs.incoming[fs.receivedLength:], msg[:fs.incomingLength-fs.receivedLength])
				msg = msg[fs.incomingLength-fs.receivedLength:]
				fs.frameComplete()
			} else {
				copy(fs.incoming[fs.receivedLength:], msg)
				fs.receivedLength += len(msg)
				msg = nil
			}
		}
	}
}

func (fs *FrameSocket) readPump(conn *websocket.Conn, ctx context.Context) {
	fs.log.Debugf("Frame websocket read pump starting %p", fs)
	defer func() {
		fs.log.Debugf("Frame websocket read pump exiting %p", fs)
		go fs.Close(statusForceClose)
	}()
	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			// Ignore the error if the context has been closed
			if !fs.closed && !errors.Is(ctx.Err(), context.Canceled) {
				fs.log.Errorf("Error reading from websocket: %v", err)
			}
			return
		} else if msgType != websocket.MessageBinary {
			fs.log.Warnf("Got unexpected websocket message type %d", msgType)
			continue
		}
		fs.processData(data)
	}
}
