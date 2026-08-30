# Achado — o renderer do WhatsApp para de responder, e o alvo parece saudável

**Classificação:** `MEASURED`, confiança **HIGH** (o estado existe).
`OBSERVED` — o estado é **intermitente**. **Gatilho: desconhecido.**

**Data:** 2026-08-10 · **Origem:** Fase 6, nó H1 (`RELATORIO-FASE-6.md` §6)
**Ambiente:** Chromium 151 headless, contêiner Linux arm64, 3 CPU / 3 GiB,
sessão do WhatsApp pareada e viva.

> Documento avulso de propósito: o achado **não pertence ao nó que o encontrou** e
> vale para as duas pilhas — `wa-worker` (`whatsapp-web.js`) e `headless`
> (Chromium + CDP). Portável para o `disparazaap`, onde não posso escrever.

---

## 1. O que foi medido

A página do WhatsApp carrega, registra o service worker, e **para de executar
JavaScript**. Sondagem por `Runtime.evaluate`, com prazo do lado Go, uma a cada
2 s, prazo de 30 s por sondagem:

```
settle0  t=1s     RESPONDEU
settle1  t=33s    deadline of 30s exceeded
settle2  t=65s    deadline of 30s exceeded
settle3  t=97s    deadline of 30s exceeded
settle4  t=129s   deadline of 30s exceeded
settle5  t=161s   deadline of 30s exceeded
settle6  t=193s   deadline of 30s exceeded
settle7  t=225s   deadline of 30s exceeded
settle8  t=257s   deadline of 30s exceeded
```

**Uma resposta em t=1 s e mais nada por 4 minutos**, nove sondagens
consecutivas esperando o prazo cheio.

Antes deste teste, com prazo de 5 s, o mesmo estado aparecia como 12 timeouts
seguidos — indistinguível de "renderer ocupado com o sync inicial". **Subir só o
prazo, mantendo o resto, é o que separou as duas leituras**: ocupação de main
thread não dura quatro minutos ininterruptos.

Para o produto a distinção colapsa de qualquer forma: uma sessão que não
responde por 4 minutos é inutilizável, qualquer que seja a causa interna.

## 2. O que torna isto perigoso: por fora está tudo certo

Durante a janela inteira de não-resposta:

| sinal | estado |
|---|---|
| processo do renderer | **vivo** |
| `Target.getTargets` → `page https://web.whatsapp.com/` | **presente, `attached=true`** |
| `service_worker .../sw.js` | **presente, `attached=true`** |
| contagem de processos | inalterada (11) |
| conexão CDP | **aceita comandos de browser normalmente** |

**Nenhum sinal estrutural denuncia o estado.** A conexão administrativa responde,
os targets estão lá, o processo está de pé. O que não funciona é a página.

## 3. A regra que decorre

> **Liveness de sessão não pode ser "o processo está vivo" nem "o target
> existe". Tem de ser um `Evaluate` com prazo do lado do controlador.**

Um health check estrutural — que é o que quase todo mundo escreve primeiro —
reportaria **saudável** durante os 4 minutos em que a sessão não faz nada. Numa
frota com standby e reciclagem por saúde, isso significa manter viva uma sessão
morta e contá-la como capacidade.

É o mesmo princípio do estágio `Verify` da InteractionPolicy, uma camada abaixo:
**"respondeu ao protocolo" e "a aplicação está executando" são afirmações
diferentes**, e só a segunda interessa.

### 3.1 O prazo tem de ser do lado do controlador

Corolário que custou 25 minutos de corrida cega: **o `WithPollingTimeout` do
`chromedp.Poll` é um timer dentro da página**. Página que não executa JS nunca
dispara o próprio timeout, então o `Poll` fica preso indefinidamente — fora da
política de prazos sem parecer que está.

Vale para qualquer espera implementada page-side, em qualquer driver. Se o
relógio mora na página, uma página parada o para junto.

## 4. O que NÃO está estabelecido

- **O gatilho.** Nas corridas de `cpubound` da Fase 5 o `Evaluate` respondeu — a
  descoberta de seletores e as sondas de layout devolveram dados. O estado é
  intermitente e não sei o que o provoca. Não nomear sem medida.
- **A duração máxima.** Medi 4 minutos porque foi o orçamento; não sei se sai
  disso sozinho.
- **Se recarregar resolve.** Não testado.
- **Se acontece com `whatsapp-web.js`.** Não testado — mas a causa está no
  Chromium/página, não no driver, então a hipótese é que sim (`INFERRED`).

## 5. O que fazer com isto

**Curto prazo, nas duas pilhas:**

1. Health check de sessão por `Evaluate` com prazo, nunca por presença de target
   ou de processo.
2. Métrica de latência dessa sondagem, não só o booleano — a degradação aparece
   antes do timeout.
3. Quando a sondagem estourar N vezes seguidas, classificar como
   `UNRESPONSIVE` e tratar como sessão perdida, não como sessão ociosa.

**Investigação própria, quando houver espaço:** achar o gatilho. Enquanto ele
for desconhecido, o mitigador é detectar e reciclar, não prevenir.

## 6. Reprodução

```bash
S=scripts/chromium-study
UA="Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36"

docker run --rm -e SKIP_BROWSER=1 --cpus=3 --memory=3g \
  -v "$PWD/$S/wa-session:/session" -v "$PWD/$S/results-p4c:/out" \
  chromium-study:p6 -mode targets -wa-ua "$UA" \
  -settle-probe 30s -settle-budget 240s -out /out/h1-probe30.json
```

O modo continua mesmo quando `waitAppReady` falha — **de propósito**: desistir
aos 15 s responde antes de perguntar, já que renderer ocupado e renderer travado
falham igual num prazo curto.

Artefato: `results-p4c/h1-probe30.json` (inclui `OpLog` com a duração de cada
sondagem).
