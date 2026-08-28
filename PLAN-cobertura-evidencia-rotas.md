# PLAN — execução em lotes, F239/F282

**Segue o SPEC** (`SPEC-cobertura-evidencia-rotas.md`). Contagem por família,
direto de `docs/OPENAPI-EVIDENCIAS.md` (2026-08-28), das 93 rotas com a
frase-modelo:

| família | rotas | risco típico |
|---|---:|---|
| Grupos | 17 | médio — criar/apagar grupo é descartável; `updateparticipants` já parcialmente medido (F280/F286) |
| Envio de mensagens | 15 | baixo — texto/mídia para "recebe", descartável |
| Canais | 15 | baixo — canal descartável (já usado em F261/F265) |
| Conversas | 12 | baixo/médio — inclui apagar mensagem, arquivar, fixar |
| Contactos e utilizadores | 10 | **alto** — inclui bloquear/desbloquear (F264, já parcialmente medido), privacidade |
| Sessões | 8 | **alto** — `/session/connect`, `/disconnect`, `/logout`, `/pairphone`, `/proxy`: deixar a sessão fora do ar é o próprio teste |
| Integrações e configuração | 6 | baixo — webhook/S3/HMAC, já costuma ter evidência específica em rascunho |
| Saúde | 4 | nenhum — `/health*`, provavelmente já é ✅ genuíno, só falta a frase |
| Comunidades | 4 | médio — igual a Grupos, já parcialmente medido (F286) |
| Administração | 2 | baixo — `/admin/*`, sessões descartáveis |
| **total** | **93** | |

## Fase 0 — infraestrutura (SPEC §1-2), antes de medir a primeira rota

1. Adicionar as três colunas (`data`, `observador`, `evidência`) a
   `api/openapi/evidencias.tsv`, com as linhas ainda não medidas em
   branco.
2. Escrever o gerador da tabela de `docs/OPENAPI-EVIDENCIAS.md` a partir
   da TSV (SPEC §2) — escopo mínimo: só a tabela, não a prosa.
3. Gate: rodar o gerador, comparar contra o documento atual sem mudança
   de conteúdo (só formato/fonte) — confirma que a migração não perdeu
   informação já existente (a coluna "Evidência" das 43 rotas que já têm
   texto específico, herdada da `CAMPANHA-DESCARTAVEL.md`).
4. `go build`, `go vet`, `gofmt`, suíte completa verdes antes de começar
   a medir.

**Esta fase é código novo — não é medição.** Tem o mesmo rigor de
qualquer mudança nesta sessão: teste, controle negativo quando aplicável,
`HOUSEKEEP.md` se achar algo incidental.

## Fases de medição — ordem sugerida

Menor risco e maior familiaridade primeiro (já testei rotas destas
famílias nesta sessão inteira), risco alto por último e com confirmação
explícita antes de entrar:

| fase | família | rotas | pré-requisito |
|---|---|---:|---|
| 1 | Saúde + Administração | 6 | nenhum — provavelmente fecha em poucos minutos |
| 2 | Integrações e configuração | 6 | nenhum |
| 3 | Envio de mensagens | 15 | nenhum — "envia" → "recebe", texto/mídia, confirma por `websocket` em "recebe" |
| 4 | Canais | 15 | nenhum — mesmo canal de teste descartável já usado |
| 5 | Comunidades | 4 | nenhum — mesmo padrão do F286 |
| 6 | Grupos | 17 | nenhum, mas é a maior fase — pode split em 2 sessões |
| 7 | Conversas | 12 | confirmar antes: inclui apagar mensagem/conversa, ações com efeito visível no cliente |
| 8 | Contactos e utilizadores | 10 | **confirmar antes**: bloqueio/desbloqueio e privacidade tocam configuração da conta real |
| 9 | Sessões | 8 | **confirmar antes de entrar nesta fase** — decisão explícita separada (ver abaixo) |

Cada fase é um lote no sentido do SPEC §3 — meço tudo dentro dela, num
turno (ou mais, se o número de rotas exigir, respeitando a parada por
número fixo). Fases pequenas (1, 2, 5) podem ser combinadas num turno só.

## A decisão pendente da Fase 9 (Sessões)

A bateria original (F239) excluiu `/session/*` **por desenho**: usar a
sessão que serve a própria bateria para testar
connect/disconnect/logout/pairphone/proxy arriscava derrubar a conta em
uso no meio da medição. Isso já não é totalmente verdade — "envia" e
"recebe" são descartáveis — mas **ainda são as sessões reais que usei o
resto desta campanha inteira**: derrubar "envia" no meio da Fase 9
interromperia qualquer fase posterior que dependesse dela.

Proposta: Fase 9 por último, e SÓ com confirmação explícita imediatamente
antes — inclui a opção de criar uma TERCEIRA sessão descartável e
pareada, só para a Fase 9, para não arriscar "envia"/"recebe". Fica para
o usuário decidir quando a Fase 9 chegar, não agora.

## Critério de parada por fase (SPEC §3, decisão 3 do RFC)

Número fixo por turno: até **20 rotas medidas** (não 20 "tentadas" — uma
rota que exige parar por reprovação conta para o número, mas encerra o
turno de qualquer forma, ver protocolo de PARAR do SPEC §4). Fases
grandes (Grupos, 17) cabem inteiras num turno; nenhuma fase precisa de
mais de um turno neste plano.

## Saída de cada fase

- `evidencias.tsv` atualizado para as rotas da família.
- `docs/OPENAPI-EVIDENCIAS.md` regenerado.
- `HOUSEKEEP.md` com uma entrada por achado incidental (se houver) —
  nenhuma entrada "resumo de fase", cada achado é a sua própria entrada,
  como sempre.
- Fixtures desfeitos (grupos/canais de teste apagados, participantes
  removidos) — nenhum resíduo em "envia"/"recebe" entre fases.
- Gates verdes (`go build/vet/test`, `gofmt`, `handler-route`, `logcov`,
  `TestHousekeep*`) antes de passar para a próxima fase.

## Fechamento da campanha (depois da Fase 9, ou da última fase decidida)

Uma entrada nova no `HOUSEKEEP.md` fechando F239 e F282, com o total final
(quantas ✅ confirmadas, quantas rebaixadas para 🟡, quantas reprovaram e
viraram achado próprio) e link para as entradas de achado individuais.
