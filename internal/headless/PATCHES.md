# Patches locais em internal/headless/

Registro do que divergimos da implementação de referência
[whatsapp-web.js](https://github.com/wwebjs/whatsapp-web.js) (`wwebjs`) e por
quê.

`internal/headless/` é a segunda pilha de transporte do projeto. Enquanto
`internal/noise/` fala o protocolo direto no WebSocket com o Noise
framework, este módulo conduz o **SPA real** do `web.whatsapp.com` dentro de um
browser, que é o que certos recursos e campanhas exigem.

A segunda motivação — a camada adicional de mitigação de ban — **não é
propriedade automática desta pilha**. O spike do
[ADR-0006](../../docs/adr/0006-headless-engine-de-browser-e-custo-por-sessao.md)
mediu `navigator.webdriver === true` nas duas engines avaliadas, e a D3 daquela
ADR transforma a mitigação em frente de trabalho própria, com medição própria.
Até que ela exista, esta pilha entrega o primeiro motivo e não o segundo. As duas
pilhas
convivem: a `wa-api` é a ponte, e o consumo desta biblioteca acontece em
`pkg/infra/headless/`, exatamente como `pkg/infra/noise/` consome a
outra.

---

## Por que este arquivo existe aqui, se não há upstream para rebasear

No `internal/noise/` o `PATCHES.md` é mecanismo de reconciliação contra
versões futuras do upstream: existe um ancestral de código em Go, e existe
`git diff` contra ele. **Aqui não existe nenhum dos dois.** O `wwebjs` é
JavaScript, roda sobre Puppeteer, e não é ancestral deste código — é
implementação de referência. Nada será rebaseado a partir dele.

O arquivo continua sendo necessário, pelo motivo que o `CLAUDE.md` já fixa em
"Consultar as implementações de referência antes de resolver": divergir da
referência é aceitável, mas **tem de ser decisão consciente e registrada**,
dizendo o que eles fazem, o que nós fazemos e por que a diferença. Divergir
sem saber que se está divergindo é como se reescrevem os bugs deles junto.

O que muda é o **gatilho da reconciliação**. Não é o rebase; são estes dois:

1. **O `wwebjs` mudar** o comportamento que uma entrada nossa espelhou ou
   rejeitou — release, commit de correção, issue que revela que a forma antiga
   estava errada.
2. **O `web.whatsapp.com` mudar** e quebrar aquilo em que a divergência se
   apoiava — a superfície que este módulo dirige é a de um SPA que a Meta
   altera sem aviso.

Por isso toda entrada precisa **ancorar o ponto de referência**: qual arquivo
do `wwebjs`, em qual commit ou tag, e qual versão do WA Web estava em campo
quando medimos. Entrada sem âncora não é reconciliável — daqui a três meses
ninguém sabe contra o que ela divergiu.

## O que conta como entrada

Divergência **deliberada** de comportamento, arquitetura ou contrato em
relação ao `wwebjs`. Os casos que já se sabe que virão:

- **Escala e multi-cluster.** O `wwebjs` foi medido em produção e não sustenta
  o cenário de N réplicas; a posse de sessão aqui segue o ADR-0005 (lease com
  TTL), que é decisão nossa e não tem equivalente lá.
- **Resiliência e ciclo de vida do browser.** Consumo de RAM do browser mais o
  SPA é o custo central desta pilha; qualquer política de reciclagem, de teto
  de instâncias ou de degradação é divergência a registrar.
- **Fronteira com a `wa-api`.** Envelope de erro, taxonomia `apperr`, logging
  estruturado e os demais pilares do repositório valem para dentro deste
  diretório. Onde isso obrigar a uma forma diferente da do `wwebjs`, é entrada.
- **Recusa consciente.** Comportamento que o `wwebjs` tem e que nós
  **decidimos não ter** também é divergência, e some da memória mais rápido do
  que a divergência positiva.

Não conta: escolha de implementação sem efeito observável, e diferença que é
apenas consequência de Go não ser JavaScript.

## Formato da entrada

Herdado do `internal/noise/PATCHES.md`, com um campo a mais e um padrão
invertido. Cada entrada diz:

- **quais arquivos** — os nossos, com caminho;
- **âncora da referência** — arquivo do `wwebjs` + commit/tag, e a versão do
  WA Web observada;
- **o que mudou** em relação à referência;
- **por quê**;
- **se o comportamento mudou** — e aqui está a inversão. No `noise` o
  padrão da Fase A era "comportamento não mudou", porque aquilo era refatoração
  de código herdado. Aqui o padrão é o oposto: a entrada existe **porque** o
  comportamento diverge. Uma entrada que afirme não divergir precisa explicar
  por que foi registrada.

## Fora do escopo

- **`internal/noise/`** — outra pilha, outro `PATCHES.md`. Divergência de
  uma não é divergência da outra, e comparar as duas entre si não é o assunto
  deste arquivo.
- **Achados incidentais** — bug, gap ou dívida encontrados de lado vão para
  `internal/headless/HOUSEKEEP.md`, não para cá. `PATCHES.md` registra o que
  decidimos; `HOUSEKEEP.md` registra o que descobrimos.
- **Decisão de arquitetura** — vai para uma ADR em `docs/adr/`. Este arquivo
  aponta para a ADR, não a substitui.
- **Código gerado**, se vier a existir. O `.gitattributes` deste diretório hoje
  é herança da cópia do `noise` e marca `*.pb.go`, `*.pb.raw` e
  `internals.go` — nenhum dos três existe aqui.

## Relação com os outros registros

| arquivo | responde |
|---|---|
| `PATCHES.md` (este) | em que divergimos do `wwebjs`, e por quê |
| `internal/headless/HOUSEKEEP.md` | o que foi descoberto de lado nesta biblioteca |
| `HOUSEKEEP.md` (raiz) | o mesmo, para a aplicação `wa-api` |
| `docs/adr/` | as decisões que valem para o projeto inteiro |
| `ARMADILHAS.md` | os defeitos que já passaram por testes verdes |

## Gates

Este diretório **não** herda a posição do `internal/noise/` nos gates, e a
diferença é a favor dele. O `COVER_PKGS` do `Makefile` exclui por prefixo
apenas `wa-api/internal/noise`; `wa-api/internal/headless` não casa com
esse filtro. Consequência medida em 2026-08-08 (`go list ./...`):

| gate | cobre `internal/headless/`? |
|---|---|
| `vet`, `lint` (`ALL_PKGS` / `LINT_TARGETS` = `./...`) | **sim** |
| `test`, `coverage-gate` (`TEST_PKGS := COVER_PKGS`) | **sim** |
| `waclient-facade` | **sim** — como consumidor externo do fork |
| `waclient-filesize` (teto de 300 linhas) | não |
| `waclient-test`, `waclient-drift` | não |

Duas consequências práticas que valem antes da primeira linha de código:

1. **Estamos dentro do denominador da cobertura.** Código novo aqui sem teste
   derruba o número e **falha o `coverage-gate`** — que é catraca, falha quando
   o valor cai. Não é aviso; é build quebrado.
2. **O teto de 300 linhas não trava aqui.** Ele vale por disciplina (ADR-0004,
   decisão 2, que fixa o pilar para o repositório), mas nenhum script o
   verifica neste caminho. Enquanto for assim, é revisão que segura — ver "Em
   aberto".

O `waclient-facade` merece leitura atenta: ele falha se qualquer `.go` **fora**
de `internal/noise/` importar `wa-api/internal/noise/core` direto. Se
esta pilha vier a reaproveitar algo da outra, o import é pela fachada
`internal/noise` (o `main.go` dela), nunca por baixo.

## Entradas

### P1 — A superfície de módulos é inventariada e verificada, não espalhada

- **Nossos arquivos**: `internal/headless/spa/` (o inventário e a
  verificação de arranque). Ainda sem implementação — a divergência está
  decidida e registrada antes do código, e a entrada passa a citar os arquivos
  concretos quando eles existirem.
- **Âncora da referência**: `wwebjs` em `main`, commit `942d236a11ad`
  (2026-07-27), `src/Client.js` e `src/util/Injected/Utils.js`. WA Web
  observado: Chrome 151 contra `web.whatsapp.com` em 2026-08-08, pré-login.
- **O que a referência faz**: acessa os internos por
  `window.require('<NomeDoModulo>')` com o nome **literal no ponto de uso** —
  50 chamadas só no `Client.js`, com nomes como `WAWebChatGetters`,
  `WAWebMsgKey`, `WAWebConnModel`, `WAWebCollections`. Não usa `moduleRaid` nem
  `webpackChunkwhatsapp_web_client`; ambos estão ausentes do projeto nesse
  commit.
- **O que nós fazemos**: um inventário único de nomes como constantes nomeadas,
  resolvido por inteiro no arranque da sessão, falhando alto com a lista do que
  faltou. Zero literal no ponto de uso.
- **Por quê**: os nomes são contrato não documentado da Meta e mudam sem aviso.
  Espalhados, uma renomeação quebra no meio de uma operação com erro que não
  denuncia a causa. Concentrados e verificados, ela quebra no arranque, num
  lugar previsível, dizendo qual módulo sumiu.
- **O comportamento mudou?** **Sim, deliberadamente** — é onde a falha acontece
  e o que ela informa. A renomeação continua quebrando: isto não é resiliência
  contra a mudança, é diagnóstico dela.
- **Decidido em**: [ADR-0006](../../docs/adr/0006-headless-engine-de-browser-e-custo-por-sessao.md) D4.

> **Nota sobre a procedência desta entrada.** A primeira redação do ADR-0006
> afirmava que o `wwebjs` fixava `webpackChunkwhatsapp_web_client` via
> `moduleRaid`. Estava errada, e foi afirmação de memória sobre projeto de
> terceiro dentro de um documento do repositório. A leitura da fonte no commit
> acima desfez o engano — e revelou que o acoplamento real é **mais largo** do
> que o suposto: dezenas de nomes, não um. É a razão de este arquivo exigir
> âncora em vez de aceitar "o que a referência faz" de cabeça.

## Em aberto

- **A ADR existe, mas está `proposed`.** O
  [ADR-0006](../../docs/adr/0006-headless-engine-de-browser-e-custo-por-sessao.md)
  fixa a engine (`chromedp`), o orçamento de flags, o custo medido por sessão e
  como a posse de sessão do ADR-0005 se aplica a um browser vivo. Enquanto não
  for aceita, o esqueleto de pacotes deste diretório é forma, não compromisso.
- **O teto de 300 linhas não é verificado aqui.** `waclient-filesize` casa com
  `internal/noise/`; ou o script passa a aceitar os dois caminhos, ou o
  pilar vale só enquanto a revisão lembrar dele. Ver "Gates".
- **Os números do `wwebjs` não estão no repositório.** A motivação registrada
  ("não performa em multi-cluster", "resiliência aquém") vem de dias de uso em
  produção, mas nenhuma medição foi trazida para cá. Pelo "Medir antes de
  projetar" do `CLAUDE.md`, superar a referência exige o número dela primeiro:
  sem a linha de base, qualquer alegação de superioridade é especulação.
