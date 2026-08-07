package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"go.mau.fi/util/dbutil"
	"go.mau.fi/util/exslices"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
)

const (
	putContactNameQuery = `
		INSERT INTO whatsmeow_contacts (our_jid, their_jid, first_name, full_name) VALUES ($1, $2, $3, $4)
		ON CONFLICT (our_jid, their_jid) DO UPDATE SET first_name=excluded.first_name, full_name=excluded.full_name
	`
	putRedactedPhoneQuery = `
		INSERT INTO whatsmeow_contacts (our_jid, their_jid, redacted_phone)
		VALUES ($1, $2, $3)
		ON CONFLICT (our_jid, their_jid) DO UPDATE SET redacted_phone=excluded.redacted_phone
	`
	putPushNameQuery = `
		INSERT INTO whatsmeow_contacts (our_jid, their_jid, push_name) VALUES ($1, $2, $3)
		ON CONFLICT (our_jid, their_jid) DO UPDATE SET push_name=excluded.push_name
	`
	putBusinessNameQuery = `
		INSERT INTO whatsmeow_contacts (our_jid, their_jid, business_name) VALUES ($1, $2, $3)
		ON CONFLICT (our_jid, their_jid) DO UPDATE SET business_name=excluded.business_name
	`
	getContactQuery = `
		SELECT first_name, full_name, push_name, business_name, redacted_phone FROM whatsmeow_contacts WHERE our_jid=$1 AND their_jid=$2
	`
	getAllContactsQuery = `
		SELECT their_jid, first_name, full_name, push_name, business_name, redacted_phone FROM whatsmeow_contacts WHERE our_jid=$1
	`
)

var putContactNamesMassInsertBuilder = dbutil.NewMassInsertBuilder[store.ContactEntry, [1]any](
	putContactNameQuery, "($1, $%d, $%d, $%d)",
)

var putRedactedPhonesMassInsertBuilder = dbutil.NewMassInsertBuilder[store.RedactedPhoneEntry, [1]any](
	putRedactedPhoneQuery, "($1, $%d, $%d)",
)

func (s *SQLStore) PutPushName(ctx context.Context, user types.JID, pushName string) (bool, string, error) {
	s.contactCacheLock.Lock()
	defer s.contactCacheLock.Unlock()

	cached, err := s.getContact(ctx, user)
	if err != nil {
		return false, "", err
	}
	if cached.PushName != pushName {
		_, err = s.db.Exec(ctx, putPushNameQuery, s.JID, user, pushName)
		if err != nil {
			return false, "", err
		}
		previousName := cached.PushName
		cached.PushName = pushName
		cached.Found = true
		return true, previousName, nil
	}
	return false, "", nil
}

func (s *SQLStore) PutBusinessName(ctx context.Context, user types.JID, businessName string) (bool, string, error) {
	s.contactCacheLock.Lock()
	defer s.contactCacheLock.Unlock()

	cached, err := s.getContact(ctx, user)
	if err != nil {
		return false, "", err
	}
	if cached.BusinessName != businessName {
		_, err = s.db.Exec(ctx, putBusinessNameQuery, s.JID, user, businessName)
		if err != nil {
			return false, "", err
		}
		previousName := cached.BusinessName
		cached.BusinessName = businessName
		cached.Found = true
		return true, previousName, nil
	}
	return false, "", nil
}

func (s *SQLStore) PutContactName(ctx context.Context, user types.JID, firstName, fullName string) error {
	s.contactCacheLock.Lock()
	defer s.contactCacheLock.Unlock()

	cached, err := s.getContact(ctx, user)
	if err != nil {
		return err
	}
	if cached.FirstName != firstName || cached.FullName != fullName {
		_, err = s.db.Exec(ctx, putContactNameQuery, s.JID, user, firstName, fullName)
		if err != nil {
			return err
		}
		cached.FirstName = firstName
		cached.FullName = fullName
		cached.Found = true
	}
	return nil
}

func (s *SQLStore) PutAllContactNames(ctx context.Context, contacts []store.ContactEntry) error {
	if len(contacts) == 0 {
		return nil
	}
	origLen := len(contacts)
	contacts = exslices.DeduplicateUnsortedOverwriteFunc(contacts, func(t store.ContactEntry) types.JID {
		return t.JID
	})
	if origLen != len(contacts) {
		s.log.Warnf("%d duplicate contacts found in PutAllContactNames", origLen-len(contacts))
	}
	err := s.db.DoTxn(ctx, nil, func(ctx context.Context) error {
		for slice := range slices.Chunk(contacts, contactBatchSize) {
			query, vars := putContactNamesMassInsertBuilder.Build([1]any{s.JID}, slice)
			_, err := s.db.Exec(ctx, query, vars...)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.contactCacheLock.Lock()
	// Just clear the cache, fetching pushnames and business names would be too much effort
	s.contactCache = make(map[types.JID]*types.ContactInfo)
	s.contactCacheLock.Unlock()
	return nil
}

func (s *SQLStore) PutManyRedactedPhones(ctx context.Context, entries []store.RedactedPhoneEntry) error {
	if len(entries) == 0 {
		return nil
	}
	origLen := len(entries)
	entries = exslices.DeduplicateUnsortedOverwriteFunc(entries, func(t store.RedactedPhoneEntry) types.JID {
		return t.JID
	})
	if origLen != len(entries) {
		s.log.Warnf("%d duplicate contacts found in PutManyRedactedPhones", origLen-len(entries))
	}
	err := s.db.DoTxn(ctx, nil, func(ctx context.Context) error {
		for slice := range slices.Chunk(entries, contactBatchSize) {
			query, vars := putRedactedPhonesMassInsertBuilder.Build([1]any{s.JID}, slice)
			_, err := s.db.Exec(ctx, query, vars...)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.contactCacheLock.Lock()
	for _, entry := range entries {
		if cached, ok := s.contactCache[entry.JID]; ok && cached.RedactedPhone == entry.RedactedPhone {
			continue
		}
		delete(s.contactCache, entry.JID)
	}
	s.contactCacheLock.Unlock()
	return nil
}

// getContact assume que contactCacheLock ja' esta' tomado pelo chamador — todos
// os metodos publicos deste arquivo o tomam antes de chamar.
func (s *SQLStore) getContact(ctx context.Context, user types.JID) (*types.ContactInfo, error) {
	cached, ok := s.contactCache[user]
	if ok {
		return cached, nil
	}

	var first, full, push, business, redactedPhone sql.NullString
	err := s.db.QueryRow(ctx, getContactQuery, s.JID, user).Scan(&first, &full, &push, &business, &redactedPhone)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	info := &types.ContactInfo{
		Found:         err == nil,
		FirstName:     first.String,
		FullName:      full.String,
		PushName:      push.String,
		BusinessName:  business.String,
		RedactedPhone: redactedPhone.String,
	}
	s.contactCache[user] = info
	return info, nil
}

func (s *SQLStore) GetContact(ctx context.Context, user types.JID) (types.ContactInfo, error) {
	s.contactCacheLock.Lock()
	info, err := s.getContact(ctx, user)
	s.contactCacheLock.Unlock()
	if err != nil {
		return types.ContactInfo{}, err
	}
	return *info, nil
}

func (s *SQLStore) GetAllContacts(ctx context.Context) (map[types.JID]types.ContactInfo, error) {
	s.contactCacheLock.Lock()
	defer s.contactCacheLock.Unlock()
	rows, err := s.db.Query(ctx, getAllContactsQuery, s.JID)
	if err != nil {
		return nil, err
	}
	output := make(map[types.JID]types.ContactInfo, len(s.contactCache))
	for rows.Next() {
		var jid types.JID
		var first, full, push, business, redactedPhone sql.NullString
		err = rows.Scan(&jid, &first, &full, &push, &business, &redactedPhone)
		if err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}
		info := types.ContactInfo{
			Found:         true,
			FirstName:     first.String,
			FullName:      full.String,
			PushName:      push.String,
			BusinessName:  business.String,
			RedactedPhone: redactedPhone.String,
		}
		output[jid] = info
		s.contactCache[jid] = &info
	}
	return output, nil
}
