package binary

import (
	"fmt"

	"wa-api/internal/wa-noise/binary/token"
	"wa-api/internal/wa-noise/types"
)

// Representacao de fio dos quatro formatos de JID que o XML binario conhece,
// nas duas direcoes. Pelo mesmo racional de packing.go: qual formato o encoder
// escolhe (writeJID) e como o decoder o le' (readJIDPair/readADJID/readFBJID/
// readInteropJID) sao a mesma decisao vista de dois lados, e um formato novo
// tem que entrar aqui nas duas metades ou o round trip quebra.

func (w *binaryEncoder) writeJID(jid types.JID) {
	if ((jid.Server == types.DefaultUserServer || jid.Server == types.HiddenUserServer) && jid.Device > 0) ||
		jid.Server == types.HostedServer || jid.Server == types.HostedLIDServer {
		w.pushByte(token.ADJID)
		w.pushByte(jid.ActualAgent())
		w.pushByte(uint8(jid.Device))
		w.writeString(jid.User)
	} else if jid.Server == types.MessengerServer {
		w.pushByte(token.FBJID)
		w.write(jid.User)
		w.pushInt16(int(jid.Device))
		w.write(jid.Server)
	} else if jid.Server == types.InteropServer {
		w.pushByte(token.InteropJID)
		w.write(jid.User)
		w.pushInt16(int(jid.Device))
		w.pushInt16(int(jid.Integrator))
		w.write(jid.Server)
	} else {
		w.pushByte(token.JIDPair)
		if len(jid.User) == 0 {
			w.pushByte(token.ListEmpty)
		} else {
			w.write(jid.User)
		}
		w.write(jid.Server)
	}
}

func (r *binaryDecoder) readJIDPair() (interface{}, error) {
	user, err := r.read(true)
	if err != nil {
		return nil, err
	}
	server, err := r.read(true)
	if err != nil {
		return nil, err
	} else if server == nil {
		return nil, ErrInvalidJIDType
	} else if user == nil {
		return types.NewJID("", server.(string)), nil
	}
	return types.NewJID(user.(string), server.(string)), nil
}

func (r *binaryDecoder) readInteropJID() (interface{}, error) {
	user, err := r.read(true)
	if err != nil {
		return nil, err
	}
	device, err := r.readInt16(false)
	if err != nil {
		return nil, err
	}
	integrator, err := r.readInt16(false)
	if err != nil {
		return nil, err
	}
	server, err := r.read(true)
	if err != nil {
		return nil, err
	} else if server != types.InteropServer {
		return nil, fmt.Errorf("%w: expected %q, got %q", ErrInvalidJIDType, types.InteropServer, server)
	}
	return types.JID{
		User:       user.(string),
		Device:     uint16(device),
		Integrator: uint16(integrator),
		Server:     types.InteropServer,
	}, nil
}

func (r *binaryDecoder) readFBJID() (interface{}, error) {
	user, err := r.read(true)
	if err != nil {
		return nil, err
	}
	device, err := r.readInt16(false)
	if err != nil {
		return nil, err
	}
	server, err := r.read(true)
	if err != nil {
		return nil, err
	} else if server != types.MessengerServer {
		return nil, fmt.Errorf("%w: expected %q, got %q", ErrInvalidJIDType, types.MessengerServer, server)
	}
	return types.JID{
		User:   user.(string),
		Device: uint16(device),
		Server: server.(string),
	}, nil
}

func (r *binaryDecoder) readADJID() (interface{}, error) {
	agent, err := r.readByte()
	if err != nil {
		return nil, err
	}
	device, err := r.readByte()
	if err != nil {
		return nil, err
	}
	user, err := r.read(true)
	if err != nil {
		return nil, err
	}
	return types.NewADJID(user.(string), agent, device), nil
}
