// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"slices"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

func (cli *Client) handleDeviceNotification(ctx context.Context, node *waBinary.Node) {
	cli.userDevicesCacheLock.Lock()
	defer cli.userDevicesCacheLock.Unlock()
	ag := node.AttrGetter()
	from := ag.JID("from")
	fromLID := ag.OptionalJID("lid")
	if fromLID != nil {
		cli.StoreLIDPNMapping(ctx, *fromLID, from)
	}
	cached, ok := cli.userDevicesCache[from]
	if !ok {
		cli.Log.Debugf("No device list cached for %s, ignoring device list notification", from)
		return
	}
	var cachedLID deviceCache
	var cachedLIDHash string
	if fromLID != nil {
		cachedLID = cli.userDevicesCache[*fromLID]
		cachedLIDHash = participantListHashV2(cachedLID.devices)
	}
	cachedParticipantHash := participantListHashV2(cached.devices)
	for _, child := range node.GetChildren() {
		cag := child.AttrGetter()
		deviceHash := cag.String("device_hash")
		deviceLIDHash := cag.OptionalString("device_lid_hash")
		deviceChild, _ := child.GetOptionalChildByTag("device")
		changedDeviceJID := deviceChild.AttrGetter().JID("jid")
		changedDeviceLID := deviceChild.AttrGetter().OptionalJID("lid")
		switch child.Tag {
		case "add":
			cached.devices = append(cached.devices, changedDeviceJID)
			if changedDeviceLID != nil {
				cachedLID.devices = append(cachedLID.devices, *changedDeviceLID)
			}
		case "remove":
			cached.devices = slices.DeleteFunc(cached.devices, func(existing types.JID) bool {
				return existing == changedDeviceJID
			})
			if changedDeviceLID != nil {
				cachedLID.devices = slices.DeleteFunc(cachedLID.devices, func(existing types.JID) bool {
					return existing == *changedDeviceLID
				})
			}
		case "update":
			// Exact meaning of "update" is unknown, clear device list cache to be safe
			cli.Log.Debugf("%s's device list updated, dropping cached devices", from)
			delete(cli.userDevicesCache, from)
			continue
		default:
			cli.Log.Debugf("Unknown device list change tag %s", child.Tag)
			continue
		}
		newParticipantHash := participantListHashV2(cached.devices)
		if newParticipantHash == deviceHash {
			cli.Log.Debugf("%s's device list hash changed from %s to %s (%s). New hash matches", from, cachedParticipantHash, deviceHash, child.Tag)
			cli.userDevicesCache[from] = cached
		} else {
			cli.Log.Warnf("%s's device list hash changed from %s to %s (%s). New hash doesn't match (%s)", from, cachedParticipantHash, deviceHash, child.Tag, newParticipantHash)
			delete(cli.userDevicesCache, from)
		}
		if fromLID != nil && changedDeviceLID != nil && deviceLIDHash != "" {
			newLIDParticipantHash := participantListHashV2(cachedLID.devices)
			if newLIDParticipantHash == deviceLIDHash {
				cli.Log.Debugf("%s's device list hash changed from %s to %s (%s). New hash matches", fromLID, cachedLIDHash, deviceLIDHash, child.Tag)
				cli.userDevicesCache[*fromLID] = cachedLID
			} else {
				cli.Log.Warnf("%s's device list hash changed from %s to %s (%s). New hash doesn't match (%s)", fromLID, cachedLIDHash, deviceLIDHash, child.Tag, newLIDParticipantHash)
				delete(cli.userDevicesCache, *fromLID)
			}
		}
	}
}

func (cli *Client) handleFBDeviceNotification(ctx context.Context, node *waBinary.Node) {
	cli.userDevicesCacheLock.Lock()
	defer cli.userDevicesCacheLock.Unlock()
	jid := node.AttrGetter().JID("from")
	userDevices := parseFBDeviceList(jid, node.GetChildByTag("devices"))
	cli.userDevicesCache[jid] = userDevices
}

func (cli *Client) handleOwnDevicesNotification(ctx context.Context, node *waBinary.Node, fromJID types.JID) {
	cli.userDevicesCacheLock.Lock()
	defer cli.userDevicesCacheLock.Unlock()
	ownLID := cli.getOwnLID().ToNonAD()
	ownID := cli.getOwnID().ToNonAD()
	if ownID.IsEmpty() {
		cli.Log.Debugf("Ignoring own device change notification, session was deleted")
		return
	}
	fromJIDPlain := fromJID.ToNonAD()
	var altJID types.JID
	switch fromJIDPlain {
	case ownID:
		altJID = ownLID
	case ownLID:
		altJID = ownID
	default:
		cli.Log.Warnf("Unexpected own device notification sender %s", fromJID)
		return
	}
	var oldHash string
	if cached, ok := cli.userDevicesCache[fromJIDPlain]; ok {
		oldHash = participantListHashV2(cached.devices)
	}
	expectedNewHash := node.AttrGetter().String("dhash")
	var newDeviceList, altDeviceList []types.JID
	for _, child := range node.GetChildren() {
		jid := child.AttrGetter().JID("jid")
		if child.Tag == "device" && !jid.IsEmpty() {
			newDeviceList = append(newDeviceList, jid)
			altDeviceJID := altJID
			altDeviceJID.Device = jid.Device
			altDeviceList = append(altDeviceList, altDeviceJID)
		}
	}
	newHash := participantListHashV2(newDeviceList)
	if newHash != expectedNewHash {
		cli.Log.Debugf("Received own device list change notification %s -> %s from %s, but expected hash was %s", oldHash, newHash, fromJID, expectedNewHash)
		delete(cli.userDevicesCache, ownID)
		delete(cli.userDevicesCache, ownLID)
	} else {
		cli.Log.Debugf("Received own device list change notification %s -> %s from %s", oldHash, newHash, fromJID)
		cli.userDevicesCache[fromJIDPlain] = deviceCache{devices: newDeviceList, dhash: expectedNewHash}
		cli.userDevicesCache[altJID] = deviceCache{devices: altDeviceList, dhash: participantListHashV2(altDeviceList)}
	}
}
