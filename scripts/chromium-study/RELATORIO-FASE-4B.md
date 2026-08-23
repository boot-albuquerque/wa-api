# Relatório — Fase 4B

Fechamento dos bloqueadores reais de produção — **parcial**.
Continuação de `RELATORIO-FASE-4.md`. Data: 2026-08-09.

> **Esta fase mediu o alvo final pela primeira vez e aumentou o número de
> bloqueadores em vez de reduzi-lo.** Duas das perguntas centrais — restrição de
> aba única e recovery — **não foram respondidas**, por defeito do próprio
> harness. §5 documenta o defeito sem atenuação.

---

## 0. Respostas diretas

```text
Controller:                    chromedp
ConnectionPolicy:              SHARED_PER_BROWSER (provisional, LOW — Fase 4 §1)
BrowserProfile:                CanonicalBrowserProfileV1 + --user-agent explícito
                               (sem o UA o alvo recusa o browser — §3)
BrowserContextPolicy:          desabilitado no Chromium 151 + perfil validado
InteractionPolicy:             NÃO IMPLEMENTADA
RecyclingPolicy:               NÃO DEFINIDA

Target:                        WhatsApp Web, conta real pareada
Authenticated session RAM:     788 MB memory.current | 689 MB anon | pico 831–862 MB
Target knee:                   NÃO DETERMINADO
Correctness boundary:          NÃO DETERMINADO
Production operating point:    NÃO DETERMINADO

Correct jobs/core-hour:        NÃO MEDIDO no alvo final
Correct jobs/GiB-hour:         NÃO MEDIDO no alvo final

False-success rate:            NÃO MEDIDO no alvo final
p99:                           NÃO MEDIDO no alvo final

Browser crash recovery:        NÃO MEDIDO — teste travou antes da injeção
Pod recovery:                  NÃO MEDIDO
Blast radius:                  NÃO MEDIDO

SHORT SOAK:                    60 min na SPA de referência (Fase 4 §3) — sem leak
24h soak:                      NOT EXECUTED — operational constraint

Site Isolation:                N=1 apenas (Fase 3) — não replicado
```

```text
FINAL DECISION: GO WITH CONDITIONS
```

---

## 1. O alvo final foi medido — e o regime inverte

O pareamento foi concluído e a sessão persiste. Uma sessão autenticada, ociosa,
logo após a restauração:

| grandeza | SPA de referência (filarapida) | **WhatsApp Web** | razão |
|---|---|---|---|
| `memory.current` | 314–358 MB | **788 MB** | 2,3× |
| **`anon`** | ~248 MB | **689 MB** | **2,8×** |
| `file` | ~160 MB | 79 MB | 0,5× |
| pico | 361–502 MB | **831–862 MB** | 1,8× |
| nós de DOM | 1.015 | 2.757 | 2,7× |
| recursos / transferência | 27 / ~0 KB | 105 / 6.292 KB | — |
| origens | 3 | 3 | — |
| heap JS | — | **9,5 MB** (limite 3.185) | — |
| processos | 12–15 | 12 | — |
| renderers | 4–7 | 4 | — |

**O custo não é JavaScript.** 9,5 MB de heap JS contra 689 MB de memória anônima:
o consumo é memória nativa de browser e renderer. Nenhuma otimização no código da
automação toca essa parcela.

Isso confirma, no BrowserProfile atual e como o §4 exigia, a faixa de
**474–790 MB/sessão** que o spike inicial deste estudo tinha observado. O número
não era artefato.

### Consequência arquitetural

A SPA de referência é **CPU-bound**: `memory.events` zerado em toda concorrência,
throttling de CFS subindo a 79%. O alvo final é **memory-bound**: ~690 MB de anon
por sessão contra um piso de browser de 231–256 MB.

Num pod de 3 GiB isso dá, grosseiramente, 3 sessões — e a topologia de 4 slots
herdada da SPA de referência **não se transfere**. O `RELATORIO-FASE-4.md` §0
registra `2 Chromium : 2 pages : 4 slots` como topologia de produção; **essa linha
não vale para o WhatsApp Web** e precisa ser marcada como escopo restrito à SPA de
referência.

---

## 2. Restauração de sessão

| medida | valor |
|---|---|
| estado detectado | `app`, via seletor `#pane-side` |
| conversas renderizadas | 67 |
| `sync_settle` | **0,027 s** |
| perfil em disco | 145 MB (158 MB após as corridas seguintes) |

A sessão restaura praticamente instantânea a partir do perfil persistido, sem
re-sincronização perceptível. Isso é pré-requisito de qualquer `RecyclingPolicy`
— reciclar browser só é barato se restaurar for barato — e está estabelecido.

---

## 3. O alvo recusa o BrowserProfile canônico

Com o perfil canônico, o WhatsApp Web **não serve a aplicação nem o QR**. Renderiza:

```text
WhatsApp works with Google Chrome 100+
To use WhatsApp, update Chrome or use Mozilla Firefox, Safari,
Microsoft Edge or Opera.
```

O motor **é** Chrome 151. O que difere é um token: sob `--headless=new` o Chromium
se anuncia como `HeadlessChrome/151.0.0.0`, e o sniffing do alvo não o reconhece
como Chrome.

Trocando **apenas** esse token para `Chrome/151.0.0.0` — mesma engine, mesma
versão, nada mais:

```text
state=qr  matched_by=canvas[aria-label*="Scan"]
QR renderizado, 57.550 bytes
```

```text
Causa:      user-agent sniffing sobre o token HeadlessChrome
Confidence: HIGH — probe A/B com uma única variável
```

### O que isso muda no roadmap

A frente de anti-ban (ADR-0006 D3) **deixa de ser refinamento posterior e vira
pré-requisito**: sem ela o alvo não carrega. E o Browser Runtime precisa de uma
caixa que hoje não existe no diagrama:

```text
Internal Browser Runtime
   ├── LaunchPolicy
   ├── IdentityPolicy      <-- NOVA: UA e superfície de fingerprint
   ├── ConnectionPolicy
   ├── WaitPolicy
   ├── InteractionPolicy
   ├── RecoveryPolicy
   └── FastPathPolicy
```

### Trade-off que precisa ficar registrado

Fingerprint medido no perfil canônico (`mode=waprep`, dois boots):

```text
navigator.webdriver      false
userAgent                ...HeadlessChrome/151.0.0.0
plugins                  5
hardwareConcurrency      10
platform                 Linux x86_64
```

Corrige uma afirmação anterior deste estudo: `navigator.webdriver` é **false** —
nem o perfil canônico nem o chromedp via `RemoteAllocator` passam
`--enable-automation`. A superfície exposta é o **User-Agent**.

Sobrescrever o UA abre a aplicação, mas **não torna o browser indistinguível**:
cria um descasamento entre identidade declarada ("Chrome comum") e capacidades
observáveis (headless), e descasamento é sinal de detecção. A decisão foi tomada
de forma explícita, com o custo declarado, e não como otimização silenciosa.

---

## 4. Perfil persistente não sobrevive à troca de container

O Chromium grava um lock de singleton de processo como symlink:

```text
SingletonLock -> 410f829ba5be-171     (hostname-pid)
SingletonCookie -> 9582335330973686888
SingletonSocket -> /tmp/org.chromium.Chromium.XXXX/SingletonSocket
```

Ele compara aquele hostname com o próprio. Cada container — e cada pod — nasce com
hostname novo, então a checagem sempre conclui que outra máquina detém o perfil:

```text
The profile appears to be in use by another Chromium process (171)
on another computer (410f829ba5be). Chromium has locked the profile
so that it doesn't get corrupted.
```

O browser **não sobe**.

```text
Consequência: um pod que reinicia com o volume de sessão montado NÃO volta
              sem reclamar o perfil antes.
Confidence:   HIGH — reproduzido, e a correção foi validada em execução
```

A recuperação (remover as três entradas antes do launch) está implementada. Ela é
segura **apenas** porque o desenho dá posse exclusiva do perfil a um runtime — o
lease da ADR-0005. Num volume compartilhado seria incorreta, e nada no código a
torna segura: a exclusividade é que torna.

---

## 5. O que falhou, e por quê

Três execuções travaram, todas pela **mesma causa**, e a causa é minha:

| execução | orçamento | tempo real | onde travou |
|---|---|---|---|
| `wasession` (sessão 8 min) | 8 min | **38 min** | probe de leitura no laço de amostragem |
| `watabs` (aba única) | ~3 min | **10 min** | leitura de estado após abrir a 2ª aba |
| `warecover` (recovery) | ~4 min | **9 min** | antes da injeção — navigate/poll inicial |

**Causa raiz: chamadas CDP sem deadline por operação.** É exatamente o defeito que
este estudo documentou na Fase 1 como achado sobre os *wait primitives* do
chromedp — e que eu reintroduzi no meu próprio harness três vezes, corrigindo
pontualmente em vez de aplicar a regra ao conjunto.

Correções aplicadas até aqui: deadline de 10 s no probe do `wasession`; deadline de
5 s **dentro** de `waPageState`, com `unresponsive: true` registrado em vez de
bloqueio. Pontos **ainda sem deadline**: `RunWARecover` (navigate e poll inicial) e
`runWAJob`.

**Confound que não sei separar:** o perfil persistente foi morto à força três vezes
no meio de escrita (158 MB de IndexedDB). Pode estar degradado, e nesse caso parte
do travamento não é do meu código. Separar as duas causas exige recriar o perfil
do zero — não feito.

### Resultado da tentativa de concorrência

Uma execução anterior (`wacap`, 1 slot, 3 jobs) terminou com **3 timeouts em 3**:
cada job abre uma aba nova e navega ao alvo, e `#pane-side` nunca apareceu nessas
abas. A memória, porém, foi coletada:

```text
piso do browser   256 MB
por slot          321 MB acima do piso
pico              862 MB
renderers         6
CPU               46,4 s     throttling  1%
```

A hipótese é que o **WhatsApp Web seja single-tab por perfil**, e a arquitetura do
`whatsapp-web.js` — um `Client` por browser, `userDataDir` próprio por sessão —
aponta na mesma direção.

```text
Status: NÃO CONFIRMADO. Timeout não é diagnóstico.
```

O teste que leria o texto da segunda aba é o que travou. Se a hipótese se
confirmar, a topologia do alvo final não é escolhida por CPU nem por RAM: é
imposta pela aplicação, e vira `1 perfil = 1 sessão = 1 aba`, com concorrência
vindo de múltiplos browsers/pods — e o blast radius passa a ser exatamente uma
sessão por browser morto.

---

## 6. Estado dos critérios de GO

Critério revisado da Fase 4B (§20), com 24h substituído por lifecycle + recovery:

| critério | estado |
|---|---|
| target real medido | ✔ **parcial** — RAM de sessão sim; capacidade não |
| false success = 0 no operating point | ✘ operating point não determinado |
| InteractionPolicy validada | ✘ não implementada |
| CPU headroom conhecido | ✘ |
| RAM headroom conhecido | ✔ **parcial** — 689 MB anon/sessão conhecido, teto por nó não |
| browser crash recovery medido | ✘ travou |
| pod recovery medido | ✘ |
| blast radius aceito | ✘ |
| SHORT SOAK sem leak não limitado | ✔ 60 min na SPA de referência |
| RecyclingPolicy implementada e medida | ✘ |
| custo/capacidade final calculados | ✘ |

Dois de onze, ambos parciais. **`GO WITH CONDITIONS`** — e o saldo da fase é
negativo: o alvo final revelou-se memory-bound, o que invalida a topologia
recomendada, e nenhuma das duas medições bloqueantes avançou.

Nada medido contradiz a arquitetura `chromedp + Chromium`. O que falta continua
sendo medição.

---

## 7. Ordem para retomar

1. **Auditar deadline por operação em todo o harness de uma vez** — regra: nenhuma
   chamada CDP sem `context.WithTimeout`. Pontos conhecidos: `RunWARecover`
   (navigate + poll inicial), `runWAJob`.
2. **Recriar o perfil do zero** (novo pareamento) antes de medir recovery, para
   eliminar a hipótese de perfil degradado pelos kills.
3. Confirmar **aba única** lendo o texto da segunda aba, com deadline curto.
4. **Recovery**: `Chromium SIGKILL → novo Chromium → mesmo perfil → sessão segue
   autenticada?` — o código já existe e o laço de detecção já tem deadline curto.
5. Só então concorrência, `RecyclingPolicy` e modelo econômico.

Os Tracks E (InteractionPolicy), D (correctness boundary) e G (site isolation
N≥3) seguem **não executados**.

---

## 8. Reprodutibilidade

Ambiente: Docker Desktop 27.3.1 no macOS, VM Linux 6.10.11-linuxkit aarch64,
3 vCPU / 3 GiB por container. Chromium 151.0.7922.108. cgroup v2.

```bash
UA="Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36"

# Valida o harness SEM tocar no WhatsApp (dois boots do mesmo perfil)
docker run --rm -e SKIP_BROWSER=1 --cpus=3 --memory=3g \
  -v $PWD/results-p4:/out -v $PWD/wa-session:/session \
  chromium-study:p4 -mode waprep -out /out/wa-prep.json

# ÚNICO modo que pode exibir QR
docker run --rm -e SKIP_BROWSER=1 --cpus=3 --memory=3g \
  -v $PWD/results-p4:/out -v $PWD/wa-session:/session \
  chromium-study:p4 -mode waopen -wa-wait 12m -wa-ua "$UA" -out /out/wa-open.json

# Sem -wa-ua reproduz a recusa do alvo (tela "update Chrome")
```

Artefatos: `results-p4/wa-prep.json`, `wa-open.json`, `wa-open-uaprobe.json`,
`wa-cap-c1.json`. Perfil pareado em `wa-session/profile` — **fora do git**, contém
credencial de sessão.

Garantias mantidas em código: nenhum caminho envia mensagem; nenhuma conversa é
aberta sem `-wa-chat` explícito; screenshot apenas na tela de QR; relatórios
contêm somente contagens, tamanhos e durações.
