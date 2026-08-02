// Package contractsfake fornece dublês de teste para toda porta declarada em
// wa-api/pkg/application/contracts (pacote `port`).
//
// # Forma
//
// Cada porta P ganha um struct contractsfake.P que a implementa. Para cada
// método M da porta o struct expõe:
//
//   - MFunc — campo de função com a assinatura de M. Quando nil, o método
//     devolve os valores zero (e nil para error), o que torna o zero-value do
//     fake imediatamente utilizável no caminho feliz.
//   - MCalls — slice das chamadas recebidas, com os argumentos. O tipo do
//     elemento é PMCall (prefixado pelo nome do fake, porque nomes de método
//     se repetem entre portas).
//
// A gravação acontece SEMPRE, inclusive quando MFunc é nil.
//
// # Portas que embutem SessionGuard
//
// São embutidas por valor, o que promove tanto EnsureSessionFunc quanto
// EnsureSessionCalls:
//
//	f := &contractsfake.GroupDirectory{}
//	f.EnsureSessionFunc = func(context.Context, string) error { return errNoSession }
//
// Em literal composto, use a forma aninhada:
//
//	f := &contractsfake.GroupDirectory{
//		SessionGuard: contractsfake.SessionGuard{EnsureSessionFunc: fail},
//	}
//
// # Concorrência
//
// Apenas Logger é seguro para uso concorrente (é o único que na prática é
// compartilhado entre goroutines em teste). Os demais assumem um único
// goroutine por fake, que é como os use cases os exercitam.
//
// Use sempre ponteiros para os fakes: eles gravam estado, e cópias por valor
// perdem as chamadas registradas.
package contractsfake
