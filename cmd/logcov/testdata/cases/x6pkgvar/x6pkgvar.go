// Package x6pkgvar e' fixture de R1: um *ast.FuncLit dentro de uma
// declaracao `var` de nivel de pacote tem de ser visivel a metrica,
// aplicando a MESMA promocao X6 usada para FuncLits dentro de corpos de
// funcao. O caso real que prova o bug e' pkg/bootstrap/lifecycle.go:
// `var webhookTLSSkipVerify = sync.OnceValue(func() bool {...})`.
package x6pkgvar

import "sync"

// Promovido por X6 (sync.OnceValue) mesmo estando em escopo de pacote —
// elegivel.
var Once = sync.OnceValue(func() bool {
	n := 1
	m := 2
	return n+m > 0
})

// Literal comum atribuido a uma var de pacote, sem qualificar nenhuma forma
// de X6 — continua excluido.
var Plain = func() int {
	return 1
}
