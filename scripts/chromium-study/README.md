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
| `METHODOLOGY.md` | regra epistemológica e separação de camadas |
| `ACHADO-RENDERER-NAO-RESPONSIVO.md` | o achado da Fase 6 que sobreviveu à limpeza dos relatórios fase a fase |
| `main.go`, `controllers.go`, `levels*.go` | harness das Fases 1–2 |
| `p3_*.go` | Fase 3 — profiling, topologia, SPA real, soak |
| `p4_*.go`, `p4b_*.go` | Fase 4/4B — connection policy, contextos, WhatsApp Web |
| `metrics/`, `workload/`, `proxy.go` | cgroup v2, página hostil com ground truth, contador CDP |
| `results*/` | JSONs e perfis pprof de todas as corridas citadas nos relatórios |
| `Dockerfile`, `entrypoint.sh`, `k8s-bench.yaml` | ambiente de medição |

## Estado das conclusões

As decisões com confiança HIGH, e as condições para `GO WITH CONDITIONS` em
produção, estão registradas em `docs/adr/0006-wa-headless-engine-de-browser-e-custo-por-sessao.md`
e `docs/adr/0007-decisao-final-wa-headless-escopo-e-custo.md` — as ADRs são a
fonte de verdade; os relatórios fase a fase que as sustentaram (Fases 1 a 6)
foram removidos em 2026-08-28 por serem histórico de campanha já incorporado
às ADRs, exceto `ACHADO-RENDERER-NAO-RESPONSIVO.md`, mantido por ser um
achado autônomo (Fase 6, nó H1).

Resumo do que a Fase 4C estabeleceu: a restrição de aba única (uma sessão
ativa por perfil), e a causa real da perda de credencial — o
**desligamento** do Chromium, não invalidação pelo WhatsApp. SIGTERM corrompe
o estado de sessão; o browser tem de ser encerrado por `Browser.close` via
CDP.

## Cuidados ao executar

- O Track J (`p4_wa.go`) dirige o **WhatsApp Web com uma conta real**. Nenhum
  caminho de código envia mensagem, e nenhuma conversa é aberta sem `-wa-chat`
  explícito. Use apenas conta autorizada para teste.
- O perfil pareado fica em `wa-session/` e está no `.gitignore`: contém
  credencial de sessão.
- Vários modos sobem Chromium e consomem CPU/RAM de forma agressiva. Os números
  só valem em host sem contenção — a Fase 4 registra uma corrida inteira perdida
  por load average alto na máquina de teste.
