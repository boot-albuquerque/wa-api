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

## CAP-10 (2026-08-27) — a segunda ronda, dois casos que a tabela não cobre

A tabela resolve renomeações 1-para-1 com o MESMO manipulador. Dois achados
depois da F269 não são isso, e ficam registados aqui em vez de forçados na
tabela:

**`POST /session/pairphone` → `POST /session/pair/phone`, e
`GET /session/qr` → `GET /session/pair/qr`.** Duas renomeações simples, cabem
na tabela como qualquer outra linha — estão lá. Agrupadas sob `/session/pair/`
porque são as duas formas de emparelhar (QR e código de telefone) — a relação
que antes só a documentação afirmava passa a estar no caminho.

**As cinco rotas de descarga (`/chat/download{tipo}`) → `POST
/chats/download/{kind}`.** Isto NÃO é uma renomeação 1-para-1: eram CINCO
rotas legadas para UMA canónica nova, com um manipulador NOVO
(`DownloadMediaHandler`) que despacha pelo `{kind}` do caminho para o mesmo
`mediaDownloadFlow` que as cinco já usavam. A tabela assume o mesmo
manipulador nos dois lados — por isso esta rota é registada directamente em
`wiring_routes.go`, não via `RegisterCanonicalAliases`, e documentada à mão
em `api/openapi/paths/conversa.yaml`.

**Por que consolidar em vez de só pluralizar** (como a F269 tinha feito com
estas cinco, gerando `/chats/downloadimage` etc.): pluralizar corrigiu a
regra 1 (`/chat` → `/chats`), mas a forma pluralizada continuava a violar a
regra 2 — `downloadimage`, `downloadvideo` etc. são o verbo `download`
colado ao TIPO no nome, exactamente o padrão que a regra 2 proíbe
(`/group/requestparticipants` foi o exemplo original). O tipo já vem no
corpo do cliente (`Message.imageMessage` / `videoMessage` / ... no
`data_json` de `GET /chats/history`) — cinco nomes de rota duplicavam no
CAMINHO uma informação que o cliente já tem para o CORPO. `{kind}` na
RELAÇÃO do caminho, não colado ao nome, é a forma que a regra 2 pede.

**A forma intermédia (`/chats/downloadimage` etc.) foi RETIRADA, do
contrato e do serviço**, no mesmo dia em que nasceu — nenhum cliente real
chegou a depender dela.

### Reversão (2026-08-27, HOUSEKEEP.md F297) — as cinco formas ORIGINAIS já não respondem

Este parágrafo dizia, até esta reversão, que "as cinco formas ORIGINAIS
(singulares, `/chat/downloadimage` etc.) continuam a responder — essas sim
têm histórico e coexistência garantida, mesmo padrão desta página" — ou
seja, aplicava a esta família a política de coexistência permanente descrita
acima (§"Como isto NÃO parte clientes").

**Essa decisão foi revertida por instrução explícita do utilizador**: corte
limpo ("hard cutover"), sem aliases nem rotas antigas a responder, porque
**nenhum consumidor real existe antes do lançamento** — a garantia de
"tempo de vida permanente" não protege ninguém que ainda não integrou. As
cinco rotas (`/chat/downloadimage`, `downloadvideo`, `downloadaudio`,
`downloaddocument`, `downloadsticker`) foram removidas do `wiring_routes.go`
e devolvem `404`; os handlers e a excepção `consolidadaEm` /
`consolidadasCAP10` que as cobriam também foram removidos —
`TestOpenAPICobreTodasAsRotasRegistadas` e
`TestNenhumaFamiliaDeColeccaoFicouNoSingular` já não precisam de exceção
para esta família, porque `registadas`/`Routes(Deps{})` nunca mais as inclui.
O controlo negativo (`TestF297_LegacyDownloadRoutesAreGone`,
`pkg/bootstrap/caminhos_canonicos_test.go`) trava as duas pontas: as cinco
rotas devolvem 404 pelo roteador real, e `/chats/download/{kind}` continua
casada para os cinco kinds.

**Esta reversão é específica da família download, não da política geral**
(§"Como isto NÃO parte clientes" continua a valer para as ~90 renomeações da
tabela `caminhos.tsv`). `POST /chats/download/{kind}` fica ⬜ até ter a sua
própria medição: reusa o código das cinco ✅ que tinham, mas herdar evidência
de código não é medição.
