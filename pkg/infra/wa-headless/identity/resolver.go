// Package identity adapts the page's lookup capability to the application's
// IdentityResolver port.
//
// This build files nearly everything under LID — 397 of 399 messages on the
// reference account — so "who is this" is a real question with two possible
// answers for the same person, and the page is the only place that knows.
// H127 proved the resolution with three cases: a real number resolving to
// another identity, an impossible number giving a DEFINITIVE not-on-WhatsApp,
// and a group short-circuiting instead of being reported absent.
package identity

import (
	"context"
	"errors"
	"fmt"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/wa-headless"
	"wa-api/pkg/infra/wa-headless/registry"
)

const (
	checkLabel = "adapter/is-on-whatsapp"
	pairLabel  = "adapter/identity-pair"
)

// lookuper is the slice of the page capability this adapter uses.
type lookuper interface {
	NumberID(ctx context.Context, jid, label string) (waheadless.Identity, error)
	LidAndPhone(ctx context.Context, jid, label string) (waheadless.IdentityPair, error)
}

// Resolver implements appport.IdentityResolver over a headless session.
type Resolver struct {
	sessions  *registry.Registry
	configFor func(txtID string) (waheadless.StartConfig, error)
	runner    *waheadless.Runner
	// newLookuper is overridable in tests. Nil uses the real page capability.
	newLookuper func(ctx context.Context, txtID string) (lookuper, error)
}

// NewResolver builds the adapter.
func NewResolver(sessions *registry.Registry, configFor func(string) (waheadless.StartConfig, error)) *Resolver {
	return &Resolver{sessions: sessions, configFor: configFor, runner: waheadless.NewRunner()}
}

// EnsureSession reports whether this process can serve txtID, without booting.
func (r *Resolver) EnsureSession(_ context.Context, txtID string) error {
	if !r.sessions.Holds(txtID) {
		return fmt.Errorf("%w: %q", registry.ErrUnknownSession, txtID)
	}
	return nil
}

// IsOnWhatsApp reports which of the phones have an account.
//
// A phone that does NOT resolve is a normal answer with IsIn false — not an
// error. Failing the whole batch because one number is unregistered would make
// the caller unable to learn anything about the others.
func (r *Resolver) IsOnWhatsApp(ctx context.Context, txtID string, phones []string) ([]domain.WhatsAppCheck, error) {
	resolver, err := r.lookuper(ctx, txtID)
	if err != nil {
		return nil, err
	}

	out := make([]domain.WhatsAppCheck, 0, len(phones))
	for _, phone := range phones {
		pageJID, convErr := adapter.ToPageJID(domain.JID(phone))
		if convErr != nil {
			out = append(out, domain.WhatsAppCheck{Query: phone, IsIn: false})
			continue
		}
		id, lookupErr := resolver.NumberID(ctx, pageJID, checkLabel)
		check, fatal := classifyCheck(phone, id, lookupErr)
		if fatal != nil {
			return nil, fatal
		}
		out = append(out, check)
	}
	return out, nil
}

// classifyCheck turns one lookup outcome into one answer, and it is a PURE
// function so the rule can be tested without a browser.
//
// The rule it carries: ErrNotOnWhatsApp is a DEFINITIVE answer and becomes
// IsIn:false, while any other error ABORTS. Reporting "not on WhatsApp" for
// somebody we merely failed to ask about would be a fact-shaped guess — the
// caller would act on an absence that was really a timeout.
//
// It lives as a function because the first version had this rule inline, and a
// negative control showed no test could reach it: inverting it compiled and the
// suite stayed green.
func classifyCheck(phone string, id waheadless.Identity, err error) (domain.WhatsAppCheck, error) {
	switch {
	case errors.Is(err, waheadless.ErrNotOnWhatsApp):
		return domain.WhatsAppCheck{Query: phone, IsIn: false}, nil
	case err != nil:
		return domain.WhatsAppCheck{}, err
	default:
		return domain.WhatsAppCheck{Query: phone, IsIn: true, JID: id.JID}, nil
	}
}

// GetLIDForPN resolves the LID of a phone identity.
//
// An unknown mapping is an EMPTY JID with no error, matching the port's
// documented contract: absence of mapping is an answer, not a failure.
func (r *Resolver) GetLIDForPN(ctx context.Context, txtID string, jid domain.JID) (domain.JID, error) {
	pair, err := r.pair(ctx, txtID, jid)
	if err != nil {
		return "", err
	}
	return domain.JID(pair.LID), nil
}

// GetPNForLID is the INVERSE direction, and it exists for the same reason:
// identity is double and the caller does not always know which half it holds.
func (r *Resolver) GetPNForLID(ctx context.Context, txtID string, lid domain.JID) (domain.JID, error) {
	pair, err := r.pair(ctx, txtID, lid)
	if err != nil {
		return "", err
	}
	return domain.JID(pair.PN), nil
}

// GetManyLIDsForPNs resolves in bulk. A phone with no known mapping is ABSENT
// from the map — the caller keeps the original, which is what the port says.
func (r *Resolver) GetManyLIDsForPNs(ctx context.Context, txtID string, jids []domain.JID) (map[domain.JID]domain.JID, error) {
	resolver, err := r.lookuper(ctx, txtID)
	if err != nil {
		return nil, err
	}

	out := make(map[domain.JID]domain.JID, len(jids))
	for _, j := range jids {
		pageJID, convErr := adapter.ToPageJID(j)
		if convErr != nil {
			continue
		}
		pair, pairErr := resolver.LidAndPhone(ctx, pageJID, pairLabel)
		if pairErr != nil {
			// Best-effort by contract: one identity this build cannot resolve
			// must not cost the caller the whole batch.
			continue
		}
		if pair.LID != "" {
			out[j] = domain.JID(pair.LID)
		}
	}
	return out, nil
}

func (r *Resolver) pair(ctx context.Context, txtID string, j domain.JID) (waheadless.IdentityPair, error) {
	pageJID, err := adapter.ToPageJID(j)
	if err != nil {
		return waheadless.IdentityPair{}, err
	}
	resolver, err := r.lookuper(ctx, txtID)
	if err != nil {
		return waheadless.IdentityPair{}, err
	}
	return resolver.LidAndPhone(ctx, pageJID, pairLabel)
}

func (r *Resolver) lookuper(ctx context.Context, txtID string) (lookuper, error) {
	if r.newLookuper != nil {
		return r.newLookuper(ctx, txtID)
	}
	return r.capability(ctx, txtID)
}

func (r *Resolver) capability(ctx context.Context, txtID string) (*waheadless.Resolver, error) {
	cfg, err := r.configFor(txtID)
	if err != nil {
		return nil, fmt.Errorf("waheadless: config for session: %w", err)
	}
	holder, err := r.sessions.Acquire(txtID, cfg, registry.KindOperational)
	if err != nil {
		return nil, err
	}
	sess, err := holder.Session(ctx)
	if err != nil {
		return nil, err
	}
	return waheadless.NewResolver(r.runner, sess.Tab().Evaluate), nil
}

// Compile-time proof that this adapter satisfies the port.
var _ appport.IdentityResolver = (*Resolver)(nil)
