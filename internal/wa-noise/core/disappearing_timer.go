package wanoise

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
)

const (
	DisappearingTimerOff     = time.Duration(0)
	DisappearingTimer24Hours = 24 * time.Hour
	DisappearingTimer7Days   = 7 * 24 * time.Hour
	DisappearingTimer90Days  = 90 * 24 * time.Hour
)

// ParseDisappearingTimerString parses common human-readable disappearing message timer strings into Duration values.
// If the string doesn't look like one of the allowed values (0, 24h, 7d, 90d), the second return value is false.
func ParseDisappearingTimerString(val string) (time.Duration, bool) {
	switch strings.ReplaceAll(strings.ToLower(val), " ", "") {
	case "0d", "0h", "0s", "0", "off":
		return DisappearingTimerOff, true
	case "1day", "day", "1d", "1", "24h", "24", "86400s", "86400":
		return DisappearingTimer24Hours, true
	case "1week", "week", "7d", "7", "168h", "168", "604800s", "604800":
		return DisappearingTimer7Days, true
	case "3months", "3m", "3mo", "90d", "90", "2160h", "2160", "7776000s", "7776000":
		return DisappearingTimer90Days, true
	default:
		return 0, false
	}
}

// SetDisappearingTimer sets the disappearing timer in a chat. Both private chats and groups are supported, but they're
// set with different methods.
//
// Note that while this function allows passing non-standard durations, official WhatsApp apps will ignore those,
// and in groups the server will just reject the change. You can use the DisappearingTimer<Duration> constants for convenience.
//
// In groups, the server will echo the change as a notification, so it'll show up as a *events.GroupInfo update.
func (cli *Client) SetDisappearingTimer(ctx context.Context, chat types.JID, timer time.Duration, settingTS time.Time) (err error) {
	switch chat.Server {
	case types.DefaultUserServer, types.HiddenUserServer:
		if settingTS.IsZero() {
			settingTS = time.Now()
		}
		_, err = cli.SendMessage(ctx, chat, &waE2E.Message{
			ProtocolMessage: &waE2E.ProtocolMessage{
				Type:                      waE2E.ProtocolMessage_EPHEMERAL_SETTING.Enum(),
				EphemeralExpiration:       proto.Uint32(uint32(timer.Seconds())),
				EphemeralSettingTimestamp: proto.Int64(settingTS.Unix()),
			},
		})
	case types.GroupServer:
		if timer == 0 {
			_, err = cli.sendGroupIQ(ctx, iqSet, chat, waBinary.Node{Tag: "not_ephemeral"})
		} else {
			_, err = cli.sendGroupIQ(ctx, iqSet, chat, waBinary.Node{
				Tag: "ephemeral",
				Attrs: waBinary.Attrs{
					"expiration": strconv.Itoa(int(timer.Seconds())),
				},
			})
			if errors.Is(err, ErrIQBadRequest) {
				err = wrapIQError(ErrInvalidDisappearingTimer, err)
			}
		}
	default:
		err = fmt.Errorf("can't set disappearing time in a %s chat", chat.Server)
	}
	return
}
