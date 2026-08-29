package user

import (
	"context"
	"fmt"
	"slices"

	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
)

// GetDevices gets the list of devices that the given user has. The input should be a list of
// regular JIDs, and the output will be a list of AD JIDs.
//
// Todos os dispositivos que o servidor devolver entram na saida, INCLUSIVE o
// dispositivo local quando o JID do proprio usuario esta' na entrada. O godoc
// original prometia o contrario ("The local device will not be included in the
// output"), mas nao ha' filtragem nenhuma no corpo — parseDeviceList devolve
// tudo e esta funcao so' concatena (F38 em HOUSEKEEP.md).
//
// A correcao foi no comentario, nao no codigo: os chamadores do caminho de
// envio ja' contam com a lista completa, e reintroduzir o filtro aqui mudaria
// para quem a mensagem e' cifrada.
//
// Segura o lock do cache do inicio ao fim, inclusive atraves da consulta usync
// ao servidor — exatamente como o GetUserDevices original fazia. Ver o doc de
// DeviceCache.
func GetDevices(ctx context.Context, t Transport, jids []types.JID) ([]types.JID, error) {
	cache := t.DeviceCache()
	cache.Lock()
	defer cache.Unlock()

	var devices, jidsToSync, fbJIDsToSync []types.JID
	for _, jid := range jids {
		cached, ok := cache.GetLocked(jid)
		if ok && len(cached.Devices) > 0 {
			devices = append(devices, cached.Devices...)
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
		list, err := USync(ctx, t, jidsToSync, ModeQuery, ContextMessage, []waBinary.Node{
			{Tag: devicesNodeTag, Attrs: waBinary.Attrs{"version": deviceListVersion}},
		})
		if err != nil {
			return nil, err
		}

		for _, u := range list.GetChildren() {
			jid, jidOK := u.Attrs["jid"].(types.JID)
			if u.Tag != usyncUserTag || !jidOK {
				continue
			}
			userDevices := ParseDeviceList(jid, u.GetChildByTag(devicesNodeTag))
			cache.SetLocked(jid, DeviceEntry{
				Devices: userDevices,
				DHash:   t.ParticipantListHash(userDevices),
			})
			devices = append(devices, userDevices...)
		}
	}

	if len(fbJIDsToSync) > 0 {
		userDevices, err := GetFBIDDevices(ctx, t, fbJIDsToSync)
		if err != nil {
			return nil, err
		}
		devices = append(devices, userDevices...)
	}

	return devices, nil
}

// ParseDeviceList le o <devices><device-list> da resposta usync.
func ParseDeviceList(user types.JID, deviceNode waBinary.Node) []types.JID {
	deviceList := deviceNode.GetChildByTag(deviceListNodeTag)
	if deviceNode.Tag != devicesNodeTag || deviceList.Tag != deviceListNodeTag {
		return nil
	}
	children := deviceList.GetChildren()
	devices := make([]types.JID, 0, len(children))
	for _, device := range children {
		deviceID, ok := device.AttrGetter().GetInt64("id", true)
		isHosted := device.AttrGetter().Bool("is_hosted")
		if device.Tag != deviceNodeTag || !ok {
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

// ParseFBDeviceList le a lista de dispositivos do formato `fbid:devices`. Ao
// contrario de ParseDeviceList, nao tem o conceito de `is_hosted`.
func ParseFBDeviceList(user types.JID, deviceList waBinary.Node) DeviceEntry {
	children := deviceList.GetChildren()
	devices := make([]types.JID, 0, len(children))
	for _, device := range children {
		deviceID, ok := device.AttrGetter().GetInt64("id", true)
		if device.Tag != deviceNodeTag || !ok {
			continue
		}
		user.Device = uint16(deviceID)
		devices = append(devices, user)
		// TODO take identities here too?
	}
	// TODO do something with the icdc blob?
	return DeviceEntry{
		Devices: devices,
		DHash:   deviceList.AttrGetter().String("dhash"),
	}
}

// GetFBIDDevicesInternal manda um unico IQ `fbid:devices` e devolve o <users>
// da resposta.
func GetFBIDDevicesInternal(ctx context.Context, t Transport, jids []types.JID) (*waBinary.Node, error) {
	users := make([]waBinary.Node, len(jids))
	for i, jid := range jids {
		users[i].Tag = usyncUserTag
		users[i].Attrs = waBinary.Attrs{"jid": jid}
		// TODO include dhash for users
	}
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: fbidDevicesIQNamespace,
		Type:      IQGet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag:     usersNodeTag,
			Content: users,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send usync query: %w", err)
	} else if list, ok := resp.GetOptionalChildByTag(usersNodeTag); !ok {
		return nil, t.ElementMissing(usersNodeTag, "response to fbid devices query")
	} else {
		return &list, err
	}
}

// GetFBIDDevices consulta as listas de dispositivo de JIDs do Messenger, em
// lotes de fbIDDeviceChunkSize, gravando cada uma no cache.
//
// Escreve no cache SEM tomar o lock: o unico chamador de producao e' GetDevices,
// que ja' o segura. Um Lock() aqui seria deadlock imediato (sync.Mutex nao e'
// reentrante) — era assim antes da extracao e continua sendo.
//
// A fachada da raiz (cli.getFBIDDevices) NAO preserva mais esse contrato: ela
// toma o lock, porque o gerador de internals.go a expoe como
// DangerousInternalClient.GetFBIDDevices e por ali ninguem segurava nada
// (F54 em HOUSEKEEP.md). Ela nao esta' no caminho de producao, entao travar la'
// nao pode deadlockar.
func GetFBIDDevices(ctx context.Context, t Transport, jids []types.JID) ([]types.JID, error) {
	cache := t.DeviceCache()
	var devices []types.JID
	for chunk := range slices.Chunk(jids, fbIDDeviceChunkSize) {
		list, err := GetFBIDDevicesInternal(ctx, t, chunk)
		if err != nil {
			return nil, err
		}
		for _, u := range list.GetChildren() {
			jid, jidOK := u.Attrs["jid"].(types.JID)
			if u.Tag != usyncUserTag || !jidOK {
				continue
			}
			userDevices := ParseFBDeviceList(jid, u.GetChildByTag(devicesNodeTag))
			cache.SetLocked(jid, userDevices)
			devices = append(devices, userDevices.Devices...)
		}
	}
	return devices, nil
}
