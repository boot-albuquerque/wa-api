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

**Correção sugerida**: no primeiro boot contra alvo real (CAP-03 LOOP 03.4),
matar o Chromium e inspecionar o perfil:

```
ls -la <perfil>/SingletonLock   # esperado: symlink -> <host>-<pid>
```

Colar a saída aqui e, se divergir, ajustar o parser E o dublê juntos — dublê
mais permissivo que a produção esconde defeito (ARMADILHAS §1).

**Status**: **não corrigido** — premissa documentada, verificação agendada
para o LOOP 03.4. O comportamento sob divergência é degradação para o
comportamento do estudo, não quebra.

**Nota de 2026-08-12**: a verificação continua pendente e o LOOP 03.4 não a
produz de graça. O perfil pareado foi aberto e fechado três vezes nesta data
(M3 do `EVIDENCIA-SPA.md`) e **não havia `SingletonLock` nem antes nem depois**
— o desligamento limpo o remove. Para medir o FORMATO é preciso inspecionar
com o Chromium **em execução**, ou depois de uma parada suja; um perfil parado
de forma limpa nunca vai mostrar o arquivo. A H4 segue aberta com o método
corrigido.

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
