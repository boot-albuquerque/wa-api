# Caminhos canónicos — a padronização das rotas

**Decisão de 2026-08-26**, a executar a F269 ponto 1 e 2.

## A regra

1. **Plural para colecção**, singular só para singleton — recurso de que existe
   exactamente um naquele contexto.
2. **Relação explícita no caminho**, não colada ao nome:
   `/groups/{group_jid}/join-requests`, não `/group/requestparticipants`.
3. **Método diz a operação**: `PUT` para ligar, `DELETE` para desligar, `GET`
   para ler — em vez de o verbo ir no substantivo.

## Como isto NÃO parte clientes

Cada caminho antigo continua registado e a funcionar. O que muda:

| | canónico | antigo |
|---|---|---|
| existe? | sim | sim |
| no OpenAPI | operação principal | `deprecated: true`, a apontar para o canónico |
| no `ENDPOINTS.md` | é o que se documenta | listado na tabela de equivalência |
| tempo de vida | permanente | até uma remoção **anunciada**, que ainda não foi decidida |

Não há data de remoção. Marcar `deprecated` sem plano de remoção é dizer
"preferimos o outro", que é verdade; anunciar remoção sem decisão seria mentira.

## O que fica singular, e porquê

`/session/*` — o token no cabeçalho **é** o identificador da sessão. Um
`/sessions/{id}/status` teria o identificador duas vezes, e as duas cópias
poderiam discordar.

`/health`, `/livez`, `/webhook`, `/s3/*`, `/hmac/*`, `/proxy/set` — um por
sessão. `/labels` e `/admin/users` já eram plurais e ficam.

`/status/set/*` — é publicação, não colecção endereçável: não há
`/statuses/{id}`.

`/call/reject` — operação sobre um evento efémero que não tem recurso.

## A tabela

Vive em `api/openapi/caminhos.tsv`, e é a **fonte única**: o registo de rotas
lê-a, o gerador de OpenAPI lê-a, e há gate que recusa divergência entre ela e
as rotas realmente servidas.
