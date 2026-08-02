package whatsmeow

import (
	"runtime/debug"

	"github.com/rs/zerolog/log"
)

// SafeGo runs fn in a new goroutine with a defer recover so a panic inside
// fire-and-forget side-effects (webhook delivery, MQ push) cannot crash
// the whole process. Losing one delivery is preferable to taking wa-api
// down for every connected user.
func SafeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().
					Str("goroutine", name).
					Interface("panic", r).
					Str("stack", string(debug.Stack())).
					Msg("panic recovered in goroutine")
			}
		}()
		fn()
	}()
}
