// Package x6 e' fixture da regra X6: FuncLit e' excluido, exceto quando for
// operando de go/defer, membro de errgroup/sync.Once* ou de forma http.Handler.
package x6

import (
	"net/http"
	"sort"
)

// X6 negativo (o literal E' elegivel): operando de GoStmt.
func GoRoutine(items []int) {
	go func() {
		n := len(items)
		m := cap(items)
		_ = n + m
	}()
}

// X6 positivo (o literal e' EXCLUIDO): closure de sort.Slice.
func SortSlice(items []int) {
	sort.Slice(items, func(i, j int) bool {
		return items[i] < items[j]
	})
}

// Operando de DeferStmt — elegivel.
func Deferred(items []int) {
	defer func() {
		n := len(items)
		m := cap(items)
		_ = n + m
	}()
	_ = items
}

// Forma http.Handler — elegivel.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Fixture", "1")
		_ = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
}

// Forma func(http.Handler) http.Handler — elegivel.
func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		name := "fixture"
		_ = name
		_ = next
		return next
	}
}
