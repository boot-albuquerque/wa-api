package jid

import (
	"strings"

	"wa-api/internal/wa-noise/protocol/types"

	"github.com/rs/zerolog/log"
)

// ParseJID parses a phone number or JID string into a WhatsApp JID
func ParseJID(arg string) (types.JID, bool) {
	// An empty string used to panic here on arg[0] (F101): the port promises
	// (JID, error) and delivered a crash instead. The guard belongs at the
	// source of the rule, not in each of the eleven callers.
	if arg == "" {
		return types.JID{}, false
	}
	if arg[0] == '+' {
		arg = arg[1:]
	}
	if !strings.ContainsRune(arg, '@') {
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
