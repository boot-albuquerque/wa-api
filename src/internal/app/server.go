// Package app provides shared server types and utilities extracted
// from the root package main during the Clean Architecture migration.
package app

import (
	"sync"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
)

// Version is the application version.
const Version = "1.0.6"

// ServerMode represents the server operating mode.
type ServerMode int

const (
	Http ServerMode = iota
	Stdio
)

// Server holds the core server dependencies previously spread across
// global variables in package main.
type Server struct {
	DB     *sqlx.DB
	Router *mux.Router
	ExPath string
	Mode   ServerMode
}

// KillChannel holds the map and mutex for per-session kill channels.
type KillChannel struct {
	mu     sync.Mutex
	chans  map[string](chan bool)
}

// NewKillChannel returns an initialised KillChannel.
func NewKillChannel() *KillChannel {
	return &KillChannel{
		chans: make(map[string](chan bool)),
	}
}

// Set registers ch for userID.
func (k *KillChannel) Set(userID string, ch chan bool) {
	k.mu.Lock()
	k.chans[userID] = ch
	k.mu.Unlock()
}

// Get returns the channel for userID and whether it exists.
func (k *KillChannel) Get(userID string) (chan bool, bool) {
	k.mu.Lock()
	ch, ok := k.chans[userID]
	k.mu.Unlock()
	return ch, ok
}

// Delete removes userID's entry, but only if it still maps to ch.
func (k *KillChannel) Delete(userID string, ch chan bool) {
	k.mu.Lock()
	if current, ok := k.chans[userID]; ok && current == ch {
		delete(k.chans, userID)
	}
	k.mu.Unlock()
}

// Signal delivers a non-blocking kill signal to userID's session
// goroutine, if one is registered.
func (k *KillChannel) Signal(userID string) {
	ch, ok := k.Get(userID)
	if !ok {
		return
	}
	select {
	case ch <- true:
	default:
	}
}
