package appstate

import "errors"

// Errors that this package can return.
var (
	ErrMissingPreviousSetValueOperation = errors.New("missing value MAC of previous SET operation")
	ErrMismatchingLTHash                = errors.New("mismatching LTHash")
	ErrMismatchingPatchMAC              = errors.New("mismatching patch MAC")
	ErrMismatchingContentMAC            = errors.New("mismatching content MAC")
	ErrMismatchingIndexMAC              = errors.New("mismatching index MAC")
	ErrKeyNotFound                      = errors.New("didn't find app state key")
	// ErrShortMutationBlob: o blob da SyncdValue chegou menor que o layout
	// minimo (IV + MAC). Ver mutation_blob.go e F19 em HOUSEKEEP.md.
	ErrShortMutationBlob = errors.New("mutation value blob shorter than the minimum layout")
	// ErrNilMutationValue: MutationInfo.Value chegou nil em EncodePatch. Ver
	// F49 em HOUSEKEEP.md.
	ErrNilMutationValue = errors.New("mutation info has no value")
)
