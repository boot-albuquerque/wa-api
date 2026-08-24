# Metodologia e reprodutibilidade

Estudo comparativo de controllers CDP em Go, para automação determinística de
SPAs Chromium em alta escala.

## As quatro camadas, mantidas separadas

O erro central que este estudo existe para evitar é atribuir à biblioteca o que
pertence à política de lançamento do browser.

| camada | o que é | como foi fixada |
|---|---|---|
| browser | Chromium 151.0.7922.108 (Debian bookworm) | uma única imagem, um único binário |
| launch policy | `CanonicalBrowserProfileV1` | lançada pelo `entrypoint.sh`, **antes** de qualquer controller conectar |
| controller | chromedp / rod / cdproto direto / CDP cru | o que está sob teste |
| workload | SPA A (leve) e B (pesada), servidas localmente | embutidas no binário |

Nenhum controller lança browser. Todos recebem a URL de um browser **já de pé**.
É isso que torna a comparação uma comparação de controllers.

## Ambiente medido

```
host        macOS 15.6 arm64, 10 cores (4P+6E), 16 GiB
container   Docker 27.3.1, linux/arm64, cgroup v2
kernel      6.10.11-linuxkit aarch64
limites     --cpus=4 --memory=4g --memory-swap=4g
cgroup      memory.max=4294967296  cpu.max="400000 100000"
Go          1.25.12
chromedp    v0.16.0
cdproto     v0.0.0-20260804232424-e85f50dbfd32
rod         v0.116.2
playwright-go  v0.6000.0 (driver Playwright 1.60.0)
websocket   github.com/coder/websocket v1.8.12
```

**Ressalva de arquitetura, não contornável aqui:** as medições são
**linux/arm64**. Nodes de produção em amd64 têm outro alocador, outro custo de
Chromium e outro perfil de cache. Emular amd64 no Docker Desktop invalidaria
CPU e latência, então rodar nativo e declarar a limitação é mais honesto que
produzir números emulados. **Reexecutar em amd64 é obrigatório antes de decidir
capacidade.**

## `CanonicalBrowserProfileV1`

Definida em dois lugares — `main.go` (Go) e `entrypoint.sh` (shell) — e a lista
completa é impressa em cada relatório JSON, no campo `meta.canonical_flags`.

```
--headless=new
--no-sandbox
--disable-dev-shm-usage
--disable-gpu
--no-first-run
--no-default-browser-check
--disable-background-networking
--disable-background-timer-throttling
--disable-backgrounding-occluded-windows
--disable-renderer-backgrounding
--disable-breakpad
--disable-client-side-phishing-detection
--disable-default-apps
--disable-extensions
--disable-component-extensions-with-background-pages
--disable-hang-monitor
--disable-ipc-flooding-protection
--disable-popup-blocking
--disable-prompt-on-repost
--disable-sync
--metrics-recording-only
--disable-features=site-per-process,Translate,TranslateUI,BlinkGenPropertyTrees,AcceptCHFrame,MediaRouter,OptimizationHints
--disable-site-isolation-trials
--force-color-profile=srgb
--window-size=1280,800
--lang=en-US
--remote-debugging-address=0.0.0.0
```

Duas escolhas exigem justificativa explícita, porque reduzem segurança:

- `--no-sandbox` — necessário para Chromium em container sem user namespaces.
  Está aqui **declarado**, e não herdado de uma biblioteca que detecta container
  e desliga o sandbox sozinha (ver a auditoria do rod).
- `--disable-site-isolation-trials` + `site-per-process` — a maior alavanca de
  memória encontrada. Isolamento de site é **fronteira de segurança do browser**;
  desligá-la é troca consciente, e o perfil `no-isolation` existe para medir a
  alavanca em vez de supô-la.

## Métrica de memória

`sum(RSS)` **não** é usada como métrica principal. Processos do Chromium
compartilham centenas de MB de mapeamentos (o próprio binário, as bibliotecas),
e somar RSS conta essa memória uma vez por processo.

A métrica principal é **cgroup v2 `memory.current`**, o número que o kernel
cobra e que o Kubernetes usa para OOM-kill, complementada por `memory.stat`
(`anon`, `file`, `shmem`, `slab`, `kernel_stack`, `sock`).

Ordem de grandeza da diferença, medida neste estudo: um Chromium com páginas
abertas somava **~1,1 GiB** em `sum(RSS)` enquanto o `memory.current` do mesmo
container marcava **~268 MiB**.

O lado do controller é medido separadamente (RSS do processo Go, goroutines,
threads via `pprof.Lookup("threadcreate")`, FDs via `/dev/fd`) e **nunca**
somado ao lado do browser.

## Estatística

- warmup descartado antes de cada série;
- N = 100 jobs por controller no workload A, 60 no B;
- três repetições independentes do experimento principal, **com a ordem dos
  controllers rotacionada** — foi a rotação que revelou um comportamento
  destrutivo do rod que a ordem fixa esconderia;
- percentis por rank mais próximo, sem interpolação;
- reportados N, média, mediana, p95, p99, máximo e desvio padrão.

## Como reproduzir

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o study-linux-arm64 .
docker build -t chromium-study:v1 .
./run-matrix.sh                     # matriz completa
python3 analyze.py results          # tabelas consolidadas
```

Variáveis: `CPUS`, `MEM`, `OUT`, `IMG`, `PROFILE=canonical|no-isolation`.

Para escalas que esta máquina não alcança (concorrência 128+, nodes de
16 vCPU/64 GiB, soak de 24 h), o mesmo container roda como Job/Pod sem alteração:
o binário só precisa de um endpoint CDP e de um diretório de saída.

## O que este estudo NÃO mediu

Declarado para que nenhuma tabela seja lida além do que ela suporta:

- **amd64** — tudo aqui é arm64;
- **concorrência acima de 16** — a VM do Docker tinha 5,79 GiB;
- **soak de 1 h / 6 h / 24 h** — não executado;
- **nodes reais de Kubernetes** — o modelo de capacidade é extrapolação
  aritmética dos números por unidade, e está rotulado como tal;
- **Playwright-Go em execução** — auditado na fonte, não executado;
- **baseline de terceiros** — nenhum benchmark de terceiro foi usado como fonte.
