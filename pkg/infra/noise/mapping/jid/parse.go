package jid

import (
	"strings"

	"wa-api/internal/noise/protocol/types"

	"github.com/rs/zerolog/log"
)

// Bounds for the bare-phone form, in digits. E.164 caps a subscriber number at
// 15; the lower bound is deliberately generous — the point is to reject text,
// not to police numbering plans, and a wrong guess here rejects a real user.
const (
	minPhoneDigits = 5
	maxPhoneDigits = 15
)

// ParseJID parses a phone number or JID string into a WhatsApp JID.
//
// A bare string (no "@") is treated as a phone number and gets the default
// server. That leniency is deliberate — F203, decision 35=a of the channel: a
// field named `Phone` that demands "@s.whatsapp.net" is a surprising contract.
//
// What is NOT deliberate, and is what F209 fixed: accepting a bare string
// WITHOUT looking at it. `{"Phone":"abc"}` became `abc@s.whatsapp.net`, went to
// the WhatsApp server, and the request hung for 75 SECONDS before answering
// 500 — measured in the field on 2026-08-21, against ~1s for a well formed
// number that has no account. The server never answers a JID like that, so the
// info query burns its whole deadline.
//
// That is worse than a wrong status code: it is a client holding a connection
// and a handler slot for 75 seconds on input we could reject in microseconds.
// Repeated, it is a cheap denial of service that costs the caller nothing.
func ParseJID(arg string) (types.JID, bool) {
	// Empty first: `arg[0]` below PANICS on "" (F101), and the port promises
	// (JID, error) — it delivered a crash instead. The only reason production
	// never saw it is that the use cases happen to guard, which is one careless
	// caller away from taking the process down. The guard belongs at the SOURCE
	// of the rule, not in each of the eleven callers.
	//
	// Achado duas vezes, por caminhos independentes (feature/wa-noise e
	// feature/wa-headless-foundation), e as duas notas estao fundidas aqui.
	if arg == "" {
		return types.JID{}, false
	}
	if arg[0] == '+' {
		arg = arg[1:]
	}
	if !strings.ContainsRune(arg, '@') {
		// A verificação vive AQUI, e não num ajudante, porque o log da recusa
		// vive aqui: separar os dois criaria uma função de decisão sem registo
		// próprio, e fazê-la registar duplicaria esta linha — as duas fontes de
		// verdade que o CLAUDE.md proíbe. Decisão e registo ficam juntos.
		//
		// Só dígitos, e num comprimento plausível. Não valida país nem operadora
		// de propósito: o objetivo é separar "número" de "texto", não policiar
		// planos de numeração que não conhecemos. Recusar de mais aqui é PIOR
		// que o defeito — quem tem um número válido deixa de conseguir usar a
		// rota, e isso não aparece em nenhum teste da guarda.
		//
		// `ContainsFunc` em vez de um laço com `break`: o laço somava três
		// ramos à complexidade de `ParseJID` e punha-a em 11, acima do limite
		// informativo do lint. A closure não conta para o gate de log
		// (regra X6 de cmd/logcov/METRIC.md — closures que não são goroutine,
		// defer nem middleware saem do denominador), então esta forma satisfaz
		// os dois gates sem mexer em baseline de nenhum.
		naoEhDigito := func(r rune) bool { return r < '0' || r > '9' }
		if len(arg) < minPhoneDigits || len(arg) > maxPhoneDigits || strings.ContainsFunc(arg, naoEhDigito) {
			log.Error().Str("raw", arg).
				Msg("bare target is not a plausible phone number; refusing before it reaches the network")
			return types.JID{}, false
		}
		return types.NewJID(arg, types.DefaultUserServer), true
	} else {
		recipient, err := types.ParseJID(arg)
		if err != nil {
			log.Error().Err(err).Msg("Invalid JID")
			return recipient, false
		} else if recipient.User == "" {
			log.Error().Err(err).Msg("Invalid JID no server specified")
			return recipient, false
		}
		return recipient, true
	}
}
