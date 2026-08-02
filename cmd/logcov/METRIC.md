# A metrica de cobertura de log — definicao normativa

Este arquivo e' a **autoridade**. `cmd/logcov` e' a implementacao dele.
Divergencia entre este documento e a ferramenta e' **bug da ferramenta**.

Versionado junto com o codigo de proposito: qualquer mudanca de regra aparece
como diff revisavel no mesmo PR que muda o numero.

---

## 1. Escopo e unidade

**Escopo.** Todo `*.go` sob `pkg/**` que nao termine em `_test.go` e nao
contenha a marca canonica `// Code generated ... DO NOT EDIT.`.

**Unidade.** Cada `*ast.FuncDecl` com corpo nao-nil (inclui metodos), mais os
`*ast.FuncLit` qualificados por X6. Os `FuncLit` **nao** qualificados tambem
aparecem no golden, marcados `EXCLUDED X6` — a exclusao e' auditavel, nao
implicita.

**Resolucao por tipo, nunca por nome.** A carga usa
`golang.org/x/tools/go/packages` com
`NeedSyntax|NeedTypes|NeedTypesInfo` (mais `NeedImports|NeedDeps` para
alcancar os tipos de referencia). Nenhuma regra decide por texto de
identificador: o nome de um metodo so' entra na decisao **depois** que o tipo
do receptor foi resolvido por `go/types`. Se a porta
`wa-api/pkg/application/contracts.Logger` nao for resolvida, a ferramenta
**aborta** em vez de reportar zero — reportar zero seria indistinguivel de um
repositorio sem log.

**Corpo proprio.** O corpo de um `FuncLit` promovido a entrada propria **sai**
do corpo da unidade que o contem, em qualquer nivel de aninhamento. Sem esse
recorte, uma cadeia de closures contaria o mesmo call site duas vezes.

---

## 2. Denominador — elegibilidade (X1..X8)

Toda funcao da unidade, **menos** o conjunto de isencao.
X1..X7 sao mecanicamente decidiveis e nao dependem de anotacao; X8 e' a
valvula orcada.

| # | Isencao | Justificativa |
|---|---|---|
| **X1** | Corpo com <=2 statements **e** nenhum caminho de saida no sentido da secao 4 | Getters, setters, construtores triviais |
| **X2** | Corpo e' um unico `ReturnStmt` cujos operandos sao apenas identificadores, literais ou selectors sobre o receiver | Acessor puro |
| **X3** | `func init()` | Executa antes da configuracao do logger |
| **X4** | `String() string`, `Error() string`, `MarshalJSON() ([]byte, error)`, `UnmarshalJSON([]byte) error`, `Format(fmt.State, rune)` — verificados por assinatura, nao so' por nome | Logar dentro deles e' **recursao infinita** via zerolog |
| **X5** | Funcoes em pacotes listados em `.logcov-exclude` (lista inicial: `cmd/` apenas) | Os binarios sao `main` triviais; e a ferramenta nao se julga |
| **X6** | `*ast.FuncLit`, **exceto** quando for operando de `GoStmt`; operando de `DeferStmt`; argumento de `(*errgroup.Group).Go`, `sync.OnceFunc`, `sync.OnceValue`; ou de forma `func(http.ResponseWriter, *http.Request)` / `func(http.Handler) http.Handler`. Nesses casos **e' elegivel** como entrada propria (`pkg/foo.Bar.func1`) | Goroutines, defers de cleanup, membros de errgroup e closures de middleware sao precisamente onde erro se perde silenciosamente. Closures de `sort.Slice` nao sao |
| **X7** | Wrapper de delegacao pura: corpo e' exatamente uma chamada e um return, os argumentos da chamada sao exatamente os parametros da funcao (sem transformacao), e o retorno e' exatamente o resultado da chamada | Wrappers que ja logam inflariam numerador e denominador juntos, barateando o custo marginal das funcoes dificeis |
| **X8** | Comentario `//log:exempt <motivo>` imediatamente acima da declaracao | Valvula de escape — **orcada** por `max_exempt_annotations` |

**Precedencia.** Quando mais de uma regra se aplica, vale a primeira desta
ordem: **X5, X6, X3, X4, X8, X1, X2, X7**. A ordem e' declarada aqui porque
ela decide qual motivo aparece no golden, e uma mudanca de precedencia muda o
diff sem mudar o numero.

**Delegacao NAO da' credito.** Creditar uma funcao pelo log de uma funcao que
ela chama esta' rejeitado: (i) o recorte "mesmo arquivo" e' arbitrario;
(ii) torna a metrica nao-local — editar A muda silenciosamente o status de B;
(iii) premia indirection. O caso legitimo (wrappers finos) e' coberto por
X1/X2/X7 **removendo-os do denominador**.

**Sobre `pkg/domain` e `pkg/domain/apperr`.** Nao ha exclusao de pacote para
eles. Saem naturalmente por X1/X2/X4. Se ganharem logica nao-trivial voltam ao
denominador sozinhos, e isso aparece no golden.

---

## 3. Numerador de `func_coverage` — L1 e L3

### L1 — Presenca

O corpo proprio da funcao contem >=1 **chamada de log reconhecida**, em
qualquer das tres formas:

- **L1-a — porta de aplicacao.** Chamada a metodo `Info`/`Warn`/`Error` sobre
  valor cujo tipo **implementa** `wa-api/pkg/application/contracts.Logger`
  (verificado com `types.Implements`, nao pelo nome da variavel), com um
  primeiro argumento que implementa `context.Context`.
- **L1-b — zerolog direto.** Cadeia iniciando em
  `log.Trace|Debug|Info|Warn|Error|Fatal|Panic` (pacote
  `github.com/rs/zerolog/log`, resolvido pelo `types.PkgName` do import) ou em
  expressao de tipo `zerolog.Logger` / `*zerolog.Logger`, terminando em
  `.Msg(...)`, `.Msgf(...)` ou `.Send()`.
- **L1-c — hlog.** Cadeia iniciando em `hlog.FromRequest(...)`, mesmas
  terminacoes de L1-b.

> **Restricao por diretorio.** Nos diretorios listados na regra
> `depguard`/`camada-de-aplicacao` do `.golangci.yml`, **L1-b nao e' forma
> valida** — so' L1-a conta. Aceitar zerolog ali seria premiar o que o lint
> reprova. A lista e' lida do proprio `.golangci.yml` a cada execucao, para que
> as duas nao possam divergir em silencio.

### L3 — Estruturacao, POR CALL SITE

**Toda** chamada de log reconhecida na funcao deve carregar estrutura:

- **L1-a:** `len(keyvals) >= 2` **e par** — `keyvals` impar e' violacao (pega
  tambem o bug real de par quebrado);
- **L1-b / L1-c:** a cadeia encadeia >=1 metodo de campo antes do `.Msg`:
  `Str|Strs|Int|Int64|Uint|Float64|Bool|Dur|Time|Err|AnErr|Interface|Any|Bytes|Stringer|RawJSON|Fields|Ctx`.

Uma unica violacao em qualquer call site reprova a funcao inteira. L3 e' por
call site, e nao por funcao, senao um `.Str()` na entrada mais dez `.Msg()`
pelados nos caminhos de erro passaria.

```
func_coverage = |elegiveis que satisfazem L1 e L3| / |elegiveis|
```

---

## 4. Numerador de `errpath_coverage` — L2, sobre caminhos de saida

**Caminho de saida** e' qualquer um de:

- **S-ret** — `*ast.ReturnStmt` que retorna, em qualquer posicao, valor de tipo
  `error` que nao seja o literal `nil`. `return f(...)` com resultado multiplo
  que inclua `error` conta: a tupla carrega o erro do callee.
- **S-http** — escrita de resposta com status >=400: `WriteHeader` sobre um
  `http.ResponseWriter` com argumento **constante** >=400 (a avaliacao
  constante de `go/types` resolve tanto `400` quanto `http.StatusBadRequest`),
  `http.Error`, ou qualquer helper de `pkg/presentation/http` cuja assinatura
  receba um `http.ResponseWriter` e um `int` de status, e cujo argumento nesse
  call site seja >=400.
- **S-consume** — bloco de `if err != nil` (ou `switch` cuja tag e' de tipo
  `error`) que **nao** propaga `err` inalterado: o bloco termina sem retornar
  `err`, ou retorna erro construido sem encadear a causa. O caminho e'
  ancorado no **corpo do `if`**, que e' onde o log da forma (a) tem de estar.

**Um caminho de saida e' coberto quando:**

- **(a)** o menor `*ast.BlockStmt` que o contem tem uma chamada de log
  reconhecida de nivel **>= Warn** (`Warn`, `Error`, `Fatal`, `Panic`); **ou**
- **(b)** *(restrita)* o caminho e' **S-ret** e a expressao retornada **propaga
  a causa sem descarte**: o identificador do erro recebido inalterado
  (`return err`), `fmt.Errorf("...: %w", err)`, `errors.Join(...)`, um
  construtor de `pkg/domain/apperr` **recebendo um error nao-nil**, ou
  `return f(...)` devolvendo verbatim a tupla do callee.
  **Construcao de erro novo que descarta a causa nunca satisfaz (b)**, mesmo
  usando `apperr.New` sem passar `err`.

```
errpath_coverage = |caminhos de saida cobertos| / |caminhos de saida|
```

Denominador restrito a caminhos dentro de funcoes **elegiveis**.

> **Por que S-http e S-consume existem.** `pkg/presentation/http/handlers` tem
> 90 funcoes `func(w http.ResponseWriter, r *http.Request)`, sem retorno de
> erro. Sob a definicao original (so' S-ret) o denominador nesse pacote seria
> ~0 e ele **pontuaria 100% vacuamente** — justo no maior gap de log que
> existe. S-http torna cada 4xx/5xx um caminho; S-consume pega o
> `if err != nil` que loga e segue.
>
> **Por que (b) foi restrita.** Sem a restricao, os 77 sites de
> `fmt.Errorf("no session")` virariam "cobertos" so' por trocar o tipo do erro,
> **sem emitir um unico log novo**. Com a restricao, os 55 do shape (a) ja
> satisfazem (b) hoje, e os 22 do shape (b) continuam descobertos ate' ganharem
> log de verdade.

---

## 5. Golden file de elegibilidade

`cmd/logcov/testdata/eligible.golden` — versionado, uma linha por funcao do
universo, ordenado deterministicamente por chave:

```
<pacote>.<Receiver>.<Func>\tELIGIBLE|EXCLUDED\t<detalhe>
```

Exemplo:

```
pkg/application/usecase/chat.ArchiveChatUseCase.Execute	ELIGIBLE	uncovered:L2(S-ret,archive_chat.go:26)
pkg/domain.Message.String	EXCLUDED	X4
pkg/infra/db.repo.get	EXCLUDED	X7
```

`go run ./cmd/logcov -golden | diff - cmd/logcov/testdata/eligible.golden`
deve sair vazio. **Qualquer mudanca nas regras X1..X8 ou L1..L3 aparece como
diff revisavel no PR, com o motivo da exclusao nomeado por regra.** E' isto
que torna o denominador auditavel — uma banda de tolerancia sobre a contagem
de elegiveis nao reprovaria quase nada.

---

## 6. As quatro travas de `.log-coverage-baseline`

```
stage=advisory | ratchet | floor
min_func_coverage      # decimos de %, ratchet-up
min_errpath_coverage   # decimos de %, ratchet-up
min_eligible           # numero EXATO de elegiveis; falha se CAIR
max_exempt_annotations # contagem de //log:exempt, ratchet-DOWN
```

As duas ultimas atacam a mesma superficie por lados opostos e nenhuma
substitui a outra: `max_exempt_annotations` impede subir o numero **anotando**
funcoes para fora do numerador; `min_eligible` impede subir o numero
**encolhendo o denominador** (adicionar um pacote a `.logcov-exclude`, ou
afrouxar X1..X7).

**O limite honesto destas travas.** "Fail-closed" aqui e' leitura de um arquivo
que o mesmo PR pode editar. Um ratchet in-repo e' um **dispositivo de
visibilidade**, nao uma trava criptografica: converte regressao silenciosa em
linha de diff que alguem tem de aprovar conscientemente.

---

## 7. Determinismo

Duas execucoes sobre a mesma arvore produzem bytes identicos. As entradas sao
ordenadas por chave e, em empate, por linha; as tabelas por pacote sao
ordenadas por nome. Nenhuma saida depende da ordem de iteracao de mapa.
