package jid

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"wa-api/internal/wa-noise/types"
)

// JIDResolverAdapter implementa appport.JIDResolver sobre ParseJID, a mesma
// função que os use cases chamavam diretamente antes da ADR-001.
type JIDResolverAdapter struct{}

// NewJIDResolverAdapter cria o resolvedor.
func NewJIDResolverAdapter() *JIDResolverAdapter { return &JIDResolverAdapter{} }

// ResolveJID devolve o JID canônico de raw.
func (JIDResolverAdapter) ResolveJID(_ context.Context, raw string) (domain.JID, error) {
	jid, ok := ParseJID(raw)
	if !ok {
		return "", fmt.Errorf("whatsmeow: could not parse JID %q", raw)
	}
	return domain.JID(jid.String()), nil
}

// ResolveQualifiedJID aplica a regra estrita: exige servidor explícito, sem
// aplicar o padrão. É literalmente o helper parseJID que group_request.go
// mantinha duplicado.
func (JIDResolverAdapter) ResolveQualifiedJID(_ context.Context, raw string) (domain.JID, error) {
	jid, err := types.ParseJID(raw)
	if err != nil {
		return "", fmt.Errorf("whatsmeow: could not parse JID %q: %w", raw, err)
	}
	// types.ParseJID não falha para uma string sem "@": ela devolve o texto
	// inteiro em Server e User vazio. Sem esta checagem o resultado volta a
	// ser o mesmo telefone cru, e o adapter que o reparseia com o ParseJID
	// leniente aplicaria o servidor padrão — exatamente o que "qualificado"
	// existe para impedir.
	if jid.User == "" {
		return "", fmt.Errorf("whatsmeow: JID %q has no server", raw)
	}
	return domain.JID(jid.String()), nil
}

// ToJID reconverte um domain.JID para o tipo do SDK. O domain.JID sempre vem
// de ResolveJID, portanto já está canônico; o vazio mapeia para o JID zero,
// que é como o upstream representava "sem remetente" em MarkRead.
func ToJID(j domain.JID) (types.JID, error) {
	if j == "" {
		return types.JID{}, nil
	}
	parsed, ok := ParseJID(string(j))
	if !ok {
		return types.JID{}, fmt.Errorf("whatsmeow: could not parse JID %q", string(j))
	}
	return parsed, nil
}

// Verificação em tempo de compilação de que o adapter implementa a porta.
var _ appport.JIDResolver = (*JIDResolverAdapter)(nil)

// ToJIDs converte uma lista de domain.JID para o tipo do SDK.
func ToJIDs(in []domain.JID) ([]types.JID, error) {
	out := make([]types.JID, len(in))
	for i, j := range in {
		parsed, err := ToJID(j)
		if err != nil {
			return nil, err
		}
		out[i] = parsed
	}
	return out, nil
}
