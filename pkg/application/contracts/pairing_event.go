package port

import "time"

// PairingEvent cobre o fluxo de pareamento hoje em lifecycle.go:295-389: o
// canal de eventos que client.GetQRChannel devolve, com seus três casos
// observados (evt.Event == "code" | "timeout" | "success"). É a porta pela
// qual Session.Pair (Fase 2, session_provider.go) expõe esse fluxo sem
// vazar o tipo whatsmeow.QRChannelItem.
//
// Mesma escolha de design de SessionEvent: struct única com Kind + campos
// diretos em vez de uma variante por tipo, porque os três casos aqui não têm
// payloads conflitantes (Code/Timeout só fazem sentido em Kind == QR, JID só
// em Kind == Success) e não há um quarto consumidor previsto que justifique
// o custo extra de tipos aninhados.
type PairingEvent struct {
	Kind PairingEventKind

	// Code é o payload cru do QR (lifecycle.go:317, evt.Code antes da
	// codificação base64 feita hoje inline em lifecycle.go). Só populado
	// quando Kind == PairingEventKindQR.
	Code string

	// Timeout é a validade real deste código específico (20s para os 5
	// primeiros, 60s para o último — lifecycle.go:341-344, evt.Timeout). Só
	// populado quando Kind == PairingEventKindQR.
	Timeout time.Duration

	// JID é o identificador resultante do pareamento (lifecycle.go:378,
	// gravado em users.jid após "success"). Só populado quando
	// Kind == PairingEventKindSuccess.
	JID string
}

// PairingEventKind identifica qual dos 3 casos do canal de pareamento
// ocorreu: código de QR emitido, QR expirado sem scan, ou pareamento
// concluído.
type PairingEventKind string

const (
	PairingEventKindQR      PairingEventKind = "qr"
	PairingEventKindTimeout PairingEventKind = "timeout"
	PairingEventKindSuccess PairingEventKind = "success"
)
