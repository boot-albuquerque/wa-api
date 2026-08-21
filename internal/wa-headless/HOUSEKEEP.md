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
