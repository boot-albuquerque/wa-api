# Estado da iniciativa `wa-headless`

Consolidação de 2026-08-19, pedida pela orquestração depois de a matriz de
paridade fechar. É um retrato: o que existe, o que está PROVADO, o que está
aberto e por quê, e o que depende de decisão humana.

Números vieram do repositório, não de memória: **254 testes**, **12 pacotes**,
**45 commits** em `feature/wa-headless-foundation` (nada empurrado), **6
armadilhas** catalogadas, **20 achados fechados** e **9 abertos**.

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
| `backupNow` | `capabilities/backup` | 1163 arquivos restaurados, READY 9,9 s, identidade presente |

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

**Nenhum deles é guarda sem teste.** Essa categoria zerou com o H27.

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
