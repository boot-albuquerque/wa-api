package port

import "time"

// SessionEvent cobre exatamente os eventos de sessão/transporte que o
// SessionOrchestrator (Fase 2 do plano de arquitetura multi-sessão nativa)
// precisa observar para gravar users.connected/qrcode e disparar webhook de
// status: Connected, Disconnected, LoggedOut, PairSuccess, QR,
// StreamReplaced. Eventos de domínio (mensagem, presença, grupo, histórico)
// ficam fora — continuam tratados por myEventHandler
// (pkg/bootstrap/eventhandler.go), que este tipo não substitui.
//
// Escolha de design: struct única com Kind + um ponteiro por variante que
// carrega payload, em vez de interface com tipos concretos. Idiomático em Go
// (mesmo padrão de watch.Event do client-go e de rpc.Response com campos
// opcionais) e evita que consumidores façam type-switch só para ler Kind —
// a maioria dos usos do SessionOrchestrator só precisa saber qual dos 6
// aconteceu. Nenhuma variante usa `any`: cada ponteiro aponta para um tipo
// próprio com campos tipados, extraídos do que os handlers hoje em
// pkg/bootstrap/eventhandler_session.go de fato consomem.
type SessionEvent struct {
	Kind SessionEventKind

	Disconnected *SessionDisconnectedEvent
	LoggedOut    *SessionLoggedOutEvent
	PairSuccess  *SessionPairSuccessEvent
	QR           *SessionQREvent
}

// SessionEventKind identifica qual dos 6 tipos de SessionEvent ocorreu.
type SessionEventKind string

const (
	SessionEventKindConnected      SessionEventKind = "connected"
	SessionEventKindDisconnected   SessionEventKind = "disconnected"
	SessionEventKindLoggedOut      SessionEventKind = "logged_out"
	SessionEventKindPairSuccess    SessionEventKind = "pair_success"
	SessionEventKindQR             SessionEventKind = "qr"
	SessionEventKindStreamReplaced SessionEventKind = "stream_replaced"
)

// SessionDisconnectedEvent acompanha SessionEventKindDisconnected.
//
// Reason espelha o que handleDisconnected hoje loga via
// fmt.Sprintf("%+v", evt) — events.Disconnected do whatsmeow não carrega
// campo estruturado, só o próprio evento vazio; o adapter formata a mesma
// string que já ia pro log.
type SessionDisconnectedEvent struct {
	Reason string
}

// SessionLoggedOutEvent acompanha SessionEventKindLoggedOut. Reason espelha
// evt.Reason.String() (handleLoggedOut, eventhandler_session.go:212).
type SessionLoggedOutEvent struct {
	Reason string
}

// SessionPairSuccessEvent acompanha SessionEventKindPairSuccess. Campos
// espelham o que handlePairSuccess consome de events.PairSuccess
// (eventhandler_session.go:80-102): JID resultante do pareamento, nome
// comercial e plataforma reportados pelo WhatsApp.
type SessionPairSuccessEvent struct {
	JID          string
	BusinessName string
	Platform     string
}

// SessionQREvent acompanha SessionEventKindQR. Code é o payload cru do QR
// (o mesmo que lifecycle.go hoje codifica em base64 e grava em
// users.qrcode); Timeout é a validade real daquele código específico (20s
// para os 5 primeiros, 60s para o último — ver comentário em
// lifecycle.go:341-344), usada para calcular o expiresAt enviado no
// webhook.
type SessionQREvent struct {
	Code    string
	Timeout time.Duration
}
