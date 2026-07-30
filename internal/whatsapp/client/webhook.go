package client

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/rs/zerolog/log"
)

func (c *MyClient) sendGlobal(jsonData []byte) {
	jStr := string(jsonData)
	instanceName := ""
	if ui, found := c.Ctx.UserCache.Get(c.Token); found {
		instanceName = ui.(interface{ Get(string) string }).Get("Name")
	}
	if c.Ctx.GlobalWH != "" {
		c.Ctx.CallHook(c.Ctx.GlobalWH, map[string]string{
			"jsonData": jStr, "userID": c.UserID, "instanceName": instanceName,
		}, c.UserID, c.Ctx.GlobalHMACK)
	}
}

func (c *MyClient) sendUser(wh, path string, jsonData, hk []byte) {
	instanceName := ""
	if ui, found := c.Ctx.UserCache.Get(c.Token); found {
		instanceName = ui.(interface{ Get(string) string }).Get("Name")
	}
	d := map[string]string{"jsonData": string(jsonData), "userID": c.UserID, "instanceName": instanceName}
	if wh != "" {
		if path == "" {
			SafeGo("callHookWithHmac", func() { c.Ctx.CallHook(wh, d, c.UserID, hk) })
		} else {
			if err := c.Ctx.CallHookFile(wh, d, c.UserID, path, hk); err != nil {
				log.Error().Err(err).Msg("Error calling hook file")
			}
		}
	}
}

func (c *MyClient) updateSubs() ([]string, error) {
	ce := ""
	if ui, found := c.Ctx.UserCache.Get(c.Token); found {
		ce = ui.(interface{ Get(string) string }).Get("Events")
	} else {
		if err := c.Ctx.DB.Get(&ce, "SELECT events FROM users WHERE id=$1", c.UserID); err != nil {
			return nil, err
		}
	}
	ea := strings.Split(ce, ",")
	var se []string
	if !(len(ea) == 1 && ea[0] == "") {
		for _, a := range ea {
			a = strings.TrimSpace(a)
			if a != "" && c.Ctx.Find(c.Ctx.SuppTypes, a) {
				se = append(se, a)
			}
		}
	}
	return se, nil
}

func (c *MyClient) whURL() string {
	if ui, found := c.Ctx.UserCache.Get(c.Token); found {
		return ui.(interface{ Get(string) string }).Get("Webhook")
	}
	return ""
}

func subOK(find func([]string, string) bool, sub []string, et, uid string) bool {
	if !find(sub, et) && !find(sub, "All") {
		log.Warn().Str("type", et).Strs("subscribed", sub).Str("user", uid).Msg("not subscribed")
		return false
	}
	return true
}

// SendEvent dispatches an event to webhook, MQ, or stdio.
func (c *MyClient) SendEvent(postmap map[string]interface{}, path string) {
	wh := c.whURL()
	sub, err := c.updateSubs()
	if err != nil {
		return
	}
	et, _ := postmap["type"].(string)
	if !subOK(c.Ctx.Find, sub, et, c.UserID) {
		return
	}
	if c.Mode == Stdio {
		if c.NotifyFn != nil {
			c.NotifyFn(et, postmap)
		}
		return
	}
	jd, err := json.Marshal(postmap)
	if err != nil {
		return
	}
	var hk []byte
	if ui, found := c.Ctx.UserCache.Get(c.Token); found {
		if b := ui.(interface{ Get(string) string }).Get("HmacKeyEncrypted"); b != "" {
			hk, _ = base64.StdEncoding.DecodeString(b)
		}
	}
	c.sendUser(wh, path, jd, hk)
	SafeGo("sendToGlobalWebHook", func() { c.sendGlobal(jd) })
	SafeGo("sendToGlobalRabbit", func() { c.Ctx.Rabbit(jd, c.Token, c.UserID) })
}

// ResolveConnectEvents decides event subscription string on (re)connect.
func ResolveConnectEvents(find func([]string, string) bool, supported []string, subscribe []string, existing string) (evStr string, changed bool) {
	if len(subscribe) < 1 {
		return existing, false
	}
	var subscribed []string
	for _, arg := range subscribe {
		if !find(supported, arg) {
			log.Warn().Str("Type", arg).Msg("Event type discarded")
			continue
		}
		if !find(subscribed, arg) {
			subscribed = append(subscribed, arg)
		}
	}
	resolved := strings.Join(subscribed, ",")
	return resolved, resolved != existing
}
