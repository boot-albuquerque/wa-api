package core

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	sdklog "wa-api/internal/noise/observability/log"
	"wa-api/internal/noise/protocol/types/events"
)

// TestQRChannelHandleEventConcurrentTerminalEventsNaoFechaCanalDuasVezes trava
// o F33 (HOUSEKEEP.md): dois eventos terminais concorrentes (ex.: PairError
// seguido de perto por Disconnected, a sequencia normal de uma falha de
// pareamento) nao podem os dois chegar ao close(qrc.stopQRs)/close(qrc.output).
// Antes do fix, o close ficava FORA do CompareAndSwap que serializa os dois
// caminhos — duas goroutines podiam ambas passar pela checagem de "ja fechado"
// e as duas tentarem fechar o mesmo canal, gerando panic de "close of closed
// channel".
func TestQRChannelHandleEventConcurrentTerminalEventsNaoFechaCanalDuasVezes(t *testing.T) {
	t.Parallel()

	cli := &Client{Log: sdklog.Noop}
	ch := make(chan QRChannelItem, 64)
	qrc := &qrChannel{
		cli:     cli,
		log:     sdklog.Noop,
		ctx:     context.Background(),
		output:  ch,
		stopQRs: make(chan struct{}),
	}
	qrc.handlerID = cli.AddEventHandler(func(interface{}) {})

	const goroutines = 50
	var wg sync.WaitGroup
	var panics int32
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt32(&panics, 1)
					t.Errorf("handleEvent entrou em panic (goroutine %d): %v", i, r)
				}
			}()
			if i%2 == 0 {
				qrc.handleEvent(&events.PairError{Error: errors.New("falha de pareamento simulada")})
			} else {
				qrc.handleEvent(&events.Disconnected{})
			}
		}(i)
	}
	wg.Wait()

	if panics != 0 {
		t.Fatalf("%d de %d goroutines entraram em panic ao fechar o canal concorrentemente", panics, goroutines)
	}
}

// TestEmitQRsFirstCodeTimeoutMatchesOfficial locks HOUSEKEEP.md F69 item 2:
// the first QR code must be as short-lived as every other code (~20s),
// matching the officially measured WhatsApp Web behavior, instead of the
// upstream whatsmeow default of 60s. A pairing QR is a credential; the
// wider window on the first code had no offsetting benefit.
func TestEmitQRsFirstCodeTimeoutMatchesOfficial(t *testing.T) {
	t.Parallel()

	output := make(chan QRChannelItem, qrCodeFirstBatchSize)
	stopQRs := make(chan struct{})
	defer close(stopQRs)

	qrc := &qrChannel{
		log:     sdklog.Noop,
		ctx:     context.Background(),
		output:  output,
		stopQRs: stopQRs,
	}

	codes := make([]string, qrCodeFirstBatchSize)
	for i := range codes {
		codes[i] = "code"
	}
	go qrc.emitQRs(codes)

	first := <-output
	if first.Timeout != qrCodeTimeout {
		t.Fatalf("first QR code timeout = %v, want %v (parity with qrCodeTimeout, HOUSEKEEP.md F69 item 2)", first.Timeout, qrCodeTimeout)
	}
}
