// Package extra e' fixture das formas menos comuns de cada regra: X4 por
// assinatura, X2 com literais e ponteiros, X6 por errgroup/sync.Once,
// S-consume por switch, e as raizes de cadeia zerolog que nao sao o pacote
// github.com/rs/zerolog/log.
package extra

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"wa-api/pkg/domain/apperr"
)

type Doc struct {
	Raw  []byte
	Name string
	logg zerolog.Logger
	ptr  *zerolog.Logger
}

// X4 por assinatura: MarshalJSON.
func (d *Doc) MarshalJSON() ([]byte, error) {
	out := d.Raw
	out = append(out, '!')
	return out, nil
}

// X4 por assinatura: UnmarshalJSON.
func (d *Doc) UnmarshalJSON(b []byte) error {
	d.Raw = b
	d.Name = string(b)
	return json.Unmarshal(b, &d.Name)
}

// X4 por assinatura: Format.
func (d *Doc) Format(f fmt.State, verb rune) {
	_ = verb
	_, _ = f.Write(d.Raw)
	_ = d.Name
}

// X4 negativo por assinatura: nome certo, assinatura errada.
func (d *Doc) MarshalJSONish() ([]byte, string) {
	out := d.Raw
	out = append(out, '?')
	return out, d.Name
}

// X2 com literal e com ponteiro de receiver.
func (d *Doc) Zero() (int, error) {
	return 0, d.err()
}

func (d *Doc) err() error {
	n := len(d.Raw)
	_ = n
	return errors.New("doc")
}

// X6: argumento de errgroup.Group.Go — elegivel.
func Group(g *errgroup.Group) {
	g.Go(func() error {
		n := 1
		_ = n
		return errors.New("grupo")
	})
}

// X6: sync.OnceFunc — elegivel.
func Once() func() {
	return sync.OnceFunc(func() {
		n := 1
		m := 2
		_ = n + m
	})
}

// X6: sync.OnceValue — elegivel.
func OnceVal() func() int {
	return sync.OnceValue(func() int {
		n := 1
		m := 2
		return n + m
	})
}

// X6: literal passado por `go f(...)` (nao e' operando direto do GoStmt).
func GoIndirect() {
	go run(func() {
		n := 1
		m := 2
		_ = n + m
	})
}

func run(f func()) { f() }

// S-consume por switch sobre err.
func SwitchConsume(err error) string {
	out := "ok"
	switch err {
	case nil:
		out = "nil"
	default:
		out = "erro"
	}
	return out
}

// S-consume por switch que propaga.
func SwitchPropagate(err error) (string, error) {
	out := "ok"
	switch err {
	case nil:
		out = "nil"
	default:
		return "", err
	}
	return out, nil
}

// Raiz de cadeia zerolog por TIPO: zerolog.Logger de campo.
func (d *Doc) LogValue(id string) {
	d.logg.Warn().Str("id", id).Msg("valor")
	n := len(id)
	_ = n
}

// Raiz de cadeia zerolog por TIPO: *zerolog.Logger.
func (d *Doc) LogPointer(id string) {
	d.ptr.Warn().Str("id", id).Msg("ponteiro")
	n := len(id)
	_ = n
}

// Cadeia que nao e' log: mesma forma .Msg sobre um tipo que nao e' zerolog.
type fake struct{}

func (fake) Info() fake        { return fake{} }
func (f fake) Msg(string) fake { return f }

func NotALog(id string) {
	var f fake
	f.Info().Msg(id)
	n := len(id)
	_ = n
}

// (b): errors.Join encadeia a causa.
func JoinPropagates(err error) error {
	n := 1
	_ = n
	if err != nil {
		return errors.Join(err, errors.New("extra"))
	}
	return nil
}

// (b) descarte: chamada de pacote fora da lista, com erro dentro.
func UnknownConstructor(err error) error {
	n := 1
	_ = n
	if err != nil {
		return apperr.New("x", apperr.CategoryInternal, "x", false, nil)
	}
	return nil
}

// S-http: helper sem parametro de status nao e' caminho de saida.
func NoStatusHelper(w http.ResponseWriter) {
	_, _ = w.Write([]byte("x"))
	_ = w.Header()
	w.WriteHeader(200)
}

// S-http: http.Error com status >= 400.
func HTTPError(w http.ResponseWriter, r *http.Request) {
	_ = r
	http.Error(w, "nope", http.StatusTeapot)
	_ = w
}

// Parametro sem nome nenhum — exercita paramNames.
func Anonimo(string) error {
	n := 1
	_ = n
	return errors.New("anonimo")
}

// Parametro descartado — exercita paramNames.
func Unnamed(_ string, id string) error {
	n := len(id)
	_ = n
	return errors.New("sem nome")
}

// Receiver generico — exercita typeExprName.
type Bag[T any] struct{ items []T }

func (b *Bag[T]) Add(v T) error {
	b.items = append(b.items, v)
	n := len(b.items)
	_ = n
	return nil
}

// Lazy e' um literal atribuido a variavel de PACOTE: os blocos de cobertura
// dele ficam fora de qualquer *ast.FuncDecl, e `go tool cover -func` os
// descarta do denominador. E' a unica fonte de divergencia entre o total dele
// e um agregado ingenuo do perfil.
var Lazy = func(flag bool) error {
	if flag {
		return errors.New("preguicoso")
	}
	return nil
}
