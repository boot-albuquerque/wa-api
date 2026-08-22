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

**Status**: ~~**não corrigido** — é débito consciente do CAP-02, com o raio já
limitado por `maxErrTextLen` e travado por `TestAddTruncatesErrorText`. A
decisão exige saber a forma dos erros reais, que o CAP-04 vai produzir;
escolher agora seria projetar sem medida.~~
**CORRIGIDO em 2026-08-19 (LOOP 06.4)** — a pré-condição que esta entrada
impunha foi satisfeita, e a política saiu de MEDIÇÃO. Ver a resolução abaixo.

### Resolução — a medição primeiro, porque ela estreitou o problema

A entrada dizia que escolher sem medir seria projetar às cegas. Então o primeiro
passo foi produzir os erros reais contra um browser de verdade e ler o que eles
carregam:

| caso | vaza? | texto do erro |
|---|:---:|---|
| `throw new Error("SEGREDO")` | **SIM** | `exception "Uncaught" (0:9): Error: SEGREDO…` |
| `throw {chat: "SEGREDO"}` | não | `exception "Uncaught" (0:9): Object` |
| retorno não-JSON | não | **erro nenhum** |
| referência inexistente | não | `ReferenceError: nonExistente is not defined` |

**O vetor é UM só, e é estreito**: a mensagem de um `Error` lançado pela página.
Objeto lançado vira o nome do tipo; `ReferenceError` nomeia um identificador do
nosso próprio script. Nenhuma delas é conta alheia.

E o erro tem **estrutura**: `exception "<marcador>" (linha:col): Tipo: mensagem`.
Isso permitiu a opção (a) da entrada numa forma melhor que "registrar só a
classe": **preservar a forma e descartar só a mensagem**. O que sobra —
marcador, posição, tipo do erro — é vocabulário do driver, e é o que torna o
erro diagnosticável.

**Escape hatch mantido**: sob `WA_HEADLESS_OP_TRACE` o texto completo sobrevive.
A política é sobre o que se registra **por padrão**, não sobre destruir
diagnóstico — e há teste que falha se o toggle parar de preservar.

**Falha FECHADA**: marcador reconhecido com cauda que a política não sabe
interpretar perde a cauda inteira. Quando a forma é desconhecida é exatamente
quando um redator não pode adivinhar onde acaba a parte segura.

**RESÍDUO DECLARADO, com teste**: a detecção ancora no marcador MEDIDO
(`exception "`). Uma exceção que chegue noutro formato futuro **não é
reconhecida**, e o único anteparo é a truncagem — que limita raio, não conteúdo.
Por isso a truncagem foi MANTIDA junto com a redação: os dois falham de formas
diferentes. `TestResidualRiskIsBoundedByTruncation` trava isso.

**Uma afirmação minha caiu no controle negativo.** Eu escrevi que a ORDEM
importava — que truncar antes de redigir manteria meia mensagem. O controle que
inverte a ordem **PASSOU**, e isso provou a afirmação falsa: o marcador fica no
COMEÇO da string e a truncagem corta o FIM, então a redação reconhece a forma
nos dois casos. Comentário e nome do teste corrigidos para dizer o que o
mecanismo realmente garante. Um teste nomeado por uma propriedade que o código
não tem é falsa garantia — a classe exata que este módulo passou a semana
caçando.

**Controles negativos executados**: não redigir nada (falha); falhar ABERTO numa
forma não reconhecida (falha); redigir também sob o toggle (falha, porque mata o
diagnóstico que o toggle existe para dar). O quarto, da ordem, não mordeu — e
está registrado acima como correção em vez de escondido.

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

**Status**: ~~**não corrigido** — o gate de lint está quebrado nesta branch e o
CI vai reprovar. `build`, `vet`, `test` (suíte inteira, com `-race`),
`log-coverage-gate`, `waclient-facade` e `waclient-filesize` passam.~~
**CORRIGIDO** — o lint em `e5ee22e` + `2aa304d` (FASE B0 no `EXECUCAO.md`), e a
imagem em 2026-08-12 (LOOP H2.1), abaixo.

### O build de produção, CONSTRUÍDO em vez de declarado — LOOP H2.1 (2026-08-12)

A pendência que restava desta entrada era literal: *"`golang:1.26-bookworm` não
foi construído nesta sessão, só declarado."* Construído agora, ponta a ponta,
sem alterar nenhum arquivo de build:

```
docker build -t wa-api:h2-check .
EXIT=0
imagem   wa-api:h2-check   911MB
binário  /app/wa-api       43.048.079 bytes, -rwxr-xr-x waapi:waapi
```

A tag resolve para uma toolchain que satisfaz a diretiva do `go.mod`, o que era
a dúvida real — `1.26-bookworm` é uma tag móvel, e o módulo exige 1.26 por causa
do `chromedp v0.16.0`:

```
docker run --rm golang:1.26-bookworm go version
go version go1.26.5 linux/arm64
```

Então o `CGO_ENABLED=1 go build ./cmd/core` do estágio builder compila de fato
com o Go que a branch passou a exigir, e o estágio final (`debian:bookworm-slim`)
recebe um binário que existe e tem bit de execução.

**O que isto NÃO prova**, e vale dizer porque o achado nasceu de uma declaração
tomada por verificação: que o binário SIRVA — não foi executado contra
dependências, só inspecionado na imagem. E a construção foi em **arm64**, que é
a mesma limitação de arquitetura já registrada no `HANDOFF-INICIATIVA.md` §7.
`amd64` continua não medido.

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

## H4 — o formato do `SingletonLock` é premissa, não medida

**Data**: 2026-08-11 · **Contexto**: CAP-03 LOOP 03.2, `ReclaimProfile`.

**Onde**: `internal/wa-headless/engine/profile.go`, `readLockHolder`.

**Problema**: a decisão de apagar ou recusar depende de ler o `SingletonLock`
como **symlink cujo alvo é `<hostname>-<pid>`**. Isso vem da convenção do
`chrome/browser/process_singleton_posix.cc`, e o dublê do teste imita essa
regra — mas **nenhum perfil real foi inspecionado nesta sessão** para confirmar
o formato na versão 151 que o projeto usa.

Se o formato divergir, o `readLockHolder` cai no ramo "ilegível" e o reclaim
apaga sem provar nada. Isso não é catastrófico — é exatamente o comportamento
do estudo, que rodou assim por todas as fases — mas a guarda que a H4 protege
deixa de guardar, e em silêncio.

**Nota de método (2026-08-12)**: o plano original — "matar o Chromium e olhar o
perfil" — **não produz nada**. O perfil pareado foi aberto e fechado três vezes
naquela data (M3 do `EVIDENCIA-SPA.md`) e não havia `SingletonLock` nem antes
nem depois: o desligamento limpo o remove. Medir o FORMATO exige inspecionar
com o Chromium **em execução**.

---

### MEDIDO em 2026-08-12 (LOOP 03.9) — a premissa está CORRETA

**Instrumento**: `TestRealSPASingletonLockFormat`, em
`internal/wa-headless/realspa_test.go`. Sobe o perfil com o **`engine.Launcher`
de produção** (não um `exec` à mão: o formato observado tem de ser o formato que
o parser de produção vai enfrentar), inspeciona com `os.Lstat` + `os.Readlink`
com o browser **VIVO**, e depois entrega a string medida ao
`engine.ReclaimProfile` — o parser de produção, não um dublê.

**Ambiente**: Google Chrome **151.0.7922.109**, darwin/arm64, `--headless=new`.
Perfil **pareado** (`scripts/chromium-study/wa-session/profile`), via
`WA_HEADLESS_PROFILE_DIR`.

Saída crua da corrida, **com o hostname redigido para `<HOST>`** (a máquina
carrega o nome de uma pessoa, e PII é BLOCKER do repositório). Nada mais foi
alterado:

```
BASELINE profile_files=2781 singleton_lock_present=false
profile_dir=.../scripts/chromium-study/wa-session/profile overridden=true
MEASURED lstat: mode=Lrwxr-xr-x symlink=true
MEASURED readlink target="<HOST>-63733"
MEASURED os.Hostname()="<HOST>" launched_pid=63733
PRODUCTION PARSER (real probe): lock_holder="<HOST>-63733" reason="" removed=[] \
  err=profile is held by a live browser: pid 63733 on "<HOST>" is still running
PRODUCTION PARSER (holder declared dead): reason="holder pid 63733 on this host is gone" \
  removed=[SingletonLock SingletonCookie SingletonSocket] err=<nil>
stopped_via=browser.close
HYGIENE profile_files before=2781 after=2781 delta=+0
AFTER CLEAN STOP singleton_lock_present=false
```

O `<HOST>` redigido tem forma que importa para o parser e por isso fica
registrada: contém **hífens** e um **ponto**, e termina em `.local`. Ou seja, a
regra "o pid vem depois do ÚLTIMO hífen" foi exercitada contra um hostname
hifenizado de verdade, não contra um nome simples.

**Veredito: a premissa está CONFIRMADA, ponto por ponto.**

| o que `readLockHolder` assume | o que foi medido |
|---|---|
| o arquivo existe enquanto o browser vive | existe (`Lstat` OK com o browser vivo) |
| é um **symlink** | `mode=Lrwxr-xr-x`, `symlink=true` |
| o alvo é `<hostname>-<pid>` | `"<HOST>-63733"` |
| `hostname` é o mesmo que `os.Hostname()` devolve | idêntico, inclusive o sufixo `.local` |
| `pid` é o do processo browser | `63733` = `launched_pid` do `Launcher` |

A verificação que decide não é a inspeção crua — é a **segunda metade**: a
string medida entregue ao `ReclaimProfile` real. Com a sonda de liveness de
produção ele devolve `ErrProfileHeldByLiveBrowser` e **não apaga nada**; esse
erro tipado só é alcançável DEPOIS de o alvo ter sido decomposto em host igual
ao nosso e pid vivo. Com o detentor declarado morto, ele apaga os três. Se o
formato divergisse, o ramo "ilegível" devolveria erro nulo e apagaria — que é
exatamente o silêncio que esta H4 existia para descartar.

**Controle negativo: NÃO se aplica, e dizer isso é a resposta honesta.** Um
controle negativo prova que um teste MORDE quando o defeito volta. Aqui não há
defeito, não há correção e não há dublê novo: nada foi alterado em
`profile.go` nem em `profile_test.go`. Inventar uma mutação cerimonial travaria
uma regra que já está travada pelos testes do `profile_test.go` — o que a
medição acrescenta é que o dublê daquele arquivo **não é mais permissivo que a
produção**, porque a produção agora foi vista escrevendo exatamente o que o
dublê escreve.

**Limite declarado, e ele é real**: isto foi medido em **macOS**, e a invariante
que a guarda protege é "**Reclaim de `Singleton` no boot é obrigatório em
contêiner**" (`HANDOFF-INICIATIVA.md` §6, numerada **13**) — ou seja, Linux. Os
dois lados usam `gethostname(2)` (`base::GetHostName` no Chromium,
`os.Hostname` no Go), então a igualdade de host deve valer também lá, mas
**deve** não é medido. Para o contêiner o formato continua **DESCONHECIDO**, e
está nomeado como tal em vez de assumido.

> **Divergência de numeração, sinalizada e NÃO corrigida**. A invariante é, pelo
> TEXTO, *"Reclaim de `Singleton` no boot é obrigatório em contêiner"*, que o
> `HANDOFF-INICIATIVA.md:394` — fonte de verdade — numera **13**. Chamam-na de
> **15** dois pontos do código, e o segundo só apareceu na avaliação adversarial
> do LOOP 03.9:
>
> | onde | texto literal |
> |---|---|
> | `engine/profile.go:8` | *"it is **invariant 15** of the initiative"* |
> | `engine/profile_test.go:48` | *"The happy path of **invariant 15**"* ← quarta ocorrência do deslocamento, não sinalizada antes |
>
> O mesmo deslocamento atinge OUTRAS invariantes, e essas já estavam
> sinalizadas: `PARIDADE-WWEBJS.md:88` chama `livenessCheck` de *"invariante
> 11"* quando o `HANDOFF` a numera **10**, e `HOUSEKEEP.md:580` fala em
> *"invariante 14"* para zero-PII, numerada **12**. A nota do
> `EXECUCAO.md` (auditoria da CAP-03) sinaliza esse conjunto — "invariantes 11,
> 14 e 15".
>
> **Nada foi renumerado**, aqui nem em lugar nenhum: o deslocamento está em
> triagem HUMANA, e renumerar por conta própria trocaria uma divergência
> conhecida por uma silenciosa. Cite pelo TEXTO, que não é ambíguo, e pelo
> número **13** do `HANDOFF`.

**Status**: **CONFIRMADO — H4 fechada** para Chrome 151 em macOS. Travada por
`TestRealSPASingletonLockFormat` (gated por `WA_HEADLESS_REAL_SPA`) e pelos
testes de `engine/profile_test.go`, que continuam verdes e cujo dublê a medição
validou. Fica aberto apenas o caso **contêiner/Linux**, nomeado acima como
desconhecido.

## H5 — o teste do renderer travado vaza o browser, e ninguém percebe

**Data**: 2026-08-12 · **Contexto**: achado de lado durante o LOOP B1.4, ao
inventariar processos antes de abrir o perfil pareado. Não faz parte daquele
escopo.

**Onde**: `internal/wa-headless/integration_test.go:197-202`, o `defer` de
`TestBrowserChainReportsAWedgedPageAsUnresponsive`.

```go
defer func() {
    // A wedged renderer will not honour Browser.close, so the dirty path is
    // the expected outcome here. What matters is that it terminates.
    via := engine.CleanStop(context.Background(), runner, browser)
    t.Logf("stopped_via=%s", via)
}()
```

**Problema**: o comentário ENUNCIA o requisito — *"what matters is that it
terminates"* — e o código só **registra** o desfecho. Nada afirma que o
processo morreu. Se o `CleanStop` voltar sem matar o browser, o teste passa
verde e deixa o Chromium para trás.

Foi o que aconteceu, e foi medido:

```
6 processos Chromium órfãos, --user-data-dir=/var/folders/.../
  TestBrowserChainReportsAWedgedPageAsUnresponsive166315251/001
idade: 1 dia 6h    CPU acumulada: 20min43s no processo principal
diretório de perfil temporário sobrevivente: 81 MB
```

Honestidade sobre o alcance: **uma** ocorrência, de uma execução anterior a
esta sessão. O `go test -race ./internal/wa-headless/...` rodado hoje **não**
vazou. Então não é "todo run" — pode depender de a execução ser interrompida.
A causa NÃO foi determinada; isto é achado, não diagnóstico.

**Por que isto é mais que higiene de teste**: o comentário afirma, sem medida
ao lado, que *um renderer travado não honra `Browser.close`*. Se isso for
verdade — e o vazamento é compatível com isso —, então o módulo **não tem
caminho de desligamento provado para o estado UNRESPONSIVE**, que é exatamente
o estado que a CAP-04 existe para detectar. A invariante 2 proíbe sinal em
perfil com credencial (o `SIGTERM` deu logout na 4ª e na 6ª iteração), e um
browser travado segurando credencial é um dilema que a política ainda não
resolveu. Atinge o item 11 da Definition of Done (nenhum vazamento conhecido
de Chromium).

**Correção sugerida**, em ordem:

1. **Travar o requisito que o comentário já declara**: depois do `CleanStop`,
   afirmar a morte do processo com `WaitExit`/`ProcessAlive` (existem desde os
   loops 03.4/03.5 e não são usados aqui). Isto transforma o vazamento em
   falha de teste — hoje ele é invisível.
2. **Medir a afirmação**: `Browser.close` realmente não é honrado por um
   renderer travado, ou o `CleanStop` desiste antes? São causas diferentes com
   correções diferentes, e a distinção não foi medida.
3. **Decidir a política de escalada** para UNRESPONSIVE com credencial, e
   registrá-la — é decisão de arquitetura, não de teste. Aplicar a Regra 4 do
   `CLAUDE.md`: o conserto também é um mecanismo, e matar por sinal é
   exatamente o que a invariante 2 proíbe.

**Status**: ~~**não corrigido** — fora do escopo do LOOP B1.4, e o item 3 é
decisão humana de arquitetura. Os processos órfãos e o diretório de 81 MB
foram removidos em 2026-08-12 com autorização do usuário; o DEFEITO que os
produziu continua no lugar.~~
**PARCIALMENTE CORRIGIDO em 2026-08-12 (LOOP H5.1)**: itens 1 e 2 fechados, item
3 aberto e com a pergunta ESTREITADA pela medição. Ver abaixo.

### Resolução dos itens 1 e 2 — LOOP H5.1 (2026-08-12)

**Item 2 primeiro, porque ele desmonta a premissa do item 3.** A afirmação do
comentário — *"a wedged renderer will not honour `Browser.close`"* — é **FALSA**,
medida em três corridas consecutivas:

```
corrida 1   stopped_via=browser.close   (36,1s)
corrida 2   stopped_via=browser.close   (36,1s)
corrida 3   stopped_via=browser.close   (37,9s)
```

O mecanismo deixa de ser surpreendente assim que enunciado: o travamento é um
`for(;;)` na thread principal do **RENDERER**, e o `Browser.close` é servido pelo
processo **BROWSER**, que é outro processo e não está girando. Uma aba pendurada
não deixa o browser surdo.

Isso significa que **o módulo TEM caminho de desligamento provado para o estado
UNRESPONSIVE** — que era exatamente a dúvida que esta entrada levantava sobre a
CAP-04. A hipótese alternativa ("o `CleanStop` desiste antes do prazo") fica
descartada pelo mesmo dado: ele não desiste, ele conclui pelo caminho limpo.

**Item 1: o requisito que o comentário só declarava agora é asserção.** O
`defer` captura o PID antes de parar e, depois do `CleanStop`, exige as duas
coisas: `via.Clean()` e `!engine.ProcessAlive(pid)`.

**Controle negativo EXECUTADO** — `CleanStop` devolve `StopViaNoop` sem parar
nada:

```
--- FAIL: TestBrowserChainReportsAWedgedPageAsUnresponsive (36.01s)
    stopped_via=noop pid=56700
    pid 56700 is still alive after CleanStop returned noop: the test leaked the
    browser it owns, which is the H5 defect
```

Repare em qual asserção mordeu: `StopViaNoop.Clean()` devolve **true**, então a
guarda de `via.Clean()` passou e quem pegou foi o `ProcessAlive`. As duas não são
redundantes — a que faltava era justamente a que o comentário prometia.

A mutação deixou **7 processos órfãos**, removidos em seguida. A mesma ordem de
grandeza dos 6 medidos no achado original, o que corrobora que o vazamento
relatado é este caminho e não outro.

### O que continua aberto — item 3, com a pergunta estreitada

A medição **não** fecha o item 3; ela reduz o que ele abrange. O dilema já não é
"como desligar um renderer travado" — isso sai limpo. É o caso RESIDUAL: quando
o próprio processo browser não honra o `Browser.close`, o `CleanStop` cai no
`SignalStop`, e um sinal contra perfil com credencial é o que a **invariante 2**
proíbe e o que deu logout na 4ª e na 6ª iteração da fase 4C.

As três saídas, e por que nenhuma é escolha de mesa:

- **A · manter o sinal**: garante que nada vaza, ao custo de poder corromper a
  sessão pareada. É o comportamento de hoje, e ele contradiz a invariante 2.
- **B · nunca sinalizar perfil com credencial**: honra a invariante, e aceita
  deixar processo e perfil para trás com causa classificada. Troca corrupção por
  vazamento de recurso.
- **C · escalada**: reabrir conexão e repetir o `close` N vezes antes de
  qualquer sinal, registrando cada tentativa.

### O caso residual, agora MEDIDO (2026-08-12)

O passo (a) que esta entrada listava como pendente foi executado: `SIGSTOP` no
processo browser produz exatamente um browser que não pode responder ao
`Browser.close`, e não escreve nada no perfil. Perfil temporário, nunca o
pareado. Três corridas:

```
stopped_via=DIRTY_signal_close_refused   30,006s   processo vivo depois: NÃO
stopped_via=DIRTY_signal_close_refused   30,002s   processo vivo depois: NÃO
stopped_via=DIRTY_signal_close_refused   30,003s   processo vivo depois: NÃO
```

**A escalada CONVERGE.** Os 30 s são os três orçamentos em sequência (close 10 s
+ wait-exit 10 s + `signalGrace` 10 s), e o `SIGKILL` no GRUPO derruba até um
processo parado por `SIGSTOP` — sinal que não pode ser bloqueado nem ignorado.
O desfecho ainda se anuncia honestamente: `Clean()` devolve **false**, então o
chamador consegue agir sobre a diferença.

Fidelidade do instrumento, dita antes que alguém a assuma: `SIGSTOP` modela com
fidelidade **"CDP sem resposta possível"**. A palavra "recusado" estava errada e
foi corrigida pela validação independente: um processo parado mantém o socket de
escuta aberto, então a conexão **pendura** em vez de ser recusada — que é
justamente por que as três corridas gastaram o orçamento inteiro de 10 s no
`close`. Pior caso dos dois jeitos, então a conclusão não muda; a frase mudou. Ele **não** modela um
browser em deadlock mas escalonável, que ainda serviria `SIGTERM` — nesse
aspecto o `SIGSTOP` é MAIS severo que a realidade, o que torna a convergência
medida um limite inferior, não otimismo.

### O que a medição faz com a decisão

Ela desfaz a simetria que o trade-off aparentava ter. A opção **B** ("nunca
sinalizar perfil com credencial") não custa apenas vazar um processo:
`reclaimVerdict` recusa com `ErrProfileHeldByLiveBrowser` enquanto o pid do
detentor estiver vivo (`engine/profile.go`), de propósito — apagar o
`SingletonLock` de um browser vivo põe dois browsers num perfil, que é como uma
sessão pareada morre de verdade. Então B deixa o perfil **irrecuperável naquele
host** enquanto o processo travado existir: ela protegeria a sessão de
corrupção bloqueando permanentemente a própria sessão.

Isso NÃO decide a questão, e não a decidi. O custo de A continua real e medido
(fase 4C: logout na 4ª e na 6ª iteração), a invariante 2 continua proibindo o
sinal, e escolher entre "arriscar a sessão para recuperar o perfil" e "preservar
a sessão perdendo o perfil" é trade-off de produto. O que mudou é que agora
existe número dos dois lados em vez de um lado imaginado.

**Status do item 3**: ~~**aberto — decisão do usuário**~~ · **DECIDIDO em
2026-08-12: opção A, pelo usuário.** O sinal fica, e o preço de mantê-lo é pago
por mecanismo — ver a seção de implementação no fim desta entrada. As três
opções foram medidas e não mais especuladas. A opção C (escalar o `close` N vezes antes de
qualquer sinal) ficou sem justificativa aparente para este modo de falha: contra
um browser inalcançável, cada tentativa extra custa 10 s e não muda o desfecho.
Ela só ganharia sentido se existir um travamento TRANSITÓRIO, que não foi
observado.

---

## H6 — `spa.Probe` pode capturar 200 caracteres de texto da página quando a estrutura não casa

**Data / contexto**: 2026-08-12, LOOP 04.3A (liveness contra sessão que perdeu
o servidor). Achado de lado: a sonda de corte de rede chama `spa.Probe` a cada
tick contra a conta real, e ler o caminho para confirmar que isso é seguro
mostrou que ele é seguro **por uma guarda só**.

**Onde**: `internal/wa-headless/spa/probe.go:83-87` e `:116-123`.

```go
const textScript = `JSON.stringify((() => {
	if (document.querySelector('#pane-side')) return '';
	const t = document.body ? document.body.innerText : '';
	return t.slice(0, 200);
})())`
```

**Problema**: numa página classificada `OTHER` pela estrutura, o `Probe` faz a
segunda avaliação e traz até 200 caracteres de `body.innerText`. Contra
`web.whatsapp.com` com sessão pareada, esse texto é a lista de conversas —
nomes de contato e prévias de mensagem. A única coisa que impede isso é
`#pane-side` estar presente.

Ou seja: **a proteção de PII depende do mesmo seletor cuja fragilidade este
módulo inteiro assume**. No dia em que a Meta renomear `#pane-side`, o efeito
não é só "classifica errado" — é que toda sonda passa a puxar texto de conversa
para dentro do processo. O M4 mediu 180 amostras (90 severadas + 90 de
controle) em que o painel se manteve, então nada foi capturado nesta corrida;
o risco é o próximo build da Meta, não este.

Duas condições agravantes, ambas já verdadeiras hoje:

1. O caminho já é exercitado contra a conta real, não só contra dublês
   (`realspa_test.go`, sondas de observação e agora a do 04.3A).
2. O texto capturado vai para `PageSnapshot.TextSample`, que é campo
   **exportado** — qualquer chamador futuro pode logá-lo sem saber o que ele
   carrega. Nenhum loga hoje; a invariante 14 é mantida por convenção.

**Evidência**: leitura do caminho, não medição — o estado que dispara a captura
não foi produzido de propósito contra a conta real, e produzi-lo exigiria
quebrar o seletor no navegador.

**Correção sugerida**, em ordem de custo:

1. **Não confiar num seletor só para decidir se pode ler texto.** A guarda pode
   exigir a AUSÊNCIA de qualquer sinal de sessão pareada — hoje a identidade do
   dono (`WAWebUserPrefsMeUser`, M3.2) é o candidato barato: se o perfil está
   pareado, não leia texto. Nas telas que precisam de texto (conflito, "atualize
   o Chrome") a identidade também é lida do store, então a regra precisa ser
   medida antes de ser adotada — é achado, não diagnóstico.
2. **Redigir na fronteira**: `TextSample` deixa de ser `string` livre e passa a
   carregar só o que casou com um conjunto fechado de padrões conhecidos
   (conflito, atualização de browser). Fecha a classe inteira em vez de um caso.
3. **Teste que trave**: página com `#pane-side` renomeado E texto de conversa no
   corpo; asserção de que o snapshot volta com `TextSample` vazio. Com controle
   negativo: reintroduzir a leitura e ver o teste falhar.

**Status**: ~~**não corrigido** — fora do escopo do LOOP 04.3A, que é medição de
liveness, e mexer na guarda de PII sem medir qual sinal a substitui trocaria
uma dependência frágil por outra. Registrado para decisão do usuário.~~
**CORRIGIDO em 2026-08-12 (LOOP H6.1)** — ver a seção de resolução no fim desta
entrada. Testes que o travam: `TestProbeCarriesNoPageTextWhenTheSelectorIsRenamed`,
`TestProbeDiscardsMarkersItNeverAskedAbout`, `TestMarkerScriptNeverReturnsPageText`
(`spa/probe_test.go`) e `TestBrowserChainCarriesNoPageTextWhenTheSelectorIsRenamed`
(`integration_test.go`, contra Chrome real).

### Agravante removido em 2026-08-12 (LOOP 04.3B): o seletor estava escrito TRÊS vezes

O `#pane-side` aparecia como literal solto em `spa/probe.go:84` (o `textScript`
citado acima) e em `spa/liveness.go:60` (o `livenessScript`), enquanto
`spa/probe.go:50` já declarava `paneSideSelector` e o `structureScript` o usava.
Três cópias, uma nomeada.

Isso **não é cosmético para este achado**: a H6 é exatamente sobre a Meta
renomear esse seletor, e uma renomeação que atualizasse a constante e passasse
por cima das duas cópias produziria o cenário da H6 **por nossa própria mão** —
o classificador perguntando pelo nome novo, a guarda de PII e a sonda de
liveness pelo velho, e a captura de texto de conversa liberada sem que a Meta
tivesse mudado nada. É também o que a `CLAUDE.md` proíbe: *"literal repetido em
dois lugares é o mesmo bug esperando divergir."*

As duas cópias passaram a referenciar `paneSideSelector`. A superfície da H6
continua a mesma — uma guarda só —, mas agora renomear o seletor é **uma**
edição em vez de três lugares onde esquecer um.

### Resolução — LOOP H6.1 (2026-08-12): a guarda deixou de ser guarda e virou mecanismo

**O que foi adotado**: o texto da página **não atravessa mais a fronteira**. O
segundo `Evaluate` deixou de devolver 200 caracteres de `innerText` e passou a
receber um conjunto FECHADO das nossas próprias strings, perguntando quais delas
a página viu. A busca de substring acontece dentro da página; o que volta é um
subconjunto do que foi enviado, e `knownMarkers` descarta na chegada qualquer
coisa fora do conjunto.

`PageSnapshot.TextSample string` foi **removido** e substituído por
`Markers []string`. Isso encerra também o agravante 2 desta entrada: não existe
mais campo exportado de texto livre para um chamador futuro logar sem saber o
que carrega.

O significado ficou em Go. Qual combinação de marcadores é conflito e qual é
"atualize o Chrome" continua em `classifyByMarkers`, testável por unidade — só a
busca desceu para a página.

**Por que NÃO foi a guarda por identidade** (correção 1 desta entrada, que era o
caminho previsto). A tela de conflito só existe num perfil **pareado** — é isso
que a torna conflito — e o M3.3 mediu que a identidade é PERSISTIDA e reaparece
em T+0,01s, antes do socket abrir. Gatear a leitura de texto por "identidade
ausente" fecharia a leitura exatamente na tela que a leitura existe para
reconhecer, e a `ClassSessionConflict` é terminal: o chamador age sobre ela.
Seria trocar uma dependência frágil por uma perda funcional.

Isto é **inferência, não medição** — não produzi uma tela de conflito contra a
conta real para ler a identidade nela, porque a correção adotada torna a
pergunta irrelevante: sem texto cru atravessando, não há o que gatear. O
falsificador fica registrado: se algum dia se medir identidade AUSENTE numa tela
de conflito, esta justificativa cai (mas a correção adotada continua de pé, por
outro motivo).

**A ordem de preferência desta entrada estava, portanto, invertida.** A correção
2 ("redigir na fronteira") não era mais cara que a 1 — é a mais barata das duas
em consequência, porque não depende de medir sinal nenhum.

**O que a correção NÃO cobre.** A guarda de `#pane-side` continua no script,
rebaixada ao que de fato protege agora: CORREÇÃO, não privacidade. Uma lista de
conversas varrida por estas frases pode acertar uma por coincidência — alguém
escrevendo "outra aba" numa mensagem viraria `SESSION_CONFLICT` numa sessão
saudável. Essa guarda é exercitada pelo dublê e por asserção estática sobre o
script; a corrida real (painel aparecendo ENTRE as duas avaliações) não foi
reproduzida contra navegador, e continua sendo o buraco de cobertura conhecido.

**Controles negativos EXECUTADOS** (quatro mutações, todas revertidas):

**(NC-1) o script volta a fatiar o texto** — `markerScript` devolve
`[t.slice(0, 200)]`:

```
--- FAIL: TestProbeCarriesNoPageTextWhenTheSelectorIsRenamed
    the marker probe never ran, so this test did not exercise H6
--- FAIL: TestMarkerScriptNeverReturnsPageText
    the marker script slices the body text; that is the H6 defect returning
    the marker script does not filter the marker list; it is returning something else
--- FAIL: TestBrowserChainCarriesNoPageTextWhenTheSelectorIsRenamed
    renamed selector, conflict wording: classified as "OTHER", want "SESSION_CONFLICT"
```

Repare no que este controle revelou: a PII **não vazou**, porque `knownMarkers`
a barrou sozinha. As duas camadas são independentes, e uma só já segura — que é
o que se queria de defesa em profundidade, e não estava previsto no desenho.

**(NC-2) a fronteira aceita o que a página inventar** — `knownMarkers` devolve
`reported` sem filtrar:

```
--- FAIL: TestProbeDiscardsMarkersItNeverAskedAbout
    the page smuggled strings we never sent: [Mum: see you at 8 outra aba but not really]
```

**(NC-3) as DUAS camadas removidas — o defeito H6 inteiro, contra Chrome real.**
É o único controle que prova que a asserção de PII morde:

```
--- FAIL: TestBrowserChainCarriesNoPageTextWhenTheSelectorIsRenamed (1.65s)
  renamed selector, chat list: page text crossed the boundary: "Mum" is in
    {... Markers:[Mum — see you at 8
                  Work — the deploy is out
                  +55 11 99999-0000 — are we still on?]}
  ... idem para "see you at 8", "the deploy is out", "99999-0000"
  renamed selector, conflict wording: page text crossed the boundary: "Mum" is in {...}
```

**(NC-4) a guarda de correção removida do script** — a linha do `#pane-side`
apagada de `markerScript`:

```
--- FAIL: TestMarkerScriptNeverReturnsPageText
    the marker script no longer refuses a loaded application
```

> **Nota de método sobre o NC-4.** A primeira tentativa deste controle PASSOU, e
> passou porque a mutação nunca aplicou: procurei o literal `'#pane-side'` no
> fonte, que ali é a constante `paneSideSelector` concatenada. É a ARMADILHA 3
> do repo em ação — controle negativo que não aplica não prova nada, e um que
> "passa" é indistinguível de um teste que não morde. Refeito com `assert` na
> substituição antes de escrever o arquivo.

**Gate executado** (2026-08-12, árvore limpa antes e depois):

```
go build ./...                                   OK  (repo inteiro)
go vet ./...                                     OK  (repo inteiro)
scripts/chromium-study: build + vet              OK  (o laboratório não quebrou)
gofmt -l internal/wa-headless/                   vazio
go test -race -count=1 ./internal/wa-headless/...
  wa-api/internal/wa-headless               ok  98.166s
  wa-api/internal/wa-headless/engine        ok   5.015s
  wa-api/internal/wa-headless/observability ok   1.189s
  wa-api/internal/wa-headless/spa           ok   2.222s
make lint   269 issues  ·  pai (stash, MESMA árvore) 269  ·  delta 0
  internal/wa-headless: as MESMAS 2 gocyclo pré-existentes, nenhuma tocada
```

Nenhum browser desta correção saiu por sinal: `stopped_via=browser.close` nas
duas corridas do teste novo. `web.whatsapp.com` não foi tocado — as fixtures são
servidas por `httptest`, e o caminho de marcadores é alcançado servindo o host
no PATH da URL, que é a regra do próprio classificador exercitada como escrita.

---

## H7 — `PageSnapshot.Title` carrega conteúdo da página, ninguém o lê, e ele nunca foi medido

**Data**: 2026-08-12 · **Contexto**: LOOP H6.1. Achado por ataque à própria
alegação do commit, não por revisão de código — a frase "não existe caminho pelo
qual texto da página chegue a uma string Go" foi posta à prova e não sobreviveu
inteira.

**Onde**: `internal/wa-headless/spa/probe.go`, `structureScript`
(`title: document.title`) e `internal/wa-headless/spa/page.go`,
`PageSnapshot.Title`.

**Problema**: o H6 fechou o caminho do `body.innerText`, mas o `structureScript`
continua trazendo `document.title`, que é **texto autorado pela página**, e o
campo é **exportado**. É a mesma forma do agravante 2 do H6, em escala menor:
qualquer chamador futuro pode logá-lo sem saber o que carrega.

Duas coisas agravam, e uma atenua:

1. **Ninguém lê o campo.** `grep -rn '\.Title\b' --include='*.go'
   internal/wa-headless/` não devolve **nenhum** leitor. Ele é decodificado do
   JSON e nunca consultado — nem pelo `Classify`, nem por teste. Um campo que
   ninguém lê não paga o risco que carrega.
2. **Nunca foi medido.** `EVIDENCIA-SPA.md` não tem uma única observação de
   `document.title` contra o SPA real, nos dois perfis. Não sei se o título do
   WhatsApp Web é `"WhatsApp"`, `"(3) WhatsApp"` ou algo que nomeie conversa ou
   contato. **Afirmar que é inócuo seria repetir exatamente o erro que o H6
   documenta**: tratar suposição sobre a página como propriedade dela.
3. *(atenuante)* diferente do `TextSample`, o título é curto e não é a lista de
   conversas. O risco é menor em magnitude, não em natureza.

**Evidência**: leitura do caminho e o `grep` de leitores acima. **Não** é
medição contra a conta real — e é essa ausência que constitui o achado.

**Correção sugerida**, em ordem de custo:

1. **Remover o campo e o `title:` do script.** É a correção mais barata e fecha
   a classe: zero consumidores hoje, então o diff é de duas linhas e o risco de
   remoção é nulo. Reintroduzir depois, se uma classificação precisar do título,
   custa o mesmo e aí virá com medição junto.
2. **Medir antes de decidir**, se houver intenção de usar o título para
   classificar (telas de erro costumam ser identificáveis por título): uma
   observação só-leitura do perfil pareado, registrando só a FORMA do título e
   não o valor, resolve — e é a mesma sonda que o M1–M3 já usa.

**Status**: **CORRIGIDO em 2026-08-12 (LOOP H7.1)**, e a correção 1 foi a
escolhida — `title: document.title` saiu do `structureScript` e
`PageSnapshot.Title` saiu da struct. Zero consumidores, diff de duas linhas.

Registrei primeiro como "não corrigir sem perguntar", e mudei de posição por um
motivo que é do próprio achado: isto **não é conserto de graça de bug alheio**,
é a alegação do commit anterior sendo trazida de volta à verdade. O `997cebe`
afirma que "não existe caminho pelo qual texto da página chegue a uma string
Go", e enquanto o `Title` atravessasse a frase era falsa. Corrigir a alegação
por comentário e deixar o campo seria escolher a documentação em vez do
mecanismo — exatamente o que a `CLAUDE.md` proíbe.

**O achado foi feito DUAS vezes, de forma independente e simultânea**: por
ataque do Chief à própria frase do commit, e pela validação independente
(evaluator em sessão separada), que chegou nele por outro caminho — notando que
as duas fixtures do teste de navegador **não tinham `<title>`**, de modo que a
asserção de PII passava de forma VAZIA justamente no único campo que ainda
cruzava. Convergência de dois métodos diferentes no mesmo ponto cego.

**Evidência BEFORE_FIX / AFTER_FIX.** A correção do teste veio ANTES da
correção do código, de propósito — foi o evaluator quem apontou que sem título
nas fixtures a asserção não morde. Com `<title>Mum — WhatsApp</title>` nas duas
fixtures e o campo ainda no lugar:

```
--- FAIL: TestBrowserChainCarriesNoPageTextWhenTheSelectorIsRenamed (2.04s)
  renamed selector, chat list: page text crossed the boundary: "Mum" is in
    {URL:.../renamed Title:Mum — WhatsApp ... Markers:[]}
  renamed selector, conflict wording: page text crossed the boundary: "Mum" is in
    {URL:.../renamed-conflict Title:Mum — WhatsApp ... Markers:[open in another another window]}
```

Repare que os `Markers` da segunda perna continuam corretos: o conflito segue
sendo detectado, então a falha é do `Title` e de mais nada. Depois de remover o
campo e a linha do script, `PASS`, com as fixtures mantendo o título — a
asserção agora cobre o caminho que estava escondido.

**O que continua NÃO medido**, e por isso não vira alegação: se o
`document.title` do WhatsApp Web carrega nome de contato com uma conversa
aberta. A remoção tornou a pergunta desnecessária para a privacidade, mas ela
volta a importar no dia em que alguém quiser usar o título para classificar tela
de erro. Se esse dia chegar, a medição vem junto — e registre só a FORMA do
título, nunca o valor.

---

## H8 — a checagem de host do classificador é `Contains`, e aceita domínio sósia

**Data**: 2026-08-12 · **Contexto**: LOOP H6.1, achado pela **validação
independente** ao auditar por que o teste de navegador consegue alcançar o
caminho de marcadores. Não é regressão destes commits: a regra é anterior.

**Onde**: `internal/wa-headless/spa/page.go`, no `Classify`:

```go
case s.URL != "" && !strings.Contains(strings.ToLower(s.URL), whatsappHost):
    return ClassRedirect
```

**Problema**: `strings.Contains` sobre a URL INTEIRA não é uma checagem de host.
Ela aceita como "no host certo":

```
https://web.whatsapp.com.attacker.example/     (sufixo de domínio)
https://evil.example/web.whatsapp.com          (no caminho)
```

Uma página sósia deixa de ser `REDIRECT`, e como não casa estrutura nenhuma, ela
**recebe a sonda de marcadores** — ou seja, o corpo dela é varrido e ela pode
responder `SESSION_CONFLICT` ou `ERROR_PAGE` à vontade. Desde o LOOP H6.1 isso
não vaza PII (o texto não atravessa mais), mas continua sendo classificação
controlada por quem serve a página.

**Severidade prática: BAIXA** — nós controlamos a navegação, e o motor só vai
para `web.whatsapp.com`. Vira relevante se algum dia houver redirecionamento
seguido sem validação, ou captive portal.

**Evidência**: leitura da regra, mais o fato de o próprio teste
`TestBrowserChainCarriesNoPageTextWhenTheSelectorIsRenamed` explorá-la de
propósito — ele serve a fixture em `/web.whatsapp.com/renamed` para alcançar o
caminho de marcadores sem tocar o host real.

**Correção sugerida**: `url.Parse` e comparar `u.Hostname()` com igualdade (ou
sufixo `.whatsapp.com` com ponto). **Quando isso for feito, o teste do H6 vai
falhar alto** — e isso é característica, não defeito: o comentário dele promete
exatamente essa falha. A fixture então passa o host pelo cabeçalho `Host` em vez
do caminho.

**Status**: **não corrigido** — apertar a regra muda comportamento de
classificação e derruba um teste de propósito, o que merece loop próprio em vez
de carona num loop de PII. Registrado com o caminho de correção pronto.

---

## H9 — `TestBrowserChainSeversTheWebSocketTransport` degrada para desligamento SUJO sob contenção

**Data**: 2026-08-12 · **Contexto**: LOOP H6.1, observado pela **validação
independente** — a primeira corrida de `-race` dela cruzou com testes de
navegador de outra sessão na mesma máquina.

**Onde**: `internal/wa-headless/realspa_test.go`,
`TestBrowserChainSeversTheWebSocketTransport`.

**Problema**: com a máquina disputada, os dois subtestes estouraram o prazo e a
corrida terminou com:

```
stopped_via=DIRTY_signal_after_close_timeout
```

Sozinho, o teste passa — reproduzido depois pela mesma sessão. Então não é
regressão dos commits do H6/H5, e é **pré-existente**.

**Por que isto não é só flakiness de teste**: `DIRTY_signal_after_close_timeout`
é o desligamento que a **invariante 2** proíbe em perfil com credencial, e a
fase 4C mediu que ele dá logout (4ª e 6ª iteração). O gatilho aqui foi carga da
máquina, não defeito do browser — e "só acontece sob carga" é precisamente como
uma condição chega à produção, onde a densidade por nó é o objetivo declarado da
iniciativa. Um nó com N sessões É a máquina disputada.

A pergunta que fica: o orçamento de `Shutdown` (10 s) é suficiente sob a
densidade que a CAP-10/CAP-11 pretendem, ou o caminho limpo passa a falhar em
volume exatamente quando mais importa?

**Evidência**: uma ocorrência, sob contenção conhecida, com a corrida isolada
passando. Não é medição de distribuição — é achado.

**Correção sugerida**: medir o tempo real de `Browser.close`→saída sob carga
crescente antes de fixar o orçamento; e considerar distinguir "estourou o prazo
porque a máquina está lenta" de "o browser não vai sair", que hoje colapsam no
mesmo desfecho sujo.

**Status**: **não corrigido** — pré-existente, fora do escopo dos loops de PII e
de shutdown desta sessão, e a correção depende de medição sob carga que não foi
feita. Referência cruzada: a invariante 2 do `HANDOFF-INICIATIVA.md` §6 e o item
6 da Definition of Done (`stopped_via` limpo em 100% das paradas).

### Implementação da decisão A — LOOP H5.3 (2026-08-12)

**A decisão, do usuário**: manter o sinal como recurso final. O argumento que a
sustenta não é conforto — é que a medição de 4C (logout na 4ª e na 6ª iteração)
foi feita com `SIGTERM` como caminho de ROTINA, tomado toda vez, e o caso
residual é sinal RARO depois de o caminho limpo falhar. Transportar aquele
número para cá seria usar uma medição fora da condição que a produziu, que é a
armadilha que este repositório mais paga. Do outro lado, "nunca sinalizar" tem
custo medido e certo: o `reclaimVerdict` recusa enquanto o pid viver.

**O preço, pago por mecanismo e não por promessa** (`engine/suspect.go`):

- toda parada suja escreve `.wa-headless-session-suspect` **dentro do perfil**,
  com o `StopVia` que a causou;
- a marca é escrita **pelo `CleanStop`**, não pelo chamador. `ProfileDir()`
  entrou na interface `BrowserProcess` exatamente por isso: deixar a marcação a
  cargo do call site faria ela depender de alguém lembrar, e o defeito que este
  pacote existe para impedir é precisamente um call site que esqueceu;
- a marca fica **dentro do perfil** de propósito: o perfil é o que viaja, e um
  perfil restaurado noutro host ou num contêiner leva a própria dúvida junto;
- ler **não** limpa. Só `ClearSessionSuspect` limpa, e ela só deve ser chamada
  depois de a sessão ter sido verificada de verdade — se a leitura limpasse, um
  boot que lesse o estado e caísse em seguida perderia a dúvida;
- perfil ilegível devolve **erro**, nunca `false`. "Não consegui checar" virando
  "está bom" é a forma fail-open que a invariante 12 proíbe.

**Testes que travam a decisão**: `TestCleanStopMarksTheProfileWhenTheStopGoesDirty`,
`TestCleanStopLeavesNoMarkWhenTheStopIsClean`,
`TestSuspectMarkSurvivesReadingAndIsClearedOnlyExplicitly`,
`TestSessionSuspectRefusesToGuessOnAnUnreadableMarker`,
`TestMarkingIsANoopWithoutAProfile` (`engine/suspect_test.go`).

**Controles negativos EXECUTADOS**, dois, ambos revertidos:

**(NC-A1) a parada suja não é registrada** — a chamada de marcação removida do
`CleanStop`:

```
--- FAIL: TestCleanStopMarksTheProfileWhenTheStopGoesDirty (0.15s)
    the browser went down by signal and the profile was left unmarked; the next
    boot would presume the session survived, which is exactly what decision A
    buys with the signal it keeps
```

**(NC-A2) marca em TODA parada, inclusive limpa** — é o controle que decide se a
marca SIGNIFICA alguma coisa. Um alarme permanente é um alarme ignorado, e
tornaria o sinal indistinguível do caminho do protocolo — a mesma armadilha que
fez o braço `browserclose` do estudo virar réplica do próprio controle:

```
--- FAIL: TestCleanStopLeavesNoMarkWhenTheStopIsClean (0.00s)
    a clean stop marked the session suspect; the signal would then be
    indistinguishable from the protocol path
```

**Fidelidade do dublê**: o `fakeProcess` devolve um diretório temporário REAL em
`ProfileDir()`, não `""`. Com string vazia o `MarkSessionSuspect` retorna cedo e
todos os testes do caminho sujo passariam sem a marcação jamais rodar — dublê
mais permissivo que a produção escondendo o mecanismo inteiro (ARMADILHA 1).

**O que fica aberto, e é consequência desta decisão, não pendência dela**: quem
CONSOME a marca. O `SessionSuspect` existe e é lido por teste; nenhum caminho de
boot ainda age sobre ele, porque o ciclo de vida que agiria é a CAP-05. A marca
não é auto-executável — ela garante que a informação sobreviva até existir quem
a use, que é o oposto de perdê-la em log.

## H10 — "o perfil nunca encolhe" é falso no granulado de UM boot: o Chromium rotaciona `Default/Sessions/*`

**Data**: 2026-08-12 · **Contexto**: achado **de lado** no LOOP 03.9, ao medir
higiene de perfil em volta da observação do `SingletonLock`. Não faz parte
daquele escopo, e não foi transformado em trabalho deste ciclo.

**Onde**: comportamento do Chromium 151 sobre `<perfil>/Default/Sessions/`. Do
nosso lado o que ele atinge é a expectativa escrita como observável da **CAP-05**
no `HANDOFF-INICIATIVA.md` §10 — *"N ciclos dormir/acordar sem degradação,
`Singleton` = 0, perfil **não-decrescente**"* — e o texto do LOOP 04.3D no
`EXECUCAO.md` ("perfil 2686 → 2781 arquivos, **cresceu sempre**").

**Problema, e ele tem duas metades de peso diferente** — a primeira é MEDIDA, a
segunda é HIPÓTESE. Elas estavam escritas como se fossem a mesma coisa, e a
avaliação adversarial do LOOP 03.9 mostrou que não são.

### MEDIDO — a asserção "um boot nunca reduz a contagem" é FALSA

Primeira corrida da observação, no perfil de laboratório:

```
BASELINE profile_files=457 singleton_lock_present=false
...
stopped_via=browser.close
HYGIENE profile_files before=457 after=456 delta=-1
the profile SHRANK (457 -> 456): a boot must never lose profile state
```

Isto é medição direta e não depende de explicação nenhuma: um boot **limpo**
(`stopped_via=browser.close`, sem parada suja, sem crash) devolveu o perfil com
**um arquivo a menos**. Portanto o observável "perfil **não-decrescente**" por
contagem de arquivos **reprova em boot sadio**. Esta metade é firme, e é ela
que invalida a checagem de higiene — o *porquê* do −1 não muda isso.

### HIPÓTESE — que a rotação de `Session_*`/`Tabs_*` seja a causa DESSE −1

Que o Chromium **rotacione** o par `Session_*`/`Tabs_*` está provado: o `diff`
das duas listagens (só os nomes relativos ao perfil) mostra par descartado e par
novo escrito, não deleção sem reposição.

```
350d349
Default/Sessions/Session_13431009036226323
352c351
Default/Sessions/Tabs_13431009036997542
---
Default/Sessions/Session_13431037567163574
353a353
Default/Sessions/Tabs_13431037567202699
```

**O que NÃO está provado é que essa rotação explique o −1**, e a razão é de
método: este `diff` foi tirado em volta de uma corrida **posterior**, cujo delta
foi **0** — 2 removidos, 2 acrescentados, saldo zero, como as próprias linhas
acima mostram. A corrida que de fato encolheu (457 → 456) **nunca foi diffada**.
Dizer "a rotação causou o −1" é inferir de um mecanismo real para um evento que
ele não foi visto causando. É exatamente o tipo de passagem que a regra "medir
antes de projetar" existe para barrar, e ela passou despercebida aqui.

**O que confirmaria o mecanismo** (barato, e ainda não feito): tirar a listagem
completa ANTES e DEPOIS **da mesma corrida cujo delta seja negativo**, e diffar
essa. Confirma se: o único nome que sumiu sem substituto estiver sob
`Default/Sessions/`. **Refuta se**: o arquivo perdido estiver fora de
`Default/Sessions/` — aí o −1 é perda de estado de verdade e esta entrada muda
de "comportamento saudável" para defeito. Como o delta negativo não apareceu em
todas as corridas (as seguintes deram `+0`), pode ser preciso repetir o boot até
reproduzi-lo; registrar quantas corridas custou faz parte da medida.

Enquanto isso não for feito, o que se pode afirmar é: `delta<0` num boot limpo
**acontece**, e a rotação é a explicação **plausível e não confirmada**.

**Por que isso importa — e repare que isto se apoia só na metade MEDIDA**: um
gate que trave "perfil não-decrescente" por contagem de arquivos vai reprovar em
boot sadio, e vai reprovar quer a causa seja a rotação, quer seja outra. O custo
é pior que o falso positivo — ensina o próximo leitor a APAGAR a checagem em vez
de lê-la, e aí o dia em que o perfil encolher de verdade não terá alarme nenhum.

**Correção sugerida** (para quem fechar a CAP-05, não para agora): o observável
"perfil não-decrescente" precisa ser enunciado sobre o que de fato não pode
sumir — o material de sessão (`Default/Local Storage`, `Default/IndexedDB`, as
chaves que a M3 mostrou carregarem a identidade) — e não sobre `find | wc -l`.
Excluir `Default/Sessions/*` da conta é o passo natural, **mas ele depende da
hipótese acima**: enquanto o diff da corrida negativa não for tirado, quem
excluir aquele diretório está tirando da vigilância um lugar que ainda não foi
visto sendo a causa. A ordem certa é medir primeiro, excluir depois.

**Status**: **não corrigido, e de propósito**. No LOOP 03.9 a asserção foi
trocada por REGISTRO no `TestRealSPASingletonLockFormat` — a contagem sai no
log — porque travar um número que nunca foi medido é a mesma especulação de
sempre, só que com cara de teste. O que o teste trava de verdade é o que foi
medido: depois do `browser.close` não sobra `SingletonLock`. **Revisado em
2026-08-12** pela avaliação adversarial do 03.9: a redação anterior apresentava
a rotação como causa medida do −1, e o comentário do teste dizia o mesmo; os
dois passaram a separar medição de hipótese. O achado **não** foi enfraquecido —
a metade que invalida o observável da CAP-05 é a medida.

---

## H11 — a coluna "janela em `OPENING`" do M6 não é o tempo em `OPENING`

**Data**: 2026-08-13 · **Contexto**: LOOP 04.3E (a perna que deveria PIORAR,
`EVIDENCIA-SPA.md` M7). Achado de lado, ao construir a medição da cauda.

**Onde**: `internal/wa-headless/EVIDENCIA-SPA.md`, seção **M6.2**, cabeçalho da
tabela — a coluna rotulada `janela em OPENING`.

**Problema**: os valores daquela coluna são `CONNECTED − meReadyTriggered`, e
não o tempo que o socket passou no estado `OPENING`. Confere linha a linha na
própria tabela do M6.2: 5,54−5,04 = 0,50; 6,79−6,29 = 0,50; 6,57−6,30 = 0,27.
`meReadyTriggered` é uma **âncora escolhida**, não o estado medido, e o rótulo
não diz que houve escolha.

Medido no 04.3E, com as duas âncoras colhidas na mesma corrida e o mesmo
instrumento (21 boots): a medida DIRETA — primeira amostra em `OPENING` até
`CONNECTED` — é **50–70 ms menor em 18 dos 21 boots**; em três boots de CPU da
rodada 1 a diferença chega a **80, 110 e 280 ms**, e os 280 ms de `cpu-1x` r1 são
**42% da janela daquele boot** (0,66 s) — que é o máximo de todas as pernas de
CPU, ou seja, o número mais carregado da falsificação do eixo CPU. Nenhum boot
ficou em 40 ms: o mínimo observado é 50. Os três *outliers* estão todos onde o
espaçamento observado foi pior, o que é o que se esperaria de um artefato de
resolução e não de uma propriedade das âncoras. Exemplos da corrida, colados do
M7.3 (note que o terceiro é um dos três *outliers*, com 80 ms):

```
unstressed r1   janela(M6) 0,49s   janela(direta) 0,44s
net-heavy  r1   janela(M6) 1,36s   janela(direta) 1,31s
cpu-2x     r1   janela(M6) 0,46s   janela(direta) 0,38s
```

**Nenhuma conclusão do M6 ou do M7 depende da escolha de âncora**, e isso deixou
de ser impressão: o N2a-EVAL re-derivou as duas colunas nos 21 boots e verificou
que **nada vira** sob a âncora direta — a monotonicidade da rede sobrevive
(0,64 → 0,70 → 1,31), o máximo global vai de 1,36 para 1,31 s, e a falsificação
do eixo CPU fica **mais forte**: a margem passa de 0,66/0,73 = 0,90 para
0,49/0,68 = **0,72**. (A justificativa anterior desta linha era a razão de 24×
contra o piso de detecção; ela foi **retirada** porque compara grandezas de eixos
diferentes e não limita coisa nenhuma — ver H12 e `EVIDENCIA-SPA.md` M7.6.)

O defeito é de ROTULAGEM,
e o custo é o de sempre: quem ler só o M6 vai citar "0,27–0,50 s de tempo em
`OPENING`" como se fosse o estado, e a próxima pessoa que medir o estado de
verdade vai achar que encontrou uma divergência.

**Correção sugerida**: renomear a coluna do M6.2 para `CONNECTED − meReady` e
acrescentar uma linha dizendo que é uma âncora, com ponteiro para o M7.2, onde
as duas medidas aparecem lado a lado. **Não** recalcular nem substituir os
números do M6: eles são evidência congelada e continuam corretos para o que
realmente mediram.

**Status**: **não corrigido**. Fora do escopo do 04.3E, que é medição, e a
política do repositório é não corrigir de graça achado fora de escopo sem
perguntar. O M7.2 já registra a ambiguidade e reporta as DUAS âncoras em todas
as 21 corridas, então nenhuma medição nova herda o problema; o que falta é
consertar o rótulo na evidência antiga.

---

## H12 — o guarda do restauro de rede não exercitava o restauro

**Data**: 2026-08-13 · **Contexto**: N2a, aplicação dos fixes exigidos pela
avaliação adversarial do LOOP 04.3E / M7. Achado **do avaliador**, não do
implementador — encontrado por mutação, não por leitura.

**Onde**: `internal/wa-headless/engine/network_test.go`, o antigo
`TestClearNetworkConditionsSendsAnEmptyRuleList`:

```go
func TestClearNetworkConditionsSendsAnEmptyRuleList(t *testing.T) {
	if got := degradedConditions(NetworkDegradation{}); len(got) != 1 {
		t.Fatalf("a degradation is one global rule, got %d", len(got))
	}
}
```

**Problema**: o nome anuncia o contrato de **restauro**; o corpo chama
`degradedConditions` e mede a cardinalidade do caminho de **degradação**.
`ClearNetworkConditions` nunca era chamado, e o caminho de sucesso do restauro
não era exercitado por teste nenhum. É a **ARMADILHA 2** deste repositório em
forma pura — e por isso o achado é de mutação: a suíte estava verde.

Evidência do avaliador (MUTANTE 4): trocar o corpo de `ClearNetworkConditions`
por uma regra de **queda permanente** deixava a suíte **verde**.

**Alcance na época**: contido, porque cada boot do M7 abre browser e aba novos,
então uma limpeza quebrada não vazava para o boot seguinte. Mas o `t.Cleanup` do
`applyDegradation` é o único usuário do contrato, e a próxima reutilização de
aba pagaria a conta.

**Correção aplicada**: extraído `clearedConditions()` em `engine/network.go` —
o **único ponto de construção** da lista do caminho de limpeza, espelhando o que
`degradedConditions` já é para o caminho de degradação — e
`ClearNetworkConditions` passa a chamá-lo. **O comportamento CDP emitido não
muda**: antes passava `nil` a `applyConditions`, agora passa
`clearedConditions()`, que retorna `nil`. O teste passou a se chamar
`TestClearNetworkConditionsClearsWithAnEmptyRuleList` e afirma sobre esse ponto
de construção. A propriedade que o teste antigo de facto media foi preservada,
com o nome certo, em `TestDegradationIsASingleGlobalRule` (que ganhou também a
asserção de que o padrão de URL é o global).

### Controle negativo EXECUTADO

Laboratório isolado (`scratchpad/lab`: cópia de `engine/` + `observability/` com
`go.mod`/`go.sum` do repo). **O repositório não foi mutado.** Linha de base da
cópia: `ok wa-api/internal/wa-headless/engine 3.783s`.

**MUTANTE 4A** — a queda permanente instalada no ponto de construção do caminho
de limpeza, que é onde os MUTANTES 1–3 do avaliador também foram aplicados
(`degradedConditions`, não `SetNetworkDegraded`):

```go
func clearedConditions() []*network.Conditions {
	return []*network.Conditions{{
		URLPattern: allRequests, Offline: true,
		DownloadThroughput: 0, UploadThroughput: 0,
	}}
}
```

Antes da correção este mutante passava **VERDE**. Depois dela, **compila E
falha com mensagem**:

```
--- FAIL: TestClearNetworkConditionsClearsWithAnEmptyRuleList (0.00s)
    network_test.go:90: the restore path built rule 0 of 1: {URLPattern: Latency:0 DownloadThroughput:0 UploadThroughput:0 ConnectionType: PacketLoss:0 PacketQueueLength:0 PacketReordering:false Offline:true}
    network_test.go:92: the restore path built 1 rule(s) and the contract is ZERO: the rule list is replaced wholesale, so an EMPTY list is the only form that leaves nothing behind — anything else is an emulation that outlives the boot that asked for it
FAIL
FAIL	wa-api/internal/wa-headless/engine	0.198s
```

(A mensagem desreferencia os ponteiros de propósito: a primeira versão imprimia
`[0x5b15c256a050]`, que nomeia nada e por isso é meio guarda.)

### O que este guarda NÃO pega, medido e não suposto

**MUTANTE 4B** — a mesma queda permanente escrita **direto no corpo** de
`ClearNetworkConditions`, sem passar por `clearedConditions()`. Continua
**VERDE**:

```
--- PASS: TestClearNetworkConditionsClearsWithAnEmptyRuleList (0.00s)
ok  	wa-api/internal/wa-headless/engine	0.191s
```

Isto **não é uma fraqueza deste guarda em particular**: é uma propriedade de
pacote único em Go, e o guarda da degradação — que a avaliação aceitou — tem
exatamente o mesmo limite. Medido, para não ficar em palavra: reescrever o corpo
de `SetNetworkDegraded` para embutir uma regra com `Offline: true`, desviando de
`degradedConditions`, também passa **verde** (`ok ... 0.193s`), embora o MUTANTE 1
do avaliador — a mesma queda aplicada DENTRO de `degradedConditions` — mate o
teste.

Fechar o desvio exigiria uma costura de observação no código de produção (um
campo em `Tab`, ou uma variável de pacote no lugar de `applyConditions`) para que
o teste pudesse chamar o método real sem browser. Isso é mais largo que o
"pequeno helper extraído" que o pacote do N2a autorizou, e troca uma estrutura de
produção limpa por alcance de teste. **Não foi feito**, e fica aqui nomeado: o
contrato está guardado no ponto de construção, com a mesma força — nem mais, nem
menos — que o contrato da degradação.

**Status**: **corrigido**. Coberto por
`TestClearNetworkConditionsClearsWithAnEmptyRuleList` (contrato de restauro,
controle negativo 4A acima) e `TestDegradationIsASingleGlobalRule` (a
propriedade que o teste antigo media). O desvio do ponto de construção
(MUTANTE 4B) fica registrado acima como limite conhecido e **não** corrigido.

---

## H13 — o F-24 do `EXECUCAO.md` ainda afirma a unidirecionalidade que o M7.5 retirou

**Data**: 2026-08-13 · **Contexto**: N2b (M8, a permanência em `OPENING` sob
corte). Achado ao ler o M7 e o F-24 para saber o que já estava estabelecido
sobre a janela de `OPENING` — **fora do escopo** desta tarefa, que é medição.

**Onde**: `internal/wa-headless/EXECUCAO.md:904-910`, dentro do F-24:

```
**Ressalva de instrumento, e ela é grande.** Na perna `cpu-2x` o pior
espaçamento entre amostras foi de **5,11 s** contra uma janela de 0,46 s — o
tick pior é dez vezes a coisa medida. A falsificação do eixo CPU sobrevive
porque um amostrador grosseiro reporta a transição TARDE e portanto **infla**
a janela: o erro possível aponta ao contrário da conclusão. Não sobrevive por
a resolução ter sido suficiente, e o M7.5 diz isso com as duas direções de
erro.
```

**Problema**: o parágrafo afirma exatamente o que a avaliação adversarial do
N2a derrubou, e depois cita como testemunha o texto que o derrubou. O
`EVIDENCIA-SPA.md` M7.5 e o M7.8 item 4 dizem o oposto, com todas as letras:

> **Não** sobrevive porque o erro apontasse para o outro lado — não aponta: um
> buraco sobre a marca de INÍCIO encolhe a janela, e o texto anterior desta
> linha afirmava uma unidirecionalidade que os dados não dão.

O erro do amostrador **não é unidirecional**: a janela é a diferença de duas
marcas, ambas enviesadas para tarde (`Ŵ = W + δc − δm`), então um buraco sobre o
FIM infla e um buraco sobre o INÍCIO **encolhe**. O que sustenta a falsificação
do eixo CPU é o delta de **80/60/70 ms** entre as duas âncoras nas três corridas
de `cpu-2x`, que prova resolução de ~70 ms **na vizinhança das marcas** — e é
esse número, não o sentido do enviesamento, que o M7.5 usa.

A última frase do parágrafo (*"Não sobrevive por a resolução ter sido
suficiente, e o M7.5 diz isso com as duas direções de erro"*) mostra que a
correção do N2a chegou ao EVIDENCIA e **não** ao diário: o parágrafo tem a
correção colada no fim de uma afirmação que ela contradiz, e as duas ficam de pé
lado a lado. Quem ler só o `EXECUCAO.md` — que é onde se lê "o que já está
estabelecido" — sai com o argumento retirado.

**Gravidade**: documentação, não código. Nenhum número do M7 muda, nenhum teste
depende disto. Mas é o mesmo tipo de defeito que o próprio N2a foi corrigir
(o enquadramento dos 24,4× congelado num comentário onde a próxima pessoa o
leria como assentado), e sobreviveu no arquivo vizinho.

**Correção sugerida**: substituir as duas frases finais do parágrafo pelo
argumento do M7.5 — as duas direções de erro, e o delta entre âncoras de
80/60/70 ms como o que de fato sustenta a falsificação. Uma frase, sem tocar em
número nenhum.

**Status**: **corrigido** pelo Chief, no mesmo ciclo em que o defeito nasceu.

O worker do N2b agiu certo ao registrar e não consertar: para ele o achado era
externo ao escopo, e a política do projeto manda registrar e perguntar em vez de
"consertar de graça". Quem tinha de decidir era o Chief, e a decisão foi
consertar — porque `git log -S` põe o parágrafo no `77722e2`, que é **commit
deste ciclo**. Não era dívida herdada: era defeito recém-introduzido, e terminar
o próprio trabalho não é ampliar escopo.

Não há teste a travar: é prosa de diário, e a política anti-regressão do
`CLAUDE.md` se aplica a achado com defeito de comportamento.

### O mecanismo do engano, que é a parte reutilizável

O defeito não foi um erro de escrita. Foi uma **correção aplicada a um documento
e não ao seu espelho**:

```
avaliação adversarial derruba o argumento da unidirecionalidade
        ↓
fix F3 nomeia o alvo: "EVIDENCIA-SPA.md M7.5 e M7.8 item 4"
        ↓
aplicado exatamente ali  ✓
        ↓
o espelho do MESMO argumento no EXECUCAO.md fica para trás  ✗
```

Três leituras não pegaram: o avaliador (o escopo dele era o M7, não o diário), o
worker de correção (seguiu a lista fechada, corretamente), e o Chief, que
conferiu os sete fixes **exatamente nos `file:line` que a lista nomeava**.
Verificar contra a lista prova que a lista foi cumprida; **não prova que a lista
estava completa**.

Agravante que torna o caso pior que uma omissão: a frase corretiva foi parar no
FIM do mesmo parágrafo, ao lado da afirmação que ela contradiz — e citando como
testemunha (`"e o M7.5 diz isso"`) justamente o texto que a desmente. As duas
ficaram de pé, e quem lesse só o diário — que é onde se lê "o que já está
estabelecido" — sairia com o argumento retirado.

**Regra que sai daqui**: quando uma avaliação derrubar um ARGUMENTO (e não um
número), a correção MUST varrer os espelhos daquele argumento, não só o
documento auditado. O comando é barato — procurar a frase derrubada em todos os
`.md` do módulo — e a ausência dele custou este achado.

## H14 — o elo entre `OpeningWindowThreshold` e a evidência medida existe só em prosa

**Data**: 2026-08-18 · **Contexto**: LOOP 04.4, avaliação adversarial
independente de `internal/wa-headless/spa/socket.go`, arquivo novo que
introduz o limiar `C = 3030ms`.

**Onde**: `internal/wa-headless/spa/socket.go` (comentário de derivação de
`OpeningWindowThreshold`) e `internal/wa-headless/spa/socket_test.go`
(`TestOpeningWindowThresholdMatchesDocumentedTerms`).

**Problema**: o teste que parece travar a derivação de `C` trava, na
verdade, a CONSTANTE contra três literais hardcoded no próprio arquivo de
teste (`healthyUpperBound`, `measurementUncertainty`, `explicitGuardBand`).
Ele não amarra nada ao comentário de `socket.go` nem ao `EVIDENCIA-SPA.md`
M7.3/M7.5.

**Evidência (mutação executada pelo avaliador)**: alterar APENAS o
comentário de derivação — termo 1 de "1.36s" para "9.99s", sem tocar na
constante nem no teste — fez a suíte inteira PASSAR. Já a mutação que altera
só a constante (3030ms → 3000ms) FALHA no teste, com a mensagem "the
constant and its derivation comment have drifted apart".

**Consequência**: alguém pode reescrever a seção M7.3 do `EVIDENCIA-SPA.md`
com outros números, ou editar o comentário de `socket.go`, e nenhum teste
morde. O que está protegido é o drift entre a constante e os literais do
teste; o que não está é a narrativa de derivação em si, nem o vínculo com o
`.md` de onde os números vêm.

**Correção sugerida**: nenhuma automatizável de forma razoável — um teste Go
não faz parsing daquele markdown. O caminho seria um gate fora do Go (script
que extraia os números do `.md` e compare com os do `.go`), e isso é decisão
à parte, não desta capacidade. Registrar a lacuna vale mais que escondê-la.

**Status**: **FECHADO em 2026-08-18, LOOP 05.10** — ver o bloco de fechamento no
fim desta entrada. O histórico abaixo fica porque a entrada foi resolvida em duas
etapas e a segunda só faz sentido lendo a primeira.

**Etapa 1 — PARCIALMENTE CORRIGIDO em 2026-08-18, LOOP 04.5.** O que mudou:
os três termos deixaram de ser literais dentro do arquivo de teste e passaram a
ser **dados nomeados em código de produção** (`healthyUpperBound`,
`measurementUncertainty`, `explicitGuardBand` em `spa/socket.go`), com
`OpeningWindowThreshold` **composto** por eles em vez de comparado a eles. O
teste — agora `TestOpeningWindowThresholdTermsAreLocked` — trava cada termo
contra a sua fonte medida, e a mutação foi executada pelo Chief: alterar
`healthyUpperBound` de 1360 para 1400 ms faz o teste reprovar com
`healthyUpperBound = 1.4s, want 1.36s (M7.3)`.

O que **permanece aberto**, e é o achado original: nenhum teste Go liga esses
valores ao `EVIDENCIA-SPA.md`. Provar automaticamente que prosa humana está
semanticamente correta é impossível, e a direção técnica confirmou que não se
deve criar teste que prometa isso. A diferença é que os números agora vivem em
código executável, não só em comentário — mutar o comentário continua passando,
mas o comentário deixou de ser onde o valor mora.

**CORREÇÃO DESTA PRÓPRIA ENTRADA, 2026-08-18 (LOOP 05.9).** A frase acima
*"um teste Go não faz parsing daquele markdown"* — que eu escrevi — está
**errada**, e a distinção que ela perde é justamente a que importa.

Provar que a NARRATIVA está semanticamente correta é de fato impossível. Mas
não é isso que faltava: faltava travar que o VALOR citado no `.md` e o valor
da constante em `.go` continuam sendo o mesmo número. E isso um teste Go faz,
porque o número tem âncora estável — `EVIDENCIA-SPA.md:1331` é uma linha de
tabela:

    | pior janela de boot saudável medida (21 boots, 7 condições) | **1,36 s** |

Ler o arquivo e afirmar que `healthyUpperBound` bate com o número dessa linha
não promete nada sobre a prosa; promete exatamente uma coisa verdadeira e
verificável — **que os dois lados não divergiram em silêncio**, que é o achado
original desta entrada.

**E o desenho, MEDIDO antes de ser prometido** — porque "os três números ligam
ao `.md`" era a minha segunda afirmação apressada sobre esta entrada, e ela
também não sobreviveu ao contato. São **dois links e uma igualdade**, coisas
diferentes:

| termo | valor | o que dá para travar | âncora |
|---|---|---|---|
| `healthyUpperBound` | 1,36 s | link com o `.md` | linha `pior janela de boot saudável medida`, **1 ocorrência** |
| `measurementUncertainty` | 0,31 s | link com o `.md` | linha de tabela `` | `net-*` | `` do M7.5, **1 ocorrência**, tomando o TOPO da faixa `0,30–0,31` — que é o que o `socket.go` declara fazer ("the worse of the two") |
| `explicitGuardBand` | 1,36 s | **nada no `.md`** | é *"an explicit engineering choice, not a further measurement"*, declarada ancorada ao termo 1. O travável é `explicitGuardBand == healthyUpperBound`, invariante de CÓDIGO |

As duas âncoras foram verificadas por extração real (uma ocorrência cada, número
extraído batendo com a constante). O termo 3 não tem o que ancorar por
construção — e um teste que fingisse ancorá-lo seria exatamente o tipo de
promessa vazia que a F-28 ensinou a não fazer.

Não implementado neste ciclo: a tarefa autorizada no momento era a medição de
retenção longa (token `HOLD`), e abrir uma segunda frente no meio de uma
medição é como se perde a rastreabilidade. Registrado com o desenho pronto,
as âncoras verificadas e o limite dele declarado.

**Status revisto**: parcialmente corrigido, com a lacuna restante agora sabida
**fechável** — ao contrário do que esta entrada afirmava.

**FECHADO em 2026-08-18 (LOOP 05.10)**, com autorização da orquestração (token
`H14`). Dois testes em `spa/socket_test.go`:

- `TestMeasuredTermsMatchTheEvidenceDocument` — lê o `EVIDENCIA-SPA.md`, exige
  que cada âncora case com **exatamente uma** linha, e compara o número contra a
  constante de produção. A unicidade da âncora é parte do teste, não higiene:
  âncora que passa a casar duas linhas parou de identificar uma medição.
- `TestGuardBandIsAnchoredToTermOne` — o termo 3 não tem fonte no documento, e o
  travável é a identidade de CÓDIGO `explicitGuardBand == healthyUpperBound`.

**Um erro de desenho meu, pego pelo próprio teste na primeira execução.** A
regra inicial era "a última duração da LINHA". Ela lê `1,36 s` de
`` | `net-*` | 0,30–0,31 s | 0,49–1,36 s | `` — a coluna da JANELA MEDIDA, não a
do espaçamento do instrumento, de onde o termo vem. Pior que não achar nada:
pegaria um número real da medição errada. Corrigido com seleção de célula
(`evidenceAnchor.cell`), e o porquê está escrito no campo.

O protótipo que eu havia rodado antes de prometer devolvia `['0.31','1.36']` e eu
li como confirmação **porque o 0,31 estava lá** — sem verificar qual dos dois a
regra escolheria. Medir e depois ler a medição com a hipótese na cabeça é a
mesma falha que o `CLAUDE.md` descreve: *se a medição só confirmou o que você já
achava, provavelmente ela não mediu nada*.

**Quatro controles negativos, executados, e os três primeiros FALHARAM na
primeira tentativa** — pela armadilha 3, silêncio não é prova. O `replace` com
`count=1` acertava a primeira ocorrência de `**1,36 s**` no documento, que é
outra linha, então a mutação nem tocava a âncora. Refeitos por índice de linha:

- reescrever o M7.3 (`1,36 → 9,99`): *"healthyUpperBound = 1.36s in code, but
  M7.3 ... says 9.99s"* — **este é o achado ORIGINAL desta entrada**, e agora
  morde.
- reescrever a coluna do M7.5 (`0,31 → 0,99`): *"measurementUncertainty = 310ms
  in code, but M7.5 ... says 990ms"*.
- duplicar a linha-âncora: *"matched 2 lines ..., want exactly 1"*.
- desancorar o termo 3 (`1360 → 1500 ms`): *"the band is a number with no stated
  derivation behind it"*.

Documento e `socket.go` restaurados byte-idênticos após cada mutação.

**O que estes testes NÃO provam**, dito aqui para não virar citação errada: que
a PROSA está correta. Isso não é mecanicamente verificável e a direção técnica
está certa em recusar teste que prometa isso. O que eles provam é a coisa
estreita e verdadeira: **o valor citado no documento e o valor na constante não
divergiram em silêncio**.

## H15 — o envelope de validade de `C` não tem expressão executável

**Data**: 2026-08-18 · **Contexto**: LOOP 04.4, avaliação adversarial
independente de `internal/wa-headless/spa/socket.go`, mesma revisão da H14.

**Onde**: `internal/wa-headless/spa/socket.go`, comentário de
`OpeningWindowThreshold` (seção "VALIDITY ENVELOPE") e a função
`ClassifyOpeningDuration`.

**Problema**: o comentário declara que `C` só vale até 900ms de latência
ADICIONADA — o teto da curva de três pontos do M7 — e que além disso a
resposta é UNKNOWN. Mas `ClassifyOpeningDuration` recebe apenas uma
`time.Duration` crua e não tem nenhuma noção de latência adicionada.
Operacionalmente, um enlace real acima de 900ms de RTT adicionado produz
exatamente o mesmo `SocketHealthy`/`SocketDegraded` de qualquer outro, sem
nenhum sinal de que a garantia por trás de `C` deixou de valer ali.

**Evidência**: verificado por leitura do código pelo avaliador —
`ClassifyOpeningDuration` não recebe, não mede e não consulta latência em
lugar nenhum; sua assinatura é `func ClassifyOpeningDuration(d
time.Duration) SocketLiveness`. O envelope é 100% prosa.

**Correção sugerida**: quando existir consumidor (CAP-06), o caminho de
liveness precisaria conhecer a latência observada para saber que está fora
do envelope, e reportar isso em vez de devolver um veredito com confiança
que não tem. Enquanto não houver consumidor, não há o que corrigir.

**Status**: **não corrigido nesta sessão**. Lacuna conhecida e declarada.

## H16 — a lista de pontos não fecha o meio do domínio, e isso está escrito no teste

**Data**: 2026-08-18 · **Contexto**: errata da F-28, depois da CAP-04 fechada.

**Onde**: `internal/wa-headless/spa/socket_test.go`,
`TestClassifyOpeningDurationSampledPointsMapToTwoValues`.

**Problema**: o teste percorre uma **lista finita de 11 pontos**, não o domínio.
`time.Duration` é `int64` de nanossegundos — ~1,8 × 10^19 valores representáveis.
Fechar as pontas (0, `C±1ns`, `math.MaxInt64`) não fecha o **meio**: uma mutação
que dispare estritamente entre dois pontos amostrados escapa.

**Evidência (mutação executada, e reproduzida pelo Chief)**: com
`if d > 48h && d < 72h { return SocketLiveness("MUT_ENTRE") }` no classificador,
a suíte inteira **PASSA**. Já a mutação acima do antigo máximo
(`d > 400*24h`) agora **FALHA**, pega pelo ponto `math.MaxInt64` — essa metade
foi fechada por esta errata.

**Por que não foi corrigido**: fechar a propriedade exigiria varredura real ou
teste de propriedade, e a instrução da orquestração vetou introduzir framework
de property testing nesta errata. Listar cada fronteira de intervalo à mão não
escala.

**O que MITIGA**: a garantia central não é do teste, é **estrutural** —
`ClassifyOpeningDuration` tem exatamente dois `return`, ambos devolvendo uma das
duas constantes declaradas, e não existe terceira constante para devolver. Um
terceiro ramo é diff visível em `socket.go`, não algo que o teste prove ausente
rodando. Isso está escrito no próprio arquivo de teste, sob o título
"WHAT IS GUARANTEED BY TESTS VS BY STRUCTURE".

**Status**: **não corrigido, e declarado no lugar onde engana** — o nome do
teste deixou de prometer exaustão, e o comentário nomeia a fuga que permanece.
É a lição da F-28 aplicada ao conserto da própria F-28: o nome não pode
prometer o que o corpo não executa.

## H17 — o ponteiro do futuro consumidor de `C` está obsoleto no próprio código

**Data**: 2026-08-18 · **Contexto**: trace canônico da CAP-05, pedido pela
orquestração antes de implementar.

**Onde**: `internal/wa-headless/spa/socket.go:100`.

**Problema**: o comentário do tipo `SocketLiveness` diz que a taxonomia completa
"lives in EVIDENCIA-SPA.md and in **CAP-06's still-unopened scope** (the caller
that tracks history and combines axes)". A CAP-06 fechou, e ela era outra coisa:
**inventário de módulos do SPA**. Ela não tem — e não deveria ter — nada que
combine eixos de liveness ou guarde histórico de sessão. O ponteiro aponta para
um lugar que existe e não é aquele.

**Como apareceu**: eu afirmei à orquestração que "o consumidor de `C`" era um
dos adiamentos que apontavam para a CAP-05. Ela mandou não contar isso sem
confirmar no contrato canônico. Ao conferir, descobri duas coisas: a minha
afirmação era inferência, e a fonte escrita dizia **outra coisa ainda** — CAP-06.

**O que o contrato canônico realmente diz**: a CAP-05 é *"pareamento por QR,
restauração, desligamento limpo, reclaim de `Singleton`"* (`HANDOFF §10`). Ela
**não menciona** consumir veredito de degradação. Por outro lado, `core/doc.go`
— que é a casa da CAP-05 — declara que o pacote *"holds the state that outlives
a single command"* e carrega *"heartbeat renews, TTL covers abrupt death"*.
Estado que sobrevive a um comando é exatamente a pré-condição que o consumidor
de `C` precisa (*"has this socket ever reached CONNECTED? was the previous
classification DEGRADED?"*).

**Conclusão honesta**: `core/` é o único lugar canônico que satisfaz a
pré-condição, mas **nenhum contrato atribui a consumação de `C` a ele**. Isso é
diferente de "é ali". O consumidor verdadeiro segue **INDETERMINADO**.

**Correção sugerida**: quando a CAP-05 existir e o seu call graph estiver
escrito, decidir por evidência onde `C` é consumido — e só então corrigir o
ponteiro de `socket.go:100`. Não corrigi agora porque trocaria um palpite errado
por outro palpite: apontar para a CAP-05 sem prova repetiria exatamente o erro
que esta entrada registra.

**Status**: **CORRIGIDO em 2026-08-18 (LOOP 05.4)**.

A pré-condição desta entrada — *"quando a CAP-05 existir e o seu call graph
estiver escrito"* — foi satisfeita. E a resposta veio por medição, não por
palpite: `grep` por `Socket|Liveness|ClassifyOpening` em `internal/wa-headless/core/`
retorna **zero ocorrências**. A CAP-05 que existe tem a pré-condição certa
(`core/doc.go`: *"holds the state that outlives a single command"*) e **não é**
a consumidora.

O ponteiro foi corrigido para dizer o que se sabe e parar aí: a CAP-06 estava
errada duas vezes (fechou, e era inventário de módulos), a CAP-05 não é, e o
consumidor segue **INDETERMINADO**. Nenhum destino novo foi nomeado — nomear um
sem call site repetiria exatamente o erro que esta entrada registra.

**Achado incidental, corrigido no mesmo bloco**: o comentário ainda carregava a
afirmação exagerada da **F-28** — *"makes that impossible to violate by
construction, not just by policy"*. A orquestração já tinha julgado isso
(*"NOT IN DECLARED VOCABULARY != UNREPRESENTABLE BY THE TYPE SYSTEM"*), mas a
correção fora aplicada às minhas falas e **não ao código**. Agora o comentário
diz "impossible to express in the DECLARED VOCABULARY", registra a distinção e
nomeia quem de fato guarda a propriedade:
`TestClassifyOpeningDurationExhaustsToTwoValues`.

## H18 — `BootFailure` não carrega o PID do browser que não conseguiu subir

**Data**: 2026-08-18 · **Contexto**: CAP-05, levantado pelo próprio executor.

**Onde**: `internal/wa-headless/core/session.go`, tipo `BootFailure`.

**Problema**: em qualquer caminho de falha, `StartSession` não devolve `*Session`
— por desenho, um boot que falhou não tem nada vivo para entregar. Logo não há
PID. Os testes de teardown provam "sem processo órfão" pelo artefato
`SingletonLock`, não por PID recuperado. Isso basta para o que a CAP-05 precisa,
mas um consumidor futuro de métrica/alerta que queira identificar o processo que
morreu não tem por onde.

**Correção sugerida**: acrescentar `PID int` ao `BootFailure`. Mudança estreita e
aditiva — não feita porque nada neste escopo precisou dela.

**Status**: ~~não corrigido, aguardando consumidor que justifique.~~
**CORRIGIDO em 2026-08-20 (LOOP 06.9)**.

`BootFailure` ganha `PID int`, populado no `fail()` e renderizado no `Error()`.
Zero é **omitido**, não impresso: `pid=0` lê como um id de processo real para
quem faz `grep`.

**A TENSÃO COM O H23, resolvida por USO e não por gosto.** O H23 **recusou**
devolver um pid depois que ele deixa de significar algo, porque sistemas
operacionais reusam pids e agir sobre um obsoleto alcança o processo que herdou
o número. Este campo é o mesmo número com propósito oposto: **obsoleto por
construção** — um `BootFailure` só existe depois de o browser ter sido
derrubado — e existe para **correlacionar** a falha com o registro do sistema
operacional. Correlação não é ação. Está escrito no campo.

**Teste sem subir browser**, como a orquestração pediu:
`TestBootFailurePIDIsRenderedForCorrelation`. `BootFailure` é um VALOR, e o que
ele renderiza é propriedade do valor, não do boot que o produziu — um teste que
falhasse um boot real dependeria de qual estágio quebrou e exercitaria a
formatação só por acidente. Controle negativo executado: imprimir `pid=0` faz
falhar.

### Dois controles que NÃO morderam, e o que fiz com cada um

**A ordem de leitura do pid.** Escrevi que ler antes do `CleanStop` era carga.
O controle que inverteu **PASSOU** — `Browser.PID()` devolve o mesmo número
depois da parada, que é exatamente o que o H23 mediu. Comentário corrigido para
dizer que a ordem é legibilidade, não correção. **Segunda vez no mesmo dia** que
escrevo "a ordem importa" sem verificar.

**A guarda da fronteira com o H23 foi REMOVIDA.** Eu tinha escrito um teste
varrendo `session.go` por `ProcessAlive(pid)`, `syscall.Kill` e `Signal(`, para
garantir que nada age sobre o campo. Uma mutação com `engine.ProcessAlive(e.PID)`
passou direto: a grafia diferia. E alargar não é opção — `session.go`
LEGITIMAMENTE chama `engine.ProcessAlive` para o `Session.ProcessAlive` (H21),
então o identificador não pode ser proibido, e texto não distingue "pid da
sessão viva" de "pid obsoleto do BootFailure". É o H32 outra vez.

Removi em vez de manter. **Teste que não morde é pior que teste nenhum**: é
garantia falsa, e achar este silencioso foi sorte. A fronteira passa a ser
mantida pelo comentário e pela revisão, com essa limitação dita no lugar onde o
teste estava.

## H19 — a sequência ler-e-limpar da marca de suspeita não tem teste próprio

**Data**: 2026-08-18 · **Contexto**: CAP-05, levantado pelo próprio executor.

**Onde**: `internal/wa-headless/core/session.go` — `engine.SessionSuspect` na
entrada do boot e `engine.ClearSessionSuspect` depois do sucesso.

**Problema**: `engine/suspect_test.go` cobre o mecanismo da marca **isolado**, e
a CAP-05 agora o compõe no boot. Mas todos os testes desta fatia partem de perfis
`t.TempDir()` recém-criados, que **nunca** foram marcados suspeitos — `wasSuspect`
deu `false` em todos. Ou seja: o caminho composto ler→limpar nunca foi exercitado
a partir de um perfil de fato suspeito.

**Consequência**: se `ClearSessionSuspect` deixasse de rodar no sucesso, ou se a
leitura passasse a devolver `false` por engano, nenhum teste desta fatia morde. A
invariante 2 diz que a sessão deve ser **verificada** no boot seguinte a uma
parada suja; a verificação existe, a prova de que ela roda no caminho composto,
não.

**Correção sugerida**: fixture que pré-grava `.wa-headless-session-suspect` e
prova que o boot lê, e que um boot bem-sucedido limpa.

**Status**: **CORRIGIDA em 2026-08-18**, na fatia de fechamento da CAP-05A.
Dois testes novos partem de um perfil primado com a marca de produção
(`engine.MarkSessionSuspect`, não escrita à mão): um prova que o boot lê e que o
sucesso limpa; o outro prova que um boot que FALHA **não** limpa. A mutação
exigida — mover `ClearSessionSuspect` para antes do `Launch` — faz o segundo
falhar, e foi reproduzida pelo Chief. Nada em produção mudou: a posição do clear
já estava certa; faltava a prova.

## H20 — a suíte do `core` ficou 5× mais lenta, e a causa é o conserto certo

**Data**: 2026-08-18 · **Contexto**: LOOP 05.2, medido depois da correção.

**Onde**: `internal/wa-headless/core/` — em especial
`TestStartSession_SuspectMarker_NotClearedOnFailedBoot` e os testes cuja página
nunca monta.

**Problema**: `go test -race -count=1 ./internal/wa-headless/core/...` foi de
**28 s para 152 s**. A causa é correta e esperada: antes, uma página que não
estava pronta falhava no primeiro probe; agora o laço de assentamento espera o
`DefaultSettleBudget` inteiro (60 s) antes de desistir. Um teste que prova
"nunca fica pronto" **precisa** pagar o orçamento para provar isso.

**Por que registrar mesmo sendo correto**: suíte lenta é suíte que se pula.
152 s no pacote mais central do módulo é o tipo de custo que, acumulado, faz
alguém rodar `-run` seletivo e deixar de ver regressão.

**Correção sugerida**: os testes de caminho negativo podem injetar um orçamento
menor em vez de usar o default — `spa.WaitForReady` já recebe `budget` como
parâmetro, então é questão de o teste passar um valor curto e afirmar que o
laço respeita o orçamento **recebido**, o que aliás é uma propriedade melhor de
travar do que "espera 60 s". Não aplicado nesta sessão: mexer nos testes de
regressão logo depois de eles terem acabado de validar uma correção é
exatamente quando se introduz um defeito sem perceber.

**Status**: **CORRIGIDO em 2026-08-18 (LOOP 05.5)**, pela correção desenhada
acima. O motivo do adiamento — *"mexer nos testes de regressão logo depois de
eles terem acabado de validar uma correção"* — expirou: os testes já validaram
dois ciclos de correção desde então, e o custo tinha PIORADO (152 s na medição
original, **201 s** medidos agora antes de mexer).

**O que mudou**: `StartConfig` ganha `SettleBudget`, com zero significando
`spa.DefaultSettleBudget`. Os dois testes de caminho negativo que pagavam o
orçamento inteiro — `FailureTearsDownDeterministicallyWithNoOrphan` e
`SuspectMarker_NotClearedOnFailedBoot`, 61 s cada, 122 s dos 201 s — passam a
receber `negativePathSettleBudget = 3s`. A duração da espera era incidental a
ambos: um prova desmontagem sem órfão, o outro prova que a marca sobrevive.

**Medição**: pacote `core` de **201,2 s para 98,4 s** (−51%), mesma máquina,
mesma sessão. Suíte completa verde sob `-race`.

**A propriedade melhor, travada**:
`TestStartSession_SettleLoopRespectsTheBudgetItWasGiven`. "O boot espera 60 s" é
um fato sobre um DEFAULT; "o boot desiste no orçamento que recebeu" é um fato
sobre o MECANISMO, e continua significando algo quando o default mudar. Sem ele,
tornar o orçamento configurável seria só aceleração, com nada afirmando que o
botão está ligado.

**Controles negativos, executados, cada um mordendo a SUA asserção** — o teste
tem limite superior e inferior de propósito, porque uma regressão de tiro único
também terminaria bem abaixo do default e passaria só com o limite superior:
- ignorar `cfg.SettleBudget`: *"boot took 1m1.72s, at or beyond the 1m0s DEFAULT
  budget despite being given 4s — SettleBudget is not reaching the settle loop"*.
- voltar ao tiro único: *"boot gave up after 1.67s, sooner than the 4s budget it
  was given — the settle loop is not waiting out its budget"*.

`session.go` restaurado byte-idêntico após cada mutação.

## H21 — o detentor não tem como perguntar se a sessão ainda está viva

**Data**: 2026-08-18 · **Contexto**: LOOP 05.6, ao construir o `runtime.Holder`
com WRITE_SET restrito a `runtime/` (`core/` fora de escopo por decisão da
orquestração).

**Onde**: API pública de `internal/wa-headless/core` — `Session` expõe
`Browser()`, `Tab()`, `ProfileDir()` e `Stop()`, e nada mais.

**Problema**: o `Holder` guarda uma sessão entre comandos e a devolve a quem
pedir. Ele **não tem como saber se ela ainda está utilizável**. Se o browser
morrer sozinho — crash, OOM, morte externa — o `Holder` continua entregando o
mesmo ponteiro, e o comando seguinte descobre o problema como um erro de CDP no
meio de uma operação de negócio, não como uma sessão inválida.

Isto **não é hipotético para este módulo**: `engine.ProcessAlive` existe e o
teste de N ciclos já o usa para caçar órfãos. O sinal existe; o que não existe
é um caminho pela API do `core` para o detentor consultá-lo sem alcançar
`Browser().PID()` e reimplementar a política por fora — que é exatamente o tipo
de conhecimento vazado que o item 4 do briefing separa entre as camadas.

**Por que não corrigi**: `core/` está fora do WRITE_SET deste ciclo (resposta
`SORUNTIME` da orquestração). Fazer o `Holder` sondar `Browser().PID()` por
conta própria seria contornar a restrição pela porta dos fundos e colocar
política de liveness na camada errada — e a liveness deste módulo já tem uma
lição cara sobre isso (item 12 do briefing: processo, alvo, service worker,
socket, SPA, sessão e identidade são sinais DIFERENTES).

**Correção sugerida**: `core.Session` ganhar uma consulta de liveness cuja
semântica seja declarada — provavelmente sobre o PROCESSO, o sinal mais barato e
menos ambíguo — com nome que não prometa mais do que mede (`ProcessAlive`, não
`Healthy`). Um detentor então decide o que fazer; a política fica nele, o sinal
fica no `core`.

**Status**: **CORRIGIDO em 2026-08-18 (LOOP 05.8)**, depois de a orquestração
autorizar `core/` no WRITE_SET (token `H21`).

**O sinal, no `core`**: `Session.ProcessAlive()`. O nome é a coisa verdadeira
mais estreita que cabia, de propósito — não `Healthy`, não `Alive`. O item 12 do
briefing é o motivo: processo, alvo, service worker, socket, SPA, sessão e
identidade são sinais DIFERENTES, e este módulo já pagou caro por confundi-los.
Um processo vivo ainda não diz nada sobre o socket, a SPA ou a identidade. O que
ele dá é o negativo barato e sólido: **processo morto significa que tudo acima
dele também morreu**.

**A política, no detentor**: `Holder.Session` pergunta antes de entregar e
**recusa** com `ErrSessionDied`. Recusar em vez de re-bootar é decisão, não
omissão — re-bootar em silêncio esconderia um browser que morre sempre, virando
vazamento lento em vez de falha alta (ADR-0005 D7), e "quantos browsers este
perfil já teve" passaria a depender de sorte. Se uma sessão morta deve ser
substituída automaticamente é decisão de PRODUTO, e este pacote não a toma
sozinho.

**O teste é construído para o detector poder FALHAR** (item 15 do briefing): o
browser é morto **de fora** com `SIGKILL`, do jeito que um crash ou um OOM
chegam, então toda a contabilidade em processo continua dizendo que a sessão
está boa. Um detector que respondesse sempre "vivo" passaria em todos os outros
testes do arquivo e falharia só neste.

**Controles negativos, executados, ambos mordendo com a mesma mensagem**:
- `ProcessAlive` mentindo (`return true`): *"the Holder handed out a session
  whose process (pid 70487) is gone, with err=<nil>; want ErrSessionDied"*.
- checagem removida do `Holder`: idem, pid 70609.

**Precisão sobre o que a mutação D NÃO pegou**: ela não derrubou
`TestHolder_ProcessAliveIsFalseAfterStop`, porque a guarda de `stopped` retorna
antes de alcançar a linha mutada. São mecanismos distintos — o teste do `Stop`
trava a guarda, não a sonda — e registro assim em vez de contar como cobertura
que não é.

**Testes**: `TestHolder_RefusesAHeldSessionWhoseProcessDied`,
`TestHolder_ProcessAliveIsFalseAfterStop`. Ambos arquivos restaurados
byte-idênticos após cada mutação.

## H22 — commitei com o portão de módulo vermelho, por ter rodado só testes seletivos

**Data**: 2026-08-19 · **Contexto**: ferramentas de QR (LOOP 06.0), descoberto
ao rodar a suíte inteira antes do commit da `capabilities/liveness`.

**Onde**: `internal/wa-headless/realspa_test.go`, commit `4969834`.

**Problema**: as duas ferramentas de QR importavam `github.com/chromedp/chromedp`
para capturar a tela. A ADR-0006 D1 reserva o driver ao `engine/`, e o
`TestOnlyTheEngineImportsTheDriver` verifica isso — **inclusive nos arquivos de
teste**. O commit `4969834` foi feito com esse portão VERMELHO.

**Como passou**: rodei só `-run TestRealSPACaptureQRCode` e
`-run TestRealSPALiveQR` para validar as ferramentas, e não rodei a suíte. É
literalmente o risco que o **H20** descreve — *"suíte lenta é suíte que se
pula"* — mordendo em quem o escreveu, no dia seguinte a tê-lo fechado.

**Correção**: `Tab.Screenshot(runner, label) ([]byte, error)` passa a existir no
`engine/`, que é onde o `chromedp` mora. As ferramentas de QR chamam a
primitiva. Não abri exceção no portão: o portão estava certo, o código é que
estava no lugar errado.

A primitiva ganhou duas coisas que o código embutido não tinha: captura vazia
vira ERRO explícito (o `chromedp` não reporta erro quando o alvo some no meio da
captura, então o vazio É o sinal), e um comentário dizendo que a imagem é
**conteúdo de página** — diferente de tudo o mais que o `engine/` devolve, que é
classe, estado, duração ou pid. Numa tela de pareamento ela é credencial.

**Verificado**: portão verde, suíte completa verde sob `-race`, e a ferramenta
de captura segue funcionando (falha nomeando o perfil pareado, que é o
comportamento correto hoje).

**Regra que fica**: `-run` seletivo valida a mudança, **não autoriza o commit**.
O que autoriza é a suíte.

**Status**: **CORRIGIDO em 2026-08-19**. `Tab.Screenshot` passou para o `engine/`,
o portão voltou ao verde e a suíte completa está verde sob `-race`.

*(Esta linha faltava. A entrada foi escrita sem status nenhum, e o
`TestHousekeepEntriesAreMachineReadable` — acrescentado no mesmo dia — é o que
não deixa isso passar de novo.)*

## H23 — `engine.Browser.PID()` devolve o mesmo número depois da parada, e o SO reusa PIDs

**Data**: 2026-08-19 · **Contexto**: `getBrowserPid`, quarta capacidade de
paridade — que a matriz descreve como *"trivial dado o CAP-02"*.

**Onde**: `internal/wa-headless/engine/browser.go`, `Browser.PID()`.

**Medido antes de projetar** (a capacidade parecia um getter, e a medição é o
que mostrou que não era):

```
ANTES do Stop:  pid=10819 alive=true
DEPOIS do Stop: pid=10819 alive=false   (via=browser.close)
o PID mudou? false
```

**Problema**: o número sobrevive à sessão. Ele não está *errado* — é o que o
browser foi — mas entregá-lo a um supervisor é: sistemas operacionais **reusam
PIDs**, então um pid guardado e sinalizado depois pode alcançar qualquer
processo que tenha herdado o número. E o consumidor previsto desta capacidade é
exatamente um supervisor que registra o processo (`PARIDADE-WWEBJS.md` §3).

É a mesma falha que este módulo vem encontrando com outras roupas: **um valor
que não se distingue de um válido**.

**Correção**: `runtime.Holder.BrowserPID()` só responde enquanto o número
significa algo, e as três recusas são distintas — `ErrNoSession` (nunca subiu),
`ErrHolderStopped` (parado), `ErrSessionDied` (processo morto). Não devolve o
pid obsoleto ao lado do erro: um chamador que escreve `"pid=%d err=%v"` poria um
número reusável no registro ao lado de uma mensagem que ninguém relê.

**Não corrigido no `engine/`**, e isso é deliberado: `Browser.PID()` está certo
no que faz — reportar o pid do processo que ele lançou. Quem tem contexto para
saber se o número ainda vale é quem SEGURA a sessão. Pôr a política no `engine/`
seria a camada errada, pelo mesmo argumento do H21.

**Testes**: `TestBrowserPIDIsRefusedOnceItIsMeaningless` (nunca iniciado, morto
por `SIGKILL` de fora, parado) e `TestBrowserPIDAfterCleanStopIsRefusedToo`, que
cobre o caminho ORDINÁRIO — o teste do `SIGKILL` sozinho não cobriria a parada
limpa, que é o caso comum.

**Controle negativo executado**: a versão "trivial" — `return
h.session.Browser().PID(), nil` — falha com *"after the process died: pid=14835
err=<nil>, want ErrSessionDied ... a supervisor storing that number would later
signal whatever inherited it"*. Restaurado byte-idêntico.

**Status**: CORRIGIDO.

## H24 — o nome do evento da coleção de mensagens é herdado do `wwebjs` e **não verificado**

**Data**: 2026-08-19 · **Contexto**: `onMessageMeta`, quinta capacidade de
paridade.

**Onde**: `internal/wa-headless/capabilities/messagemeta/messagemeta.go`,
constante `eventAdd`.

**Problema**: a assinatura escuta `MsgCollection.on('add', ...)`. O nome `add`
vem do **entendimento** do `whatsapp-web.js`, não de medição contra este build.
Tudo o mais nesta capacidade foi medido — o módulo, a coleção, a superfície de
eventos, a forma do modelo, o `_serialized` nulo — **menos isto**.

**Por que não foi medido**: verificar exige uma mensagem CHEGAR, e nenhum teste
pode causar isso sem enviar uma. Enviar está fora do escopo desta capacidade e
seria mudar o que o ciclo faz para poder testá-lo.

**A ambiguidade que isto deixa, dita para ninguém ler silêncio como prova**: uma
assinatura que instala limpo e nunca entrega é **indistinguível** de uma conta
que ninguém está mensageando. `TestRealSPAMessageMetaInstalls` prova que ela
ANEXA — não que dispara.

**O instrumento existe**: `TestRealSPAMessageMetaDelivery`, atrás do portão
`WA_HEADLESS_MSG_DELIVERY`, observa por 3 minutos enquanto um humano envia uma
mensagem para a conta de laboratório. Ele distingue os dois casos que a ausência
de evento funde, e a mensagem de falha diz isso em vez de culpar o código.

**Status**: **VERIFICADO em 2026-08-19**, com o humano enviando a mensagem.

```
DELIVERED after 54s: Meta(jid=<redacted> id=3A7D... dir=out type=chat
                          at=2026-08-19T22:39:11Z)
(ignored 58 replayed history event(s) along the way)
```

O nome `add` **dispara em entrega ao vivo** neste build, e o mapeamento
sobrevive: `id.id` (o campo que a `PARIDADE §6.4` escolheu no lugar do
`_serialized` nulo), `type`, `timestamp` e `jid` chegaram todos preenchidos.

**A primeira corrida deu FALSO POSITIVO** e está registrada no `ARMADILHAS.md`:
o teste aceitava qualquer evento, e a coleção emite `add` ao repor histórico —
passou contra uma imagem de 19h46m atrás, e teria passado sem envio nenhum. A
correção foi tornar o **frescor** o discriminador.

**O que fica verificado ao vivo, e o que não**: a direção **`out`** foi
observada ao vivo (a conta de laboratório está vinculada ao telefone do humano,
então o que ele envia aparece como saída no dispositivo vinculado). A direção
**`in`** foi exercitada apenas contra histórico reposto — que é dado real da
página, não um dublê, mas não é entrega ao vivo. Registrado como está em vez de
somado.

## H25 — o buffer da assinatura pode descartar, e o teto não tem medição por trás

**Data**: 2026-08-19 · **Contexto**: mesma capacidade.

**Onde**: `DefaultBufferSize = 500` em `messagemeta.go`.

**Problema**: é um recurso LIMITADO, e a regra 1 do `CLAUDE.md` exige inventário
de quem o disputa e o pior caso de cada um. Aqui o inventário é curto — só o
handler da coleção escreve, só o `Drain` lê — mas o teto **não sai de medição**:
não existe número de taxa real de mensagens para esta conta.

**O que torna isso aceitável, e é a única coisa que torna**: exceder o teto é
**CONTADO e reportado**, não silencioso. `Drain.Dropped` diz quantos foram
recusados, `Drain.Seen` dá o denominador que torna o número legível, e
`Drain.Complete()` é falso quando há buraco. Um fluxo que perde em silêncio
parece completo — é isso que seria pior que não ter fluxo.

**Regra 2 aplicada** (medir onde deveria PIORAR): o cenário em que o mecanismo
cobra o preço é conta de alto volume com `Drain` esparso. **Não medido** — não
há conta assim disponível. Declarado em vez de estimado.

**Correção sugerida**: quando houver conta com volume real, medir eventos por
minuto e o intervalo de drenagem do produto, e só então trocar 500 por um
número com medição atrás. Até lá o teto é um teto declarado, não calibrado.

**MEDIDO EM 2026-08-20, e o cenário apareceu sozinho.** Esta entrada dizia que
o pior caso — "conta de alto volume com `Drain` esparso" — **não fora medido**
porque não havia conta assim disponível. Havia: a **sincronização de histórico
de um perfil recém-pareado**.

Na primeira corrida de retenção sob carga contra o laboratório re-pareado, com
`Drain` a cada 60 s:

```
t+0s     drain=0    drop=0
t+1m0s   drain=500  drop=37     <- o buffer ENCHEU
t+2m0s   drain=2    drop=0
```

`drain=500` é exatamente o `DefaultBufferSize`. O buffer saturou e **37 eventos
foram recusados**, num intervalo de 60 s, sem conta de alto volume nenhuma — só
o WhatsApp repondo histórico depois do pareamento.

**O que isso confirma e o que não confirma.** Confirma que o teto é alcançável
em condição ordinária, e que o descarte contado era a decisão certa: sem
`Dropped`, esses 37 sumiriam em silêncio e o fluxo pareceria completo. **Não**
confirma que 500 é errado — o pico é transitório e some na amostra seguinte.

**O que a medição muda no problema**: o pior caso deixa de ser hipotético e
passa a ter forma conhecida — **rajada logo após o pareamento**, não volume
sustentado. Isso importa porque as duas pedem remédios diferentes: rajada pede
buffer maior ou drenagem mais frequente **na janela inicial**; volume sustentado
pediria repensar o mecanismo.

**Correção sugerida, revista**: drenar mais rápido enquanto a sessão é nova, ou
dimensionar o teto pela rajada de sincronização medida — não por um número de
mensagens por minuto que ninguém tem.

**REPRODUZIDO**: a segunda corrida deu `drain=500 drop=13` no mesmo instante
(t+1m). Duas observações independentes, então a rajada é característica do
pareamento e não acaso de uma execução.

**A PERGUNTA QUE ISTO ABRE, e que eu NÃO respondi**: os eventos descartados são
**perdidos** ou são **reposição de histórico** que ninguém queria?

A rajada acontece exatamente quando o WhatsApp repõe o histórico, e a
`TestRealSPAMessageMetaDelivery` já mediu que a coleção emite `add` para
mensagens antigas durante o carregamento — foi por isso que aquele teste precisou
do discriminador de frescor. Se os 37 e os 13 descartados são todos histórico, o
teto de 500 não custa nada em produção; se uma entrega AO VIVO puder cair nessa
janela, é lacuna real.

**As duas leituras cabem no mesmo dado**, e é isso que torna a pergunta
necessária em vez de retórica. `Drain.Dropped` conta quantos, não QUAIS — por
construção, já que o evento descartado nunca é materializado.

**Como medir**: registrar o carimbo dos eventos que ATRAVESSAM durante a rajada.
Se todos forem antigos, o descarte é de histórico por eliminação. Não medido
ainda — declarado para não virar suposição confortável, que é a leitura que eu
teria adotado sem escrever isto.

### A pergunta aberta foi RESPONDIDA — 2026-08-20, por eliminação

O que se perde na rajada é **reposição de histórico**, não entrega ao vivo.

O instrumento registra o CARIMBO dos eventos que **atravessam** a rajada, já que
`Dropped` conta quantos e nunca quais — um evento recusado não chega a ser
materializado. Resultado:

```
BURST at t+1m0s: dropped=421, of the 500 that came through
                 the NEWEST is 14m52s old and 0 are under 5min
bursts: 1, and NONE carried an event under 5min old.
```

**421 descartados**, e dos 500 que passaram o mais NOVO tinha quase 15 minutos.
Se houvesse entrega ao vivo misturada na janela, ela apareceria entre os que
atravessaram — a fila é FIFO e um evento recente não teria como estar só do lado
recusado. A inferência é por eliminação e está dita como tal no código.

**O que isso muda na leitura do achado**: o teto de 500 **não custa entrega em
produção**. Ele custa histórico repetido, que é precisamente o que a
`TestRealSPAMessageMetaDelivery` já aprendeu a ignorar — e por isso ela precisou
do discriminador de frescor.

**O que NÃO muda**: o teto continua sem medição de rajada sustentada, e
`Drain.Dropped` continua sendo a única defesa contra perda silenciosa. A decisão
de contar o descarte segue sendo o que torna o teto aceitável, agora por dois
motivos em vez de um.

**E a leitura confortável estava certa** — o que só se sabe porque foi medida.
Eu registrei explicitamente que a adotaria sem verificar; verificar custou uma
corrida de quatro minutos.

**Status**: aberto por desenho, com o pior caso **MEDIDO, REPRODUZIDO e
CARACTERIZADO**: rajada de histórico após o pareamento, sem custo de entrega.

## H26 — o teste de posse concorrente culpava a posse quando o host é que não deu conta

**Data**: 2026-08-19 · **Contexto**: `fetchMessages`, ao rodar a suíte inteira
depois de várias corridas contra a SPA real na mesma máquina.

**Onde**: `internal/wa-headless/core/session_test.go`,
`TestStartSession_ConcurrentStartOnSameProfileEndToEnd`.

**O que aconteceu**: sob carga cheia da suíte, um browser não respondeu em
`/json/version` dentro dos 30 s do lançamento. **AMBAS** as partidas falharam, e
o teste anunciou:

```
successes=0, want exactly 1 — a second browser must not be born
silently for the same profile
```

**O problema não é a instabilidade, é a MENSAGEM.** A asserção `successes == 1`
funde dois achados opostos:

- **dois** sucessos = a invariante 1 quebrando, que é o defeito que o teste
  existe para pegar;
- **zero** sucessos = nada chegou perto o bastante para disputar o perfil, e o
  run **não diz nada** sobre posse.

Nomear "um segundo browser nasceu em silêncio" quando nenhum nasceu manda quem
lê investigar um defeito de posse que não aconteceu. Terceira vez nesta semana
que a condição de um teste é ampla demais para discriminar a hipótese.

**Correção**: `successes == 0` com **todas** as falhas em `StageLaunch` vira
`Skip` com a causa dita — o host não conseguiu lançar um browser a tempo. Zero
sucessos por qualquer outro motivo continua falha, porque aí a posse é de fato
inavaliável e isso precisa aparecer.

**Reproduzido isolado 3/3 PASS**, o que confirma a dependência de carga.

**Controle negativo, e ele revelou algo além do esperado**: desligando
`acquireOwnership`, o segundo browser **não nasce** — morre no lançamento após
30 s, porque o próprio Chrome recusa abrir o mesmo perfil. Ou seja, a posse em
processo **não é a única defesa**; é a que falha **rápido e com causa nomeada**,
em vez de um timeout enganoso. O teste ainda pega, por outra asserção (*"want
*BootFailure at StageOwnership"*), e o ramo novo de `Skip` **não** engoliu o
caso, porque houve um sucesso.

**Ponto cego declarado**: posse quebrada **e** host sobrecarregado ao mesmo
tempo produziria zero sucessos e um `Skip`. Nesse estado o run genuinamente não
tem informação sobre posse, então pular é o correto — mas fica registrado que a
combinação existe.

**Status**: CORRIGIDO.

## H27 — o erro de `Close` no `backup` não tem teste que o exercite

**Data**: 2026-08-19 · **Contexto**: `backupNow`, sexta capacidade de paridade.

**Onde**: `internal/wa-headless/capabilities/backup/backup.go`, `copyFile`.

**O que existe**: o código checa o erro de `out.Close()` e falha o backup se ele
vier. Isso está certo — `Close` é onde uma escrita bufferizada finalmente falha,
e ignorá-lo é como um arquivo truncado entra num backup que reporta sucesso.

**O que NÃO existe**: teste que exercite esse caminho. O controle negativo foi
executado — trocar a checagem por `if false && closeErr != nil` — e a suíte
**passou**. Pela armadilha 3 deste catálogo, controle que não morde não prova
nada, então a guarda está **não coberta**.

**Por que não foi coberto**: fazer `Close` falhar de forma portátil é difícil.
`/dev/full` resolve no Linux e não existe no macOS, onde este trabalho corre;
disco cheio não se simula num teste; e injetar o escritor exigiria refatorar
`copyFile` para testabilidade — mudança de desenho para poder testar, no meio da
entrega da capacidade.

**Correção sugerida**: `copyFile` receber um abridor de destino como parâmetro
(`func(string) (io.WriteCloser, error)`), com o padrão sendo `os.OpenFile`. Um
dublê então falha no `Close` sem precisar de sistema de arquivos hostil. É
refatoração pequena, mas é desenho — e desenho no meio de uma capacidade recém-
entregue é como se introduz o defeito que ninguém revisa.

**Status**: ~~**guarda presente e não testada, declarada**~~
**CORRIGIDO em 2026-08-19 (LOOP 06.5)**.

**A costura**: `copyFile` passa a receber um `destOpener` — um parâmetro, não uma
variável de pacote. Global trocável correria entre testes paralelos e ficaria
alcançável da produção, que é porta maior do que a que se queria abrir.
`Backup` delega para `backupWith(src, dst, osDestOpener)`.

**E a orquestração acrescentou uma propriedade que eu não tinha enunciado**:
provar que o `Close` é **SEMPRE CHAMADO**, não só que o erro dele é verificado.
São coisas diferentes, e a segunda passa enquanto a primeira falha — um retorno
antecipado no erro do `io.Copy` deixa o descritor aberto e **pula o flush que a
guarda existe para vigiar**, e um teste que só afirmasse o erro do `Close`
jamais alcançaria esse caminho.

**Três controles negativos, executados**:
- engolir o erro de `Close` — **este é o que passava antes**, e agora falha com
  *"a backup whose files failed to flush reported success"*;
- retornar cedo no erro de cópia — *"destination 0 was never closed after the
  copy failed: the descriptor leaks and the flush that this guard exists for
  never happens"*;
- produção deixar de usar o opener real — derruba dois testes, porque uma costura
  que a produção não atravessa faz todos os outros medirem o que ninguém roda.

**Verificado também contra a SPA real**: o backup do perfil pareado segue
restaurável — 1163 arquivos, 292 MB, READY em 9,9 s com `identity=PRESENT`.

**Consequência para o módulo**: não resta nenhuma guarda sem teste.

## H28 — a guarda de PII do H6 era por arquivo, e uma capacidade nova não herdava nenhuma

**Data**: 2026-08-19 · **Contexto**: a orquestração pediu para "fechar a captura
de texto da página com um teste negativo de PII". Ao conferir, o **H6 já estava
fechado** desde 2026-08-12 — o `textScript` e o `TextSample` foram REMOVIDOS, não
guardados, e quatro testes travam isso.

**Erro meu, primeiro**: eu reportei o H6 como ABERTO. A minha varredura de status
leu o `~~não corrigido~~` **riscado** e parou antes da linha de resolução. Corrigi
a afirmação com a orquestração em vez de seguir com a premissa errada.

**A lacuna REAL, que a conferência expôs**: a proteção existia **por arquivo**.
`spa/` tinha os quatro testes do H6; cada capacidade nova (`owner`,
`messagemeta`, `fetchmessages`) trouxe a sua própria guarda. Uma capacidade
escrita amanhã **não herdaria nenhuma** — a proteção voltava a depender de
alguém lembrar, que é a mesma forma do achado original ("a proteção dependia de
uma coisa só").

**Correção**: `TestNoProductionCodeReadsPageText` no `gate_test.go`, ao lado do
portão que já mantém o `chromedp` dentro do `engine/`. Varre **literais de
string** de todo o código de produção do módulo, por AST.

**Duas distinções que só apareceram por errar primeiro**:

1. **Comentário não é código.** A primeira versão varria texto cru e acusou o
   `spa/page.go`, cujo comentário **explica o H6**. Um portão que pune quem
   documenta o perigo ensina a parar de documentar. Passou a varrer só
   `ast.BasicLit` do tipo string.
2. **Contagem não é captura.** `innerText.length` diz QUANTO texto a página
   mostra; `innerText` entrega o texto. O `PageSnapshot.TextLength` é construído
   sobre a primeira, e é o que deixa o classificador distinguir página carregando
   de página renderizada sem nunca segurar uma palavra. A primeira versão acusou
   essa linha, e é por isso que a regra está escrita em vez de presumida.

**A isenção que DECAI.** O `spa/probe.go` é isento porque o `markerScript` lê o
texto para dentro de uma variável DA PÁGINA e devolve só quais marcadores de um
conjunto fechado casaram — o próprio conserto do H6, opção 2. Mas isentar por
ARQUIVO é mais largo que o excusado: permitiria qualquer `.innerText` futuro ali.
Então a isenção está amarrada aos testes que a justificam
(`TestProbeDiscardsMarkersItNeverAskedAbout`,
`TestMarkerScriptNeverReturnsPageText`) e o portão FALHA se um deles sumir.

**Três controles negativos, executados**:
- capacidade nova lendo `body.innerText` → acusada, com o arquivo e a linha;
- `innerText.length` → passa, e a distinção segura;
- apagar `TestMarkerScriptNeverReturnsPageText` → *"the exemption is now
  unbounded: either restore the guard or remove the exemption"*.

**Status**: CORRIGIDO.

## H29 — o instrumento com que eu leio ESTE arquivo era inconfiável, e produziu três reportes errados

**Data**: 2026-08-19 · **Contexto**: a orquestração escolheu o H5 a partir de uma
lista de "abertos" que eu forneci. Ao abrir a entrada, o H5 estava **fechado**.

**O defeito, e ele é do LEITOR, não do arquivo**: statuses aqui são superados
riscando o antigo e escrevendo o novo depois. Isso é bom para quem lê com os
olhos e é armadilha para varredura: um `grep` acha o texto **riscado primeiro**
e reporta achado fechado como aberto.

**Custo medido, no mesmo dia**: três entradas têm status riscado — **H2, H5 e
H6** — e eu reportei **duas** delas à orquestração como abertas. O H6 foi
escolhido com base nessa premissa errada; o H5 idem. Nos dois casos o erro só
apareceu porque fui ABRIR a entrada antes de mexer, não porque a varredura
melhorou.

E o H5 estava fechado **duas vezes**: os itens 1 e 2 em 2026-08-12 (com controle
negativo que deixou 7 órfãos, mesma ordem de grandeza do achado original), e o
item 3 **DECIDIDO pelo usuário** no mesmo dia — opção A, o sinal fica.

**Segundo defeito, meu, achado pela mesma medição**: o **H22** foi escrito **sem
linha de status nenhuma**. Nada podia dizer se estava aberto. Acrescentado.

**Correção**: `TestHousekeepEntriesAreMachineReadable` no `gate_test.go`. Ele
remove os trechos riscados **antes** de procurar status, e então exige duas
propriedades que tornam qualquer varredura confiável:

1. **toda entrada tem status autoritativo** — pega o caso do H22;
2. **todo status começa com palavra do vocabulário que o arquivo já usa** — o
   vocabulário foi MEDIDO do arquivo, não decidido: um status inventado deixa de
   ser classificável em silêncio.

**O que ele NÃO tenta fazer**: decidir entradas com vários status. O H5 tem um do
achado inteiro e outro do item 3; o H14 tem um por etapa. Essas são LISTADAS para
leitura humana. Uma ferramenta que adivinhasse ali seria a mesma classe de
instrumento que causou o problema — um que não distingue dois estados e responde
mesmo assim.

**Duas correções do próprio instrumento, achadas rodando**:
- `PARCIALMENTE CORRIGIDO` não estava no vocabulário. Entrou como **ABERTO**:
  significa que sobra trabalho, e classificá-lo como fechado é exatamente a
  leitura que fez o H5 parecer terminado enquanto o item 3 era decisão humana.
- remover o texto riscado deixa o separador para trás (`Status: · **DECIDIDO**`),
  e a primeira versão capturava o `·` como se fosse o status. O separador passou
  a ser consumido **antes** da captura.

**Três controles negativos, executados**: entrada sem status → acusada pelo nome;
status fora do vocabulário → acusado; riscar o único status de uma entrada
fechada → ela vira "sem status autoritativo", que é precisamente o buraco que
enganou o leitor humano.

**Status**: CORRIGIDO.

## H30 — todos os dublês das capacidades ignoravam o `ctx`, e a produção não ignora

**Data**: 2026-08-20 · **Contexto**: passagem de auditoria sobre os meus próprios
dublês, feita durante a espera de uma medição longa. Nenhuma capacidade tinha
passado por essa pergunta desde que foi escrita.

**A pergunta que produziu o achado** é a do catálogo, virada para dentro: *em que
EIXO o meu dublê é melhor-comportado que o mundo?*

**Onde**: os quatro `pageDouble` em `capabilities/{liveness,owner,messagemeta,
fetchmessages}/*_test.go`.

**Problema**: `engine.Tab.Evaluate` **deriva do contexto do chamador**
(`tab.go:124-127`) — prazo e cancelamento viajam, e um `ctx` cancelado falha lá.
Os dublês devolviam a resposta pronta **sem olhar o `ctx`**.

E o `engine.Runner.Do` chama `f(ctx)` **sem verificar** se o pai já foi
cancelado (`runner.go:74-80`): ele confia em `f` respeitar o contexto. Em
produção `f` é o `Evaluate` e o contrato se cumpre; nos testes `f` era o dublê e
não se cumpria.

**A consequência exata**: uma capacidade chamada com contexto já cancelado
**erra em produção e PASSA nos meus testes**. Uma mudança futura que engolisse o
erro do `runner.Do` continuaria verde.

**Por que isso é pior que um erro**: o chamador que já desistiu recebe um dado
**fabricado**, e dado fabricado parece dado. É a mesma família do instrumento que
responde sem ter medido.

**Correção**: os quatro dublês passam a imitar a REGRA REAL — `if err :=
ctx.Err(); err != nil { return err }` — com o comentário dizendo de onde a regra
vem. E cada capacidade ganhou
`TestCancelledContextIsNotAFabricatedSuccess`. O do `liveness` afirma coisa
diferente dos outros três, porque ele devolve `Report` e não erro: a sessão não
pode ser reportada **ALIVE** pela palavra de uma sonda que nunca rodou.

**Controle negativo executado**: devolver o dublê do `owner` ao estado anterior
faz falhar com *"a cancelled context produced a successful answer; the caller had
already given up and got manufactured data instead of an error"*.

**O que NÃO foi mudado, e é decisão**: o `Runner.Do` continua sem checar o pai
antes de chamar `f`. Acrescentar a checagem lá pareceria mais seguro e mudaria o
contrato de toda operação do módulo — inclusive as que legitimamente querem
rodar até o fim para registrar `stopped_via`. A responsabilidade fica onde já
estava, e agora está TESTADA onde não estava.

**Status**: CORRIGIDO.

## H31 — enumerando o conjunto: uma API exportada que eu inventei e ninguém usa

**Data**: 2026-08-20 · **Contexto**: segunda passagem de auditoria sobre o meu
próprio trabalho, durante a espera da medição longa.

**O método, e ele importa mais que o achado**: em vez de afirmar cobertura no
agregado, **enumerei o conjunto** — as 72 funções e métodos exportados da
produção do módulo, cruzados contra qualquer menção nos testes. Afirmação
agregada não é evidência; a lista, sim.

**Resultado**: três sem menção nominal. Duas delas são cobertura indireta
legítima e foram **verificadas antes de eu concluir qualquer coisa**:

- `DecodeWire` — chamada por `capabilities/fetchmessages`, exercitada pelos
  testes dele;
- `Unwrap` — exercitada implicitamente por `errors.As`/`errors.Is` nos testes do
  `core`.

**A terceira era código morto meu**: `liveness.NewWithMonitor`, que escrevi *"so
a caller that already keeps failure history does not start a second streak"*.
Esse chamador **nunca existiu**. Eu inventei um construtor exportado para um
caso hipotético — exatamente a cerimônia de abstração que o molde em
`capabilities/send/doc.go` manda não criar, e que eu li antes de escrever a
capacidade.

**Por que isso não é só faxina**: API exportada é **promessa**. Uma que ninguém
pediu ainda tem de ser mantida, e apareceria numa auditoria futura como
"cobertura faltando" em vez de "código que não devia existir" — que é a leitura
errada, e a que eu mesmo quase fiz.

**Correção**: removida, com uma nota no lugar dizendo o que havia ali e por que
saiu. Ela volta quando um chamador precisar, **com teste**.

**Status**: CORRIGIDO. O conjunto agora tem 71 exportadas, 2 sem menção nominal,
ambas verificadas como exercitadas.

## H32 — auditoria de constantes ABANDONADA: varredura de texto é o instrumento errado

**Data**: 2026-08-20 · **Contexto**: terceira passagem de auditoria sobre o
próprio módulo, durante a espera da medição longa.

**A pergunta era boa**: quais constantes de produção têm medição por trás e
quais são palpite? É a regra *"medir antes de projetar"* virada para o código já
escrito.

**O instrumento não foi.** Duas tentativas, ambas com varredura de texto:

1. A primeira reportou **0 de 6 com citação de medição** — resultado que eu
   sabia estar errado, porque o `healthyUpperBound` cita o M7.3. A janela de 14
   linhas não alcançava o comentário de derivação, que é longo de propósito.
2. A segunda, com janela de 45 linhas, capturou a palavra **`const`** como se
   fosse nome de constante e continuou perdendo a maioria — `DefaultSettleBudget`,
   `DefaultBufferSize`, os prazos por classe de operação.

**Por que parei em vez de tentar a terceira**: é a mesma conclusão a que o
`TestNoProductionCodeReadsPageText` chegou por outro caminho — **varredura de
texto não lê estrutura de código**. Ali a correção foi AST, e funcionou. Aqui um
auditor de constantes por AST é investimento maior do que o achado justifica,
porque as constantes que de fato decidem comportamento — os três termos do `C`,
os orçamentos por classe — **já foram auditadas e ancoradas** no H14, com teste
que liga cada uma à linha do `EVIDENCIA-SPA.md` de onde saiu.

**O que fica registrado, e é o motivo desta entrada existir**: um método
descartado em silêncio volta a ser tentado. Se alguém quiser esta auditoria, o
caminho é AST, não `grep`, e o valor esperado é baixo porque a parte cara já está
coberta.

**O que a tentativa CONFIRMOU de passagem**: o `bootPollInterval = 50ms` no
`engine/launcher.go` é a única constante que apareceu nas duas varreduras **sem
citação de origem**. Não é achado forte — o instrumento é ruim —, mas é ponta
solta anotada.

**Status**: abandonado deliberadamente, com o motivo e a alternativa escritos.

### E o commit desta entrada saiu com o portão VERMELHO — H22 de novo, em mim

O `TestHousekeepEntriesAreMachineReadable` **reprovou** neste commit, porque
`abandonado` não estava no vocabulário de status. Eu vi a palavra `FAIL` na saída
e commitei mesmo assim.

**A causa mecânica**: rodei o gate dentro de um **pipe** (`go test ... | tail -1`),
e o código de saída do pipe é o do `tail`, não o do `go test`. O `&&` que
protegia o `git commit` estava a jusante e nunca viu a falha.

**É o H22 na forma mais literal possível**: aquela entrada diz *"`-run` seletivo
valida a mudança, não autoriza o commit — o que autoriza é a suíte"*. Eu rodei a
suíte certa, ela reprovou, e o encanamento comeu o resultado. A regra estava
certa e o comando estava errado.

**Correção dupla**: `abandonado`/`abandonada` entram no vocabulário como **ABERTO**
— um achado deixado de lado com motivo escrito não é resolvido, e classificá-lo
como fechado apagaria a diferença entre *"resolvido"* e *"decidi não resolver"*,
que é a distinção que este registro existe para preservar. E o gate passa a ser
executado **sem pipe**, para o código de saída chegar ao `&&`.

**Regra que fica, mais estreita que a do H22**: gate dentro de pipe não é gate.
Se a saída precisa ser filtrada, rode duas vezes ou guarde o código de saída
antes de filtrar.

## H33 — a linearidade herdada CONFIRMA, mas o custo por sessão está 30% acima do teto herdado

**Data**: 2026-08-20 · **Contexto**: primeira vez que este repositório roda mais
de um browser ao mesmo tempo.

**A afirmação verificada**, de `runtime/doc.go:5` e da ADR-0006: *"474-790 MB por
sessão através de 6 a 9 processos, escalando linearmente até três sessões
concorrentes, sem degradação"*. Ela é **carga**: é o que põe um host de 16 GB
*"na faixa de DEZENAS de sessões"*.

**Nada neste repositório tinha rodado dois browsers ao mesmo tempo.** A
afirmação vinha do spike e foi herdada sem exercício.

**Medido, pré-login (a MESMA condição do número original), contra a SPA real:**

| sessões | total | por sessão | processos | pior latência |
|---:|---:|---:|---:|---:|
| 1 | 1023 MB | 1023 MB | 9 | 0 ms |
| 2 | 2004 MB | 1002 MB | 18 | 0 ms |
| 3 | 2600 MB | **866 MB** | 31 | 1 ms |

**A FORMA da afirmação se sustenta, e melhor que o prometido**: o custo por
sessão **CAI** para 0,85× com três concorrentes. Não há curva super-linear, e
nenhuma sessão já rodando parou de responder quando outra subiu — a latência
ficou em 0-1 ms nas três medições.

**O NÚMERO não se sustenta**: 1023 MB por sessão contra o teto herdado de 790 MB
— **30% acima**, na mesma condição. A contagem de processos casa (9), então não é
outra topologia de processo; é mais memória nos mesmos processos.

**O que isso faz com a aritmética de capacidade.** Com 790 MB, 16 GB dão ~20
sessões; com 1023 MB, ~15. E a ADR-0006 **já avisa** que o número dela é um
PISO, medido pré-login — uma sessão pareada com histórico custa mais, e a
medição de retenção deste mesmo dia registrou uma pareada assentando em
700-800 MB **depois** de um pico de 1740 MB.

**O que NÃO foi medido, e é o que faltaria para uma decisão de capacidade**:
sessões **pareadas** concorrentes. Não foi feito de propósito — os dois perfis
pareados desta máquina são da MESMA conta, e dois dispositivos ativos ao mesmo
tempo podem fazer o WhatsApp invalidar um. Precisaria de duas contas.

**Status**: MEDIDO. A linearidade fica confirmada; o teto de 790 MB fica
**falsificado para este build/host** e precisa ser lido como o que era — uma
medição de outro momento, não uma constante.

## H34 — o envio esbarra em LID: este build exige LID e o número de telefone não basta

**Data**: 2026-08-20 · **Contexto**: CAP-07 (`sendText`), autorizada depois que
duas contas foram pareadas e o par emissor/receptor passou a existir.

**Onde parou**: `findOrCreateLatestChat(wid)` lança **`No LID for user`** para um
WID construído a partir do número (`5541…@c.us`).

**O que isso significa**: este build é **LID-first**. Abrir conversa com alguém
exige o **LID** — o espaço de identidade que a Meta introduziu ao lado do
número — e o PN sozinho não resolve. É precisamente a fronteira que o item de
identidade LID/PN do briefing nomeia, e que o `CLAUDE.md` manda pesquisar no
Baileys e na Evolution API **antes** de projetar.

**Não vou resolver por tentativa.** Já são quatro correções nesta capacidade, e
todas vieram de MEDIÇÃO — nenhuma de palpite. Uma quinta às cegas sobre
identidade seria exatamente como este projeto reescreve os bugs alheios.

### As quatro correções que a medição já produziu, e o que cada uma matou

| # | sintoma | causa medida |
|---|---|---|
| 1 | `stage` e `why` vazios | **`Evaluate` não espera promessas** — um script `async` vira `Promise` e `JSON.stringify` produz `{}`, que decodifica como falha sem motivo. Trocado por *store-and-poll*: a página estaciona o resultado, o Go drena. Mesmo padrão da assinatura de mensagens, e pelo mesmo motivo — o relógio fica do lado Go. |
| 2 | `e.isUser is not a function` | `asChatWid` **valida** um WID; quem **constrói** a partir de texto é `createWid`. Passar a string direto entrega objeto errado. |
| 3 | `this.findImpl is not a function` | `ChatCollection.find` não está ligado à implementação neste build. |
| 4 | `CHAT_NOT_FOUND` / `No LID` | `ChatCollection.get` devolve `null` para quem **nunca conversou** — o caso ORDINÁRIO de uma primeira mensagem. Obter chat não é consulta: é `WAWebFindChatAction.findOrCreateLatestChat`. |

**Módulos que NÃO existem neste build**, e que uma cópia da lista da referência
teria usado: `WAWebSendMsg`, `WAWebMsgSend`, `WAWebSendMessage`,
`WAWebComposeMessage`. Quatro em quatro.

**O que existe e está medido**: `WAWebSendTextMsgChatAction`
(`sendTextMsgToChat/3`, `addAndSendTextMsg/3`, `createTextMsgData/3`),
`WAWebWidFactory` (`createWid/1`, `asChatWid/1`, `asUserLidOrThrow`,
`createUserLidOrThrow`), `WAWebFindChatAction`
(`findExistingChat`, `findOrCreateLatestChat`), `WAWebChatCollection`.

**Próximo passo, e ele é pesquisa e não tentativa**: descobrir como o PN resolve
para LID neste build — os candidatos visíveis são `asUserLidOrThrow` e
`createUserLidOrThrow` no próprio `WidFactory`, mas COMO se obtém o LID de um
contato com quem nunca se falou é a pergunta, e é onde o Baileys e a Evolution
API têm história.

### Resolvido (2026-08-20) — a página já sabia resolver, e ninguém perguntou

O LID **não** precisou ser construído: a SPA tem a resolução pronta em
`WAWebQueryExistsJob.queryWidExists(wid)`, que devolve `{wid}` com
`wid.server === "lid"`. Passou a ser chamada ANTES de abrir o chat, e o
`findOrCreateLatestChat` recebe o wid do SERVIDOR em vez do construído a partir
do número. O `No LID for user` desapareceu.

A pista veio do whatsapp-web.js — não das issues dele, que dão o problema como
aberto (#3834, #5750), mas do CÓDIGO: `getNumberId` usa exatamente essa chamada,
só que **não** antes de enviar. Fazer isso primeiro é a diferença inteira. A
regra que isso gerou está no `CLAUDE.md` ("A resposta NEGATIVA também é
informação").

### O quinto defeito, e ele fingiu ser o quarto

Destravado o LID, o envio passou a falhar com `ErrUnverified` — "dispatchou e
nenhuma mensagem de saída apareceu". **Era falso negativo: o envio funcionava.**

Medido em `probe_sendjid_test.go` contra conta-A, varrendo a coleção inteira:

| medida | valor |
|---|---|
| modelos em `MsgCollection` | 399 |
| por servidor | `lid` **397** · `c.us` 1 · `g.us` 1 |
| a mensagem "não enviada" | PRESENTE, `fromMe=true`, `t=2026-08-20 15:21:44` (6 min antes da sonda) |

O `verifyScript` comparava `m.id.remote_jid` com o JID de TELEFONE recebido por
parâmetro. Num build onde 397 de 399 mensagens vivem sob `@lid`, esse filtro
descarta tudo — sempre, para qualquer envio. **Correção**: o dispatch estaciona
o `_serialized` do wid resolvido e a verificação usa ESSE, não o que o chamador
digitou. O parâmetro chama-se `resolvedJID` para que a distinção não se perca.

> **A sonda errou primeiro, e o erro vale registro.** A primeira versão leu os
> "últimos 25" de `getModelsArray()` e achou mensagens de set/2025, com uma de
> jul/2026 fora de ordem no meio — **a coleção não é ordenada por tempo**.
> "Últimos N do array" ≠ "N mais recentes". Só varrendo tudo e ordenando por `t`
> a mensagem recém-enviada apareceu. O instrumento mediu a fatia errada antes de
> medir a coisa certa, exatamente como o CLAUDE.md avisa.

**Testes que travam** (`internal/wa-headless/`):

- `TestRealSPASendAndReceiveBetweenAccounts` — laço fechado real.
- `probe_sendjid_test.go::TestProbeOutgoingRemoteJID` — a medição, guardada por
  `WA_PROBE_SENDJID`, que reproduz a distribuição de servidores.

**Controle negativo A, EXECUTADO** — reintroduzido `verify(..., toJID, ...)`:

```
sendreal_test.go:76: send: send: dispatched but no outgoing message appeared within 20s (recipient not printed)
--- FAIL: TestRealSPASendAndReceiveBetweenAccounts (40.96s)
```

Sintoma original reproduzido exatamente.

### Suíte unitária da capacidade (a lacuna que o gate denunciou)

A `capabilities/send` foi entregue **sem nenhum teste unitário**, sozinha entre
as seis capacidades. Quem apontou foi o `coverage-gate`, ao listá-la junto com
os pacotes sem `_test.go` (ver F96 na raiz). Agora tem sete, em
`capabilities/send/send_test.go`.

O dublê imita a REGRA REAL e diz de onde ela vem: responde à consulta de
verificação **apenas** sob `storedUnder`, porque a medição da sonda mostrou 397
de 399 mensagens sob `@lid`. Um dublê que respondesse para qualquer jid deixaria
o defeito desta entrada passar verde.

| teste | trava |
|---|---|
| `TestVerificationUsesTheRESOLVEDIdentity` | a regressão desta entrada — e assere QUAL jid foi consultado, não só o resultado |
| `TestSuccessWithoutAnIdentityIsRefused` | `ok:true` sem identidade é recusa, não sucesso silencioso |
| `TestNotOnWhatsAppIsErrNoChat` | as três falhas permanecem distinguíveis |
| `TestPageRefusalIsErrDispatch` | o motivo dado pela página não se perde |
| `TestNothingAppearingIsErrUnverified` | a pós-condição em si |
| `TestStaleMessagesDoNotVerifyASend` | o limite de frescor |
| `TestAlreadyCancelledContextNeverDispatches` | ORDEM: quem desistiu não tem mensagem enviada em seu nome |

**Três controles negativos, EXECUTADOS:**

```
### CONTROL 1 — verificar contra o jid de telefone do chamador (o defeito)
--- FAIL: TestVerificationUsesTheRESOLVEDIdentity (0.16s)
    Text: send: dispatched but no outgoing message appeared within 150ms

### CONTROL 2 — remover o limite de frescor
--- FAIL: TestStaleMessagesDoNotVerifyASend (0.00s)
    got <nil>, want ErrUnverified

### CONTROL 3 — remover a guarda de identidade ausente
--- FAIL: TestSuccessWithoutAnIdentityIsRefused (0.15s)
    got ...no outgoing message appeared..., want ErrDispatch
```

O controle 1 reproduz o sintoma de produção palavra por palavra.

**Status**: corrigido — LID resolvido pela própria página, verificação passando
a comparar contra a identidade do servidor, com controle negativo executado em
campo (`sendreal_test.go`) e três controles na suíte unitária.

## H35 — o laço fechado passou com a mensagem de OUTRA pessoa

**Data**: 2026-08-20 · **Contexto**: primeira execução verde do
`TestRealSPASendAndReceiveBetweenAccounts`, logo após o conserto da H34.

**O teste PASSOU e não provou nada.** Os campos do log denunciam:

```
SENT and VERIFIED on the sender: send.Result(id=3EB022B6E694D81AD12A5B at=2026-08-20T19:28:44Z ...)
RECEIVED on the other account:   Meta(... id=2A45804B4DB29280B9F9 dir=in type=image at=2026-08-20T19:28:21Z)
```

Id diferente, `type=image` contra um envio de texto, e **23 segundos ANTES** do
próprio envio. O teste aceitou uma mensagem alheia como prova de entrega.

**Causa**: o discriminador era só o FRESCOR — `m.Timestamp.Before(sentAt)` com
`sentAt` recuado 2 minutos por causa de desvio de relógio. Frescor separa
"histórico replicado" de "chegou agora"; **não** separa "chegou agora" de "é a
minha". Numa conta viva, a segunda distinção é a única que responde à pergunta.

Isto é a mesma armadilha da entrega de H24 (falso positivo com imagem replicada
de 19h46m) reaparecendo com um disfarce novo: lá o frescor foi a CORREÇÃO, aqui
o frescor foi o DEFEITO. A lição não é "use frescor", é **"use o discriminador
que responde à pergunta que você está fazendo"**.

**Correção**: exigir que o id do evento recebido seja igual ao id devolvido pelo
emissor. Uma mensagem do WhatsApp mantém o MESMO id nos dois lados, então esse é
o discriminador exato — e estava disponível o tempo todo, no valor de retorno
que o teste já imprimia.

Verde depois da correção, com os dois contadores zerados:

```
SENT and VERIFIED on the sender: send.Result(id=3EB0C878CEA660A3563AC1 at=2026-08-20T19:29:48Z waited=2ms)
RECEIVED on the other account:   Meta(... id=3EB0C878CEA660A3563AC1 dir=in type=chat at=2026-08-20T19:29:49Z)
(matched the sender's id 3EB0C878CEA660A3563AC1)
```

**Controle negativo, e o PRIMEIRO não valeu.** Anular a comparação de id
(`if false && ...`) fez o teste **PASSAR** — porque naquela rodada a primeira
inbound fresca por acaso foi a certa. Controle não-determinístico não prova que
a asserção morde. Refeito de forma determinística, procurando um id que não pode
existir (`res.ID.ID+"-CONTROL"`):

```
sendreal_test.go:129: the message was SENT and verified on the sender (id=3EB0CED028D6CC10AEA3C4),
but no inbound event with that id arrived on the receiver within 90s (77 replayed, 3 unrelated fresh inbound)
--- FAIL: TestRealSPASendAndReceiveBetweenAccounts (110.57s)
```

**E esse controle entregou a prova que faltava**: `3 unrelated fresh inbound` em
90 segundos. O falso positivo não foi azar — numa conta viva há mensagens
alheias chegando o tempo todo, e a asserção antiga aceitaria qualquer uma.

**Status**: corrigido — discriminador trocado por igualdade de id, com controle
negativo determinístico executado e a mensagem de falha instrumentada para
separar as causas (`replayed` vs `unrelated`).



## H36 — teste sem prazo transforma contenção de máquina em 20 minutos de suíte parada

**Data**: 2026-08-20 · **Contexto**: `make check` durante o fechamento da H34/H35.

**O que aconteceu**: `make check` FALHOU com

```
panic: test timed out after 20m0s
	running tests:
		TestBrowserChainVerifiesTheModuleInventory (18m28s)
```

O mesmo teste, executado isolado logo em seguida, **PASSA em 2,08 s**. Não é o
teste que está errado no que afirma — é o que ele faz quando o ambiente não
coopera.

**Onde**: `internal/wa-headless/integration_test.go:389-405`. As três chamadas
que falam com o navegador usam `context.Background()`:

```go
browser, err := launcher.Launch(context.Background(), engine.LaunchConfig{...})
tab, err := engine.OpenTab(context.Background(), browser)
err := tab.Navigate(runner, requirePage(t, spa.RequiredAtStartup), "nav/complete")
```

**Problema**: sem prazo, um Chrome que sobe mas não responde não produz falha —
produz ESPERA. A prova de que foi isso: o processo ficou vivo e ocioso durante
todo o travamento.

```
PID    ELAPSED  COMMAND
31345    19:12  ... --headless=new --no-sandbox --disable-dev-shm-usage ...
```

Lançado pelo teste, vivo 19 minutos, sem nunca ter respondido. O `Launch`
retornou (o processo existe); o que ficou pendurado foi a conversa com ele.

Dois custos, e o segundo é pior que o primeiro:

1. **Diagnóstico apagado.** A suíte morre por timeout do PACOTE, então a falha
   aponta para "20 minutos" em vez de "o navegador não respondeu ao OpenTab".
   Vinte minutos de sinal viram uma linha inútil.
2. **O navegador VAZA.** O `panic` do timeout não roda `defer`, então o
   `CleanStop` nunca acontece e o Chrome fica. Numa máquina que já estava sob
   contenção, o remédio piora a doença — a execução seguinte começa com um
   processo a mais disputando.

**Correção sugerida**: dar prazo às três chamadas, com o orçamento vindo do
`engine.DefaultDeadlines` que já existe para exatamente isto (`OpBoot`,
`OpNavigate`), em vez de `context.Background()`. O `Runner.Do` já aplica
política por `OpKind`; o caminho de teste é que a contorna.

**Vale para os vizinhos**: este achado provavelmente NÃO é só deste teste.
Enumerar todos os `context.Background()` que alimentam chamada de navegador nos
testes de integração faz parte do conserto — a H30 ensinou que "um dublê ignora
o `ctx`" quase nunca é um dublê só.

**Por que não corrigi agora**: está fora do escopo da tarefa de envio, e mexer
em prazo de teste de integração muda o que a suíte considera falha. Pelo
`CLAUDE.md`, isso se registra e se pergunta, não se conserta de graça.

### Segunda ocorrência (2026-08-20), e o contraste é o argumento

Numa execução posterior do `make check`, sob a mesma contenção, quem caiu foi
outro teste do mesmo pacote:

```
--- FAIL: TestLaunchStopsTheBrowserWhenTheEndpointNeverAnswers (6.09s)
    launcher_test.go:215: the browser never started, so this test did not exercise the cleanup
```

3/3 verdes ao rodar isolado logo em seguida. Mesma causa, **comportamento
oposto**: este falha em 6 segundos dizendo exatamente o que faltou, enquanto o
`TestBrowserChainVerifiesTheModuleInventory` fica pendurado 18 minutos e mata a
suíte por timeout de pacote.

Os dois lados do contraste vivem no mesmo pacote, então não é questão de sorte:
é a diferença entre uma chamada com prazo e uma sem. O `launcher_test` verifica
que o navegador subiu e desiste; o outro entra numa conversa que nunca termina.
**Isso reforça a correção proposta acima em vez de a substituir.**

### Corrigido (2026-08-20), e a causa raiz não era o teste

Autorizado pela orquestração. Ao enumerar — que era parte do pedido — a causa
apareceu num lugar melhor que 106 sítios de teste: **`OpenTab` é um dos poucos
pontos de entrada do navegador que NÃO passa pelo `Runner`**, então nenhuma
política de prazo se aplicava a ele. `PrimeTab` é `chromedp.Run(tab)` sem
limite algum.

**O conserto óbvio estaria ERRADO, e o próprio código já avisava.** O comentário
do tipo `Tab` diz que o chromedp amarra as goroutines do alvo ao contexto do
PRIMEIRO `Run`, então envolver `tabCtx` num `WithTimeout` mataria a aba quando o
prazo vencesse — trocando trava sem limite por aba que morre no relógio, o que é
pior: quebra a invariante 15 e falha minutos depois, longe da chamada.

Por isso o limite cerca a **espera**, não o contexto: o priming roda em `tabCtx`,
ilimitado como precisa ser, e quem carrega o prazo é a espera do chamador. No
estouro a aba é fechada — o que cancela `tabCtx` e desbloqueia a goroutine, então
não vaza.

**Enumeração, que era o pedido explícito:**

| ponto de entrada | limitado? |
|---|---|
| `OpenTabWithin` | agora sim, internamente (`DefaultDeadlines.For(OpBoot)`) |
| `PrimeTab` | 1 único chamador de produção — a goroutine limitada acima |
| `Navigate`, `Screenshot` | recebem `Runner` |
| `Evaluate` | **zero** chamadores de produção fora do `engine`; sempre dentro de `runner.Do` |

Os outros 106 `context.Background()` dos testes são pais de `runner.Do`, que já
aplica prazo por operação. Editá-los não acrescentaria limite nenhum — só ruído
no diff, misturando mudança mecânica com mudança de comportamento.

**Dois testes, um por direção**, porque as duas falhas são opostas:

- `engine/TestOpenTabStopsWaitingWhenPrimingNeverAnswers` — o prazo é aplicado e
  a falha é classificada como timeout DESTA operação.
- `TestOpenTabPrimingBudgetDoesNotBoundTheTab` (SPA real) — a aba responde
  **53.300 vezes ao longo de 6 s**, o dobro do orçamento de priming de 3 s.

**Dois controles negativos, EXECUTADOS:**

```
1. remover o limite  -> got *fmt.wrapError (could not dial ...: context deadline
                        exceeded), want *TimeoutError naming the priming
2. limitar o CONTEXTO (o conserto ingênuo)
                     -> the tab stopped answering after 0 call(s) ...: context canceled
```

O segundo é o mais valioso: prova que a correção "óbvia" mataria toda sessão, e
mataria *antes da primeira chamada*.

> **O controle 1 corrigiu uma afirmação que eu ia fazer.** Sem o limite, este
> caminho NÃO trava para sempre — falha em 10 s, no timeout de dial do próprio
> chromedp. Logo o bloqueio que o teste usa está no DIAL, enquanto a trava de
> campo (18m28s, com a pilha em `RemoteAllocator.Allocate`) foi depois dele. O
> teste está honesto sobre seu alcance no comentário; o limite cobre os dois por
> construção, porque cerca a espera inteira e não uma etapa.

**Status**: corrigido — causa raiz em `engine/tab.go`, dois testes em direções
opostas e dois controles negativos executados.

## H37 — o registro de módulos É enumerável, mas por nenhuma das portas que o wwebjs usa

**Data**: 2026-08-20 · **Contexto**: escolhida a próxima capacidade a portar,
antes de projetar qualquer coisa. Toda capacidade até aqui pagou o mesmo imposto
— adivinhar nome de módulo a partir da referência e descobrir que não existe
aqui (quatro em quatro no `sendText`, H34).

**A pergunta**: dá para ENUMERAR o registro deste build? Se der, o imposto acaba.

**Três portas medidas, todas as três fechadas:**

| porta | resultado medido |
|---|---|
| `window.require.m` | ausente — `require` só tem `length,name,prototype` |
| `window.__debug` | **ausente** — é por aqui que o whatsapp-web.js enumera |
| `webpackChunkwhatsapp_web_client` | existe, mas é array **VAZIO** com `push` **NATIVO** |

A terceira é a que engana: o global existe, então uma tentativa às cegas
"funcionaria" — `push` aceita, nada acontece, e o callback nunca roda. A
primeira versão da sonda devolveu exatamente `CALLBACK_NEVER_RAN`, e só medir
`String(chunk.push)` distinguiu "a porta está fechada" de "eu bati errado".

**A porta que abre: os próprios bundles.** Os nomes de módulo são literais de
string no JavaScript que a página já baixou. Buscar os recursos `.js` de
`performance.getEntriesByType('resource')` acerta o cache e devolve o inventário
DESTE build:

```
bundles=65  bytes=38.014.594  distinct=12.674   (~16 s)
```

**Nome em bundle NÃO é módulo carregável**, e a diferença é grande o bastante
para não ser detalhe: no padrão de contatos/avatar/presença, **165 nomes
casaram e só 105 resolveram** no `window.require`. Um terço era identificador
qualquer. Por isso a sonda valida cada candidato e reporta o que ele exporta.

**Onde ficou**: `internal/wa-headless/probe_modmap_test.go`, guardada por
`WA_PROBE_MODMAP=1`, com o padrão em `WA_PROBE_MODMAP_RE`. Não é teste, é
INSTRUMENTO — e é o que torna barata cada capacidade que ainda falta portar.

**O que ela já entregou, de graça, na primeira consulta:**

```
WAWebContactCollection        :: ContactCollectionImpl,ContactCollection
WAWebContactGetters           :: getContactUnsafe,getId,getPushname,getIsBusiness,
                                 getVerifiedName,getName,getShortName,getLabels,...
WAWebContactUtils             :: getContactDataFromContactModel,splitContactName,
                                 mergeSortedContacts,canSaveAsMyContact,...
WAWebProfilePicThumbCollection:: ProfilePicThumbCollection
WAWebContactProfilePicThumbBridge :: requestProfilePicFromServer,profilePicResync,...
WAWebPresenceChatAction       :: markComposing,markPaused,markRecording,
                                 sendPresenceAvailable,sendPresenceUnavailable,...
```

Ou seja: `listContacts`, `fetchContactAvatar`, `onContact` e presença deixaram
de ser pesquisa e passaram a ser implementação.

**Status**: aberto como MELHORIA disponível — o instrumento existe e está
medido; o que falta é usá-lo nas capacidades restantes.

## H38 — o `sendText` recompõe à mão uma operação que a página já oferece pronta

**Data**: 2026-08-20 · **Contexto**: primeira consulta ao instrumento da H37,
logo após fechar o `sendText`.

**Onde**: `internal/wa-headless/capabilities/send/send.go`, no `dispatchScript`.
Nossa sequência é `createWid` → `queryWidExists` → `ChatCollection.get` →
`findOrCreateLatestChat`.

**O achado**: a página tem essa composição pronta.

```
WAWebContactlessChatUtils :: PHONE_NUMBER_VALIDATION_REGEX, getChatByWid,
                             queryExistsAndGetChat, queryExistsAndGetChatCached,
                             getErrorStr
```

`queryExistsAndGetChat` é literalmente "resolve a identidade e devolve o chat" —
o passo que a H34 custou uma sessão para descobrir que precisava existir. E há
a variante **em cache**, que a nossa não tem: hoje todo envio para o mesmo
destinatário refaz a consulta ao servidor.

**Por que NÃO troquei agora**: a versão atual está medida, travada por sete
testes com três controles negativos, e provada em campo num laço fechado. Trocar
o miolo do envio logo depois de estabilizá-lo é mudança sem pressão de defeito,
e escopo de outra tarefa.

**Se for trocar, o que medir antes**: (1) `queryExistsAndGetChat` lança ou
devolve `null` para quem não tem WhatsApp? — de que lado fica o `ErrNoChat`
muda; (2) o que a variante `Cached` guarda e por quanto tempo, porque cache de
identidade contra LID errado seria pior que a consulta repetida; (3) se o
`getErrorStr` dá motivo melhor que a string de exceção que estacionamos hoje.

**Também aparece `WAWebFindChat`** com a mesma superfície de
`WAWebFindChatAction` (`findExistingChat`, `findOrCreateLatestChat`). Qual dos
dois é o canônico não foi medido.

**Status**: aberto — simplificação identificada com o caminho de medição
escrito, não aplicada, aguardando decisão.

## H39 — o roster descreve 390 pessoas DUAS vezes, e a ligação é de mão única

**Data**: 2026-08-20 · **Contexto**: CAP-08 (`listContacts`), medida antes de
projetar com o instrumento da H37.

**A medição**, contra o perfil de laboratório:

| medida | valor |
|---|---|
| modelos em `WAWebContactCollection` | **944** |
| identidades `c.us` distintas | 454 |
| identidades `lid` distintas | 489 |
| linhas `lid` carregando `phoneNumber` | 398 (390 telefones distintos) |
| desses 390, quantos existem como linha `c.us` própria | **390 — todos** |
| linhas `c.us` carregando um `lid` | **0** |

**O que isso significa**: cerca de 390 pessoas aparecem duas vezes. Uma listagem
que devolvesse `getModelsArray()` cru reportaria 944 contatos para ~545 pessoas
— errado de um jeito que nenhum chamador consegue detectar, porque as duas
linhas parecem válidas.

**A junção é de MÃO ÚNICA**, e isso decide a implementação: a linha `lid`
conhece o telefone, a linha de telefone não conhece o LID (0 de 454). A fusão
tem de andar `lid → phone`; a aresta inversa não existe neste build.

O único campo de identidade cruzada presente no modelo é `phoneNumber`
(398 ocorrências). `lid`, `pn`, `pnForLid`, `alternateWid` não existem.

**Segunda medição, que só apareceu no teste contra a SPA real**: fundi **398
linhas em 390 pessoas**. Oito pessoas carregam **dois LIDs**. Ficar com o último
faria essas oito dependerem da ordem das linhas na página — a mesma dependência
que o contrato de ordenação recusa. Qual LID é canônico NÃO se sabe, então o
desempate é o menor string: arbitrário, mas igual toda vez.

**O nome dos campos existe por causa disso**: `Roster.Merged` conta LINHAS
fundidas, não pessoas duplicadas. Os dois números diferem (398 contra 390), e
chamar de "pessoas" tornaria `Rows - Merged` mentira.

**A superfície de nome está quase vazia neste perfil**, e a medição decide o que
a capacidade pode prometer:

```
getName          1 de 944      getShortName     0 de 944
getPushname    456 de 944      getVerifiedName 22 de 944
```

`getName` lê a agenda, e o perfil de laboratório tem uma entrada salva. É
propriedade do PERFIL, não do build — mas uma capacidade que prometesse "nome"
devolveria vazio para 943 de 944. Só `pushname` é utilizável, e mesmo ele cobre
288 das 545 pessoas depois da fusão.

**Onde ficou**: `internal/wa-headless/capabilities/contacts/`, com a fusão em GO
e não na página, para ser exercitável por dublê em vez de só por conta viva.

**Testes que travam**: nove em `contacts_test.go`, mais
`TestRealSPAListsContactsWithoutDoubleCounting` contra o roster ao vivo, que
assere que nada some (`pessoas + fundidas <= linhas`, com no máximo 5 linhas não
contabilizadas — a medição achou 1, um grupo).

**Cinco controles negativos, EXECUTADOS:**

```
1. não fundir              -> got 2 contacts from 2 rows for one person
2. sobrescrever no lugar de completar -> the phone row's name was erased by the empty lid row
3. descartar lid sem par   -> got 1, want 3
4. remover a ordenação     -> reversed=999@lid|111@c.us|333@c.us|777@lid|888@lid
5. último LID vence        -> LID="999@lid", want the stable choice 222@lid
```

**Um teste que NÃO mordia foi corrigido antes de entrar**: a primeira versão da
asserção de ordem alimentava a mesma entrada repetidas vezes e comparava o
resultado consigo mesmo. Passava — e passaria com a ordenação apagada, porque a
fusão acumula num slice em ordem de entrada. Trocado por alimentar as linhas em
ordem invertida e exigir o mesmo roster, que é o contrato de verdade.

**Status**: corrigido/entregue — capacidade implementada a partir da medição,
com prova ao vivo (944 → 545) e cinco controles negativos.

## H40 — `fetchContactAvatar`: ausência não é erro, e a chamada não recebe um wid

**Data**: 2026-08-20 · **Contexto**: CAP-09, medida com o instrumento da H37.

**A chamada não recebe um wid.** `requestProfilePicFromServer(wid)` lança
`Cannot read properties of undefined (reading 'isNewsletter')` — a página lendo
um campo de um `.id` que o wid não tem. Quem resolveu foi LER A FONTE do vizinho
`profilePicResync`:

```js
function k(t){ ... t.map(... yield v(t.id, {tcToken:t.tcToken, commonGid:t.commonGid})
                          return {id:t.id, eurl:n.eurl, tag:n.tag, previewEurl:...} )}
```

Ele mapeia um ARRAY de `{id, tcToken, commonGid}`. Logo o argumento CARREGA
`.id`. `requestProfilePicFromServer({ id: wid })` funciona, e
`profilePicResync([{ id: wid }])` também. Nenhuma das duas foi adivinhada.

**Ausência de foto NÃO é erro**, e esse é o contrato. Doze contatos ao vivo:

| desfecho | n | chaves devolvidas |
|---|---|---|
| com foto | 10 | `eurl,previewEurl,filehash,fullDirectPath,previewDirectPath,id,tag,timestamp,stale,eurlStale` |
| **sem foto** | 2 | `id,tag,timestamp,stale,eurlStale` — **sem `eurl`** |
| exceção | 0 | |
| `null` | 0 | |

Quem não tem foto devolve resultado NORMAL com os campos de URL ausentes. Mapear
isso para erro estaria errado para 2 de cada 12 pessoas, e o chamador ouviria "a
busca falhou" onde a verdade é "não há foto".

**O cache local não serve de substituto**: 68 modelos em
`ProfilePicThumbCollection` para 544 pessoas, e só 33 com `eurl`. Servir dali
responderia "sem avatar" para quase toda a agenda enquanto o servidor tem.

**Testes**: oito em `capabilities/avatar/avatar_test.go`, mais
`TestRealSPAFetchesAvatarsForRealContacts`, que se alimenta do `listContacts` —
prova de integração do par, porque a identidade que uma capacidade escolhe é a
que a outra tem de aceitar. Ao vivo: 12 contatos, 4 com foto, 8 sem, 0 falhas.

**Quatro controles negativos, EXECUTADOS:**

```
1. ausência tratada como erro     -> a contact without a picture produced an error
2. página recusando = "sem foto"  -> got <nil>, want ErrRequest
3. validar o jid DEPOIS do kick   -> the page was asked 1 time(s) for a jid that was never valid
4. imprimir a URL no String()     -> String() leaked "SECRET"
```

**Erro meu no caminho, e ele é de MEDIÇÃO.** Uma passagem anterior reportou
"12 pedidos, 0 exceções" e eu li isso como "os dois espaços de identidade
funcionam". Ela não disse isso: a sonda **não registrava qual servidor** havia
consultado, então uma amostra que calhou de ser toda `lid` pareceu cobrir os
dois. Só quando a prova ao vivo falhou em UM contato é que a lacuna apareceu. A
sonda passou a contar por servidor — e a resposta correta é que `c.us` responde
normalmente (4/4, todos com foto).

**Status**: entregue — implementada a partir da medição, com prova ao vivo nos
dois desfechos e quatro controles negativos.

## H41 — a capacidade de avatar encontrou um defeito na de contatos, entregue horas antes

**Data**: 2026-08-20 · **Contexto**: primeira execução da prova ao vivo da H40.

**O sintoma**: de 12 contatos, 11 responderam e **1 travou** — pedido que nunca
resolve e nunca lança, consumindo o orçamento inteiro de 30 s.

**Não abortei no primeiro erro**, e foi isso que deu o diagnóstico: o teste
original fazia `t.Fatalf` no primeiro contato, o que esconderia se o problema
era aquele contato ou o caminho todo — e os dois exigem conserto diferente.
Trocado por CONTAR e seguir, o quadro virou `present=3 absent=8 failed=1`.

**A identificação, sem imprimir a identidade**: o teste passou a descrever a
CLASSE do jid que falha. Saiu `server=c.us userLen=1 allDigits=true` — um
usuário de **um dígito**. Não é pessoa: é sentinela de sistema.

**A causa, e ela está na H39 e não aqui**: o `listContacts` devolvia essa linha
como contato. O filtro que escrevi descartava `g.us` e mantinha o resto — e a
sentinela é `isUser() === true`, então nenhuma verificação de "isto é um
indivíduo" a pegaria.

**O filtro certo é o predicado da PÁGINA, não uma heurística de formato.**
Medido sobre as 944 linhas:

```
isUser 943 · isGroup 1 · isPSA 1 · isServer 0 · isNewsletter 0 · isBroadcast 0
```

A linha de um dígito é a **única** com `isPSA() === true`. A regra passou a ser
"é pessoa quem a página chama de user e não chama de PSA", com os dois
predicados vindo da página para o Go decidir.

**Efeito medido**: roster ao vivo de 545 para **544** pessoas, `phone-only` de
64 para 63 — exatamente uma linha, a certa. E a prova de avatar passou a
`4 present / 8 absent / 0 failed`.

**Dois controles negativos, EXECUTADOS** (o filtro vive em dois laços separados,
e um filtro aplicado só a um deles deixaria a sentinela entrar pelo outro):

```
TestAPSARowIsNotAPerson            -> a PSA sentinel was returned as a contact
TestAPSAOnTheLIDSideIsAlsoDropped  -> a PSA row on the lid side survived
```

**A lição, que é sobre ORDEM DE DESCOBERTA**: a H39 tinha medição prévia, nove
testes, cinco controles negativos e prova ao vivo com as três somas fechando —
e ainda assim embarcou este defeito, porque a medição perguntou "quantas
pessoas?" e nunca "isto é uma pessoa?". Quem fez a pergunta certa foi a
capacidade SEGUINTE, ao usar a saída da anterior como entrada. Encadear
capacidades em prova de integração vale mais que uma bateria a mais na mesma.

**Status**: corrigido — filtro por predicado da página, com dois controles
negativos e o efeito medido no roster ao vivo.

## H42 — prazo do PAI estourado é reportado como prazo da OPERAÇÃO, e isso produziu duas conclusões falsas

**Data**: 2026-08-20 · **Contexto**: medição da superfície de eventos de contato
(CAP-10, `onContact`).

**Onde**: `internal/wa-headless/engine/runner.go:74-113`.

```go
ctx, cancel := context.WithTimeout(parent, deadline)
...
timedOut := ctx.Err() == context.DeadlineExceeded
...
if timedOut { return &TimeoutError{Op: k, Label: label, Deadline: deadline} }
```

**O problema**: `ctx` DERIVA de `parent`. Se o prazo do pai já expirou, o
derivado também reporta `DeadlineExceeded`, e o erro sai dizendo

```
StateProbe(probe/contactev): deadline of 5s exceeded
```

quando o que acabou foram os **60 s do chamador**. A mensagem acusa a página de
não responder em 5 s; a página nunca foi consultada.

**A guarda existente NÃO cobre este caso.**
`TestDoDoesNotReportParentCancellationAsTimeout`
(`engine/runner_test.go:126`) protege o cancelamento do pai — e funciona,
porque `context.Canceled` é distinguível. Expiração do pai é
`DeadlineExceeded`, exatamente o valor que a classificação usa. A metade
protegida deu a impressão de que as duas estavam.

**O custo, medido nesta sessão: DUAS conclusões falsas seguidas.**

1. Uma sonda ligou um handler `'all'` na `ContactCollection` e não conseguiu ler
   os próprios resultados: seis tentativas, todas com "deadline of 5s exceeded".
   Conclusão a que cheguei: *o handler saturou a página*. Cheguei a reduzir de
   60 para 8 handlers e escrever o comentário explicando o custo.
2. O controle — mesma espera de 90 s **sem nada ligado** — falhou idêntico.
   Nova conclusão: *a sessão para de responder quando fica ociosa*. Também
   falsa.

A verdade só apareceu com a terceira medição
(`TestProbeIdleResponsiveness`, com contexto de 12 minutos): a página responde
após 90 s de ociosidade em **2 ms**, e responde em todos os intervalos testados
(0 s, 10 s, 20 s, 30 s, 60 s, 90 s). O que expirava era o `nCycleReadyDeadline`
de 60 s que eu havia passado como pai enquanto dormia 90 s dentro dele.

> **Erro meu no harness, e é o mesmo dos dois lados.** Passei um contexto de
> BOOT para uma medição LONGA. Mas um erro de chamador que produz uma mensagem
> apontando para o alvo é precisamente o que esta entrada registra: com a
> atribuição correta, a primeira execução teria dito "seu orçamento acabou" e
> não teria custado duas hipóteses.

**Correção sugerida**: consultar o pai antes de culpar a operação.

```go
if timedOut {
    if parent.Err() != nil {
        return fmt.Errorf("%s(%s): the CALLER's context ended first: %w", k, label, parent.Err())
    }
    return &TimeoutError{Op: k, Label: label, Deadline: deadline}
}
```

**O teste que trava, e o controle negativo que ele exige**: um `Do` cujo pai
tem prazo menor que o da operação, com a asserção de que o erro NÃO é
`*TimeoutError` e nomeia o chamador. O controle é reverter para
`ctx.Err() == context.DeadlineExceeded` e confirmar que volta a sair
`TimeoutError` — porque o teste irmão de cancelamento passa hoje e ainda assim
esta metade está descoberta, que é a própria razão de o controle ser
obrigatório.

**Vale para o registro de observabilidade também**: `rec.Result` recebe
`ResultTimeout` no mesmo ramo, então toda métrica de timeout deste módulo mistura
"o alvo demorou" com "o chamador desistiu". As duas contagens querem ações
opostas.

**Por que não corrigi agora**: está fora do escopo da capacidade em curso, e é
mudança no que a suíte classifica como falha. Pelo `CLAUDE.md`, isso se registra
e se pergunta.

### Corrigido (2026-08-20), pela via ESTRUTURAL

Autorizado pela orquestração, que pediu explicitamente para preferir distinguir
a causa de forma estrutural em vez de inferir por `parent.Err()` — mais robusto
quando os dois prazos coincidem. Go 1.26 tem `context.WithTimeoutCause`, então:

```go
ctx, cancel := context.WithTimeoutCause(parent, deadline, errOperationDeadline)
timedOut     := errors.Is(context.Cause(ctx), errOperationDeadline)
callerGaveUp := !timedOut && ctx.Err() != nil
```

A causa privada nunca chega ao chamador; existe só para o `context.Cause`
separar o relógio desta operação do relógio de quem chamou.

**Observabilidade também**, como pedido: novo `observability.ResultCallerGaveUp`,
distinto de `ResultTimeout`, com `OpLog.CallerGaveUp()` ao lado de `Timeouts()`.
`Timeouts()` deliberadamente não conta o novo — um desligamento que cancela
cinquenta operações em voo reportaria cinquenta timeouts e faria uma parada
limpa parecer incidente.

**Erro novo**: `CallerGaveUpError`, que nomeia o chamador e diz *"the target was
not asked"*. Ele desembrulha para a causa do pai, então `errors.Is` continua
funcionando para quem só quer saber se foi cancelamento ou expiração.

**Os quatro testes exigidos**, em `engine/runner_test.go`:

| caso | exige |
|---|---|
| prazo do pai MENOR que o da operação | não é `*TimeoutError`; `Timeouts()==0`; `CallerGaveUp()==1` |
| cancelamento do pai | idem, e desembrulha para `context.Canceled` |
| prazo genuíno da operação | É `*TimeoutError`; `Timeouts()==1`; `CallerGaveUp()==0` |
| sucesso com pai próximo do fim | nenhum dos dois contadores sobe |

**Controle negativo, EXECUTADO** — revertendo para `ctx.Err() == DeadlineExceeded`:

```
--- FAIL: TestDoBlamesTheCallerWhenTheParentDeadlineExpiresFirst
    the caller's expiry was reported as the operation's own deadline:
    Navigate(parent-expires): deadline of 300ms exceeded
```

**E o irmão de cancelamento CONTINUOU PASSANDO com o defeito de volta** — que é
precisamente a armadilha desta entrada: a metade protegida dava a impressão de
que as duas estavam.

**Status**: corrigido — classificação estrutural por `context.Cause`,
observabilidade separada, quatro testes e controle negativo executado.

## H43 — `onContact` escuta `change`, não `add`: o vocabulário do irmão não serve aqui

**Data**: 2026-08-20 · **Contexto**: CAP-10 (`onContact`).

**A armadilha**: o `capabilities/messagemeta` liga em `add` na coleção de
mensagens e funciona. Copiar essa palavra para contatos instalaria limpo e não
entregaria nada — **a falha sem sintoma**: nenhum erro, nenhum log, só silêncio
indistinguível de "ninguém mudou nada".

**Medido em 90 s sobre o roster ao vivo**, com handlers nomeados para
`add/change/remove/reset/update/sort/sync/destroy`:

| evento | disparos |
|---|---|
| `change` | **7** |
| `add` | 0 |
| `remove`, `reset`, `update`, `sort`, `sync`, `destroy` | 0 |

Pelo catch-all: **35 nomes distintos**, TODOS `change` ou `change:<campo>`,
agrupados em `profilePicThumb` 24, `businessProfile` 22, `change` 7.

A razão é estrutural e vale para qualquer coleção deste tipo: um roster muda por
linha ATUALIZADA, não por linha inserida.

### O auto-teste que tornou a medição confiável

"Nada disparou" é **ambíguo** entre *"este build não tem esse evento"* e *"o
roster ficou quieto"*, e a primeira versão da sonda embarcou essa ambiguidade —
reportou `allSupported: false` com todos os contadores em zero, o que não
distinguia nada.

A sonda passou a disparar um evento privado em si mesma
(`CC.trigger('waHeadlessSelfTest')`) e a verificar que o handler o viu. Viu:
`selfTestSeen: true`. Logo o vocabulário funciona e **os zeros acima são zeros
de verdade**.

**`add` continua ligado assim mesmo.** Noventa segundos quietos não provam que
um contato novo não chegaria por ali, ligar um segundo nome não custa nada, e o
nome do evento viaja com cada evento — então os contadores dirão a verdade
depois, em vez de um comentário ter de adivinhar hoje.

### A prova ao vivo PROVOCA a mudança em vez de esperar

Esperar funcionaria (7 eventos em 90 s ociosos), mas um teste que depende de a
conta alheia estar movimentada falha numa tarde quieta e passa numa barulhenta.

Como 24 dos 46 eventos de campo vinham de `profilePicThumb`, e o
`fetchContactAvatar` escreve exatamente esses campos, **a capacidade de avatar
virou o estímulo**. Resultado: 6 buscas de avatar → **7 eventos `change`, 0
`add`**.

É a segunda prova de integração de um par nesta sessão — a saída de uma
capacidade sendo a entrada de outra, que foi o que pegou a H41.

**Reúso, não segunda lista**: `RowExpr` foi extraída e é a MESMA usada pela
listagem e pela assinatura. Duas extratoras divergiriam — e a pior divergência
seria uma delas parar de aplicar o `isPSA`, trazendo de volta pela outra porta
exatamente o que a H41 tirou. Mesmo motivo do `messagemeta.MetaExpr`.

**Quatro controles negativos, EXECUTADOS:**

```
1. ligar só em 'add' (copiar o irmão) -> the subscription does not bind "change"
2. deixar a sentinela PSA passar      -> the PSA sentinel survived the subscription
3. engolir o flag Reinstalled         -> the subscription was lost and put back, and the drain did not say so
4. marcar evento de linha única como fundido -> a single-row event must not claim to be merged
```

**Status**: entregue — vocabulário medido com auto-teste, prova ao vivo com
estímulo determinístico, quatro controles negativos.

## H44 — comentário que dizia "NÃO VERIFICADO" sobre algo já verificado

**Data**: 2026-08-20 · **Contexto**: leitura do `messagemeta` antes de escrever
o `onContact`.

**Onde**: `internal/wa-headless/capabilities/messagemeta/messagemeta.go`, no
comentário de `eventAdd`.

O comentário dizia que o evento `add` fora tirado do whatsapp-web.js e **NÃO
verificado** neste build, que verificar "exige um humano enviar mensagem para a
conta de laboratório", e que até lá silêncio não deveria ser lido como prova.

Era verdade quando escrito. Deixou de ser em 2026-08-20, quando a segunda conta
foi pareada: o `TestRealSPASendAndReceiveBetweenAccounts` envia de uma e observa
a chegada na outra **sem humano nenhum**, e a metade receptora passa por essa
assinatura, carregando o MESMO id que o emissor devolveu, um segundo depois.

**Por que isto é um achado e não só um typo**: um "não verificado" velho tem
custo próprio. Convida alguém a refazer trabalho que está feito, ou a
desconfiar de um caminho que tem evidência — e neste repositório o comentário é
onde a evidência mora.

**Status**: corrigido — comentário atualizado nomeando o teste que verifica, com
o registro de que a afirmação anterior era correta na data em que foi escrita.

## H45 — `primeContactRoster`: o nome promete o que o mecanismo não faz, e o que ele faz eu quase não medi

**Data**: 2026-08-20 · **Contexto**: CAP-11, a ÚNICA das catorze que o
whatsapp-web.js não tem. Sem referência para copiar, tudo saiu de medição.

### A pergunta barata primeiro: existe lacuna a preencher?

Antes de acionar qualquer sync numa conta real, medi se o roster está incompleto
— usando `WAWebNonAddressBookContactsJob.getAllContactsFromChatCollectionIntoChunks`,
que é LEITURA pura (percorre a `ChatCollection` e filtra pela regra da própria
página, `getIsEligibleForContactSync`).

| medida | valor |
|---|---|
| contatos no roster | 944 |
| chats | 383 |
| contatos referenciados pelos chats | 391 |
| **referenciados que faltam no roster** | **0** |
| política de atualização da própria página | 86 400 s |

**Não há lacuna de pertencimento.** Nada a "preencher".

### Ler antes de chamar, porque a família muta

O enumerador (H37) trouxe `doFullContactSync`, `syncContactList`,
`runSyncDirtyContactsJob`. Os três são invólucros minificados que a fonte não
explica — mas `markContactsSyncCompleted` escreve através de
`LidAwareContactsDB.bulkCreateOrMerge`. Disparar às cegas numa conta seria
experimento no roster de alguém, então li antes.

### O que a chamada realmente faz, medido antes/depois

```
before  944 contatos · getName 1 · pushname 456 · verifiedName 52
after   944 contatos · getName 1 · pushname 456 · verifiedName 56
custo   41,9 SEGUNDOS, devolvendo undefined
```

É **refresh, não fetch**. Não acrescenta pessoas, e não pode inventar nomes de
agenda que o aparelho primário não tem — daí `getName` responder para 1 de 944
antes e depois.

### E aí a prova ao vivo mostrou que eu medi a coisa ERRADA

O teste passou, e o log entregou o que o meu instrumento não via: depois do
refresh, o `List` leu **521 pessoas de 944 linhas, merged=421**, onde antes lia
**544 de 944, merged=398**.

Mais linhas `@lid` haviam ganhado `phoneNumber`, então mais delas fundiram. O
refresh tinha feito algo material — e o meu `PrimeResult` chamou de
`changed=false`, porque eu contava NOMES.

**A correção**: `Snapshot.LidWithPhone` e `PrimeResult.Linked()`. É o número
certo de vigiar porque a aresta `lid → phone` é a ÚNICA ligação cruzada que este
build oferece (0 de 454 linhas de telefone carregam lid — H39), então é dela que
a deduplicação depende. Cada ligação nova é uma pessoa que deixa de ser contada
duas vezes.

**Segunda execução, já com o instrumento certo:**

```
added=0 linked=0 changed=false waited=42,056s
before/after idênticos, lidWithPhone=421 nos dois
```

Primeira chamada faz o trabalho, segunda não tem o que fazer — e ainda cobra 42
segundos. É o estado estacionário, e é por isso que `Changed()==false` é sucesso
e não falha.

> **Ressalva de atribuição, dita porque a medição não a isola.** Os 398 → 421
> foram observados ENTRE execuções, não dentro de uma. Como o `lidWithPhone` só
> passou a existir depois — que é precisamente a omissão — não posso descartar
> que um sync de fundo tenha feito parte. O que a segunda execução prova é o
> estado estacionário; a atribuição do salto é plausível, não isolada.

### A pós-condição é contra DANO, não contra ausência de melhora

A chamada muta. A falha que importa não é "nada melhorou" — é **"o roster voltou
MENOR"**, e é a única que o chamador não consegue detectar sozinho.
`ErrRosterShrank` carrega os dois números.

**Três controles negativos, EXECUTADOS:**

```
1. remover a pós-condição de encolhimento -> got <nil>, want ErrRosterShrank: 944 -> 900 reportado como sucesso
2. tratar "nada mudou" como falha          -> an unchanged roster produced an error
3. validar o contexto DEPOIS do kick       -> kicked the sync 1 time(s) for a caller that had given up
```

O terceiro importa mais aqui que nas outras capacidades: quem desistiu não deve
ter uma mutação de 42 segundos disparada em seu nome.

### O buraco que a orquestração nomeou: o detector não podia falhar

A versão acima tratava "nada mudou" como sucesso. Isso torna
**indistinguíveis** *"o roster já estava atualizado"* e *"o sync nunca
executou"* — um `doFullContactSync` trocado por no-op seria reportado como
refresh saudável. Um detector que não pode falhar não é detector.

**Três candidatos a marca observável, e DOIS falharam na medição:**

| candidato | resultado |
|---|---|
| `isContactSyncCompleted` nas linhas | 944 têm o campo, 913 marcadas — e as **31 pendentes seguem 31** antes e depois de um sync completo. São permanentemente pendentes, não marca por execução. |
| `CONTACT_CHECKSUM` em prefs | **ausente** antes e depois |
| `contact-sync-refresh-seconds` | **muda** |

Parei de adivinhar candidato a candidato e fiz a medição genérica: retrato de
TODO o `localStorage` por hash, antes e depois. De 84 chaves, moveram-se duas —
`WAWebTimeSpentSession` (que se move o tempo todo e nada significa) e
`contact-sync-refresh-seconds`.

**E "mudou depois que eu chamei" não é atribuição.** O controle:

```
leitura                 526473
após 45 s OCIOSOS       526473   <- não deriva sozinho
após doFullContactSync  549772   <- move
```

Apesar do nome, não é configuração: é valor que o refresh REGENERA a cada
execução. E regenera mesmo quando não há trabalho a fazer — nas execuções
medidas o roster já estava atualizado (`linked=0`, `pending` inalterado) e a
marca moveu assim mesmo, que é exatamente o que a torna utilizável.

> **Risco residual, dito em vez de escondido**: um refresh de fundo caindo
> dentro da nossa janela também moveria a marca. O controle limita isso a 45
> segundos de imobilidade observada, não a zero.

**`ErrPrimeDidNotRun`**, verificado ANTES do encolhimento — um refresh que nunca
rodou não pode ter danificado nada, e reportar `ErrRosterShrank` mandaria o
leitor atrás da falha errada.

**Os dois controles negativos que a orquestração exigiu, EXECUTADOS:**

```
A (unitário) remover a checagem de execução
   -> got <nil>, want ErrPrimeDidNotRun

B (AO VIVO, código de produção, SPA real) produção deixa de chamar o sync
   -> Prime: contacts: the refresh returned but the page's sync mark did not move
      (the command returned after 5ms and the roster was left as it was found)
```

O B é o que ela pediu literalmente — provar que o detector falha quando o
priming não acontece — e a mensagem entrega o indício sozinha: **5 ms contra os
42 s reais**.

**Prova ao vivo final**: `ran=true added=0 linked=0 changed=false waited=42,059s`,
roster legível depois (`521 pessoas / 944 linhas / 421 fundidas`).

**Status**: entregue — nascida de medição sem referência, com a promessa do nome
corrigida, o instrumento corrigido depois da prova ao vivo, uma pós-condição
falsificável e cinco controles negativos (três de forma, dois de execução).

## H46 — envio de MÍDIA: quatro assinaturas lidas, duas delas contra o palpite óbvio

**Data**: 2026-08-20 · **Contexto**: primeira capacidade FORA das catorze,
autorizada como expansão funcional. O objetivo declarado do projeto é "métodos
principais de envio de mensagens e chat", e só havia texto.

**Não houve tentativa e erro.** A H34 custou quatro correções medidas exatamente
nesta camada, então desta vez as assinaturas foram LIDAS antes de existir código
— e **duas** delas contrariam o palpite natural:

| chamada | forma REAL | o palpite |
|---|---|---|
| `MediaPrep.prototype.sendToChat` | **um objeto**: `{chat, earlyUpload, options}` | `(chat, options)` |
| opaque data | **`WAWebMediaOpaqueData`** | `WAWebOpaqueData` — que aqui é **NULL** |
| `prepRawMedia(file, opts)` | ramifica em `isPtt`/`asDocument`/`asGif`/`isAudio`/`asSticker`/`asStickerPack` | — |
| `createFromData(data, type)` | Promise; MANTÉM o objeto recebido quando o tipo bate | — |

O nome do módulo de opaque data foi achado lendo a fonte do próprio
`WAWebMediaPrep.getMediaPropsNew`, que o cita. É a H34 de novo: o nome que a
referência usaria não existe aqui.

A quarta linha decide um detalhe visível ao destinatário: como o
`createFromData` preserva o objeto, passar um **`File`** (e não um `Blob`) é o
que faz o NOME DO ARQUIVO chegar. Um Blob chegaria anônimo.

### Reúso em vez de segunda cópia

A resolução de identidade e a obtenção do chat — as quatro correções da H34 —
foram extraídas para `resolveChatExpr`, compartilhado por texto e mídia. Uma
segunda cópia seria um segundo lugar para reaprender aquelas quatro.

O refator mexeu no caminho de PRODUÇÃO do envio de texto, então não bastou a
suíte de dublê: o laço fechado real foi reexecutado e passou, com o mesmo id
casando e 1 inbound alheia ignorada.

### A verificação passou a distinguir o TIPO

Texto e mídia caem na mesma coleção, então "apareceu uma mensagem de saída"
deixa de ser evidência assim que dois tipos podem estar em voo — a mesma forma
de falso positivo da H35, uma camada abaixo. `verify` recebe agora um `kind`, e
`kindOf` mapeia o tipo da página: `chat` é texto; `image`, `document`, `video`,
`audio`, `ptt` e `sticker` são mídia; qualquer outra coisa não serve como prova
de nenhum dos dois.

### Limites, medidos e não estimados

`MaxMediaBytes` é 4 MiB. A página constrói Blob de 8 MB sem reclamar e faz
round-trip de 3 MB por base64 (string de 4 MB), então o teto está ABAIXO do que
foi medido de propósito: é o tamanho para o qual existe evidência, não o tamanho
em que ainda não falhou.

Vazio, sem mime e acima do teto são recusados ANTES de a página ser tocada — o
erro do chamador não deve virar tentativa de upload.

**Prova ao vivo**: conta A envia PNG 1x1 real (70 bytes), conta B recebe o
**MESMO id** com `type=image`, 3 s depois, 0 inbound alheias. O teste recusa
explicitamente um `type=chat` com o id certo: seria a página degradando o anexo
para legenda, e um id-only check aceitaria.

**Quatro controles negativos, EXECUTADOS:**

```
1. verificar mídia com kindAny          -> a text message was accepted as proof that an attachment was sent
2. validar o payload DEPOIS do kick     -> the page was asked to send 1 time(s) with no bytes
3. mandar Blob em vez de File           -> the payload is not built as a File, so the filename cannot survive
4. sendToChat(chat, options)            -> sendToChat is not called with a single object
```

O quarto é o que mede a lição: sem a leitura da fonte, essa teria sido a quinta
correção cega desta camada.

**Status**: entregue — quatro assinaturas lidas antes do desenho, resolução
compartilhada em vez de duplicada, verificação por tipo, e laço fechado real
entre as duas contas.

## H47 — a LEGENDA é aceita e sua entrega é INVERIFICÁVEL aqui, por desenho

**Data**: 2026-08-20 · **Contexto**: fechamento das duas promessas do
`send.Media` — `AsDocument` e `Caption` — logo após a H46.

### `AsDocument`: provado

Os MESMOS bytes enviados duas vezes, uma com a flag e outra sem:

```
inline kind="image"   document kind="document"
```

A comparação usa o mesmo payload de propósito. Enviar arquivos diferentes
deixaria "a flag não fez nada" indistinguível de "PNG é documento".

**Controle negativo, EXECUTADO** — a flag deixa de ter efeito:

```
inline kind="image"  document kind="image"
the SAME bytes produced the same kind "image" with and without AsDocument,
so the flag did nothing
```

### `Caption`: NÃO provado, e não pode ser

A invariante 12 torna este módulo **metadata-only**, e uma legenda é CONTEÚDO de
mensagem. Verificá-la exigiria ler o corpo — exatamente o que a invariante
existe para impedir.

Então o estado honesto é:

- a capacidade ACEITA `Caption` e a repassa à página;
- que ela CHEGUE não é algo que esta suíte possa afirmar;
- e um teste que passasse sem tocar no assunto insinuaria o contrário.

Por isso a limitação está escrita no comentário do teste ao vivo, não só aqui.
Um leitor que veja `TestRealSPASendsADocumentAndTheKindIsObservable` verde
poderia concluir que a legenda foi verificada junto; o comentário nega isso em
voz alta.

**As três saídas, e por que a escolhida é a terceira:**

1. Remover `Caption` da API — pior: a página aceita, é útil, e tirar não torna
   nada mais verdadeiro.
2. Verificar lendo o corpo — quebra a invariante 12, que é um dos pilares do
   módulo, para provar um campo.
3. Aceitar, repassar e **declarar o limite** onde alguém vá ler.

**O que MUDARIA isto**: se algum dia existir prova de entrega de legenda que não
exija ler o corpo — um contador, um flag da própria página, um evento — a
invariante continua de pé e a verificação passa a ser possível. Não foi
procurada nesta sessão.

**Status**: parcialmente corrigido — `AsDocument` provado com controle negativo;
`Caption` declarado inverificável sob a invariante 12, com a razão escrita no
teste e a condição que reabriria o caso.

## H48 — enviar para GRUPO estava quebrado, e falhava com o motivo errado

**Data**: 2026-08-20 · **Contexto**: primeira pergunta ao abrir a área de grupos
— *o envio que já existe funciona para grupo?*

**Não funcionava.** E o modo de falhar era pior que a falha: `NOT_ON_WHATSAPP`,
para um grupo presente na coleção de chats.

**A medição, feita SEM enviar nada:**

| passo | grupo |
|---|---|
| `createWid("…@g.us")` | **funciona**, e mantém `server === "g.us"` |
| `queryWidExists(wid)` | **NULL** — é resolução de USUÁRIO |
| `ChatCollection.get(wid)` | **funciona** direto |

O `resolveChatExpr` mandava todo destinatário pela resolução de identidade, que
existe por causa da H34 (este build é LID-first e o número sozinho não abre
conversa). Um grupo não tem contraparte LID: **o jid do grupo JÁ É a
identidade**, e não há o que resolver.

**Correção**: grupos pulam a resolução inteira e vão direto à coleção. Duas
linhas, mas só depois de medir os três passos acima — a tentação era mexer no
`queryWidExists`.

### Provar isso sem mandar mensagem para pessoas reais

Um grupo tem membros. Uma prova que precisasse ENVIAR para verificar a
RESOLUÇÃO seria uma prova que ninguém pode rodar.

Daí `send.Resolve`: roda a etapa de identidade-e-chat e para exatamente onde o
despacho começaria. Ela compartilha o `resolveChatExpr` com os remetentes de
verdade — é isso que torna uma afirmação sobre `Resolve` uma afirmação sobre
eles, em vez de sobre um caminho paralelo.

**Prova ao vivo**: 1 grupo e 382 individuais na conta; o grupo resolve com
`group=true`, o individual com `group=false`, e o jid do grupo NÃO é reescrito —
porque não há lid para pôr no lugar.

**Controle negativo, EXECUTADO** — removendo o ramo de grupo:

```
Resolve(group): send: the recipient did not resolve to a chat (NOT_ON_WHATSAPP)
```

O sintoma de produção, palavra por palavra.

**O que fica em aberto, e é honesto dizer**: isto prova a RESOLUÇÃO, não o
envio. Provar o envio para grupo exige um grupo em que se possa mandar mensagem
sem incomodar ninguém — ou seja, um grupo criado entre as duas contas de
laboratório. Isso depende da capacidade de CRIAR grupo, que ainda não existe, e
é o próximo passo natural desta área.

**Status**: corrigido — resolução de grupo consertada e provada ao vivo com
controle negativo; o envio para grupo permanece NÃO provado, e a razão está
escrita.

## H49 — criar grupo, e a TERCEIRA vez que "o argumento carrega um id, não É um id"

**Data**: 2026-08-20 · **Contexto**: a H48 provou a RESOLUÇÃO de grupo e deixou o
ENVIO por provar, porque o único grupo da conta é real e tem gente de verdade.
Esta capacidade existe para tornar aquela prova possível.

### O padrão que já custou três vezes num dia

| capacidade | erro | causa |
|---|---|---|
| avatar (H40) | `Cannot read properties of undefined (reading 'isNewsletter')` | passei o wid; queria `{id: wid}` |
| mídia (H46) | — (evitado por leitura) | `sendToChat` recebe UM objeto, não posicionais |
| grupo | `Cannot read properties of undefined (reading 'isLid')` | passei o wid; queria o **modelo de contato** |

A fonte do `getGroupMutationParticipant` decide sozinha:

```js
function c(t, n, r) { var a = t.id.isLid() ? t.phoneNumber : t.id; ... t.username ... }
```

`t.id`, `t.phoneNumber`, `t.username` são campos de CONTATO — nenhum deles existe
num wid. Confirmado nos dois sentidos: com wid lança; com o contato devolve
`{lid, phoneNumber}`. E `ContactCollection.get(wid)` dá a ponte.

**A regra que isto vira**: quando uma função da página morre lendo campo de
`undefined`, o argumento é quase sempre um MODELO, e não a identidade dentro
dele. Vale mais que qualquer nome de módulo — os nomes mudam por build, esta
forma se repetiu três vezes no mesmo dia.

### Duas portas, e a óbvia é a errada

`WAWebCreateGroupAction` é a camada de INTERFACE: a fonte abre um toast pelo
`WAWebToastManager` e monta elementos React. Um driver headless chamando aquilo
estaria dirigindo a UI para alcançar a operação — e quebraria assim que a UI
mudasse. `WAWebGroupCreateJob` é o que aquela ação chama por baixo.

A forma da chamada foi lida do código do PRÓPRIO app, procurando nos bundles
como ele mesmo cria grupo:

```js
const args = {title, thumb: null, full: null, restrict: false, announce: false,
              membershipApprovalMode: false, memberAddMode: false,
              memberShareGroupHistoryMode: false};
const res = await GroupCreateJob.createGroup(args, participants, outContacts);
const gid = WidFactory.asGroupWidOrThrow(res.wid);
```

> **Ferramenta nova, e ela se pagou aqui**: a sonda de módulos da H37 ganhou modo
> GREP — devolve o TEXTO ao redor de um literal nos bundles, não só nomes. Nome
> de módulo responde "o que existe"; isto responde "como se chama", que é a outra
> metade e que nenhuma lista de nomes dá.

### Idempotência não é otimização aqui

Um grupo é ARTEFATO PERSISTENTE na conta de alguém. Sem idempotência, cada
execução de um teste que precisa de grupo deixaria mais um, e quem é dono da
conta acharia uma pilha de grupos quase idênticos que ninguém distingue.

`Ensure` procura por ASSUNTO e só cria se não achar. Provado na mesma execução:
a segunda chamada devolve `created=false` e o mesmo jid.

**Nada aqui apaga grupo.** Remover é ação humana no aplicativo; fazer isso daqui
seria operação destrutiva que ninguém pediu. O nome escolhido diz isso a quem
encontrar: `wa-headless-lab — teste automatizado, pode apagar`.

### A pós-condição

Participante pedido que não aparece no grupo é a falha que o chamador NÃO
consegue ver — ele usaria o grupo acreditando que as pessoas certas estão lá. O
erro diz QUANTOS faltam de quantos foram pedidos, e nunca QUAIS: isso seria
identidade em log.

Os participantes são resolvidos ANTES de criar, e a ordem é contrato: um grupo
criado com membro irresolúvel teria de ser limpo por uma pessoa.

**Prova ao vivo**: grupo criado com 2 participantes; segunda chamada reutiliza;
texto enviado ao grupo e VERIFICADO — que é o que a H48 não podia provar.

**Quatro controles negativos, EXECUTADOS:**

```
1. remover a pós-condição de participante -> got <nil>, want ErrParticipantMissing
2. reportar Created sempre               -> an existing group was reported as newly created
3. criar ANTES de resolver               -> the group is created BEFORE the participants are resolved
4. usar a camada de UI                   -> the script calls the UI action layer
```

**Nota de processo**: a primeira tentativa ao vivo falhou no estágio `create`, e
por isso NENHUM grupo foi criado — o erro veio antes do efeito. Foi sorte da
ordem, não desenho, e é parte do motivo de a resolução vir antes.

**Status**: entregue — criação idempotente por assunto, pós-condição de
participantes, envio a grupo provado ao vivo, quatro controles negativos.

## H50 — presença: o ANÚNCIO entregue, a OBSERVAÇÃO não provada, e três hipóteses derrubadas no caminho

**Data**: 2026-08-20 · **Contexto**: terceira e última área que a orquestração
autorizou (grupos, mídia, presença).

### O que foi entregue

`capabilities/presence`, com anúncio (`composing`/`paused`/`recording`,
`available`/`unavailable`) e observação (`subscribeUserPresence` + leitura).
Dez testes de dublê.

**A camada escolhida é a do APP, não a crua.** `WAWebChatStateBridge` recebe um
wid e manda o protocolo direto; `WAWebPresenceChatAction` recebe um CHAT e
aplica as guardas do próprio aplicativo — `getIsNewsletter`, `id.isBot()`,
`getIsBroadcast` — antes de fazer o mesmo, além de manter os timers de reenvio
que um cliente real mantém. Usar a ponte seria decidir que sabemos melhor que o
app onde um indicador de digitação cabe. Não sabemos.

**Anunciar não cria conversa.** Dizer a alguém que você está digitando não é
abrir um chat com essa pessoa, então a busca é `ChatCollection.get` e nunca
`findOrCreateLatestChat`. Isso forçou uma fatoração melhor: o que `send` e
`presence` compartilham é a RESOLUÇÃO DE IDENTIDADE, não a obtenção do chat.
Ela virou `spa.ResolveIdentityExpr` — no pacote que existe para conhecimento de
página — e os dois a usam para fins opostos sem que um herde o efeito colateral
do outro.

### O que NÃO foi provado

**Anunciar não tem pós-condição local.** A página aceita `markComposing` e nada
muda deste lado, então uma capacidade que "teve sucesso" é indistinguível de uma
que não mandou nada. A evidência só pode vir da OUTRA conta.

E a outra conta não confirmou: `isSubscribed` ficou `true` **uma vez** e `false`
em todas as execuções seguintes, mesmo com 20 s de espera.

**Três hipóteses minhas, todas derrubadas por medição:**

| hipótese | como caiu |
|---|---|
| observar pelo PN | correto derrubá-la — o build arquiva sob LID (H34/H39), e trocar para LID fez a subscrição aparecer na primeira vez |
| a conta não anunciou disponibilidade | `setPresenceAvailable` passou a ser chamado antes; não mudou |
| a página do observador recarrega | **falsa**: marcador plantado sobreviveu 8 leituras em 24 s, contador subindo 1→8 |

A quarta hipótese, que NÃO consegui testar daqui: privacidade. O WhatsApp permite
restringir "visto por último e online" a contatos, e as duas contas de
laboratório não se têm salvas na agenda — `getName` responde para 1 de 944. Se a
subscrição é recusada por privacidade, nenhuma correção de código a faria valer,
e a prova exigiria mudar uma configuração na conta.

### Por que isto fica assim em vez de continuar

Segui a mesma regra da legenda (H47): a capacidade aceita e repassa; o que não
posso afirmar, não afirmo. O teste ao vivo **PULA com a razão escrita**, em vez
de passar sem tocar no assunto — um verde silencioso ali insinuaria que a
observação está provada.

**O que reabriria o caso**: salvar uma conta na agenda da outra e reexecutar. É
ação humana no aparelho, e por isso está registrada aqui em vez de tentada.

> **ATUALIZAÇÃO 2026-08-21 (H91): tentado, e a hipótese ficou MAIS PRECISA em
> vez de resolvida.** A H90 entregou `addressbook.Save`, então salvar uma conta
> na outra deixou de exigir aparelho. Feito, com `syncToAddressbook=false`. O
> teste de presença **continua pulando** com `not subscribed`. Mas a medição do
> registro de contato depois do save diz por quê:
>
> | campo | valor |
> |---|---|
> | `isAddressBookContact` | **1** |
> | `name` / `pushname` / `shortName` | todos presentes |
> | `__x_syncToAddressbook` | false |
> | `__x_isContactSyncCompleted` | **0** |
>
> O contato existe e é de agenda **localmente**. O que não aconteceu foi a
> SINCRONIZAÇÃO — e o filtro de "visto por último: meus contatos" é avaliado no
> servidor, que só conhece o que foi sincronizado. A hipótese quatro deixa de ser
> "talvez seja privacidade" e passa a ser: *o servidor não considera contato quem
> nunca foi sincronizado, e `syncToAddressbook=false` não sincroniza*.
>
> **Nova condição de reabertura, mais estreita**: repetir com
> `syncToAddressbook=true`, que escreve na agenda do TELEFONE físico. Continua
> sendo autorização humana — mas agora por uma razão medida, e não por não haver
> outro caminho.

> **Defeito real encontrado no caminho, e corrigido**: a primeira versão de
> `Observe` subscrevia e lia na MESMA chamada de página. `subscribeUserPresence`
> retorna antes de `isSubscribed` virar, então era corrida — passou uma vez e
> falhou na seguinte, que é o pior tipo de correto. Agora a leitura é repetida
> até a página concordar, com o relógio do lado Go (invariante 6), e há um
> dublê que só vira depois de três leituras para que a espera seja carregada por
> teste.

**Status**: parcialmente entregue — anúncio implementado e travado por dez
testes; observação implementada e NÃO PROVADA contra a conta par, com três
hipóteses derrubadas, uma quarta registrada e a ação humana que a resolveria
nomeada.

## H51 — listar conversas: o getter ÓBVIO responde para 1 chat em 384

**Data**: 2026-08-20 · **Contexto**: maior buraco restante de "chat" — não há
produto de conversa sem listar conversas.

**A armadilha, e ela passaria em toda suíte de dublê:**

| campo | cobertura, medida sobre os 384 chats |
|---|---|
| `WAWebChatGetters.getName` | **1** |
| `formattedTitle` | **384** |
| nenhum dos dois | 0 |

`WAWebChatGetters` EXPORTA `getName`. Alcançá-lo é o movimento natural — e
produziria uma listagem com título para uma conversa a cada 384. Pior: passaria
em todo teste unitário, porque um dublê devolve o que mandarem. É a mesma
armadilha do roster, onde `getName` também respondia para 1 de 944 (H39): num
dispositivo companheiro a agenda vive no telefone, e `formattedTitle` é o que o
app renderiza.

**Por isso a asserção é sobre o SCRIPT**: é o único lugar onde um dublê não pode
mentir. E a prova ao vivo mede COBERTURA de título — 384 de 384 — que é a única
coisa que só uma conta real diz.

> **Correção de uma asserção minha, na primeira execução**: o teste procurava a
> string `getName` e falhou no COMENTÁRIO do script, que explica por que não usar
> `getName`. Trocado para procurar a CHAMADA (`getName(`). Casar texto onde se
> queria chamada é como uma guarda acaba policiando prosa (H32).

**A ordem é NOSSA, não da página.** A coleção foi medida `newest-first`, mas nada
documenta isso, e uma lista cuja ordem muda por baixo é uma que ninguém consegue
comparar. Ordenar custa nada e remove a dependência — mesma lição da H39, onde a
listagem de contatos quase herdou a ordem da página.

O empate quebra pela identidade, porque duas conversas podem compartilhar
carimbo e duas chamadas não podem discordar sobre uma lista que ninguém mudou.

**O limite se aplica DEPOIS da ordenação.** Antes dela, "as dez mais recentes"
devolveria as dez que a página guardou primeiro, que é outra coisa.

**`WithUnread` conta a LOJA INTEIRA, não a página devolvida.** Quem pergunta "há
algo esperando" não está perguntando sobre as dez primeiras conversas.

**Medido ao vivo**: 384 conversas, 384 com título, 2 grupos, **122 com
não-lidas**, ordenação verificada sobre carimbos reais.

**Quatro controles negativos, EXECUTADOS**: usar `getName`; remover a ordenação;
aplicar o limite antes de ordenar; calcular `WithUnread` a partir da página
devolvida.

**Status**: entregue — campo certo escolhido por medição, ordenação e limite
decididos deste lado, denominadores honestos, e quatro controles negativos.

## H52 — marcar como lida: implementado e provado por dublê; o caminho REAL não foi exercitado

**Data**: 2026-08-20 · **Contexto**: lote de operações de mensagem, seguindo o
pedido de cobrir a superfície do `whatsapp-web.js`.

**A forma da chamada, lida do próprio app:**

```js
sendConversationSeen({ chat, key, threadId, unreadDelta })
```

Um OBJETO, e a `key` é o `lastReceivedKey` do chat — a mensagem que está sendo
reconhecida. Reconhecer sem nomear o que se leu não é algo que o protocolo
ofereça, então uma chamada que omitisse a chave pediria algo inexistente.

**Pós-condição real**, o que é mais raro aqui do que parece: `unreadCount` tem de
chegar a zero. A maioria das operações de saída deste módulo só pode ser
verificada pela outra conta; esta muda algo observável deste lado.

**Nove testes e quatro controles negativos, todos mordendo** — inclusive um que
me pegou:

> A asserção "a conversa é resolvida antes da busca" procurava `queryWidExists`,
> que aparece dentro do `spa.ResolveIdentityExpr` e portanto está presente mesmo
> quando a resolução NUNCA é invocada. O controle que removia a chamada passou —
> foi assim que descobri. Trocada para procurar a CHAMADA
> (`await resolveIdentity(`). **Segunda vez no mesmo dia** que uma guarda casou
> prosa em vez de comportamento (a outra foi o `getName` da listagem de chats).

### O que a prova ao vivo NÃO conseguiu

Duas tentativas, e a segunda é a honesta:

1. A primeira passou com `before=0` — a conversa já estava lida, então só o ramo
   "nada a fazer" foi exercitado. **Verde pelo motivo errado**, exatamente o
   falso positivo que este módulo vive encontrando.
2. Corrigido para SEMEAR o não lido: a conta par envia uma mensagem, e o teste
   espera ela aparecer como não lida antes de reconhecer. **Não apareceu em 90
   s.**

Hipótese não confirmada: a SPA pode marcar como lida sozinha quando a conversa
está ativa na sessão, e uma sessão headless pode ter a conversa aberta. Não
medi isso, e por isso está escrito como hipótese e não como causa.

O teste **PULA com a razão escrita** em vez de passar sem exercitar o caminho.

**Cuidado de escopo mantido**: marcar como lida envia RECIBO DE LEITURA a
terceiros. A conta de laboratório tem 122 conversas não lidas com pessoas reais,
e reconhecer qualquer uma delas mudaria o que um estranho vê para fazer um teste
passar. O alvo é só a conta par — a mesma regra que impediu a prova de grupo de
mandar mensagem para um grupo real (H48).

### O padrão que já são TRÊS

| item | por que não foi provado |
|---|---|
| legenda de mídia (H47) | é conteúdo; provar quebraria a invariante 12 |
| observação de presença (H50) | `isSubscribed` não se mantém; hipótese restante é privacidade |
| reconhecimento de leitura (H52) | o não lido semeado não apareceu |

São todos EFEITOS DE SAÍDA cuja evidência mora fora deste processo. Vale dizer
em voz alta: um módulo que dirige uma SPA consegue provar o que ele mesmo lê, e
depende de terceiros para o que ele mesmo escreve.

**Status**: parcialmente entregue — chamada correta lida da fonte, pós-condição
implementada, nove testes e quatro controles negativos; caminho real não
exercitado ao vivo, com as duas tentativas e a hipótese registradas.

## H53 — reagir: a pós-condição teve de ser DESCOBERTA, e as duas metades não são iguais

**Data**: 2026-08-20 · **Contexto**: lote de operações de mensagem.

**Não havia caso para copiar.** Medido antes de escrever: de 395 mensagens da
conta, **ZERO** carregavam reação e a `ReactionsCollection` estava **vazia**. A
pós-condição não podia ser aprendida de um exemplo existente — teve de ser
criada.

**Adicionar: provado.** `hasReaction` vai de `false` a `true` em ~0,5 s. A prova
manda uma mensagem para a conta par e reage à PRÓPRIA mensagem, então nada é
feito na mensagem de outra pessoa.

### Remover funciona, e eu levei três medições para acreditar

A verificação falhou três vezes seguidas com "a reação ainda está lá". As três
vezes eu estava errado, e a página estava certa:

| tentativa | o que li | o que era |
|---|---|---|
| 1 | leitura imediata após o `await` | corrida — li antes de a página aplicar |
| 2 | espera de 30 s pelo `hasReaction` | o campo é **GRUDENTO na sessão**: fica `true` |
| 3 | `getReactionEmojisAndSum().sum` | a forma do retorno não é essa; devolveu nada |

**E a evidência de que a remoção funciona veio de fora da sessão**: TRÊS leituras
em sessões NOVAS mostraram `hasReaction` falso em todas as mensagens e **zero**
linhas na `ReactionsCollection`. O efeito é real e persistido; o que não existe é
como observá-lo de dentro da sessão que o causou.

**A decisão, e ela é de contrato e não de código**: `Result.Verified` distingue
as duas metades. `Add` verifica; `Remove` não, e diz isso. Um `Result` que
escondesse a diferença deixaria "removido" ser lido com a mesma força de
"adicionado", que não é o que a evidência sustenta.

> Polir isso com um `poll` que nunca converge teria produzido **uma capacidade
> que sempre falha em algo que sempre funciona** — pior que não verificar.

**Quatro controles negativos, EXECUTADOS**: verificar também na remoção; remover
a pós-condição do `Add`; passar o id em vez do modelo; aceitar emoji vazio no
`Add` (que é como o protocolo REMOVE, e aceitá-lo faria o oposto do pedido, em
silêncio).

**Ainda respeitando a regra da página**: `WAWebReactionsUtils.canReactToMessage`
é consultado antes, porque existe por um motivo que não conhecemos e contorná-lo
seria pedir ao app que quebre a própria guarda.

**Status**: entregue — adicionar provado com pós-condição descoberta, remover
implementado e declarado NÃO VERIFICÁVEL na sessão, com a evidência de que
funciona e quatro controles negativos.

## H54 — responder citando: a ÚNICA inferência do pacote, e uma leitura errada no caminho

**Data**: 2026-08-20 · **Contexto**: lote de operações de mensagem.

**Todo o resto deste pacote teve a forma LIDA do código do app.** Esta não teve:
os bundles nunca mostram o app chamando `sendTextMsgToChat` COM uma citação,
porque a interface de resposta passa pelo compositor. O que foi lido:

```
createTextMsgData(chat, text, options)   monta o dado da mensagem
quotedMsg                                é o campo que uma resposta carrega
sendTextMsgToChat(chat, text, options)   repassa options a createTextMsgData
```

Passar `{quotedMsg}` pelas opções é **inferência a partir de dois fatos
medidos**, não um fato medido. Por isso a prova ao vivo aqui CARREGA PESO em vez
de confirmar, e o teste diz isso na mensagem de falha, para que a próxima pessoa
não procure no lugar errado.

### A leitura errada, e como ela apareceu

`createQuotedMsgObj` PARECIA a forma de construir a citação. Não é: a fonte
exige `e.quotedStanzaID` e devolve `null` sem ele, porque converte uma mensagem
que **JÁ É** resposta no objeto citado — é da via de renderização.

Passar a mensagem alvo devolveu `null`, e a execução ao vivo falhou com
`QUOTE_NULL`. O valor que `quotedMsg` quer é **a própria mensagem sendo
respondida**.

### A pós-condição, e por que ela é a coisa toda

O texto chega das duas formas. **Só a citação distingue** uma resposta de uma
mensagem comum — e o remetente não vê a diferença sem perguntar. Então `Reply`
verifica que o que saiu carrega citação.

**Controle negativo AO VIVO, EXECUTADO** — produção enviando sem a opção:

```
Reply: send: the message was sent but carries no quote
```

A mensagem SAIU e a pós-condição a recusou. É o controle mais forte que este
pacote tem, porque prova que a verificação distingue exatamente o que a
inferência arriscava.

> **Duas execuções perdidas para o meu próprio dublê, e a lição é de roteamento.**
> TRÊS dos quatro scripts percorrem a coleção de mensagens, então
> `getModelsArray` não os distingue: o kick estava sendo roteado para o ramo de
> verificação, `lastScript` nunca era gravado, e uma asserção sobre o kick
> falhava por motivo que nada tinha a ver com o código sob teste. Cada ramo passou
> a chavear em algo que SÓ aquele script contém.

**Status**: entregue — inferência declarada como tal, leitura errada corrigida e
registrada, pós-condição verificada e controle negativo executado AO VIVO.

## H55 — arquivar/fixar: implementado e travado por teste; a APLICAÇÃO ao vivo é recusada pelo app

**Data**: 2026-08-20 · **Contexto**: lote de estado de conversa.

**O que funciona e está provado por dublê**: `capabilities/chatstate` com
`SetArchived` e `SetPinned`, 12 testes e 4 controles negativos, pós-condição
POLLED (não lida uma vez — a lição da H53), limite de fixados consultado antes
de pedir, e nada que crie conversa.

**O que NÃO funciona**: contra a conta real, `setArchive` responde
**"Could not perform action."** — mensagem do PRÓPRIO app, não uma exceção
nossa. Duas formas tentadas (`setArchive(chat, want)` e
`setArchive(chat, want, true)`, já que a aridade é 3) e ambas recusadas.

Causa **não determinada**. Não vou inventar uma: a mensagem é a recusa
deliberada do aplicativo, e descobrir por quê exige ler o caminho que a produz,
que não foi feito.

### Dois achados de caminho, esses sim resolvidos

**`ChatCollection.get` NÃO basta.** Ele devolve `null` para conversas que
existem — é por isso que o caminho de envio cai no `findOrCreateLatestChat`, e
foi por isso que a primeira versão desta busca reportou `NO_CHAT` para a conta
par que os testes mensageiam todo dia. `findExistingChat` é a metade do par que
NÃO cria, e criar é exatamente o que mudar estado de conversa não pode fazer.

**Quarta ocorrência de "reading 'isNewsletter'"**, desta vez por eu chamar
`getPinLimit()` sem argumento: ele quer um WID. O padrão do dia continua valendo.

### Terceira vez que uma guarda minha casou PROSA

O teste "nunca cria conversa" procurava a string `findOrCreateLatestChat` — e
falhou no COMENTÁRIO do script, que explica por que o caminho de envio a usa.
Trocado para a CHAMADA.

As três, todas hoje:

| guarda | casou |
|---|---|
| listagem de chats: `getName` | o comentário que explica por que não usá-lo |
| marcar lida: `queryWidExists` | a definição do `ResolveIdentityExpr` |
| estado de chat: `findOrCreateLatestChat` | o comentário que explica o fallback |

**A regra, agora com três evidências**: uma guarda que procura uma STRING num
script encontra também os comentários e as definições. Procure a CHAMADA —
`nome(` — ou não procure.

### RESOLVIDO (2026-08-21) — a causa era pedido REDUNDANTE

A orquestração mandou atacar a dívida antes de abrir capacidade nova, e a causa
saiu de um A/B controlado numa única execução contra a conta real:

| pedido | resultado |
|---|---|
| o estado OPOSTO ao atual | **ACCEPTED**, e o flag virou |
| o mesmo estado que já tem | **ActionError: "Could not perform action."** |

`setArchive` **recusa um pedido redundante**. Nunca foi argumento faltando — a
primeira teoria, de que a aridade 3 exigia um terceiro parâmetro, estava errada
e foi descartada pela medição. E a execução ao vivo anterior falhou porque a
conversa ficara arquivada de uma tentativa anterior, então eu pedia exatamente o
que ela já era.

**A correção é a mesma forma do "nada a reconhecer" da H52**: quando o estado
pedido é o atual, não se chama o app. Nada a mudar é um no-op bem-sucedido.

### Três defeitos de encanamento no caminho, todos meus

A releitura separada do flag brigou comigo em três frentes antes de eu perceber
que ela não era necessária:

1. `Chats.get` devolve `null` para conversas que existem — precisa de
   `findExistingChat`;
2. `findExistingChat` é assíncrona, então a resposta teve de ser estacionada;
3. e o laço **re-disparava a leitura a cada volta**, zerando a resposta
   estacionada antes de ela chegar — de modo que o valor nunca aparecia.

**A saída foi apagar as três**: a sonda já mostrara que o flag está atualizado
quando o `setArchive` resolve, então o script de escrita reporta o depois lendo
o objeto que ele mesmo acabou de mudar. Três defeitos em encanamento para um
valor que já estava na mão.

> **E o dublê quebrou DUAS vezes por roteamento**, ambas falhando asserções por
> motivo alheio ao código: quatro scripts rodam neste pacote e três compartilham
> substrings — o de escrita embute o `ResolveIdentityExpr`, que contém
> `createWid(`, e os dois de leitura carregam a mesma chave. Roteamento passou a
> usar marcador ÚNICO de cada script.

**Prova ao vivo, ciclo completo**: arquivar (no-op, já arquivada), desarquivar
(`changed=true`), fixar (`changed=true`), desfixar (`changed=true`). O teste
restaura tudo ao estado inicial.

**Um teste foi REESCRITO em vez de remendado**: o que assegurava que a releitura
usava a identidade resolvida perdeu o objeto, porque a releitura deixou de
existir. A preocupação continua real e agora vive num lugar só — o script
resolve antes de tocar em qualquer coisa — e é isso que ele assere.

**Status**: corrigido — causa medida por A/B, guarda de pedido redundante,
encanamento desnecessário removido, e ciclo completo provado ao vivo.

## H56 — apagar para todos: a primeira capacidade DESTRUTIVA, e ela pergunta antes

**Data**: 2026-08-21 · **Contexto**: fila de capacidades depois de a dívida da
H55 ser fechada.

**É a primeira coisa neste módulo que não tem desfazer.** Tudo o mais acrescenta
ou muda algo recuperável; esta remove uma mensagem do telefone de outras
pessoas. Por isso ela recusa mais facilmente que as outras.

**A forma veio do código do app, e o primeiro argumento é um REGISTRO:**

```js
sendRevoke({type: 'message', data: msg}, revokeType, clearMedia)
```

`revokeType` é `WAWebCmd.Revoke.Sender` ou `.Admin`, e o app escolhe com
`WAWebMsgActionCapability.canSenderRevokeMsg`. **Consultar isso não é
delicadeza**: revogar como Sender quando a conta é só Admin — ou o inverso — é
pedir algo a que ela não tem direito. Se nenhum dos dois vale, a capacidade
recusa com erro PRÓPRIO (`ErrNotRevocable`), porque o chamador pode agir sobre
isso: apagar localmente em vez disso.

> Note a direção: aqui o argumento é um REGISTRO que ENVOLVE a mensagem, ao
> contrário das H40/H46/H49, onde o erro era passar o id quando queriam o modelo.
> A regra não é "sempre passe o modelo" — é **leia a chamada**.

**`clearMedia` é escolha do chamador**, não constante. Destruir também a cópia
local é outra intenção, e assumir a mais destrutiva não é um padrão que este
pacote adota em nome de alguém.

**A pós-condição importa mais aqui do que em qualquer outro lugar do módulo**:
sem ela, o chamador acreditaria que uma mensagem sumiu dos telefones alheios
quando não sumiu, e não haveria como descobrir depois.

**Prova ao vivo**: mensagem enviada pelo próprio teste segundos antes, para a
conta par de laboratório, apagada com `as=sender`. Nada que existisse antes do
teste foi tocado.

**Quatro controles negativos, EXECUTADOS**: remover a pós-condição; escolher o
direito sem perguntar; passar a mensagem em vez do registro; e ignorar a escolha
do chamador sobre `clearMedia`.

**Status**: entregue — forma lida da fonte, direito consultado na página,
pós-condição verificada, alvo restrito ao que o próprio teste criou, e quatro
controles negativos.

## H57 — link de convite de grupo: NÃO ENTREGUE, e o que foi aprendido no caminho

**Data**: 2026-08-21 · **Contexto**: fila de capacidades após a H56.

**Não funciona, e não vou embarcar como se funcionasse.** O código existe em
`capabilities/group/invite.go`, com o cuidado que a coisa merece — um código de
convite é CREDENCIAL, e quem o tem entra no grupo — mas a chamada não passa.

### O que foi medido, e é progresso real

`queryGroupInviteCode` lê `iAmAdmin` da METADATA do grupo, e a metadata não vem
pronta:

| tentativa | resultado |
|---|---|
| `queryGroupInviteCode(wid)` | `reading 'iAmAdmin'` de `undefined` |
| `queryGroupInviteCode(chat)` | idem |
| `queryAndUpdateGroupMetadataById(wid)` antes | `reading 'toString'` de `undefined` |
| `queryAndUpdateGroupMetadataById(chat.id)` antes | **passa**, e o erro volta a ser `iAmAdmin` |

Ou seja: a etapa de metadata **avança** (o estágio saiu de `metadata` para
`query`), e ainda assim o `iAmAdmin` não está onde a função procura.

**Medições auxiliares**: `chat.iAmAdmin` existe; `chat.groupMetadata` existe mas
**não tem** `iAmAdmin`; a coleção de metadata tem 2 linhas e a do grupo de
laboratório está lá.

### Quinta ocorrência do padrão, e a primeira em que ele NÃO explica tudo

`reading '<campo>' of undefined` já apareceu em avatar, mídia, grupo, pin e
agora aqui. Nas quatro primeiras a resposta foi trocar o argumento. **Nesta não
é** — trocar não resolveu, e o que falta é um passo de PREPARAÇÃO que ainda não
identifiquei. A regra do dia continua útil e deixou de ser suficiente.

### Por que parei

Três formas tentadas sem leitura que as sustentasse. Continuar seria a quinta
tentativa cega na mesma camada, que é exatamente o que a H34 registrou como o
modo caro de trabalhar. O próximo passo honesto é LER o caminho que o app usa
para abrir o painel de convite — ele necessariamente popula o que falta — em vez
de tentar mais uma assinatura.

**O teste ao vivo permanece VERMELHO de propósito.** Pular esconderia que a
capacidade não funciona; e o valor do que já foi medido está aqui, para que a
próxima tentativa comece de onde esta parou em vez de do zero.

### A camada, lida de uma vez (2026-08-21)

A orquestração mandou parar de abrir capacidades e entender a preparação de
estado da página, porque três das quatro capacidades anteriores tinham
tropeçado nela. Entendida, e a resposta corrige a leitura de todas:

**`iAmAdmin` NÃO É UM CAMPO. É um MÉTODO na coleção de participantes.**

```js
chat.iAmAdmin = function(){ return this.groupMetadata
    ? this.groupMetadata.participants.iAmAdmin() : false }
```

Então `"Cannot read properties of undefined (reading 'iAmAdmin')"` **nunca quis
dizer** "falta o campo". Quis dizer: a função faz `X.participants.iAmAdmin()` e
recebeu um X que não tem `participants` — ou seja, **o argumento é a METADATA**,
não o chat e não o wid.

Medido lado a lado, com os participantes comprovadamente presentes
(`count: 2`, `iAmAdmin() === true`, `chat.iAmAdmin() === true`):

| argumento | resultado |
|---|---|
| `chat` | lança `reading 'iAmAdmin'` |
| `wid` | lança `reading 'iAmAdmin'` |
| **`chat.groupMetadata`** | **não lança** — devolve `undefined` |

E a etapa de metadata que eu havia acrescentado era **desnecessária**: os
participantes já estavam lá, e o próprio job lança no seu argumento. Foi
removida.

> **A quinta ocorrência não era uma quinta ocorrência.** As quatro anteriores
> foram "passei o id, queriam o modelo". Esta é "passei o dono, queriam a parte"
> — e o erro tem a mesma FORMA porque JavaScript não distingue os dois casos.
> A regra corrigida: `reading '<x>' of undefined` diz **qual objeto falta**, e o
> nome do campo diz **onde procurá-lo** — aqui, `iAmAdmin` vive em
> `participants`, então quem faltava era `participants`.

### O que ainda bloqueia, e é outra coisa

Com a metadata, a chamada passa e devolve `undefined`: o grupo **não tem código
em cache**. Quem o buscaria é `WAWebGroupQueryJob.queryGroupInvite`, e ela **não
retorna** — uma sonda que a chamou não assentou em 90 segundos.

Isso é um bloqueio DIFERENTE do que a entrada começou investigando, e está
nomeado para que a próxima tentativa não recomece pela assinatura.

**A mensagem de erro da capacidade diz isso**, em vez de culpar o chamador por
perguntar.

**Status**: ~~parcialmente entregue~~ **entregue** — fechada no mesmo dia pela
H73 (ver continuação abaixo).

---

## H58 — participantes: as duas irmãs têm assinaturas diferentes, e a remoção continua fechada

**Data**: 2026-08-21
**Contexto**: aposta da orquestração de que entender a camada de metadata do
grupo (a mesma que destravou a H57) destravaria participantes e assunto.
**Onde**: `internal/wa-headless/capabilities/group/participants.go`,
`internal/wa-headless/spa/modules.go` (`ModuleGroupParticipantsJob`).

### O que foi medido, e vale mesmo com o bloqueio

Lidas as duas funções no fonte do bundle, lado a lado no MESMO módulo:

```
addParticipantsJob({group, participants, isOffline, reason})   // UM objeto
removeParticipantsJob(group, participants, timestamp, author,
                      reason, groupMetadata, isOffline)        // SETE posicionais
```

**Este é o achado.** Duas operações simétricas em produto, vizinhas no arquivo,
com formas de chamada incompatíveis. Assumir que a segunda seguia a primeira —
que é o que a simetria sugere — teria sido a sexta correção cega nesta camada.
A regra da H57 vale ao contrário também: **a forma do irmão não prediz a sua.**

O `timestamp` vem do Go, não de `Date.now()` na página (invariante 6).

### O que ainda bloqueia

`removeParticipantsJob` lança, com a assinatura correta:

```
Cannot read properties of undefined (reading 'toString')
```

Duas execuções ao vivo, idênticas, variando só o quarto argumento:

| `author` | resultado |
|---|---|
| `null` | `reading 'toString'` |
| `Me.getMeUserMatchingAddressingModeOrThrow(gwid)` | `reading 'toString'` |

Que o erro NÃO mude entre os dois é a informação: o `.toString()` provavelmente
não é lido do `author`. Qual dos sete argumentos ele lê está por medir — e a
medição é uma sonda que varia UM argumento por vez, não um terceiro palpite.

**Parei antes do terceiro palpite**, que é a disciplina que a H34 registrou e a
H57 aplicou. O caro neste módulo nunca foi o bloqueio; foi a sequência de
tentativas cegas na mesma camada.

### O que a prova ao vivo garantiu mesmo falhando

O `defer` de restauração do teste devolveu o par ao grupo nas duas execuções —
`before=2 after=2 changed=false`. O grupo de laboratório nunca ficou com um só
membro. Efeito externo que falha tem de falhar reversível.

### Correção sugerida

Sonda que chama `removeParticipantsJob` variando um argumento por vez contra o
fonte de `WAWebGroupsParticipantsApi.removeParticipants`, para descobrir de quem
o `.toString()` é lido. Sem isso, qualquer valor novo é chute.

**Status**: ~~não entregue~~ **entregue** (2026-08-21, ver a continuação abaixo).
**Testes que travam o medido**: `participants_test.go` —
`TestTheTwoSiblingsHaveDifferentShapes` (trava as DUAS formas, e falha se
alguém "simetrizar" a remoção), `TestTheClockStaysOnTheGoSide`,
`TestNoOpsAreSuccesses`, `TestACountThatDoesNotMoveIsAFailure`,
`TestOnlyCountsAreReported`, e as recusas.

---

## H59 — bloquear e desbloquear contato

**Data**: 2026-08-21
**Contexto**: varredura da superfície restante do whatsapp-web.js.
**Onde**: `internal/wa-headless/capabilities/block/`,
`internal/wa-headless/spa/modules_block.go`,
`internal/wa-headless/probe_block_test.go`.

### A medição veio antes, e mudou o desenho duas vezes

Uma sonda de enumeração (11,6 s, 65 bundles) destravou **cinco** capacidades de
uma vez ao devolver o módulo de ação E o módulo de verificação de cada uma:

| capacidade | ação | verificação |
|---|---|---|
| bloqueio | `WAWebBlockContactAction` | `WAWebBlocklistCollection` |
| encaminhar | `WAWebChatForwardMessage` | id na conversa destino |
| favoritar | `WAWebChatSendStarMsgsBridge` | `WAWebStarredMsgCollection` |
| silenciar | `WAWebChatMuteBridge` | `WAWebMuteGetters.getIsMuted` |
| editar | `WAWebSendMessageEditAction` | `WAWebMessageEditUtils` |

Depois, `String(fn)` deu as assinaturas exatas — e **as duas discordam**:

```
blockContact({bizOptOutArgs, blockEntryPoint, contact, skipCtwa1pdNbfSignal})
unblockContact(contact, blockEntryPoint)
```

Segunda ocorrência do padrão em dois dias (a primeira foi a H58). Virou entrada
própria no `ARMADILHAS.md`, porque duas vezes em módulos diferentes é
propriedade do código de lá, não coincidência.

E o grep do bundle achou um `throw` que nenhuma assinatura mostraria:

> `[blocklist] trying to block a pn contact (id: …) without a chat`

Bloquear um contato de TELEFONE exige conversa existente. A capacidade checa
isso ela mesma e devolve `ErrNoChatToBlockFrom`, porque "não há conversa com
esta pessoa" é acionável e uma string lançada de dentro da telemetria alheia
não é.

### A prova ao vivo

Linha de base medida ANTES: blocklist com **0** entradas.

```
blocked:  block.Result(before=0 after=1 already=false waited=1.021s)
restored: block.Result(before=1 after=0 already=false waited=1.006s)
```

O bloqueio redundante foi exercitado no mesmo teste e devolveu no-op, não erro
— mesma forma da conversa já arquivada (H55). O desbloqueio roda de um `defer`
registrado ANTES do bloqueio, e o teste diz em voz alta que precisa de humano
se a restauração falhar.

Nada de identidade sai em log: `Result` carrega CONTAGENS.

### Controles negativos EXECUTADOS

| mutação | falha observada |
|---|---|
| `unblockContact` "simetrizado" para objeto | `unblock is called with an object; its source takes two positional arguments` |
| pós-condição da blocklist removida | `got <nil>, want ErrBlocklistUnchanged` |
| deixar o app lançar em vez de checar o chat | `got block: the page refused at apply (NO_CHAT), want ErrNoChatToBlockFrom` |

**Status**: entregue.
**Testes**: `capabilities/block/block_test.go` (11 testes, três com controle
negativo colado acima) e `blockreal_test.go` (prova ao vivo, reversível).

---

## H60 — editar mensagem, e o teste que passava sem morder

**Data**: 2026-08-21
**Contexto**: continuação da superfície do whatsapp-web.js, escolhida por ser a
única das quatro restantes com assinatura completamente legível.
**Onde**: `internal/wa-headless/capabilities/edit/`,
`internal/wa-headless/spa/modules_block.go`,
`internal/wa-headless/probe_edit_test.go`.

### A sonda respondeu três coisas antes de qualquer código

1. **A janela é 1200 s.** `canEditText` deu `false` para as **cinco** mensagens
   `fromMe` mais recentes da coleção — todas de dias atrás. Consequência de
   desenho, não curiosidade: o teste ao vivo tem de **enviar** antes de editar.
   Um teste que reusasse mensagem antiga falharia por motivo alheio ao código.
2. **`isParentWithinEditProcessingWindow` LANÇA** quando recebe a mensagem. Não é
   o portão que o nome sugere. `canEditText` é — e é o que a própria
   `sendMessageEdit` consulta.
3. **`latestEditMsgKey` já existe** em mensagens nunca editadas. Presença do
   campo não prova nada; só o **valor** deixando de ser nulo prova. Um script
   testando `!== undefined` reportaria toda mensagem como editada.

A assinatura veio do `toString()` porque `sendMessageEdit` é **síncrona** —
`sendMessageEdit(msg, text, options)`, três posicionais, e o corpo mostra que ela
abre recusando por `canEditText`/`canEditCaption` com a mensagem inútil
`"Cannot edit message"`.

### A prova ao vivo

```
sent:   send.Result(id=…902 at=2026-08-21T05:39:02Z waited=1ms)
edited: edit.Result(fromLen=42 toLen=51 recorded=true waited=509ms)
```

Dois textos de comprimentos DIFERENTES, para que a pós-condição não possa passar
comparando um corpo consigo mesmo. A recusa de editar mensagem alheia foi
exercitada ao vivo no mesmo teste, contra uma mensagem real de entrada.

### O achado que vale mais que a capacidade: um controle negativo que PASSOU

O CN1 desativou a guarda (`if (!allowed)` → `if (false)`) e o teste **passou**.
A asserção olhava a presença de `Cap.canEditText(msg)` no script; o predicado
continuava calculado e não controlava mais nada.

Corrigido nos dois lugares — a agulha agora nomeia o desvio, não a condição — e
o padrão virou entrada no `ARMADILHAS.md`. O mesmo buraco estava no teste de
pré-condição da H59, escrito no mesmo dia: **buraco achado num teste é buraco nos
irmãos escritos junto.**

### Controles negativos EXECUTADOS

| mutação | falha observada |
|---|---|
| guarda do app desativada (`if (false)`) | *passou* → teste corrigido → `the gate is computed but does not guard a return` |
| `latestEditMsgKey !== undefined` no lugar do valor | `the script tests for the field rather than its value` |
| pós-condição do corpo removida | `got <nil>, want ErrUnchanged` |
| texto vazio liberado | `got <nil>, want ErrEmptyText` |
| (H59, releitura) guarda do chat desativada | `the chat is looked up but does not guard a return` |

Um sexto detalhe, de graça: a primeira versão do teste de ORDEM da guarda
reportou a ordem invertida porque `strings.Index` achou `sendMessageEdit` num
**comentário** do script. É a armadilha "guarda casa com a prosa" já catalogada,
e ela pegou o autor da regra. A agulha virou `A.sendMessageEdit(`.

**Status**: entregue.
**Testes**: `capabilities/edit/edit_test.go` (13 testes) e
`editreal_test.go` (prova ao vivo, com envio próprio e recusa de mensagem alheia).

---

## H61 — favoritar mensagem, e o `await` que não era a conclusão

**Data**: 2026-08-21
**Contexto**: continuação da superfície do whatsapp-web.js.
**Onde**: `internal/wa-headless/capabilities/star/`,
`internal/wa-headless/spa/modules_block.go`,
`internal/wa-headless/probe_star_test.go`.

### A leitura não bastava, então a medição foi EXPERIMENTO

`sendStarMsgs` é async: o `toString()` mostra
`function u(e,t,n){return d(t,n)}` — o bastante para saber que o primeiro
argumento é descartado, e não o bastante para saber o resto. É o limite já
catalogado.

Três formas candidatas foram tentadas **uma de cada vez** contra uma mensagem
real, julgadas por mover a flag. A primeira venceu e as outras nunca foram
usadas:

```
Cmd.sendStarMsgs(chat, [msg], true)
Cmd.sendUnstarMsgs(chat, [msg], true)
```

Experimento é medição legítima quando é barato e reversível, e favoritar é o
efeito externo mais seguro desta superfície inteira: a flag é **local**, o par
nunca a vê.

### A medição contrariou o que eu já tinha ESCRITO

A `PARIDADE-WWEBJS.md` §6.14 dava `WAWebStarredMsgCollection` como pós-condição,
pela força do nome. O experimento mediu a coleção **lançando**: a contagem voltou
`-1` antes e depois de um `star` que demonstravelmente funcionou.

Uma pós-condição construída sobre ela teria aprovado tudo, inclusive o fracasso.
A entrada foi **corrigida**, não remendada — a linha estava errada nas duas
colunas.

### O achado principal, pago com uma falha ao vivo

```
Star: star: the page accepted the change and the flag did not move (before=false after=false)
```

O `await` resolve **antes** de o modelo virar. O mesmo teste, com espera, mediu
**696 ms** até `msg.star` mudar. A sonda só tinha passado porque dormia 800 ms
com `setTimeout` — ela mediu a PÁGINA, a capacidade mediu a PROMESSA.

O conserto não foi dormir na página (funcionaria e violaria a invariante 6): o
modelo vivo é estacionado no estado e o Go relê um booleano a cada volta.
Orçamento esgotado em `settling` devolve `ErrFlagUnchanged`, não "página travou"
— consertos diferentes.

Virou entrada no `ARMADILHAS.md`, porque as H50 e H53 já haviam tropeçado nisto
sem o padrão estar nomeado.

### Prova ao vivo

```
starred:  star.Result(before=false after=true  already=false waited=696ms)
restored: star.Result(before=true  after=false already=false waited=509ms)
```

O no-op redundante foi exercitado no mesmo teste. A restauração roda de um
`defer` registrado antes.

### Controles negativos EXECUTADOS — e DOIS passaram

| mutação | primeira execução | depois de corrigir o teste |
|---|---|---|
| script confia no `await` | **passou** | `the apply branch does not hand the settling decision to Go` |
| pós-condição do Go removida | **passou** (rodei o teste do ramo errado) | `got <nil>, want ErrFlagUnchanged` |
| `settling` indistinto de página travada | `got …never settled…, want ErrFlagUnchanged` | — |
| consultar a coleção medida lançando | `the script consults the collection that was measured throwing` | — |

As duas falhas de controle viraram entrada própria no `ARMADILHAS.md`: **o
controle negativo tem de mutar a camada que o teste observa**, e tem de ser
apontado para o teste do ramo mutado. Um controle apontado para um parente mente
dizendo "está coberto".

**Status**: entregue.
**Testes**: `capabilities/star/star_test.go` (12 testes, quatro com controle
negativo registrado acima) e `starreal_test.go` (prova ao vivo, reversível).

---

## H62 — silenciar conversa, e o argumento que decide se o efeito é real

**Data**: 2026-08-21
**Contexto**: continuação da superfície do whatsapp-web.js.
**Onde**: `internal/wa-headless/capabilities/mute/`,
`internal/wa-headless/spa/modules_block.go`,
`internal/wa-headless/probe_mute_test.go`.

### A camada nomeada pela enumeração era a errada

A sonda de enumeração deu `WAWebChatMuteBridge`, e o grep mostrou a ponte sendo
chamada com um objeto cuja chave é `$MuteImpl3`. A lista de chaves do MODELO
resolve o enigma: `$MuteImpl$p_4`, `$MuteImpl$p_5`, `$MuteImpl$p_6` são artefatos
de minificação de métodos privados. **Passar artefato como contrato** é o chute
que a H58 pagou.

O modelo `Mute` fica acima da ponte, e seus métodos são **síncronos** — logo
integralmente legíveis:

```
mute({expiration, fromMultiselect, isAutoMuted, sendDevice, showToast, toastId})
unmute({fromMultiselect, sendDevice, showToast, toastId})
```

### O achado que decide se a capacidade faz alguma coisa

No corpo do modelo:

```js
if (sendDevice === true) { …alcança a ponte… }
```

**Sem `sendDevice: true` o silenciamento é LOCAL** — e o modelo local ainda
atualiza, então a expiração ainda se move, então **toda** pós-condição continua
passando enquanto as notificações continuam chegando no telefone. É um
meio-sucesso silencioso embutido na API, e nada além de uma asserção explícita
sobre o argumento o pega. `TestSendDeviceIsTrue` é essa asserção.

Segundo fato do mesmo corpo: `expiration` tem de ser número, senão a chamada
rejeita com `ActionError`, e o app registra "wrong units?" acima de 2e9 — que é
como ele diz **segundos de época**.

### Prova ao vivo

```
muted:       before=0          after=1787321477 muted=true  waited=507ms
second mute: before=1787321477 after=1787321478 muted=true  waited=511ms
restored:    before=1787321478 after=0          muted=false waited=506ms
```

O segundo mute movendo **um segundo** é confirmação de que a expiração é
recalculada do relógio a cada chamada — informação que só a repetição dá.

A lição do `await` (H61) foi aplicada aqui **antes** de custar uma segunda
falha ao vivo: o modelo é estacionado e o Go relê.

### Controles negativos EXECUTADOS — e um MENTIU

| mutação | falha observada |
|---|---|
| `sendDevice` omitido | `mute does not pass sendDevice: true; the change would be local only` |
| script confia no `await` | `the apply branch does not hand the settling decision to Go` |
| `canMute` computado sem desviar | `canMute is computed but does not guard a return` |
| pós-condição do ramo `done` removida | `got <nil>, want ErrExpirationUnchanged` |
| `Always` sem a sentinela | **passou** → a mutação não se APLICOU (texto concatenado no fonte Go) → refeita com `assert` → `Always does not reach the page's sentinel path` |

O quinto virou entrada no `ARMADILHAS.md` e fecha o trio de modos de um controle
negativo mentir, os três medidos neste mesmo dia: mutar a camada errada, apontar
para o teste do ramo vizinho, e **não se aplicar**. Os três se relatam com a
mesma palavra — "passou".

**Status**: entregue.
**Testes**: `capabilities/mute/mute_test.go` (12 testes, cinco com controle
negativo registrado acima) e `mutereal_test.go` (prova ao vivo, reversível).

---

## H63 — encaminhar mensagem, e a assinatura que só o chamador tinha

**Data**: 2026-08-21
**Contexto**: última das quatro capacidades da superfície do whatsapp-web.js
mapeadas pela sonda de enumeração.
**Onde**: `internal/wa-headless/capabilities/forward/`,
`internal/wa-headless/spa/modules_block.go`,
`internal/wa-headless/probe_forward_test.go`.

### As três técnicas foram necessárias, nesta ordem

Esta capacidade é a que exercita o catálogo inteiro do `ARMADILHAS.md`:

1. **`toString()` falhou.** As duas funções exportadas são invólucros async de um
   argumento opaco — `function v(e){return S.apply(this,arguments)}`.
2. **A camada do modelo não existia.** Estrelar e silenciar foram resolvidos
   subindo para o modelo; aqui a sonda mediu `chatForwardMethods: []`. O modelo
   do chat **não tem** método de encaminhar, então a saída das duas capacidades
   anteriores estava fechada.
3. **O chamador respondeu.** Grep no bundle por `forwardMessagesToChats(`
   devolveu o objeto que o próprio app monta:

```js
forwardMessagesToChats({msgs, chats, includeCaption, appendedText})
forwardMessages({chat, msgs, multicast, includeCaption, appendedText})
```

`chats` é array de **modelos de chat** — o chamador o constrói a partir de
`findOrCreateLatestChat`, não de ids.

### Onde divergimos do app de propósito

O fluxo do app usa `findOrCreateLatestChat`; **nós não**. Abrir conversa com
alguém para reenviar-lhe uma mensagem é ato maior do que encaminhar, e o
chamador não pediu isso. Chat não carregado é `ErrNoChat`, e há teste.

### A pós-condição é estrita porque já houve falso positivo

Uma verificação de laço fechado neste módulo já aceitou mensagem de um
desconhecido chegada **23 segundos ANTES** do envio. Por isso a cópia tem de
satisfazer **três** condições: ser nossa (`fromMe`), estar no chat de destino, e
ter id **ausente do conjunto** capturado antes da chamada. Comparação por
timestamp está proibida em teste.

### Prova ao vivo

```
sent:      send.Result(id=…5302 at=2026-08-21T06:22:15Z waited=2ms)
forwarded: forward.Result(newID=…65BB bodyLen=45 waited=512ms)
```

O teste falha explicitamente se `newID == sourceID` — devolver a origem como
cópia é o modo de falhar de qualquer verificação frouxa. A recusa de chat
desconhecido foi exercitada ao vivo.

Encaminha uma mensagem que ele mesmo acabou de enviar, de volta ao MESMO chat de
laboratório: encaminhar conteúdo alheio a terceiros é a versão deste ato com
custo real de privacidade, e o laboratório tem exatamente duas contas.

### Controles negativos EXECUTADOS

Todos com `assert` de que a mutação se aplica — a lição da H62.

| mutação | falha observada |
|---|---|
| identificar a cópia por timestamp | `the copy check is missing the "s.seen.has(m.id.id)" condition` |
| criar o chat inexistente | `the script creates a chat that did not exist` |
| achatar o campo `reasons` | `the script does not read the reasons field off the error` |
| script confia no `await` | `the apply branch does not hand the settling decision to Go` |
| aceitar cópia vazia | `got <nil>, want ErrNotDelivered` |
| não consultar `canForward` | `the script does not consult the message's own forwardability` |

**Status**: entregue.
**Testes**: `capabilities/forward/forward_test.go` (13 testes, seis com controle
negativo acima) e `forwardreal_test.go` (prova ao vivo).

---

## H64 — renomear grupo, e um teste que falhou com o efeito correto aplicado

**Data**: 2026-08-21
**Contexto**: último módulo não localizado da superfície do whatsapp-web.js.
**Onde**: `internal/wa-headless/capabilities/group/subject.go`,
`internal/wa-headless/spa/modules_block.go`,
`internal/wa-headless/utf16len_test.go`.

### A assinatura foi legível de primeira

`WAWebSetSubjectGroupAction.setGroupSubject` tem invólucro **síncrono**:

```
setGroupSubject(chat, subject = "")
```

**O default é o problema.** Chamar sem o segundo argumento **apaga o nome do
grupo**. A capacidade passa o assunto explicitamente e recusa string vazia com
`ErrEmptySubject` — há controle negativo para as duas coisas.

**Sem palpite de admin**: renomear é governado por uma configuração por grupo que
pode permitir qualquer membro, então recusar por conta própria negaria um ato que
o grupo permite. A recusa da página passa adiante.

### A pós-condição é AUTO-MEDIDORA, e a medição saiu

Qual campo do modelo carrega o assunto neste build nunca havia sido medido. Em
vez de escolher um, o script tenta `subject`, `name` e `formattedTitle`, e
**reporta qual moveu**:

```
MEASURED: on this build a group's subject lives in chat.formattedTitle
```

`Rename.Field` existe para isso. Uma capacidade que não sabe dizer onde olhou não
pode ser conferida pela próxima pessoa.

### A falha que valeu mais que o sucesso

```
renamed: group.Rename(fromLen=49 toLen=72 field=formattedTitle already=false waited=1.019s)
the new subject's length is 72 and 74 was asked for
```

**O rename tinha funcionado.** A asserção comparava `len()` do Go (bytes) com
`.length` do JS (unidades UTF-16), e o travessão do assunto do grupo de
laboratório custa 2 de diferença.

Pior desfecho possível: asserção errada em teste ao vivo gasta uma execução
inteira para não dizer nada, e manda a próxima pessoa depurar a capacidade certa.

**E estava latente em outros dois lugares** — os testes ao vivo de editar (H60) e
encaminhar (H63) fazem a mesma comparação e passavam por serem ASCII. Corrigidos
os três com `utf16Len`, que tem teste próprio com casos que DIFEREM de `len()`
(travessão: 1 unidade / 3 bytes; emoji: 2 unidades / 4 bytes) para não virar
sinônimo num refactor. Entrada no `ARMADILHAS.md`.

### Restauração que não é cosmética

Todo teste ao vivo de grupo acha o grupo de laboratório **pelo assunto**. Uma
execução que renomeasse e parasse faria a próxima **criar um segundo grupo**. O
`defer` diz isso na mensagem de erro.

### Controles negativos EXECUTADOS

Todos com `assert` de aplicação.

| mutação | falha observada |
|---|---|
| usar o default vazio do app | `the call does not pass the subject explicitly` |
| deixar passar assunto vazio | `got <nil>, want ErrEmptySubject` |
| assumir um único campo | `the result script does not consider 'name'` |
| script confia no `await` | `the apply branch does not hand the settling decision to Go` |
| `settling` indistinto de travamento | `got …never settled…, want ErrSubjectUnchanged` |

**Status**: entregue.
**Testes**: `capabilities/group/subject_test.go` (10 testes, cinco com controle
acima), `subjectreal_test.go` (prova ao vivo, restaura) e
`utf16len_test.go::TestUTF16LenMatchesJavaScript`.

---

## H58 (continuação) — resolvida, e por duas causas que nada tinham a ver com a assinatura

**Data**: 2026-08-21, mesmo dia.

O bloqueio tinha DUAS causas empilhadas, e nenhuma era o `author` que as duas
tentativas cegas atacaram.

### Causa 1 — participantes são REGISTROS, não wids

Achada pela técnica que resolveu o encaminhar: **procurar o chamador do app**.

```js
removeParticipantsJob(n, a.participants, x, t.author, a.reason, r, i)
```

e a linha imediatamente acima, na mesma função, faz
`a.participants.some(e => e.id)`. Cada entrada carrega `.id`. Eu passava
`[wid]`, e algo lá dentro fazia `.toString()` no `.id` inexistente.

**O erro idêntico nas duas tentativas era a evidência**, e ela estava na entrada
original desde o começo: variar só o `author` e obter a MESMA mensagem significa
que o `author` não é lido ali. Registrei isso e depois não agi conforme —
a informação estava escrita e a próxima ação não a usou.

### Causa 2 — este build não confirma mudança de participante na mesma sessão

Esta custou mais que a primeira, porque produziu **três diagnósticos errados
seguidos** e deixou o grupo de laboratório com um membro por uma tarde.

Medido, com sessão nova servindo de fonte da verdade:

| sinal | o que reportou durante 90 s | verdade (sessão nova) |
|---|---|---|
| `chat.groupMetadata.participants` | contagem antiga | mudada |
| `GroupMetadataCollection.get()` | idem — **é o mesmo objeto** | idem |
| `MsgCollection` (`gp2/add`, `gp2/remove`) | **nenhuma** notificação | — |

O `gp2/subject` de um rename CHEGOU na mesma coleção, então o canal funciona e é
esta notificação específica que não é entregue aqui.

**Três mudanças reais foram para o servidor** enquanto os testes diziam
"membership did not change". A pós-condição estava mentindo na direção pior:
dizendo que nada aconteceu quando tudo tinha acontecido.

### O contrato passou a dizer a verdade

`Membership.Verified` é **false** para toda mudança real, e true só para no-op —
o contrato assimétrico que a H53 já usava para remoção de reação. E
`Manager.Count` existe para a única prova honesta disponível: **mudar numa sessão
e contar na seguinte**.

```
removed:       group.Membership(before=2 wantedAfter=1 noop=false verified=false waited=1.024s)
CROSS-SESSION: 2 participants before the removal, 1 after
restored:      group.Membership(before=1 wantedAfter=2 noop=false verified=false waited=1.011s)
```

### A versão de sessão única não era só mais fraca — era INSEGURA

Remover e readicionar na mesma sessão faz a segunda chamada ler a metadata
obsoleta, concluir `ALREADY_MEMBER` e **não fazer nada**. Foi assim que o grupo
ficou com um membro: o `defer` de restauração reportou sucesso sem restaurar. Por
isso cada passo do teste tem sessão de navegador própria, e o comentário do teste
diz isso em voz alta.

**Status**: entregue.
**Testes**: `capabilities/group/participants_test.go` (incluindo
`TestParticipantsArePassedAsRecords`, `TestARealChangeIsReportedUnverified`,
`TestTheScriptDoesNotWaitOnSomethingThatNeverMoves`) e
`participantsreal_test.go` (prova entre sessões, com restauração em sessão
própria).

---

## H65 — promover, rebaixar e sair de grupo

**Data**: 2026-08-21
**Contexto**: Fase 1, cobertura de funcionalidades do whatsapp-web.js.
**Onde**: `internal/wa-headless/capabilities/group/admin.go`.

### Uma TERCEIRA forma no mesmo módulo

```
addParticipantsJob({group, participants, isOffline, reason})              // 1 objeto
removeParticipantsJob(group, participants, timestamp, author, reason,
                      groupMetadata, isOffline)                           // 7 posicionais
promoteParticipantsJob(group, participants, groupMetadata, isOffline)     // 4 posicionais
```

Três assinaturas para quatro operações irmãs, exportadas lado a lado. O corpo do
promote é **síncrono** e mostra o objeto que ele monta, então esta veio de
graça. `TestAThirdShapeInTheSameModule` falha se alguém uniformizar.

As lições da H58 foram **carregadas, não redescobertas**: participante é
registro (`[found]`, o modelo da metadata), e `Verified` é false para mudança
real.

### A prova ao vivo é um único booleano, e ele não pode ser satisfeito por engano

A sessão dois lê metadata fresca. Se a promoção pegou, o par é admin lá, então o
rebaixamento é mudança real e `NoOp` é **false**. Se a promoção não fez nada, o
par continua membro comum, o rebaixamento acha o papel que quer já no lugar, e
`NoOp` é true.

```
promoted: group.AdminChange(promoted=true  noop=false verified=false waited=1.014s)
demoted:  group.AdminChange(promoted=false noop=false verified=false waited=1.012s)
```

É exatamente a forma de verificação que faltou à H58 por uma tarde.

### Sair do grupo: entregue, e deliberadamente NÃO provado ao vivo

`sendExitGroup(chat)` — um argumento, modelo. Implementado, com recusa quando a
conta não é membro (um no-op silencioso ali deixaria o chamador acreditando ter
saído de algo em que nunca entrou).

**Não há prova ao vivo, e a omissão está escrita em três lugares** (comentário do
módulo, comentário da função, comentário do teste): uma conta que sai de um grupo
que criou não consegue voltar sem convite de quem ficou dentro, e o laboratório
tem duas contas. Rodar uma vez custaria o grupo de que todos os outros testes ao
vivo de grupo dependem.

**Status**: entregue (sair do grupo: entregue sem prova ao vivo, por desenho).
**Testes**: `capabilities/group/admin_test.go` (11 testes) e
`adminreal_test.go` (promover/rebaixar, prova entre sessões, restaura).

---

## H66 — limpar e apagar conversa, e o nome de exibição que este build recusa

**Data**: 2026-08-21
**Contexto**: Fase 1, cobertura.
**Onde**: `internal/wa-headless/capabilities/chats/lifecycle.go`,
`internal/wa-headless/capabilities/profile/`.

### O achado que vale além destas capacidades: a conta de laboratório é BUSINESS

`Conn.canSetMyPushname()` é `!getIsSMB(this)` e mediu **false**. Ou seja, a
conta-A é uma conta WhatsApp Business.

Isto é propriedade do **fixture contra o qual todas as medições deste módulo
foram tomadas**, e vale mais que a capacidade que o revelou. Comportamentos
medidos aqui podem não valer para conta pessoal, e o contrário também.

A capacidade existe, pergunta a guarda ANTES e devolve `ErrCannotSetDisplayName`
com o motivo. **Não há prova ao vivo possível nesta conta** — não por escolha,
por recusa do build.

### Limpar e apagar: entregues, deliberadamente NÃO provadas ao vivo

```
sendClear(chat, keepStarred)
sendDelete(chat, syncToDevices = true)
```

Ambas síncronas, ambas confirmadas no chamador do app. O segundo argumento do
`sendClear` foi nomeado pela UI que o chama: um checkbox sob as palavras *"a
conversa ficará vazia, mas continuará na sua lista"*.

**Por que não são provadas ao vivo**: a conta de laboratório tem exatamente uma
conversa com par, e todo o resto dos testes ao vivo lê ou escreve nela —
`fetchmessages` conta o histórico, editar e encaminhar acham mensagens nela,
silenciar e marcar-como-lida agem sobre ela. Limpar ou apagar uma vez custaria
todos eles, e nenhum dos dois atos se desfaz.

Uma capacidade que existe e **diz** que nunca foi provada ao vivo é mais honesta
que uma provada às custas do fixture.

**Divergência registrada**: o fluxo do app faz `sendExitGroup` e depois
`sendDelete`. Nós **não** saímos do grupo ao apagar a conversa — apagar a
conversa continuando no grupo é estado real que alguém pode querer, e fazer o
passo extra em silêncio seria decidir pelo chamador. Há teste.

### Não entregue, e medido: recado (`about` / status de texto)

`WAWebSetAboutJob.setAbout` e `WAWebSetTextStatusJob.setTextStatus` **não são
funções**. Medidos duas vezes:

```
{type: "object", isArray: true, length: 1, zeroType: "object", ctor: "Array"}
```

São **arrays de um elemento**, e o elemento é um objeto, não uma função — um
descritor de job de ligação preguiçosa. Chamar qualquer coisa ali às cegas é o
chute que a H58 pagou.

Parei na segunda sonda, de propósito: a regra é medir antes de projetar, e
também não cavar. O que está medido basta para a próxima pessoa começar de onde
isto parou, em vez de refazer as duas sondas.

**Terceira medição, 2026-08-21** — e ela achou a API certa e um bloqueio novo.
O caminho real não é `WAWebSetAboutJob`: é
`WAWebTextStatusAction`, que exporta `getTextStatus` e `setMyTextStatus`. A
LEITURA foi entregue na H70. A ESCRITA continua bloqueada, por um motivo
diferente e mais duro:

```
setMyTextStatus(e, t, n, r, o)      // cinco posicionais
```

e o grep no bundle **não acha nenhum chamador**. A função é exportada e nada no
código embarcado a chama — ou o chamador vive num bundle carregado sob demanda
que a sonda não busca. Sem chamador, as cinco posições são cinco chutes, e a H69
acabou de registrar o que chutar assinatura custa.

**Não delegável a mais uma sonda cega**: o próximo passo útil é abrir a tela de
edição de recado na página real e capturar a chamada, o que exige interação de
UI que este módulo não faz.

**Status**: entregue (limpar, apagar, nome de exibição — este sem prova ao vivo
possível); recado **não entregue**, com a medição preservada acima.
**Testes**: `capabilities/chats/lifecycle_test.go` (9 testes, três com controle
negativo asserido) e `capabilities/profile/profile_test.go` (8 testes).

---

## H67 — baixar mídia, e a primeira pós-condição criptográfica do módulo

**Data**: 2026-08-21
**Contexto**: Fase 1, cobertura.
**Onde**: `internal/wa-headless/capabilities/media/download.go`.

### A pós-condição que nenhuma página consegue falsificar

Em todo o resto deste módulo a pós-condição **pergunta à página** se algo
aconteceu e precisa confiar na resposta. Aqui não: `filehash` no modelo da
mensagem é o **SHA-256 do texto claro**, então bytes que decifram para outra
coisa reprovam num teste que nenhum comportamento da página forja.

E o teste ao vivo acrescenta a metade que a capacidade não pode fazer sozinha —
ele **sabe** o que o texto claro deveria ser. Uma página que devolvesse um
arquivo diferente cujo `filehash` batesse com o próprio conteúdo passaria na
verificação da capacidade e reprovaria na do teste.

```
downloaded: media.Attachment(bytes=1024 mime=application/octet-stream sha256=6fd10114… shape=arraybuffer waited=504ms)
MEASURED: on this build downloadAndMaybeDecrypt returns a arraybuffer
refused a text message: media: that message carries no attachment
```

### O argumento que ninguém adivinharia: telemetria obrigatória

Primeira tentativa ao vivo:

```
TypeError: Cannot read properties of undefined (reading 'addAnnotations')
```

`addAnnotations` vive num **QPL** — o objeto de log de performance. O
`downloadQpl` é **obrigatório**, e todo chamador do app passa um. Encanamento de
telemetria se apresentando como parâmetro obrigatório é a espécie menos
adivinhável que existe.

**A regra corrigida da H57 leu isto certo de primeira**: o nome do campo diz
ONDE o objeto que falta mora. Uma leitura, uma correção, sem tentativas cegas.

### O que veio de volta nunca tinha sido medido

O script aceita `ArrayBuffer`, typed array e `Blob`, e **reporta qual recebeu**
(`Attachment.PageShape`). Mediu `arraybuffer`. Uma capacidade que não sabe dizer
o que recebeu não pode ser conferida.

### Controles negativos EXECUTADOS — e dois falharam nos modos já catalogados

| mutação | primeira execução | depois |
|---|---|---|
| não verificar o hash | **não compilou** (`want` não usado) | `got <nil>, want ErrHashMismatch` |
| omitir `downloadQpl` | `the download omits downloadQpl` | — |
| assumir uma única forma de retorno | `the script does not handle the typedarray shape` | — |
| baixar antes de recusar por tamanho | **passou** — o teste asseria a ORDEM, e posição não é imposição | `the declared size is read but does not guard a refusal` |
| renderizar os bytes | `the rendering carries the payload` | — |

Os dois que falharam são os dois modos já no `ARMADILHAS.md` — controle que não
compila, e agulha no predicado em vez do desvio. Ambos foram pegos porque os
controles foram **executados**.

**Status**: entregue.
**Testes**: `capabilities/media/download_test.go` (11 testes, cinco com controle
acima) e `downloadreal_test.go` (ida e volta byte a byte contra a página viva).

---

## H68 — grupos em comum, e o `null` que significa outra coisa

**Data**: 2026-08-21
**Contexto**: Fase 1, cobertura. Capacidade de **leitura** — nada é enviado.
**Onde**: `internal/wa-headless/capabilities/contacts/commongroups.go`.

### O corpo diz três coisas que só se aprenderiam por acidente

`findCommonGroups(contact)` tem invólucro síncrono, então o corpo é legível
inteiro:

1. **Devolve `null` para o contato DESTA CONTA** — `Promise.resolve(null)` sob
   `getIsMe(contact)`. Isso não é resposta vazia, é **recusa**. Achatar as duas
   faria o chamador ler *"você não compartilha grupos consigo mesmo"* como fato,
   quando é erro de categoria. `ErrIsSelf` mantém a distinção, e há controle
   negativo.
2. **Exclui grupos-pai (comunidade) e travados.** A resposta é "grupos onde
   vocês dois poderiam conversar", que é quase sempre o que se quer — mas a
   diferença fica **dita**, em vez de invisível.
3. **Guarda em cache no contato e reaproveita promessa pendente.** Perguntar
   duas vezes é barato; perguntar depois de mudança de participação pode não ser
   fresco — o que importa neste build, já medido como não atualizando metadata
   de grupo na mesma sessão (H58).

### Prova ao vivo

```
with the peer: contacts.CommonGroups(count=1 waited=510ms)
asking about this account was refused, as the page's null requires
```

`count=1` é o número **esperado**, não meramente plausível: a conta e o par
compartilham exatamente o grupo de laboratório, e o teste confere que o jid
retornado é ele, achado por assunto na mesma sessão.

### Um detalhe do teste que virou correção

A primeira execução **pulou** a asserção do "eu mesmo" porque a leitura do jid
próprio tentava só `getMaybeMeUser` e `getMeUser`, e este build não expõe o
segundo. Um `t.Skip` silencioso é uma asserção que não roda — corrigido com a
lista de fallback que uma sonda anterior já tinha medido.

**A lição pequena**: quando um teste tem caminho de `Skip`, ele pode silenciar a
própria verificação. Vale conferir o que o `Skip` engoliu antes de aceitar o
PASS.

### Controles negativos EXECUTADOS

| mutação | falha observada |
|---|---|
| achatar `null` em lista vazia | `the script does not treat null as its own answer` |
| passar o wid em vez do modelo | `the call does not pass the contact model` |
| renderizar a lista de grupos | `the rendering lists the groups: …@g.us…` |

**Status**: entregue.
**Testes**: `capabilities/contacts/commongroups_test.go` (7 testes, três com
controle acima) e `commongroupsreal_test.go` (leitura ao vivo).

---

## H69 — enquete: não entregue, e o erro estava na minha leitura

**Data**: 2026-08-21
**Contexto**: Fase 1, cobertura.
**Onde**: `internal/wa-headless/capabilities/send/poll.go`,
`internal/wa-headless/pollreal_test.go`.

### O que falhou

Duas execuções ao vivo, **erro idêntico**:

```
Cannot read properties of undefined (reading 'name')
```

A primeira com `correctOptionKey: null`, a segunda com o campo **omitido**. Pela
regra que a H58 pagou caro: **erro que não muda quando o argumento variado muda é
evidência de que o argumento variado não é a causa.** Duas tentativas bastaram
para saber disso — foi o que a H58 levou uma tarde para aprender.

### O que a segunda falha revelou, e é a lição

Eu li o chamador da UI:

```js
const r = b({correctOptionKey: P, filteredOptions: n, isPhotoPoll: ye,
             isSingleOption: D, pollEndTime: w?O:null, pollType: R,
             question: k, hideVoterNames: q});
sendPollCreation({poll: r, chat: i, quotedMsg: …, isWamoSub: …});
```

e tomei a lista de argumentos de `b` como sendo a de
`createPollCreationMsgData`. **Não é.** O dado de mensagem que a função da API
produz carrega `correctOptionIndex`; o objeto que copiei diz `correctOptionKey`.
Nomes diferentes são funções diferentes — `b` é um ajudante local da UI, e eu o
confundi com a API.

**A técnica "leia o chamador do app" continua certa**; o que faltou foi
**confirmar que o chamador chama a função que eu ia chamar**. É um passo, e ele
não estava escrito em lugar nenhum.

### O que ficou medido e vale

| fato | de onde |
|---|---|
| uma opção é `{name, localId}` | caminho de merge de opção adicionada, que constrói isso e indexa um `Set` por `option.name` |
| a mensagem carrega `pollName`, `pollOptions`, `pollSelectableOptionsCount`, `pollContentType`, `pollType`, `correctOptionIndex` | construtor do dado de mensagem |
| `sendPollCreation({poll, chat, quotedMsg, isWamoSub})` | chamador da UI — **este** é da API |

**O que falta**: o que `createPollCreationMsgData` desestrutura. Exige a cabeça
do corpo do gerador, que o grep do bundle truncou.

### Por que o código fica

Ele codifica as três medições acima e os testes as travam — inclusive
`TestAnOptionIsAnObjectNotAString`, que impede a próxima pessoa de repetir o
palpite óbvio (strings). Não está ligado a nada. O teste ao vivo fica
**vermelho de propósito**, atrás do seu próprio interruptor, para que a próxima
tentativa tenha um harness em vez de uma página em branco.

**Status**: ~~não entregue~~ **entregue** — destravada pela H73, no mesmo dia.
**Testes**: `capabilities/send/poll_test.go`.

---

## H70 — ler o recado de um contato, e um PASS que não prova o que parece

**Data**: 2026-08-21
**Contexto**: Fase 1, cobertura. Capacidade de **leitura**.
**Onde**: `internal/wa-headless/capabilities/contacts/about.go`.

### A regra nova da H69 foi aplicada e funcionou

O chamador foi procurado **qualificado pelo módulo**:

```js
o("WAWebTextStatusAction").getTextStatus(contact.id)
```

`getTextStatus` toma o **WID**. Seu vizinho `findCommonGroups`, no mesmo pacote
de capacidade, toma o **MODELO**. Não há regra — há leitura. A H69 é a entrada
que explica o que assumir custa, e desta vez a leitura veio antes.

### A guarda que separa duas respostas diferentes

`receiveTextStatusEnabled()` é consultada ANTES. Um build com o recurso
desligado seria indistinguível de um contato que não escreveu nada, e essas são
respostas diferentes — `ErrAboutDisabled` de um lado, um recado vazio bem
sucedido do outro.

### O PASS ao vivo, e o que ele NÃO prova

```
peer about:  contacts.About(len=0 fetched=false waited=509ms)
second read: contacts.About(len=0 fetched=false waited=509ms)
```

Verde, e **fraco**. O recado do par voltou VAZIO e já em cache, então o que está
provado é o caminho de leitura em cache e a guarda. **Não** está provado: a busca
no servidor, nem carregar um recado não vazio.

Isso está dito **dentro do teste**, com `t.Log("NOT PROVEN by this run: …")`,
porque um PASS que não anuncia seus limites é a forma mais barata de um teste
mentir. Exercitar a busca exigiria consultar o recado de terceiros, o que as
regras do laboratório não permitem.

### Controles negativos EXECUTADOS

| mutação | falha observada |
|---|---|
| guarda computada que não desvia | `the gate is computed but does not guard a return` |
| passar o modelo em vez do wid | `the call does not pass the wid` |
| renderizar o texto do recado | `the rendering carries the text: …disponível para conversar…` |
| medir o comprimento em bytes | `the rune length is wrong or missing: len=26` |

O quarto é a armadilha UTF-16/bytes da H64 reaparecendo de outro ângulo: um
recado cheio de acentos não é mais longo que um sem eles, e `len()` diria que é.

**Status**: entregue, com os limites da prova ao vivo registrados acima.
**Testes**: `capabilities/contacts/commongroups_test.go` (secção `about`,
7 testes, quatro com controle acima) e `commongroupsreal_test.go`.

---

## H71 — estado de entrega de mensagem, e o enum lido em vez de decorado

**Data**: 2026-08-21
**Contexto**: Fase 1, cobertura. Capacidade de **leitura**.
**Onde**: `internal/wa-headless/capabilities/ack/`.

### A medição mudou o escopo antes de existir código

A intenção era o equivalente ao `Message.getInfo` do wwebjs — quem recebeu, quem
leu. A sonda mediu:

```
{"infoCount":0,"fromMeCount":368,"newest":[{"ack":2,"hasInfo":false}, …]}
```

A `MsgInfoCollection` está **vazia**: zero modelos contra 368 mensagens enviadas,
e `get()` não devolve nada para nenhuma recente. O detalhe por participante é
preenchido quando o app abre a gaveta de informações — uma sessão headless que
nunca a abre tem `msg.ack` e mais nada.

**Então a capacidade entregue é o `ack`**, e o que NÃO foi entregue está dito:
"quem leu" é outra capacidade, com outro custo.

### O enum vem da página

O script procura, **dentro** de `WAWebAck`, a entrada cujo VALOR é o ack desta
mensagem, e reporta o NOME dela — além de dizer de onde veio:

```
ack: ack.Status(state=sent raw=1 fromMe=true enum=WAWebAck.ACK waited=0s)
```

`enum=WAWebAck.ACK` prova que a tabela de emergência **não** foi usada. Uma
constante copiada de um blog é uma constante que ninguém pode conferir; um build
que renumerar seus estados muda `Raw` e nada mais, porque o mapeamento é por
NOME.

`Unknown` é o valor zero de propósito: um ack que ninguém soube nomear não pode
virar "pendente", que é uma afirmação.

### Um dublê mais permissivo que a produção, pego pelo próprio teste

`TestCancelledContextReadsNothing` falhou na primeira execução — o dublê deste
pacote não honrava o contexto, e o avaliador real honra (o chromedp recusa
contexto cancelado). O dublê foi corrigido para imitar a produção, com o motivo
escrito nele.

É a primeira armadilha do `ARMADILHAS.md` — dublê mais permissivo esconde o
defeito — aparecendo ao contrário: aqui ele deixou o teste FALHAR corretamente
em vez de esconder algo, porque a asserção era sobre não tocar a página.

### Controles negativos EXECUTADOS

| mutação | falha observada |
|---|---|
| mapear por número em vez de por nome | `name "READ" with raw 99 mapped to unknown, want read` |
| não ler o enum da página | `the script does not look the ack up in the page's enum` |
| ack sem nome virar "pendente" | `an unnamed ack became pending` |

**Status**: entregue (o `ack`); "quem leu" **não entregue**, com a medição que
explica por quê acima.
**Testes**: `capabilities/ack/ack_test.go` (7 testes, três com controle acima) e
o teste ao vivo em `commongroupsreal_test.go`.

---

## H72 — etiquetas de negócio (leitura)

**Data**: 2026-08-21
**Contexto**: Fase 1, cobertura. Capacidade de **leitura**.
**Onde**: `internal/wa-headless/capabilities/contacts/labels.go`.

### Só foi possível medir porque a conta é Business

Etiquetas são recurso do WhatsApp Business, e a H66 mediu esta conta como sendo
uma. É o caso raro em que uma propriedade inconveniente do fixture — descoberta
ao tentar mudar o nome de exibição e ser recusado — **destravou** outra coisa.

```
labels: contacts.Labels(count=3 waited=1ms)
  contacts.Label(id=1 nameLen=10 color=0 count=0)
  contacts.Label(id=2 nameLen=10 color=0 count=0)
  contacts.Label(id=3 nameLen=7 color=0 count=0)
the lab chat carries 0 label(s)
```

Três etiquetas padrão, nenhuma aplicada. O teste ao vivo assere **forma**, não
aquelas etiquetas: o dono da conta pode renomeá-las, e um teste que fixasse os
nomes quebraria por motivo alheio ao código.

### Duas respostas vazias que são respostas

- **Conjunto vazio de etiquetas** é sucesso. Conta pessoal não tem etiquetas, e
  este pacote não consegue distinguir isso de conta Business que não criou
  nenhuma — então reporta o que vê em vez de afirmar qual dos dois é.
- **Conversa sem etiqueta** devolve slice **vazio**, nunca `nil`. "Esta conversa
  não está etiquetada" é uma resposta, e `nil` convida o chamador a tratá-la como
  falha.

### O que NÃO foi entregue, e por quê

Escrever. `editLabelAssociation(arg, chats)` recebe um array de **modelos de
chat** como segundo argumento — isso está lido, ele faz `chat.id.toString()`. A
forma do PRIMEIRO argumento não é legível do invólucro, e chutá-la é o erro que a
H69 acabou de cobrar. Espera um chamador qualificado pelo módulo.

### Controles negativos EXECUTADOS

| mutação | falha observada |
|---|---|
| renderizar o nome da etiqueta | `the rendering carries the name: …name=Cliente novo…` |
| devolver `nil` para conversa sem etiqueta | `an unlabelled chat returned nil` |
| não normalizar os ids para string | `the script does not normalise label ids to strings` |

**Status**: entregue — leitura, e a **escrita também**, destravada pela H73 no
mesmo dia (ver continuação abaixo).
**Testes**: `capabilities/contacts/commongroups_test.go` (secção `labels`,
7 testes, três com controle acima) e o teste ao vivo.

---

## H73 — o instrumento que pergunta à função o que ela lê

**Data**: 2026-08-21
**Contexto**: decisão da orquestração — parar de abrir capacidades e resolver
**uma vez** a camada que vinha bloqueando várias.
**Onde**: `internal/wa-headless/spa/argprobe.go`,
`internal/wa-headless/probe_argshape_test.go`.

### O problema era um instrumento faltando, não uma assinatura

Três técnicas para descobrir a forma de uma chamada, e o `ARMADILHAS.md` registra
onde cada uma para:

| técnica | para quando |
|---|---|
| `String(fn)` | a função é `async` — devolve a casca |
| chamador do app | não existe chamador no bundle, ou ele chama outra função (H69) |
| camada do modelo | nenhum modelo expõe a operação |

Enquete (H69), etiqueta (H72) e recado (H66) estavam bloqueadas pelas TRÊS ao
mesmo tempo. Isso não é uma assinatura faltando; é um instrumento faltando.

### Como ele funciona

Uma função que desestrutura um objeto precisa **ler** as propriedades dele, e
ler é observável: ela recebe um `Proxy` cujo `get` registra cada chave. Depois
ela quase sempre lança — `undefined` raramente serve para alguma coisa — e a
essa altura já disse o que queria.

Com profundidade, o `get` responde com **outro** registrador, então o caminho
sai inteiro: `poll.options.map` em vez de parar em `poll`.

Dois detalhes que decidem se funciona:

- **`then` precisa devolver `undefined`.** Um proxy aguardado cujo `then` é
  chamável nunca assenta, e a sonda travaria em vez de responder.
- **O alvo do proxy é uma FUNÇÃO.** Alguns caminhos testam o argumento com
  `typeof` ou o chamam; um proxy sobre `{}` responde `'object'` e não é
  chamável, encerrando a leitura cedo com um `TypeError` que não diz nada.

### O controle veio antes das respostas

As duas primeiras alvos foram assinaturas **já conhecidas**, lidas por outras
técnicas. Um instrumento que não reproduz uma resposta medida não merece as não
medidas:

```
blockContact           arg0=[bizOptOutArgs blockEntryPoint contact skipCtwa1pdNbfSignal]  ✓ bate
forwardMessagesToChats arg0=[appendedText chats includeCaption msgs]                      ✓ bate
```

### O que ele destravou, numa execução

```
sendPollCreation({poll, chat, quotedMsg, isWamoSub})
    poll.name         string (o .length é lido)
    poll.contentType
    poll.options      array  (o .map é lido)

editLabelAssociation(arrayDeEtiquetas, arrayDeChats)   e NÃO lançou

setMyTextStatus(a, b, c, d, e)   nenhum argumento é lido como objeto
                                 -> são primitivos; ainda bloqueada, mas por
                                    um motivo agora CONHECIDO
```

O payload da enquete não tem nada parecido com `correctOptionKey` ou
`filteredOptions` — que é exatamente o que a H69 diagnosticou e não conseguiu
provar. Duas execuções ao vivo custaram o que uma sonda respondeu.

### O que ele NÃO responde

Ele diz o que a função **procura**, nunca o que o app **passaria**. São perguntas
diferentes, e confundi-las foi o erro da H69 na direção oposta. Também não
distingue campos lidos sob condição que a sonda nunca alcança.

**Status**: entregue.
**Testes**: os controles acima são o teste do instrumento; a prova de que ele
serve é a H69 fechada no mesmo dia.

---

## H72 (continuação) — a escrita de etiqueta, destravada pelo instrumento

**Data**: 2026-08-21.

A entrada original parou em: *"a forma do PRIMEIRO argumento não é legível do
invólucro, e chutá-la é o erro que a H69 acabou de cobrar."* O instrumento da
H73 respondeu — mas só depois de uma melhoria que vale por si.

### O instrumento não via dentro de um callback, e isso era estrutural

A primeira passada devolveu `arg0=[forEach]` e nada mais. A razão não é
específica de etiquetas: **o registrador responde à própria iteração**, então o
callback nunca roda e o elemento nunca é lido.

O conserto foi uma **dica de forma por argumento**: com `"array"`, o instrumento
passa um array REAL contendo um registrador. A iteração acontece de verdade, e o
elemento denuncia o que o callback lê:

```
editLabelAssociation([{id, type}], [chatModel])
    arg0=[[0].id [0].type]
    arg1=[[0].id [0].id.toString]
```

Qualquer API que receba lista tinha o mesmo problema. A dica é geral.

### O verbo é a única parte NÃO lida — e por isso a pós-condição existe

`type` recebe `'add'` ou `'remove'`, inferido de a chamada-espelho do app se
chamar `addOrRemoveLabelsMD`. Isso é inferência, não leitura, e está dito no
código e no teste.

**É exatamente por isso que a pós-condição lê `chat.labels`**: um verbo errado
produz falha limpa em vez de no-op silencioso. Ele estava certo — e teria sido
pego se não estivesse.

### O espelho local não é opcional

O app chama `editLabelAssociation` e logo depois
`LabelCollection.addOrRemoveLabelsMD`. Sem o segundo, `chat.labels` não se move
— e `chat.labels` é a única pós-condição disponível, então pular o espelho faria
**toda** aplicação parecer fracassada. Há controle negativo.

### Prova ao vivo

```
applied: contacts.LabelChange(before=0 after=1 noop=false waited=505ms)
removed: contacts.LabelChange(before=1 after=0 noop=false waited=509ms)
```

Com leitura de volta pela capacidade de leitura, apply redundante como no-op, e
etiqueta inexistente recusada.

### Controles negativos EXECUTADOS

| mutação | falha observada |
|---|---|
| passar id nu em vez de `{id, type}` | `the mutation is not the measured {id, type} shape` |
| pular o espelho local | `the local mirror is not updated, so chat.labels would never move` |
| não checar se a etiqueta existe | `the known-label check is computed but does not guard a return` |
| script confia no `await` | `the apply branch does not hand the settling decision to Go` |

**Status**: entregue.

---

## H57 (continuação) — o convite: a chamada nunca travou, o retorno é que não era o código

**Data**: 2026-08-21.

### A hipótese que o instrumento produziu, e que CAIU

O instrumento da H73 mostrou que `queryGroupInviteCode` lê
`$ProxyState$state.groupInviteCodePromise` e **retorna sem lançar**. Isso
explicava elegantemente os dois sintomas irreconciliáveis da entrada original —
"lança `reading iAmAdmin`" e "trava para sempre" — como uma função com duas
saídas, memo e caminho fresco, sendo a memo envenenada por uma tentativa
anterior.

**A medição derrubou a hipótese.** A sonda leu `memoBefore: ["neither"]`: não
existe campo de memo em lugar nenhum, nem no chat nem na metadata. A explicação
bonita estava errada, e está registrada como errada porque um achado com
diagnóstico errado é pior que nenhum.

### O que a mesma sonda mediu, e é a resposta

```
afterClear: {ok: true, v: "undefined"}
inviteCodeAfter: "present"
```

A chamada **assenta** — não trava. Ela resolve para `undefined` e **popula o
modelo**. O código antigo lia o valor de retorno, achava vazio, e o que parecia
"nunca produz nada" era isso.

O travamento de 90 s da medição original continua sem explicação, e fica dito
assim em vez de recebendo uma causa inventada. O que mudou entre as duas
execuções não foi medido.

### Prova ao vivo

```
read:    group.Invite(code=true len=22 revoked=false)
revoked: group.Invite(code=true len=22 revoked=true)
```

O código nunca aparece em log — só o comprimento e o fato de existir. Um link de
convite é credencial.

### Terceira capacidade a precisar da lição do `await`

O `await` assenta antes de o campo aterrissar, então o modelo é estacionado e o
Go relê. E orçamento esgotado devolve `ErrNoCode`, não "página travada": os
consertos são diferentes, e esta entrada gastou um dia exatamente nessa
diferença.

### Controles negativos EXECUTADOS

| mutação | falha observada |
|---|---|
| voltar a ler o retorno como código | `the return value is being taken as the code, which is what failed` |
| consultar um único dono do campo | `only one owner is consulted for the code` |
| código ausente indistinto de travamento | `got …never settled…, want ErrNoCode` |

**Status**: entregue.

---

## H74 — figurinha, e a ação de figurinha que não serve para enviar uma

**Data**: 2026-08-21
**Contexto**: Fase 1, continuando a ordem que a orquestração deu.
**Onde**: `internal/wa-headless/capabilities/send/media.go`,
`internal/wa-headless/testdata/lab-sticker.webp`.

### O módulo com o nome certo era o caminho errado

A enumeração deu `WAWebSendStickerAction :: sendStickerToChat`, e o instrumento
da H73 mediu o que ela quer:

```
sendStickerToChat(chat, {mediaData})
    arg1=[mediaData mediaData.stickerPremiumStatus …]
```

`mediaData` é um **modelo de figurinha que a conta já tem** — favoritos, recentes,
pacotes. A ação **reenvia** uma figurinha existente; ela não carrega bytes.

Ir pelo nome teria custado uma implementação inteira antes de descobrir isso. O
instrumento custou dez segundos.

### O caminho para bytes já estava medido, e documentado, meses antes

O comentário do `prepRawMedia` no `media.go` já dizia:

```
prepRawMedia(file, opts)   opts branches on isPtt / asDocument / asGif /
                           isAudio / asSticker / asStickerPack
```

A capacidade foi **um campo** no `Media` que já existia. O trabalho foi descobrir
que não era o outro caminho.

### Duas recusas, e as duas são sobre o mesmo perigo

- **Figurinha + documento** — a página ramifica em UM flag; mandar os dois deixa
  o primeiro que ela testar decidir em silêncio.
- **Figurinha + legenda** — figurinha não carrega legenda. Aceitar e descartar
  deixaria o chamador acreditando que mandou palavras que ninguém verá.

**Nenhuma conversão é tentada.** O WhatsApp espera WebP; enfiar um codificador no
meio de um envio transformaria o modo de falha de *"você mandou os bytes
errados"* em *"sua figurinha ficou estranha"*.

### O fixture é versionado, não gerado

512×512 em WebP dá **554 bytes**. Chamar `cwebp` ou `ffmpeg` no teste o faria
falhar numa máquina sem eles, por motivo alheio ao código sob teste.

### Prova ao vivo

```
sticker: send.Result(id=…74C6 at=2026-08-21T10:05:20Z waited=8ms)
MEASURED: the message this build produced is type="sticker"
```

O verificador de envio prova que **uma mídia** saiu; só o tipo prova **qual**. E
o ack confirma que saiu.

### Controles negativos EXECUTADOS

| mutação | falha observada |
|---|---|
| o flag não chega ao `prepRawMedia` | `asSticker=true does not reach prepRawMedia` |
| aceitar legenda em figurinha | `got <nil>, want ErrMediaStickerCaption` |
| aceitar figurinha-documento | `got <nil>, want ErrMediaStickerConflict` |

O primeiro **não se aplicou na primeira tentativa** — o `zsh` engoliu o padrão
passado como argumento. Refeito por heredoc, com `assert`. É a terceira vez que
um controle mente por não se aplicar, e as três foram pegas pelo mesmo `assert`.

**Status**: entregue.
**Testes**: `capabilities/send/sticker_test.go` (4 testes, três com controle
acima) e `stickerreal_test.go`.

---

## H75 — localização e vCard: não entregues, e a busca está fechada

**Data**: 2026-08-21
**Contexto**: Fase 1, últimos dois itens da ordem que a orquestração deu.

### Quatro buscas independentes, e nenhuma achou um caminho de envio

| busca | resultado |
|---|---|
| enumeração por nome (`Location`, `Vcard`) | **só módulos de exibição**: `WAWebFormatLocationMsgText`, `WAWebLocationMsgDisplayClass`, `WAWebVcardMsgDisplayClass`, `WAWebSendLocationChatAction` (que exporta apenas `displayName`, ou seja, é um componente React) |
| grep pelo campo do protocolo, `degreesLatitude` | achou o **parser de ENTRADA** — protobuf para modelo — e nada de saída |
| módulos `AddAndSend*` carregáveis | **nenhum** |
| superfície de envio do `Cmd` | `sendDeleteMsgs`, `sendPttRecording`, `sendRevokeMsgs`, `sendStarMsgs`, `sendUnstarMsgs` — e **zero** com `location`, `vcard` ou `contactcard` |

### O que isso significa, dito com precisão

Não existe primitivo de envio **exportado e carregável** para tipos de mensagem
arbitrários neste build. O texto tem o seu
(`WAWebSendTextMsgChatAction.sendTextMsgToChat`), a mídia tem o dela
(`prepRawMedia` + o método do `MediaPrep`), e localização e vCard não têm
equivalente.

O parser de entrada mostra a FORMA que a mensagem tem quando chega —
`{type: MSG_TYPE.LOCATION, kind: MsgKind.Location, loc, lat, lng, clientUrl}` —
e isso é útil para quem for tentar de novo. O que falta não é a forma do dado: é
a função que o aceita.

### Por que parei aqui

O caminho restante é dirigir a UI da página — abrir o anexo, escolher
localização, capturar a chamada. Isso é técnica nova, muda a natureza do módulo
(que hoje nunca toca no DOM), e não foi autorizado.

**Não inventei um caminho.** A alternativa seria montar `msgData` à mão e
procurar uma função interna não exportada para engoli-lo, que é exatamente o tipo
de chute que a H69 cobrou caro.

### O que a próxima tentativa herda

O instrumento da H73 e as quatro buscas acima. Quem retomar não precisa refazer
nenhuma delas — precisa de uma quinta ideia, e a mais provável é interceptar a
chamada real com a UI da página, que é o que a orquestração já apontou como
"instrumentação de captura real" na decisão que abriu esta sequência.

**Status**: não entregue.

---

## H76 — o ledger global de paridade, e o número que ele revelou

**Data**: 2026-08-21
**Contexto**: decisão da orquestração — *"(3) agora é prioridade absoluta.
Pare de abrir novas capabilities e construa o GLOBAL-WWEBJS-PARITY-LEDGER antes
de decidir localização/vCard/status."*
**Onde**: `internal/wa-headless/LEDGER-WWEBJS.md`,
`internal/wa-headless/gate_ledger_test.go`,
`internal/wa-headless/testdata/wwebjs-v1.34.7-surface.txt`.

### Eu tinha recebido esta ordem em 21/08 00:01 e NÃO tinha cumprido

A orquestração mandou criar o ledger naquele momento. Eu segui implementando por
família e mantendo o `PARIDADE-WWEBJS.md`, que **não é a mesma coisa**: ele lista
o que foi ATACADO, e por construção não mostra o que nunca foi procurado.

Isso está registrado como falha de execução minha, não como decisão.

### O número

Upstream fixado: **v1.34.7**, SHA `f935b500117e264c2b3abc25b63a280bd98182a7`,
2026-04-24. Superfície extraída do **código-fonte** da tag — `Client`, as
estruturas públicas e os 31 eventos.

| estado | itens | fração |
|---|---|---|
| `PROVEN` | 35 | 15% |
| `PARTIAL` | 31 | 14% |
| `BLOCKED` | 2 | — |
| `INTENTIONAL_DIFFERENCE` | 2 | — |
| `MISSING` | 150 | **68%** |
| **total** | **220** | |

**Duzentos e vinte itens, e 150 nunca foram procurados.** Eu vinha medindo
progresso pelo número de capacidades de que me lembrava — que é exatamente o que
a orquestração previu ao pedir o ledger.

### O que estava invisível e agora tem linha

Famílias inteiras: **canais/newsletters**, **listas de transmissão**,
**chamadas**, **comércio** (pedidos, pagamentos, catálogo), **pedidos de entrada
em grupo**, **configurações de grupo** (quem pode falar, quem pode editar
informação), **configurações de download automático**, **agenda de contatos**,
**notas de cliente**, **eventos agendados**, **votos de enquete**.

E itens soltos que passariam despercebidos: `markChatUnread` (temos o inverso),
`sendStateRecording` (temos "digitando", não "gravando"), `Message.pin` (fixar
MENSAGEM é distinto de fixar conversa), `getInviteInfo` (ler convite de terceiro,
distinto de `getInviteCode`).

### A regra de completude virou GATE, não intenção

A orquestração chamou de "gate conceitual". Uma intenção não pega o modo de falha
real: alguém acrescenta uma capacidade, atualiza a linha que estava olhando, e
não repara nas doze que não estava.

Quatro testes, todos com controle negativo executado:

| teste | controle negativo | falha observada |
|---|---|---|
| cobre a superfície inteira | remover uma linha | `1 upstream item(s) are absent … getWWebVersion` |
| só o vocabulário declarado | inventar `TODO_LATER` | `state "TODO_LATER" is not in the declared vocabulary` |
| placar bate com as linhas | inflar `PROVEN` para 90 | `the scoreboard says 90 PROVEN and the rows contain 35` |
| fixa um upstream exato | apagar o SHA | `the ledger does not record "f935b500…"` |

**A superfície é versionada**, não buscada no GitHub: um gate que busca falha
offline e, pior, passa a medir outro upstream no dia em que a tag se mexer — que
é a deriva que o pin existe para impedir.

### Duas armadilhas conhecidas reapareceram ao escrever o gate

1. O teste de vocabulário acusou **dezoito nomes de EVENTO** como estados
   inválidos: eles também são maiúsculas. Forma não separa; **dado** separa — a
   superfície fixada lista os eventos, então eles são excluídos por nome. É a
   mesma lição do padrão de status do HOUSEKEEP, três commits atrás.
2. O placar veio um a mais em cada estado, porque a **tabela de vocabulário**
   declara cada estado uma vez e estava sendo contada como dado. Recorte por
   seção, não por forma.

**Status**: entregue.

---

## H77 — o ledger tinha uma linha errada, e a auditoria que a achou tem um ponto cego

**Data**: 2026-08-21
**Contexto**: primeira onda do `COMPLETE-FAMILIES` que a orquestração ordenou.
**Onde**: `internal/wa-headless/LEDGER-WWEBJS.md`.

### O erro

Marquei `Chat.sendStateRecording` como `MISSING` com a nota *"temos digitando,
não gravando"*. **Já existia**: `presence.StateRecording`, mapeado para
`markRecording`, com o mapeamento asserido em teste unitário desde antes.

O mapeamento do ledger foi feito **de memória** sobre 220 itens, e pelo menos um
saiu errado — na direção pessimista, que é a menos perigosa das duas mas
continua sendo uma linha que mandaria alguém reimplementar o que existe.

### A auditoria, e por que ela quase não pegou

Rodei uma varredura dos 120 nomes `MISSING` contra o nosso código. Ela devolveu
17 candidatos, e **quase todos eram colisão de palavra genérica em outra
classe** — `mute` aparece porque temos `Client.muteChat`, mas a linha `MISSING`
era `Channel.mute`, que de fato não temos.

**O ponto cego**: `sendStateRecording` NÃO apareceu na varredura, porque o nosso
identificador é `StateRecording`/`markRecording`. Nomes iguais são coincidência;
nomes diferentes são o normal, porque o `CLAUDE.md` manda escolher o nome que
descreve a NOSSA semântica.

Ou seja: a auditoria por nome tem recall ruim por desenho. Ela é útil como rede,
não como prova.

### O que fica valendo

1. **A linha corrigida** para `PARTIAL`, com a prova ao vivo bloqueada pelo mesmo
   impedimento da observação de presença (H50), que exige as duas contas na
   agenda uma da outra — ação de telefone, humana.
2. **A anotação de que o mapeamento é de memória.** Cada família que for aberta
   deve reconferir suas próprias linhas contra o código antes de trabalhar, em
   vez de confiar no estado que eu escrevi.
3. O gate do ledger **não pega isto** e não pode: ele garante que toda linha
   existe e que o placar bate, não que o ESTADO de cada linha esteja certo.
   Isso é limitação real e está dita aqui em vez de descoberta de novo.

**Status**: corrigido.

---

## H78 — marcar conversa como não lida: não entregue, e o que o `Cmd` é de verdade

**Data**: 2026-08-21
**Contexto**: primeira onda do `COMPLETE-FAMILIES`.
**Onde**: `internal/wa-headless/capabilities/chats/markunread.go`.

### Duas primitivas medidas, nenhuma marca a conversa

| tentativa | resultado entre sessões |
|---|---|
| `sendConversationSeen({chat, key, unreadDelta: -1})` | 0 -> 0 |
| `Cmd.markChatUnread(chat, true)` | 0 -> 0 |

A primeira veio de supor que marcar não-lida fosse marcar-lida com o número
invertido. A segunda veio do chamador do próprio app, encontrado por grep.

### O achado estrutural, que vale muito além deste item

```js
i.markChatUnread = function(t, n) { this.trigger("mark_chat_unread", …) }
```

**O `Cmd` é barramento de EVENTOS.** Um verbo dele só faz algo se houver ouvinte
ligado àquele nome. Isso explica o que até aqui parecia arbitrário:
`Cmd.sendStarMsgs` funciona (ouvinte no núcleo sempre carregado) e
`Cmd.markChatUnread` não (ouvinte num pedaço de UI que sessão headless não
carrega).

**Para toda chamada futura ao `Cmd`**: "não lança e não faz nada" significa
ouvinte ausente, não argumento errado. Está no `ARMADILHAS.md`.

### Um erro meu de método, que escondeu a resposta por uma hora

Levantei a superfície do `Cmd` filtrando por `/^send/`, porque tudo que eu tinha
dirigido ali se chamava `sendAlgumaCoisa`. O filtro codificou uma suposição sobre
nomes e escondeu `markChatUnread` do meu próprio levantamento. Levantamento
estreitado por um palpite é um palpite.

### O que a verificação entre sessões evitou

A primeira versão do teste afirmava **na mesma sessão** e reportou o chat com 22
não-lidas antes e depois. Uma sessão nova leu o mesmo chat como **0**. Sem a
verificação entre sessões eu teria concluído "a marcação falhou porque o contador
não moveu", quando o contador não move de qualquer jeito.

**Consequência registrada e não escondida**: a pós-condição do `MarkRead` afirma
que o contador moveu NA MESMA SESSÃO — exatamente a leitura que esta medição
mostrou não confiável. A H52 provou o `MarkRead` contra um chat em que o contador
moveu; se isso generaliza é pergunta em aberto.

### O que a próxima tentativa herda

As duas primitivas descartadas com evidência, o fato de o `Cmd` precisar de
ouvinte, e o campo certo (`chat.markedUnread`, indefinido até alguém marcar). O
que falta é uma quinta ideia — provavelmente a mesma instrumentação de UI que a
orquestração já autorizou como medição para localização e vCard.

**Status**: não entregue.

---

## H79 — políticas de grupo, e o oráculo que o próprio app oferece

**Data**: 2026-08-21
**Contexto**: `COMPLETE-FAMILIES`, família Group.
**Onde**: `internal/wa-headless/capabilities/group/policy.go`.

### Quatro caminhos errados antes do certo, todos medidos

| tentativa | o que aconteceu |
|---|---|
| `mexUpdateGroupPropertyJob` com aridade 1 | `Bad Request` — a assinatura tem DOIS argumentos |
| o mesmo com aridade 2 e id como string | `Bad Request` de novo; o objeto é repassado opaco ao GraphQL |
| `sendSetPropertyRPC` (o RPC cru) | devolveu o vocabulário de flags, e travou por faltar o resto da stanza |
| `WAWebSetDescriptionGroupAction` | não existe |

O certo é `WAWebSetPropertyGroupAction.setGroupProperty(chat, nome, 1|0)`, achado
enumerando **todos** os módulos com "Group" e filtrando a SAÍDA pelo nome da
função — o filtro por nome de MÓDULO nunca o acharia.

### O oráculo, e por que ele foi seguro de usar

O switch do app recusa nome desconhecido **antes de enviar qualquer coisa**.
Isso o torna um enumerador gratuito de nomes válidos.

O detalhe que o tornou seguro: cada candidato recebeu o valor **ATUAL** do
grupo, então um nome válido era no-op. Sem isso, enumerar os nomes teria virado
as políticas do grupo de laboratório uma candidata por vez.

```
aceitos:  announcement, restrict, membership_approval_mode,
          no_frequently_forwarded, ephemeral
recusados: locked, announce, description, subject,
          allow_admin_reports, group_history
```

**As palavras que parecem certas são as que não funcionam.** `locked` e
`announce` são exatamente o que alguém escreveria para "só admin edita" e "só
admin fala", e a página recusa as duas.

### E há uma assimetria a mais, que o teste trava

O nome que se **escreve** é `announcement`; o campo que se **lê** na metadata é
`announce`. Escrever e ler a mesma política usa strings diferentes.

### Prova ao vivo, entre sessões

```
MEASURED: the lab group's announcement is true
flipped:  group.PolicyChange(policy=announcement wanted=false noop=false verified=false)
CROSS-SESSION: announcement was true before the flip and reads false after
restored: group.PolicyChange(policy=announcement wanted=true noop=false verified=false)
```

`Verified` é false pela razão da H58, e `canSetGroupProperty` é consultado como
MÉTODO — a lição que a H57 pagou com quatro tentativas cegas.

### Controles negativos EXECUTADOS

| mutação | falha observada |
|---|---|
| aceitar os nomes que parecem certos | `"locked": got <nil>, want ErrUnknownPolicy` |
| mandar booleano em vez de 1/0 | `on did not become 1` |
| ler o campo com o nome de escrita | `the read field equals the write name; measured, they differ` |

**Status**: entregue — três linhas do ledger (`setMessagesAdminsOnly`,
`setInfoAdminsOnly`, `setAddMembersAdminsOnly`) saem de `MISSING` para `PARTIAL`,
parciais apenas porque este build não confirma na mesma sessão.

---

## H80 — descrição e foto de grupo: medidas, não entregues

**Data**: 2026-08-21
**Contexto**: `COMPLETE-FAMILIES`, continuação da família Group.

Nenhuma das duas foi implementada. As medições ficam aqui porque custaram
enumerações e sondas, e sem elas a próxima tentativa recomeça do zero.

### Descrição do grupo

O módulo é `WAWebGroupModifyInfoJob`, achado enumerando **todos** os módulos com
"Group" e filtrando a SAÍDA pelo nome da função. Ele exporta os quatro juntos:

```
setGroupSubject, setGroupDescription, setGroupProperty, setEphemeralGroupProperty
```

`WAWebSetPropertyGroupAction.setGroupProperty`, que a H79 usa, é a AÇÃO que
embrulha o job homônimo daqui. Para a descrição **não foi encontrada ação
equivalente** — `WAWebSetDescriptionGroupAction` não existe.

A forma, pelo instrumento:

```
setGroupDescription({desc, groupWid, newDescId, prevDescId})
```

**O que falta é `newDescId`/`prevDescId`.** São identificadores que o app gera, e
inventá-los é o chute que a H69 cobrou. O caminho provável é ler `prevDescId` da
metadata e gerar o novo do mesmo jeito que o app gera id de mensagem.

### Foto — de grupo E da conta, quatro linhas do ledger de uma vez

```
WAWebProfilePicThumbAction :: setProfilePic, deleteProfilePic
    setProfilePic(thumb, …)      lê thumb.id e thumb.canSet
    deleteProfilePic(thumb, …)   lê thumb.id e thumb.canDelete
```

O primeiro argumento é um **modelo de miniatura** — o mesmo tipo que a capacidade
`avatar` já lê da `ProfilePicThumbCollection`. As duas fecham
`Client.setProfilePicture`, `Client.deleteProfilePicture`, `GroupChat.setPicture`
e `GroupChat.deletePicture`.

Ambas lançaram `Could not perform action.` com um registrador no lugar do modelo,
que é o `ActionError` da H55 — a guarda recusando, não um argumento errado.

**O que falta**: o modelo real da coleção e a forma da imagem no segundo
argumento.

### Por que parei aqui e não continuei

Cada uma é outro ciclo de medição, e o valor de registrar agora é maior que o de
entregar mais uma nesta sessão: quem retomar tem módulo, assinatura e o nome do
que falta em cada caso, em vez de quatro enumerações para refazer.

**Status**: não entregue — medições preservadas acima.

---

## H81 — fixar mensagem: não entregue, e o achado que parecia decisivo e não era

**Data**: 2026-08-21
**Contexto**: `COMPLETE-FAMILIES`, família Message.
**Onde**: `internal/wa-headless/capabilities/pin/`.

### O que está medido, e é bastante

```
WAWebSendPinMessageAction.sendPinInChatMsg(msg, estado, segundos)
    msg lê: id, id.toString, revisionNumber, to, from, id.fromMe,
            id.remote, id.remote._serialized

PIN_STATE                     = {INVALID: 0, PIN: 1, UNPIN: 2}         (UI)
Message$PinInChatMessage$Type = {UNKNOWN_TYPE: 0, PIN_FOR_ALL: 1,
                                 UNPIN_FOR_ALL: 2}                     (fio)

DEFAULT_PIN_EXPIRY_DURATION_OPTION = "SevenDays" -> 604800 segundos
```

### O achado que parecia decisivo e não era

O chamador do próprio app usa o enum de **protobuf**, não o de UI:

```js
sendPinInChatMsg(t, Message$PinInChatMessage$Type.UNPIN_FOR_ALL)
```

Encontrar isso pareceu a resposta — eu estava mandando o enum errado. **E os
dois carregam os MESMOS números.** Trocar não mudou nada.

Vale registrar exatamente por isso: um achado pode ser verdadeiro, específico,
vindo da técnica certa, **e ainda assim não ser a causa**. O sinal de que era
esse o caso estava disponível antes de eu testar — bastava comparar os valores.

### O que a chamada faz: nada

A chamada é aceita e não lança. Uma sessão nova lê `PinInChatCollection` com
**zero** modelos e nenhuma mensagem com marca de fixada.

**Nada ficou fixado**, e isso foi verificado — não é suposição. Uma mensagem
fixada por engano ficaria uma semana no topo da conversa do par.

### Duas tentativas, e então parei

A H58 registrou que o caro neste módulo é a sequência de tentativas cegas na
mesma camada. As duas aqui foram medidas; a terceira seria chute.

**A hipótese mais provável para quem retomar**, não testada: o par
ponte + espelho local que a H72 mediu nas etiquetas.
`WAWebPinMessageAction` exporta `craftPinMessage` e `updatePinCollection`, e o
segundo nome descreve exatamente um espelho. `craftPinMessage` recebe o modelo da
mensagem (medido); `updatePinCollection` recebe algo iterável (medido: lançou
`e is not iterable`).

**Status**: não entregue — medições preservadas.
**Testes**: `capabilities/pin/pin_test.go` travam o vocabulário, a duração, o
modelo e o contrato honesto; `pinreal_test.go` fica vermelho atrás do próprio
interruptor.

---

## H82 — como esta SPA confirma uma escrita, e o classificador que responde em trinta segundos

**Data**: 2026-08-21
**Contexto**: decisão da orquestração — investigar o mecanismo de confirmação
antes de abrir mais capacidade.
**Onde**: `internal/wa-headless/spa/confirmation.go`,
`internal/wa-headless/probe_writeclass_test.go`.

### O que motivou

Das últimas cinco tentativas, **uma entregou e quatro viraram registro**, e as
quatro falharam do mesmo jeito: *a página aceita, não lança, e nada acontece*.
Cada capacidade nova vinha pagando o mesmo pedágio, uma execução ao vivo por vez.

### As três classes, com as evidências que já tínhamos

| classe | o que acontece | onde foi medido |
|---|---|---|
| `IMMEDIATE` | o modelo move nesta sessão, em menos de um segundo | favoritar, bloquear, editar, silenciar, arquivar/fixar conversa, encaminhar, enviar, reagir, apagar, etiquetas, assunto de grupo, código de convite, **políticas de grupo (H85)** |
| `CROSS_SESSION` | chega ao servidor e esta sessão nunca vê | participantes (H58), contador de não-lidas (H78) — **políticas saíram daqui na H85** |
| `NOTHING` | aceito e nada acontece em lugar nenhum | verbo do `Cmd` sem ouvinte (H78), fixar mensagem (H81) |

### A hipótese óbvia está ERRADA, e vale dizer qual é

**A camada do módulo não prediz a classe.** `WAWebSetSubjectGroupAction` é
`IMMEDIATE` e `WAWebSetPropertyGroupAction` é `CROSS_SESSION` — mesmo sufixo,
mesma família, respostas diferentes. `Cmd.sendStarMsgs` é `IMMEDIATE` e
`Cmd.markChatUnread` é `NOTHING`.

**O que correlaciona em todos os casos medidos:**

```
o app escreve o modelo ele mesmo, otimisticamente   -> IMMEDIATE
o modelo é atualizado por evento vindo do servidor  -> CROSS_SESSION
nada está ligado à chamada                          -> NOTHING
```

Favoritar escreve `msg.star` localmente; arquivar escreve `chat.archive`; as
etiquetas só se moveram quando o espelho local foi chamado explicitamente (H72) —
o mesmo fato visto do outro lado. Não-lidas são limpas por um recibo de leitura,
participantes por notificação de grupo, políticas por notificação de propriedade:
tudo de entrada, e esta sessão não aplica.

**É hipótese com evidência, não lei**, e está escrita para que a próxima
capacidade comece de uma previsão testável em trinta segundos em vez de uma
surpresa descoberta numa execução ao vivo perdida.

### O classificador, e os dois defeitos MEUS que os controles pegaram

`spa.ClassifyWriteExpr` + `ClassifyReadExpr`: recebem uma escrita e um leitor,
medem antes, escrevem, e o **Go** consulta o leitor até ele mover ou o orçamento
acabar.

Os controles foram três casos com resposta **já registrada**, e eles pegaram
duas falhas antes de qualquer caso novo ser classificado:

1. **A primeira versão lia uma vez, logo após o `await`** — e reportou o controle
   do favoritar, medido `IMMEDIATE` a 696 ms, como "não moveu". *O instrumento
   construído para detectar o defeito da H61 tinha o defeito da H61.*
2. **A função externa era `async`**, então o `Evaluate` recebia uma promessa e os
   três controles vieram como `cannot unmarshal object into string`.

E um terceiro, no laço em Go: ele parava em qualquer estágio que não fosse
`settling`, e `pending` — escrita ainda em curso — também é "continue
esperando". Dois controles vieram como recusa.

Resultado final, os três batendo:

```
star          before=false after=true  -> IMMEDIATE  (esperado IMMEDIATE)
group policy  before=true  after=true  -> not-here   (esperado not-here)
message pin   before=0     after=0     -> not-here   (esperado not-here)
```

### O que a investigação custou a uma linha PROVEN

`Client.sendSeen` (`chats.MarkRead`) foi **rebaixada para `PARTIAL`**. A
pós-condição dela afirma que `chat.unreadCount` moveu na mesma sessão, e a H78
mediu esse contador como `CROSS_SESSION`. A H52 provou contra um chat em que ele
moveu; se generaliza é pergunta em aberto, e manter `PROVEN` seria confiar numa
prova que a medição posterior põe em dúvida.

Placar: `PROVEN` 34, `PARTIAL` 36.

### A regra prática, para as 146 restantes

Antes de escrever pós-condição, rode o classificador. Ele responde `IMMEDIATE`
ou "não move aqui"; separar `CROSS_SESSION` de `NOTHING` exige uma segunda
sessão, que é **exatamente o passo que as quatro capacidades falhadas pularam**.

**Status**: entregue.

---

## H81 (continuação) — a hipótese do espelho, testada em trinta segundos

**Data**: 2026-08-21, logo após a H82.

A H81 deixou escrita uma hipótese não testada: que faltava o par **ponte +
espelho local** que a H72 mediu nas etiquetas. `WAWebPinMessageAction` exporta
`craftPinMessage` e `updatePinCollection`, e o segundo nome descreve um espelho.

**Testada com o classificador da H82, como quarto caso, ao lado dos três
controles.** Resultado:

```
pin WITH the local mirror   before=0  after=0  -> not-here  (esperado IMMEDIATE)
MIRROR: Got unexpected null or undefined
```

### O resultado exige precisão, e é fácil errar aqui

A hipótese **não foi refutada**. Ela também **não foi confirmada**. O espelho
lançou antes de fazer qualquer coisa — `craftPinMessage` devolveu nulo, ou
`updatePinCollection` recebeu algo que não esperava — então o caminho
ponte+espelho nunca chegou a ser exercitado.

Dizer "a hipótese está errada" seria a conclusão confortável e não é o que a
medição mostra. O que ela mostra é que **o espelho tem sua própria forma a
descobrir**, e isso é trabalho, não resposta.

### O que isto provou de verdade

O classificador entregou o que prometia. Testar esta hipótese custou **um caso
num teste que já existia** — trinta segundos — em vez de outra capacidade
inteira implementada, provada ao vivo e desfeita. É a primeira vez nesta sessão
que uma hipótese sobre escrita foi descartada sem pagar o ciclo completo.

E os três controles rodaram junto e continuaram batendo, o que é o outro
propósito deles: cada uso do instrumento reconfirma que ele ainda mede o que
media.

A hipótese do espelho passa de "não testada" para "testada e bloqueada na forma
do próprio espelho".

**Status**: não entregue.

---

## H83 — ler reações: bloqueado, e um ramo de código que nunca rodou

**Data**: 2026-08-21
**Contexto**: `COMPLETE-FAMILIES`, família Message — as três que faltavam são
LEITURAS, que não têm o problema de confirmação da H82.
**Onde**: `internal/wa-headless/capabilities/react/react.go`.

### O fixture não tem o dado, e isso foi medido antes de qualquer código

```
385 mensagens, campos presentes em todas:
    hasReaction, mentionedJidList, groupMentions, nonJidMentions
mensagens COM menção:  0
mensagens COM reação:  0
ReactionsCollection:   0 modelos
```

Reação eu posso **criar** — a H53 provou o `Add` — então o laço fechado era
possível: reagir, ler, remover. Foi o que tentei.

### Duas tentativas, e então o instrumento

| tentativa | erro |
|---|---|
| `getReactionEmojisAndSum(msg)` | `e.forEach is not a function` |
| `getReactionEmojisAndSum([msg])` | `Cannot read properties of undefined (reading 'slice')` |

Parei e perguntei ao instrumento da H73:

```
arg0=[[0].reactions [0].reactions.slice [0].reactions.slice().map]
```

A função recebe uma **lista de registros que carregam `.reactions`** — não
mensagens. E `msg.reactions` **não existe** neste build; a `ReactionsCollection`
que seria a fonte está vazia. Não há de onde tirar a entrada.

### O achado que vale mais que a capacidade

O `react.go` **já chamava** `getReactionEmojisAndSum(m)` no caminho de
verificação, dentro de um `try/catch`. Ou seja: **toda chamada lançava, o catch
engolia, `sum` ficava em -1, e o fallback para a flag pegajosa decidia todas as
respostas.** O ramo do agregado nunca rodou.

E o comentário acima dele dizia:

> *"O agregado é a verdade de nível de exibição — o que a bolha mostraria — e é
> o que isto reporta."*

**Descrevia código que nunca executou.** Isso é pior que nenhum comentário: ele
disse ao próximo leitor que a verificação era mais forte do que é. A H53 registrou
que a remoção não era verificável e atribuiu isso à flag pegajosa; a razão real é
que o único sinal disponível SEMPRE foi a flag.

O ramo morto foi removido e o sinal honesto está nomeado.

**A regra**: `try/catch` em volta de um caminho "melhor" com fallback silencioso
esconde que o caminho melhor nunca funciona. Se o fallback é aceitável, ele é o
comportamento — e o comentário tem de dizer isso.

### O que fica

Ler reações: **não entregue**, sem fonte de entrada neste build.
Ler menções: implementável, e **improvável neste fixture** — nenhuma das 385
mensagens menciona alguém, e criar uma exigiria enviar com menção, que é outra
capacidade.

**Status**: não entregue.

---

## H84 — EVENT-BUS FOUNDATION: um ingresso, um Hub, sete propriedades provadas

**Data**: 2026-08-21
**Contexto**: ordem congelada da orquestração, depois do `COMPLETE-FAMILIES`.
Ela foi específica: *"não implemente 23 subscriptions independentes; desenhe um
barramento único"*, com arquitetura
`SPA → EventIngress → Hub → eventos tipados → consumidores`.
**Onde**: `internal/wa-headless/events/`.

### Por que um mecanismo único, e não 23

O módulo tinha DUAS assinaturas — `messagemeta` e `contacts.onContact` — cada uma
escrita do zero, cada uma com o próprio buffer e a própria ideia do que é um
descarte. Acrescentar as outras 23 assim seriam 23 instalações disputando a
mesma página, 23 tetos sem relação entre si, e nenhuma forma de dizer em que
ordem as coisas aconteceram.

Há **um** instalador, **um** buffer e **um** contador de sequência. A ordem entre
tipos diferentes sobrevive à fronteira porque a página a carimba, e um descarte
significa a mesma coisa para todo assinante.

### A recarga é tratada por CURA, não por detecção

Uma recarga apaga os handlers e o buffer. Detectar recarga exige um sinal que
este módulo não tem — então a bomba **reinstala a cada ciclo**. A instalação é
idempotente e responde "já" no caso comum, então o custo é uma chamada barata
por sondagem e a recuperação não precisa de detecção nenhuma.

O risco óbvio dessa escolha — instalar duas vezes e receber tudo em dobro — é
exatamente o que a propriedade 5 mede.

### As sete propriedades, todas numa execução

Numa execução só de propósito: várias delas são sobre como interagem — "reinstala
sem duplicar" só significa algo se a entrega funcionava antes E depois — e
testes separados começariam de uma página limpa e provariam a metade fácil.

```
MEASURED: delivered=16  droppedInPage=0 seenInPage=27  gaps=0 reinstalls=1
MEASURED: delivered=765 droppedInPage=0 seenInPage=776 gaps=0 reinstalls=2
```

| propriedade | como foi provada |
|---|---|
| instalação única | `reinstalls=1` antes de qualquer recarga |
| entrega VIVA | mensagem enviada depois da bomba chegou com `Replay=false` |
| unsubscribe | zero eventos após soltar, com envio real no meio |
| recarga reinstala | `reinstalls` foi a 2 sozinho |
| **sem duplicação** | a mensagem pós-recarga chegou **uma** vez |
| overflow contado | `seenInPage` acumulado atravessa a recarga |
| teardown | `Close` zera assinantes; `Uninstall` solta os handlers |

### Dois achados que a execução ao vivo entregou

**1. O ingresso instala ANTES de a página poder enviar.** As coleções existem —
que é tudo de que o ingresso precisa — enquanto o `comms` ainda está subindo, e
um envio nessa janela morre com `[comms] sendIq called before startComms`.

Prontidão para **observar** e prontidão para **enviar** são coisas diferentes, e
o teste espera as duas separadamente. Confundi-las faria o barramento parecer
quebrado porque outra coisa não estava pronta.

**2. Os contadores da página VOLTAM A ZERO na recarga.** Eu os sobrescrevia a
cada dreno, então depois de uma recarga a estatística andava para trás — e uma
estatística que anda para trás é pior que nenhuma, porque o número que as pessoas
citam é o de depois da última recarga. Agora são acumulados, com a base somada no
momento da reinstalação.

### O que os números dizem

`delivered=765` contra `seenInPage=776`: a diferença são os eventos que chegaram
durante a janela sem assinante — `delivered` só conta quando alguém escuta. E os
749 eventos do segundo lote são a **história sendo recarregada**: é para isso que
`Replay` existe, e um consumidor que contasse aquilo como novo contaria a
conversa inteira de novo.

**Status**: entregue.
**Testes**: `events/events_test.go` (11 testes, incluindo `-race` com assinantes
entrando e saindo durante a entrega) e `eventbusreal_test.go` (as sete).

---

## H85 — a classificação estava errada, e o controle é que não podia falhar

**Data**: 2026-08-21
**Contexto**: a orquestração mandou usar o barramento novo para investigar os
`CROSS_SESSION`. A investigação corrigiu a própria pergunta.

### O que a investigação perguntou

Duas explicações cabiam em "a sessão não vê a mudança", e exigem consertos
diferentes: a sessão **recebe** a notificação e não a aplica, ou a notificação
**não chega**. Até o barramento existir não havia como distinguir.

### O que ela mediu

```
8 eventos nos segundos após a mudança; 8 deles nomeiam o grupo que mudou
a política ficou visível NESTA SESSÃO após 1s
```

**A sessão recebe.** E mais: **o modelo atualiza**. A política de grupo não é
`CROSS_SESSION` — é `IMMEDIATE`, em cerca de um segundo.

### Por que eu tinha classificado errado, e a causa é de método

O controle do classificador para "política" escrevia **o valor que o grupo já
tinha**, para não mudar nada. E **um no-op não pode mover um leitor** — ele
reportou "não move aqui" pela razão trivial de que nada mudou.

Esse resultado então "confirmou" um palpite anterior herdado da H58 ("metadata de
grupo é obsoleta"), que era verdade para PARTICIPANTES e foi generalizada demais.

> **Um controle que não pode falhar não é controle.**

É a mesma família de erro dos controles negativos que passavam — e desta vez o
controle inválido não deixou passar um teste fraco: fez uma medição correta
parecer confirmar uma hipótese errada, que é pior, porque virou documentação.

### O que foi corrigido

| onde | antes | agora |
|---|---|---|
| controle do classificador | no-op | vira e desvira de verdade, com restauração aguardada |
| `SetPolicy` | `Verified: false`, sem pós-condição | pós-condição real; `Verified: true`; `ErrPolicyUnchanged` |
| tabela da H82 | políticas em `CROSS_SESSION` | políticas em `IMMEDIATE` |
| teste ao vivo | exigia `verified=false` | exige `verified=true`; mede `waited=1.027s` |

**A H58 continua de pé** para participantes: aquilo foi mudança real, sondada por
noventa segundos, e confirmada entre sessões. O erro não foi a medição da H58 —
foi eu ter transformado uma medição num princípio geral.

### E eu deixei estado no grupo de laboratório

A restauração da sonda chamava `setGroupProperty` **sem aguardar a promessa** — o
`Evaluate` não aguarda, então ela era abandonada. O grupo ficou com
`announcement=false` quando o original era `true`, e só apareceu porque a
execução seguinte leu o valor e ele não batia.

Reposto, e a sonda agora **espera do lado Go** e falha alto se a restauração não
assentar. Pedir uma restauração não é restaurar.

**Status**: corrigido.

---

## H86 — participantes e políticas, mesmo objeto, comportamentos opostos

**Data**: 2026-08-21
**Contexto**: a H85 mostrou que a classificação das políticas estava errada, o
que pôs a H58 sob suspeita. Reexecutada com o barramento a observar.

### H58 fica de pé, e o par de casos é o achado

```
política  (announcement)  ->  8 eventos sobre o grupo, visível em ~1s
participante (remover)    ->  0 eventos sobre o grupo, invisível em 90s
```

As duas mudanças escrevem **o mesmo objeto de metadata do mesmo grupo**. Uma é
`IMMEDIATE`, a outra é `CROSS_SESSION`. Então a história "metadata de grupo é
obsoleta" nunca foi sobre o objeto — a H85 mostrou que ela era larga demais, e
isto mostra o quanto.

O que separa os dois é o que a H82 já tinha proposto: `setGroupProperty` é uma
**ação** que escreve o modelo localmente; `removeParticipantsJob` é um **job**
que só envia, e a atualização local depende de uma notificação de entrada que
esta sessão não aplica. Agora a correlação tem um par controlado — mesmo objeto,
mesma sessão, mesmo minuto — em vez de casos espalhados.

### E o barramento tem um limite que precisa estar escrito

Os 8 eventos da política **não** provam que uma notificação do servidor chegou.
O barramento escuta `MsgCollection` e `ChatCollection` — ou seja, **eventos de
MODELO**. Ele vê o modelo mudar; não vê a mensagem que chega no fio.

Então a pergunta original da orquestração — *"a sessão recebe o evento e não o
aplica, ou o evento não chega?"* — **continua sem resposta** para participantes.
O que está provado é mais estreito e ainda assim útil:

> nada no modelo se move, e esta sessão não tem como saber por quê.

Responder a pergunta inteira exigiria escutar **abaixo** do modelo — o socket ou
o decodificador — que é outro instrumento e não existe aqui. Dizer isso é melhor
que deixar "0 eventos" parecer prova de que nada chegou.

### O estado do laboratório

O par foi removido e reposto, cada passo em sessão própria, e a confirmação entre
sessões viu 1 e depois 2. Nada ficou para trás.

**Status**: corrigido — a H58 continua válida, com o escopo agora estreito e
medido em vez de generalizado.

---

## H87 — cinco tipos de evento, cada um provado por ser DISPARADO

**Data**: 2026-08-21
**Contexto**: a ordem da orquestração, depois da fundação do barramento: mapear
os eventos que faltam.
**Onde**: `internal/wa-headless/events/`, `internal/wa-headless/eventtypesreal_test.go`.

### A regra que governa quais tipos existem

O upstream tem 31 eventos. Seria fácil declarar 31 nomes, instalar 31 ouvintes, e
entregar um barramento cujas metades mudas ninguém percebe por meses.

> **Um nome que nunca foi visto disparando é uma promessa, não uma capacidade.**

Então um tipo entra quando um teste ao vivo consegue **fazê-lo acontecer sob
demanda**, com uma capacidade que este módulo já entrega. Cinco entraram:

```
live events by type map[chat.changed:15 message.ack:4 message.added:1
                        message.edited:1 message.revoked:2]
```

`contact.changed` está instalado e **reportado como NÃO provado neste
barramento** — nada aqui faz outra conta mudar o próprio perfil. A coleção já
provou disparar (H43); o que não está provado é este caminho.

### O revoke precisou do predicado, não do campo

`change:isRevokedMsg` sozinho **não disparou**. A capacidade de apagar já sabia
por quê: a pós-condição dela checa TRÊS sinais (`isRevokedMsg`, `type ===
'revoked'`, `revokeSender`) porque este build não concorda consigo mesmo sobre
qual se move.

O ouvinte agora escuta os três campos e emite só quando o predicado inteiro dá
positivo — assim `change:type`, que dispara por outros motivos, não produz falso
evento.

### O defeito que a intermitência denunciou

`message.added` disparou numa execução e não na seguinte. Não era rede: o
`Replay` era desligado só num lote **não vazio**, e numa página quieta o primeiro
evento REAL — possivelmente minutos depois — vinha marcado como histórico. Um
assinante que ignora replay o ignorava.

A janela de replay agora fecha **após o primeiro dreno, vazio ou não**: se havia
rajada de histórico, ela já estava no buffer quando o primeiro dreno rodou; se o
dreno veio vazio, não havia rajada.

**Intermitência é sintoma, não categoria.** Marcar como flake teria deixado o
defeito.

### Placar

`PROVEN` 41, `PARTIAL` 35, `BLOCKED` 2, `INTENTIONAL_DIFFERENCE` 2,
`MISSING` 140, de 220.

`MESSAGE_CREATE` fica `PARTIAL` de propósito: o upstream distingue "criada por
mim" de "recebida" e o nosso `message.added` cobre as duas sem separar. É
divergência real, não equivalência.

**Status**: entregue.

---

## H87 (continuação) — a reação entra, e três eventos de grupo saem com motivo

**Data**: 2026-08-21.

`message.reaction` é o sexto tipo, disparado ao vivo por `react.Add`. O que ele
entrega está dito no tipo: **que as reações se moveram, não quais são**. A flag
em que ele viaja é pegajosa na sessão (H53) e o agregado que diria o emoji não
tem fonte neste build (H83). "Vá olhar" é mais que silêncio, e é honesto.

### E três eventos do upstream saem de `MISSING` mudo para `MISSING` medido

`GROUP_JOIN`, `GROUP_LEAVE` e `GROUP_ADMIN_CHANGED` **não podem ser entregues por
este barramento**, e isso não é opinião: a H86 mediu **zero** eventos na sessão
que mudou os participantes, contra oito para uma mudança de política no mesmo
grupo no mesmo minuto.

A diferença entre "não implementamos" e "medimos que este caminho não entrega"
é a diferença entre uma linha que convida alguém a tentar e uma que diz por onde
não adianta. As três agora dizem.

**Status**: entregue.

---

## H88 — o ciclo de vida da sessão entra no barramento por uma porta, não por dependência

**Data**: 2026-08-21.
**Contexto**: LIFECYCLE-EVENTS, a família que a orquestração mandou fechar
antes de voltar às capacidades.

### O problema que a decisão evita

Os seis tipos que o barramento já tinha vêm todos das coleções da página. Os
eventos de sessão do upstream — `READY`, `DISCONNECTED`, `STATE_CHANGED` — não
vêm de coleção nenhuma: **a página não tem como saber** que um boot verificou um
inventário de módulos, nem que o Go decidiu pará-la. Esses fatos só existem
deste lado.

O movimento óbvio seria o `core` publicar no Hub. Isso faria a camada que **é
dona do browser** depender da camada que apenas o reporta, e todo consumidor
futuro do barramento arrastaria o ciclo de vida da sessão atrás de si.

### O que foi feito

Uma porta, com as três pontas separadas:

```
core (declara o callback)  →  runtime (o único que conhece os dois lados)  →  events (segunda porta do Hub)
```

- `core/lifecycle.go`: `LifecycleFact` / `LifecycleObserver`, e
  `StartConfig.OnLifecycle`. **`core` não importa `events`.**
- `events`: `Hub.PublishSessionState`, `Origin` (`page` / `local`), e a divisão
  de `KnownTypes` em `PageTypes` + `LocalTypes`.
- `runtime/lifecycle.go`: `Holder.AttachHub` e `StateWatcher`. **É o único
  arquivo que importa os dois.**

Quatro tipos novos: `session.ready`, `session.boot_failed`, `session.stopped`,
`session.state_changed`.

### As quatro decisões que não são estilo

1. **`Seq` fica ZERO nos fatos locais.** O Hub conta salto de sequência como
   evento perdido. Um fato de ciclo de vida com sequência inventada entre dois
   eventos de página seria contado como perda — e o teste
   `TestLifecycleFactsDoNotCorruptTheGapCount` mede exatamente isso: com `Seq:
   999` o contador de lacunas foi a **991**.

2. **`Reason` é vocabulário FECHADO** (um `BootStage`, um `StopVia`), nunca um
   erro formatado. Mensagem de erro é por onde um caminho de perfil ou um jid
   acabaria vazando para o único lugar que toda capacidade lê.

3. **O observer roda FORA do mutex da sessão.** Um handler que reage à morte
   derrubando mais coisa chama `Stop` de volta. O controle negativo que emitiu
   dentro do lock não falhou com mensagem: **travou**, e o Go reportou
   `panic: test timed out after 40s`.

4. **Só a TRANSIÇÃO é evento.** O `StateWatcher` produz um veredito por tique e
   publica só quando ele muda. Um barramento que repete "ainda vivo" a cada dois
   segundos é heartbeat vestido de evento: custa um acordar a cada assinante
   para não dizer nada, e enterra a mensagem que importava. O **primeiro**
   veredito sempre é evento — sem ele, quem assina uma sessão já morta não ouve
   nada até ela mudar de novo, o que para um processo morto é nunca.

### O que NÃO entrou, e por quê

`AUTHENTICATED`, `CODE_RECEIVED`, `LOADING_SCREEN` e `REMOTE_SESSION_SAVED`
ficam `MISSING` **com motivo escrito**: não há fatia de pareamento, não há store
remoto, e o loop de settle mede CLASSES de página e não progresso de carga.
Declarar os quatro custaria nada e compraria um barramento que parece completo.

`AUTHENTICATION_FAILURE` subiu para `PARTIAL`: o evento carrega o **estágio** do
boot, mas não a **classe da página**, que é o que distingue "ainda montando" de
"tela de QR, precisa de um humano". `BootFailure` guarda a classe na mensagem de
erro, e mensagem de erro não entra no barramento (ver decisão 2). Fechar isso
pede um campo próprio em `BootFailure` — registrado, não feito.

### Defeito encontrado de lado: o gate do ledger contava a coisa errada

`TestTheLedgerUsesOnlyTheDeclaredVocabulary` procurava estados por **forma**
(maiúsculas entre crases, em qualquer lugar do arquivo). Uma NOTA que citou o
vocabulário de liveness deste módulo fez o gate acusar três estados que nunca
foram estados: `ALIVE`, `PROCESS_GONE`, `APP_ABSENT`.

A primeira correção — "o estado é a terceira célula" — **perdeu 28 linhas em
silêncio**, porque o ledger tem TRÊS formatos de tabela (sete colunas, quatro, e
duas para as caudas inteiramente não atacadas). O gate passou verde com 110 de
138 `MISSING`.

A correção certa lê o **cabeçalho da própria tabela** (`estado`) e usa aquele
índice nas linhas abaixo. É a terceira vez que este arquivo aprende a mesma
lição: ancore no que o documento diz de si, nunca no que as linhas parecem.

**Controles negativos executados** (todos falharam como devido):

| mutação | teste | saída |
|---|---|---|
| `LifecycleFact{Phase: PhaseReady}` sem `WasSuspect` | `TestLifecycle_ReadyRemembersTheProfileWasSuspect` | "a verified recovery is indistinguishable from an ordinary start" |
| emitir o stop DENTRO do mutex | `TestLifecycle_ObserverMayCallBackIntoTheSession` | `panic: test timed out after 40s` |
| não copiar `bf.Stage` para o `Reason` | `TestLifecycle_BootFailureCarriesTheStage` + `...EarliestFailureReports` | `reason "", want the stage "not_ready"` |
| `Origin: SourcePage` no publish local | `TestPublishSessionState_DeliversWithALocalOrigin` | `origin = "page", want "local"` |
| `Seq: 999` no publish local | `TestLifecycleFactsDoNotCorruptTheGapCount` | `gaps = 991, want 0` |
| `MessageAdded` também em `LocalTypes` | `TestTypeListsAreDisjointAndComplete` | `"message.added" is in both` |
| `AttachHub` sem a recusa pós-boot | `TestAttachHub_RefusesAfterTheBootItWouldHaveMissed` | "accepted a Holder that had already booted" |
| `moved := true` no watcher | `TestStateWatcher_OnlyTransitionsReachTheBus` | `9 events for one unchanging state` |
| watcher suprimindo o primeiro veredito | idem | `condition never held within 5s` |
| `getBatteryStatus` de `MISSING` para `PROVEN` (linha de DUAS colunas) | `TestTheLedgerScoreboardMatchesItsRows` | `says 138 MISSING and the rows contain 137` |
| estado `ALMOST` numa linha de SETE colunas | `TestTheLedgerUsesOnlyTheDeclaredVocabulary` | `state "ALMOST" is not in the declared vocabulary` |

As duas últimas provam que o gate corrigido enxerga os **três** formatos de
tabela — que é a falha que a primeira correção tinha.

### Um erro meu no caminho

O primeiro `TestStateWatcher_OnlyTransitionsReachTheBus` começava com o processo
VIVO e um avaliador que falhava, e acusou o watcher de repetir eventos. Não
repetia: `PAGE_SLOW` vira `PAGE_UNRESPONSIVE` quando a sequência de falhas cruza
o limiar do monitor, então a perna "estado que não muda" estava medindo um
estado que muda sozinho. Trocado por `PROCESS_GONE`, que é estável porque
curto-circuita antes de qualquer sonda de página.

**Status**: entregue.

---

## H89 — GROUP-REQUESTS: a família inteira, e a rejeição que era a resposta

**Data**: 2026-08-21.
**Contexto**: primeira família aberta sob a regra nova da orquestração —
métodos **e** eventos no mesmo bloco.

### A referência acertou os nomes pela primeira vez

Quatro módulos do `whatsapp-web.js` existem neste build **sem tradução**:
`WAWebApiMembershipApprovalRequestStore`, `WASmaxGroupsMembershipRequestsActionRPC`,
`WAWebWidToJid`, `WAWebGroupInviteJob`. Contra **quatro de quatro ausentes** no
`sendText` (H34). Vale registrar exatamente porque o oposto tem sido a norma:
copiar a lista às vezes funciona, e a única forma de saber é medir.

### O refresh "específico" que NÃO foi usado, e por quê

Este build exporta `WAWebGroupQueryJob.maybeQueryAndUpdateMembershipApprovalRequests`,
que a referência não usa. Parecia a escolha óbvia — mais barata que refazer a
metadata inteira. Medido:

| argumento | resultado |
|---|---|
| wid | `resolved: undefined` |
| `{id: "...@g.us"}` | `resolved: undefined` |
| string crua | `resolved: undefined` |
| objeto chat inteiro | `resolved: undefined` |

**Aceita quatro formas incompatíveis e resolve `undefined` em todas.** Uma função
que não consegue recusar um argumento errado não consegue confirmar um certo:
não há como distingui-la de um no-op. Ficou o `queryAndUpdateGroupMetadataById`
da referência.

### A rejeição ERA a resposta

`joinGroupViaInvite` num grupo que pede aprovação **não resolve**: ele **rejeita**,
com um objeto chamado `UnexpectedJoinGroupViaInviteResponse` cujas chaves são
`[message, taalOpcodes, name, gid, membershipApprovalMode]`.

O `wwebjs` faz `res.gid._serialized` sobre o valor RESOLVIDO e quebraria aqui.
Copiar a forma dele teria falhado; copiar o entendimento — "existe um módulo que
entra por convite" — funcionou.

Reconhecemos pelo **campo carregado** (`membershipApprovalMode`), não pelo nome:
o nome é detalhe de build, o campo é o fato.

**Isto só apareceu porque o primeiro relato de erro estava vazio.** O script
reportava `String(e && e.message)` e este app rejeita com objetos simples tanto
quanto com `Error`. Um motivo vazio é pior que um errado: parece falha silenciosa
quando na verdade o diagnóstico foi jogado fora na fronteira. O relato agora
carrega construtor, nome, status, code e **as chaves** do que foi lançado.

### O que a prova ao vivo mediu que nada mais mediria

Campos de um pedido real: `[id, t, addedBy, requestMethod, parentGroupId]`. Os
dois últimos não estão em lugar nenhum da referência. `requestMethod` distingue
"seguiu um link" de "alguém tentou adicionar" — que é a diferença sobre a qual um
admin decide.

E o solicitante chega como **`@lid`**, não batendo com o `@c.us` do peer. A
primeira versão do teste comparava jids para provar proveniência e teria
**reprovado uma execução correta**. A proveniência vem da linha de base — zero
pendentes antes — que é a afirmação mais forte.

### O evento, medido com a saída SUBTRAÍDA

Primeira medição: 14 `chat.changed` durante "sair + pedir". Número que não diz
nada, porque uma saída também mexe no chat. Isolado:

| ação | barramento |
|---|---|
| saída de conta-B sozinha | `chat.changed:5`, `message.added:1` |
| **pedido sozinho** | `chat.changed:9`, `message.added:1` |

O pedido **é** observável nesta sessão. Não há tipo dedicado e `chat.changed` é
grosso demais para ser um, então fica `PARTIAL` com a medida escrita.

Contraste com a H86: a sessão que MUDA participantes vê **zero**. Quem recebe
enxerga; quem age não.

### Dois defeitos meus, no meu próprio teste

1. **`t.Cleanup` roda DEPOIS de todo `defer`.** A restauração da política usava
   `t.Cleanup` e falhou com `context canceled`, porque `defer hA.Stop` já tinha
   derrubado a sessão. A medição tinha dado certo e o grupo ficou **armado**.
   `defer` registrado depois do `Stop` roda antes dele (LIFO).

2. **Uma restauração que falha envenena o "original" da execução seguinte.** A
   corrida seguinte leu `approvalWas = true` e restaurou para `true`. A saída
   não foi um cleanup mais esperto: foi `TestLabSetApprovalMode`, que diz o que
   o grupo DEVE ser, independente do que ele está.

Estado do laboratório ao fim: 2 participantes, aprovação **desligada**,
zero pendentes — verificado por sonda depois de tudo.

**Controles negativos executados**:

| mutação | teste | saída |
|---|---|---|
| refresh DEPOIS da leitura | `TestTheListRefreshesFromTheServerFirst` | "the refresh runs AFTER the read; the read would answer with boot-time metadata" |
| `rejectArgs` trocado por `approveArgs` | `TestApproveAndRejectSendDifferentKeys` | "Reject did not send rejectArgs" / "Reject also sent approveArgs" |
| `if !out.OK` virado `if false` | `TestAPageFailureIsNotAnEmptyList` | `err = <nil>, want ErrRead` |

### Achado incidental, NÃO corrigido

`capabilities/group/policy.go:161`, doc de `PolicyOf`:

> IT IS CORRECT AT SESSION START AND STALE AFTER A CHANGE THIS SESSION MADE,
> the same as Count — which is what makes cross-session the only honest proof.

Isso é **falso desde a H85**, que mediu políticas visíveis na mesma sessão em
~1s, e foi contrariado de novo hoje: `TestLabSetApprovalMode` lê `true -> false`
na mesma sessão que fez a mudança. É prosa sobrevivente da hipótese ampla demais
da H58. Correção sugerida: reescrever o comentário citando a H85 e a medição de
hoje; `Count` continua sendo o caso stale de verdade, e juntar os dois é o erro
original.

**Status**: entregue. ~~O achado do `PolicyOf` fica pendente~~ — **corrigido na H91**, no mesmo dia, junto com um terceiro caso da mesma frase herdada em `pin.go`.

---

## H90 — ADDRESS BOOK, e o único evento que tinha ouvinte e não tinha prova

**Data**: 2026-08-21.
**Contexto**: segunda família sob a regra de fechar métodos e eventos juntos.

### Por que esta família fecha um evento

`events.ContactChanged` estava instalado desde a H87 e **nunca provado**: o
barramento tinha ouvinte e nenhuma forma de fazê-lo disparar, porque nada neste
módulo movia um registro de contato — os únicos caminhos passavam por *outra*
conta editar o próprio perfil, que este lado não causa.

`saveContactAction` move o registro **nesta** sessão. Medido: nomear o par
produziu **8** `contact.changed`. O tipo sai de promessa para capacidade.

### Duas assimetrias que a referência esconde

1. **O save quer dígitos crus; o delete quer wid.** O `wwebjs` passa o mesmo
   valor aos dois. Aqui, `createWid("5541…")` sem sufixo morre com
   `wid error: invalid wid` — que foi como o primeiro delete ao vivo falhou,
   **depois** de o save ter dado certo, deixando um contato que a limpeza não
   conseguia remover. Travado por `TestTheDeleteBuildsAWidAndTheSaveDoesNot`.

2. **`getDeviceIds` tem aridade 2 aqui**, e a referência passa um argumento. O
   par não tem registro de dispositivo nesta conta, e "sem registro" **não** foi
   fundido no número 0: um usuário com quem nunca se trocou chave pareceria um
   usuário sem telefone.

### Eu quebrei a invariante 6, sabendo dela

A primeira versão do save consultava o registro com `setTimeout` **dentro da
página**. Funcionaria. Nada na suíte notaria.

A regra não é frescura: uma página que conta o próprio tempo limite conta
durante um reload, uma aba estrangulada e um renderizador travado, num lugar
onde o Go não vê a decisão nem a cancela. Todo prazo deste módulo está do lado
Go justamente para que o contexto de quem chama signifique alguma coisa.

Isto virou **gate**: `gate_pageclock_test.go` varre os scripts de página de
produção atrás de `setTimeout`, `setInterval`, `requestAnimationFrame` e
`Date.now`. Duas exceções, **com motivo escrito**:

| arquivo | motivo |
|---|---|
| `events/ingress.go` | `Date.now()` **carimba** um evento; `Event.At` documenta que é diagnóstico e nunca decide |
| `capabilities/group/group.go` | **PRÉ-EXISTENTE, achado incidental** — ver abaixo |

O gate também verifica que **a exceção ainda é verdadeira**: um arquivo isento
que perdeu o construto é uma isenção que ninguém percebe ter vencido.

### Achado incidental — NÃO corrigido

**Onde**: `capabilities/group/group.go:274`

```js
if (!chat) { await new Promise(r => setTimeout(r, 250)); }
```

**Problema**: a espera pela criação do grupo dorme **na página**, violando a
invariante 6 pelo mesmo motivo acima. Está fora do escopo desta tarefa e é
pré-existente.

**Correção sugerida**: parkear assim que a chamada é aceita e consultar o chat
por um script síncrono, com o laço em Go sob o orçamento de quem chama — que é
exatamente a forma que `addressbook.Save` acabou tendo.

**Status**: não corrigido; nomeado na allowlist do gate para não passar
despercebido.

### E eu caí na H85, do outro lado

O teste ao vivo falhou com "o `contact.changed` nunca disparou". Não era o
barramento: o contato **já tinha o nome** — sobra de uma execução cujo delete
havia falhado — então o save escreveu o valor que o registro já continha. **Um
no-op não move leitor nenhum.**

É a armadilha da H85 vista do outro lado: lá, um controle que não podia falhar
CONFIRMOU uma hipótese falsa; aqui, um controle que não podia falhar ACUSOU um
mecanismo correto.

O conserto não foi esperar mais: o teste agora **apaga o nome primeiro** e
afirma `HadName == false` antes de olhar para o barramento. Se o registro ainda
carrega nome nessa linha, nada depois dela prova coisa alguma.

**Controles negativos executados**:

| mutação | teste | saída |
|---|---|---|
| `setTimeout` de volta no script do save | `TestNoClockInProductionPageScripts` | "invariant 6: a page script decides its own waiting at capabilities/addressbook/script.go:77" |
| isentar um arquivo cujo relógio só existe em comentário | idem | "is allowlisted (…) and no longer contains a clock; remove the exception" |

**Status**: entregue. ~~O achado do `group.go:274` fica pendente~~ — **corrigido na H91**, no mesmo dia, por decisão explícita da orquestração: uma violação conhecida não mora numa allowlist.

---

## H91 — as duas correções que a orquestração mandou fazer, e a H50 reaberta pela metade

**Data**: 2026-08-21.

### 1. O `setTimeout` saiu de `group.go` — a allowlist não é lugar de morar

A H90 nomeou `capabilities/group/group.go:274` na allowlist do gate novo em vez
de corrigir, por estar fora do escopo. A decisão que voltou foi explícita e vale
guardar como regra:

> *Não quero uma violação conhecida da invariante 6 estabilizada numa allowlist.
> A allowlist pode existir apenas enquanto a correção está no mesmo bloco.*

A espera pelo chat recém-criado agora **parkeia** `awaiting_chat` e para. O laço
de polling que já existia no Go passou a tratar esse estágio como "continue", e
o script de leitura virou `ensureVerifyScript`: síncrono, gasta um turno
procurando o chat, e escreve o resultado final quando acha.

Dois ganhos que não eram o objetivo:

- **O corpo de verificação virou uma string compartilhada** (`verifyFnJS`), usada
  pelos dois caminhos. Antes existia uma cópia só; agora que há dois lugares
  perguntando "quem está de fato no grupo?", uma cópia que divergisse deixaria um
  caminho aplicando a pós-condição e o outro deixando de aplicar.
- **"Criado mas invisível" deixou de ser "a página não respondeu"**. Os dois
  compartilhavam uma mensagem. O grupo existe no servidor — `createGroup`
  devolveu um wid — e a coleção nunca o mostrou; quem lesse "never settled"
  procuraria no lugar errado.

**Controles negativos executados**:

| mutação | teste | saída |
|---|---|---|
| laço do Go sem tratar `awaiting_chat` | `TestTheChatIsWaitedForFromGo` | `Ensure: … at awaiting_chat ()` |
| remover a mensagem específica de "criado mas invisível" | `TestAGroupThatNeverAppearsSaysSo` | `err = … the page never settled within 200ms, want the created-but-invisible reason` |

### 2. `PolicyOf` — e um TERCEIRO lugar com a mesma frase herdada

O doc de `PolicyOf` afirmava que políticas ficam stale na sessão que as mudou,
"como o `Count`". Falso desde a H85 e contrariado de novo pela H90. Corrigido —
e o comentário agora **conta a correção** em vez de simplesmente ficar certo,
porque a generalização errada da H58 é a coisa que se repete.

Procurando a frase, achei-a em mais três arquivos:

| onde | veredito |
|---|---|
| `group/participants.go:189` (`Count`) | **verdadeira** — é o caso stale de verdade (H58) |
| `chats/markunread.go:89` (`UnreadCount`) | **verdadeira** — medida na despromoção do `sendSeen` |
| `pin/pin.go:164` (`PinnedIn`) | **afirmação sem medição** — corrigida |

A do `pin` merece nota: ela descreve o que acontece *depois de um pin
bem-sucedido*, e **nunca houve um** (H81 — a página aceita e nada é fixado).
Descrevia o rescaldo de um evento que não ocorre, que é a forma exata do defeito
da H83. O comentário agora diz que a classe de escrita é desconhecida e como
medi-la quando um pin passar a funcionar.

### 3. H50 reaberta, e a quarta hipótese ficou mais estreita

Ver a atualização dentro da própria H50. Resumo: salvar deixou de precisar de
humano, o save funcionou (`isAddressBookContact = 1`, nome presente), e a
presença **continua sem subscrição** — porque `isContactSyncCompleted = 0`. O
servidor não conhece um contato que nunca foi sincronizado.

**Estado de laboratório deixado de propósito**: as duas contas ficam salvas uma
na outra, com os nomes `wa-headless-lab A` e `wa-headless-lab B`. É configuração
útil e reconhecível como artefato de teste; desfazer com
`WA_LAB_MUTUAL=forget go test -run TestLabMutualAddressbookSave`.

**Status**: entregue.

---

## H92 — CALLS: o link entregue, a recusa pronta, e o gatilho que toca um telefone

**Data**: 2026-08-21.
**Contexto**: terceira família sob a regra de métodos e eventos juntos.

### A varredura de módulos rendeu mais que a referência inteira

Enumerar todo módulo carregável cujo nome contém `Call` — 228 nomes, 155
carregáveis — encontrou uma superfície VOIP que o `whatsapp-web.js` **não tem
equivalente para**:

| módulo | o que faz |
|---|---|
| `WAWebVoipStartCall` | **origina** chamada (`startWAWebVoipCall`, aridade 5) |
| `WAWebVoipCancelOutgoingCall` | cancela a que está saindo |
| `WAWebVoipCreateCallLink` | link de chamada próprio deste build |
| `WAWebVoipCallStateUtils` | `isCallIncoming`, `isCallRinging`, `isCallTerminal` |
| `WAWebCallCollectionUtils` | `buildCallPropsFromOffer`, `createCallModel` |

Isto é o método da H78/H79/H80 finalmente pagando: **enumerar largo e filtrar a
SAÍDA por nome de função**, em vez de adivinhar o nome do módulo.

### Duas funções com nome parecido, e a errada trava

`WAWebVoipCreateCallLink.createCallLink` parecia a escolha óbvia — é deste build
e tem o nome exato da linha do ledger. **Travou.** `createCallLink('video')` não
resolveu em 40 s, mesma classe de resposta que o travamento de convite da H57:
quer uma pilha VOIP que a sessão headless não sobe.

A `Client.createCallLink` do `whatsapp-web.js` **não chama essa**. Chama
`WAWebGenerateEventCallLink.createEventCallLink`, que pertence a eventos
agendados. Essa resolve, com string de 54 caracteres, para `voice` e `video`.

> **Defeito meu no instrumento, e a correção vale mais que a medição.** A
> primeira sonda parkeava o resultado **uma vez, no fim** — então o travamento
> fez ela reportar "never settled" e **jogar fora as quatro respostas que já
> tinha**. Agora parkeia depois de cada tentativa e marca `PENDING` antes: o
> travamento continua sendo resposta, e agora diz QUAL chamada travou.

### A estranha à mão não é improviso da referência

Este build **não exporta ação de recusa nenhuma**: `WAWebRejectCallAction`,
`WAWebEndCallAction`, `WAWebCallActions` e `WAWebOfferCallAction` todos ausentes
(medido). A estranha `call`/`reject` montada à mão e lançada por
`WADeprecatedSendIq` é o único caminho — o que muda a leitura de "a referência
gambiarrou" para "é assim que se faz".

`Reject` não tem pós-condição local que dê para checar: a evidência de quem
chama é o telefone parar de tocar. Está dito no doc em vez de disfarçado com uma
leitura que sempre passaria.

### O que NÃO foi feito, e por quê — decisão humana, não de escopo

`INCOMING_CALL` e a prova ao vivo de `reject` exigem uma chamada entrante. O
gatilho existe e está nomeado. **Não disparei.**

Toda outra ação externa desta suíte aterrissa dentro de um aplicativo. Esta faz
**hardware fazer barulho** no bolso de alguém. O teste
`TestIncomingCallReal` existe, pula por padrão, e diz isso no motivo do skip —
em vez de ficar ligado numa execução que alguém começa de madrugada.

**Controles negativos executados**:

| mutação | teste | saída |
|---|---|---|
| aceitar link vazio como sucesso | `TestAnEmptyLinkIsAnError` | `answer {"ok":true,"link":""} gave <nil>, want ErrNoLink` |
| tirar `call-creator` da estranha | `TestTheRejectStanzaIsShapedLikeTheProtocolWantsIt` | `the stanza is missing "call-creator"` |
| remover a validação de `Kind` | `TestOnlyVoiceAndVideoAreAccepted` | `CreateLink("") = <nil>` e `"" reached the page` |

**Status**: entregue parcialmente — link provado ao vivo; recusa implementada e
travada por teste, aguardando autorização humana para a chamada que a provaria.

---

## H93 — a chamada autorizada: três hipóteses derrubadas, e um leitor que nunca contou nada

**Data**: 2026-08-21.
**Contexto**: continuação da H92, com autorização humana explícita para originar
uma chamada real entre as contas de laboratório.

### O que foi medido, em ordem

| passo | resultado |
|---|---|
| `startWAWebVoipCall(wid, false)` | resolve, sem erro |
| **o telefone tocou?** | **não** — confirmado pelo humano, que é o único instrumento que responde isso |
| `isCallingEnabled` | `true` |
| `isUnsupportedBrowserForWebCalling` | `false` |
| `getUnsupportedBrowserReason` | `null` |
| `crossOriginIsolated` / `SharedArrayBuffer` / `WebAssembly` | todos presentes |
| `ensureVoipInitialized()` | **resolve**, nos dois lados |
| depois do init nos dois lados, `Pending` da conta-A | **0** |
| barramento da conta-B | zero `call.incoming` |

### Três hipóteses minhas, todas derrubadas por medição

1. **"O ambiente headless não suporta chamada."** Falsa: o gating da própria
   página diz que chamada está habilitada e o navegador é suportado, com
   isolamento cross-origin e WASM presentes.
2. **"A pilha VOIP nunca sobe."** Falsa: `ensureVoipInitialized()` resolve.
3. **"Falta inicializar antes de discar."** Plausível e implementada — o
   `Place` agora inicializa antes, o que tem a forma exata da lição da H34
   (chamar antes a resolução que a página já tem). **Não bastou.**

### O que este resultado NÃO prova, e por que isso importa

`Pending` da conta-A leu **0** depois de discar. A leitura tentadora é "o
disparo não fez nada". Ela não é válida:

> **Este leitor nunca foi observado contando uma chamada.** Todas as execuções
> dele, em todo o histórico, devolveram zero.

É exatamente a armadilha número 2 do `ARMADILHAS.md` — *teste o caminho de
SUCESSO, não só a recusa*. Um contador que só foi visto devolvendo zero não
distingue "não há nada para contar" de "não sei contar". Separar "o disparo não
fez nada" de "o leitor não conta nada" exige uma chamada que se saiba existir, e
essa é justamente a coisa que falta.

Por isso a linha do ledger é `PARTIAL` com a história escrita, e não uma
conclusão sobre o `startWAWebVoipCall`.

### O achado que vale mais que o resultado das chamadas

**Uma sessão recém-iniciada INUNDA o barramento.** Medido em três execuções
consecutivas, na conta-B, com o bus instalado logo após o boot:

| execução | `chat.changed` | `message.added` | `contact.changed` |
|---|---|---|---|
| 1 | 2220 | 530 | 30 |
| 2 | 2099 | 441 | 25 |
| 3 | 1898 | 327 | 485 |

Nada disso é atividade: é a **sincronização de histórico** que a SPA faz ao
subir. E tudo chega marcado como **live**, porque a janela de replay fecha
depois do PRIMEIRO drain (H87) — e a sincronização continua por minutos depois
dele.

A H87 corrigiu o caso oposto (numa página quieta, o primeiro evento REAL era
marcado como histórico) e a correção está certa para aquele caso. O que ela não
viu é que a mesma janela é curta demais para um boot: um consumidor que conte
`message.added` contaria **centenas de mensagens antigas como novas** na primeira
execução depois de cada boot.

**Correção sugerida, NÃO aplicada** (é decisão de projeto, não conserto óbvio):
fechar a janela por um sinal da própria SPA — o fim da sincronização inicial —
em vez de pelo primeiro drain. Se esse sinal não existir, a alternativa é o
consumidor receber `Replay` até que a taxa caia, o que troca uma regra clara por
uma heurística e merece decisão explícita.

### E a F100 finalmente ganhou uma CAUSA, na nona ocorrência

O `make check` ficou vermelho com
`Boot(open_tab/prime): deadline of 30s exceeded` — numa sessão configurada para
**noventa** segundos. O orçamento injetado não estava chegando ao lugar que
importa:

```go
tab, err := engine.OpenTab(sessionCtx, browser)   // DefaultDeadlines.For(OpBoot)
```

`engine.OpenTab` fixa o prazo padrão. Um chamador que subiu
`Runner.Policy.Boot` — que é exatamente o que o harness faz, e o que o
`harnessbudget_test.go` documenta — tinha **todo** passo do boot honrando isso
**menos a preparação da aba**. E essa é justamente a que demora sob contenção,
então o orçamento maior ajudava onde não era preciso.

Trocado por `engine.OpenTabWithin(sessionCtx, browser, runner.Policy.For(engine.OpBoot))`.
Comportamento de produção inalterado: `engine.NewRunner` parte dos padrões.

A asserção é sobre o **código-fonte**, não sobre tempo — tempo é o que tornou
essas oito falhas ilegíveis: um teste que espera um boot lento falha em máquina
rápida e passa em código quebrado.

**Depois disso o gate caiu de novo**, agora esperando os noventa segundos
inteiros. Isso não era mais defeito: eram **quatro boots de browser a mais** que
a H88 acrescentou a um pacote que já sobe dezenas, sob `-race`. Duas medidas,
nesta ordem:

1. **Apaguei um teste meu** — o de observador nulo — porque **toda** outra prova
   deste pacote já o afirma, deixando `OnLifecycle` nulo. Um teste cuja
   propriedade a suíte já carrega não é grátis: é pago em relógio a cada
   execução, e a conta chega como uma intermitência que parece defeito.
2. **Subi o teto do harness de 90s para 150s.** Subir teto é o movimento que
   este repositório mais desconfia, então veio com as duas coisas que o tornam
   honesto: continua estritamente maior que o de PRODUÇÃO, e um boot travado
   continua terminando dentro dele — as duas travadas por teste que já existiam.

**Status**: parcialmente entregue — `Place`, `Cancel` e `EnsureReady`
implementados e travados por teste; `INCOMING_CALL` e a prova ao vivo de
`reject` continuam sem disparo, com o caminho todo medido. O achado da inundação
do barramento fica pendente de decisão. A F100 fica **corrigida na causa** e o
teto do harness registrado.

---

## H94 — presença: a agenda destravou a subscrição UMA vez, e o leitor de digitação lia um campo que não existe

**Data**: 2026-08-21.
**Contexto**: reabertura da H50 com autorização humana para
`syncToAddressbook=true`.

### A quarta hipótese da H50 teve uma observação a favor — e ela NÃO se sustentou

> **CORREÇÃO, escrita no mesmo dia, algumas horas depois.** O parágrafo abaixo
> descreve o que foi observado, e a leitura causal que eu tirei dele está
> **errada**. Leia a seção "O que a repetição fez com esta hipótese" no fim
> desta entrada antes de usar qualquer coisa daqui.

Com as duas contas salvas na agenda uma da outra **e sincronizadas**, o teste ao
vivo leu `subscribed=true` — o que **nunca** tinha acontecido. A H50 registrou a
mesma coisa uma vez e nunca mais; a H91, com `sync=false`, não conseguiu nem
isso.

O que ficou faltando na mesma execução: `online=false` e `chatstate=""` enquanto
a outra conta anunciava `composing`.

### E aí apareceu um defeito de verdade, na capacidade entregue

`Observe` lia `p.chatstate.type`, que é o que a referência documenta. Medido
neste build, no modelo de presença real:

| campo | valor |
|---|---|
| `chatstate.type` | **undefined** |
| coleção `chatstates` | **vazia** |
| `typingUserIds` | `array` — **é aqui** |
| `recordingUserIds` | `array` |

**O leitor antigo não podia reportar outra coisa senão `""`.** É a mesma classe
do defeito da H83 (o agregado de reações que era chamado sempre e lançava
sempre), só que mais silenciosa: ler campo indefinido nem lança. Uma capacidade
que sempre responde a mesma coisa é indistinguível de uma que não está ligada —
e esta foi entregue assim.

Corrigido: `composing` quando `typingUserIds` não está vazio, `recording` quando
`recordingUserIds` não está. **Contagens, nunca os ids** — aquelas listas contêm
identidades.

O doc do `Snapshot.ChatState` afirmava valores "medidos" que jamais foram
observados aqui (`paused`, `available`, `unavailable`). Reescrito para dizer o
que este build produz de fato, com a afirmação antiga preservada como o motivo
da mudança.

### O que continua sem resposta, com a hipótese trocada

Depois daquela única execução boa, a subscrição voltou a falhar. Três execuções
seguidas, **uma delas com orçamento de 120 s** contra os 20 s padrão:

> **Não é constante de tempo.** Dois minutos inteiros e `isSubscribed` não vira.

Isso derruba a hipótese mais barata e sugeriu uma nova — **a subscrição parece
funcionar uma vez por janela** — que a tentativa com 40 minutos de intervalo
também derrubou. Ver a seção final: nenhuma das duas ocorrências positivas está
explicada.

`subscribeBudget` virou `var` com `SetSubscribeBudget`, exatamente para que essa
distinção seja mensurável em vez de discutível — e o doc diz que é para isso, em
vez de deixar um botão sem propósito declarado.

**Próximo experimento, barato**: repetir depois de um intervalo longo. Se voltar
a passar, a hipótese de janela ganha uma segunda observação.

### Um vazamento de PII meu, na sonda

A primeira execução da sonda de presença imprimiu quatro dígitos de um número de
conta, porque o texto de erro da própria página cita o jid que ela recusou.
Mensagem de erro não é isenta da regra. A sonda agora redige antes de reportar,
no ponto onde a mensagem é MONTADA, não onde é lida.

### E a oitava vez que uma asserção casou com a própria prosa

O teste do leitor novo procurava `p.chatstate.type` no script e achou **o
comentário que explica por que o script não o lê mais**. Mesmo conserto que a
`capabilities/call` precisou: tirar os comentários antes de casar. O helper está
duplicado nos dois pacotes porque um auxiliar de teste não atravessa pacote sem
virar código de produção que ninguém chama.

**Controles negativos executados**:

| mutação | teste | saída |
|---|---|---|
| `typing = 0` fixo no script | `TestTheTypingStateIsReadFromTheListsThatMove` | `the observe script does not read p.typingUserIds` |

### E a F100 caiu mais duas vezes, pelo mesmo buraco

O `make check` reprovou de novo com `deadline of 30s` — depois de o teto do
harness já estar em 150 s. Causa: **dois testes montam o `StartConfig` à mão** e
nunca receberam o `Runner` do harness. Ficaram com o prazo de PRODUÇÃO enquanto
o resto do pacote tinha cinco vezes mais, e sob contenção eram justamente eles
que caíam — o que se lê como intermitência, não como configuração esquecida.

É exatamente a nota da F100 que dizia que a primeira correção era estreita
demais, agora concreta.

Uma regra em comentário seria esquecida do mesmo jeito, então virou **gate**:
todo literal `StartConfig{` nos testes deste pacote nomeia um `Runner`, com
exceção só por marcador `no-runner:` e motivo escrito.

> **O gate reprovou a si mesmo na primeira execução**, porque a linha que
> procura `StartConfig{` contém `StartConfig{` dentro de uma string. Comentário
> já era ignorado; string entre aspas é a mesma armadilha com pontuação
> diferente. Terceira forma dela nesta sessão — prosa, comentário, e agora
> literal — e as três se resolvem tirando o que não é código antes de casar.

**Controle negativo executado**: remover o `Runner` de um dos dois consertos →
`StartConfig literal(s) with no Runner at session_test.go:422`.

### O que a repetição fez com esta hipótese

Quatro tentativas depois, sob condições variadas, a subscrição **não voltou a
funcionar nenhuma vez**:

| tentativa | condição | resultado |
|---|---|---|
| 1 | orçamento padrão de 20 s | não subscrito |
| 2 | repetição imediata | não subscrito |
| 3 | orçamento de **120 s** | não subscrito |
| 4 | ~40 min de intervalo | não subscrito |
| 5 | **esquecer, re-salvar com sync, e medir em seguida** — a sequência exata que produziu o sucesso, e desta vez com mudança real (`hadName=false`) | não subscrito |

A tentativa 5 é a que decide. Ela reproduz o cenário do sucesso passo a passo e
não reproduz o sucesso.

**Então a leitura causal cai.** "Sincronizar a agenda destrava a subscrição" era
uma inferência de UMA observação, e este repositório já tem regra para isso:
quando a medição contraria a hipótese, a hipótese cai — inclusive se já estiver
escrita num HOUSEKEEP com número, porque um achado com diagnóstico errado é pior
que nenhum, já que parece resolvido.

O que continua verdade: `subscribed=true` foi observado duas vezes na história
do projeto (H50 e aqui), e nenhuma das duas foi explicada. O que deixa de ser
verdade é que a agenda seja a explicação.

**Estado de laboratório**: as contas continuam salvas uma na outra, com
`syncToAddressbook=true`. Não faz mal e aproxima o laboratório do mundo real;
desfaz com `WA_LAB_MUTUAL=forget`.

**Status**: parcialmente entregue — leitor de digitação corrigido e travado, que
é um defeito real consertado. A subscrição continua **sem explicação**, agora com
cinco tentativas negativas documentadas e uma hipótese a menos. A F100 ganhou
gate.

---

## H95 — a chamada não sai: cinco hipóteses eliminadas, e o zero que finalmente vale

**Data**: 2026-08-21.
**Contexto**: continuação autorizada da H93, com o usuário liberando as duas
pendências físicas.

### O bloqueio da H93 era o INSTRUMENTO, e ele foi consertado primeiro

A H93 parou num zero que não podia interpretar: a conta-A lia zero chamadas
depois de discar, e aquele leitor **nunca tinha sido observado contando uma**.
Dois defeitos diferentes produzem o mesmo zero.

O conserto foi despejar **todo contêiner** da coleção de chamadas, por nome e
tamanho, antes e durante:

```
$CallCollectionImpl$p_1  Map:0        pendingOutgoingCall  null
pendingOffers            object       lastActiveCall       null
pendingVoipCapChecks     object       pendingCallLink      null
isInConnectedCall        boolean      #models              0
```

`pendingOutgoingCall` é exatamente o campo que ficaria não-nulo numa chamada
saindo. **Nada mudou em nada, por 20 s.** Agora o zero vale.

### Cinco hipóteses, todas derrubadas por medição

| hipótese | como caiu |
|---|---|
| ambiente headless não suporta | `isCallingEnabled` true, navegador suportado, `crossOriginIsolated`, `SharedArrayBuffer`, WASM |
| a pilha VOIP não sobe | `ensureVoipInitialized()` **resolve** |
| falta inicializar ANTES de discar | implementado (forma da H34); sem mudança |
| falta abrir a aba de Chamadas | os call sites do app fazem `setActiveNavBarItem(Calls)` + `navigateToVoipCallsTab({})` antes; feito igual, `navBar:ok tab:ok`; sem mudança |
| o leitor não conta | todo contêiner observado por nome; `pendingOutgoingCall` continua `null` |

E o que ninguém tinha lido: **`startWAWebVoipCall` devolve `undefined`**. A
capacidade dava `await` e descartava o valor — certo para uma capacidade,
inútil para um diagnóstico.

### Conclusão

Classe **NOTHING** (H82), terceira ocorrência neste repositório: a página aceita
e nenhuma parte do aplicativo reage. Diferente das duas anteriores, esta vem com
o campo que deveria mudar identificado por nome, o que torna a próxima
investigação muito mais barata.

`Place` fica no código, implementado exatamente como os call sites do app, mas
**o doc dele agora diz que não funciona** e lista as cinco eliminações. Uma
capacidade que não faz nada e não diz isso é pior que uma ausente.

### Sexta eliminação, e o fim da investigação por ordem da orquestração

A hipótese (b) foi executada como investigação **limitada**: o app tem um ponto
de decisão explícito para "não deu para ligar", e se a recusa fosse interna a
razão estaria lá.

```
WAWebVoipCallBlockedModals.showCallBlockedModalIfNeeded()  ->  false
```

**A própria SPA diz que a chamada NÃO está bloqueada.** As quatro funções do
módulo têm aridade zero, então nem sequer há um alvo por quem a recusa pudesse
ser específica.

Seis eliminações, nenhuma causa acionável. A ordem foi explícita: não entrar em
(a) — instanciar componente React —, marcar a família e seguir. *"Reabra apenas
com evidência nova; não continue enumerando hipóteses."*

`INCOMING_CALL` fica **`BLOCKED`**, que é o estado do vocabulário para "atacado,
medido, e impedido por algo fora do nosso alcance". Depois de seis medições,
incluindo o veredito do próprio aplicativo, essa é a leitura honesta: o
impedimento está no caminho VOIP da SPA, que nós dirigimos e não inspecionamos.

**Condição de reabertura**: evidência nova — não hipótese nova. Concretamente,
qualquer coisa que faça `pendingOutgoingCall` deixar de ser `null`.

### O que sobra para tentar, quando houver ideia nova

- O app pode exigir um COMPONENTE montado, não só a aba navegada — a H78 mostrou
  que `Cmd` só age com ouvinte ligado, e um ouvinte pode viver num componente
  React que a navegação não instancia sozinha.
- Ler o que `WAWebVoipCallBlockedModals.showCallBlockedModalIfNeeded` decide:
  o app tem modal para "não deu para ligar", e se ele é consultado internamente,
  a razão da recusa está lá.

**Status**: não entregue — cinco hipóteses eliminadas, causa isolada até o ponto
de "a função devolve `undefined` e nada reage", e o campo que mudaria nomeado
para quem continuar.

---

## H96 — EVENT-REPLAY-FIX: o sinal explícito existe e mede outra coisa

**Data**: 2026-08-21.
**Contexto**: a orquestração pôs isto na frente de polls/votes, com a razão
escrita: *"deixar ~2400 eventos históricos por boot classificados como live é
mais grave do que adicionar nova superfície."*

### A ordem era procurar um barrier explícito. Procurei, e ele não serve

A instrução foi específica: opção (i), um sinal da própria SPA de fim de
sincronização — **e nada de heurística por queda de taxa**. A varredura de
módulos achou dois candidatos de nome perfeito:

| candidato | o que respondeu |
|---|---|
| `WAWebUserPrefsHistorySync.getInitialHistorySyncComplete()` | **`true` em t+0**, antes de qualquer coisa carregar — é flag PERSISTIDA da conta, não desta sessão |
| `WAWebHistorySyncProgressGetters.getInProgress(model)` | **`false` em t+0**, e `getProgress` devolve `100` a partir de t+1 |

E enquanto os dois diziam "acabou", a coleção de mensagens ia de **133 para
1170 em oito segundos**.

> Os getters rastreiam a sincronização de histórico do SERVIDOR para a conta, que
> de fato já terminou há muito. Não rastreiam a **hidratação local desta
> sessão**, que é o que inunda o barramento. O nome é o mesmo e a coisa é outra.

Registro também o meu erro de instrumento: a primeira leitura chamou os getters
sem argumento e recebeu três `threw`. Getter que lança não é sinal ausente — é
sinal perguntado errado. Com o modelo (`getHistorySyncProgressModel()`) eles
respondem, e a resposta é que derruba a opção (i).

### Então: três estados, como mandado

`REPLAY | LIVE | UNKNOWN`, com a regra que veio junto e é absoluta:
**UNKNOWN nunca vira LIVE por tempo nem por taxa.**

O único caminho para LIVE é evidência **causal sobre o próprio evento**. Para
`message.added` isso existe: o carimbo da mensagem. Uma mensagem criada há dois
dias e anunciada agora é hidratação, por mais devagar que os eventos cheguem —
que é exatamente o que uma heurística de taxa erraria.

Para `chat.changed` e os demais não existe discriminador, e eles dizem isso.

**`Replay` continua existindo e ficou CONSERVADOR**: é `true` para tudo que não
foi PROVADO vivo. Todo consumidor escrito antes disto continua funcionando, e
funcionando do lado que não machuca — pular algo que era novo custa um evento
perdido; contar mil e cem mensagens antigas como chegadas corrompe um total.

### Prova ao vivo, contra a explosão de verdade

```
hidratação: {replay:714, unknown:2864}  total 3578   LIVE: 0
            um consumidor antigo contou 0 como novidade

depois do envio A->B: {live:1, replay:764, unknown:3389}
            um consumidor antigo contou 1
```

As quatro propriedades exigidas: a explosão não contém nenhum live; uma ação
depois dela produz exatamente um; a recarga reinicia a janela (unitário); e
nenhum consumidor antigo recebe histórico como novo.

### Um defeito meu, e ele é instrutivo

O primeiro classificador guardava em `AgeSeconds > 0`, usando o zero como "não
tem carimbo". **Zero é a idade mais fresca que existe** — um envio e o eco dele
caem no mesmo segundo rotineiramente —, então a mensagem mais nova possível era
classificada `UNKNOWN`. Separado num campo `Aged`, e travado por teste.

E o dublê da suíte não emitia `msgT`, então dois testes existentes falharam
acusando o classificador de um defeito que ele não tinha. **Dublê menos fiel que
a produção** — o espelho da armadilha do dublê permissivo, segunda vez nesta
sessão.

**Controles negativos executados**:

| mutação | teste | saída |
|---|---|---|
| `UNKNOWN` virando `LIVE` | `TestATypeWithNoDiscriminatorStaysUnknown` | `a chat.changed after the first drain is live` |
| voltar ao guarda `AgeSeconds > 0` | `TestAZeroSecondOldMessageIsLive` | `is unknown, want live` |
| remover a janela do primeiro drain | `TestTheFirstBatchAfterAnInstallIsReplay` | `the first batch was not marked replay: []` |

### E mais uma F100, em forma nova

O gate ficou vermelho em `TestStartSession_NotReadyFailure_PreservesFinalSnapshot`,
falhando em `launch`, `open_tab` e `navigate` em três execuções seguidas — nunca
chegando ao laço de settle que ele existe para testar.

O teste dava **três segundos para o boot inteiro**, com o comentário dizendo que
um ctx curto "mantém o teste rápido". Isso assumia em silêncio uma máquina onde
subir o Chrome e abrir uma aba cabem em três segundos. Sob `-race`, ao lado de
todos os outros pacotes, não cabem.

A propriedade — *o contexto de quem chama é soberano sobre o orçamento de
settle* — foi preservada mudando a EXPRESSÃO dela: o orçamento de settle vai a
minutos e o ctx a uma fração disso. Uma falha em `StageNotReady` só pode
significar que o contexto cortou o settle. Metade generosa é a que se pode
crescer com segurança.

É a mesma família da F100 sob outra roupa: um prazo curto fixo que codifica a
velocidade de uma máquina.

**Status**: entregue.

---

## H98 — POLLS/VOTES: a leitura entregue, e uma capacidade já entregue que não envia

**Data**: 2026-08-21.
**Contexto**: família polls/votes, na ordem da orquestração, depois do
EVENT-REPLAY-FIX.

### A leitura de votos: entregue e provada

`poll.Votes` lê contra uma enquete **real** e devolve `options=map[0:0 1:0]` —
as duas opções presentes, ambas com zero. Todo teste posterior de tally depende
dessa linha de base.

**Onde a referência não pode ser seguida.** O `whatsapp-web.js` monta a chave com
`MsgKey.fromString(msg.id._serialized)`. Neste build isso lança
`MsgKey.fromString error: str is null or not a string` — o mesmo fato de
identificador nulo que moldou o `capabilities/messagemeta`.

Mas **`m.id` JÁ É a chave**, e o `toString` dela dá a forma de 47 caracteres que
a tabela indexa. A conversão de que a referência precisa é uma que este módulo
pula. Mesma forma da H34: use o que a página já tem em vez de reconstruir a
partir de uma string que não existe.

### O achado que vale mais: a enquete não sai daqui

O round trip é a primeira vez que alguém olhou o **outro lado**. A H69 provou o
envio pela mensagem APARECER nesta sessão — que é uma afirmação mais fraca do
que parece.

```
poll on conta-A: {"found":true,"ack":0,"kind":"poll_creation","options":2}   t+00s
poll on conta-A: {"found":true,"ack":0,...}                                  t+18s

conta-B: {"loaded":1272,"polls":3,"prefixes":["3EB064F7","3AB2ADBD","3AF73F6D"]}
```

Vinte segundos em **ack 0**, que é PENDENTE. Qualquer mensagem que tenha saído
tem pelo menos 1. E o par, com 1272 mensagens carregadas e 3 enquetes antigas,
nunca recebe esta.

É exatamente o sucesso silencioso que a invariante 14 proíbe, e sobreviveu
porque **ninguém perguntou ao destinatário**.

**Suspeito principal, já registrado pela H69 e nunca medido**: o
`pollTypeFrom=omitted` no resultado. A H69 anotou que o campo `pollType` nunca
foi medido e é omitido; uma enquete sem o tipo pode ser aceita localmente e
recusada no servidor sem erro.

**Corrigido no que dá para corrigir agora**: `PollTo` ganhou pós-condição de
`ack`, com erro próprio (`ErrPollNeverLeft`). "Criada" e "enviada" eram o mesmo
fato e agora são dois. A falha passa a ser **alta** em vez de silenciosa.

### Três descobertas de percurso, todas por medição

1. **`PollTo` envia para um chat JÁ CARREGADO.** Passar o jid do par responde
   "no such chat is loaded", e varrer a coleção pela parte de usuário do telefone
   também não acha: este build arquiva sob LID, e as duas partes de usuário são
   números diferentes. O caminho é mandar uma linha antes — `send.Text` resolve
   e devolve o chat onde a mensagem caiu.
2. **`MsgCollection` guarda o que a sessão OLHOU**, não o que a conta tem. Uma
   sessão que subiu e nunca abriu a conversa não tem as mensagens dela.
3. Por isso **carregar é responsabilidade de quem chama**, e `poll.ErrNotFound`
   diz isso. A alternativa era um scan que carrega todo chat até a mensagem
   aparecer — na conta de laboratório, novecentos carregamentos por consulta, e
   uma heurística vestida de conveniência.

### Dois dublês menos fiéis que a produção, no mesmo dia

O dublê de enquete não respondia à leitura de `ack`, então toda prova existente
teria começado a falhar por um motivo alheio ao que elas afirmam. Estendido com
`ackStuck`, cujo **valor zero é o saudável** — de propósito, porque todo teste
escrito antes da pós-condição constrói esse dublê sem pensar em ack.

Terceira ocorrência da armadilha nesta sessão. As três se resolvem pela mesma
pergunta: *o dublê responde tudo que a produção responde?*

**Controles negativos executados**:

| mutação | teste | saída |
|---|---|---|
| aceitar voto parcialmente casado | `TestAPartiallyMatchedVoteIsRefused` | `err = <nil>, want ErrUnknownOption` |
| ler pela chave reconstruída da referência | `TestTheVoteReadUsesTheMessagesOwnKey` | `the read rebuilds the key from a string; that throws on this build` |
| tirar o preenchimento com zero das opções sem voto | `TestUnvotedOptionsArePresentWithZero` | `the page script does not pre-fill every option with zero` |
| aceitar `ack >= 0` como saída | `TestAPollThatNeverLeavesIsAnError` | `err = <nil>, want ErrPollNeverLeft` |

> O terceiro controle **não mordeu na primeira vez**, e isso é o achado: a
> propriedade vive no script da página e o dublê a substituía. O teste afirmava
> sobre a PARSAGEM e não sobre a PRODUÇÃO. Agora afirma sobre o script, onde ela
> mora.

### E mais um teste que media relógio de parede

`TestStartSession_ImmediateReadyPage_DoesNotPayTheSettleBudget` afirmava
`elapsed < 10s` para provar que o laço de settle retornou na primeira olhada.
Sob `-race`, ao lado de todos os outros pacotes, subir o Chrome e abrir a aba
sozinhos levaram **10,07 s** — e o teste falhou sem medir nada sobre settle.

O raciocínio do comentário estava certo ("dez é generoso perto dos 60 s do
orçamento") e a EXPRESSÃO estava errada: dez segundos também tinham de cobrir o
boot inteiro. Agora o limite é uma fração do próprio orçamento de settle, que é
o que a propriedade diz. Terceira vez nesta sessão que a mesma correção é
aplicada em outra forma.

**Status**: parcialmente entregue — leitura de votos provada ao vivo; voto
implementado e travado por teste, sem prova ao vivo porque a enquete não chega
ao par; e o envio de enquete rebaixado com a medição que o rebaixa.

---

## H99 — "enviada" passa a significar "saiu", em todo o pacote send

**Data**: 2026-08-21.
**Contexto**: consequência direta da H98. A enquete era reportada como enviada
por APARECER nesta sessão, e o mesmo método verifica texto, mídia, figurinha,
documento e resposta.

### A pergunta que a H98 obriga

Quantas outras linhas "enviável" foram provadas só pela aparição local?

`send.verify` procura uma mensagem de saída do tipo certo na `MsgCollection`,
mais recente que o instante do envio. **Nunca olhou o `ack`.** O texto de fato
chega — todo teste entre contas desta suíte prova isso pelo destinatário — mas a
VERIFICAÇÃO não distinguia as duas coisas. Ela acertava por sorte.

### Medi antes de apertar

| fato | tempo |
|---|---|
| verificação local (aparição) | **2 ms** |
| `ack >= 1` — chegou ao servidor | **508 ms** |
| `ack >= 2` — chegou ao aparelho | **1,01 s** |

Meio segundo é o preço de provar que a mensagem saiu, contra um orçamento de
verificação de vários segundos. Barato o bastante para não ser decisão de
produto disfarçada de detalhe.

`send.Result` ganhou `Ack`, e `ErrNeverLeft` é erro próprio — separado de
`ErrUnverified`, porque os consertos diferem: **nada aparecer** é um despacho que
não aconteceu; **aparecer em ack 0** é um despacho que o aplicativo aceitou e o
socket não carregou. O segundo é o que a enquete faz aqui, toda vez.

Confirmado ao vivo: `send.Result(... ack=2 waited=2ms)`.

### O dublê, pela quarta vez, e agora com nome

Os cinco dublês do pacote respondiam o mesmo JSON a qualquer script, então a
leitura de ack recebia o payload de verificação e o envio falhava numa forma de
JSON.

A primeira correção casou por `"m.id && m.id.id ==="` — **que o script de
RESPOSTA também contém**. O dublê passou a engolir o despacho e devolver ack, e
uma asserção sobre o despacho falhou por um motivo alheio ao que ela afirma.

Conserto: o script carrega o **próprio nome** (`ackReadMarker`). *Um script que
precisa ser reconhecido deve dizer como se chama.* É a mesma família das oito
vezes em que um guarda casou com a própria prosa: casar por conteúdo acidental
em vez de por identidade.

Em todos eles o valor ZERO é o saudável — `ackStuck` — para que todo teste
escrito antes da pós-condição continue afirmando o que queria afirmar.

### Um instrumento a mais, num teste que falhou e depois passou

`TestRealSPASendsMediaBetweenAccounts` falhou uma vez com "500 replayed, 0
unrelated fresh inbound" e passou na repetição. Não consertei por palpite:
acrescentei o que faltava para a PRÓXIMA falha ser informativa — a contagem de
eventos **descartados pela página**.

Buffer limitado mais sessão recém-iniciada é exatamente a forma que a H93 mediu
(~1200 mensagens de hidratação nos primeiros segundos), e uma assinatura que
transborda joga fora chegadas reais junto com o histórico. "Nunca chegou" e
"chegou e foi descartada" são falhas diferentes, e o teste as reportava
igualmente.

### E o gate ficou vermelho por causa da MÁQUINA, não do código

Dois boots falharam nos 150 s inteiros do teto do harness. A tentação era subir
o teto de novo. Medi antes: **load average 11**, subindo para 23 — a máquina do
usuário, com Chrome, Slack e as próprias execuções ao vivo. Só dois browsers
órfãos de execuções minhas interrompidas.

Matei os dois órfãos, repeti, e ficou verde **sem tocar em nenhum teto**. Subir
um limite porque a máquina está ocupada é como se apaga a diferença entre um
teto e a ausência de um.

**Controles negativos executados**:

| mutação | teste | saída |
|---|---|---|
| aceitar `ack >= 0` como saída | `TestAMessageThatNeverLeavesIsAnError` | `err = <nil>, want ErrNeverLeft` |
| idem no envio de enquete | `TestAPollThatNeverLeavesIsAnError` | `err = <nil>, want ErrPollNeverLeft` |

**Status**: entregue.

---

## H100 — "broadcast" não é lista de transmissão, e o nosso ledger repetia o engano

**Data**: 2026-08-21.
**Contexto**: família broadcast, na ordem da orquestração.

### O primeiro achado é o nome

O nosso ledger descrevia as três linhas como **"família de listas de
transmissão"**. Ler a referência antes de projetar — que é a regra — mostrou
outra coisa:

```
Client.getBroadcasts  ->  window.WWebJS.getAllStatuses()
                      ->  WAWebCollections.Status.getModelsArray()
```

E a estrutura `Broadcast` carrega `msgs`, `totalCount` e `unreadCount`,
**indexada por um contato**. Isso é o **STATUS** — as stories de 24 horas.

Três linhas iam ser implementadas contra a ideia errada. O custo de descobrir
foi um `grep` na referência; o custo de não descobrir teria sido uma capacidade
inteira com o nome de outra coisa.

### O que foi entregue, e o que a conta não deixa provar

`capabilities/status` lê feeds pela coleção certa, através dos **getters da
própria página** — `getTotalCount`, `getUnreadCount`, `getReadCount`, `getT` —
e não pelos campos `__x_`. Isso é a lição da H51 aplicada antes de custar:
alcançar o campo por baixo do getter foi o que fez um leitor responder para 1
registro em 384.

Ao vivo: **zero feeds**, e o par sem feed reportado como `ErrNotFound` em vez de
um feed de zeros — que um chamador renderizaria como *"essa pessoa não postou
nada"*, uma afirmação que ninguém mediu.

### O que NÃO foi ligado, e por que isso não é escopo

Este build exporta `WAWebSendStatusMsgAction` com `sendStatusTextMsgAction` e
`sendStatusMediaMsgAction`. **Ele sabe postar status**, coisa que o upstream nem
expõe.

Não está ligado, e a razão não é falta de tempo:

> Um status é visível a **toda a agenda**. O roster desta conta foi medido em
> **944 contatos** (H39). Todo efeito externo que esta suíte já produziu caiu
> sobre um par conhecido ou sobre um grupo de laboratório. Novecentas pessoas
> que nunca concordaram em participar de um teste é outra categoria de ato.

Isso é **raio de alcance**, não decisão de escopo, e pertence a um humano. Há um
teste que trava a decisão: `TestThisPackageDoesNotPost` falha se qualquer script
deste pacote mencionar as funções de envio.

**Controles negativos executados**:

| mutação | teste | saída |
|---|---|---|
| feed ausente devolvido como feed vazio | `TestAnAbsentFeedIsItsOwnError` | pânico de ponteiro nulo — o feed simplesmente não existe para ser devolvido |
| ler `__x_totalCount` em vez do getter | `TestTheReadGoesThroughTheGetters` | `the read does not use G.getTotalCount` e `the read reaches into the model's raw storage` |
| falha de página virando lista vazia | `TestAPageFailureIsNotAnEmptyList` | `err = <nil>` |

**Status**: parcialmente entregue — leitura provada ao vivo contra uma conta sem
status; o não-vazio depende de um ato cujo alcance é a agenda inteira.

---

## H101 — o `pollType` era mesmo o suspeito nomeado, e não era a causa

**Data**: 2026-08-21.
**Contexto**: investigação LIMITADA autorizada pela orquestração — *"uma medição
da chamada real da SPA/UI para descobrir o valor exato, aplicar e exigir ack>=1;
se isso não resolver, registre e não abra uma sequência de hipóteses cegas."*

### A medição encontrou o que a H69 disse que faltava

A H69 fechou com: *"O que falta: o que `createPollCreationMsgData` desestrutura.
Exige a cabeça do corpo do gerador, que o grep do bundle truncou."*

O grep truncava porque a janela era de **260 caracteres fixos**, e a resposta
estava na CABEÇA de uma função minificada cujo corpo tem mais de mil. Parametrizei
a janela e a função apareceu inteira:

```js
createPollCreationMsgData({chat, isWamoSub, poll, quotedMsg})
  s = yield d(poll)            // um validador/normalizador
  y = s.pollType               // <- daqui sai o pollType
```

E o enum estava em **`WAWebPollCreationUtils`** — **singular**. Todo o resto da
família é `WAWebPolls*`. **Uma letra** é a razão de três buscas o terem perdido,
inclusive a da H69.

```
PollType         { POLL, QUIZ }
PollContentType  { TEXT, IMAGE }
```

### Aplicado, lido da própria página — e não resolveu

O objeto agora carrega `type: PollType.POLL` e `contentType: PollContentType.TEXT`,
lidos do módulo em vez de escritos aqui, para que um build que os renomeie falhe
alto em vez de mandar uma string sem sentido.

**O ack continua 0.** O `pollType` era o suspeito nomeado e **não é a causa**.

### Por que isto para aqui

A instrução foi explícita, e ela está certa: uma hipótese nomeada merece uma
medição; a próxima seria a primeira de uma sequência cega. O que ficou é melhor
do que era — a omissão conhecida sumiu, o `typeSource` reporta o módulo real em
vez de `omitted`, e a próxima pessoa não gasta a busca que eu gastei.

**O que ainda não foi olhado**, escrito para quem continuar: `d` é um
**validador**, e `WAWebPollsProtoUtils` exporta `validatePollCreationMessage` e
`validatePhotoPollCreationMessage`. Se a validação recusar em silêncio, a razão
está lá. Não é hipótese cega — é o único ponto do caminho que ainda não foi lido.

**Ganho de instrumento**: `WA_PROBE_MODMAP_WINDOW` torna a janela do grep
ajustável. A H69 registrou "o grep truncou" e parou; a mesma limitação teria
truncado de novo hoje.

**Status**: não entregue quanto ao envio — suspeito nomeado perseguido, medido e
DESCARTADO, com o próximo ponto de leitura nomeado.

---

## H102 — a primeira wave por substrato ORCA, e o que ela mediu em canais e comércio

**Data**: 2026-08-21.
**Contexto**: o humano mandou inicializar o protocolo ORCA e paralelizar por
`orca` CLI, não por subagente nativo do provider. Duas sondas read-only,
write sets disjuntos, dois workers `claude-sonnet-5` effort `medium`.

### O substrato mordeu na primeira tentativa

`worker-start` injetou o packet e **não submeteu**. Os dois workers ficaram com o
prompt parado no composer, cursor congelado em 22 por noventa segundos, zero
arquivos — o silêncio idêntico ao de quem está trabalhando.

Recuperado com `orca terminal send --enter`, e os dois começaram no mesmo
segundo. É a falha que o protocolo já catalogou duas vezes (`GC-006`, `GC-012`) e
que o próprio humano lembrou antes de eu tropeçar nela: **`--enter` é opt-in, e
sem ele o comando é malformado**, não um lapso.

### Validar não é ler o relato — e isso pegou um exagero

Rodei as duas sondas eu mesmo. O worker de comércio afirmou, no `worker_done`,
que existem *"coleções de primeira classe `WAWebCatalogCollection`,
`WAWebProductCollection`, `WAWebOrderCollection` que o `whatsapp-web.js` não tem
equivalente"*.

A sonda dele tenta **dez** nomes de coleção. O dicionário de módulos volta com
**quatro** entradas, e nenhuma é essas. As coleções existem — dentro de
`WAWebCollections` (`Catalog`, `Order`, ambos confirmados `true`) — mas o caminho
de módulo afirmado não foi medido por nada.

> Ausência de erro na execução de um worker não é prova de que o relato dele
> confere. O relato foi corrigido no commit; nunca entrou em documento de estado.

### CANAIS: a superfície está toda aqui

Vinte e quatro módulos presentes, aridade batendo com a referência. **Um** falta:
`WAWebMexFetchNewsletterSubscribersJob` resolve `falsy`.

Dois fatos que mudam o que a família significa:

| fato | valor |
|---|---|
| `isNewsletterCreationEnabled` | **true** |
| `getMaxSubscriberNumber` | **5000** |
| superfície de envio | `sendNewsletterTextMsg`, `MediaMsg`, `PollCreationMsg`, `AlbumMsg`, `EditMsg` |
| `modelCount` | **0** — a conta não segue canal nenhum |

Este build **cria e publica** em canal. A forma do modelo ficou sem resposta pelo
zero: não há canal para inspecionar.

### COMÉRCIO: a pista era o instrumento, de novo

`sellerWidPresent` vinha `false` por `WAWebConnModel.Conn.wid`, e o worker marcou
a própria sonda como suspeita — corretamente. Perguntei em mais lugares:

```
connWid    false        connKeys   []          <- o campo NAO EXISTE ali
mePn       "c.us"       meLid      "lid"       <- a identidade esta em WAWebUserPrefsMeUser
bizProfiles 58          catalogCount 0
collectionsCatalog true  collectionsOrder true
```

`connKeys` **vazio** é a prova: não era campo ausente, era módulo errado. Todo o
resto deste repositório já lê identidade por `WAWebUserPrefsMeUser`.

### E com a identidade certa, a chamada finalmente foi feita

`queryCatalog` nunca tinha sido invocado porque a sonda acreditava que a conta
não tinha wid — crença produzida por ler o módulo errado. Com o wid real, duas
formas de argumento, **duas respostas diferentes**:

| forma | resposta |
|---|---|
| posicional `(meWid)` | `ServerStatusCodeError` |
| objeto de opções `({catalogWid, limit})` | `CatalogUnknownError` |

Nenhuma das duas é silêncio, e a diferença é informação:
`ServerStatusCodeError` diz que a chamada **chegou ao servidor** e ele recusou
com um código; `CatalogUnknownError` é erro de domínio, de um argumento
mal-formado o bastante para o catálogo não ser identificado.

A leitura mais provável é que **a forma posicional está certa** e a recusa é
"esta conta não tem catálogo" — coerente com `catalogCount: 0`. A aridade
declarada é **10**, não 9 como o relato dizia.

**O que falta para a família de comércio**: uma conta COM catálogo. Não é código.

### E o `ESTADO.md` saiu da triagem

Ficou três ciclos parado dizendo 263 testes e 12 pacotes contra **843 e 35**
reais — cometendo o defeito que ele próprio documenta no H29. Atualizado, com o
envelhecimento registrado em vez de apagado: um retrato que envelheceu é
evidência de como ele envelhece.

### 41=a tentado: o diretório de canais TRAVA, e nem o próprio relógio o solta

A orquestração autorizou **seguir** um canal público (barato, reversível) em vez
de criar um. Para seguir é preciso achar um, e inventar um link seria adivinhar o
canal de outra pessoa — então o caminho certo era o discovery do próprio app.

Ele existe, inteiro: `WAWebNewsletterDirectorySearchJob` exporta
`getRecommendedNewsletters`, `getSimilarNewsletters`, `getNewsletterDirectoryList`,
`getNewsletterDirectorySearchResults`, `getNewsletterDirectoryCategoriesPreview`.

**E `getRecommendedNewsletters` não responde.** Quatro execuções, com e sem
argumento, e com três formas de objeto de opções. O parque incremental diz qual
chamada travou — é sempre a primeira.

O detalhe que faz disto mais que "demorou": a chamada está dentro de um
`Promise.race` com rejeição em 8 s, **e o timeout não dispara**. Um `await` que
não volta nem quando o relógio corre ao lado dele não é lentidão — é a mesma
classe do travamento de `createCallLink` (H92) e do convite (H57).

> **Um defeito meu no caminho, e ele é o de sempre.** As três primeiras execuções
> reportaram `PENDING` porque eu inseri o bloco DEPOIS de `out.finished = true`,
> e o laço do Go sai justamente nessa bandeira. **A sonda não estava travando; o
> leitor estava indo embora cedo.** Só depois de mover a bandeira para o fim é
> que os 30 s inteiros foram gastos e o travamento virou fato medido em vez de
> artefato do instrumento.

**O que 41=a precisa e ninguém aqui produz**: um link de canal público real. O
`queryNewsletterMetadataByInviteCode` existe e tem aridade 2 — com um código, a
inscrição é um passo. Sem ele, e com o diretório travado, a forma do modelo de
canal fica sem medição.

**Status**: entregue — duas sondas medidas ao vivo por mim, um relato de worker
corrigido, a pista do comércio resolvida até "falta uma conta com catálogo", e a
forma do modelo de canal bloqueada por um travamento medido no diretório, não por
falta de código.

---

## H103 — o catálogo lido sem criar nada, e o vendedor que respondeu

**Data**: 2026-08-21.
**Contexto**: o humano perguntou o que precisava fazer para destravar. Eu ia
pedir que criasse um produto no catálogo da conta — e retirei o pedido **antes de
fazê-lo**, ao perceber que existia caminho sem criar nada.

### A pergunta que dissolveu o pedido

`queryCatalog` existe para um **cliente** abrir a vitrine de um **vendedor**.
Pedir ao humano para criar um produto resolveria — e deixaria um item real,
visível a quem abrisse aquele perfil comercial. A conta já conhece **58 perfis de
negócio**; ler a vitrine de um deles exercita exatamente o mesmo caminho e não
cria coisa alguma.

### O que respondeu

| vendedor | resposta |
|---|---|
| 1 | `ServerStatusCodeError` |
| 2 | `ServerStatusCodeError` |
| 3 | **1 produto**, com 19 campos |

**O terceiro é o que torna os dois primeiros legíveis.** Sem ele,
`ServerStatusCodeError` seria indistinguível de uma capacidade quebrada — a
armadilha do leitor que só foi visto devolvendo zero, catalogada na H93. Com ele,
a recusa é uma resposta *sobre aquele vendedor*, e `ErrNoCatalog` é um estado, não
um erro.

### Duas correções à referência

**A forma não é a que ela sugere.** A resposta real é
`{data, catalog_id, catalog_name, catalog_type, paging}` — os itens em `data`, e
`products` era palpite herdado. Um leitor que olhasse `products` devolveria zero
para sempre: terceira ocorrência dessa classe neste repositório.

**A chamada é POSICIONAL.** A aridade declarada é 10 e a referência passa 2. O
objeto de opções devolve `CatalogUnknownError`; a forma posicional **chega ao
servidor**. A diferença entre os dois erros é o que decidiu a assinatura.

### O que ficou vazio, e por que não virou asserção

No vendedor observado, `catalog_id`, `catalog_name`, `catalog_type` e o preço
vieram **vazios**. A primeira versão do teste reprovou em *"o catálogo não tem id
próprio"* — e uma booleana não distingue leitor olhando no lugar errado de loja
que deixou o campo em branco.

Uma observação não separa "sempre vazio" de "esta loja". Então os campos são
**reportados** e a pós-condição exige só o que um vendedor sustenta: produto com
id, e preço nunca sem moeda. Um número de preço sem moeda é um número sobre o
qual ninguém pode agir.

### Preço não é convertido, e há teste que morde

O preço cruza como o inteiro da página, na menor unidade da moeda. Dividir aqui
pelo expoente errado é o defeito que ninguém percebe até alguém ser cobrado —
`TestThePriceIsNotConverted` falha se aparecer `/ 100`, `* 0.01` ou `toFixed(`.

E a paginação é **reportada, não seguida**: seguir em silêncio transforma uma
leitura em um número indeterminado delas.

**Controles negativos executados**:

| mutação | teste | saída |
|---|---|---|
| ler itens de `products` em vez de `data` | `TestTheItemsAreReadFromTheMeasuredField` | `the script does not read the items from data` |
| tratar vendedor sem vitrine como falha | `TestASellerWithNoShopIsItsOwnAnswer` | `err = <nil>, want ErrNoCatalog` |
| dividir o preço por 100 | `TestThePriceIsNotConverted` | `the script converts the price with "/ 100"` |

### E um erro meu que já é o segundo do dia

Comentário com crase dentro de string crua de Go quebrou o build. Mesma coisa
tinha acontecido horas antes em `presence.go`. O comentário agora diz isso no
próprio lugar onde a tentação existe.

**Status**: entregue — leitura de catálogo provada ao vivo contra vendedor real,
sem criar nada, com a forma e a assinatura corrigidas contra a referência.

---

## H104 — o canal lido de fora, e a inscrição que não foi precisa

**Data**: 2026-08-21.
**Contexto**: o humano forneceu um link de canal público, que era a única coisa
que faltava e a única que nem eu nem a orquestração podíamos produzir sem
adivinhar o canal de terceiro.

### O plano era seguir e devolver. Não foi preciso nenhum dos dois

`queryNewsletterMetadataByInviteCode` responde para um canal que esta conta
**não** segue, e carrega a forma inteira. A família destravou **sem tocar na
conta**.

E a prova é interna: o `newsletterMembershipMetadataMixin` volta **`null`**, que é
exatamente o que diz "não sou membro". A leitura prova a si mesma — não preciso
afirmar que não segui, o dado afirma.

Medido contra o canal real:

```
jid @newsletter · nome presente · 45.460 assinantes
state=active · verification=verified · following=false · picture=true
```

### A forma é de MIXINS, e é a quarta vez que isto aparece

Não existe campo `name` no topo. Existe:

```
newsletterNameMetadataMixin.nameElementValue
newsletterSubscribersMetadataMixin.subscribersCount
newsletterStateMetadataMixin.stateType
newsletterVerificationMetadataMixin.verificationState
newsletterCreationTimeMetadataMixin.creationTimeValue
newsletterInviteLinkMetadataMixin.inviteCode
newsletterDescriptionMetadataMixin.descriptionQueryDescriptionResponseMixin  <- mais um nivel
newsletterMembershipMetadataMixin  -> null quando nao se e membro
```

A primeira leitura reportou `hasName: false` e `subscribersType: undefined`, e a
tentação era concluir "o campo não vem". Vinha — uma camada abaixo. **Ler o campo
óbvio devolveria `undefined` para sempre**, que é a mesma classe do
`chatstate.type` (H94), do `products` em vez de `data` (H103) e do agregado de
reações (H83). Quarta ocorrência.

### O link inteiro é aceito, e isso não é conveniência

Quem chama segura um **link**, não um código. Obrigá-lo a cortar é obrigá-lo a
saber uma coisa que este pacote já sabe, e o link colado inteiro é a forma que de
fato chega a um programa.

### O que continua faltando, e agora está nomeado com precisão

`getChannels` lista os canais **seguidos**, e a conta não segue nenhum. Não é
falta de código: falta uma inscrição. E `getRecommendedNewsletters` continua sem
responder **nem ao próprio timeout de 8 s** — não é mais necessário para LER, mas
continua sendo o que falta para DESCOBRIR um canal sem link.

**Controles negativos executados**:

| mutação | teste | saída |
|---|---|---|
| ler `r.name` em vez do mixin | `TestTheReadGoesThroughTheMixins` | `the script does not read newsletterNameMetadataMixin` |
| fixar `member: false` | `TestFollowingComesFromTheMembershipMixin` | `membership is hardcoded` |
| tratar código desconhecido como falha de leitura | `TestAnUnknownCodeIsItsOwnError` | `err = …, want ErrNotFound` |

**Status**: entregue — `getChannelByInviteCode` provado contra canal real, sem
seguir nada e sem deixar rastro na conta.

---

## H105 — os quatro fatos que um grupo conta sobre si, e a quinta vez do mesmo esconderijo

**Data**: 2026-08-21.
**Contexto**: bloco escolhido por ser o mais coeso sem insumo externo —
`owner`, `createdAt`, `description` e `participants` são PROPRIEDADES na
referência, populadas de uma metadata que este módulo já busca por outros
motivos.

### Medido antes de projetar, e a medição pagou

| campo | onde está |
|---|---|
| `md.owner` | objeto, com `_serialized` |
| `md.creation` | número, em segundos |
| `md.participants` | coleção com `getModelsArray` |
| participante | `id` (**LID**), `isAdmin`, `isSuperAdmin`, `joinTime` |
| `md.desc` | **`undefined`** |
| `md.displayedDesc` | **`undefined`** — mas `__x_displayedDesc` existe no armazenamento cru |
| `md.descTime` | número |

### O campo que não dá para decidir com um grupo só

Os dois candidatos de descrição leem `undefined`, e `descTime` é um número. Isso
é compatível com **duas** coisas: o grupo não tem descrição, ou o leitor olha o
campo errado. **Um grupo não separa as duas.**

A saída fácil seria escolher um campo e seguir. Seria a quinta ocorrência da
mesma classe — `chatstate.type` (H94), `products` em vez de `data` (H103),
mixins de canal (H104), agregado de reações (H83) — em que ler o campo óbvio
devolve vazio para sempre e ninguém percebe.

Então os dois são consultados e o **vencedor é nomeado**: `DescriptionSource`
viaja junto do texto. `none` diz *"nada veio"*, não *"nada existe"*. Não é
fallback que engole — é a dúvida entregue a quem chama, como o `PollTypeSource`
e o `Fields` do `groupreq` já fazem.

### A pós-condição que separa flags coladas

Ao vivo: 2 participantes, 1 admin, e esse mesmo é super admin. O teste exige
**exatamente um** super admin e que ele seja admin também — um criador que não
fosse admin significaria que as duas flags saem do mesmo lugar, que é como um
leitor de duas coisas vira um leitor de uma.

**Controles negativos executados**:

| mutação | teste | saída |
|---|---|---|
| ler só `md.desc`, sem reportar fonte | `TestTheDescriptionCarriesItsSource` | `the script does not consult/report md.displayedDesc` |
| refrescar DEPOIS de ler | `TestMetadataRefreshesFirst` | `the refresh runs AFTER the read; it would answer with boot-time metadata` |

**Status**: entregue — três linhas `PROVEN` e uma `PARTIAL` cuja parcialidade é a
única leitura honesta com um grupo só.

## H106 — a origem de uma mensagem: o remetente não é o chat, e o grupo se pergunta

**Data**: 2026-08-21. **Contexto**: fechar `Message.getChat` / `Message.getContact`
do LEDGER-WWEBJS (família `Message`, 13 `MISSING`).

**Onde**: `internal/wa-headless/capabilities/message/` (novo: `message.go`,
`script.go`, `message_test.go`), `internal/wa-headless/probe_msgorigin_test.go`.

### O que foi medido ANTES de projetar

`getMentions` era o vizinho óbvio e foi medido primeiro
(`probe_mentions_test.go`, contra a conta pareada):

```json
{"scanned": 395, "withMentions": 0, "withGroupMentions": 0,
 "candidates": {"mentionedIds":0,"mentionedJids":0,"groupMentions":0,
                "mentions":0,"quotedParticipant":0}}
```

**395 mensagens carregadas, ZERO com menção sob nenhum de cinco nomes de campo
candidatos.** Embarcar um leitor que nunca foi visto devolvendo algo é
exatamente a armadilha catalogada na H93, então ele NÃO foi embarcado: a medição
ficou escrita e as duas linhas seguem `MISSING` com a nota `medido`. A medição é
o entregável aqui — sem ela, "não atacado" e "não existe neste build" são
indistinguíveis.

Pivotei dentro da mesma família para `getChat`/`getContact`, que são
observáveis agora.

### O problema que o tipo existe para resolver

`Origin` tem `ChatJID` e `SenderJID` separados porque **numa conversa de grupo
eles são diferentes**: o participante é quem falou, o chat é o grupo. Fundi-los
atribuiria a fala ao grupo. Em um-para-um coincidem legitimamente, e isso é
REGISTRADO em `SenderIsChat` em vez de o chamador ter de comparar.

**Grupo é PERGUNTADO à página** (`WAWebChatGetters.getIsGroup`), não inferido do
sufixo `@g.us`. Este build já trocou de espaço de identidade uma vez (LID), e
inferir do sufixo fica errado à distância de uma mudança de build.

### Prova ao vivo (2026-08-21, conta-A)

```
loaded: group=2 direct=393
group messages read=2 senderDiffersFromChat=2
absent id -> message: this session has not loaded that message
```

2 de 2 mensagens de grupo com remetente ≠ chat. A sonda **falha** se todas
lerem com remetente = chat, porque nesse caso o leitor seria um leitor de chat
com dois nomes.

### Controles negativos EXECUTADOS

1. `isGroup = chat.indexOf("@g.us") >= 0` (inferir do sufixo em vez de
   perguntar):
   ```
   --- FAIL: TestTheGroupFlagIsAskedOfThePage
       message_test.go:113: the script does not ask the page whether the chat is a group
   ```

2. Trocar toda a derivação do remetente por `sender = chat`:
   **NÃO MORDEU na primeira tentativa.** O teste `TestTheGroupSenderIsNotTheChat`
   afirmava sobre a PARSAGEM, e o dublê entrega `sender` pronto — a propriedade
   vive na PRODUÇÃO (o script), que o dublê não exercita. Terceira vez nesta
   sessão que um controle revela essa distância. Corrigido acrescentando ao
   mesmo teste a asserção sobre o script; com ela:
   ```
   --- FAIL: TestTheGroupSenderIsNotTheChat
       message_test.go:99: the script does not read the participant; in a group
       the sender would collapse into the chat
   ```

**Regra que sai daqui**: quando o dublê FORNECE o valor que a asserção examina,
o teste mede a parsagem, não a regra. Ou o dublê deriva como a página deriva, ou
o teste tem de afirmar sobre o script — e afirmar sobre o script é o que se faz
quando a regra mora na página.

### Achado incidental da sonda

O primeiro `kick` da sonda devolveu `encountered an undefined value`: uma IIFE
sem `return` não dá valor ao `Evaluate`. As outras sondas terminam com
`return 'kicked';` e essa não terminava. Não é defeito de produção — é forma do
harness de sonda, corrigida na hora.

**Status**: corrigido/entregue nesta sessão. Linhas `getChat` e `getContact` da
família `Message` passam a `PROVEN`; `getMentions`/`getGroupMentions` seguem
`MISSING` com a medição registrada.

## H107 — `rawData`: a forma pode sair, os valores não

**Data**: 2026-08-21. **Contexto**: `Message.rawData` do LEDGER-WWEBJS, logo
depois da H106.

**Onde**: `internal/wa-headless/capabilities/message/script.go` (`shapeScript`),
`message.go` (`Reader.ShapeOf`), `probe_msgorigin_test.go`.

### O conflito, dito antes de resolvido

`Message.rawData` na referência devolve o objeto cru inteiro. Para uma mensagem
de texto, o objeto cru **é** o corpo. Este módulo mantém corpos fora
estruturalmente — invariante 12, travada em
`capabilities/messagemeta/messagemeta_test.go:TestNoBodyFieldExistsAnywhere`,
que proíbe `body`, `__x_body`, `caption`, `media` e `text` por nome. Embarcar
`rawData` como é não seria entregar funcionalidade: seria derrubar uma guarda.

### A medição que resolveu a discussão

Contra a conta-A, sobre uma mensagem de grupo carregada:

```
shape: 598 field names; sample of the harmless ones: [id t type ack from to]
the page DOES carry a "body" field (name only; no value was read)
the page DOES carry a "caption" field (name only; no value was read)
```

**598 nomes de campo, e `body` e `caption` estão entre eles.** Não era hipótese.

### O que foi entregue

`ShapeOf` devolve os NOMES, ordenados, e nunca um valor. O que `rawData` serve
de fato — ver o que a página tem sobre uma mensagem quando uma capacidade não se
comporta — continua servido; o que ele carregaria de PII, não.

Estado no ledger: `INTENTIONAL_DIFFERENCE`, que é o vocabulário exato para
"resolvemos de outro jeito, de propósito e registrado".

### Controles negativos EXECUTADOS

1. Ler o lado do valor, mesmo só para decidir se o campo existe
   (`seen[k] = (m[k] !== undefined)`):
   ```
   --- FAIL: TestTheShapeReaderNeverReadsAValue
       message_test.go:190: the shape script contains "m[k]", which reads the value side
   ```
   Este é o controle que importa: uma versão que lê `m[k]` **já tem o corpo em
   mãos**, e um `JSON.stringify` depois ele estaria em Go. A guarda é sobre o
   SCRIPT, não sobre o tipo de retorno, porque é no script que a decisão vive.

2. Remover o `.sort()`:
   ```
   --- FAIL: TestTheShapeIsNamesAndIsSorted
       message_test.go:209: the script does not sort, so two reads of one message will not compare
   ```

**Status**: entregue nesta sessão.

## H108 — `reload`: o que muda depois que a mensagem existe

**Data**: 2026-08-21. **Contexto**: `Message.reload` do LEDGER-WWEBJS.

**Onde**: `internal/wa-headless/capabilities/message/` (`currentScript`,
`Reader.CurrentOf`), `internal/wa-headless/probe_msgmutable_test.go` (novo).

### A medição veio antes do desenho

Sobre as 395 mensagens carregadas da conta-A:

```json
{"total": 395,
 "acks": {"0": 25, "1": 19, "2": 205, "3": 141, "absent": 5},
 "starred": {"yes": 0, "no": 395},
 "revokedByFlag": 0, "revokedByType": 0, "revokedBySender": 0,
 "types": {"chat": 378, "poll_creation": 4, "pinned_message": 3,
           "interactive": 2, "notification_template": 2, "audio": 1,
           "call_log": 1, "ciphertext": 1, "gp2": 1, "image": 1, "ptt": 1}}
```

Três coisas que eu não teria adivinhado:

1. **`ack` está AUSENTE em 5 mensagens**, não em zero delas. "Ausente" e "0" são
   respostas diferentes — fundi-las diria "não saiu" sobre uma mensagem sobre a
   qual a página não disse nada. É o mesmo erro que a H90 cometeu com contagem
   de dispositivos e que a frescura de eventos cometeu com `AgeSeconds`. Daí o
   campo `HasAck`.
2. **ZERO mensagens revogadas** por qualquer dos três vocabulários. O ramo
   "revogada" do `reload` não tem como ser provado ao vivo aqui, então este
   pacote **não o implementa**: `capabilities/revoke` já é dono dessa
   decisão, e uma segunda cópia seriam duas listas livres para divergir. O
   `type` sai cru.
3. **25 mensagens em ack 0.** É a H98 outra vez, agora em repouso: mensagens
   criadas que não saíram, sentadas no store.

### Prova ao vivo (2026-08-21, conta-A)

32 mensagens re-lidas, **11 estados distintos**, incluindo os dois casos de
`hasAck=false` (`gp2` x1, `pinned_message` x3) — a distinção não é teórica. A
sonda **falha** se todas re-lerem idênticas, porque aí `CurrentOf` seria uma
constante com nome de função.

### Controles negativos EXECUTADOS

1. Fundir ausente e zero (`hasAck: true, ack: m.ack || 0`):
   ```
   --- FAIL: TestAnAbsentAckIsNotAZeroAck
       message_test.go:262: the script does not test for the ack's presence, so absent and zero collapse before Go ever sees them
   ```
2. Duplicar o vocabulário de revogação aqui:
   ```
   --- FAIL: TestTheRevokedVocabularyIsNotDuplicatedHere
       message_test.go:285: this script names "isRevokedMsg", which capabilities/revoke owns
       message_test.go:285: this script names "revokeSender", which capabilities/revoke owns
   ```

### Achado incidental — NÃO corrigido, vai para triagem

`type: "pinned_message"` aparece **3 vezes** no store desta conta, enquanto a
H81 registra que `pin`/`unpin` são aceitos e não fixam nada. As duas coisas
podem conviver (as três podem ter vindo do par, ou serem mensagens de sistema
sobre fixação alheia), mas isso não foi verificado. É informação que a H81 não
tinha: existe `pinned_message` neste build, e a H81 concluiu apenas que a NOSSA
chamada não produz um.

**Não investigado nesta sessão** — fora do escopo da tarefa. Fica registrado
para decisão sobre reabrir a H81.

**Status**: entregue e provado. O achado do `pinned_message` acima não faz parte
desta entrega e segue para triagem.

## H109 — a família `Chat` era fachada, e o ledger a contava como ausência

**Data**: 2026-08-22. **Contexto**: atacar as 16 linhas `MISSING` da família
`Chat` do LEDGER-WWEBJS.

**Onde**: `internal/wa-headless/LEDGER-WWEBJS.md`, seção `## Chat`.

### O que a leitura da referência mostrou

Busquei `Chat.js` do upstream FIXADO (`f935b500…`, v1.34.7) antes de projetar
qualquer coisa, e quase todo método é uma delegação de UMA LINHA:

```
Chat.js:102   sendMessage  -> this.client.sendMessage(this.id._serialized, …)
Chat.js:110   sendSeen     -> this.client.sendSeen(…)
Chat.js:137   archive      -> this.client.archiveChat(…)
Chat.js:144   unarchive    -> this.client.unarchiveChat(…)
Chat.js:152   pin          -> this.client.pinChat(…)
Chat.js:160   unpin        -> this.client.unpinChat(…)
Chat.js:168   mute         -> this.client.muteChat(…)
Chat.js:182   unmute       -> this.client.unmuteChat(…)
Chat.js:284   getContact   -> this.client.getContactById(…)
Chat.js:292   getLabels    -> this.client.getChatLabels(…)
Chat.js:301   changeLabels -> this.client.addOrRemoveLabels(…)
Chat.js:317   syncHistory  -> this.client.syncHistory(…)
```

Todos esses pares no `Client` **já estavam provados ou parciais** neste ledger.
`archiveChat` é `PROVEN` desde a H55; `muteChat` desde a H62; `getChatLabels` e
`addOrRemoveLabels` desde a H72. As linhas da família `Chat` estavam
`MISSING` — e um leitor do ledger concluiria que não sabemos arquivar uma
conversa, o que é falso.

### O que MUDOU e o que NÃO mudou

**Não foi escrita uma linha de código.** Doze linhas passaram a herdar o estado
do seu par no `Client`, cada uma nomeando o arquivo e a linha do upstream que
justifica a herança, para que ela seja auditável em vez de assumida.

Digo isto em voz alta porque é o tipo de movimento que pode ser
auto-interessado: o placar sobe de 59 para 67 `PROVEN` sem nenhuma prova nova.
A defesa é a auditabilidade — qualquer pessoa pode abrir `Chat.js` na SHA fixada
e verificar cada delegação — e o fato de que o inverso seria pior: um ledger que
conta fachada como ausência mede o tamanho da API da referência, não a nossa
paridade.

**As duas que continuam `MISSING`** são as que não têm par provado:
`addOrEditCustomerNote` e `getCustomerNote`, cujos pares no `Client` também
estão `MISSING`. Herança não inventa estado.

### Diferença real registrada

`Chat.mute`/`unmute` atualizam `isMuted`/`muteExpiration` no objeto em memória
depois da chamada. Não temos objeto `Chat` vivo — quem chama passa o jid — então
não há cache para envelhecer, e toda leitura nossa é fresca. Está escrito no
cabeçalho da família em vez de repetido em dez notas.

**Armadilha evitada**: `Chat.pin` delega para `Client.pinChat`, que é fixar
CONVERSA e está provado. A H81, que falha, é fixar MENSAGEM. Os dois verbos têm
o mesmo nome e resultados opostos; a nota da linha diz isso.

**Status**: decidido e registrado. Sem código novo.

## H110 — `capabilities/settings`: a referência devolve o pedido, nós relemos

**Data**: 2026-08-22. **Contexto**: cinco linhas `MISSING` da família `Client`
(`setAutoDownload{Audio,Documents,Photos,Videos}`, `setBackgroundSync`).

**Onde**: `internal/wa-headless/capabilities/settings/` (novo),
`internal/wa-headless/probe_settings_test.go` (novo).

### Medição de módulo ANTES de projetar

A regra existe porque copiar a lista de módulos do wwebjs já falhou quatro em
quatro (H34). Desta vez a resposta foi outra: **todos existem** neste build —
`WAWebUserPrefsGeneral`, `WAWebUserPrefsNotifications`, `WAWebPhoneUtils`,
`WAPhoneFindCC`, e `window.Debug.VERSION` presente. As nove funções nomeadas
também. A medição negativa e a positiva custam o mesmo; só uma delas é
adivinhável.

E os valores vieram **mistos**: `audio=true, documents=false, photos=true,
videos=false, backgroundSync=false`. Isso não é curiosidade — é o que torna um
leitor por categoria PROVÁVEL. Se a conta tivesse os quatro iguais, um leitor
que ignorasse a categoria passaria em tudo.

### O defeito que a referência tem e nós não

Todo setter do `Client.js` termina em `return flag`: devolve o valor PEDIDO,
sem olhar se a página aceitou. É sucesso silencioso, que a invariante 14
proíbe. Aqui cada escrita relê e falha com `ErrNotTaken` quando a página não
moveu.

A escrita redundante é evitada — a H55 mediu um pedido redundante sendo a CAUSA
de uma falha — mas "não precisou" e "escreveu" são respostas diferentes, e
`Outcome.Changed` as separa. A referência não consegue distingui-las.

### Prova ao vivo (2026-08-22, conta-A)

```
baseline: map[audio:true documents:false photos:true videos:false] sync=false
audio:     true  -> Outcome(now=false changed=true)
documents: false -> Outcome(now=true  changed=true)
photos:    true  -> Outcome(now=false changed=true)
videos:    false -> Outcome(now=true  changed=true)
redundant write:  Outcome(now=false changed=false)
backgroundSync: false -> Outcome(now=true changed=true)
restored: map[audio:true documents:false photos:true videos:false] sync=false
```

As quatro categorias são viradas, não uma — um setter ligado à categoria errada
apareceria como valor que se moveu no lugar errado, e não como teste verde. A
restauração é registrada com `defer` e não com `t.Cleanup` (que roda DEPOIS de
todo `defer`, inclusive o que para a sessão), e ela própria é verificada: uma
restauração não verificada é a falha que envenena a PRÓXIMA execução.

### Controles negativos EXECUTADOS

1. Devolver o pedido em vez de reler (o defeito da referência, copiado de
   propósito):
   ```
   --- FAIL: TestTheWriteScriptReadsTheValueBack
       settings_test.go:158: the script calls getAutoDownloadVideos() fewer than twice: it cannot be comparing before against after, so the postcondition is decorative
   ```
2. Ler sempre o getter de `audio`, ignorando a categoria pedida — **não mordeu
   na primeira tentativa**.
3. Escrever mesmo quando redundante — **também não mordeu na primeira
   tentativa**.

### A regra que sai daqui, agora pela QUARTA vez

Os controles 2 e 3 não morderam pelo mesmo motivo que o da H106: **o dublê
responde do próprio modelo, não do que o script diz.** Sempre que o dublê
FORNECE o valor que a asserção examina, o teste mede a parsagem e a regra vive
na produção. Corrigido com asserção sobre o script nos dois; depois disso:

```
--- FAIL: TestTheFourCategoriesAreReadSeparately
    settings_test.go:143: the read script never calls getAutoDownloadDocuments(): that category is not being read at all
    (idem Photos, Videos)
--- FAIL: TestARedundantWriteIsSkippedAndSaidSo
    settings_test.go:213: the script has no guard against a redundant write; H55 measured a redundant request being the CAUSE of a failure
```

Quatro ocorrências em uma sessão deixaram de ser coincidência. Vale como
armadilha catalogável: **num módulo que dirige a página, a asserção sobre o
resultado só mede o parser. Se a regra mora no script, o teste tem de olhar
para o script.**

**Status**: entregue e provado.

## H111 — `findCC("notaphone")` devolve `"not"`: a página não recusa lixo

**Data**: 2026-08-22. **Contexto**: `getFormattedNumber`, `getCountryCode` e
`getWWebVersion` do LEDGER-WWEBJS (famílias `Client` e `Contact`).

**Onde**: `internal/wa-headless/capabilities/phone/` (novo),
`internal/wa-headless/spa/webversion.go` (novo),
`internal/wa-headless/probe_settings_test.go`.

### A medição que mudou o desenho

Medi o que as funções ANSWER antes de embrulhá-las. Nenhum valor foi registrado
— só forma, porque um número formatado É um número de telefone:

```
entrada real (jid)  -> cc "55",  formatado len 17, com "+", 2 espaços
"notaphone"         -> cc "not", formatado len 11, com "+"
""                  -> cc "",    formatado len 0
"1"                 -> cc "1",   formatado len 3, com "+"
```

**`findCC("notaphone")` devolve `"not"`.** A função entrega os primeiros
caracteres do que recebeu, sem validar nada, e a resposta tem exatamente a
FORMA de um código de país de verdade. Um embrulho fino — que é o que a
referência é — reportaria país `"not"` para lixo e quem chamasse acreditaria.

Isto é o "eu não teria adivinhado" desta medição. Nenhuma leitura do código da
referência sugeriria a necessidade de uma guarda, porque a referência não tem
nenhuma.

### O que foi entregue

Guarda dos DOIS lados:

1. **Entrada**: tem de ser dígitos depois de tirar decoração e sufixos de jid
   (`@c.us`, `@lid`, `@s.whatsapp.net`, `@g.us`) — quem chama tem jid, não
   número, e exigir que descasque só moveria esta função para cada chamador.
2. **Resposta**: o código devolvido tem de ser dígitos, senão `ErrNonsenseAnswer`
   — erro SEPARADO de `ErrNotANumber`, porque significa que a guarda de entrada
   tem uma brecha, e isso é bug nosso, não erro de quem chamou.

O teto de 15 dígitos vem do E.164 e está escrito no código como regra
**emprestada**, não medida: a medição estabelece que a página não tem teto
nenhum.

`ErrNotANumber` **não cita a entrada**. Um erro é uma linha de log esperando
para acontecer, e essa carregaria justamente o que não pode ser registrado.

### As duas guardas são independentes, e o controle provou

Ao remover só a guarda de ENTRADA, o teste falhou com
`ErrNonsenseAnswer` em vez de `ErrNotANumber` — ou seja, a segunda guarda pegou
o lixo sozinha. Isso não estava planejado; apareceu na saída do controle:

```
--- FAIL: TestGarbageNeverReachesThePage
    phone_test.go:74: Lookup(junk) err = phone: the page answered something that is not a country code, want ErrNotANumber
    phone_test.go:77: junk reached the page 1 time(s); findCC would have answered it
```

Outros dois controles: remover a validação da resposta
(`TestANonsenseAnswerIsItsOwnError` falha) e fazer o erro citar a entrada
(`TestTheRefusalDoesNotQuoteTheNumber` falha).

### Prova ao vivo (2026-08-22, conta-A)

```
real jid -> phone.Number(formattedLen=17 cc="55")
junk refused on all three inputs
web version: 2.3000.1045798079
```

`Number.String()` reporta comprimento e código de país, nunca o número.

### `getWWebVersion`

Ficou em `spa/`, ao lado do inventário de módulos, porque é o complemento
natural dele: a referência FIXA a versão da web com `initWebVersionCache`, nós
não (linha `INTENTIONAL_DIFFERENCE`) e usamos o inventário como guarda. Ler a
versão não muda essa decisão — torna-a reportável, e a versão é a coisa mais
útil para pôr ao lado de uma lista de módulos ausentes.

Página sem versão devolve `ErrNoWebVersion`, não string vazia: quem recebesse
`""` não distinguiria "este build esconde" de "a leitura falhou".

**Controle negativo que precisou de segunda tentativa**: a primeira mutação
removeu a guarda e deixou `strings` sem uso — não compilou, e a ARMADILHA §3 diz
que controle que não compila não prova nada. Refeito devolvendo
`strings.TrimSpace(out.Version)` direto, que compila e falha nas três respostas.

**Status**: entregue e provado.

## H112 — o diretório de canais responde, e devolve MODELOS, não mixins

**Data**: 2026-08-22. **Contexto**: `Client.searchChannels`, primeira das 16
linhas da família `Channel`.

**Onde**: `internal/wa-headless/capabilities/channel/` (`searchScript`,
`DirectoryEntry`, `Reader.Search`), `internal/wa-headless/probe_chansearch_test.go`.

### Primeiro achado: ele responde

`getRecommendedNewsletters` está `BLOCKED` neste ledger por TRAVAR, ignorando o
próprio timeout de 8s. A suspeita razoável era que `fetchNewsletterDirectories`
sentasse na mesma máquina. **Não senta**: respondeu 50 resultados, sem
assinatura nenhuma. A sonda foi escrita com prazo e reportando o travamento como
RESULTADO, justamente porque a hipótese era a outra.

### Segundo achado: `getNewsletterDirectoryPageSize` não existe

A referência implementa a opção `limit` **remendando** essa função da página e
restaurando-a depois. Ela mediu `false` neste build — quinta vez que um nome
vindo da lista do wwebjs não existe aqui. Nossa `SearchOptions` **não tem
`limit`**, e a ausência está documentada como decisão: além de o remendo não
funcionar aqui, remendar um global da página significa que uma restauração
falha deixa a página alterada para todo chamador seguinte.

### Terceiro achado, e o que custou uma volta

A primeira versão do leitor procurou os **mixins** — `newsletterNameMetadataMixin`
e companhia, o vocabulário que a consulta de metadados usa (H104). Resultado ao
vivo:

```
directory: 50 results; jid=50 name=50 subscribers>0=0 state=0 following=0
```

Nome e jid vinham do topo do modelo; todo o resto voltou vazio. **Isso não parece
um bug: parece um diretório magro.** Foi só a asserção "nenhum resultado trouxe
contagem" que transformou a leitura silenciosamente vazia em falha.

A medição seguinte disse onde os valores moram num resultado de diretório, sobre
os 50:

```
__x_size           number  50/50   (a contagem; primeiro resultado: 5.114.818)
__x_verified       boolean 50/50
__x_membershipType string  50/50   ("guest")
__x_creationTime   number  50/50
__x_description    string  50/50
__x_state          UNDEFINED 50/50
subscribers        object  50/50   (uma coleção, não um número)
```

Um resultado de diretório é um **modelo vivo** com campos `__x_`; a consulta de
metadados devolve um saco de mixins. Os dois se sobrepõem e não coincidem.

### Por que um tipo NOVO em vez de reusar `Channel`

`DirectoryEntry` existe porque `Channel` teria `State` e `Verification`
permanentemente vazios — e um struct com campo vazio diz "este canal não tem
estado", não "esta forma não carrega estado". Além disso `verified` aqui é
BOOLEANO e lá é STRING; mapear um no outro inventaria um valor que este pacote
nunca viu, o que o próprio `Channel` já proíbe em comentário.

### Prova ao vivo (2026-08-22, conta-A)

```
directory: 50 results; jid=50 name=50 desc=43 subscribers>0=50 verified=50 created=50
query search: 50 results, first result identical to unfiltered = false
```

43 de 50 com descrição é legítimo — nem todo canal tem uma. E a busca com termo
ESTREITA, o que prova que `searchText` chega à página: sem isso, os dois
resultados seriam idênticos.

### Controles negativos EXECUTADOS

1. Voltar a ler os mixins (a versão que devolveu 50 vazios):
   ```
   --- FAIL: TestTheSearchScriptReadsTheMeasuredFields
       channel_test.go:265: the search script does not read __x_size
       channel_test.go:272: the search script reads newsletterSubscribersMetadataMixin, which is the metadata query's vocabulary and is absent from a directory result
   ```
2. Remendar a página como a referência faz:
   ```
   --- FAIL: TestTheSearchScriptDoesNotPatchThePage
       channel_test.go:290: the search script contains "getNewsletterDirectoryPageSize", which alters the page
   ```
3. Tratar diretório vazio como erro: `TestAnEmptyDirectoryIsNotAnError` falha.

### Dívida paga de passagem

Os nomes dos mixins eram literais em linha dentro de `byInviteScript`. Ao
precisar do mesmo vocabulário num segundo script, viraram constantes (ADR-0004):
duas cópias de `newsletterNameMetadataMixin` divergiriam em silêncio, porque
mixin ausente lê como campo vazio, não como erro.

### Erro de processo desta sessão

Rodei `git checkout capabilities/channel/channel.go` para desfazer uma mutação
de controle negativo, e o arquivo continha o `Search` ainda **não commitado** —
o método foi descartado e teve de ser reescrito. A mutação estava em
`script.go`; o `checkout` no arquivo errado não tinha por que existir. Fica
registrado porque a política do projeto proíbe justamente essa classe de comando
e eu a usei por reflexo.

**Status**: entregue e provado.

## H113 — posse de canal: criado, renomeado e apagado; a descrição não pega

**Data**: 2026-08-22. **Contexto**: as 15 linhas da família `Channel` que exigem
POSSUIR um canal, mais `Client.createChannel` e `Client.deleteChannel`.

**Autorização**: criar canal é ato para fora numa conta real. Foi pedida e
concedida explicitamente, com a condição de o canal ser apagado ao final. O
delete não é um passo de teste que possa ser pulado: é a condição sob a qual
isto rodou.

**Onde**: `internal/wa-headless/capabilities/channel/owner.go` e `script.go`
(novos), `internal/wa-headless/probe_chanowner_test.go` (novo).

### O erro que deixou um canal de pé, e como foi resolvido

A primeira execução criou o canal com sucesso e **falhou em tudo depois**, com
`channel not loaded in this session`. A busca usava `WAWebChatCollection`,
copiada do lugar errado. Medido: essa coleção tem **384 conversas e ZERO
newsletters**.

Resultado: um canal real ficou de pé na conta, e a sonda registrava só FORMAS
(por disciplina de PII), então nem o jid nem o código de convite estavam no log
para apagá-lo.

A recuperação veio da referência, no arquivo certo: `Utils.js:928` mostra que a
coleção é `window.require("WAWebCollections").WAWebNewsletterCollection` — **não
é módulo próprio**; requerê-la pelo nome devolve `undefined`. Com isso, enumerei,
identifiquei pelo prefixo do nome (`wa-headless probe`) e apaguei. Confirmado
`total: 0` numa segunda passagem.

**Lição**: uma sonda que registra só formas não consegue limpar o que criou. Para
efeitos que CRIAM coisa, o identificador tem de ser recuperável — aqui foi o
prefixo do NOME, que não é PII e serviu de alça. Isso passa a ser requisito de
qualquer sonda que crie entidade.

### O que ficou provado

```
created: channel.Created(jid=true code=true at=true)
rename took and was read back from the server
deleted and confirmed gone (channel: no channel for that invite code)
```

`Create` verifica relendo pelo código de convite — o leitor da H104, provado
contra um canal que a conta NÃO segue, o que o torna oráculo e não segunda
opinião do mesmo caminho. `Delete` sem código devolve erro dizendo que **não deu
para verificar**, em vez de sucesso.

O gate desabilitado virou erro próprio: a referência devolve a string
`'CreateChannelError: A channel creation is not enabled'` para o chamador casar
com padrão.

### O que NÃO pega: a descrição

```
SetDescription immediate verdict: channel: the change did not take
MEASURED: the description never appeared within 20s, while the rename was
readable back immediately. This is not latency.
```

A página aceita a chamada e o servidor nunca reporta a descrição nova. A
descrição passada na CRIAÇÃO também não fica. É exatamente o sucesso silencioso
que a invariante 14 existe para pegar, e a pós-condição pegou — a referência
devolveria `true`.

`setDescription` fica `MISSING` **por medição, não por omissão**.

### O sucesso que passou pelo motivo errado

Na primeira execução, `SetDescription("")` **passou**. Não porque limpar
funcione: porque o servidor já estava vazio, já que a descrição nunca tinha sido
gravada. A asserção comparava o lido com o pedido, e vazio == vazio.

Isto é da mesma família da escrita redundante da H110, e a sonda foi corrigida
para só exercitar a limpeza **depois** de uma descrição existir de fato —
pulando o passo, com a razão dita em voz alta, quando ela não existe.

### `getSubscribers` não é atacável

`WAWebMexFetchNewsletterSubscribersJob` **não existe neste build** (medido) —
sexto nome vindo da lista do wwebjs que falta aqui.

### Controles negativos EXECUTADOS

1. Confiar na escrita sem reler: `TestARenameTheServerIgnoredIsAnError` falha.
2. Usar o flag como chave do valor (`editName` em vez de `name`): falha com "the
   server still reports the old value" — que é exatamente como o defeito real
   apareceria.
3. Não verificar a criação: `TestACreateThatCannotBeReadBackIsAnError` falha.
4. Voltar a busca para `ChatCollection`:
   ```
   owner_test.go:260: the script does not look in WAWebNewsletterCollection
   owner_test.go:263: the script looks in ChatCollection, which holds no newsletters
   ```

**Status**: entregue e provado, com `setDescription` e `getSubscribers` abertos
com evidência.

## H114 — busca: o termo curto acha nada, e o escopo por conversa quebra

**Data**: 2026-08-22. **Contexto**: `Client.searchMessages` e
`Client.getChatsByLabelId`.

**Onde**: `internal/wa-headless/capabilities/search/` (novo),
`internal/wa-headless/probe_search_test.go` (novo).

### A primeira medição quase enterrou a capacidade

Buscar `"a"` sobre 395 mensagens carregadas — um store cheio da letra a —
devolveu **0 resultados**. A leitura natural seria "a busca não funciona neste
build", e a linha teria ido para `MISSING` com evidência FALSA.

O que faltava era a segunda medição: pegar um termo REAL de dentro do próprio
store. Com uma palavra de 4 letras tirada de uma mensagem, **20 resultados**. A
página tem um comprimento mínimo que este pacote não conhece.

**Lição**: uma medição que produz zero precisa de um controle POSITIVO antes de
virar conclusão. Zero pode ser "não funciona" ou "perguntei errado", e as duas
se parecem exatamente.

### O escopo por conversa mede zero

Quatro formas testadas com o mesmo termo real:

```
global-1arg  -> 20 hits
global-4arg  -> 20 hits
global-5arg  -> 20 hits
scoped       ->  0 hits, eof=true
```

Passar o jid da conversa como quarto argumento é **exatamente o que a referência
faz** com `options.chatId`. Aqui devolve zero. Então este pacote **não oferece
escopo por conversa**: uma opção que mede zero é pior que uma ausente, porque a
resposta vazia lê como "não há resultados" em vez de "a opção não funciona".

`Msg.search` declara aridade **5**; a referência passa 4. O quinto argumento não
foi identificado e não é necessário — as três formas dão o mesmo.

### O corpo não sai da página

Buscar é a única capacidade cujo propósito É conteúdo, o que a torna a mais
provável de contrabandear um corpo para Go. A projeção acontece **dentro da
página**: só chat, id, direção, tipo e instante são copiados de cada resultado.
Filtrar em Go teria posto o corpo na resposta primeiro — o mesmo erro que a H107
recusou no `rawData`.

O termo da sonda é escolhido DENTRO da página e só o comprimento é registrado.

### Prova ao vivo (2026-08-22, conta-A)

```
term picked inside the page: {"len":15}
search: 16 hits (eof=false); chat=16 id=16 type=16 fromMe=16
nonsense term: 0 hits (eof=true)
```

Os 16 resultados trazem endereço completo, e o termo sem sentido devolve zero
SEM erro — porque zero é resultado normal, e tratá-lo como falha faria a busca
reportar fracasso para o caso mais comum.

### `getChatsByLabelId` não é atacável

A conta tem **3 rótulos, todos com ZERO itens** (`chatLabelItems: 0`). Um leitor
nunca seria visto devolvendo nada, que é a armadilha da H93. Fica `MISSING` com
a medição.

### Controles negativos EXECUTADOS

1. Serializar a mensagem em vez de projetar:
   `TestNoBodyCanCrossFromThePage` falha em `"serialize()"`.
2. Reintroduzir o escopo por conversa:
   `TestTheSearchIsNotScopedToAChat` falha.
3. Tratar zero resultados como erro:
   `TestNoMatchesIsNotAnError` falha.

**Status**: entregue e provado; `getChatsByLabelId` aberto com medição.

## H115 — a contagem de lint subiu 267→418, e a maior parte não é desta sessão

**Data**: 2026-08-22. **Contexto**: o `make check` imprimiu
`418 issue(s) (informativo, baseline 267)` e eu fui apurar antes de continuar
empilhando capacidades.

**Onde**: `.golangci-baseline` (`count=267`), saída de `make lint`.

### O que a apuração mostrou

O portão que TRAVA é a complexidade máxima, e ela ficou **inalterada em 56**. A
contagem é declaradamente informativa — o próprio arquivo de baseline explica
por quê: *"Contagem de issues pune decomposição"*, já que quebrar uma função
grande em cinco pequenas aumenta a contagem enquanto melhora o código.

Atribuição por tipo, sobre os 418:

```
gocyclo      a esmagadora maioria
gofmt        6
errcheck     3
staticcheck  3
ineffassign  2
unused       1
goimports    1
```

**Erro meu na primeira leitura**: usei `9737944..HEAD` como "esta sessão" e
concluí "149 avisos são meus". O intervalo tem **118 commits** — é o ramo
inteiro, não a sessão. A atribuição por ARQUIVO também superconta: tocar uma
função de `realspa_test.go` credita a mim os 20 avisos do arquivo.

Dos não-`gocyclo`, exatamente **dois** nasceram hoje, e foram corrigidos:

- `probe_msgorigin_test.go` — `gofmt`.
- `capabilities/phone/phone_test.go:103` — `errcheck`: retorno de `p.eval`
  ignorado ao primar o dublê. Agora falha o teste se a primagem falhar, o que é
  o comportamento correto e não só o silenciamento do aviso.

### O que NÃO foi corrigido, e por quê

Os treze restantes são anteriores a esta sessão e ficam registrados em vez de
consertados de graça (regra do CLAUDE.md):

```
engine/tab_test.go:35,50            errcheck  (ln.Close, c.Close)
capabilities/call/call_test.go:102  gofmt
capabilities/group/group.go:382     gofmt
capabilities/group/metadata.go:10   gofmt
capabilities/groupreq/groupreq.go:62 gofmt
events/ingress.go:168               gofmt
spa/confirmation_test.go:3          goimports
capabilities/react/react.go:183,184 ineffassign (last, lastSum)
capabilities/ack/ack_test.go:119    staticcheck QF1001
capabilities/contacts/prime.go:191  staticcheck S1016
capabilities/chatstate/chatstate.go:269  unused (const readResultScript)
```

Dois merecem atenção quando alguém tocar neles: o `ineffassign` duplo em
`react.go` sugere que `last`/`lastSum` são calculados e descartados — pode ser
lógica morta ou pode ser um bug de comparação. E `readResultScript` sem uso em
`chatstate.go` é script de página que ninguém chama: ou falta um caminho, ou é
resto.

**Não investigados** — fora do escopo. Pergunta em aberto para o usuário: quer
que eu limpe esta lista numa passagem própria?

### Sobre atualizar `count=267`

Não atualizei. A contagem só faz sentido como marco se for medida numa árvore
estável, e este ramo está recebendo pacotes novos a cada bloco. Atualizá-la
agora congelaria um número que muda na próxima hora. Fica para o fim da Fase 1.

**Status**: parcialmente corrigido — os dois avisos nascidos hoje foram
corrigidos; os treze anteriores seguem abertos e registrados acima.

## H116 — `resetState`: o zero era do instrumento, e depois foi do instrumento outra vez

**Data**: 2026-08-22. **Contexto**: `Client.resetState`.

**Onde**: `internal/wa-headless/capabilities/liveness/reset.go` e
`resetscript.go` (novos), `internal/wa-headless/probe_reset_test.go` (novo).

### Primeira medição: zero transição

`Client.resetState` chama `Socket.reconnect()` e **não devolve nada** — nem
erro, nem estado. É o sucesso silencioso mais puro da superfície: quem chama não
distingue "o socket foi reiniciado" de "o nome do módulo mudou e nada
aconteceu".

Para dar pós-condição a isso, o reconnect precisa ser observável. Amostrando
`__x_state` a cada **500ms** depois do reconnect: **CONNECTED em 24 de 24**.
Leitura natural: reconnect não faz nada, e a linha vai para `MISSING`.

### O controle positivo, aplicado pela regra escrita uma hora antes

A H114 tinha acabado de estabelecer: *medição que produz zero precisa de
controle positivo*. Apliquei duas vezes:

1. **O instrumento enxerga mudança?** Escrevi um valor sentinela em `__x_state`,
   li de volta, restaurei. `sawSentinel: true, restored: true`. O leitor
   funciona.
2. **Amostrar mais rápido.** A 50ms: **OPENING em 9 de 100 amostras**.

O reconnect TRANSITA, por cerca de 450ms, e o amostrador de 500ms passava por
cima. **O zero era do instrumento, não do mundo.** Sem a regra da H114 esta
linha teria sido enterrada com evidência falsa.

### Segunda vez, e por isso a linha é `PARTIAL`

A capacidade foi escrita amostrando pelo Go a 50ms. Ao vivo, três resets:

```
reset 1: settled=true sawOpening=false took=166ms
reset 2: settled=true sawOpening=false took=156ms
reset 3: settled=true sawOpening=false took=155ms
the transition was caught in 0 of 3 resets
```

**0 de 3**, contra 9 de 100 dentro da página. Cada leitura do Go é um
ida-e-volta do chromedp, então a taxa efetiva é muito mais grossa que a janela.

A consequência está escrita no código em vez de escondida: **esta pós-condição
prova que o socket está SAUDÁVEL depois da chamada, não que a chamada fez
alguma coisa.** Provar a transição exigiria amostrar DENTRO da página e parquear
para o Go colher — a forma store-and-poll que a invariante 6 já prescreve, mas
com um intervalo de página que precisaria de entrada própria na lista de exceção
da invariante 6. Isso é decisão a tomar, não descuido, e é por isso que a linha
é `PARTIAL` e não `PROVEN`.

### Um defeito meu, achado pelo meu próprio teste

A primeira versão exigia um mínimo de **tempo de relógio** antes de aceitar
`CONNECTED` (`time.Since(started) >= ResetTick*4`). Isso é exatamente a forma
sensível a carga da F100: numa máquina ocupada, o mesmo código amostra MENOS
vezes dentro da mesma janela, e a guarda enfraquece justamente quando o sistema
está pior. Trocado por **contagem de leituras** (`minSettleReads = 4`), que não
se move.

O teste que pegou isso foi `TestAnImmediateConnectedDoesNotCertifyItself`,
escrito para outra coisa: garantir que o verificador não certifique o estado de
onde partiu.

### Controles negativos EXECUTADOS

1. Aceitar o primeiro `CONNECTED`:
   `reset_test.go:139: the verifier accepted after 1 reads; it certified the state it started from`
2. Aceitar qualquer resposta da página como sucesso: falha em `""` e em
   `"no-reconnect"`.
3. Fundir "nunca voltou" com "recusou": falha, porque as duas têm reparo
   diferente — um socket que não volta deixa a sessão PIOR do que antes do
   reset.

**Status**: parcialmente entregue — a chamada e a saúde posterior estão
provadas; a prova de EFEITO fica aberta com o caminho registrado.

## H117 — os dois suspeitos da H115: `react.go` ineffassign e `chatstate.go` unused

**Data**: 2026-08-22. **Contexto**: limpeza dos 13 achados de lint pré-existentes
registrados na H115.

### `react.go:183,184` — ineffassign em `last` e `lastSum`

**Suspeita original**: `last` e `lastSum` recebem valor inicial que nunca é lido;
pode ser lógica morta ou bug de comparação.

**Veredito**: código morto (inicialização inútil), NÃO defeito. Evidência:

1. `last` é inicializado com `!want` na linha 183. A única leitura é nas linhas
   210 e 213, dentro do bloco de timeout. Mas a linha 205 (`last = got.Has`)
   SEMPRE executa antes do timeout — o único salto anterior é `return` na linha
   202 (`!got.Found`), que sai da função sem ler `last`. Logo, o valor
   inicial nunca chega a ser observado.

2. `lastSum` é inicializado com `-1` na linha 184. Mesma mecânica: a linha 204
   (`lastSticky, lastSum = got.Sticky, got.Sum`) sempre executa antes da
   leitura na linha 210.

3. As variáveis SÃO usadas (na mensagem de erro de timeout) — só os valores
   INICIAIS são mortos. A correção: trocar `last := !want` e
   `lastSticky, lastSum := false, -1` por declarações `var` sem inicializador.

4. Nenhuma mudança de comportamento: o zero value de `bool` (`false`) e de
   `int` (`0`) nunca chega a ser lido — o loop sempre os sobrescreve na
   primeira iteração antes de qualquer branch que os leia.

**Correção**: declaração `var` sem valor inicial. Todos os testes existentes
passam, incluindo `-race`.

### `chatstate.go:269` — unused `readResultScript`

**Suspeita original**: script de página sem uso; ou falta um caminho de código,
ou é resto de refactor.

**Veredito**: resto de refactor (código morto), NÃO caminho faltante. Evidência:

1. `readResultScript` é definido na linha 269 e NÃO é referenciado em nenhum
   outro lugar do pacote nem do repositório inteiro (`grep -rn` retorna só a
   definição).

2. `readKey` (que `readResultScript` usa) É referenciado — o teste
   `chatstate_test.go:44,49` usa `readKey` diretamente para construir as mesmas
   strings de roteamento no dublê. Ou seja, a CONSTANTE que dá nome ao slot de
   página está viva, mas a constante que MONTA o script de leitura foi
   substituída por uso inline no teste e nunca chamada de produção.

3. O comentário na linha 264 diz "it is kept because the tests route on it" —
   mas "it" é `readKey`, não `readResultScript`. O comentário justifica
   `readKey`, que de fato é usado; `readResultScript` ficou ao lado sem
   ninguém perceber que o teste construiu o mesmo padrão por conta própria.

**Correção**: remoção de `readResultScript`. `readKey` permanece (é usado pelo
teste e pela produção).

### Validação do Chief — conferida na fonte, não aceita por relatório

Confirmei os dois vereditos lendo o código, porque "nenhum era defeito real" é a
conclusão conveniente e precisava de checagem independente:

- `react.go`: as três atribuições (`lastSticky`, `lastSum`, `last`) de fato
  SEMPRE executam antes de qualquer leitura, e as únicas saídas anteriores são
  `return` que não as lê. Inicialização morta confirmada.
- `chatstate.go`: `readResultScript` não aparece mais em lugar nenhum, e o
  comentário da linha 264 é mesmo sobre `readKey`, que segue usado pelo teste.

Aritmética do lint fecha: 418 → 405 avisos, exatamente 13 a menos, com
complexidade máxima inalterada em 56. Os quatro avisos que sobram nos arquivos
tocados são `gocyclo` pré-existente, não os alvos.

### Achado incidental do Chief — NÃO corrigido, vai para triagem

Com `readResultScript` removido, **nenhum script de produção emite `readKey`**.
A constante sobrevive usada só pelo teste, e os dois ramos do dublê em
`chatstate_test.go:44,49` — que roteiam em `readKey+'"] = null'` e em
`JSON.stringify(window["readKey"])` — passam a ser **inalcançáveis**: a produção
nunca produz aquelas strings.

Isso não é defeito de comportamento, mas é cobertura falsa: dois ramos de dublê
que nunca disparam dão impressão de exercitar um caminho que não existe. E o
comentário da linha 264 diz que a constante é mantida porque "os testes roteiam
nela" — o que agora é circular.

**Não investigado nesta sessão** — a remoção estava no escopo, esta consequência
não. Pergunta em aberto: apagar `readKey` e os dois ramos, ou restaurar o
caminho de leitura assíncrona que eles pressupunham?

**Status**: **corrigido** — os dois eram código morto; nenhum era defeito real. As
correções não alteraram comportamento de produção e todos os testes passam. O
achado incidental acima segue aberto.

## H118 — a auditoria de fachada: 10 de 43 herdam, e 33 são trabalho de verdade

**Data**: 2026-08-22. **Contexto**: generalizar a H109 (a família `Chat` era
fachada) para todas as famílias do LEDGER-WWEBJS com linhas `MISSING`.

**Onde**: `internal/wa-headless/AUDITORIA-FACHADA.md` (novo, produzido por
worker sob o substrato `orca`), `LEDGER-WWEBJS.md`.

### A hipótese, e o que a medição fez com ela

A hipótese era minha: *"se `Chat` era fachada, outras famílias também serão"*.
O packet do worker dizia explicitamente para MEDIR, não para confirmar — e que
"não há mais fachada" seria resultado válido.

Resultado sobre 43 linhas `MISSING` em 10 famílias:

```
(a) delegação pura .......... 18
(b) delegação com lógica .....  6
(c) implementação própria .... 19
herdam de fato .............. 10   (as (a) cujo par no Client tem estado)
```

**A hipótese confirma-se apenas em parte, e isso é o achado.** `Broadcast` (2 de
2) e `GroupNotification` (3 de 4) são fachada como `Chat` era. Mas `Message`
(9 de 9) e `GroupChat` (3 de 3) são implementação própria: ali não há atalho, é
trabalho. Oito das (a) não herdam porque o par no `Client` também está
`MISSING` — herança não inventa estado.

### Validação da saída do worker — não aceita por relatório

Conferi na fonte, no upstream FIXADO, e não pelo que o worker disse:

- As **8** linhas herdadas com número de linha (`GroupNotification.js:77–79,
  85–87, 108–110`; `Broadcast.js:55–57, 63–65`; `Channel.js:239–241, 247–249,
  380–382`) batem EXATAMENTE, e o código é delegação de uma linha.
- As **6** classificadas `(b)` foram conferidas uma a uma: `Contact.getChat`
  tem `if (this.isMe) return null`; `Message.getMentions` faz `Promise.all`
  sobre `mentionedIds`; `Chat.addOrEditCustomerNote` tem
  `if (this.isGroup || this.isChannel) return`. Classificação correta nas seis.

Um detalhe que o worker registrou e eu teria perdido:
`GroupNotification.getContact()` delega com `this.author`, **não** com o chat.
Continua delegação pura, mas resolve pessoa diferente do que o nome sugere.

### O que mudou no ledger

10 linhas saem de `MISSING`: **3 para `PROVEN`** (`Contact.getProfilePicUrl`,
`Contact.getCommonGroups`, `Channel.deleteChannel` — pares provados) e **7 para
`PARTIAL`** (pares parciais). O placar vai de `PROVEN 82 / MISSING 79` para
`PROVEN 85 / PARTIAL 60 / MISSING 69`.

Vale a mesma ressalva da H109: **nenhuma prova nova foi produzida aqui.** A
defesa é a auditabilidade — cada linha do `AUDITORIA-FACHADA.md` cita
`arquivo:linha` da SHA fixada, e eu conferi 14 delas à mão.

**Status**: entregue — auditoria completa e heranças aplicadas.

## H119 — eventos de grupo: um portador, quatro eventos, e a H86 fica mais precisa

**Data**: 2026-08-22. **Contexto**: decisão **40=a** da orquestração — atacar a
família `Events`, a maior concentração de `MISSING` sem insumo externo.

**Onde**: `internal/wa-headless/events/group.go` e `group_test.go` (novos),
`events/ingress.go`, `events/events.go`, `probe_gp2_test.go` (novo).

### O que a leitura da referência mostrou

`GROUP_JOIN`, `GROUP_LEAVE`, `GROUP_ADMIN_CHANGED` e `GROUP_UPDATE` não são
quatro fontes: são **uma**. O upstream fixado (`Client.js:585–645`) recebe uma
mensagem comum de tipo `gp2` e a despacha em quatro eventos olhando o
`subtype`. E o nosso ingress **já escuta** `MsgCollection.on('add')` — ou seja,
essas mensagens já chegavam; faltava classificá-las.

A classificação ficou em **Go**, não na página: decidir na página é o que a
invariante 6 proíbe. Só o `subtype` viaja, num campo novo da projeção que já
existia.

### Medição antes de projetar

395 mensagens carregadas, **1** de tipo `gp2`, subtipo `membership_approval_mode`.
Campos presentes nela: `__x_subtype`, `__x_author`, `__x_recipients`, `__x_t`,
`__x_type` — tudo que uma `GroupNotification` precisa. `__x_body` ausente.

E `WAWebBatteryStore` **não existe** neste build, o que resolve `BATTERY_CHANGED`
como `BLOCKED`: a própria referência marca o evento como depreciado e não
enviado em multi-device, que é o que este build é.

### A prova ao vivo precisou PRODUZIR o evento

A primeira execução ligou o barramento e esperou: chegaram `chat.changed:4` e
`contact.changed:7`, e **nenhuma `gp2`**. O motivo não é defeito — a única `gp2`
da conta é histórica, e `MC.on('add')` só dispara para adições NOVAS.

Então produzi uma: troquei o assunto do grupo de laboratório, que emite `gp2`
com subtipo `subject`.

```
event types seen: map[chat.changed:15 contact.changed:8 group.updated:1]
subtypes seen:    map[subject:1]
the gp2 carrier was reclassified as group.updated
```

O assunto é restaurado por `defer` registrado antes de qualquer coisa que possa
falhar — `t.Cleanup` rodaria depois do `defer` que para a sessão.

### A H86 não caiu: ficou MAIS PRECISA

A H86 tinha medido *"mudança de participante produz ZERO evento na sessão que a
fez"*, e as três linhas de grupo estavam `MISSING` por causa disso.

Eu mudei o **assunto**, não os participantes. O resultado prova que o portador
`gp2` CHEGA a este barramento — o que estreita o achado da H86 em vez de
contradizê-lo: o zero dela é dos subtipos de **participante**, **na sessão que
agiu**. Uma sessão OBSERVADORA (conta-A vendo ação de conta-B) nunca foi
testada, e é por aí que `GROUP_JOIN`/`LEAVE`/`ADMIN_CHANGED` poderiam fechar.

Registro isto explicitamente porque a tentação era escrever "a H86 estava
errada". Não estava; eu medi outra coisa.

### Uma divergência deliberada da referência

O upstream termina a cadeia de `if/else` com um `else` que transforma **qualquer**
subtipo não reconhecido em `GROUP_UPDATE`. Isso significa que um subtipo que a
Meta acrescente amanhã chegaria rotulado errado, em silêncio.

Aqui a tabela é explícita e um subtipo não mapeado **permanece `MessageAdded`**,
com o `subtype` intacto no evento — visível para quem assina, em vez de
engolido num balde.

### Controles negativos EXECUTADOS

1. Reclassificar sem exigir o portador `gp2`:
   `group_test.go:47: a chat message with subtype "add" became "group.joined"`
2. Engolir subtipo desconhecido num default, como a referência faz:
   `group_test.go:84: an unmapped subtype was relabelled "group.updated"`
3. Não projetar o `subtype` na página:
   `group_test.go:98: the page projection never reads m.subtype; every group message would arrive unclassifiable`

O teste de completude do próprio pacote (`TestTypeListsAreDisjointAndComplete`)
pegou sozinho o crescimento de `KnownTypes` e foi estendido para três listas em
vez de ganhar exceção.

**Status**: parcialmente entregue — `GROUP_UPDATE` provado ao vivo,
`BATTERY_CHANGED` bloqueado com evidência, e os outros três com classificador
provado em unidade e ao vivo pendente de uma sessão observadora.

## H120 — `MEDIA_UPLOADED`: a H88 disse "não medido", e agora está medido

**Data**: 2026-08-22. **Contexto**: continuação da decisão 40=a (família
`Events`), atacando a única linha que o próprio ledger marcava como **não
medida** em vez de impossível.

**Onde**: `internal/wa-headless/probe_mediaevent_test.go` (novo),
`LEDGER-WWEBJS.md`.

### Por que esta linha e não outra

Das sete `Events` restantes, seis já tinham veredito medido na H88:
`AUTHENTICATED` sem observável, `CHAT_REMOVED` exigiria destruir a fixture,
`MESSAGE_CIPHERTEXT`/`_FAILED` vivem abaixo do modelo (o barramento escuta
COLEÇÕES, não o fio), `MESSAGE_REVOKED_ME` falta o método antes do evento,
`REMOTE_SESSION_SAVED` depende de família inexistente.

`MEDIA_UPLOADED` era a exceção, e a nota dizia isso em voz alta: *"`message.added`
com `Kind` PODE já cobrir a semântica — mas isso não foi medido, e
'provavelmente coberto' não é um estado deste vocabulário"*. Era dívida de
medição, não de implementação.

### A medição

Barramento ligado, hidratação drenada por 8s para a amostra ser atribuível, e
então um PNG de 1×1 enviado ao par de laboratório:

```
sent: ack=1
after the send: 12 events
  types = map[chat.changed:9 message.ack:2 message.added:1]
  kinds = map[image:3]
```

**A mensagem de mídia chega ao barramento** e progride pelos acks. O `Kind`
viaja em 3 dos 12 eventos.

### O que a medição NÃO autoriza dizer

Não há **momento distinto de upload concluído**. `MEDIA_UPLOADED` na referência
dispara quando a mídia termina de subir, que é um instante diferente de a
mensagem existir. Aqui os dois são indistinguíveis: temos "a mensagem apareceu"
e "o ack andou", e nada entre eles.

Por isso a linha fica `PARTIAL` e não `PROVEN`. Um assinante que precise de "o
upload terminou" tem uma aproximação — o ack — e não o fato. Dizer `PROVEN`
seria vender a aproximação como o original, que é exatamente o que o
vocabulário deste ledger existe para impedir.

### Nota de método

O PNG de 1×1 é escolha deliberada: mídia real o bastante para o caminho de
upload ser genuíno, pequena o bastante para não empurrar bytes ao par. E a
amostra só conta eventos POSTERIORES ao marco de hidratação — sem isso, os
eventos de boot entrariam na conta e o número não significaria nada.

**Status**: entregue — a dívida de medição da H88 está paga, com o resultado
contrariando parcialmente a expectativa: `Kind` cobre a mídia, mas não cobre o
upload.

## H121 — `VOTE_UPDATE`: a porta limpa existe, e desta vez ela DISPAROU

**Data**: 2026-08-22. **Contexto**: última linha da família `Events` sem
veredito medido, seguindo a decisão 40=a.

**Onde**: `internal/wa-headless/events/group.go` (tipo `VoteUpdated`),
`events/ingress.go` (ouvinte), `spa/modules.go` (`ModuleCollections`),
`events/group_test.go`, `probe_gp2_test.go`.

### A referência remenda; nós não precisamos

`Client.js:1195–1235` do upstream fixado **não instala ouvinte nenhum** para
votos: usa `WWebJS.injectToFunction` sobre
`WAWebAddonPollVoteTableMode.pollVoteTableMode.bulkUpsert` e lê os argumentos de
passagem. É a mesma situação do `INCOMING_CALL`, onde a referência patcheia um
`Map` interno.

A H112 estabeleceu por que este módulo não faz isso: remendar um global da
página significa que uma restauração falha deixa a página alterada para todo
chamador seguinte.

### O que a medição achou

```
WAWebPollVoteCollection          -> falsy  (não é módulo próprio)
WAWebCollections.PollVote        -> objeto com on:true, off:true,
                                    getModelsArray:true, count 0
pollVoteTableMode.bulkUpsert     -> existe, mas .on NÃO existe
```

**A coleção limpa existe**, escondida como MEMBRO de `WAWebCollections` em vez
de módulo próprio — a mesma forma que guardava a coleção de newsletters e que
custou uma execução na H113. Registrei isso na constante `ModuleCollections`
para a próxima pessoa não repetir.

### E desta vez disparou

A coleção media **vazia**, o que colocaria o ouvinte na posição do
`CallIncoming`: instalado, nunca visto disparar. A saída dessa posição é fazer
a coisa acontecer — foi assim que o `GROUP_UPDATE` fechou na H119.

Votei numa enquete que já estava no store desta conta:

```
poll search: {"found":true,"options":2,"optLen":16,"fromMe":true}
Vote returned: <nil>
types seen: map[chat.changed:207 contact.changed:1 message.added:34 poll.vote:1]
the vote listener FIRED: 1 poll.vote
```

Deliberadamente **não** criei uma enquete para votar nela: a H98 mediu que
enquete criada não sai deste build, então construir a prova sobre isso seria
construir sobre areia.

### O que NÃO cruza

Um voto carrega **quem votou e em quê**. A opção escolhida é texto que o autor
da enquete escreveu, e não atravessa: a linha leva o id da mensagem da enquete
e o jid do votante, para roteamento, e nada mais. Há teste que falha se
`selectedOptions` ou `options` aparecerem na projeção.

### Controles negativos EXECUTADOS

1. Trocar a porta limpa pelo remendo da referência:
   `group_test.go:146: the install contains "bulkUpsert", which is the reference's patching technique`
2. Projetar o texto da opção:
   `group_test.go:167: the vote row projects "selectedOptions", which is poll content`
3. Não registrar o handler para remoção:
   `group_test.go:177: the vote handlers are not registered in s.handlers, so uninstall cannot take them off`

### Armadilha repetida, e agora com defesa

A primeira versão do controle 1 falhou **contra o código correto**: a guarda
casou com o próprio COMENTÁRIO do script, que explica que a referência remenda
`bulkUpsert`. É a nona ocorrência desta classe neste repositório. Acrescentei
`installCodeForTest`, que remove comentários antes de verificar, com o motivo
escrito ao lado — uma guarda que casa com a própria documentação não pode ser
escrita honestamente.

**Status**: entregue e provado ao vivo.

## H122 — pareamento e `logout`: separar "falta máquina" de "falta estado"

**Data**: 2026-08-22. **Contexto**: família `Client`, a maior concentração
restante. Quatro linhas que estavam `MISSING` por nunca terem sido atacadas.

**Onde**: `internal/wa-headless/probe_gp2_test.go`
(`TestProbePairingSurface`), `LEDGER-WWEBJS.md`.

### A pergunta que a medição responde

`MISSING` significa "não atacado", e é um estado honesto enquanto ninguém
olhou. Depois de olhar, ele vira mentira: esconde a diferença entre *não existe
nada para construir em cima* e *existe tudo, e falta um estado que só um humano
produz*. As quatro linhas abaixo eram do segundo tipo e estavam contadas como o
primeiro.

### O que foi medido

```
WAWebAltDeviceLinkingApi          -> existe
  .setPairingType                 -> função
  .initializeAltDeviceLinking     -> função
  .startAltLinkingFlow            -> função
WAWebPairingCodeLinkUtils         -> NÃO existe (módulo)
WAWebLaunchSocketUtils.refreshQR  -> função
WAWebMiscBrowserUtils             -> existe, mas .info NÃO é função
Socket.logout                     -> função
window.AuthStore (injeção do wwebjs) -> ausente, como esperado
socket                            -> CONNECTED
```

### As quatro conclusões

**`requestPairingCode` → `BLOCKED`.** A máquina existe inteira, e — detalhe que
importa — é alcançável **sem** o `AuthStore` que a referência injeta. A
referência chega lá por `window.AuthStore.PairingCodeLinkUtils`; aqui as mesmas
três funções vivem em `WAWebAltDeviceLinkingApi`. O bloqueio é de ESTADO: o
próprio laço da referência para quando o socket sai de `UNPAIRED`/
`UNPAIRED_IDLE`, e o nosso lê `CONNECTED`. Desemparelhar para provar exige um
humano com o telefone.

**`cancelPairingCode` → `BLOCKED`.** Mesmo gate. Não há código ativo para
cancelar numa sessão pareada.

**`logout` → `BLOCKED`.** `Socket.logout` existe. O bloqueio aqui não é
técnico, é de política: a chamada desemparelha a conta e a restauração exige um
humano com o telefone. Um agente não deve exercitar isso.

**`setDeviceName` → `INTENTIONAL_DIFFERENCE`.** Três razões independentes, e
qualquer uma bastaria: (1) a referência REMENDA `WAWebMiscBrowserUtils.info`, e
`info` **não é função** neste build — o remendo falharia; (2) remendar um global
da página é o que a H112 recusou, com motivo que não mudou; (3) o nome só
aparece no PAREAMENTO, que uma sessão já pareada não exercita.

`WAWebPairingCodeLinkUtils` é o **sétimo** nome da lista do wwebjs que não
existe neste build, e `WAWebMiscBrowserUtils.info` é o oitavo desencontro. O
padrão já não é anedota: copiar a lista da referência falha por volta de uma vez
a cada duas.

### O que isto NÃO é

Não é implementação. Nenhuma linha de código de produção foi escrita, e o placar
não ganhou nenhum `PROVEN`. O que mudou é que quatro linhas deixaram de dizer
"não atacado" e passaram a dizer **por que** não são atacáveis, com a evidência
ao lado — que é a diferença entre um ledger que orienta trabalho e um que só
conta itens.

**Status**: decidido e registrado, sem código novo.

## H123 — assinar canal: autorizado, tentado, e medido IMPOSSÍVEL neste build

**Data**: 2026-08-22. **Contexto**: o usuário autorizou explicitamente assinar
um canal, o que destravaria `subscribeToChannel`, `unsubscribeFromChannel` e
`getChannels` em cadeia.

**Onde**: `internal/wa-headless/capabilities/channel/` (`Follow`, `Unfollow`,
`Followed`, `followScript`, `followedScript`),
`internal/wa-headless/probe_follow_test.go` (novo).

### O que foi construído

`Follow`/`Unfollow` com pós-condição de verdade: a referência devolve um
booleano **por a chamada não ter lançado**, e aqui a membership é RELIDA — uma
página que aceitou a chamada e deixou a conta como `guest` é falha, não
sucesso. `Followed` lista o que a conta segue.

### O que a medição encontrou, em três passos

**Passo 1 — `find` recebe Wid, não string.** Passar o jid cru devolveu "channel
not reachable" contra um canal que a consulta de metadados lia sem problema.
Erro meu de suposição, corrigido.

**Passo 2 — o `find` da coleção está QUEBRADO.** Com o Wid correto:

```
this.findImpl is not a function
```

**Passo 3 — a ação não aceita nenhuma forma disponível.** Testadas três, todas
falham igual:

```
subArity: 3            (a referência chama com 2 argumentos)
metadata-object -> Data passed to getter must include an id property
wid             -> idem
jid-string      -> idem
```

A mensagem diz o que falta: a ação exige um **modelo que a coleção memoize por
id**. O modelo só existiria se a coleção conseguisse buscá-lo, e o `find` dela
não funciona. A referência esconde essa resolução dentro do helper
`WWebJS.getChat` que ela INJETA e nós não injetamos — o que é decisão registrada
como `INTENTIONAL_DIFFERENCE` desde o início.

`subscribeToChannel` e `unsubscribeFromChannel` viram `BLOCKED` com esta
evidência. **Não é falta de tentativa nem de autorização**: a autorização foi
dada, o código foi escrito e as três formas foram exercitadas contra a página
real.

### `getChannels` fechou por outro caminho

O leitor respondia 0 porque a conta não segue nada, e embarcar leitor nunca
visto devolvendo algo é a armadilha da H93. Como assinar canal alheio é
impossível, provei com um canal **próprio** — que vive na mesma coleção:

```
created: channel.Created(jid=true code=true at=true)
Followed returned 1
  channel.DirectoryEntry(... membership=owner)
deleted and confirmed gone
```

### Erro meu de desenho, corrigido no meio

A primeira versão devolvia `ErrNotReachable` pelado, engolindo a razão da
página. Custou uma execução: eu não conseguia distinguir "canal não existe" de
"o `find` rejeitou a forma do argumento". A razão passou a viajar dentro do
erro, e foi ela que revelou o `findImpl`.

**Regra**: quando um erro traduz uma falha da página para um erro nosso, a
mensagem original tem de viajar junto. O nome do erro classifica; o texto da
página é o que permite depurar.

### Contagem que já não é anedota

`subscribeToNewsletterAction` com aridade 3 contra 2 é o **nono** desencontro
entre a lista do wwebjs e este build. Sete nomes ausentes, duas assinaturas
diferentes.

### Segundo erro meu, pego pelo portão

Mudei a produção de `NC.find(JID)` para `NC.find(createWid(JID))` e **não
atualizei a asserção**, que continuava exigindo a forma antiga. O `make check`
falhou em 0,00s — falha real, não de carga — e a distinção foi feita pelo tempo
antes de qualquer suposição.

A correção não foi só realinhar a string: a asserção passou a travar o **Wid**,
que é o requisito MEDIDO. Sem isso, alguém voltaria à string crua e o teste
passaria, porque a forma errada só falha em RUNTIME, contra a página real, onde
teste de unidade nenhum olha. Controle negativo executado:

```
--- FAIL: TestTheFollowScriptFetchesAChannelItDoesNotHave
    owner_test.go:404: the script passes the raw jid to find; this build needs a Wid
```

**Status**: parcialmente entregue — `getChannels` provado, assinatura bloqueada
com evidência, e o código de `Follow`/`Unfollow` fica no lugar porque a
diferença é da PÁGINA e pode voltar num build futuro.

## H124 — reações: remedi um negativo antigo, errei a leitura, e a H83 continua de pé

**Data**: 2026-08-22. **Contexto**: `Message.getReactions`, atacando a família
`Message` enquanto a orquestração decide a frente grande.

**Onde**: `internal/wa-headless/probe_reactsource_test.go` (novo),
`probe_gp2_test.go`.

### Por que remedir

A H83 concluiu que não há fonte para QUAL reação — `events.MessageReaction` diz
apenas que as reações se moveram. Isso é antigo, e **duas vezes hoje** algo que
parecia ausente estava escondido como MEMBRO de `WAWebCollections` em vez de
módulo próprio (`PollVote` na H121, a coleção de newsletters na H113). Remedir
custa uma sonda; presumir um negativo velho é como uma capacidade fica fechada
depois que o mundo mudou.

### O erro que cometi no meio, e que preciso registrar

A primeira sonda mostrou uma coleção de reações com `count: 1` e um modelo
carregando `__x_reactionText`. Eu li isso como *"o negativo da H83 está velho"* e
**disse isso em voz alta**. Estava errado.

A coleção com 1 item era **`RecentReactions`** — a lista de emojis recentes do
seletor, cujo `__x_id` é o próprio emoji (comprimento 2, sem casar com nenhum
dos 395 ids de mensagem carregados). A coleção `Reactions`, a de verdade, tinha
**0**.

O erro foi de leitura: a sonda emitia as duas coleções no mesmo objeto e eu
peguei a resposta da errada. A correção veio de medir a FORMA do `__x_id` —
quantos segmentos, se algum casa com id de mensagem conhecido — em vez de olhar
para a existência do campo.

### A medição decisiva

Coleção vazia não é resposta: pela regra da H114, um zero precisa de controle
positivo. Então reagi a uma mensagem própria com a capacidade que já existe e
olhei de novo:

```
Reactions collection before: 0
Reactions collection after:  0
reaction removed
```

**A coleção não enche.** Não é falta de coleção — ela existe, aceita ouvinte, e
permanece vazia depois de uma reação que a própria capacidade verificou como
aplicada.

`getReactions` passa de `MISSING` a `BLOCKED`, com evidência mais forte do que a
da H83: antes era "não achamos fonte", agora é "a fonte existe e não é
alimentada neste build".

### O que fica da regra de remedir

Ela continua certa mesmo tendo dado negativo aqui — duas das três vezes hoje
pagou. O que muda é o cuidado na leitura: quando a sonda mede várias fontes de
uma vez, o resultado tem de dizer QUAL fonte respondeu o quê, ou a resposta certa
da fonte errada vira conclusão.

**Status**: decidido e registrado. `getReactions` bloqueado com medição
atualizada; nenhum código de produção escrito.

## H125 — a família `Message`: separar bloqueio de MÓDULO de bloqueio de DADO

**Data**: 2026-08-22. **Contexto**: decisão **50=b** da orquestração — atacar
`Message`, as 9 linhas que a auditoria da H118 classificou como implementação
própria, sem atalho de fachada.

**Onde**: `internal/wa-headless/probe_gp2_test.go`
(`TestProbeMessageFamilyModules`), `LEDGER-WWEBJS.md`.

### Uma sonda, duas perguntas

Módulo ausente e módulo presente sem dado para exercitá-lo são vereditos
DIFERENTES, e a mesma sonda mede os dois: quais funções existem, e quantas
mensagens de cada tipo a conta tem para rodar contra elas.

```
WAWebBizOrderBridge.queryOrder                -> função
WAWebGroupInviteV4Job                         -> módulo existe
  .acceptGroupV4Invite                        -> NÃO é função
  .sendGroupInviteMessage                     -> NÃO é função
WAWebScheduledEventEditAction                 -> NÃO existe
WAWebScheduledEventCreateAction               -> NÃO existe

dados da conta: orders 0 · payments 0 · groupInvites 0 · scheduled events 0
```

### As quatro conclusões

**`getOrder` e `getPayment` → `BLOCKED` por DADO.** O módulo está lá e a função
é chamável. Falta o dado: zero pedidos e zero pagamentos entre as mensagens.
Exercitar exigiria atividade comercial real na conta, que não é coisa que um
agente produza.

**`acceptGroupV4Invite` → `BLOCKED` por MÓDULO E por DADO.** O módulo existe e
**nenhuma** das duas funções que a referência chama existe nele. Décimo
desencontro com a lista do wwebjs. E a conta não tem convite v4 nenhum para
aceitar — os dois bloqueios são independentes, e qualquer um bastaria.

**`editScheduledEvent` → `BLOCKED` por MÓDULO.** Nem o de edição nem o de
criação existem. Não há o que chamar.

### Por que isto não é "desistir"

`MISSING` diz "não atacado", e depois de uma medição isso deixa de ser verdade.
As quatro linhas ficam abertas do mesmo jeito, mas agora dizem POR QUE, e a
distinção orienta trabalho futuro: uma bloqueada por DADO abre sozinha quando a
conta tiver o dado; uma bloqueada por MÓDULO só abre se a Meta trouxer o módulo
de volta. São prazos diferentes e riscos diferentes.

### Contagem

Décimo desencontro entre a lista de módulos do wwebjs e este build: agora são
oito nomes/funções ausentes e duas assinaturas divergentes. Copiar a lista da
referência erra em torno de metade das vezes, e este número está anotado a cada
ocorrência justamente para não virar impressão.

**Status**: decidido e registrado, sem código novo. Sobram em `Message`:
`getMentions`/`getGroupMentions` (medidos zero na H106) e `pin`/`unpin`
(medidos falhando na H81).

## H126 — descrição de grupo: aridade 1 contra 4, e o servidor que aceita e não guarda

**Data**: 2026-08-22. **Contexto**: `GroupChat.setDescription`, o alvo mais
promissor do que restava — este módulo já prova `SetSubject` contra o grupo de
laboratório, então a forma era familiar e a fixture existia.

**Onde**: `internal/wa-headless/capabilities/group/description.go` e
`descriptionscript.go` (novos), `probe_gdesc_test.go` (novo).

### Duas descobertas, e a segunda anula a primeira

**Primeira: a assinatura é outra.** A referência chama
`setGroupDescription(wid, texto, newId, descId)` — quatro posicionais. Neste
build a função tem **aridade 1** e recebe um objeto. A forma posicional morre
com:

```
Cannot read properties of undefined (reading 'toJid')
```

Erro que não diz nada sobre aridade e custou uma execução ao vivo para decodificar.
Medindo as formas, `{groupWid, description, newId, prevDescId}` passa. Décimo
primeiro desencontro com a lista do wwebjs.

Também medi que os módulos *Action* que este build usa para o assunto
(`WAWebSetGroupSubjectAction`) **não têm equivalente** para descrição: só existe
o *Job*, com `setGroupSubject` (aridade 2), `setGroupDescription` (1),
`setGroupProperty` (3) e `setEphemeralGroupProperty` (1).

**Segunda: com a assinatura certa, o servidor ainda não guarda.**

```
baseline description present=false source=none
SetDescription immediate verdict: asked for 35 bytes and the server reports 0
MEASURED: a descrição nunca apareceu em 20s
```

A chamada é aceita, não lança, e a descrição não existe depois. Medi a espera
em vez de supor, exatamente como na H113.

### O padrão que emerge, e que vale mais que a linha

Este é o **mesmo comportamento** que a descrição de CANAL mostrou na H113: aceita
e não persistida. Duas superfícies independentes — grupo e canal — com a mesma
falha. Não é acidente de uma delas: **escrita de descrição não persiste neste
build**.

Isso muda o que a próxima pessoa deve fazer: não vale reimplementar a de canal
achando que o problema era a de canal, nem vice-versa. É um só fato.

### Por que o código fica

`SetDescription` fica no lugar, com a pós-condição que o pegou, seguindo o
precedente que a orquestração fixou na decisão 51=a para `Follow`/`Unfollow`: a
diferença é da PÁGINA, não nossa, e um build futuro pode devolvê-la. O que muda
é que a linha diz `BLOCKED` com a medição, em vez de fingir sucesso — que é
exatamente o que a referência faria, já que ela devolve booleano calculado da
ausência de exceção.

### Controles negativos EXECUTADOS

1. Confiar na chamada sem reler: `TestADescriptionTheServerIgnoredIsAnError` falha.
2. Inventar o `descId` em vez de lê-lo da metadata: falha.
3. Fundir recusa da página com "não pegou": falha, porque os reparos diferem.
4. Voltar à forma posicional da referência:
   ```
   description_test.go:125: the call does not pass a groupWid field; this build takes one object
   description_test.go:131: the call still uses the reference's positional form
   ```

### Lote classificado com evidência já existente

- `ClientInfo.getBatteryStatus` → `BLOCKED`: `WAWebBatteryStore` não existe
  (medido na H119).
- `Label.getChats` e `Client.getChatsByLabelId` → `BLOCKED`: os 3 rótulos da
  conta têm ZERO itens (medido na H114). Não é falta de código, é falta de dado.

A prosa da família `Label` dizia "não atacado" e estava velha; foi corrigida.

**Status**: parcialmente entregue — código escrito, provado em unidade, e
bloqueado ao vivo por comportamento medido da página.

## H127 — a varredura dos `PARTIAL`, e o que ela achou de errado no meu próprio número

**Data**: 2026-08-22. **Contexto**: o critério da Fase 1 (decisão 52) diz que
`PARTIAL` **acionável** tem de chegar a zero. Isso torna a varredura obrigatória:
sem separar impossibilidade de trabalho pendente, não há como saber o que falta.

**Onde**: `internal/wa-headless/capabilities/lookup/` (novo),
`probe_lookup_test.go` (novo), `LEDGER-WWEBJS.md`.

### O número que eu tinha dado estava errado

Reportei "22 `PARTIAL` com nota curta ou vazia" como estimativa dos acionáveis.
Ao varrer um a um, **7 eram falso positivo do meu próprio contador**: vivem em
tabelas de DUAS colunas, que não têm coluna de nota, e o script leu a coluna de
estado como se fosse a nota. Mais 1 era a própria linha do Placar.

Candidatos reais: **14**. E desses, vários diziam `idem <linha>`, que é
REFERÊNCIA a uma nota vizinha e não ausência de nota.

Registro o erro porque o número circulou antes de ser verificado, e um número
errado sobre "quanto falta" é pior que nenhum — ele vira plano.

### O padrão que a varredura revelou

Cinco linhas compartilhavam a MESMA nota, e ela é a definição de acionável:

> existe como passo interno, não como capacidade exposta

`getChatById`, `getContactById`, `getMessageById`, `getNumberId`,
`getLabelById`. O trabalho estava feito e **inalcançável**: o `send` resolve uma
identidade antes de despachar, e nenhum chamador conseguia fazer a mesma pergunta
sem enviar alguma coisa.

### O que foi entregue

**`getNumberId` → `PROVEN`**, via `capabilities/lookup`. A decisão de desenho que
importa: a expressão de resolução é **EMBUTIDA**, não copiada.
`spa.ResolveIdentityExpr` é o mesmo texto que o `send` executa; colar uma cópia
criaria duas resoluções livres para divergir, e a divergência apareceria como um
envio falhando para um número que este pacote acabara de aprovar. Há teste que
falha se a cópia voltar.

Prova ao vivo com **três** casos, porque um só não distingue o que importa:

```
real peer -> Identity(jid=true group=false resolved=true)
impossible number -> ErrNotOnWhatsApp, as a definite answer
group -> Identity(jid=true group=true resolved=false)
```

O terceiro caso existe porque a resolução responde PESSOAS: sem o curto-circuito,
um grupo real seria reportado como inexistente.

**`getMessageById` → `PROVEN`** sem código novo: `capabilities/message`
(`OriginOf`, `CurrentOf`, `ShapeOf`) recebe o id cru e foi provado ao vivo hoje
(H106, H107, H108). A linha estava desatualizada, não pendente.

### Controles negativos EXECUTADOS

1. Copiar a resolução em vez de embutir a compartilhada:
   `lookup_test.go:122: the script does not embed spa.ResolveIdentityExpr`
   — **precisou de segunda tentativa**: a primeira mutação deixou o import sem
   uso e não compilou, e pela ARMADILHA §3 controle que não compila não prova
   nada.
2. Fundir "não está no WhatsApp" com falha de leitura: falha, porque os reparos
   diferem — um é decisão do chamador, o outro é página quebrada.
3. Ecoar a entrada como se fosse resolução: falha, porque num build LID-first
   receber de volta o que se perguntou significa que o servidor não falou.

**Status**: parcialmente entregue — 2 das 5 linhas do padrão fechadas;
`getChatById`, `getContactById` e `getLabelById` seguem `PARTIAL` acionáveis,
com o caminho agora óbvio.

## H128 — os dois últimos "passo interno", e dois testes meus que não mordiam

**Data**: 2026-08-22. **Contexto**: fechar o padrão que a H127 identificou —
linhas `PARTIAL` cuja nota era "existe como passo interno, não como capacidade
exposta". Eram cinco; a H127 fechou duas.

**Onde**: `capabilities/chats/chats.go` (`ByJID`),
`capabilities/contacts/labels.go` (`LabelByID`), `probe_lookup_test.go`.

### A decisão de desenho, repetida de propósito

Ambas **reusam a leitura já provada** em vez de escrever uma segunda consulta à
página. `ByJID` filtra a projeção de `chats.List`; `LabelByID` filtra
`ListLabels` (H72).

O custo é ler tudo para responder sobre um. É real — esta conta tem 384
conversas — e compra a coisa certa: **uma projeção em vez de duas**. Duas
divergiriam exatamente onde dói, nos campos que ninguém reconfere depois
(arquivado, mudo, somente-leitura), porque quem escreve o caminho de um item
olha para o id e o nome.

### O detalhe que quase virou defeito

`ByJID` **não pode** buscar na lista truncada. `List` trunca por desenho, e uma
busca truncada responde "não existe essa conversa" para uma que apenas ordenou
tarde — a pior resposta errada possível, porque tem cara de fato.

Ao vivo isso ficou visível: a lista devolve **100 de 384**, e o `ByJID` acha a
que ordena por ÚLTIMO e reporta "384 in this session" para um jid ausente.

### Dois testes meus que NÃO mordiam

Os controles negativos pegaram dois testes que eu tinha escrito errado:

1. `TestByJIDDoesNotSearchATruncatedList` comparava **constantes**
   (`allChats > DefaultLimit`) em vez de comportamento. Apontar a chamada para o
   limite truncante não o fazia falhar — a asserção nunca tocava o código. Foi
   reescrito para montar uma página com mais conversas que o limite e pedir a que
   ordena por último.
2. `TestALabelIdThatExistsComesBackWhole` dizia no comentário que testava
   contagem ZERO, e o fixture não tinha nenhum rótulo com zero. O comentário
   descrevia um teste que não existia.

Depois da correção, os dois mordem:

```
chats_test.go:276: ByJID could not find the conversation that sorts last (105 in this session)
labels_test.go:46: LabelByID(count 0): contacts: no label with that id (2 known)
```

**A lição não é "escrevi dois testes ruins".** É que ambos passavam, pareciam
cobrir a regra, e só o controle negativo revelou que não cobriam. Um teste que
passa e não morde é pior que nenhum — ele desencoraja quem viria escrever o
certo.

### O que fica aberto

`getContactById` é a última das cinco. Fica `PARTIAL` acionável: o caminho é o
mesmo dos outros dois, e não foi feito por escolha de escopo, não por
impossibilidade — que é exatamente a distinção que o critério da Fase 1 exige
que o ledger saiba fazer.

**Status**: entregue — 4 das 5 linhas do padrão fechadas.

## H129 — `getContactById`: a pessoa tem dois nomes, e o leitor precisa dos dois

**Data**: 2026-08-22. **Contexto**: a última das cinco linhas do padrão que a
H127 identificou. Fecha o conjunto.

**Onde**: `capabilities/contacts/contacts.go` (`ByJID`), `contacts_test.go`,
`probe_lookup_test.go`.

### A dificuldade real, que não é a busca

Buscar numa lista é trivial. O que não é: neste build LID-first, **a mesma
pessoa chega em duas linhas** — uma sob jid de telefone, outra sob lid — e o
roster as funde numa só. Quem chama pode ter qualquer uma das duas.

Um leitor que comparasse só um campo acharia a pessoa sob um nome e a declararia
inexistente sob o outro. É a mesma classe de defeito que a H34 pegou no envio,
onde a verificação usava o jid de QUEM CHAMOU em vez do que o servidor devolveu.

### Medida ao vivo, sobre o roster de verdade

```
roster: 521 contatos de 945 linhas (421 merged)
merged=413 nameless=255
merged contact reachable under both identities: true
a nameless contact is findable, as it must be
```

**255 dos 521 contatos não têm nome nenhum.** Isso decide outra coisa: devolver
um `Contact` zerado para "não achei" seria indistinguível de metade do roster.
Por isso a ausência é erro, e o erro **reusa** o `ErrNoContact` que já existia
para `CommonGroupsWith` — declarar um segundo com o mesmo significado é como dois
pontos de chamada começam a discordar sobre o que é ausência.

### Controles negativos EXECUTADOS

1. Casar só pelo `PN`:
   `ByJID(lid): no such contact — the person is unreachable under the identity this build actually files them by`
2. Devolver `Contact` zerado em vez de erro: falha.
3. Tratar contato sem nome como ausente: falha — e este é o controle que mais
   importa, porque o caso é a MAIORIA aqui, não a exceção.

### O padrão da H127, fechado

Cinco linhas diziam "existe como passo interno, não como capacidade exposta".
Todas as cinco agora são `PROVEN`: `getNumberId` e `getMessageById` (H127),
`getChatById` e `getLabelById` (H128), `getContactById` (esta).

O que as quatro últimas têm em comum é a decisão de **reusar a leitura provada**
em vez de escrever uma segunda consulta. Não é preguiça: duas projeções da mesma
coisa divergem sempre nos campos que ninguém reconfere — a mescla, o arquivado,
o mudo — e a divergência só aparece quando alguém confia nela.

**Status**: entregue e provado. O padrão está fechado.

## H130 — a varredura dos `PARTIAL`: um `idem` pendurado, um mapeamento errado, e 28 linhas que não podiam carregar evidência

**Data**: 2026-08-22. **Contexto**: o critério da Fase 1 exige que todo `PARTIAL`
sobrevivente tenha veredito MEDIDO registrado. Varrer para verificar isso achou
três problemas de naturezas diferentes.

**Onde**: `LEDGER-WWEBJS.md`.

### Problema 1 — 28 linhas em tabelas que não têm coluna de nota

Nove famílias usavam tabela de DUAS colunas (`upstream | estado`). Nelas o
critério era **inverificável por construção**: não havia onde escrever o
veredito. Pior, eu já tinha medido várias delas (H113, H118, H126) e a evidência
foi para o HOUSEKEEP porque o ledger não a comportava.

Convertidas para três colunas, com as notas que eu já tinha preenchidas de
volta. 28 linhas.

### Problema 2 — um `idem` PENDURADO

`UNREAD_COUNT` dizia `idem`, e a linha imediatamente acima era `MESSAGE_EDIT`,
`PROVEN`, com nota "disparado ao vivo por uma edição". Isso não explica por que
`UNREAD_COUNT` é `PARTIAL` — a referência apontava para o lugar errado.

O veredito real: `chat.changed` DISPARA (H87, e as sondas de hoje o viram às
dezenas), mas é um evento genérico de "campos da conversa se moveram", não o
evento dedicado que o upstream entrega COM a contagem. E o contador em si é
`CROSS_SESSION` (H78).

**A causa é o estilo `idem`**: ele referencia por POSIÇÃO DE LINHA. Reordenar
uma tabela muda silenciosamente o significado de toda nota `idem` abaixo. As
outras três (`sendPresenceUnavailable`, `Chat.delete`, `removeParticipants`)
apontavam para o lugar certo, e mesmo assim foram expandidas para serem
autocontidas — uma referência frágil que hoje está certa é um defeito que ainda
não aconteceu.

### Problema 3 — um mapeamento simplesmente ERRADO

`getQuotedMessage` estava mapeado para "metadados em messagemeta". Fui conferir:
`messagemeta.Meta` tem jid, id, direção, tipo e timestamp, **e nada mais** — por
invariante 12, que proíbe corpo. Não há campo de citação nenhum. A linha
afirmava uma implementação que não existe.

O que existe de verdade: `send.Reply` lê `quotedStanzaID` da página para provar
que a citação foi anexada. O que falta é RESOLVER a mensagem citada — e isso é
**acionável**, não impossível: `message.OriginOf` poderia carregar o id citado.

Uma linha com mapeamento errado é pior que uma `MISSING`: ela consome a atenção
de quem procura trabalho e devolve uma falsa sensação de cobertura.

### Resultado

```
PARTIAL 59 | sem veredito registrado: 0
```

Todas as 59 carregam agora a razão de serem parciais. A distinção que o critério
pede — impossibilidade medida contra trabalho pendente — passou a ser legível
linha a linha, e não mais uma impressão.

**Status**: entregue. Nenhum código de produção mudou; o que mudou é que o
ledger passou a poder responder à pergunta que a Fase 1 faz.

## H131 — `getQuotedMessage`: o instrumento errado contou 110 de 110

**Data**: 2026-08-22. **Contexto**: o único `PARTIAL` que a varredura da H130
classificou como ACIONÁVEL — a linha estava mapeada para algo que não carrega
citação nenhuma.

**Onde**: `capabilities/message/` (`QuotedOf`, `quotedScript`),
`probe_quoted_test.go` (novo).

### O instrumento errado, e por que ele mentiu convincentemente

A primeira sonda procurou campos com "quoted" ou "contextInfo" no nome e contou
os que não fossem nulos. Resultado: **110 de 110 mensagens com citação**.

Isso é falso. Todo modelo de mensagem carrega `__x_fromQuotedMsg`,
`__x_isQuotedMsgAvailable` e `__x_questionReplyQuotedMessage`, e os três guardam
uma **sentinela de getter preguiçoso** — um objeto marcador, não dado. Meu teste
de "não é nulo/vazio" passava na sentinela.

O número era convincente porque era redondo e alto. Se eu tivesse escrito o
leitor em cima dele, ele reportaria que toda mensagem cita alguma coisa, e o
defeito só apareceria quando alguém confiasse na resposta.

Com o campo certo — `quotedStanzaID`, que é o MESMO que o `send` usa para provar
que uma resposta levou a citação:

```
336 mensagens · withQuotedStanzaID: 0 · withQuotedMsg: 0
```

**Zero.** Nenhuma mensagem desta conta cita outra.

### A saída não foi desistir

Zero é a armadilha H93 — embarcar leitor nunca visto devolvendo algo. A saída é
a mesma que fechou `GROUP_UPDATE` (H119) e `VOTE_UPDATE` (H121): **produzir o
fato**. `send.Reply` está provado, então mandei uma resposta e li de volta.

```
plain message -> Quoted(quotes=false ...)
reply         -> Quoted(quotes=true id=true loaded=true sender=true)
quoted message resolved: Origin(chat=true fromMe=true at=true)
```

A perna da mensagem COMUM existe porque sem ela um leitor que dissesse
"quotes=true" para tudo passaria na perna da resposta.

### Duas distinções que o tipo carrega

**"Cita X" e "X está carregada" são fatos diferentes.** Fundi-los diria "não há
citação" sobre uma resposta cujo alvo apenas não hidratou — mentira sobre a
MENSAGEM, não sobre a sessão. É a mesma distinção que a H108 teve de fazer entre
ack ausente e ack zero.

**Não citar nada não é erro.** É a maioria absoluta dos casos aqui, e tratá-lo
como falha faria o caso comum parecer quebrado.

### Controles negativos EXECUTADOS

1. Ler os campos sentinela em vez de `quotedStanzaID`: falha, nomeando os três.
2. Fundir "cita" com "alvo carregado": falha.
3. Tratar "não cita" como erro: falha.

**Status**: entregue e provado ao vivo. Era o último `PARTIAL` acionável que a
varredura da H130 tinha identificado.

## H132 — as duas que os pares provados destravaram

**Data**: 2026-08-22. **Contexto**: a auditoria da H118 classificou
`Contact.getChat` e `GroupNotification.getRecipients` como "(b) delegação com
lógica extra — não herda". Isso estava certo NAQUELE momento: os pares no
`Client` ainda não estavam provados. Depois da H127–H129 estão, e a lógica extra
passou a ser a única coisa faltando.

**Onde**: `capabilities/chats/chats.go` (`OfContact`),
`capabilities/contacts/contacts.go` (`Recipients`).

### `Contact.getChat` — a guarda É a diferença

`ByJID` já existia e estava provado. O que a referência acrescenta é uma linha:
devolver null quando o contato É esta conta (`Contact.js:144`).

Delegar sem ela entregaria a conversa que a página guarda para o self — que
EXISTE e não significa nada para quem chama. E `null` não é "não achei": é "essa
pergunta não se aplica". Fundi-los faria um chamador concluir que a conta não
tem conversa com alguém que ela plainly tem.

A identidade própria é PASSADA, não lida aqui: `capabilities/owner` já responde
"quem sou eu", e um segundo lugar que resolvesse isso seria uma segunda resposta
à mesma pergunta.

Ao vivo, a guarda dispara para as DUAS identidades da conta (pn e lid) — o que
importa porque quem chama pode ter qualquer uma.

### `getRecipients` — o desconhecido volta, não some

A referência usa `Promise.all` sobre os ids, e uma falta vira uma entrada
`undefined` que o chamador precisa notar. Aqui os desconhecidos voltam
SEPARADAMENTE, por jid.

Encurtar a lista em silêncio seria pior que a referência: quem contasse
destinatários teria um número MENOR do que o evento de grupo nomeou, e nada
diria por quê. Ao vivo: pediu 4, achou 3, faltou 1, e a soma bate.

Uma decisão de custo: **uma leitura de roster para todos os ids**, não uma por
id. Chamar `ByJID` por destinatário releria as 945 linhas vezes o tamanho do
grupo, e — pior — deixaria o roster mudar entre dois ids da mesma resposta.

### Controles negativos EXECUTADOS

1. Remover a guarda de self: falha.
2. Descartar destinatários desconhecidos em silêncio: falha.
3. Indexar o roster só pelo `PN`: falha — uma pessoa nomeada por lid não
   resolve, que é a classe de defeito da H34 outra vez.

### O que isto diz sobre a auditoria

A H118 não errou ao dizer "não herda". Ela mediu o estado de então. O que faltava
era o par ficar provado — e quando ficou, a linha virou trabalho pequeno em vez
de trabalho desconhecido. É o valor de um ledger que registra POR QUE algo está
aberto: a resposta muda sozinha quando a dependência fecha.

**Status**: entregue e provado ao vivo.

## H133 — política de reação: uma terceira resposta que não é sim nem não

**Data**: 2026-08-22. **Contexto**: `Channel.setReactionSetting`, atacável
porque o usuário autorizou criar e apagar canal.

**Onde**: `capabilities/channel/` (`SetReactionPolicy`, `reactionScript`,
`ReactionPolicyRaw`), `probe_chanowner_test.go`.

### Medi o oráculo ANTES de escrever a escrita

A escrita passa pelo mesmo `editNewsletterMetadataAction` que a H113 mediu
ACEITANDO uma descrição e nunca guardando. Escrever sem pós-condição repetiria
aquilo, então a primeira pergunta foi: o campo é LEGÍVEL?

Num canal estabelecido (o do usuário), a metadata traz
`newsletterReactionCodesSettingMetadataMixin`. Oráculo existe — então vale
escrever.

### Dois vocabulários que não podem ser comparados entre si

A referência aceita 0/1/2 e mapeia para 3/1/0 antes de enviar
(`Channel.js:186–190`). São vocabulários DIFERENTES, e a verificação tem de
comparar contra o valor de FIO, não contra o código.

Comparar contra o código passaria por acidente em `ReactionsBasic`, onde os dois
coincidem em 1, e falharia nos outros dois. **Certo às vezes é a pior espécie de
errado**, e há teste que trava isso.

### A terceira resposta

Ao vivo, num canal recém-criado, o servidor devolveu `-1` nas três tentativas —
que é o MEU sentinela para "o campo não veio", não um valor.

A metadata de um canal novo **não carrega o mixin de reação**. A pós-condição
não tem o que ler. A escrita pode ter chegado; ninguém pode dizer.

Chamar isso de `ErrNotTaken` seria afirmar que a escrita falhou, quando o que se
sabe é que não dá para saber. Criei `ErrUnverifiable` para exatamente isso — e a
distinção é a terceira ocorrência da mesma classe neste módulo: ack ausente
contra ack zero (H108), descrição ausente contra descrição velha (H126), e agora
esta.

**A regra vale a pena escrever de vez**: sempre que um leitor tem um sentinela
para "não veio", quem o consome precisa tratá-lo ANTES de comparar valores.
Comparar o sentinela é reportar a nossa própria ignorância como resposta do
mundo.

### Controles negativos EXECUTADOS

1. Comparar o código em vez do valor de fio:
   `asked for wire value 3 and the server reports 3` — falha exatamente onde
   deveria, porque 3 é o fio e o código é 0.
2. Confiar na chamada sem reler: falha.
3. Tratar campo ausente como falha da escrita: falha.

**Status**: parcialmente entregue — código escrito e provado em unidade,
`BLOCKED` ao vivo por falta de oráculo num canal que este agente consegue criar.

## H134 — vinte e duas linhas classificadas, e duas conclusões minhas que eram falsas

**Data**: 2026-08-22. **Contexto**: fechar a classificação do que resta, para que
o critério da Fase 1 tenha resposta em cada linha.

**Onde**: `LEDGER-WWEBJS.md`, `probe_gp2_test.go`
(`TestProbeRemainingModules`).

### Uma sonda para vinte linhas

A distinção que a H125 estabeleceu — módulo AUSENTE contra módulo presente sem
dado ou sem segundo participante — merece uma medição, não vinte suposições. Uma
sonda mediu a existência de todos os módulos que as vinte linhas restantes
precisariam.

### Duas conclusões minhas que a segunda passagem derrubou

**1. `Channel.mute` NÃO está bloqueado por módulo ausente.** Testei
`WAWebMuteChatAction`, vi `false`, e quase escrevi "módulo não existe". A
referência usa `WAWebNewsletterUpdateUserSettingJob.updateNewsletterUserSetting`
— que **existe**. Eu tinha testado um nome que inventei a partir do nome da
função, não o que a referência chama.

**2. As notas de cliente NÃO estão bloqueadas por módulo ausente.** Procurei
`addOrEditNote` e `getNote` em `WAWebNoteAction`, ambos `false`. Os nomes reais
são `noteAddAction` e `retrieveOnlyNoteForChatJid` — **exatamente os que a
referência chama**, e ambos existem.

**A regra que sai daqui**: ao medir existência de função, use o nome que a
REFERÊNCIA chama, lido do código dela. Nomes inferidos do nome do método público
produzem falsos negativos que viram vereditos — e um veredito falso de
"impossível" é pior que um `MISSING`, porque encerra a investigação.

O que de fato falta para as notas de cliente é outra coisa, e é mais precisa:
`WAWebBizGatingUtils` (o portão `smbNotesV1Enabled`) **não existe**. Dá para
chamar a ação e não dá para saber se o recurso deveria estar ligado.

### As classificações

- **5 verbos de admin de canal** (×2 famílias): módulos TODOS presentes; o
  bloqueio é de SEGUNDO PARTICIPANTE — exige a conta-B pareada em sessão
  simultânea.
- **`revokeStatusMessage`**: `WAWebRevokeStatusAction` existe; falta status
  postado, e postar é decisão humana (H100).
- **fotos de perfil e de grupo** (4 linhas): `WAWebSetPicture` e
  `WAWebProfilePicThumbBridge` ausentes.
- **`sendResponseToScheduledEvent`**: módulo ausente, coerente com a H125.
- **`Channel.mute`/`unmute`**: módulo presente; bloqueio é de ASSINATURA, que a
  H123 mediu impossível.
- **`Channel.fetchMessages`**: exige canal COM mensagens; um novo não tem, um
  alheio exigiria assinatura.

### O estado do ledger

```
MISSING + PARTIAL: 78 | sem veredito registrado: 0
```

Toda linha aberta diz agora POR QUE está aberta. As últimas seis referências
frágeis (`idem`, e notas que eram só `H53`/`H65`) foram expandidas para serem
autocontidas, pelo mesmo motivo da H130: uma referência que hoje aponta certo é
um defeito que ainda não aconteceu.

**Status**: entregue. Nenhum código de produção mudou.

## H135 — a sessão dupla: a H86 estava certa sobre o ator, e errada como regra geral

**Data**: 2026-08-22. **Contexto**: montar duas contas pareadas e acordadas AO
MESMO TEMPO, para responder o que nenhuma sessão sozinha podia.

**Onde**: `internal/wa-headless/dualsession_test.go` (novo),
`probe_dualgroup_test.go` (novo).

### Por que precisou existir

Todo teste de duas contas deste pacote roda em SEQUÊNCIA — um Holder, uma
sessão, fechada antes da próxima abrir. Isso basta para "o servidor terminou
neste estado" e é inútil para **"o que a OUTRA sessão viu enquanto esta agia"**.

A H86 mediu ZERO evento na sessão que fez a mudança de participante. A H119
provou que o portador `gp2` CHEGA a este barramento. Entre os dois fatos havia
uma pergunta que só duas sessões simultâneas respondem.

### O que a sessão dupla mediu

Com conta-A observando e conta-B saindo e voltando ao grupo de laboratório:

```
after conta-B left:     map[chat.changed:8 group.left:1]   subtypes: map[leave:1]
after conta-B rejoined: map[... group.joined:1 group.left:1] subtypes: map[invite:1 leave:1]
```

**O observador recebe.** A H86 estava certa sobre o ATOR e não valia como regra
geral — e a diferença entre as duas leituras é a diferença entre "este build não
emite" e "este build não emite para quem agiu".

`GROUP_JOIN` e `GROUP_LEAVE` passam a `PROVEN`.

### E a terceira NÃO chega

Invertendo os lados — barramento em conta-B, conta-A promovendo e rebaixando —
**zero** `group.admin_changed`, na mesma montagem que acabara de entregar leave e
join. A diferença é do SUBTIPO, não do barramento, e isso agora está medido em
vez de presumido.

### O achado que eu não estava procurando

O observador viu subtipos `gp2` que **não estão na tabela da referência**:

```
biz_account_type_changed_to_hosted, biz_privacy_mode_to_fb, block_contact,
change_number, disappearing_mode, encrypt_now, ephemeral_setting, modify,
sender, url
```

A referência lista `add`, `invite`, `linked_group_join`, `remove`, `leave`,
`promote`, `demote` e joga todo o resto num `else` que vira `GROUP_UPDATE`.
Este build emite pelo menos dez subtipos que ela não nomeia.

**Foi a decisão da H119 que os tornou visíveis.** Lá eu recusei o `else` que
engole desconhecido, escrevendo que "um subtipo que a Meta acrescente amanhã
chegaria rotulado errado, em silêncio". Não era hipótese: são dez, hoje, e eles
aparecem porque ficaram como `MessageAdded` com o `subtype` intacto em vez de
serem rebatizados de `GROUP_UPDATE`.

`modify` é candidato natural a ser a mudança de admin deste build — mas apareceu
UMA vez para DUAS ações (promover e rebaixar), então é hipótese e está escrita
como tal, não como veredito.

### Cuidados que a montagem exige, e por que estão no código

- **Perfis e portas diferentes**, verificados: perfil repetido é uma conta, não
  duas, e o harness falha alto em vez de dirigir a mesma sessão duas vezes.
- **Fechar o que já subiu antes de falhar**: uma sessão dupla meio aberta vaza um
  Chrome que sobrevive ao binário de teste, e este pacote já pagou por Chromes
  órfãos a sessão inteira.
- **O código de convite vem do lado ADMIN.** Custou uma execução: conta-B é
  membro comum e ler convite exige admin. O observador é o admin aqui.
- **Desfazer registrado ANTES de agir**: o rejoin antes do leave, o demote antes
  do promote. Uma execução que morra no meio não pode deixar conta-B fora do
  grupo nem admin.

**Status**: entregue — duas linhas provadas, uma medida como não observável, e o
harness fica para as próximas.

## H136 — a cadeia de admin de canal: medir a corrente inteira antes de forjar cada elo

**Data**: 2026-08-22. **Contexto**: cinco linhas que a H134 tinha classificado
como travadas **só** por segundo participante. Com a sessão dupla da H135, elas
deixaram de estar travadas.

**Onde**: `internal/wa-headless/probe_chanadmin_test.go` (novo).

### Por que uma sonda antes de cinco capacidades

Os cinco verbos são SEQUENCIAIS: nada depois do passo dois pode ser medido se o
passo dois falhar. Escrever cinco capacidades primeiro e descobrir isso seriam
cinco pedaços de trabalho jogados fora. A sonda exercita as chamadas CRUAS em
ordem e cada passo reporta o que fez.

### Três coisas que eu errei antes de acertar

**1. `Chat.find` está quebrado** — `this.findImpl is not a function`, o mesmo
defeito que a H123 encontrou na coleção de newsletters. Não é específico de
canais: é das coleções deste build.

**2. Procurei o chat pelo jid de TELEFONE.** Nem `get` nem varredura acharam,
porque este build arquiva sob LID. É a lição da H34 num lugar novo, e a correção
foi usar `lookup.NumberID` — a resolução do próprio módulo, provada hoje na
H127 — em vez de escrever uma segunda.

**3. Testei a revogação DEPOIS do aceite**, e o servidor respondeu
`Not Allowed`. Essa é a resposta CERTA para um convite já consumido e a medição
ERRADA da capacidade. Revogar só faz sentido antes.

### O que ficou provado, e por qual pós-condição

```
ordem A (aceitar):  invite OK -> accept ok  -> assinantes 0 -> 1
ordem B (revogar):  invite OK -> revoke ok  -> accept "Not Found", assinantes 0
```

- **`sendChannelAdminInvite`**: a página responde `messageSendResult: OK`, e o
  convite é ACEITÁVEL — provado pelo passo seguinte funcionar, não pelo retorno.
- **`acceptChannelAdminInvite`**: o canal vai de **0 para 1 assinante**.
- **`revokeChannelAdminInvite`**: o aceite seguinte **FALHA** com `Not Found` e
  os assinantes ficam em 0.

A revogação é provada pelo aceite falhar. Se eu tivesse aceitado "a chamada não
lançou" como prova, teria aprovado uma revogação que na primeira ordem o próprio
servidor recusou.

A sonda guarda as DUAS ordens atrás de uma chave, porque uma execução só não
prova as duas — e ela FALHA se o aceite funcionar depois de uma revogação, que é
o sucesso silencioso que ela existe para pegar.

### Duas que continuam abertas, com o motivo exato

- **`demoteChannelAdmin`**: a cadeia foi exercitada e para aqui. Wid dá
  `Data passed to getter must include an id property`; o MODELO da coleção vai
  mais longe e morre em `Cannot read properties of undefined (reading 'isUser')`.
  A forma do SEGUNDO argumento não foi resolvida — é trabalho, não impossibilidade.
- **`transferChannelOwnership`**: a ação existe e **não foi executada de
  propósito**. Transferir a posse faria conta-A deixar de ser dona e não
  conseguir mais apagar o canal, deixando uma entidade real de pé. Recusa
  deliberada, escrita como tal.

**Status**: entregue — 3 de 5 provadas, 2 abertas com a causa isolada.

## H137 — o demote: três nomes errados, e a única saída foi perguntar ao módulo

**Data**: 2026-08-22. **Contexto**: `demoteChannelAdmin`, a linha que a H136
deixou aberta com "a forma do segundo argumento não foi resolvida".

**Onde**: `internal/wa-headless/probe_chanadmin_test.go`.

### Três tentativas, três formas de errar o mesmo alvo

1. `demoteNewsletterAdminAction(wid, [wid])` →
   `Data passed to getter must include an id property`.
2. `demoteNewsletterAdminAction(modelo, [wid])` →
   `Cannot read properties of undefined (reading 'isUser')`.
3. `demoteNewsletterAdmin(idString, wid)` — o nome que a **referência** chama
   (`Client.js:1905`) → `is not a function`.

A terceira dói mais que as outras duas: eu tinha escrito a regra da H134 há
poucas horas — *"use o nome que a REFERÊNCIA chama, lido do código dela"* — e ela
não bastou. A referência chama um nome que **este build não tem**.

### A saída foi parar de adivinhar

```
names:  [demoteNewsletterAdminAction]
arity:  {demoteNewsletterAdminAction: 2}
via:    model+contact   ->  ok
```

O módulo tem **uma** função, com aridade 2, e as formas são o **modelo do canal**
e o **modelo do contato** — nem id, nem Wid, nem array.

**A regra da H134 fica corrigida, não descartada**: ler o nome na referência é o
primeiro passo e não o último. Quando ele falha, o passo seguinte é ENUMERAR o
módulo — `Object.keys` mais aridade — em vez de tentar uma quarta variação. Três
tentativas custaram três execuções ao vivo; a enumeração custou uma.

### E mesmo assim não é `PROVEN`

A chamada responde ok. Verificar que o rebaixamento ACONTECEU exigiria ler a
lista de administradores, e `WAWebMexFetchNewsletterSubscribersJob` — o módulo
que a referência usa para isso — não existe neste build (H113).

Então a linha fica `BLOCKED` com a forma resolvida e a pós-condição ausente. É a
mesma situação da H133 com a política de reação: **a chamada funcionar não é a
coisa acontecer**, e este módulo não assina a diferença.

**Status**: parcialmente entregue — forma resolvida e registrada, prova
impossível por falta de oráculo.

## H138 — a coleção que não busca: uma causa por baixo de cinco linhas

**Data**: 2026-08-22. **Contexto**: aplicar a lição da H137 — enumerar em vez de
adivinhar — aos módulos cujas linhas eu tinha classificado a partir de uma
verificação por NOME de função.

**Onde**: `internal/wa-headless/probe_gp2_test.go`
(`TestProbeEnumerateModules`), `probe_follow_test.go`.

### A enumeração achou uma função que a referência não chama

`WAWebNewsletterSubscribeAction` exporta **duas**:

```
subscribeToNewsletterAction     arity 3   <- a que a referência chama
subscribeToNewsletterWidAction  arity 2   <- ninguém tinha olhado
```

A H123 concluiu "assinar é impossível" olhando só a primeira, que quer um modelo
memoizado que a busca quebrada não produz. A segunda aceita um **Wid**, que é
exatamente o que quem tem um código de convite consegue construir.

### E ela falhou de um jeito mais informativo

```
subscribeToNewsletterWidAction(wid, {eventSurface:3})
  -> t.markFetchStart is not a function
```

Erro NOVO. A função aceitou o argumento e morreu **dentro**, num método da
coleção que não existe.

Somando com o que já sabíamos:

```
NewsletterCollection.find  -> findImpl ausente        (H123)
Chat.find                  -> findImpl ausente        (H136)
subscribe by wid           -> markFetchStart ausente  (H138)
```

**A conclusão da H123 fica de pé e a CAUSA fica nomeada**: não é a assinatura que
falta, é a coleção que **não busca**. Este build entrega as coleções sem a
maquinaria de fetch, e tudo que exige "traga um canal que eu não tenho" morre no
mesmo buraco — assinar, silenciar, buscar mensagens.

Isso vale mais que as duas linhas que atualiza. Diz à próxima pessoa o que
verificar num build futuro: **um único** método de coleção voltando destrava
cinco linhas de uma vez, e testar cada uma separadamente seria cinco descobertas
do mesmo fato.

### Confirmação de passagem

A enumeração também confirmou a H125 pelo caminho certo: `WAWebGroupInviteV4Job`
exporta **apenas** `revokeGroupInviteV4`. Não há aceitação de convite v4 neste
build — antes eu sabia que os dois nomes que procurei não existiam, agora sei que
não existe nenhum.

**Status**: entregue — causa isolada, duas linhas com veredito mais preciso, e
uma confirmação que antes era ausência de evidência.
