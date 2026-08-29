package user

import (
	"context"
	"sort"
	"strings"

	"wa-api/pkg/infra/noise/client"
	wajid "wa-api/pkg/infra/noise/mapping/jid"
	wasession "wa-api/pkg/infra/noise/runtime/session"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// UserAdapter implementa ContactDirectory, BlocklistManager e PrivacyManager
// sobre o clientManager.
type UserAdapter struct {
	*wasession.SessionGuardAdapter
}

// NewUserAdapter cria o adapter com a função de lookup.
func NewUserAdapter(getClient client.Getter) *UserAdapter {
	return &UserAdapter{SessionGuardAdapter: wasession.NewSessionGuardAdapter(getClient)}
}

// IsOnWhatsApp verifica quais dos telefones informados têm conta.
func (a *UserAdapter) IsOnWhatsApp(ctx context.Context, txtID string, phones []string) ([]domain.WhatsAppCheck, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, apperr.New(codeUserSessionUnavailable, apperr.CategoryValidation,
			"no active session for user", false, err)
	}
	resp, err := client.IsOnWhatsApp(ctx, phones)
	if err != nil {
		return nil, apperr.New("user_is_on_whatsapp_failed", apperr.CategoryInternal,
			"failed to check whether phones are on WhatsApp", true, err)
	}

	var out []domain.WhatsAppCheck
	for _, item := range resp {
		verifiedName := ""
		if item.VerifiedName != nil {
			verifiedName = item.VerifiedName.Details.GetVerifiedName()
		}
		out = append(out, domain.WhatsAppCheck{
			Query:        item.Query,
			IsIn:         item.IsIn,
			JID:          item.JID.String(),
			VerifiedName: verifiedName,
		})
	}
	return out, nil
}

// GetUserInfo devolve os metadados dos JIDs informados, já normalizados.
//
// A normalização acontece AQUI, que é onde o vendor tem de parar. Antes da
// migração de DTO este método devolvia `any` com o map[types.JID]types.UserInfo
// do SDK, e esse mapa ia inteiro para o fio: as chaves da resposta eram nomes
// de campo Go do vendor (`VerifiedName`, `PictureID`) porque o tipo não tem
// etiquetas `json`, e mudar o vendor mudava o contrato público em silêncio.
func (a *UserAdapter) GetUserInfo(ctx context.Context, txtID string, jids []domain.JID) ([]domain.UserInfo, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, apperr.New(codeUserSessionUnavailable, apperr.CategoryValidation,
			"no active session for user", false, err)
	}
	parsed, err := wajid.ToJIDs(jids)
	if err != nil {
		// Este erro so' dispara quando os JIDs vieram malformados NO PEDIDO —
		// e' entrada do chamador, nao falha nossa. Estava como
		// CategoryInternal e Retryable=true, o que e' errado nas duas pontas:
		// Category.HTTPStatus() mapeia Internal para 500, e o doc de
		// AppError.Retryable diz que erro de validacao nunca e' retentavel,
		// porque repetir a mesma entrada da' o mesmo resultado.
		//
		// A troca ainda nao muda nada observavel: nada chama HTTPStatus() hoje
		// (ver apperr/codes.go). Muda no dia em que o boundary for ligado, que
		// e' exatamente quando ninguem lembraria de revisitar isto.
		return nil, apperr.New(codeUserInfoTargetsInvalid, apperr.CategoryValidation,
			"failed to resolve user info targets", false, err)
	}
	info, err := client.GetUserInfo(ctx, parsed)
	if err != nil {
		return nil, err
	}

	// The answer follows the order of the REQUEST, not the map's. Go
	// randomizes map iteration by design, so ranging over `info` would ship a
	// different order on every call for the same question.
	out := make([]domain.UserInfo, 0, len(parsed))
	for i, jid := range parsed {
		entry, ok := info[jid]
		if !ok {
			continue
		}
		verifiedName := ""
		if entry.VerifiedName != nil {
			verifiedName = entry.VerifiedName.Details.GetVerifiedName()
		}
		devices := make([]domain.JID, 0, len(entry.Devices))
		for _, d := range entry.Devices {
			devices = append(devices, domain.JID(d.String()))
		}
		lid := ""
		if !entry.LID.IsEmpty() {
			lid = entry.LID.String()
		}
		out = append(out, domain.UserInfo{
			// jids[i] and parsed[i] are the same target: ToJIDs preserves
			// order. Echoing the CALLER's spelling rather than the parsed one
			// keeps the answer matchable against the question that was asked.
			JID:          jids[i],
			LID:          domain.JID(lid),
			Status:       entry.Status,
			PictureID:    entry.PictureID,
			VerifiedName: verifiedName,
			Devices:      devices,
		})
	}
	return out, nil
}

// GetAllContacts devolve a agenda da sessão e a contagem.
//
// A saída é ORDENADA por JID. O store devolve um mapa, e a iteração de mapa em
// Go é aleatória — sem ordenar, `GET /user/contacts` devolveria a mesma agenda
// numa ordem diferente a cada chamada, e nenhum cliente conseguiria diffar
// duas respostas.
func (a *UserAdapter) GetAllContacts(ctx context.Context, txtID string) ([]domain.Contact, int, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, 0, apperr.New(codeUserSessionUnavailable, apperr.CategoryValidation,
			"no active session for user", false, err)
	}
	contacts, err := client.Store().Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, 0, err
	}

	out := make([]domain.Contact, 0, len(contacts))
	for jid, info := range contacts {
		c := domain.Contact{
			JID:          domain.JID(jid.String()),
			Found:        info.Found,
			FirstName:    info.FirstName,
			FullName:     info.FullName,
			PushName:     info.PushName,
			BusinessName: info.BusinessName,
		}
		// The store keys the address book by ONE identity, and which space it
		// is depends on the contact — @lid for most people on multi-device,
		// @s.whatsapp.net for the rest. The suffix is the only way to tell:
		// PN and LID are the same Go type (F65).
		if strings.HasSuffix(string(c.JID), lidServerSuffix) {
			c.LID = c.JID
		} else {
			c.PN = c.JID
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].JID < out[j].JID })
	return out, len(out), nil
}

// lidServerSuffix distingue a identidade de privacidade da de telefone. É
// string porque PN e LID são o MESMO tipo Go (types.JID), separados só pelo
// Server em tempo de execução — o compilador não ajuda aqui.
const lidServerSuffix = "@lid"

// GetLIDForPN resolve o LID correspondente a um número de telefone.
func (a *UserAdapter) GetLIDForPN(ctx context.Context, txtID string, jid domain.JID) (domain.JID, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return "", err
	}
	parsed, err := wajid.ToJID(jid)
	if err != nil {
		return "", err
	}
	lid, err := client.Store().LIDs.GetLIDForPN(ctx, parsed)
	if err != nil {
		return "", err
	}
	if lid.IsEmpty() {
		return "", nil
	}
	return domain.JID(lid.String()), nil
}

// GetPNForLID resolve o telefone correspondente a um LID.
//
// Espelha GetLIDForPN, inclusive no contrato de ausência: mapeamento
// desconhecido devolve JID vazia e erro nil. O store recusa a chamada com um
// JID que não seja @lid (sqlstore/lidmap.go:125), então passar um PN aqui é
// erro, não silêncio — e é o use case que decide a direção.
func (a *UserAdapter) GetPNForLID(ctx context.Context, txtID string, lid domain.JID) (domain.JID, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return "", err
	}
	parsed, err := wajid.ToJID(lid)
	if err != nil {
		return "", err
	}
	pn, err := client.Store().LIDs.GetPNForLID(ctx, parsed)
	if err != nil {
		return "", err
	}
	if pn.IsEmpty() {
		return "", nil
	}
	return domain.JID(pn.String()), nil
}

// GetManyLIDsForPNs resolve em lote (1 chamada ao store local, não 1 por
// JID) o LID de cada telefone informado.
//
// A ORIENTAÇÃO DO MAPA IMPORTA (F65 em HOUSEKEEP.md).
// CachedLIDMap.GetManyLIDsForPNs devolve map[PN]LID — `result[pn] = lid`
// (sqlstore/lidmap.go:148). A chave do range é o PN; o valor, o LID.
//
// Até a F65 este laço estava escrito como `for lid, pn := range resolved`,
// invertendo o mapa de saída. O dublê de teste do pacote estava invertido do
// mesmo jeito, então os dois combinavam e o teste ficava verde enquanto a
// normalização LID↔PN não acontecia em produção. Medido no ambiente real:
// 419 dos 421 telefones não normalizados TINHAM mapeamento no store.
//
// PN e LID são o mesmo tipo Go (types.JID), distintos só pelo Server em
// tempo de execução — o compilador não pode ajudar aqui.
func (a *UserAdapter) GetManyLIDsForPNs(ctx context.Context, txtID string, jids []domain.JID) (map[domain.JID]domain.JID, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	parsed, err := wajid.ToJIDs(jids)
	if err != nil {
		return nil, err
	}
	resolved, err := client.Store().LIDs.GetManyLIDsForPNs(ctx, parsed)
	if err != nil {
		return nil, err
	}
	out := make(map[domain.JID]domain.JID, len(resolved))
	for pn, lid := range resolved {
		if pn.IsEmpty() || lid.IsEmpty() {
			continue
		}
		out[domain.JID(pn.String())] = domain.JID(lid.String())
	}
	return out, nil
}

// Verificações em tempo de compilação de que o adapter implementa as portas.
var (
	_ appport.ContactDirectory = (*UserAdapter)(nil)
	_ appport.BlocklistManager = (*UserAdapter)(nil)
	_ appport.PrivacyManager   = (*UserAdapter)(nil)
)

// ContactNames devolve o roster tipado por JID.
//
// Mesma leitura de GetAllContacts, com a conversão do tipo do SDK feita AQUI,
// que é onde ela pertence: o adapter existe para que o vendor pare nesta
// fronteira. GetAllContacts continua devolvendo `any` para quem só repassa o
// bloco cru ao cliente.
func (a *UserAdapter) ContactNames(ctx context.Context, txtID string) (map[domain.JID]domain.ContactName, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, apperr.New(codeUserSessionUnavailable, apperr.CategoryValidation,
			"no active session for user", false, err)
	}
	contacts, err := client.Store().Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[domain.JID]domain.ContactName, len(contacts))
	for jid, info := range contacts {
		out[domain.JID(jid.String())] = domain.ContactName{
			FullName:     info.FullName,
			FirstName:    info.FirstName,
			PushName:     info.PushName,
			BusinessName: info.BusinessName,
		}
	}
	return out, nil
}
