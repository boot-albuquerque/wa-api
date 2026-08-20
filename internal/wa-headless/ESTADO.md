# Estado da iniciativa `wa-headless`

Consolidação de 2026-08-19, pedida pela orquestração depois de a matriz de
paridade fechar. É um retrato: o que existe, o que está PROVADO, o que está
aberto e por quê, e o que depende de decisão humana.

Números vieram do repositório, não de memória. **Atualizados em 2026-08-20**,
porque um documento de estado que envelhece em silêncio vira citação errada — que
é o defeito registrado no H29:

| | escrito 19/08 | 20/08 (manhã) | **agora** |
|---|---:|---:|---:|
| testes | 254 | 261 | **263** |
| pacotes | 12 | 12 | **12** |
| commits (nada empurrado) | 45 | 56 | **61** |
| achados fechados | 20 | 22 | **23** |
| achados abertos | 9 | 10 | **10** |

A contagem de abertos aplica a leitura humana que o próprio instrumento pede:
o scanner marca **H5** e **H14** como abertos porque têm vários status, e ambos
estão fechados. Os abertos novos desde 19/08 são o **H32** (`abandonado`) e o
**H33** (`medido`) — os dois estados que o vocabulário do gate ganhou hoje, e
ambos contam como abertos de propósito: *abandonado* é achado deixado de lado com
o motivo escrito, *medido* é medição feita cuja consequência pode não ter sido
aplicada.

---

## 1. O que o módulo faz hoje

| camada | pacote | responsabilidade |
|---|---|---|
| driver | `engine/` | único que importa `chromedp`; lançar, parar, abas, prazos, perfil |
| página | `spa/` | classificar, inventário de módulos, liveness, assentamento |
| sessão | `core/` | `StartSession` → READY verificado, ou `*BootFailure` classificado |
| retenção | `runtime/` | `Holder` — uma sessão entre comandos, posse, pid |
| capacidades | `capabilities/` | as seis da paridade |
| observabilidade | `observability/` | `OpLog`, com política de redação |

## 2. Paridade com o `whatsapp-web.js` — COMPLETA

Seis capacidades, não catorze: as outras oito o `wa-noise` já serve pelo
protocolo (`PARIDADE-WWEBJS.md` §2–3). Todas com prova contra a SPA real.

| capacidade | onde | prova real |
|---|---|---|
| `livenessCheck` | `capabilities/liveness` | pior latência **1 ms** em 5 amostras |
| `getBrowserPid` | `runtime.Holder.BrowserPID` | recusa o pid morto (`SIGKILL` de fora) |
| `refreshOwner` | `capabilities/owner` | PN e LID presentes, `display_name` NULL |
| `onMessageMeta` | `capabilities/messagemeta` | entrega ao vivo em 54 s, 58 reposições ignoradas |
| `fetchMessages` | `capabilities/fetchmessages` | 340 carregados, filtro por chat casa |
| `backupNow` | `capabilities/backup` | restaura com identidade presente; custo medido nos dois perfis (§6.6) |

### Divergências CONSCIENTES do `wwebjs` (§6 da paridade)

1. **PN e LID separados** — eles colapsam em `pn || lid` e perdem qual respondeu.
2. **`display_name` UNVERIFIED** — o getter existe e responde `NULL` neste build.
3. **`msg.id._serialized` é NULL aqui** — eles dependem dele; nós expomos as partes.
4. **`fetchMessages` lê o CARREGADO**, não pede ao servidor. Declarado, com
   `Loaded` como denominador e `Truncated()`.

## 3. Invariantes travadas em teste

Além das do `HANDOFF §6`, três nasceram nesta semana:

- **15 · o tempo de vida de uma sessão termina no `Stop`** — duas rotas de falha
  distintas (`context canceled` e `deadline exceeded`), cada uma com controle.
- **um profile = um dono** — e o controle negativo mostrou que há **duas
  camadas**: a posse em processo e o próprio lock do Chrome. Só a primeira falha
  rápido e com causa nomeada.
- **nada lê texto de página** — portão de módulo por AST, com isenção que
  **decai** se o teste que a justifica sumir.

## 3.1 Endurecimento sob concorrência (2026-08-19)

Toda capacidade foi provada **sozinha**. O produto não vai usá-las sozinhas: um
timer de liveness bate enquanto um comando busca mensagens enquanto uma
assinatura drena. *"Cada uma funciona"* não é *"elas funcionam juntas"*, e este
módulo passou a semana descobrindo que a falha interessante mora na combinação.

| teste | escala | resultado |
|---|---|---|
| `TestConcurrentCapabilityCallsOnOneSession` | 8 chamadores × 6 iterações, fixture local | **PEAK OVERLAP 8**, latência de pior caso 2 ms → **19 ms** sob sobreposição |
| `TestRealSPAConcurrentCapabilities` | 5 rodadas × 4 capacidades, SPA real | **PEAK OVERLAP 20**, identidade estável |

**Os dois MEDEM A PRÓPRIA SOBREPOSIÇÃO**, e isso não é ornamento: um teste de
concorrência cujos chamadores nunca coincidem prova **serialização**, não
segurança, e o `-race` não tem o que detectar. Com ida e volta de 2 ms isso era
possibilidade real, não pedantismo. Ambos falham se o pico ficar abaixo de 2.

**A asserção que só a concorrência permite**: a identidade do dono tem de voltar
IGUAL em todas as leituras simultâneas. Uma sessão não muda de conta no meio da
corrida, então qualquer variação seria uma resposta chegando ao chamador errado —
e um teste sequencial não consegue nem formular isso.

**O que ficou provado**: a afirmação que o `spa.Monitor` fazia no próprio
comentário — *"seguro para uso concorrente: um timer de sonda e um caminho de
comando podem ambos perguntar"* — deixou de ser prosa. E o custo da concorrência
virou número: **19 ms** de pior caso contra 2 ms sequencial.

## 3.2 Retenção de UMA HORA sob carga (2026-08-20)

`TestRealSPALongHoldUnderLoad`, 60 amostras contra o perfil pareado, com
liveness, owner, fetch e drain rodando a cada minuto.

```
liveness not-alive: 0 | identity absent: 0 | reinstalls: 0 | dropped: 13
latency: mediana 2ms, pior 96ms (a PRIMEIRA amostra, logo após o boot)
RSS: first=1740MB last=760MB min=655MB max=1740MB — deriva 0,44x
processos: 9 em 59 amostras, 10 em uma
```

**A hipótese que motivou medir era VAZAMENTO, e o dado aponta o contrário.**
A memória cai monotonicamente do pico de boot e assenta em torno de 700-800 MB;
a contagem de processos não cresce. Dois eixos independentes concordam, e nenhum
dos dois foi projetado para provar isso — vieram junto porque a amostragem os
incluía.

**Os 13 descartes são todos da rajada do minuto 1** (H25), e nenhuma amostra
posterior descartou. A assinatura seguiu entregando em volume baixo — ~23
eventos esparsos ao longo dos 59 minutos restantes —, o que resolve por
evidência a ambiguidade do H24: assinatura viva, não apenas instalada.

**O que este número NÃO autoriza.** É tentador dividir 16 GB por 800 MB e
declarar capacidade. Não faço isso aqui por três motivos: a `runtime/doc.go` já
avisa que os números do spike são um PISO; esta é UMA sessão, e nada mediu a
interação entre sessões concorrentes no mesmo host; e "sob carga" aqui são as
NOSSAS sondas, com a conta ociosa — tráfego de entrada sustentado é outra
medição, e precisaria de uma conta que ninguém tem.

**O que ele autoriza**: dizer que uma sessão segurada por uma hora **não
degrada** — não perde identidade, não perde a assinatura, não acumula memória
nem processo, e para limpo no fim.

## 3.3 Sessões CONCORRENTES, e a primeira afirmação herdada a ser falsificada

Até 2026-08-20 este repositório **nunca tinha rodado dois browsers ao mesmo
tempo**. A afirmação de `runtime/doc.go` e da ADR-0006 — *474–790 MB por sessão,
escalando linearmente até três* — vinha do spike e nunca fora exercitada. E ela
é carga: é o que põe um host de 16 GB *"na faixa de dezenas de sessões"*.

| sessões | total | por sessão | processos | pior latência |
|---:|---:|---:|---:|---:|
| 1 | 1023 MB | 1023 MB | 9 | 0 ms |
| 2 | 2004 MB | 1002 MB | 18 | 0 ms |
| 3 | 2600 MB | **866 MB** | 31 | 1 ms |

**A FORMA se confirma, e melhor que o prometido**: o custo por sessão CAI para
0,85× com três concorrentes, e nenhuma sessão já rodando parou de responder
quando outra subiu. Medi TODAS as sessões vivas a cada onda, não só a nova —
porque a afirmação é sobre o que acontece com as que já estão lá.

**O NÚMERO se falsifica**: 1023 MB contra o teto de 790 MB, **30% acima**, na
mesma condição pré-login e com os **mesmos 9 processos**. Mais memória na mesma
topologia, o que descarta a explicação fácil.

**A consequência foi APLICADA, não só registrada**: o `runtime/doc.go` passou a
carregar a re-medição e a aritmética que ela muda — **~15 sessões em 16 GB** no
lugar de ~20.

**O que segue sem medição, e é o que decide capacidade de verdade**: sessões
**PAREADAS** concorrentes. Não foi feito de propósito — os dois perfis pareados
desta máquina são da MESMA conta, e dois dispositivos ativos podem fazer o
WhatsApp invalidar um. Precisa de duas contas, e isso é decisão do humano.

## 4. Aberto, e por quê

| # | por que segue aberto |
|---|---|
| H3 | pendência de OUTRA sessão (documentos do `disparazaap`) |
| H8 | `Contains` na checagem de host; apertar **muda comportamento** |
| H9 | pré-existente, fora do escopo desta iniciativa |
| H10 | **de propósito** — "o perfil nunca encolhe" é falso e está registrado |
| H11 | leitura de coluna do M6; documental |
| H15 | envelope de `C` sem expressão executável — depende de consumidor que **não existe** (H17 mediu) |
| H16 | **não fechável por amostragem**: mutação numa janela de 24 h escapa de qualquer varredura sobre 1,8×10¹⁹ ns. A garantia é estrutural |
| H18 | `BootFailure` sem PID — aguardava consumidor; hoje há seis capacidades, então é o mais maduro para reabrir |
| H25 | teto de 500 no buffer **sem medição** — precisa de conta de volume real |

| H32 | auditoria de constantes **ABANDONADA** — varredura de texto não lê estrutura de código, e a alternativa (AST) custa mais que o achado justifica |
| H33 | **MEDIDO**: linearidade confirmada, teto de 790 MB falsificado. Segue aberto porque sessões PAREADAS concorrentes precisam de uma segunda conta |

**Nenhum deles é guarda sem teste.** Essa categoria zerou com o H27.

### Achados de 2026-08-20, todos de auditoria sobre o próprio trabalho

- **H30** — os quatro dublês das capacidades **ignoravam o `ctx`**, e a produção
  não ignora. Uma capacidade chamada com contexto cancelado errava em produção e
  **passava** nos testes: o chamador que já desistiu recebia dado *fabricado*.
- **H31** — enumerando as 72 exportadas contra os testes, achei
  `liveness.NewWithMonitor`: um construtor que **inventei** para um chamador que
  nunca existiu. API exportada é promessa; removida.
- **H32** — a auditoria de constantes foi abandonada, e o commit dela saiu com o
  portão **vermelho** porque eu rodei o gate dentro de um *pipe* e o `&&` viu o
  status do `tail`. **Gate dentro de pipe não é gate.**
- **H18** — `BootFailure` ganhou o PID, para **correlação e nunca ação** — a
  fronteira que o separa do H23. Dois controles negativos **não morderam**: a
  ordem de leitura do pid (afirmação minha, falsa) e a guarda da fronteira, que
  **removi** em vez de manter, porque teste que não morde é garantia falsa.
- **H30/H31** — ver acima. Junto com o H18, os três vieram de **auditar o próprio
  trabalho**, não de escrever código novo.
- **H33** — a primeira afirmação HERDADA que este módulo falsificou com número.

## 5. Depende do humano

1. **Merge para `feature/macbook-lucas`** — 34 conflitos, **21 implementações
   RIVAIS** de `lease`/`dispatch`/`cluster`. Escolher errado apaga trabalho.
2. **Reset do perfil de laboratório** — pareado; trava 3 testes de QR.
3. **`git push`** — 45 commits parados.
4. **Alguém de outro número enviar** — prova a direção `in` ao vivo.
5. **Conta de volume real** — calibra o H25.
6. **CAP-07 `sendText`** — fora da matriz; decisão de produto.

## 6. O que esta semana ensinou, em uma frase cada

- **O dublê não é mais permissivo que a produção — é mais bem-comportado num
  eixo em que ninguém olha.** Velocidade, tempo de vida do contexto, existência
  de socket. A pergunta é *em que EIXO meu dublê é melhor que o mundo?*
- **Este teste passaria se a ação que ele pede não tivesse acontecido?** Se sim,
  ele não mede a ação. Pegou o teste de entrega e o de boot não pareado.
- **Um instrumento que não distingue dois estados não pode decidir entre eles.**
  Quatro instrumentos cegos consertados, três quebrados por mim no mesmo dia.
- **Controle negativo que não morde não prova nada** — e sete não morderam na
  primeira tentativa.
- **A verdura da suíte não distingue "coberto" de "coberto onde importa".** A
  auditoria achou a lacuna na capacidade nº 1.
- **Medir e depois ler a medição com a hipótese na cabeça é não ter medido.**
