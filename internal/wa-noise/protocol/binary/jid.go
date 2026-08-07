package binary

import (
	"fmt"

	"wa-api/internal/wa-noise/protocol/binary/token"
	"wa-api/internal/wa-noise/protocol/types"
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

// jidString extrai a string de um valor devolvido por read(). read() devolve
// interface{} e pode legitimamente trazer nil (ListEmpty), types.JID ou
// []Node — as assercoes diretas `v.(string)` viravam panic com bytes vindos do
// socket (F24 em HOUSEKEEP.md). O nome do campo entra na mensagem para que o
// erro diga QUAL posicao do JID veio com o tipo errado.
func jidString(v interface{}, field string) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%w: %s: expected string, got %T", ErrInvalidJIDType, field, v)
	}
	return s, nil
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
	}
	serverStr, err := jidString(server, "server")
	if err != nil {
		return nil, err
	}
	if user == nil {
		return types.NewJID("", serverStr), nil
	}
	userStr, err := jidString(user, "user")
	if err != nil {
		return nil, err
	}
	return types.NewJID(userStr, serverStr), nil
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
	userStr, err := jidString(user, "user")
	if err != nil {
		return nil, err
	}
	return types.JID{
		User:       userStr,
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
	userStr, err := jidString(user, "user")
	if err != nil {
		return nil, err
	}
	serverStr, err := jidString(server, "server")
	if err != nil {
		return nil, err
	}
	return types.JID{
		User:   userStr,
		Device: uint16(device),
		Server: serverStr,
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
	userStr, err := jidString(user, "user")
	if err != nil {
		return nil, err
	}
	return types.NewADJID(userStr, agent, device), nil
}
