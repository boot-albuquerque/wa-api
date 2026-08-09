# chromium-study

Estudo experimental que sustenta a **ADR-0006** — escolha da engine de browser do
`internal/wa-headless` e custo por sessão.

Isto **não é código de produção**. É o harness de medição e as evidências brutas
que produziram as decisões registradas na ADR.

## Por que é um módulo Go separado

O diretório tem `go.mod` próprio (`module chromium-study`). Um módulo aninhado é
invisível para `go list ./...`, `go build ./...` e `go vet ./...` da raiz — e
portanto para os gates do `make check`.

Isso é deliberado. O harness depende de `go-rod` e de bibliotecas que o `wa-api`
não usa, tem cobertura de teste zero por natureza (é instrumento, não produto), e
não deve entrar no ratchet de cobertura nem no `waclient-facade`.

Para trabalhar nele:

```bash
cd scripts/chromium-study
go build ./...
```

## O que está aqui

| arquivo | conteúdo |
|---|---|
| `RELATORIO-FASE-1-E-2.md` | comparação de controllers, níveis de abstração, falso sucesso |
| `RELATORIO-FASE-3.md` | profiling causal do rod, topologia, SPA real, soak de 60 min |
| `RELATORIO-FASE-4.md` | ConnectionPolicy, custo real do BrowserContext |
| `RELATORIO-FASE-4B.md` | WhatsApp Web: RAM por sessão, UA obrigatório, SingletonLock |
| `METHODOLOGY.md` | regra epistemológica e separação de camadas |
| `main.go`, `controllers.go`, `levels*.go` | harness das Fases 1–2 |
| `p3_*.go` | Fase 3 — profiling, topologia, SPA real, soak |
| `p4_*.go`, `p4b_*.go` | Fase 4/4B — connection policy, contextos, WhatsApp Web |
| `metrics/`, `workload/`, `proxy.go` | cgroup v2, página hostil com ground truth, contador CDP |
| `results*/` | JSONs e perfis pprof de todas as corridas citadas nos relatórios |
| `Dockerfile`, `entrypoint.sh`, `k8s-bench.yaml` | ambiente de medição |

## Estado das conclusões

As decisões com confiança HIGH estão na ADR-0006. O que **não** foi estabelecido
está listado explicitamente no fim de cada relatório — em particular, ao fim da
Fase 4B seguem em aberto: restrição de aba única no WhatsApp Web, recovery/blast
radius, InteractionPolicy, correctness boundary de CPU e soak de 24h.

A decisão corrente é `GO WITH CONDITIONS`.

## Cuidados ao executar

- O Track J (`p4_wa.go`) dirige o **WhatsApp Web com uma conta real**. Nenhum
  caminho de código envia mensagem, e nenhuma conversa é aberta sem `-wa-chat`
  explícito. Use apenas conta autorizada para teste.
- O perfil pareado fica em `wa-session/` e está no `.gitignore`: contém
  credencial de sessão.
- Vários modos sobem Chromium e consomem CPU/RAM de forma agressiva. Os números
  só valem em host sem contenção — a Fase 4 registra uma corrida inteira perdida
  por load average alto na máquina de teste.
