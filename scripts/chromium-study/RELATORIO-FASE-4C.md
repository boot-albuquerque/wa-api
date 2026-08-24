# Fase 4C — Harness confiável e topologia do WhatsApp Web

**Decisão de arquitetura:** `GO`
**Prontidão para produção:** `GO WITH CONDITIONS` — as condições estão em §7,
e todas têm fix concreto, nenhuma é ressalva em aberto.

O bloqueador que fechou a Fase 4B — perda de credencial a cada poucos restarts
do browser, que inviabilizaria rodar em Kubernetes — **era defeito do harness de
medição, não do alvo**. Está identificado, corrigido e verificado.

---

## 1. Regra zero: o instrumento antes da medida

A Fase 4B terminou com o harness declarado NÃO CONFIÁVEL. Três execuções
travaram pela mesma causa — chamada CDP sem prazo — e cada correção pontual
permitiu que o defeito voltasse no arquivo seguinte: 38 minutos num orçamento de
8, 10 minutos em ~3, 9 em ~4.

A lição não foi "faltou um timeout ali". Foi que o prazo não podia ser
responsabilidade do call site. A Fase 4C começou tornando-o propriedade do
**tipo de operação**:

| camada | papel |
|---|---|
| `DeadlinePolicy` | prazo por classe de operação |
| `Watchdog` | orçamento do experimento inteiro |
| `OpLog` | registro de toda operação, para saber ONDE parou |

Nenhuma função pública executa algo remoto sem derivar contexto da policy.

**Auditoria (§7).** A policy é exercitada contra páginas que deliberadamente
nunca respondem: servidor que aceita a conexão e não escreve, seletor
inexistente, Promise que não resolve, condição de Poll sempre falsa, e — caso
acrescentado depois — três operações sequenciais na mesma aba.

Resultado: **5/5 PASS**, overhead de +1 a +6 ms sobre o prazo.

A auditoria foi verificada **nos dois sentidos**. Removendo o `primeTab`, ela
FALHA e a fase para. Um teste que só passa não prova que pegaria o defeito.

---

## 2. Três defeitos que teriam virado conclusão publicada

O valor do instrumento aparece no que ele impediu, não no que ele mediu.

**Aba morta pelo cancelamento da primeira operação.** O chromedp cria o target
preguiçosamente no primeiro `Run` e prende as goroutines que o gerenciam ao
contexto *desse* `Run`. Como `Runner.Do` cancela o filho ao retornar, a aba
morria junto com a primeira operação e tudo depois falhava em 0 ms com
`context canceled` — nunca com `deadline_exceeded`, ou seja, **sem parecer
timeout**. É interação entre a policy e a biblioteca, não defeito de nenhuma
das duas isoladamente.

**`chromedp.Poll` que nunca avaliava a condição.** A estratégia padrão é
`requestAnimationFrame`, e rAF não dispara em target headless fora de primeiro
plano. Todo `Query/ready` queimava 15 s sem testar o predicado uma vez sequer,
enquanto o `StateProbe` seguinte encontrava `#pane-side` em ~22 ms.

É o **mesmo mecanismo** que a Fase 3 isolou como causa do teto de concorrência
do go-rod (`WaitStableRAF` → `Page.WaitRepaint`). Duas bibliotecas
independentes cometendo o mesmo erro: presumir rAF em aba de fundo. Fica
registrado como padrão a evitar no `wa-headless`.

**Braço experimental que não executava.** A tentativa de desligar por
`browser.Close()` direto foi recusada em 6/6 iterações e caiu no SIGTERM de
reserva — a corrida virou réplica do braço antigo em vez de comparação. Só foi
visível porque cada iteração registra `stopped_via`. Sem esse campo, as 3
iterações saudáveis dessa corrida contra 1 da anterior teriam sido lidas como
efeito do `Browser.close`: **uma causa falsa a partir de um braço que nunca
rodou**.

---

## 3. Topologia: uma sessão ativa por perfil

Duas abas do WhatsApp Web no mesmo perfil não coexistem. A aba nova fica
`APP_READY` e a anterior cai para `SESSION_CONFLICT`.

| rep | aba A | aba B | A depois de B |
|---|---|---|---|
| 1 | APP_READY | APP_READY | SESSION_CONFLICT |
| 2 | APP_READY | APP_READY | SESSION_CONFLICT |
| 3 | APP_READY | APP_READY | SESSION_CONFLICT |

**`SESSION_MIGRATES_TO_NEWEST_TAB`** — 3/3, MEASURED, confiança HIGH.

**Consequência:** isolamento por sessão tem de ser **por perfil**, não por aba.
Isso descarta reaproveitar abas para diluir o custo de ~900 MB anônimos por
sessão medido na Fase 4B.

Ponto operacional: o app-ready chegou a 13,9 s contra um prazo de 15 s. Margem
fina — em produção precisa de folga maior ou vira flake.

---

## 4. A perda de credencial: três hipóteses, duas refutadas

Esta foi a investigação central da fase, e o resultado contraria duas
conclusões intermediárias minhas.

**Hipótese A — SIGKILL causa a perda.** Sustentada inicialmente pelo contraste
entre o Track B (3 relaunches graciosos, sessão sobreviveu) e o Track C (SIGKILL,
sessão perdida). **REFUTADA** por ablação com N=5 por braço sobre a mesma
credencial: a perda reproduziu sob parada graciosa, antes de qualquer SIGKILL.
O contraste original confundia forma de parada com número de ciclos e tempo
decorrido.

**Hipótese B — o reclaim de `Singleton` corrompe o perfil.** A ablação
pretendida não é executável: sem apagar os arquivos o Chromium não sobe, porque
o lock aponta para um `<hostname>-<pid>` de container que não existe mais. O
reclaim é infraestrutura obrigatória em container, não escolha.

Resolvida por raciocínio sobre os dados existentes: o reclaim roda em **todo**
boot desde o primeiro, e o pareamento, o Track B e as primeiras iterações
funcionaram com ele ligado. **Causa constante não produz efeito com limiar.**
Rebaixada a LOW.

**Hipótese C — o desligamento não descarrega o estado de sessão.** Levantada
por uma pista que eu quase deixei passar: o perfil **encolheu de 123 para
118 MB exatamente na primeira degradação** e depois estabilizou. Invalidação
remota não reduz armazenamento local antes do logout.

**CONFIRMADA.**

---

## 5. O experimento que fechou

Antes de gastar pareamento, foi preciso provar que o braço executava — lição
das duas tentativas anteriores. A sonda `closeprobe` mediu, sem WhatsApp:

| caminho | `Browser.close` | processos após 15 s |
|---|---|---|
| CDP cru | aceito em 2 ms | **0** (de 10, em <1 s) |
| `chromedp.Cancel` | sem erro em 19 ms | **todos vivos** |

`chromedp.Cancel` sobre `RemoteAllocator` **não envia o comando**: cai no ramo
seco de `c.first == false` (`chromedp.go:134`, `:252`) e devolve `nil` em
silêncio. Os 19 ms eram teardown de goroutines — eu havia lido essa latência
como evidência de envio, e estava errado.

Com o braço verificado (`stopped_via = browser.close`), a comparação:

| corrida | desligamento | saudáveis | logout | singletons | perfil |
|---|---|---|---|---|---|
| 1 | sigterm | 1 | iter 4 | 3 | cai p/ 118 MB |
| 2 | sigterm | 3 | iter 6 | 3 | cai p/ 118 MB |
| 3 | **browserclose** | **20** | nenhum | **0** | estável ~137 MB |
| 4 | **browserclose** | **20** | nenhum | **0** | estável ~141 MB |

**40 iterações sem uma única degradação**, contra logout na 4ª e na 6ª.

Duas corroborações mecanicistas, não previstas, sustentam a leitura além da
contagem:

1. **`singletons_antes = 0`** a partir da segunda iteração — o Chromium remove
   os próprios arquivos `Singleton` ao sair limpo. Sob SIGTERM ficavam em 3
   sempre. É evidência do mecanismo, não do desfecho.
2. **A queda do perfil para 118 MB não ocorreu nenhuma vez** nos braços limpos.

---

## 6. Achados e confiança

| achado | classificação | confiança |
|---|---|---|
| Harness com prazo por classe de operação, auditado nos dois sentidos | MEASURED | HIGH |
| Um perfil = uma sessão ativa (`SESSION_MIGRATES_TO_NEWEST_TAB`, 3/3) | MEASURED | HIGH |
| Recovery com parada graciosa: p50 10,4 s, p95 13,8 s | MEASURED | HIGH |
| `Browser.close` por CDP funciona; `chromedp.Cancel` não o envia | MEASURED | HIGH |
| Desligamento sujo causa a perda de credencial | MEASURED | HIGH |
| SIGKILL como causa da perda | **REFUTADO** | — |
| Reclaim de `Singleton` como causa | **LOW** | — |
| Invalidação pelo WhatsApp por religações repetidas | **DESCARTADA** | — |
| WhatsApp Web é memory-bound: ~900 MB anônimos, pico 1058 MB | OBSERVED | MEDIUM |

---

## 7. Requisitos de produção

Condições para o `GO`, todas com fix concreto:

1. **Desligar o Chromium por `Browser.close` via CDP.** `SIGTERM` corrompe o
   estado de sessão. O `preStop` do pod precisa fazer a chamada CDP — não
   confiar em sinal, nem em helper de biblioteca que falha em silêncio.
2. **Uma sessão ativa por perfil.** Isolamento por perfil, nunca por aba.
3. **Nunca usar `chromedp.Poll` sem `WithPollingInterval` explícito**, nem
   qualquer espera baseada em `requestAnimationFrame`, em aba de fundo.
4. **Prazo por classe de operação**, não por call site. Nenhum caminho CDP pode
   bloquear indefinidamente.
5. **Folga no app-ready.** Observado até 15,8 s; 15 s é margem insuficiente.
6. **Reclaim de `Singleton` no boot** é obrigatório em container — o lock
   sempre fica obsoleto entre containers.

---

## 8. Não executado

Registrado como limitação, não como sucesso:

- **Soak de 24 h — NOT EXECUTED**, por restrição operacional explícita. Não
  extrapolar as corridas curtas como equivalentes.
- **InteractionPolicy V1** — não implementada.
- **Correctness boundary de CPU com o WhatsApp real** — não medida. O valor da
  Fase 3 vale para a SPA de referência, não para este alvo.
- **Limite superior de ciclos de vida** — 40 iterações sem degradação mostram
  que o limite está bem acima do que o desligamento sujo produzia, mas não
  estabelecem onde ele está.

---

## 9. Nota de método

Três hipóteses causais foram derrubadas nesta fase, e as três eram minhas:
SIGKILL como causa, reclaim como causa, e a leitura de que `chromedp.Cancel`
enviava o comando. Cada uma caiu por medida direta, não por argumento.

O padrão que se repetiu, e que vale para as próximas fases: **verificar que o
braço executa antes de gastar o recurso escasso**. As duas vezes em que isso foi
pulado custaram um pareamento cada. A vez em que foi feito — a sonda
`closeprobe`, 40 segundos contra um perfil já deslogado — foi o que destravou a
fase inteira.
