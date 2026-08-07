// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package media

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	waBinary "wa-api/internal/wa-noise/binary"
	waLog "wa-api/internal/wa-noise/util/log"
)

func TestConnExpiry(t *testing.T) {
	fetchedAt := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	mc := &Conn{FetchedAt: fetchedAt, TTL: 3600}
	want := fetchedAt.Add(time.Hour)
	if got := mc.Expiry(); !got.Equal(want) {
		t.Fatalf("Expiry() = %s, esperado %s", got, want)
	}

	// TTL zero significa "ja' expirado", nao "nunca expira".
	zero := &Conn{FetchedAt: fetchedAt}
	if got := zero.Expiry(); !got.Equal(fetchedAt) {
		t.Fatalf("Expiry() com TTL 0 = %s, esperado o proprio FetchedAt", got)
	}
}

// conn_conn monta a resposta de IQ que o servidor devolveria.
func mediaConnNode(hosts ...waBinary.Node) *waBinary.Node {
	children := []waBinary.Node{{
		Tag: "media_conn",
		Attrs: waBinary.Attrs{
			"auth":        "tok",
			"ttl":         "3600",
			"auth_ttl":    "7200",
			"max_buckets": "12",
		},
		Content: hosts,
	}}
	return &waBinary.Node{Tag: "iq", Content: children}
}

func TestParseConnNode(t *testing.T) {
	node := mediaConnNode(
		waBinary.Node{Tag: "host", Attrs: waBinary.Attrs{"hostname": "a.example"}},
		waBinary.Node{Tag: "outro", Attrs: waBinary.Attrs{}},
		waBinary.Node{Tag: "host", Attrs: waBinary.Attrs{"hostname": "b.example"}},
	)
	mc, err := ParseConnNode(waLog.Noop, node)
	if err != nil {
		t.Fatalf("ParseConnNode devolveu erro: %v", err)
	}
	if mc.Auth != "tok" || mc.TTL != 3600 || mc.AuthTTL != 7200 || mc.MaxBuckets != 12 {
		t.Fatalf("atributos mal lidos: %+v", mc)
	}
	if len(mc.Hosts) != 2 || mc.Hosts[0].Hostname != "a.example" || mc.Hosts[1].Hostname != "b.example" {
		t.Fatalf("hosts = %+v, esperado a.example e b.example (filho desconhecido ignorado)", mc.Hosts)
	}
	if mc.FetchedAt.IsZero() {
		t.Error("FetchedAt nao foi preenchido")
	}
}

func TestParseConnNodeRejeitaRespostaInesperada(t *testing.T) {
	casos := map[string]*waBinary.Node{
		"sem filhos":          {Tag: "iq"},
		"filho com outra tag": {Tag: "iq", Content: []waBinary.Node{{Tag: "erro"}}},
	}
	for name, node := range casos {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseConnNode(waLog.Noop, node); err == nil ||
				!strings.Contains(err.Error(), "unexpected child tag") {
				t.Fatalf("erro = %v, esperado recusa por tag inesperada", err)
			}
		})
	}
}

func TestParseConnNodeRejeitaAtributosFaltando(t *testing.T) {
	node := &waBinary.Node{Tag: "iq", Content: []waBinary.Node{{
		Tag:   "media_conn",
		Attrs: waBinary.Attrs{},
	}}}
	if _, err := ParseConnNode(waLog.Noop, node); err == nil ||
		!strings.Contains(err.Error(), "failed to parse media connections") {
		t.Fatalf("erro = %v, esperado recusa por atributos ausentes", err)
	}
}

func TestParseConnNodeRejeitaHostSemHostname(t *testing.T) {
	node := mediaConnNode(waBinary.Node{Tag: "host", Attrs: waBinary.Attrs{}})
	if _, err := ParseConnNode(waLog.Noop, node); err == nil ||
		!strings.Contains(err.Error(), "media connection host") {
		t.Fatalf("erro = %v, esperado recusa por host sem hostname", err)
	}
}

func TestConnCacheSetGet(t *testing.T) {
	var cc ConnCache
	if got := cc.Get(); got != nil {
		t.Fatalf("Get() no zero value = %v, esperado nil", got)
	}
	mc := &Conn{Auth: "x"}
	cc.Set(mc)
	if got := cc.Get(); got != mc {
		t.Fatalf("Get() = %v, esperado o valor guardado", got)
	}
}

func TestRefreshReusaOCacheValido(t *testing.T) {
	cached := &Conn{
		Auth:      "cacheado",
		TTL:       3600,
		FetchedAt: time.Now(),
		Hosts:     []ConnHost{{Hostname: "a.example"}},
	}
	// Sem IQ configurado: se Refresh consultasse o servidor, daria erro.
	tr := &fakeTransport{log: waLog.Noop}
	tr.conn.Set(cached)

	got, err := RefreshConn(context.Background(), tr, false)
	if err != nil {
		t.Fatalf("RefreshConn devolveu erro: %v", err)
	}
	if got != cached {
		t.Fatal("RefreshConn nao devolveu a mediaConn cacheada")
	}
}

func TestRefreshConsultaQuandoExpiradoOuForcado(t *testing.T) {
	var chamadas atomic.Int32
	tr := &fakeTransport{
		log: waLog.Noop,
		iq: func(context.Context) (*waBinary.Node, error) {
			chamadas.Add(1)
			return mediaConnNode(waBinary.Node{Tag: "host", Attrs: waBinary.Attrs{"hostname": "novo.example"}}), nil
		},
	}

	t.Run("cache vazio", func(t *testing.T) {
		mc, err := RefreshConn(context.Background(), tr, false)
		if err != nil {
			t.Fatalf("erro: %v", err)
		}
		if len(mc.Hosts) != 1 || mc.Hosts[0].Hostname != "novo.example" {
			t.Fatalf("hosts = %+v", mc.Hosts)
		}
	})
	t.Run("force renova mesmo com cache valido", func(t *testing.T) {
		antes := chamadas.Load()
		if _, err := RefreshConn(context.Background(), tr, true); err != nil {
			t.Fatalf("erro: %v", err)
		}
		if chamadas.Load() != antes+1 {
			t.Fatal("force nao provocou nova consulta")
		}
	})
	t.Run("cache expirado renova", func(t *testing.T) {
		tr.conn.Set(&Conn{TTL: 1, FetchedAt: time.Now().Add(-time.Hour)})
		antes := chamadas.Load()
		if _, err := RefreshConn(context.Background(), tr, false); err != nil {
			t.Fatalf("erro: %v", err)
		}
		if chamadas.Load() != antes+1 {
			t.Fatal("cache expirado nao provocou nova consulta")
		}
	})
}

// Fidelidade ao codigo de origem: quando a consulta falha, o cache e' zerado
// (o codigo original atribuia o resultado antes de checar o erro).
func TestRefreshZeraOCacheEmFalha(t *testing.T) {
	errBoom := errors.New("boom")
	tr := &fakeTransport{
		log: waLog.Noop,
		iq:  func(context.Context) (*waBinary.Node, error) { return nil, errBoom },
	}
	tr.conn.Set(&Conn{Auth: "antigo", TTL: 3600, FetchedAt: time.Now()})

	if _, err := RefreshConn(context.Background(), tr, true); !errors.Is(err, errBoom) {
		t.Fatalf("erro = %v, esperado o erro do IQ", err)
	}
	if got := tr.conn.Get(); got != nil {
		t.Fatalf("cache = %+v, esperado nil apos falha (comportamento preservado)", got)
	}
}

func TestQueryConnEmbrulhaOErroDoIQ(t *testing.T) {
	errBoom := errors.New("boom")
	tr := &fakeTransport{
		log: waLog.Noop,
		iq:  func(context.Context) (*waBinary.Node, error) { return nil, errBoom },
	}
	_, err := QueryConn(context.Background(), tr)
	if !errors.Is(err, errBoom) || !strings.Contains(err.Error(), "failed to query media connections") {
		t.Fatalf("erro = %v, esperado embrulho do erro do IQ", err)
	}
}

// O lock do cache precisa serializar renovacoes concorrentes: com o cache
// vazio, N goroutines chamando Refresh ao mesmo tempo produzem exatamente uma
// consulta ao servidor (as demais encontram o cache ja' preenchido).
func TestRefreshSerializaConcorrencia(t *testing.T) {
	var chamadas atomic.Int32
	tr := &fakeTransport{
		log: waLog.Noop,
		iq: func(context.Context) (*waBinary.Node, error) {
			chamadas.Add(1)
			time.Sleep(10 * time.Millisecond)
			return mediaConnNode(waBinary.Node{Tag: "host", Attrs: waBinary.Attrs{"hostname": "h.example"}}), nil
		},
	}

	const n = 8
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if _, err := RefreshConn(context.Background(), tr, false); err != nil {
				t.Errorf("RefreshConn devolveu erro: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := chamadas.Load(); got != 1 {
		t.Fatalf("houve %d consultas, esperado 1 (lock do cache)", got)
	}
}
