package waclienttest

import (
	"context"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
)

// ContactStore devolve um mapa pré-carregado de contatos.
type ContactStore struct {
	Contacts map[types.JID]types.ContactInfo
	ErrOnGet error
}

func (f *ContactStore) PutPushName(ctx context.Context, user types.JID, pushName string) (bool, string, error) {
	return true, "", nil
}
func (f *ContactStore) PutBusinessName(ctx context.Context, user types.JID, businessName string) (bool, string, error) {
	return true, "", nil
}
func (f *ContactStore) PutContactName(ctx context.Context, user types.JID, fullName, firstName string) error {
	return nil
}
func (f *ContactStore) PutAllContactNames(ctx context.Context, Contacts []store.ContactEntry) error {
	return nil
}
func (f *ContactStore) PutManyRedactedPhones(ctx context.Context, entries []store.RedactedPhoneEntry) error {
	return nil
}
func (f *ContactStore) GetContact(ctx context.Context, user types.JID) (types.ContactInfo, error) {
	if c, ok := f.Contacts[user]; ok {
		return c, nil
	}
	return types.ContactInfo{}, nil
}
func (f *ContactStore) GetAllContacts(ctx context.Context) (map[types.JID]types.ContactInfo, error) {
	if f.ErrOnGet != nil {
		return nil, f.ErrOnGet
	}
	return f.Contacts, nil
}
