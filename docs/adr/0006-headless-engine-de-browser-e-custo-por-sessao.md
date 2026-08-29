# ADR-0006: `headless` — engine de browser, custo por sessão e o que o spike mediu

- **Status**: superseded em parte pelo
  [ADR-0007](0007-decisao-final-headless-escopo-e-custo.md), que fecha a
  decisão com a base empírica das Fases 1–5 e responde as perguntas em aberto
  D6/D7 daqui. O que permanece válido: D1 (engine), D2 (flags), D3 (anti-ban
  como frente própria), D4 (inventário de módulos), D5 (lease). O que mudou:
  o custo por sessão pareada e o escopo da pilha.
- **Data**: 2026-08-08
- **Relacionado**: `internal/headless/PATCHES.md` (divergência contra o
  `wwebjs`), `internal/headless/HOUSEKEEP.md`. **Depende** do
  [ADR-0005](0005-obrigatoriedade-de-stack-e-posse-de-sessao.md) D2, cuja posse
  de sessão por lease vale aqui com um dimensionamento diferente — ver D5.

## Contexto

O projeto dispara mensagens pelo protocolo direto, no WebSocket com o Noise
framework (`internal/noise/`). Esta ADR abre a **segunda pilha**: conduzir o
SPA real do `web.whatsapp.com` dentro de um browser.

Ela existe por duas razões, e as duas vêm de dias de uso do
[whatsapp-web.js](https://github.com/wwebjs/whatsapp-web.js) em produção:

1. **Recursos e campanhas que exigem o SPA de verdade**, não a reimplementação
   do protocolo.
2. **Uma camada adicional de mitigação de ban.** Mitigação, não garantia — e
   esta ADR trata a distinção como material, ver D3.

O custo conhecido é a RAM: browser mais SPA. É esse custo que precisava de
número antes de qualquer escolha, e é o que o spike foi medir.

**O que esta ADR não pode decidir.** O baseline do `wwebjs` em produção não foi
trazido para o repositório — foi decisão explícita do dono do projeto ao definir
o escopo da fase 0. Sem ele, "superar o `wwebjs` em escala e resiliência"
permanece **objetivo, não critério**: não há número da referência contra o qual
declarar superioridade. As decisões abaixo escolhem engine e forma; nenhuma
afirma vantagem comparativa.

## O que foi MEDIDO (spike, 2026-08-08)

**Método.** Um programa Go descartável, fora do repositório, com o **mesmo
probe** nas duas engines — para que a comparação medisse a engine e não a
instrumentação. Memória atribuída por `--user-data-dir` único, somando o RSS de
toda a árvore de processos daquele perfil, idêntico nas duas.

**Ambiente.** macOS 15 arm64, Google Chrome 151, Go 1.25.12, headless,
`web.whatsapp.com` **pré-login** (tela de QR). 13 corridas válidas.

| medida | chromedp v0.16.0 | rod v0.116.2 |
|---|---|---|
| RSS por sessão | **741–790 MB** (6 corridas) | **474–487 MB** (5 corridas) |
| processos | 9 | 6 |
| tempo até o SPA subir | 6,1–7,7 s | 7,1–8,5 s |
| 3 sessões simultâneas | 2336 MB, sem degradação | 1429 MB, sem degradação |
| sobrevive a reload | sim, re-pronto em ~14 s | sim, re-pronto em ~14 s |
| round-trip de `Evaluate` | 0,57–9,1 ms — **ruído, não diferencial** | 0,8–2,6 ms |

**A superfície de integração existe, e não é a que eu supus.** O probe achou
duas coisas em todas as corridas válidas: `webpackChunkwhatsapp_web_client` e o
conjunto `require`, `requireLazy`, `requireInterop`, `requireDynamic`. A
primeira é a que a literatura sobre automação de WhatsApp cita mais; **a que
importa é a segunda**.

Conferido na fonte, e não de memória — `wwebjs` em `main`, commit
`942d236a11ad` (2026-07-27): não há `moduleRaid` nem referência a
`webpackChunkwhatsapp_web_client` no projeto. O acesso aos internos é
`window.require('<NomeDoModulo>')`, com nomes como `WAWebChatGetters`,
`WAWebMsgKey`, `WAWebConnModel`, `WAWebCollections`. Só o `Client.js` faz **50**
dessas chamadas.

Isso muda o desenho, e para pior no eixo da fragilidade: o acoplamento com o
SPA não é **um** nome de chunk, é uma **superfície de dezenas de nomes de
módulo**, cada um dos quais a Meta pode renomear independentemente. Ver D4.

O SPA sobe **headless**, sem browser visível, e sobrevive a reload sem perder
nem o `require` nem o chunk.

### A diferença de RAM é de FLAGS, não de biblioteca

O achado que muda a decisão. Os ~300 MB entre as engines pareciam propriedade
da biblioteca. Não são: o `rod` passa por padrão
`--disable-site-isolation-trials` e `--disable-component-extensions-with-background-pages`,
que o `chromedp` não passa.

Dando as mesmas flags ao `chromedp`:

| configuração | RSS | processos |
|---|---|---|
| `chromedp` com os padrões dele | 790 MB | 9 |
| `chromedp` + as 3 flags do `rod` | **592 MB** | **6** |
| `rod` com os padrões dele | 475 MB | 6 |

A contagem de processos igualou; **198 MB dos ~300 são configuração que
controlamos**. O resíduo de ~110 MB não foi atribuído — as engines ainda diferem
em outras flags, e `--no-startup-window` (que o `rod` usa) é incompatível com o
`chromedp`, que precisa do target inicial para anexar.

Consequência: **RAM por sessão é orçamento de flags, não propriedade da
biblioteca.** Escolher engine pela RAM medida nos padrões teria sido escolher
pelo default de outra pessoa.

### Detecção: nenhuma das duas é discreta por padrão

`navigator.webdriver === true` nas **duas** engines, em todas as corridas. É a
checagem mais barata que existe do lado do servidor.

O `rod` ainda mascara o User-Agent para `Chrome/114.0.0.0` enquanto o binário
real é o 151. Um UA **defasado em 37 versões** é, ele próprio, uma assinatura —
provavelmente pior que o `HeadlessChrome/151` honesto do `chromedp`, porque
ninguém navega com um Chrome de duas gerações atrás enquanto reporta APIs novas.

### Dois erros do próprio instrumento, registrados

Ambos produziram resultado que parecia achado sobre o alvo (ARMADILHAS §13, "o
seu método de medição também é suspeito"):

1. **A primeira corrida do `rod` reportou página vazia** e "SPA hook absent". Era
   bug meu: `page.Eval` do `rod` recebe uma *função* e a invoca, e eu passava uma
   IIFE. O `waitReady` engolia o erro do eval, então "o probe nunca executou"
   e "o SPA não subiu" saíam idênticos. Corrigido, e o erro do eval passou a ser
   propagado.
2. **A primeira tentativa de replicar as flags** partia a lista por vírgula, e
   valores de flag do Chrome são listas separadas por vírgula
   (`--disable-features=a,b`). Resultado: um `--TranslateUI` inexistente e a
   medição de 685 MB descartada.

## Decisão

### D1 — A engine é `chromedp`

Não pela RAM, que a medição mostrou ser de flags. Pelo que resta depois disso:
**quando cada default é uma assinatura, a engine certa é a que não decide por
nós.**

O `rod` troca o UA silenciosamente por um defasado. É um default útil para
scraping genérico e é exatamente o tipo de comportamento que não queremos numa
pilha cuja razão de existir é a superfície de detecção. O `chromedp` também tem
defaults opinativos — mas eles são sobrescrevíveis, e nós provamos isso na
medição ao levar 790 MB para 592 MB.

Registrado a favor do `rod`, porque a decisão não é óbvia: a API dele é mais
ergonômica e existe ecossistema de stealth em volta. Se D3 exigir esse
ecossistema, esta decisão é reavaliável — com medição, não por preferência.

### D2 — O conjunto de flags é decisão explícita e medida

Nenhuma flag entra por ser default de biblioteca. O conjunto vive em constante
nomeada no nosso código, cada uma com o porquê ao lado, e mudança nele exige
nova medição de RSS.

`--disable-site-isolation-trials` entra nessa conta com uma ressalva que a
medição não resolve: isolamento de site é **fronteira de segurança** do browser.
Desligá-la para ganhar ~200 MB é troca legítima, mas tem de ser consciente — é
precisamente o tipo de decisão que herdamos sem perceber se seguíssemos o
default do `rod`.

### D3 — A camada anti-ban não vem da escolha de biblioteca

O spike mediu `navigator.webdriver === true` nas duas engines. **A premissa que
justifica esta pilha não é entregue por nenhuma das duas de graça.**

Portanto: a mitigação de detecção é **frente de trabalho própria**, com medição
própria, e não um efeito colateral de ter escolhido um browser. Enquanto ela não
existir, a pilha entrega o item 1 do contexto (recursos que exigem o SPA real) e
**não** o item 2.

Isto amenda a expectativa registrada no `PATCHES.md`, que tratava a camada
adicional anti-ban como característica da pilha.

### D4 — A superfície de módulos é inventariada e verificada no arranque

A integração é por `window.require('<NomeDoModulo>')`. Os nomes são contrato
não documentado da Meta, e são muitos: 50 chamadas só no `Client.js` do
`wwebjs`.

O `wwebjs` os espalha pelo código, literal a literal, no ponto de uso. A
consequência é o modo de falha que se vê em campo: a Meta renomeia um módulo,
e o projeto quebra **no meio de uma operação**, com erro que não diz que o
problema é de acoplamento com o SPA.

Nós divergimos em três pontos, e esta é a primeira divergência real a registrar
em `PATCHES.md`:

1. **Inventário único.** Todo nome de módulo vive num só lugar, como constante
   nomeada, com o que ele fornece ao lado. Zero literal no ponto de uso — o que
   já é pilar do repositório ("zero string literal solta", `CLAUDE.md`).
2. **Verificação no arranque da sessão.** Ao subir uma sessão, resolve-se o
   inventário inteiro e falha-se **alto** se algum nome sumiu, com a lista do
   que faltou. É a diferença entre "a sessão não subiu porque o módulo X
   mudou" e uma exceção no meio de um disparo.
3. **Nada de fixar o nome do chunk.** `webpackChunkwhatsapp_web_client` não
   entra no código como igualdade de string. Se algum dia for preciso, a
   descoberta enumera as chaves de `window` que casam com o padrão e usa o que
   achar.

O que isto **não** resolve: renomeação continua quebrando. O ganho é que ela
quebra num lugar previsível, no arranque, com mensagem que nomeia a causa.

### D5 — A posse de sessão do ADR-0005 vale aqui, com outro dimensionamento

O lease com TTL do ADR-0005 D2 se aplica sem mudança de mecanismo: uma sessão
tem um dono, o dono renova, o TTL cobre a morte abrupta.

O que muda é o número. O ADR-0005 dimensionou TTL de 15 s a partir de uma
retomada de 1,2–2,3 s, com margem de 3×. **Aqui a retomada não é essa**: o spike
mediu 6,1–8,5 s só para o SPA subir, pré-login e sem restaurar sessão. Um TTL de
15 s dá margem de menos de 2× contra o boot, antes de somar o restore.

Logo o TTL do lease de uma sessão headless é **parâmetro próprio**, não o mesmo
da pilha Noise, e o número final depende de uma medição que ainda falta: quanto
custa subir uma sessão **já pareada**, com histórico.

### D6 — Capacidade por host se declara a partir do piso medido, e ele é piso

474–790 MB por sessão é **pré-login**. Uma sessão pareada, com histórico
carregado, custa mais — quanto, não foi medido.

Nenhum dimensionamento de capacidade pode usar esses números como se fossem o
custo operacional. Eles servem para uma coisa só: dizer que a ordem de grandeza
é **centenas de MB por sessão**, e que um host de 16 GB fica na casa das dezenas
de sessões, não das centenas.

### D7 — O gate de tamanho passa a cobrir este diretório

`waclient-filesize` casa com `internal/noise/`. `internal/headless/` já
está dentro de `vet`, `lint`, `test` e `coverage-gate` (por não casar com o
filtro de exclusão do `COVER_PKGS`), mas o teto de 300 linhas não é verificado
aqui. Ou o script aceita os dois caminhos, ou o pilar vale só enquanto a revisão
lembrar dele.

## Consequências

**Boas.** A pilha tem número antes de ter código. A escolha de engine deixou de
depender de uma comparação de RAM que teria sido enganosa. O custo por sessão
tem ordem de grandeza declarada, e a premissa anti-ban virou frente explícita
com dono, em vez de expectativa difusa.

**Custos.** `chromedp` exige que nós montemos o que o `rod` daria pronto. O
conjunto de flags vira artefato a manter e a re-medir. E D3 admite que a
justificativa mais forte desta pilha ainda não está entregue.

**Não resolvido aqui.** O baseline do `wwebjs`. O custo de RAM de uma sessão
pareada com histórico. O TTL do lease headless. E se estes números de macOS
arm64 valem em contêiner Linux, que é onde isto vai rodar — a medição foi feita
onde dava para medir hoje, e trocar de sistema operacional troca o alocador, o
modelo de processos e o custo do Chrome.

## Perguntas em aberto para validação

- Quanto RSS custa uma sessão **pareada** com histórico carregado, contra os
  474–790 MB pré-login?
- Os mesmos números se sustentam em contêiner Linux amd64 e arm64?
- Qual o menor conjunto de flags que preserva o SPA funcional? Cada flag
  removida é RAM ou é assinatura a menos?
- `navigator.webdriver` mitigado muda alguma coisa observável do lado do
  WhatsApp, ou a detecção que importa está noutro sinal?
