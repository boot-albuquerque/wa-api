// Package client provides WhatsApp helpers extracted from root
// wuzapi/wmiau.go (Phase 11a Clean Architecture refactor).
package client

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"runtime/debug"

	"github.com/jmoiron/sqlx"
	"github.com/patrickmn/go-cache"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

// ServerMode represents the server operating mode.
type ServerMode int

const (
	HTTP  ServerMode = iota
	Stdio
)

// Context bundles external dependencies bridged from root wuzapi globals.
type Context struct {
	UserCache   *cache.Cache
	GlobalWH    string
	GlobalHMACK []byte
	SuppTypes   []string
	Find        func([]string, string) bool
	UpdUI       func(interface{}, string, string) interface{}

	CallHook     func(url string, p map[string]string, uid string, hk []byte)
	CallHookFile func(url string, p map[string]string, uid, file string, hk []byte) error
	Rabbit       func(jd []byte, token, uid string)

	EnsureS3 func(uid string)

	SaveMsg func(db *sqlx.DB, uid, cjid, sjid, mid, mt, txt, ml, qid, json string) error
	TrimMsg func(db *sqlx.DB, uid, cjid string, limit int) error

	PollOpts func(uid, mid string) []string
	SetPO    func(uid, mid string, opts []string)

	SetWA func(uid string, c *whatsmeow.Client)
	DelWA func(uid string)
	SetMC func(uid string, c *MyClient)
	DelMC func(uid string)

	KSig func(uid string)
	KSet func(uid string, ch chan bool)
	KDel func(uid string, ch chan bool)

	Container interface {
		GetDevice(ctx context.Context, jid types.JID) (*store.Device, error)
		NewDevice() *store.Device
	}

	SkipMedia   bool
	WaDebug     string
	ColorOutput bool
	OsName      string
	PlatType    string
	LogType     string
	DB          *sqlx.DB
}

// MyClient wraps a whatsmeow.Client with metadata for event handling.
type MyClient struct {
	*whatsmeow.Client
	handlerID uint32
	UserID    string
	Token     string
	NotifyFn  func(method string, params map[string]interface{})
	Mode      ServerMode
	Ctx       *Context
}

// NewMyClient creates a MyClient.
func NewMyClient(ctx *Context, c *whatsmeow.Client, uid, token string) *MyClient {
	return &MyClient{Client: c, Ctx: ctx, UserID: uid, Token: token}
}

// RegisterHandler attaches an event handler function and returns the handler ID.
func (c *MyClient) RegisterHandler(handler func(interface{})) uint32 {
	c.handlerID = c.Client.AddEventHandler(handler)
	return c.handlerID
}

// SafeGo runs fn in a goroutine with panic recovery.
func SafeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Str("goroutine", name).Interface("panic", r).
					Str("stack", string(debug.Stack())).Msg("panic recovered")
			}
		}()
		fn()
	}()
}

// ParseJID parses a JID string.
func ParseJID(arg string) (types.JID, bool) {
	if arg == "" {
		return types.EmptyJID, false
	}
	jid, err := types.ParseJID(arg)
	return jid, err == nil
}

// GetPlatformTypeEnum converts platform string to proto enum.
func GetPlatformTypeEnum(pt string) *waCompanionReg.DeviceProps_PlatformType {
	switch pt {
	case "DESKTOP":
		return waCompanionReg.DeviceProps_DESKTOP.Enum()
	case "IPAD":
		return waCompanionReg.DeviceProps_IPAD.Enum()
	case "ANDROID_TABLET":
		return waCompanionReg.DeviceProps_ANDROID_TABLET.Enum()
	case "IOS_PHONE":
		return waCompanionReg.DeviceProps_IOS_PHONE.Enum()
	case "ANDROID_PHONE":
		return waCompanionReg.DeviceProps_ANDROID_PHONE.Enum()
	default:
		return waCompanionReg.DeviceProps_DESKTOP.Enum()
	}
}

// FileToBase64 reads a file and returns base64 + mime type.
func FileToBase64(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(data), http.DetectContentType(data), nil
}
