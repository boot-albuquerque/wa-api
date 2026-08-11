# HOUSEKEEP — wa-headless (pilha de automação de browser)

Achados incidentais de `internal/wa-headless/`, a pilha que conduz o SPA real
do `web.whatsapp.com` dentro de um browser. É a segunda via de transporte do
projeto, ao lado de `internal/wa-noise/`, que fala o protocolo direto no
WebSocket com o Noise framework.

Achados das outras duas fronteiras moram noutros arquivos:

- `internal/wa-noise/HOUSEKEEP.md` — a pilha de protocolo;
- `HOUSEKEEP.md` (raiz) — a aplicação `wa-api`, o build, os gates, a
  configuração e as rotas HTTP.

A separação não é organizacional: os três têm ciclos de vida diferentes. Um
achado que atravessa a fronteira fica no arquivo de quem **causa** o problema,
com referência cruzada no outro.

Este arquivo registra o que foi **descoberto de lado**. O que decidimos
divergir da implementação de referência (`whatsapp-web.js`) vai para
`PATCHES.md`, neste mesmo diretório, e decisão que vale para o projeto inteiro
vai para `docs/adr/`.

O formato de cada entrada, e a política anti-regressão que rege a passagem
para "corrigido", estão em `CLAUDE.md` / `AGENTS.md`.

## Convenção de status

Toda entrada termina com um `**Status**:` cujo **veredito vem em negrito**,
para que uma varredura mecânica o encontre. Ele pode estar no início da linha
ou como item de lista (`- **Status**: ...`) — os dois layouts convivem nos
HOUSEKEEP do repositório, e **uma varredura tem de aceitar os dois**:

- `**Status**: **corrigido**` — com os testes que o travam e o controle
  negativo executado (ver a política anti-regressão em `CLAUDE.md`).
- `**Status**: **não corrigido**` — seguido do motivo.
- `**Status**: **fechado — não corrigir**` — decisão registrada, não pendência.
- `**Status**: **parcialmente corrigido**` — com o que ficou aberto e por quê.

O formato importa, e a varredura também. A lição vem do `HOUSEKEEP.md` da raiz,
onde em 2026-08-08 três varreduras seguidas erraram: uma leu cinco entradas
como "sem status" porque o veredito estava em texto simples, outra perdeu
quatro porque o `**Status**` era item de lista. Em todos os casos **o documento
estava certo e o método errado**, e entradas íntegras quase foram "corrigidas".

Entradas com MAIS de um `**Status**` são legítimas: o achado tem sub-itens com
desfechos diferentes. O veredito que vale é o do sub-item; não existe um status
único para elas.

Seções que são **nota** e não achado — evidência nova para entradas existentes,
observação de acompanhamento — não levam status. Dê a elas um título que diga
isso ("Nota sobre…", "Evidências novas…"), para que a varredura as distinga de
um achado que esqueceu o status.

Referências entre entradas são por **título**, nunca por número de linha.
Referência a achado de outro HOUSEKEEP diz de qual arquivo ele é — o `F` de um
arquivo não é o `F` do outro.

## Numeração

As entradas deste arquivo são numeradas a partir de **H1**, e não da série `F`
usada pela raiz e pelo `wa-noise`. Os dois prefixos convivem no repositório sem
colidir, e um `H` no texto diz de imediato de qual arquivo o achado é.

---

## H1 — o texto de erro do `OpLog` não tem política de redação

**Data**: 2026-08-11 · **Contexto**: CAP-02, porte do `OpLog` do estudo para
`internal/wa-headless/observability/`.

**Onde**: `internal/wa-headless/observability/oplog.go`, campo `OpRecord.Err` e
a constante `maxErrTextLen`.

**Problema**: o `OpRecord` foi desenhado para não carregar conteúdo de página —
classe, rótulo, tempos e desfecho bastam para localizar onde uma execução
parou. O campo `Err` fura isso e não é evitável no nível em que está: um erro
de driver pode citar o que a página lançou, e o que a página lança pode citar o
que ela estava segurando. Truncar em 200 caracteres limita o RAIO, não o
conteúdo — 200 caracteres de dado de terceiro continuam sendo dado de terceiro
num log.

Nada disso vaza hoje: o `OpLog` não está ligado ao zerolog da aplicação e
nenhum caminho de produção o consome. O achado é o **débito**, não um
vazamento.

**Correção sugerida**: decidir a política no CAP-04, quando o liveness passar a
gerar erros de página em volume, e antes de qualquer ponte para o log
estruturado. Três caminhos, em ordem de preferência: (a) classificar o erro
numa taxonomia fechada e registrar só a classe, mantendo o texto apenas sob
`WA_HEADLESS_OP_TRACE`; (b) redator injetável no `OpLog`, com o padrão sendo
redigir; (c) manter o texto e travar por catraca de PII na saída. O item 5 da
Definition of Done (`HANDOFF-INICIATIVA.md`) exige zero PII em log verificado
por catraca, então (c) sozinho não fecha.

**Status**: **não corrigido** — é débito consciente do CAP-02, com o raio já
limitado por `maxErrTextLen` e travado por `TestAddTruncatesErrorText`. A
decisão exige saber a forma dos erros reais, que o CAP-04 vai produzir;
escolher agora seria projetar sem medida.

## H2 — a fundação subiu o Go do módulo inteiro para 1.26

**Data**: 2026-08-11 · **Contexto**: CAP-02, commit `wa-headless: add target
priming`.

**Onde**: `go.mod` (`go 1.25.0` → `go 1.26`) e `Dockerfile:1`
(`golang:1.25.12-bookworm` → `golang:1.26-bookworm`).

**Problema**: o `HANDOFF-INICIATIVA.md` §11 prometia raio de alcance **zero**
para o CAP-02 — "nada consome `internal/wa-headless` hoje". A promessa não
sobreviveu ao `PrimeTab`: ele é `chromedp.Run`, o `chromedp` a partir da
v0.15.0 exige Go 1.26, e o `go get` reescreveu a diretiva `go` do módulo
`wa-api` inteiro. Com o `Dockerfile` ainda em `golang:1.25.12` o build de
produção quebraria, então ele subiu junto.

A alternativa era fixar `chromedp v0.14.2`, a última que aceita Go 1.24+, ao
custo de rodar em produção uma versão **diferente** da medida: `Cancel` (§22 da
4C), `Poll` com relógio na página (ARMADILHAS 19) e rAF em aba de fundo (4C)
são exatamente onde o estudo mediu comportamento específico de versão.

**A consequência que a decisão não previu, e que foi MEDIDA depois**: o bump
**quebra o gate de lint**, local e no CI.

```
$ make lint
Error: can't load config: the Go language version (go1.25) used to build
golangci-lint is lower than the targeted Go version (1.26)
FALHA: nao consegui extrair a contagem de issues de .lint.out.
make: *** [lint] Error 1
```

O `.github/workflows/ci.yml:37` fixa `v2.5.0` (compilada com go1.25.1) e o
`setup-go` usa `go-version-file: go.mod`, ou seja **1.26** — o CI vai reproduzir
exatamente este erro.

Três tentativas de conserto, todas medidas e todas descartadas:

1. **Recompilar a v2.5.0 com o Go local** (`go install …@v2.5.0`, inclusive com
   `GOTOOLCHAIN=local`): sai `built with go1.24.2`, porque o `go.mod` da própria
   ferramenta fixa 1.24.2. Falha igual, um número abaixo.
2. **`run.go: "1.25"` no `.golangci.yml`**, para analisar num nível de
   linguagem atrás do `go.mod`. O erro de config some — prova que o botão
   funciona — e o type checker então **panica** dentro do pacote:
   `panic: package requires newer Go version go1.26 (application built with
   go1.25)`. A causa é a DEPENDÊNCIA: o `chromedp/cdproto` declara `go 1.26`, e
   o `go/types` embutido na v2.5.0 não lê o pacote. Nenhuma configuração
   resolve. Alteração revertida.
3. **Subir para `v2.12.2`** (release mais nova, 2026-05-06): o instalador
   oficial **reprovou o checksum** do tarball darwin/arm64
   (`c8debe3b…` vs `a9c54498…`). Não contornado — e não deve ser.

Conclusão medida: **a v2.5.0 não consegue lintar este módulo enquanto o
`chromedp v0.16.0` for dependência, independente de configuração.** Restam
duas saídas, e as duas são decisão do usuário: subir o linter (e o
`.golangci-baseline` junto, no mesmo PR, como o comentário do CI exige) ou
voltar ao `chromedp v0.14.2` e ao Go 1.25.

**Nota de atribuição**: o `make coverage-gate` também falha aqui
(`go: no such tool "covdata"`), e isso **não** é efeito do bump — nem
`go1.25.12` nem `go1.26.0` trazem `covdata` em `pkg/tool` quando o toolchain
vem do module cache. O CI instala distribuição completa e não vê isso. Era
falha de ambiente local, e já existia.

**Correção sugerida**: decidir entre as duas saídas acima. Verificar também o
build da imagem: `golang:1.26-bookworm` não foi construído nesta sessão, só
declarado.

**Status**: **não corrigido** — o gate de lint está quebrado nesta branch e o
CI vai reprovar. `build`, `vet`, `test` (suíte inteira, com `-race`),
`log-coverage-gate`, `waclient-facade` e `waclient-filesize` passam.

## H3 — dois documentos do `disparazaap` afirmam o que a Fase 6 refutou

**Data**: 2026-08-11 · **Contexto**: CAP-02; a pendência vem da Fase 6 e
continua sem dono.

**Onde**: no repositório `disparazaap`, branch `features/macbook-lucas`:
`docs/architecture/wa-headless-chromium-cdp.md` §2.1.1 e
`docs/adr/0038-wa-headless-absorve-wa-worker-servico-go-unico.md`.

**Problema**: o §2.1.1 afirma que "chegar a 2 renderers seria da ordem de
100–200 MB por sessão". O nó H1 da Fase 6 refutou a segunda metade por medida
direta (`RELATORIO-FASE-6.md` §2): os 2 renderers do boot existem antes de
qualquer navegação, não são os `browser_ui` (teste causal, N=3, unânime), e
nenhuma das cinco flags candidatas reduz a contagem. A aplicação inteira soma
**+1** renderer. Separadamente, a ADR-0038 ficou **deslocada** do escopo desta
iniciativa quando o objetivo foi reescrito para "motor equivalente ao
`whatsapp-web.js` atrás da fenda que já existe".

**Correção sugerida**: propor as duas correções pelo `HOUSEKEEP.md` do
`disparazaap`, **não editar de aqui** — aquele repositório é da sessão C0/C1, e
escrever nele desta sessão é o BLOCKER de trabalho paralelo que já foi furado
uma vez.

**Status**: **não corrigido** — pendência de outra sessão, registrada aqui para
não sumir. Referência cruzada: `scripts/chromium-study/RELATORIO-FASE-6.md` §8.
