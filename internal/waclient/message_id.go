// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/util/random"

	"wa-api/internal/waclient/types"
)

const WebMessageIDPrefix = "3EB0"

// GenerateMessageID generates a random string that can be used as a message ID on WhatsApp.
//
//	msgID := cli.GenerateMessageID()
//	cli.SendMessage(context.Background(), targetJID, &waE2E.Message{...}, whatsmeow.SendRequestExtra{ID: msgID})
func (cli *Client) GenerateMessageID() types.MessageID {
	if cli != nil && cli.MessengerConfig != nil {
		return types.MessageID(strconv.FormatInt(GenerateFacebookMessageID(), 10))
	}
	data := make([]byte, 8, 8+20+16)
	binary.BigEndian.PutUint64(data, uint64(time.Now().Unix()))
	ownID := cli.getOwnID()
	if !ownID.IsEmpty() {
		data = append(data, []byte(ownID.User)...)
		data = append(data, []byte("@c.us")...)
	}
	data = append(data, random.Bytes(16)...)
	hash := sha256.Sum256(data)
	return WebMessageIDPrefix + strings.ToUpper(hex.EncodeToString(hash[:9]))
}

func GenerateFacebookMessageID() int64 {
	const randomMask = (1 << 22) - 1
	return (time.Now().UnixMilli() << 22) | (int64(binary.BigEndian.Uint32(random.Bytes(4))) & randomMask)
}

// GenerateMessageID generates a random string that can be used as a message ID on WhatsApp.
//
//	msgID := whatsmeow.GenerateMessageID()
//	cli.SendMessage(context.Background(), targetJID, &waE2E.Message{...}, whatsmeow.SendRequestExtra{ID: msgID})
//
// Deprecated: WhatsApp web has switched to using a hash of the current timestamp, user id and random bytes. Use Client.GenerateMessageID instead.
func GenerateMessageID() types.MessageID {
	return WebMessageIDPrefix + strings.ToUpper(hex.EncodeToString(random.Bytes(8)))
}
