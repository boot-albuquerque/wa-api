package chat

import "time"

// parseDisappearingDuration converts a human-readable duration string into
// the time.Duration the SDK expects. Returns false for unrecognized values.
// Accepted: "0" / "off" (disable), "24h", "7d", "90d".
func parseDisappearingDuration(val string) (time.Duration, bool) {
	switch val {
	case "0", "off":
		return 0, true
	case "24h":
		return 24 * time.Hour, true
	case "7d":
		return 7 * 24 * time.Hour, true
	case "90d":
		return 90 * 24 * time.Hour, true
	default:
		return 0, false
	}
}
