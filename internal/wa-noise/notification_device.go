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
	cache := &cli.userDevicesCache
	cache.Lock()
	defer cache.Unlock()
	ag := node.AttrGetter()
	from := ag.JID("from")
	fromLID := ag.OptionalJID("lid")
	if fromLID != nil {
		cli.StoreLIDPNMapping(ctx, *fromLID, from)
	}
	cached, ok := cache.GetLocked(from)
	if !ok {
		cli.Log.Debugf("No device list cached for %s, ignoring device list notification", from)
		return
	}
	var cachedLID deviceCache
	var cachedLIDHash string
	if fromLID != nil {
		cachedLID, _ = cache.GetLocked(*fromLID)
		cachedLIDHash = participantListHashV2(cachedLID.Devices)
	}
	cachedParticipantHash := participantListHashV2(cached.Devices)
	for _, child := range node.GetChildren() {
		cag := child.AttrGetter()
		deviceHash := cag.String("device_hash")
		deviceLIDHash := cag.OptionalString("device_lid_hash")
		deviceChild, _ := child.GetOptionalChildByTag("device")
		changedDeviceJID := deviceChild.AttrGetter().JID("jid")
		changedDeviceLID := deviceChild.AttrGetter().OptionalJID("lid")
		switch child.Tag {
		case "add":
			cached.Devices = append(cached.Devices, changedDeviceJID)
			if changedDeviceLID != nil {
				cachedLID.Devices = append(cachedLID.Devices, *changedDeviceLID)
			}
		case "remove":
			cached.Devices = slices.DeleteFunc(cached.Devices, func(existing types.JID) bool {
				return existing == changedDeviceJID
			})
			if changedDeviceLID != nil {
				cachedLID.Devices = slices.DeleteFunc(cachedLID.Devices, func(existing types.JID) bool {
					return existing == *changedDeviceLID
				})
			}
		case "update":
			// Exact meaning of "update" is unknown, clear device list cache to be safe
			cli.Log.Debugf("%s's device list updated, dropping cached devices", from)
			cache.DeleteLocked(from)
			continue
		default:
			cli.Log.Debugf("Unknown device list change tag %s", child.Tag)
			continue
		}
		newParticipantHash := participantListHashV2(cached.Devices)
		if newParticipantHash == deviceHash {
			cli.Log.Debugf("%s's device list hash changed from %s to %s (%s). New hash matches", from, cachedParticipantHash, deviceHash, child.Tag)
			cache.SetLocked(from, cached)
		} else {
			cli.Log.Warnf("%s's device list hash changed from %s to %s (%s). New hash doesn't match (%s)", from, cachedParticipantHash, deviceHash, child.Tag, newParticipantHash)
			cache.DeleteLocked(from)
		}
		if fromLID != nil && changedDeviceLID != nil && deviceLIDHash != "" {
			newLIDParticipantHash := participantListHashV2(cachedLID.Devices)
			if newLIDParticipantHash == deviceLIDHash {
				cli.Log.Debugf("%s's device list hash changed from %s to %s (%s). New hash matches", fromLID, cachedLIDHash, deviceLIDHash, child.Tag)
				cache.SetLocked(*fromLID, cachedLID)
			} else {
				cli.Log.Warnf("%s's device list hash changed from %s to %s (%s). New hash doesn't match (%s)", fromLID, cachedLIDHash, deviceLIDHash, child.Tag, newLIDParticipantHash)
				cache.DeleteLocked(*fromLID)
			}
		}
	}
}

func (cli *Client) handleFBDeviceNotification(ctx context.Context, node *waBinary.Node) {
	cache := &cli.userDevicesCache
	cache.Lock()
	defer cache.Unlock()
	jid := node.AttrGetter().JID("from")
	userDevices := parseFBDeviceList(jid, node.GetChildByTag("devices"))
	cache.SetLocked(jid, userDevices)
}

func (cli *Client) handleOwnDevicesNotification(ctx context.Context, node *waBinary.Node, fromJID types.JID) {
	cache := &cli.userDevicesCache
	cache.Lock()
	defer cache.Unlock()
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
	if cached, ok := cache.GetLocked(fromJIDPlain); ok {
		oldHash = participantListHashV2(cached.Devices)
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
		cache.DeleteLocked(ownID)
		cache.DeleteLocked(ownLID)
	} else {
		cli.Log.Debugf("Received own device list change notification %s -> %s from %s", oldHash, newHash, fromJID)
		cache.SetLocked(fromJIDPlain, deviceCache{Devices: newDeviceList, DHash: expectedNewHash})
		cache.SetLocked(altJID, deviceCache{Devices: altDeviceList, DHash: participantListHashV2(altDeviceList)})
	}
}
