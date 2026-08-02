// Package rulesx e' fixture de cmd/logcov: um par positivo/negativo para as
// regras de elegibilidade X1, X2, X3, X4, X7 e X8 (METRIC.md 9.1.2).
package rulesx

import (
	"context"
	"errors"
	"fmt"
)

type Box struct {
	name  string
	err   error
	inner Inner
}

type Inner interface {
	Fetch(ctx context.Context, id string) (string, error)
}

// X1 positivo: 2 statements, nenhum caminho de saida.
func (b *Box) X1Pos(n string) {
	b.name = n
	b.name += "!"
}

// X1 negativo: 3 statements, nenhum caminho de saida.
func (b *Box) X1Neg(n string) {
	b.name = n
	b.name += "!"
	b.name += "?"
}

// X2 positivo: unico ReturnStmt com selector sobre o receiver. Nao cai em X1
// porque devolver error e' caminho de saida (S-ret).
func (b *Box) X2Pos() error {
	return b.err
}

// X2 negativo: unico ReturnStmt, mas o operando e' uma chamada.
func (b *Box) X2Neg() error {
	return fmt.Errorf("box %s: %w", b.name, b.err)
}

// X3 positivo: func init().
func init() {
	_ = fmt.Sprint("carregado")
}

// X3 negativo: mesma forma, outro nome — nao e' init, e tem 3 statements.
func NotInit() {
	x := fmt.Sprint("carregado")
	y := x + "!"
	_ = y
}

// X4 positivo: String() string. Logar aqui e' recursao infinita via zerolog.
func (b *Box) String() string {
	s := b.name
	s += "/"
	return s
}

// X4 negativo: mesma assinatura, nome que nao e' auto-referencial.
func (b *Box) Stringy() string {
	s := b.name
	s += "/"
	return s
}

// X7 positivo: delegacao pura — uma chamada, um return, argumentos iguais aos
// parametros, sem transformacao.
func (b *Box) X7Pos(ctx context.Context, id string) (string, error) {
	return b.inner.Fetch(ctx, id)
}

// X7 negativo: mesma forma, mas o argumento foi transformado.
func (b *Box) X7Neg(ctx context.Context, id string) (string, error) {
	return b.inner.Fetch(ctx, id+"!")
}

// X8 positivo — a valvula orcada.
//
//log:exempt fixture da regra X8
func (b *Box) X8Pos() error {
	if b.err != nil {
		return errors.New("estourou")
	}
	return nil
}

// X8 negativo: corpo identico, sem a anotacao.
func (b *Box) X8Neg() error {
	if b.err != nil {
		return errors.New("estourou")
	}
	return nil
}
