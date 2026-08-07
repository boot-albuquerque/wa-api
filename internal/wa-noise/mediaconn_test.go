// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMediaConnExpiry(t *testing.T) {
	fetchedAt := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	mc := &MediaConn{FetchedAt: fetchedAt, TTL: 3600}
	want := fetchedAt.Add(time.Hour)
	if got := mc.Expiry(); !got.Equal(want) {
		t.Fatalf("Expiry() = %s, esperado %s", got, want)
	}

	// TTL zero significa "ja' expirado", nao "nunca expira".
	zero := &MediaConn{FetchedAt: fetchedAt}
	if got := zero.Expiry(); !got.Equal(fetchedAt) {
		t.Fatalf("Expiry() com TTL 0 = %s, esperado o proprio FetchedAt", got)
	}
}

func TestRefreshMediaConnReusaOCacheValido(t *testing.T) {
	cached := &MediaConn{
		Auth:      "cacheado",
		TTL:       3600,
		FetchedAt: time.Now(),
		Hosts:     []MediaConnHost{{Hostname: "a.example"}},
	}
	// Sem socket: se refreshMediaConn tentasse consultar o servidor, o teste
	// falharia em vez de devolver o cache.
	cli := &Client{mediaConnCache: cached}

	got, err := cli.refreshMediaConn(context.Background(), false)
	if err != nil {
		t.Fatalf("refreshMediaConn devolveu erro: %v", err)
	}
	if got != cached {
		t.Fatal("refreshMediaConn nao devolveu a mediaConn cacheada")
	}
}

func TestRefreshMediaConnRecusaClientNil(t *testing.T) {
	var cli *Client
	if _, err := cli.refreshMediaConn(context.Background(), false); !errors.Is(err, ErrClientIsNil) {
		t.Fatalf("erro = %v, esperado ErrClientIsNil", err)
	}
}
