// Package client adapts a headless session to the application's client port,
// mirroring pkg/infra/wa-noise/client.
//
// Its job is translation, in both directions: application intent into stack
// calls, and stack failures into apperr categories so they reach the HTTP
// boundary inside the envelope. F83 in the root HOUSEKEEP.md is the precedent —
// profile errors escaped the envelope precisely because an adapter passed a
// raw error through.
//
// Note for whoever writes the error mapping: F95 records that apperr has no
// category for conflict (409), and a browser stack produces conflicts the
// socket stack does not — asking to drive a session that another owner holds is
// the obvious one. Check whether F95 is still open before inventing a mapping.
package client
