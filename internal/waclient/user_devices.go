// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"
	"slices"

	waBinary "wa-api/internal/waclient/binary"
	"wa-api/internal/waclient/types"
)

func (cli *Client) GetUserDevicesContext(ctx context.Context, jids []types.JID) ([]types.JID, error) {
	return cli.GetUserDevices(ctx, jids)
}

// GetUserDevices gets the list of devices that the given user has. The input should be a list of
// regular JIDs, and the output will be a list of AD JIDs. The local device will not be included in
// the output even if the user's JID is included in the input. All other devices will be included.
func (cli *Client) GetUserDevices(ctx context.Context, jids []types.JID) ([]types.JID, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	cli.userDevicesCacheLock.Lock()
	defer cli.userDevicesCacheLock.Unlock()

	var devices, jidsToSync, fbJIDsToSync []types.JID
	for _, jid := range jids {
		cached, ok := cli.userDevicesCache[jid]
		if ok && len(cached.devices) > 0 {
			devices = append(devices, cached.devices...)
		} else if jid.Server == types.MessengerServer {
			fbJIDsToSync = append(fbJIDsToSync, jid)
		} else if jid.IsBot() {
			// Bot JIDs do not have devices, the usync query is empty
			devices = append(devices, jid)
		} else {
			jidsToSync = append(jidsToSync, jid)
		}
	}
	if len(jidsToSync) > 0 {
		list, err := cli.usync(ctx, jidsToSync, "query", "message", []waBinary.Node{
			{Tag: "devices", Attrs: waBinary.Attrs{"version": "2"}},
		})
		if err != nil {
			return nil, err
		}

		for _, user := range list.GetChildren() {
			jid, jidOK := user.Attrs["jid"].(types.JID)
			if user.Tag != "user" || !jidOK {
				continue
			}
			userDevices := parseDeviceList(jid, user.GetChildByTag("devices"))
			cli.userDevicesCache[jid] = deviceCache{devices: userDevices, dhash: participantListHashV2(userDevices)}
			devices = append(devices, userDevices...)
		}
	}

	if len(fbJIDsToSync) > 0 {
		userDevices, err := cli.getFBIDDevices(ctx, fbJIDsToSync)
		if err != nil {
			return nil, err
		}
		devices = append(devices, userDevices...)
	}

	return devices, nil
}

func parseDeviceList(user types.JID, deviceNode waBinary.Node) []types.JID {
	deviceList := deviceNode.GetChildByTag("device-list")
	if deviceNode.Tag != "devices" || deviceList.Tag != "device-list" {
		return nil
	}
	children := deviceList.GetChildren()
	devices := make([]types.JID, 0, len(children))
	for _, device := range children {
		deviceID, ok := device.AttrGetter().GetInt64("id", true)
		isHosted := device.AttrGetter().Bool("is_hosted")
		if device.Tag != "device" || !ok {
			continue
		}
		user.Device = uint16(deviceID)
		if isHosted {
			hostedUser := user
			if user.Server == types.HiddenUserServer {
				hostedUser.Server = types.HostedLIDServer
			} else {
				hostedUser.Server = types.HostedServer
			}
			devices = append(devices, hostedUser)
		} else {
			devices = append(devices, user)
		}
	}
	return devices
}

func parseFBDeviceList(user types.JID, deviceList waBinary.Node) deviceCache {
	children := deviceList.GetChildren()
	devices := make([]types.JID, 0, len(children))
	for _, device := range children {
		deviceID, ok := device.AttrGetter().GetInt64("id", true)
		if device.Tag != "device" || !ok {
			continue
		}
		user.Device = uint16(deviceID)
		devices = append(devices, user)
		// TODO take identities here too?
	}
	// TODO do something with the icdc blob?
	return deviceCache{
		devices: devices,
		dhash:   deviceList.AttrGetter().String("dhash"),
	}
}

func (cli *Client) getFBIDDevicesInternal(ctx context.Context, jids []types.JID) (*waBinary.Node, error) {
	users := make([]waBinary.Node, len(jids))
	for i, jid := range jids {
		users[i].Tag = "user"
		users[i].Attrs = waBinary.Attrs{"jid": jid}
		// TODO include dhash for users
	}
	resp, err := cli.sendIQ(ctx, infoQuery{
		Namespace: "fbid:devices",
		Type:      iqGet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag:     "users",
			Content: users,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send usync query: %w", err)
	} else if list, ok := resp.GetOptionalChildByTag("users"); !ok {
		return nil, &ElementMissingError{Tag: "users", In: "response to fbid devices query"}
	} else {
		return &list, err
	}
}

func (cli *Client) getFBIDDevices(ctx context.Context, jids []types.JID) ([]types.JID, error) {
	var devices []types.JID
	for chunk := range slices.Chunk(jids, 15) {
		list, err := cli.getFBIDDevicesInternal(ctx, chunk)
		if err != nil {
			return nil, err
		}
		for _, user := range list.GetChildren() {
			jid, jidOK := user.Attrs["jid"].(types.JID)
			if user.Tag != "user" || !jidOK {
				continue
			}
			userDevices := parseFBDeviceList(jid, user.GetChildByTag("devices"))
			cli.userDevicesCache[jid] = userDevices
			devices = append(devices, userDevices.devices...)
		}
	}
	return devices, nil
}
