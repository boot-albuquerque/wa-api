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

Cada caminho antigo continua **registado e a responder**. O que muda:

| | canónico | antigo |
|---|---|---|
| é servido? | sim | **sim** |
| está no OpenAPI | sim | **não** |
| onde se encontra | na página `/docs` | na tabela de equivalência do `docs/ENDPOINTS.md` |
| tempo de vida | permanente | sem data de remoção |

### Por que o antigo saiu do contrato, em vez de ficar `deprecated`

A primeira versão desta padronização documentava as duas formas, com a antiga
marcada `deprecated: true`. Ficaram **232 operações para 141 capacidades** — e
o leitor passava a ter de escolher entre `/chat/list` e `/chats/list` em cada
grupo que abrisse.

Documentar as duas contradiz aquilo que a padronização existe para resolver:
**uma operação, um nome**. Um contrato com dois nomes para a mesma coisa não é
mais informativo — é mais ambíguo, e a ambiguidade é o defeito original.

**A distinção que importa**: o caminho antigo foi removido do **contrato**, não
do **serviço**. Um cliente existente continua a funcionar exactamente como
antes; o que deixa de existir é a promessa documentada de que continuará. Quem
integrar de novo lê um nome só.

**Onde o antigo vive agora**: na tabela de equivalência do `docs/ENDPOINTS.md`,
e na nota que abre cada operação canónica — *"Substitui `GET /chat/list`"* —
para que quem chegue com o nome que conhece encontre onde ele foi parar.

**O que o gate garante**: uma rota antiga conta como coberta **apenas** se a
gémea canónica estiver documentada. Apagar a canónica acusa as duas, com a
mensagem a dizê-lo. Nunca há um estado em que uma capacidade servida fique
sem nome nenhum no contrato.

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
