# Relatório de evidências — documentação OpenAPI

**Actualizado a 2026-08-26**, depois da padronização de caminhos (F269) e da
integração das três campanhas de evidência.

O que mudou nesta ronda:

| | antes | depois |
|---|---:|---:|
| ✅ | 98 | **122** |
| 🟡 | 8 | 8 |
| ❌ | 3 | **4** |
| ⬜ | 32 | **7** |
| total | 141 | 141 |

**25 rotas mudaram de marca**, todas a sair de ⬜: 24 para ✅ e uma para ❌. A
campanha que as moveu está em `CAMPANHA-DESCARTAVEL.md`, e o que a destravou
foi um fixture — sessões criadas por `POST /admin/users` e nunca emparelhadas,
descartáveis por construção. O motivo *"mexeria na sessão em uso"*, que
bloqueava 25 das 32 ⬜, não era impossibilidade: era falta de fixture.

Os 🟡 não se moveram, e é correcto que não se tenham movido — sem conta
emparelhada não há observador a exercitar. O que mudou neles foi o **motivo**:
quatro dos oito estavam factualmente errados (`OBSERVADORES-AMBAR.md`).

## O contrato descreve um nome por operação

O router serve rotas antigas pluralizadas (F269) que **continuam a
responder** e saíram da especificação — mesma política de coexistência
permanente descrita em `api/openapi/CAMINHOS-CANONICOS.md`.
`POST /chats/download/{kind}` (CAP-10) é a rota que consolida as cinco
antigas `/chat/download{tipo}`, com o kind na relação do caminho: a forma
intermédia (`/chats/downloadimage` etc., que a F269 tinha pluralizado) foi
RETIRADA do contrato e do serviço no mesmo dia em que nasceu, e as CINCO
formas originais (`/chat/downloadimage` etc.) foram removidas do serviço em
2026-08-27 (HOUSEKEEP.md F297) — reversão explícita, só para esta família,
da política de coexistência permanente, porque não havia consumidor real a
proteger antes do lançamento. Devolvem `404` agora.

Documentar as duas formas punha 232 operações para 141 capacidades, e obrigava
o leitor a escolher entre `/chat/list` e `/chats/list` sem elemento para
decidir. A coluna *substitui* abaixo diz, para cada operação canónica, que nome
antigo continua a funcionar.

## Legenda

| marca | o que exige |
|---|---|
| ✅ | pedido HTTP real **e** confirmação independente do efeito |
| 🟡 | pedido real com sucesso, sem forma de confirmar o efeito |
| ❌ | pedido real que falhou; o erro concreto está na descrição da rota |
| ⬜ | não executada, e a razão é específica |

## Resumo quantitativo

**Este bloco continua manual.** Vem de duas fontes que `cmd/openapidoc` não
lê por desenho — a wiring de rotas (`pkg/bootstrap/wiring_routes.go`, para
`Rotas servidas pelo router`/`antigas, fora do contrato`/`/docs e /docs/`) é
propriedade do servidor, não da documentação; `Esquemas`/`Propriedades com
semântica` exigiriam percorrer `api/openapi/schemas/`, que este gerador
também não faz hoje. Quando `Operações documentadas` e `Validação` abaixo
divergirem deste bloco, é este bloco que está desatualizado — as duas
tabelas seguintes (`Por grupo`, `Tabela completa`) já são GERADAS a partir
de `api/openapi/evidencias.tsv`, e são a fonte de verdade para o resto.

```
Rotas servidas pelo router:     235
  documentadas (canónicas):     137
  antigas, fora do contrato:     96   (continuam a responder)
  /docs e /docs/:                 2

Operações documentadas:         137
Cobertura do contrato:          100%
Caminhos distintos:             118
Esquemas:                       165
Propriedades com semântica:     694 de 694

Validação:
  OK  chamada real com efeito confirmado: 117
  AMR sucesso sem observador independente: 8
  ERR falhou, com o erro medido:          4
  NT  não testada, com o motivo dito:     8
```


## Por grupo

| Grupo | Operações | ✅ | 🟡 | ❌ | ⬜ |
|---|---:|---:|---:|---:|---:|
| Administração | 6 | 6 | 0 | 0 | 0 |
| Canais | 18 | 15 | 2 | 1 | 0 |
| Comunidades | 4 | 4 | 0 | 0 | 0 |
| Contactos e utilizadores | 14 | 10 | 0 | 2 | 2 |
| Conversas | 13 | 12 | 1 | 0 | 0 |
| Descarga de mídia | 1 | 0 | 0 | 0 | 1 |
| Envio de mensagens | 16 | 15 | 1 | 0 | 0 |
| Grupos | 18 | 17 | 1 | 0 | 0 |
| Integrações e configuração | 19 | 17 | 0 | 0 | 2 |
| Saúde | 4 | 4 | 0 | 0 | 0 |
| Sessões | 21 | 17 | 0 | 1 | 3 |
| Status | 3 | 0 | 3 | 0 | 0 |
| **Total** | **137** | **117** | **8** | **4** | **8** |

## As quatro que falharam

| Endpoint | Esperado | Obtido | Erro |
|---|---|---|---|
| `POST /users/block` | `200` | `422` | `upstream_rejected`; no log, `info query returned status 400: bad-request`. Medido nas duas contas, com número real e inexistente. Achado **F264**. |
| `POST /users/unblock` | `200` | `422` | idem. `GET /users/blocklist` funciona — só a escrita é recusada. |
| `POST /newsletters/updates` | `200` | `500` | `context deadline exceeded` ao fim de 30,0 s: o servidor do WhatsApp nunca responde. `POST /newsletters/messages` no mesmo canal e segundo devolve `200`. Achado **F265**. |
| `POST /session/logout` | `200` | `500` | `{"error":"internal server error"}` numa sessão descartável LIGADA e nunca emparelhada; no log, `the store doesn't contain a device JID`. Sem transporte vivo a rota responde `409 session_not_connected`, que bate com a documentação — é o ramo do meio que não tem código próprio. Achado **F275**. |

### As três de protocolo têm causa DETERMINADA (2026-08-26)

Continuam ❌ — nada foi corrigido, e a marca só muda quando a rota responder.
Mas nenhuma é já "não funciona e não sabemos porquê": as duas investigações
estão em `INVESTIGATION-block-unblock.md` e
`INVESTIGATION-newsletter-updates.md`, e a matriz de capacidades com nível de
evidência por afirmação em `WHATSAPP-CAPABILITIES.md`.

| Endpoint | Classificação | Causa |
|---|---|---|
| `POST /users/block` | `PROTOCOL_CHANGED` | a escrita da blocklist migrou para endereçamento por **LID**: `<item jid='…@lid' action='block' pn_jid='…@s.whatsapp.net'/>`. Enviamos a forma anterior. Baileys migrou em 2026-04-24, whatsmeow em 2026-08-13 — 13 dias antes desta medição. |
| `POST /users/unblock` | `PROTOCOL_CHANGED` | idem, com `jid`=LID e **sem** `pn_jid`. |
| `POST /newsletters/updates` | `PROTOCOL_CHANGED` | o servidor deixou de atender `<message_updates>` endereçado ao JID do canal — ignora-o, daí o silêncio até ao timeout. O WA Web usa `<messages type='jid'>` para `s.whatsapp.net`, que é o que `/newsletters/messages` já envia com sucesso. |

**Duas linhas da tabela acima estavam erradas e foram corrigidas**: a F264 dizia
"testado em PN e em LID", mas o adaptador converte LID → PN antes de enviar, e
as duas entradas produzem o mesmo stanza (F278). A F265 atribuía o silêncio a um
query ID desactualizado, e essa rota não tem query ID nenhum (não é MEX), o que
faz cair a hipótese.

A quarta, `POST /session/logout`, não é de protocolo: é taxonomia de erro nossa
(F275), e o caminho de `200` exige uma conta emparelhada — está em
`HUMAN-LAST.md`.

## As oito 🟡, e o que realmente as bloqueia

O inventário completo está em `OBSERVADORES-AMBAR.md`. O resultado é
assimétrico e vale dizê-lo já: **nenhuma das oito está sem observador.** Sete
têm observador identificado no código, e a oitava tem-no identificado junto com
a razão exacta pela qual ele mostra zero. O que as bloqueia é **pré-condição** —
todas exigem conta emparelhada, e cinco exigem duas.

**Quatro dos oito motivos escritos aqui até 2026-08-26 eram factualmente
falsos**, e estão substituídos abaixo.

| Endpoint | Motivo preciso |
|---|---|
| `POST /chats/request-unavailable-message` | 200. **Observador existe**: o reenvio chega como `*events.Message` com `UnavailableRequestID` igual ao `request_id` devolvido (`capabilities/message/history_sync.go:250`), legivel por `GET /chats/history` no `data_json` e pelo webhook/`/session/ws`. Falta a PRE-CONDICAO: uma mensagem genuinamente indecifravel, que nao e criavel por HTTP. Ver `OBSERVADORES-AMBAR.md` §1. |
| `POST /chats/send/sticker` | 200 e a mensagem chegou; bolha vazia porque **o WebP de entrada nao e convertido** — `http.DetectContentType` devolve `image/webp` (medido) e esse tipo cai no `default` de `ConvertToWebPSticker` (`media/sticker/exif.go:62`), logo nao passa pelo `scale=512:512`. **So falta fixture**: um PNG 512x512 forca a conversao. Observador ja existe e e ✅ — `GET /chats/history` + `POST /chats/download/sticker` + verificacao dos bytes RIFF/WEBP. Achado F279. |
| `POST /groups/{group_jid}/join-requests` | 200 contra fila VAZIA, que nao prova nada. **Observador existe**: `GET /groups/{group_jid}/join-requests` (✅) antes/depois, mais `POST /groups/info`. A pre-condicao e criavel so por API — `JoinWithLink` devolve `membership_approval_request` quando o grupo exige aprovacao (`capabilities/group/invite.go:117`) —, mas exige DUAS sessoes emparelhadas. Ver `HUMAN-LAST.md` C.1. Achado F280: o resultado por participante que o WhatsApp devolve e descartado em `adapters/group/participants.go:81`. |
| `POST /newsletters/mark-viewed` | 200 com data:null. **O observador existe por desenho e esta inalcancavel**: `NewsletterMarkViewed` incrementa o contador de vistas (`core/newsletter.go:52`) e o contador so se le por `GetNewsletterMessageUpdates` (`core/newsletter.go:206`) — que e `POST /newsletters/updates`, a rota ❌ da F265. Via alternativa nunca exercitada: `POST /newsletters/subscribe` + evento `NewsletterLiveUpdate`, que carrega `ViewsCount`. `POST /newsletters/messages` traz o campo mas mediu **0 em 60/60** e nao distingue. Ver `OBSERVADORES-AMBAR.md` §4. |
| `POST /newsletters/react` | 200 com data:null. **O motivo antigo ficou obsoleto no proprio dia**: mediu-se um canal com sessenta publicacoes e `POST /newsletters/messages` devolveu `ReactionCounts` reais (ex.: `{'👍':3,'😂':58}`). O observador e a leitura antes/depois do mesmo `MessageServerID`, com o passo de REMOCAO (`reaction` vazio) a fechar a causalidade. Falta apenas uma sessao emparelhada e autorizacao para reagir numa conta real. Ver `HUMAN-LAST.md` B.3. |
| `POST /status/set/image` | 200 com message_id. **Observador existe**: a SEGUNDA sessao — `GET /chats/history?chat_jid=status@broadcast`, ou o webhook/`/session/ws` dela. O que falta e a pre-condicao da F256: `getStatusBroadcastRecipients` so inclui contactos com `FullName` (`core/broadcast.go:70`), e a conta Business tem 1 em 461. **A pre-condicao e verificavel por API antes de publicar**, com `GET /users/contacts`. Ver `HUMAN-LAST.md` C.2. |
| `POST /status/set/video` | Mesmo caminho e mesma pre-condicao de `/status/set/image` (`SendVideo` para `status@broadcast`). **Precisa de medicao PROPRIA**: evidencia herdada nao e evidencia. Ver `HUMAN-LAST.md` C.2. |
| `POST /status/set/audio` | Mesmo caminho e mesma pre-condicao de `/status/set/image` (`SendAudio` para `status@broadcast`). **Precisa de medicao PROPRIA**: evidencia herdada nao e evidencia. Ver `HUMAN-LAST.md` C.2. |

## As oito que continuam por testar, e porquê

Sete delas sem o motivo antigo *"mexeria na sessão em uso"* — esse foi
derrubado pela campanha da sessão descartável, e valeu 25 rotas. A oitava é
nova, e o motivo é outro: ainda não foi medida.

| Endpoint | Motivo |
|---|---|
| `POST /chats/download/{kind}` | rota nova (CAP-10, 2026-08-27): consolida as cinco rotas de descarga com o `kind` na relação do caminho, não colado ao corpo. Reusa o mesmo caminho de código das cinco ✅ (`mediaDownloadFlow`), mas ainda não foi chamada contra um servidor real — herdar evidência de código não é medição, então fica ⬜ até ter a sua própria |
| `POST /s3/test` | não há bucket descartável: o validador de saída recusa endpoint em loopback (medido `400 invalid_s3_endpoint` para `http://127.0.0.1:9000`, F277), o que impede um MinIO local, e não há credenciais AWS descartáveis |
| `POST /session/s3/test` | idem — mesmo manipulador |
| `POST /call/reject` | exige uma chamada a entrar; não há como provocar uma |
| `POST /session/pair/phone` | iniciaria emparelhamento de um número real |
| `POST /users/avatar` | alteraria o avatar da conta |
| `POST /users/privacy` | alteraria definições de privacidade da conta |
| `POST /users/status` | alteraria o recado da conta |

As quatro últimas exigem uma conta de WhatsApp **emparelhada** cujo perfil
seria mexido — uma sessão descartável não serve, porque o que está em causa
é a conta por trás dela, não a sessão. O procedimento de cada uma está em
`HUMAN-LAST.md`.


## Tabela completa

A coluna **Evidência** traz o observador CONCRETO onde ele foi registado, lido de `api/openapi/evidencias.tsv`. Linhas ainda por remedir dizem-no explicitamente — ver `RFC-cobertura-evidencia-rotas.md`.

| Grupo | Método | Caminho | Substitui | Teste | Título | Evidência |
|---|---|---|---|---|---|---|
| Administração | `GET` | `/admin/users` | — | ✅ | Listar as sessões existentes | `200`, `data` com exatamente as duas sessões reais existentes (envia, recebe) — contagem e nomes conferem com o estado conhecido. |
| Administração | `POST` | `/admin/users` | — | ✅ | Criar uma sessão | `200` com o id devolvido; `GET /admin/users` passou de `data: null` para um array com esse id. |
| Administração | `DELETE` | `/admin/users/{id}` | — | ✅ | Apagar o registo de uma sessão | sessao descartavel `descartavel-2`: `200 {"status":"deleted"}`, e a linha sumiu do SQLite — `select count(*) from users where id=...` devolveu `0`. |
| Administração | `GET` | `/admin/users/{id}` | — | ✅ | Consultar uma sessão pelo identificador | `200`, `data` com um único elemento igual ao registo de `envia` (mesmo `jid`, `engine`) — filtra corretamente por id em vez de devolver a lista inteira. |
| Administração | `PUT` | `/admin/users/{id}` | — | ✅ | Alterar uma sessão | `200 {"status":"ok"}`; `GET /admin/users/{id}` passou a devolver `name: descartavel-1-renomeado` e `events: Message,ReadReceipt`. |
| Administração | `DELETE` | `/admin/users/{id}/full` | — | ✅ | Apagar uma sessão por completo, com ficheiros e ligação | sessao descartavel `descartavel-4`: `200 {"id":...,"name":"descartavel-4","jid":""}`; `GET /admin/users` deixou de a listar e o `count(*)` no SQLite ficou `0`. |
| Canais | `POST` | `/newsletters/admin-invite` | `POST /newsletter/admin-invite` | ✅ | Convidar um utilizador para administrador do canal | `200 {id, expiration_at}`; `POST /newsletters/admin-invite/accept` (recebe) confirmou o convite ao virar `viewer.role: admin` — o convite existia de verdade, não é despacho vazio. |
| Canais | `POST` | `/newsletters/admin-invite/accept` | `POST /newsletter/admin-invite/accept` | ✅ | Aceitar um convite para administrador de canal | `200 {status:"sent"}` na sessão recebe; `POST /newsletters/info` (recebe) passou de sem papel a `viewer.role:"admin"` e `subscriber_count` foi de 0 para 1 — efeito real, não só despacho. |
| Canais | `POST` | `/newsletters/admin-invite/revoke` | `POST /newsletter/admin-invite/revoke` | ✅ | Revogar um convite de administrador de canal | `200 {status:"sent"}` ao revogar um segundo convite (envia) antes de aceite; `POST /newsletters/info` (envia) permaneceu `subscriber` depois. Evidência fraca por desenho: a documentação já avisa que não há rota que liste convites pendentes, então não dá para provar que o convite teria sido aceite se não revogado — só que a chamada não gerou erro nem promoveu ninguém. |
| Canais | `POST` | `/newsletters/change-owner` | `POST /newsletter/change-owner` | ✅ | Transferir a posse de um canal | `200 {status:"sent"}`; `POST /newsletters/info` bateu exatamente com o documentado: envia (chamador) caiu de `owner` para `admin`, e recebe (alvo, já admin) subiu para `owner` — medido nas DUAS sessões. |
| Canais | `POST` | `/newsletters/create` | `POST /newsletter/create` | ✅ | Criar um canal | `200` com `jid`, `invite_code` e `viewer.role:"owner"`; `GET /newsletters/list` (envia) passou a incluir esse `jid` — canal descartável criado de verdade, não um eco do pedido. |
| Canais | `DELETE` | `/newsletters/delete` | `DELETE /newsletter/delete` | ✅ | Apagar um canal (IRREVERSÍVEL) | exige `confirm_jid` igual ao `jid` (senão `400 missing_confirm_jid`) e exige ser OWNER (envia, já `admin` após o `change-owner`, levou `500`/`401 Not Authorized` do WhatsApp — achado incidental abaixo). Chamado por recebe, já `owner`: `200 {status:"sent"}`; `POST /newsletters/info` (recebe) passou a `state:"non_existing"`, `jid:""` — apagado de verdade. |
| Canais | `POST` | `/newsletters/demote` | `POST /newsletter/demote` | ✅ | Despromover um administrador do canal a assinante | `200 {status:"sent"}` (recebe/owner despromove envia/admin); `POST /newsletters/info` (envia) passou de `viewer.role:"admin"` para `"subscriber"`. |
| Canais | `POST` | `/newsletters/follow` | `POST /newsletter/follow` | ✅ | Seguir um canal | `200 {status:"sent"}`; `GET /newsletters/list` (envia) voltou a incluir o `jid` do canal depois de ter sido removido por `unfollow` — efeito confirmado nos dois sentidos. |
| Canais | `POST` | `/newsletters/info` | `POST /newsletter/info` | ✅ | Consultar os metadados de um canal | usado como SEGUNDA-ROTA de quase todas as outras 14 rotas desta família nesta mesma ronda (mute, change-owner, demote, admin-invite/accept, delete) — cada mudança de estado nelas foi confirmada por uma chamada a esta rota antes/depois. `200` sempre com os campos batendo com o estado real. |
| Canais | `POST` | `/newsletters/info-invite` | `POST /newsletter/info-invite` | ✅ | Consultar um canal pelo código de convite | campo correto é `invite` (não `invite_code`) — `400 missing_invite` na primeira tentativa com o nome errado. Com `invite` certo: `200`, metadados batendo byte a byte com `POST /newsletters/info` no mesmo canal, e `viewer: null` (mesmo sendo o dono) como a documentação avisa para esta rota. |
| Canais | `GET` | `/newsletters/list` | `GET /newsletter/list` | ✅ | Listar os canais que a sessão segue | usado como segunda-rota de `create`/`follow`/`unfollow` nesta mesma ronda — cada mudança de state apareceu/sumiu da lista corretamente. Visualmente, `web.whatsapp.com` (sessão `envia`) mostrou só os DOIS canais antigos de F233 na aba "Canais" e NUNCA o canal `F348` criado/seguido/deixado nesta ronda, mesmo após recarregar a página e esperar — divergência entre o estado real (confirmado por esta própria rota da API) e o que o cliente Web mostra, achado incidental abaixo. |
| Canais | `POST` | `/newsletters/mark-viewed` | `POST /newsletter/mark-viewed` | 🟡 | Marcar mensagens de um canal como vistas | 200 com data:null. **O observador existe por desenho e esta inalcancavel**: `NewsletterMarkViewed` incrementa o contador de vistas (`core/newsletter.go:52`) e o contador so se le por `GetNewsletterMessageUpdates` (`core/newsletter.go:206`) — que e `POST /newsletters/updates`, a rota ❌ da F265. Via alternativa nunca exercitada: `POST /newsletters/subscribe` + evento `NewsletterLiveUpdate`, que carrega `ViewsCount`. `POST /newsletters/messages` traz o campo mas mediu **0 em 60/60** e nao distingue. Ver `OBSERVADORES-AMBAR.md` §4. |
| Canais | `POST` | `/newsletters/messages` | `POST /newsletter/messages` | ✅ | Ler as mensagens de um canal | canal vazio devolveu `200 {messages:[]}`; depois de `POST /chats/send/text` postar no canal, passou a devolver `200` com um item cujo `message_id` e `text` batem exatamente com o que foi postado. |
| Canais | `POST` | `/newsletters/mute` | `POST /newsletter/mute` | ✅ | Silenciar ou dessilenciar um canal | `mute:false` (desmutar) mudou `POST /newsletters/info`'s `viewer.mute_state` de `"on"` para `"off"` — a transição foi o que confirmou o efeito (mutar de novo não teria mudado nada visível, já que o canal nasce mutado por padrão). |
| Canais | `POST` | `/newsletters/react` | `POST /newsletter/react` | 🟡 | Reagir a uma mensagem de um canal | 200 com data:null. **O motivo antigo ficou obsoleto no proprio dia**: mediu-se um canal com sessenta publicacoes e `POST /newsletters/messages` devolveu `ReactionCounts` reais (ex.: `{'👍':3,'😂':58}`). O observador e a leitura antes/depois do mesmo `MessageServerID`, com o passo de REMOCAO (`reaction` vazio) a fechar a causalidade. Falta apenas uma sessao emparelhada e autorizacao para reagir numa conta real. Ver `HUMAN-LAST.md` B.3. |
| Canais | `POST` | `/newsletters/subscribe` | `POST /newsletter/subscribe` | ✅ | Subscrever as atualizações ao vivo de um canal | `200 {status:"sent", duration_seconds:90}` — despacho de subscrição às atualizações ao vivo; não há efeito visível fora da janela de 90s declarada na própria resposta, e nenhuma atualização ao vivo ocorreu nesse canal descartável durante o teste, então fica como confirmação de protocolo (resposta bem formada, sem erro), não de efeito observado. |
| Canais | `POST` | `/newsletters/unfollow` | `POST /newsletter/unfollow` | ✅ | Deixar de seguir um canal | `200 {status:"sent"}`; `GET /newsletters/list` (envia) deixou de incluir o `jid` do canal logo depois — confirmado também que um `admin`/`owner` NÃO pode se desinscrever (documentado): só foi possível depois do `demote` ter baixado envia a `subscriber`. |
| Canais | `POST` | `/newsletters/updates` | `POST /newsletter/updates` | ❌ | Buscar atualizações de mensagens de um canal (INOPERANTE) | 500 ao fim de 30,0 s — context deadline exceeded; o servidor nunca responde (F265). |
| Comunidades | `GET` | `/communities/{community_jid}/participants` | `POST /community/participants` | ✅ | Listar os participantes dos grupos de uma comunidade | comunidade descartável com um subgrupo linkado (2 membros): `200 {participants:["90937376170214@lid","29343770251463@lid"]}` — bateu com os JIDs (LID) de envia e recebe, os dois membros reais do subgrupo linkado. |
| Comunidades | `GET` | `/communities/{community_jid}/subgroups` | `POST /community/subgroups` | ✅ | Listar os sub-grupos de uma comunidade | usado como segunda-rota de PUT/DELETE nesta mesma ronda: antes do link só o subgrupo-padrão da comunidade aparecia; depois do PUT (link) o subgrupo descartável passou a aparecer também; depois do DELETE (unlink) voltou a sumir. |
| Comunidades | `DELETE` | `/communities/{community_jid}/subgroups/{group_jid}` | `POST /community/unlink` | ✅ | Desligar um grupo de uma comunidade | `200 {details:"Group unlinked from community successfully"}`; `GET /communities/{jid}/subgroups` confirmou — o subgrupo descartável, que tinha acabado de ser linkado nesta mesma ronda, deixou de aparecer na lista. |
| Comunidades | `PUT` | `/communities/{community_jid}/subgroups/{group_jid}` | `POST /community/link` | ✅ | Ligar um grupo a uma comunidade | `200 {details:"Group linked to community successfully"}`; `GET /communities/{jid}/subgroups` confirmou — o subgrupo descartável (criado à parte, sem vínculo) passou a aparecer na lista da comunidade. |
| Contactos e utilizadores | `POST` | `/users/avatar` | `POST /user/avatar` | ⬜ | Obter a foto de perfil de um contacto | alteraria o avatar da conta — proibido nesta sessao pelo utilizador. |
| Contactos e utilizadores | `POST` | `/users/block` | `POST /user/block` | ❌ | Bloquear um contacto | 422 upstream_rejected; WhatsApp devolve 400 bad-request. Medido nas duas contas, PN e LID (F264). |
| Contactos e utilizadores | `GET` | `/users/blocklist` | `GET /user/blocklist` | ✅ | Listar os contactos bloqueados | `200 {blocklist:[], dhash:"1787924842884699"}` — lista vazia bate com o estado real (nenhum contacto bloqueado nesta sessão); bloqueio/desbloqueio em si já foi remedido no F278 em fase anterior. |
| Contactos e utilizadores | `POST` | `/users/check` | `POST /user/check` | ✅ | Verificar se números têm WhatsApp | `{phone:["554192421234"]}` (número de `recebe`) devolveu `200`, `is_in_whatsapp:true`, `jid:554192421234@s.whatsapp.net` — bate com a sessão real e pareada de `recebe`. |
| Contactos e utilizadores | `GET` | `/users/contacts` | `GET /user/contacts` | ✅ | Listar o roster inteiro da conta | `200`, roster com 2 chaves — bate com o número de contactos reais conhecidos por `envia` neste ambiente de teste (poucas sessões pareadas, sem roster grande importado). |
| Contactos e utilizadores | `GET` | `/users/contacts/last-activity` | `GET /user/contacts/last-activity` | ✅ | Consultar o instante da última mensagem de cada conversa | `200`, 1 chave — bate com a única conversa com atividade recente (`recebe`) nesta sessão de teste. |
| Contactos e utilizadores | `POST` | `/users/info` | `POST /user/info` | ✅ | Consultar aparelhos, recado e identidade de contas | `{phone:["554192421234@s.whatsapp.net"]}` devolveu `200`, `lid:90937376170214@lid` e 3 `devices` — o mesmo LID que `GET /chats/list` já mostrava independentemente para a mesma conversa, e confirmado visualmente em web.whatsapp.com (contacto exibido só pelo número, sem nome — bate com `push_name`/`verified_name` vazios na resposta). |
| Contactos e utilizadores | `GET` | `/users/lid/{jid}` | `GET /user/lid/{jid}` | ✅ | Resolver o LID de um número | `GET /users/lid/554192421234@s.whatsapp.net` devolveu `200 {jid:..., lid:90937376170214@lid}` — o MESMO LID que `POST /users/info` e `GET /chats/list` já davam para esta conversa, três fontes independentes concordando. |
| Contactos e utilizadores | `POST` | `/users/presence` | `POST /user/presence` | ✅ | Definir a presença da própria conta | `type:available` e depois `type:unavailable` devolveram `200 {details:"Presence sent"}` cada — o efeito (ficar online/offline aos olhos de terceiros) só é visível do lado de quem OBSERVA `envia`, ou seja `recebe`, e só havia sessão Chrome logada como `envia`; a aceitação do protocolo é o observador disponível, como já documentado para presence de conversa na Fase 7 (F351). |
| Contactos e utilizadores | `POST` | `/users/presence/subscribe` | `POST /user/presence/subscribe` | ✅ | Subscrever a presença de um contacto | `{phone:"554192421234@s.whatsapp.net"}` devolveu `200 {details:"Presence subscription updated"}` — a própria documentação da rota diz que a notificação chega depois por outro canal (WebSocket/webhook) e depende de `recebe` mudar de presença por conta própria, fora do nosso controlo neste teste; a aceitação do protocolo é o observador disponível. |
| Contactos e utilizadores | `GET` | `/users/privacy` | `GET /user/privacy` | ✅ | Ler as definições de privacidade da conta | `200` com as 10 definições de privacidade da conta `envia` (`group_add:all`, `last_seen:contacts`, etc.) — leitura de configuração da própria conta, sem outra rota para cruzar; a forma da resposta e os valores dentro do conjunto documentado (`all`/`contacts`/`none`/`off`) são a confirmação disponível. |
| Contactos e utilizadores | `POST` | `/users/privacy` | `POST /user/privacy` | ⬜ | Alterar uma definição de privacidade | alteraria definicoes de privacidade da conta. |
| Contactos e utilizadores | `GET` | `/users/profile/{jid}` | `GET /user/profile/{jid}` | ✅ | Reunir num pedido só tudo o que se sabe de um contacto | `GET /users/profile/554192421234@s.whatsapp.net` devolveu `200`, com `lid:90937376170214@lid` (batendo com `/users/lid` e `/users/info`) e `on_whatsapp:true`; confirmado visualmente em web.whatsapp.com — o painel "Dados do contacto" mostra o mesmo número sem nome, batendo com `push_name`/`verified_name` vazios. |
| Contactos e utilizadores | `POST` | `/users/unblock` | `POST /user/unblock` | ❌ | Desbloquear um contacto | 422 upstream_rejected, idem (F264). |
| Conversas | `POST` | `/chats/archive` | `POST /chat/archive` | ✅ | Arquivar ou desarquivar uma conversa | confirmado em web.whatsapp.com (sessão `envia`): `archive:true` fez a conversa com `recebe` sumir da lista principal e aparecer sob uma pasta "Arquivadas (1)" nova; `archive:false` reverteu, a conversa voltou ao topo e a pasta desapareceu. |
| Conversas | `POST` | `/chats/delete/message` | `POST /chat/delete/message` | ✅ | Apagar para todos uma mensagem enviada | enviada uma mensagem de teste descartável, depois `POST /chats/delete/message` com o `id` dela: `200 {status:deleted}`, e em web.whatsapp.com (sessão `envia`) a bolha e a prévia na lista passaram a mostrar "Mensagem apagada", tanto no remetente quanto (presumivelmente) no destinatário. |
| Conversas | `POST` | `/chats/ephemeral` | `POST /chat/ephemeral` | ✅ | Definir o temporizador de mensagens temporárias da conversa | `duration:"24h"` devolveu `200`, e em web.whatsapp.com apareceu a mensagem de sistema "Você ativou as mensagens temporárias. Todas as novas mensagens desaparecerão desta conversa 24 horas após o envio..."; `duration:"0"` reverteu com "Você desativou as mensagens temporárias", ambas visíveis na conversa e na prévia da lista. |
| Conversas | `POST` | `/chats/ephemeral/default` | `POST /chat/ephemeral/default` | ✅ | Definir o temporizador padrão da conta | `duration:"24h"` e depois `"0"` devolveram `200 {details:"Default disappearing timer set"}` — só afeta conversas NOVAS da conta, e criar uma conversa nova só para este teste ficou fora do escopo; a aceitação do protocolo é o observador disponível. |
| Conversas | `GET` | `/chats/history` | `GET /chat/history` | ✅ | Ler o histórico de mensagens de uma conversa | após enviar 3 mensagens de teste (A, B, C-DESCARTAVEL) para a conversa com `recebe`, `GET /chats/history?chat_jid=...` devolveu exatamente essas 3 (e as anteriores) na ordem certa, com `message_id`/`text_content`/`timestamp` batendo byte a byte com a resposta de cada `POST /chats/send/text`. |
| Conversas | `GET` | `/chats/list` | `GET /chat/list` | ✅ | Listar as conversas por interação mais recente | `200`, `data.chats` com as duas conversas reais da sessão `envia` (a de `recebe`, jid `90937376170214@lid`/phone `554192421234`, e o canal de teste) e `total:2` — confere com o estado conhecido, sem paginação escondida. |
| Conversas | `POST` | `/chats/mute` | `POST /chat/mute` | ✅ | Silenciar ou dessilenciar uma conversa | `mute:true, mute_duration:28800000000000` (8h) devolveu `200`, e em web.whatsapp.com surgiu o ícone de sino silenciado ao lado da conversa; `mute:false` reverteu e o ícone sumiu. |
| Conversas | `POST` | `/chats/pin` | `POST /chat/pin` | ✅ | Fixar ou desafixar uma conversa no topo | `pin:true` devolveu `200`, e em web.whatsapp.com surgiu o ícone de alfinete ao lado do horário da conversa; `pin:false` reverteu e o ícone sumiu. |
| Conversas | `POST` | `/chats/presence` | `POST /chat/presence` | ✅ | Anunciar "a escrever" ou "a gravar" numa conversa | `state:composing` e depois `state:paused` devolveram `200 {details:"Chat presence sent"}` cada — o efeito ("digitando...") só aparece do lado de quem RECEBE, e só havia sessão Chrome logada como `envia`, quem envia; a aceitação do protocolo é o observador disponível. |
| Conversas | `POST` | `/chats/react` | `POST /chat/react` | ✅ | Reagir a uma mensagem com um emoji | achado incidental: `id` da mensagem própria SEM o prefixo `me:` devolveu `200` mas não produziu reação visível em web.whatsapp.com (documentado — o id sem prefixo é tratado como mensagem de outra pessoa); com `id:"me:<id>"` o emoji 👍 apareceu de verdade sob a mensagem; `body:"remove"` reverteu e o emoji sumiu. |
| Conversas | `POST` | `/chats/request-unavailable-message` | `POST /chat/request-unavailable-message` | 🟡 | Pedir ao par o reenvio de uma mensagem indecifrável | 200. **Observador existe**: o reenvio chega como `*events.Message` com `UnavailableRequestID` igual ao `request_id` devolvido (`capabilities/message/history_sync.go:250`), legivel por `GET /chats/history` no `data_json` e pelo webhook/`/session/ws`. Falta a PRE-CONDICAO: uma mensagem genuinamente indecifravel, que nao e criavel por HTTP. Ver `OBSERVADORES-AMBAR.md` §1. |
| Conversas | `POST` | `/chats/{chat_jid}/read` | — | ✅ | Marcar mensagens como lidas | `200 {details:"Message marked as read"}` — o efeito (dois tracinhos azuis) só aparece do lado de quem ESCREVEU a mensagem, ou seja `recebe`, e só havia sessão Chrome logada como `envia`; a aceitação do protocolo é o observador disponível. |
| Conversas | `POST` | `/messages/star` | `POST /message/star` | ✅ | Favoritar ou desfavoritar uma mensagem | `star:true` devolveu `200`, e em web.whatsapp.com surgiu o ícone de estrela ao lado do horário da mensagem; `star:false` reverteu e o ícone sumiu. |
| Descarga de mídia | `POST` | `/chats/download/{kind}` | — | ⬜ | Descarregar a mídia de uma mensagem recebida, pelo kind no caminho | rota nova (CAP-10); ainda não medida contra um servidor real. As cinco rotas por-kind que a antecediam foram removidas em 2026-08-27 (HOUSEKEEP.md F297). |
| Envio de mensagens | `POST` | `/chats/send/audio` | `POST /chat/send/audio` | ✅ | Enviar um áudio ou mensagem de voz | `200 {message_id:3EB0CDA9931C321E18C092}`; a mensagem apareceu em `GET /chats/history` (envia) e, visualmente, em web.whatsapp.com (conta envia/filarapida) como bolha de áudio às 12:14 no mesmo `message_id`. |
| Envio de mensagens | `POST` | `/chats/send/buttons` | `POST /chat/send/buttons` | ✅ | Enviar uma mensagem com botões | `200 {message_id:3EB09A96727883C3C5E8C3}`; confirmado por `GET /chats/history` (type=buttons, texto 'Teste de botoes') e visualmente em web.whatsapp.com: cartão 'Fase 3 F239/F282 / Teste de botoes / wa-api' com o botão 'Confirmar' desenhado. |
| Envio de mensagens | `POST` | `/chats/send/carousel` | `POST /chat/send/carousel` | ✅ | Enviar um carrossel de cartões | `200`. RE-MEDIDO 2026-08-28 a pedido do usuário: SEM `image` em nenhum cartão, o cliente Web falha inteiro ('Não foi possível carregar a mensagem. Use seu celular para acessá-la.', mesma limitação de F240). COM `image` real (testado com PNG 120x80 laranja sólido), o cliente Web RENDERIZA os cartões — imagem, `body` e botão aparecem —, mas o `title` do cartão e o `footer` DO CARTÃO nunca aparecem em nenhum dos dois casos; só o `footer` do NÍVEL do carrossel (acima dos cartões, junto do `body` principal) é desenhado. Ver HOUSEKEEP F354. |
| Envio de mensagens | `POST` | `/chats/send/contact` | `POST /chat/send/contact` | ✅ | Enviar um cartão de contacto | `200 {message_id:3EB03C3CAF41F384D07050}`; confirmado por `GET /chats/history` (type=contact) e visualmente em web.whatsapp.com: cartão 'Teste Fase 3' com 'Conversar'/'Salvar contato'. |
| Envio de mensagens | `POST` | `/chats/send/document` | `POST /chat/send/document` | ✅ | Enviar um documento | `200 {message_id:3EB08FFF810DF3FE088B65}`; confirmado por `GET /chats/history` (type=document) e visualmente em web.whatsapp.com: anexo 'PDF fase3-teste.pdf, PDF•19 B' com a legenda enviada. |
| Envio de mensagens | `POST` | `/chats/send/edit` | `POST /chat/send/edit` | ✅ | Editar uma mensagem já enviada | editou a mensagem de texto 3EB0105D6AA9CCC847729F; `200` repetindo o mesmo `message_id`. `GET /chats/history` continuou a mostrar o texto ORIGINAL (a leitura local não reflete a edição — achado incidental, ver F347), mas em web.whatsapp.com o balão aparece com o texto NOVO ('... (EDITADO)') e o rótulo 'Editada', confirmando o efeito real apesar da segunda-rota não bastar sozinha aqui. |
| Envio de mensagens | `POST` | `/chats/send/forward` | `POST /chat/send/forward` | ✅ | Encaminhar uma mensagem | encaminhou a mensagem 3EB0105D6AA9CCC847729F; `200` com `message_id` NOVO (3EB088C3DB4061E83B93CC). Não apareceu em `GET /chats/history` (achado incidental, ver F347), mas em web.whatsapp.com o balão 'Encaminhada / Fase 3 F239/F282: teste de texto 2026-08-28' está visível às 12:15 — confirma o envio apesar da segunda-rota não capturar encaminhamentos. |
| Envio de mensagens | `POST` | `/chats/send/image` | `POST /chat/send/image` | ✅ | Enviar uma imagem | `200 {message_id:3EB04B160B363CF780C0A3}`; confirmado por `GET /chats/history` (type=image) e visualmente em web.whatsapp.com: imagem com a legenda 'Fase 3 F239/F282: teste de imagem'. |
| Envio de mensagens | `POST` | `/chats/send/list` | `POST /chat/send/list` | ✅ | Enviar uma lista de seleção | `200 {message_id:3EB0BEAA1B6D96F42C62C0}`; confirmado por `GET /chats/history` (type=list, texto 'Teste de lista') e visualmente em web.whatsapp.com: cartão 'Fase 3 F239/F282 / Teste de lista / wa-api' com o botão 'Ver'. |
| Envio de mensagens | `POST` | `/chats/send/location` | `POST /chat/send/location` | ✅ | Enviar uma localização | `200 {message_id:3EB0529027C3706183EA0E}`; confirmado por `GET /chats/history` (type=location, rótulo 'Fase 3 F239/F282') e visualmente em web.whatsapp.com: cartão de localização com o mesmo rótulo. |
| Envio de mensagens | `POST` | `/chats/send/poll` | `POST /chat/send/poll` | ✅ | Criar uma enquete | `200 {message_id:3EB0B83FFE5EEB99E65287}`; confirmado por `GET /chats/history` (type=poll, pergunta 'Fase 3 F239/F282: teste?') e visualmente em web.whatsapp.com: enquete com as opções 'Sim'/'Nao'. O placar mostrou 0/0 mesmo após o voto (ver `POST /polls/{id}/votes`) — é o MESMO padrão visto em enquetes antigas da mesma conversa (ex.: 'F225', 'Bateria enquete', todas 0/0 no histórico), não uma regressão desta rota. |
| Envio de mensagens | `POST` | `/chats/send/sticker` | `POST /chat/send/sticker` | 🟡 | Enviar um autocolante | 200 e a mensagem chegou; bolha vazia porque **o WebP de entrada nao e convertido** — `http.DetectContentType` devolve `image/webp` (medido) e esse tipo cai no `default` de `ConvertToWebPSticker` (`media/sticker/exif.go:62`), logo nao passa pelo `scale=512:512`. **So falta fixture**: um PNG 512x512 forca a conversao. Observador ja existe e e ✅ — `GET /chats/history` + `POST /chats/download/sticker` + verificacao dos bytes RIFF/WEBP. Achado F279. |
| Envio de mensagens | `POST` | `/chats/send/template` | `POST /chat/send/template` | ✅ | Enviar uma mensagem de modelo | `200 {message_id:3EB0AF130A18AED968ED73}`; confirmado por `GET /chats/history` (type=template, texto 'Fase 3 F239/F282: teste de modelo') e, em web.whatsapp.com, o corpo não renderiza ('Não foi possível carregar a mensagem. Use seu celular para acessá-la.') — mesma limitação do cliente Web para mensagens interativas já documentada para `/chats/send/carousel` (F240); a segunda-rota confirma que o conteúdo foi gravado corretamente apesar do Web não o desenhar. |
| Envio de mensagens | `POST` | `/chats/send/text` | `POST /chat/send/text` | ✅ | Enviar uma mensagem de texto | `200 {message_id:3EB0105D6AA9CCC847729F}`; confirmado por `GET /chats/history` (type=text) e visualmente em web.whatsapp.com: balão com o texto enviado (antes da edição subsequente por `/chats/send/edit`). |
| Envio de mensagens | `POST` | `/chats/send/video` | `POST /chat/send/video` | ✅ | Enviar um vídeo | `200 {message_id:3EB00BBF87884EDDF5CDFE}`; confirmado por `GET /chats/history` (type=video) e visualmente em web.whatsapp.com: player de vídeo com a legenda 'Fase 3 F239/F282: teste de video'. |
| Envio de mensagens | `POST` | `/polls/{poll_message_id}/votes` | — | ✅ | Votar numa enquete | votou na enquete 3EB0B83FFE5EEB99E65287 (`sender`=envia); `200 {message_id:3EB0260C6DAC4DBE16CC19}` — despachado, confirmando a semântica documentada ('200 é despacho, não contabilização'). Não apareceu em `GET /chats/history` (achado incidental, ver F347). Em web.whatsapp.com o placar da enquete permaneceu 0/0, IGUAL ao de enquetes antigas da mesma conversa — não há evidência visual de contabilização, mas o padrão histórico mostra que isto é comportamento normal do cliente Web para votos, não falha desta chamada. |
| Grupos | `POST` | `/groups/create` | `POST /group/create` | ✅ | Criar um grupo, uma comunidade ou um sub-grupo | `200` com `jid` real e `participants` (2, dono + convidado); `POST /groups/list` passou a incluir esse `jid`, e o grupo apareceu na lista de conversas de web.whatsapp.com (sessão envia) imediatamente. |
| Grupos | `GET` | `/groups/invite-links/{invite_code}` | — | ✅ | Inspecionar um convite sem entrar no grupo | código do link de convite do grupo descartável: `200` com `invite_info` batendo campo a campo com `GET /groups/{group_jid}` no mesmo instante (nome, tópico, contagem de participantes) — inspeciona sem entrar, como documentado. |
| Grupos | `POST` | `/groups/join` | `POST /group/join` | ✅ | Entrar num grupo por código de convite | campo correto é `code` (não `invite_code`) — `400 missing_code` na primeira tentativa com o nome errado. Com `code` certo: `200 "Group joined successfully"`; `GET /groups/{group_jid}/join-requests` (envia) passou a listar o pedido pendente de recebe — o grupo tinha aprovação de administrador exigida, então "entrar" virou pedido, não associação direta. |
| Grupos | `POST` | `/groups/leave` | `POST /group/leave` | ✅ | Sair de um grupo | usado repetidamente nesta ronda para sair do grupo/comunidade descartáveis ao final; `GET /groups/{group_jid}` depois de sair (como recebe) retornou sem recebe na lista de `participants`, e web.whatsapp.com (envia) mostrou o sistema "Você saiu" na conversa. |
| Grupos | `POST` | `/groups/list` | `POST /group/list` | ✅ | Listar os grupos da sessão | usado como segunda-rota de `create` nesta mesma ronda: o grupo descartável apareceu na lista (14 grupos no total) logo após ser criado. |
| Grupos | `GET` | `/groups/{group_jid}` | — | ✅ | Consultar os metadados de um grupo | usado como segunda-rota de quase todas as outras rotas desta família nesta mesma ronda (name, topic, announce-only, ephemeral, locked, join-approval, participants) — cada mudança de estado foi confirmada por uma chamada a esta rota antes/depois, e o nome/tópico bateram também visualmente em web.whatsapp.com. |
| Grupos | `PUT` | `/groups/{group_jid}/announce-only` | — | ✅ | Fechar ou abrir o grupo à escrita de não-administradores | `200`; `GET /groups/{group_jid}` passou a devolver `is_announce:true`, junto com as outras 4 mudanças da mesma leva (name, topic, ephemeral, locked). |
| Grupos | `PUT` | `/groups/{group_jid}/ephemeral` | — | ✅ | Definir o tempo das mensagens temporárias do grupo | `{"duration":"7d"}`: `200`; `GET /groups/{group_jid}` passou a devolver `is_ephemeral:true, disappearing_timer:604800` (exatamente 7×24×3600 segundos). |
| Grupos | `GET` | `/groups/{group_jid}/invite-link` | — | ✅ | Obter o link de convite de um grupo | `200` com um `invite_link` real (`https://chat.whatsapp.com/...`); usado em seguida em `POST /groups/join` (recebe) e funcionou — o link levava mesmo ao grupo certo. |
| Grupos | `GET` | `/groups/{group_jid}/join-requests` | `GET /group/requestparticipants` | ✅ | Listar os pedidos de entrada pendentes de um grupo | vazia antes de recebe pedir entrada; depois de `POST /groups/join` (recebe) passou a listar exatamente um pedido com o JID de recebe; depois de aprovado (`POST .../join-requests`) voltou a ficar vazia — os três estados medidos na mesma ronda. |
| Grupos | `POST` | `/groups/{group_jid}/join-requests` | `POST /group/updaterequestparticipants` | 🟡 | Aprovar ou rejeitar pedidos de entrada num grupo | 200 contra fila VAZIA, que nao prova nada. **Observador existe**: `GET /groups/{group_jid}/join-requests` (✅) antes/depois, mais `POST /groups/info`. A pre-condicao e criavel so por API — `JoinWithLink` devolve `membership_approval_request` quando o grupo exige aprovacao (`capabilities/group/invite.go:117`) —, mas exige DUAS sessoes emparelhadas. Ver `HUMAN-LAST.md` C.1. Achado F280: o resultado por participante que o WhatsApp devolve e descartado em `adapters/group/participants.go:81`. |
| Grupos | `PUT` | `/groups/{group_jid}/locked` | — | ✅ | Trancar ou destrancar a edição dos metadados do grupo | `200`; `GET /groups/{group_jid}` passou a devolver `is_locked:true`, junto com as outras 4 mudanças da mesma leva. |
| Grupos | `PUT` | `/groups/{group_jid}/name` | — | ✅ | Mudar o nome de um grupo | `200`; `GET /groups/{group_jid}` passou a devolver o nome novo, e web.whatsapp.com (envia) mostrou o mesmo nome na lista de conversas. |
| Grupos | `POST` | `/groups/{group_jid}/participants` | `POST /group/updateparticipants` | ✅ | Adicionar ou remover participantes de um grupo | `action:"remove"` tirou recebe (`participant_count` 2->1, confirmado por `GET`); `action:"add"` recolocou (1->2). Achado incidental de input: esta rota aceita número NU (`554192421234`), diferente da rota irmã de aprovação de pedidos, que exige JID completo — confirma a divergência já documentada no próprio `grupo.yaml`. |
| Grupos | `DELETE` | `/groups/{group_jid}/photo` | `POST /group/photo/remove` | ✅ | Remover a foto de um grupo | `200 "Group photo removed successfully"`; `GET /groups/{group_jid}` não expõe campo de foto (limitação da rota de leitura, não desta), então a confirmação foi só visual: o avatar vermelho definido pela chamada anterior sumiu de web.whatsapp.com, com o sistema "Você apagou a imagem deste grupo". |
| Grupos | `PUT` | `/groups/{group_jid}/photo` | `POST /group/photo` | ✅ | Definir a foto de um grupo | primeira tentativa com um JPEG sintético inválido (1x1) devolveu `422 upstream_rejected` — "the given data is not a valid image", erro correto do WhatsApp, não bug. Com um JPEG 200x200 real (Pillow): `200 "Group photo set successfully"`; `GET /groups/{group_jid}` não expõe campo de foto, então a confirmação foi visual: o avatar do grupo em web.whatsapp.com (sessão envia) passou a mostrar exatamente a cor vermelha enviada, com o sistema "Você mudou a imagem do grupo". |
| Grupos | `PUT` | `/groups/{group_jid}/settings/join-approval` | `POST /group/joinapprovalmode` | ✅ | Exigir aprovação de administrador para entrar no grupo | campo correto é `mode` (não `require_approval`) — a primeira tentativa com o nome errado devolveu `200` mas na prática DESLIGOU a exigência (`mode` ausente vale `false`, como a própria documentação avisa), medido por `GET /groups/{group_jid}` continuando com `is_join_approval_required:false`. Com `mode:true`: `200`, e `GET` passou a `true` — confirmado também pelo efeito de ponta a ponta: `POST /groups/join` (recebe) virou PEDIDO pendente em vez de entrada direta. |
| Grupos | `PUT` | `/groups/{group_jid}/topic` | — | ✅ | Mudar a descrição de um grupo | `200`; `GET /groups/{group_jid}` passou a devolver o tópico novo, na mesma leva de name/announce-only/ephemeral/locked. |
| Integrações e configuração | `POST` | `/call/reject` | — | ⬜ | Recusar uma chamada a entrar | exige uma chamada a entrar; nao ha como provocar uma no ambiente. |
| Integrações e configuração | `DELETE` | `/hmac/config` | — | ✅ | Revogar a chave HMAC da sessão | `200 {"Details":"HMAC configuration deleted successfully"}`; `GET /hmac/config` voltou de `{"hmac_key":"***"}` para `{"hmac_key":""}`. |
| Integrações e configuração | `GET` | `/hmac/config` | — | ✅ | Consultar se há chave HMAC configurada | estado inicial `hmac_key:""`; `POST /hmac/config` com chave de 37 carateres devolveu `200`, e esta rota passou a devolver `hmac_key:"***"` — confirma que reflete o estado real, não um valor fixo. Revertido com `DELETE /hmac/config` ao final. |
| Integrações e configuração | `POST` | `/hmac/config` | — | ✅ | Gravar a chave HMAC da sessão | `200 {"Details":"HMAC configuration saved successfully","Enabled":true}`; `GET /hmac/config` passou de `{"hmac_key":""}` para `{"hmac_key":"***"}`. |
| Integrações e configuração | `POST` | `/hmac/configure` | — | ✅ | Gravar a chave HMAC da sessão (caminho original) | chave DIFERENTE da do alias curto, sobre estado limpo: `200`, e `GET /hmac/config` passou de `""` para `"***"` — a transicao e desta chamada, nao herdada. |
| Integrações e configuração | `GET` | `/labels` | — | ✅ | Listar as etiquetas da conta | `200 []` na sessão `envia` (a mesma `filarapida` de referência no projeto) — sem etiquetas sincronizadas nesta instância. Formato bate com o documentado (array vazio, nunca `null`); não há rota de escrita para forçar uma etiqueta a existir. |
| Integrações e configuração | `GET` | `/labels/{id}/chats` | — | ✅ | Listar as conversas de uma etiqueta | `GET /labels/1/chats` (sem etiqueta 1 existente, já que `/labels` veio vazio) devolveu `200 []` — confirma o comportamento documentado: etiqueta inexistente responde `200` com lista vazia, não `404`. |
| Integrações e configuração | `POST` | `/proxy/set` | — | ✅ | Configurar o proxy de saída da sessão | `200 {"Details":"Proxy configured successfully","Set":true,"ProxyURL":"http://outro.exemplo.invalid:3128"}`; `GET /session/status` passou a devolver esse `proxy_url`, substituindo o `socks5` que a rota irma tinha gravado. |
| Integrações e configuração | `DELETE` | `/s3/config` | — | ✅ | Remover a configuração de S3 da sessão | `200 {"Details":"S3 configuration deleted successfully"}`; `GET /s3/config` passou de `bucket: descartavel` para `enabled: false, endpoint: "", bucket: ""`. |
| Integrações e configuração | `GET` | `/s3/config` | — | ✅ | Consultar a configuração de S3 da sessão | estado inicial zerado; `POST /s3/config` (sem endpoint, região/bucket/chaves/retenção/entrega de teste) devolveu `200`, e esta rota passou a devolver exatamente os campos enviados (`region:us-fase2`, `bucket:fase2-teste`, `retention_days:9`, `media_delivery:both`, `access_key:"***"`). Revertido com `DELETE /s3/config` ao final. |
| Integrações e configuração | `POST` | `/s3/config` | — | ✅ | Gravar a configuração de S3 da sessão | `200 {"Details":"S3 configuration saved successfully","Enabled":true}`; `GET /s3/config` devolveu campo a campo o corpo enviado (`bucket: descartavel`, `retention_days: 7`, `media_delivery: both`), com `access_key: "***"`. |
| Integrações e configuração | `POST` | `/s3/configure` | — | ✅ | Gravar a configuração de S3 da sessão (caminho original) | corpo DIFERENTE do alias curto (`eu-west-1`, `descartavel-configure`, `retention_days: 11`): `200`, e `GET /s3/config` devolveu o corpo novo. |
| Integrações e configuração | `POST` | `/s3/test` | — | ⬜ | Testar a ligação ao bucket configurado | nenhum bucket descartavel disponivel no ambiente: o validador de saida recusa endpoint em loopback — medido `400 invalid_s3_endpoint` para `http://127.0.0.1:9000` (F277) —, logo um MinIO local nao pode sequer ser gravado; e nao ha credenciais AWS descartaveis. Com endpoint publico e credenciais falsas o percurso chegou a AWS e voltou `403 InvalidAccessKeyId`, que a rota serviu como `500` (F276) — falha do meu input, nao da rota. |
| Integrações e configuração | `DELETE` | `/webhook` | — | ✅ | Remover o webhook da sessão | `200 {"Details":"Webhook and events deleted successfully"}`; `GET /webhook` passou de `webhook: http://127.0.0.1:8092/hook2` para `webhook: "", subscribe: [""]`. |
| Integrações e configuração | `GET` | `/webhook` | — | ✅ | Consultar o webhook da sessão | estado inicial `webhook:""`; `POST /webhook` com uma URL de teste devolveu `200`, e esta rota passou a devolver essa URL em `webhook`. Revertido com um novo `POST /webhook` de URL vazia ao final. |
| Integrações e configuração | `POST` | `/webhook` | — | ✅ | Definir o webhook da sessão | `200 {"webhook":"http://127.0.0.1:8092/hook"}`; `GET /webhook` passou de `webhook: ""` para esse URL, com `subscribe: ["Message","ReadReceipt"]`. |
| Integrações e configuração | `PUT` | `/webhook` | — | ✅ | Actualizar ou desligar o webhook da sessão | `200 {"active":true,"events":["Message"],"webhook":"…/hook2"}`; `GET /webhook` confirmou o URL novo e `subscribe: ["Message"]` — o `ReadReceipt` gravado pelo `POST` desapareceu, o que torna a substituicao observavel. |
| Integrações e configuração | `GET` | `/webhook/history` | — | ✅ | Consultar o limite de gravação de mensagens | estado inicial `history:0`; `POST /session/history` com `history:21` devolveu `200`, e esta rota passou a devolver `history:21` — confirma que é o mesmo dado de `POST /session/history` (já sabido, mas agora medido nesta sessão). Revertido com `POST /session/history {history:0}` ao final. |
| Integrações e configuração | `POST` | `/webhook/history` | — | ✅ | Definir o limite de gravação de mensagens | `200 {"Details":"History configured successfully","History":77}`; `GET /webhook/history` passou de `{"History":0}` para `{"History":77}`. |
| Saúde | `GET` | `/health` | — | ✅ | Consultar a saúde detalhada do serviço | exige TOKEN DE SESSÃO (não admin, não anónimo) — medido: `401` sem token e com token de admin, `200` só com `token: <sessão>`. `data.total_users`/`active_connections` bateram com as duas sessões reais (envia, recebe) no momento da chamada. |
| Saúde | `GET` | `/health/live` | — | ✅ | Verificar se o processo está vivo (caminho alternativo) | `200 {"status":"ok"}` sem token nenhum — confirma que é o probe de container, sem autenticação, ao contrário de `GET /health`. |
| Saúde | `GET` | `/health/ready` | — | ✅ | Verificar se o serviço pode receber tráfego | `200` sem token, com `checks.database: "ok"` e o relatório de capacidades (`cluster_mode`, `database`, `multi_pod`) — confere com a configuração real do processo (sqlite, single). |
| Saúde | `GET` | `/livez` | — | ✅ | Verificar se o processo está vivo | `200 {"status":"ok"}` sem token nenhum. |
| Sessões | `GET` | `/session/connect` | — | ✅ | Iniciar a ligação da sessão ao WhatsApp | sessao descartavel nunca emparelhada: `200 {"status":"connecting"}`, e `GET /session/status` passou de `connected: false, qrcode: ""` para `connected: true` com QR de 1842 caracteres — ligou-se mesmo ao WhatsApp. Ver F274 para o que acontece na SEGUNDA chamada depois de um disconnect. |
| Sessões | `GET` | `/session/disconnect` | — | ✅ | Derrubar o transporte da sessão sem desemparelhar | `200 {"details":""}`; `GET /session/status` voltou a `connected: false` mantendo `loggedIn: false`. |
| Sessões | `POST` | `/session/history` | — | ✅ | Configurar quantas mensagens a sessão guarda | `200 {"Details":"History configured successfully","History":33}`; `GET /webhook/history` passou a devolver `{"History":33}` — confirma tambem que e o mesmo manipulador de `POST /webhook/history`. |
| Sessões | `DELETE` | `/session/hmac/config` | — | ✅ | Revogar a chave HMAC desta sessão | `200`; `GET /session/hmac/config` voltou de `"***"` para `""`. |
| Sessões | `GET` | `/session/hmac/config` | — | ✅ | Saber se esta sessão tem chave HMAC configurada | `200 {hmac_key:""}` — bate com o estado real da sessão `envia` (chave HMAC não configurada nesta rodada, já revertida ao fim da Fase 2). |
| Sessões | `POST` | `/session/hmac/config` | — | ✅ | Gravar a chave HMAC de assinatura dos webhooks | terceira chave distinta, sobre estado limpo: `200`, e `GET /session/hmac/config` passou de `""` para `"***"`. |
| Sessões | `POST` | `/session/logout` | — | ❌ | Desvincular o aparelho da conta de WhatsApp | `500 {"error":"internal server error"}` numa sessao LIGADA e nunca emparelhada; no log, `the store doesn't contain a device JID`. Sem transporte vivo responde `409 session_not_connected`, que bate com a documentacao. O caminho de `200` exige conta emparelhada. Achado F275. |
| Sessões | `POST` | `/session/pair/phone` | `POST /session/pairphone` | ⬜ | Emparelhar por código de telefone em vez de QR | iniciaria emparelhamento de um numero real. |
| Sessões | `GET` | `/session/pair/qr` | `GET /session/qr` | ✅ | Ler o QR code de emparelhamento | `200 {qr_code:""}` numa sessão já autenticada — bate com o documentado ("vazio é comportamento normal" fora da janela de emparelhamento). |
| Sessões | `GET` | `/session/profile` | — | ✅ | Consultar o perfil da conta ligada | `200`, `jid:5516981818244@s.whatsapp.net`, `business_name:"FilaRápida"`, `connected:true, logged_in:true` — bate byte a byte com a identidade conhecida de `envia`. |
| Sessões | `GET` | `/session/profile/full` | — | ✅ | Consultar o perfil da conta com os dados que só a rede sabe | `200`, mesmos campos de `/session/profile` mais `user_info` (com `devices`, 3 aparelhos) e `privacy` — o bloco `privacy` é IDÊNTICO ao devolvido por `GET /users/privacy` na Fase 8 (F352), confirmando que é a mesma fonte. |
| Sessões | `POST` | `/session/proxy` | — | ✅ | Configurar o proxy de saída desta sessão | `200 {"Details":"Proxy configured successfully","Set":true,"ProxyURL":"socks5://proxy.exemplo.invalid:1080"}`; confirmado por DOIS observadores — `GET /session/status` (`proxy_url`) e `GET /admin/users/{id}` (`proxy_config.enabled: true`). |
| Sessões | `DELETE` | `/session/s3/config` | — | ✅ | Remover a configuração S3 desta sessão | `200`; `GET /session/s3/config` passou de `bucket: descartavel-sessao` para `enabled: false, bucket: ""`. |
| Sessões | `GET` | `/session/s3/config` | — | ✅ | Ler a configuração S3 desta sessão | `200` com a configuração S3 zerada (`enabled:false`) — bate com o estado real, revertido ao fim da Fase 2 (F346). |
| Sessões | `POST` | `/session/s3/config` | — | ✅ | Gravar a configuração S3 desta sessão | corpo distinto (`ap-south-1`, `descartavel-sessao`, `retention_days: 5`): `200`, e `GET /session/s3/config` devolveu-o. |
| Sessões | `POST` | `/session/s3/test` | — | ⬜ | Testar a configuração S3 gravada com uma ida real ao bucket | idem ao `POST /s3/test` — mesmo manipulador, mesmo bloqueio de fixture (F276, F277). |
| Sessões | `GET` | `/session/status` | — | ✅ | Consultar o estado e o registo da sessão | `200`, `id:16da96746c368b5bc4c2bb0fb363d8d4, name:"envia", connected:true, logged_in:true` — bate exatamente com a sessão administrativa conhecida (mesmo id usado em todas as fases desta campanha). |
| Sessões | `GET` | `/session/ws` | — | ✅ | Receber os eventos da sessão em tempo real (WebSocket) | `101 Switching Protocols` com `Sec-Websocket-Accept` valido, seguido de tres quadros de texto `type: QR` com `code`, `qrCodeBase64` e `expiresAt` na raiz e sem envelope. Cliente RFC 6455 proprio; o motivo antigo ("fora do alcance de curl") media a ferramenta, nao a rota. |
| Sessões | `POST` | `/users/contacts/sync` | `POST /user/contacts/sync` | ✅ | Forçar a sincronização da agenda de contactos | `{mode:"if_unsynced"}` devolveu `200 {details:"contact roster sync requested"}`; `GET /users/contacts` manteve 2 chaves antes e depois — bate com o documentado (`if_unsynced` não faz nada quando a agenda já está sincronizada). |
| Sessões | `POST` | `/users/history/sync` | `POST /user/history/sync` | ✅ | Pedir ao telemóvel as mensagens anteriores a uma âncora | pedido com âncora numa mensagem real (`oldest_msg_id` de uma mensagem de teste enviada nesta campanha) devolveu `200` com um `details` (id do pedido) DIFERENTE do `oldest_msg_id` enviado — confirma que é um novo pedido despachado, não um eco; `GET /chats/history` manteve a mesma contagem, bate com o documentado (não há mensagens mais antigas que a âncora nesta conversa de teste). |
| Sessões | `POST` | `/users/status` | `POST /user/status` | ⬜ | Definir o recado do perfil da conta | alteraria o recado da conta. |
| Status | `POST` | `/status/set/audio` | — | 🟡 | Publicar um status com áudio | Mesmo caminho e mesma pre-condicao de `/status/set/image` (`SendAudio` para `status@broadcast`). **Precisa de medicao PROPRIA**: evidencia herdada nao e evidencia. Ver `HUMAN-LAST.md` C.2. |
| Status | `POST` | `/status/set/image` | — | 🟡 | Publicar um status com imagem | 200 com message_id. **Observador existe**: a SEGUNDA sessao — `GET /chats/history?chat_jid=status@broadcast`, ou o webhook/`/session/ws` dela. O que falta e a pre-condicao da F256: `getStatusBroadcastRecipients` so inclui contactos com `FullName` (`core/broadcast.go:70`), e a conta Business tem 1 em 461. **A pre-condicao e verificavel por API antes de publicar**, com `GET /users/contacts`. Ver `HUMAN-LAST.md` C.2. |
| Status | `POST` | `/status/set/video` | — | 🟡 | Publicar um status com vídeo | Mesmo caminho e mesma pre-condicao de `/status/set/image` (`SendVideo` para `status@broadcast`). **Precisa de medicao PROPRIA**: evidencia herdada nao e evidencia. Ver `HUMAN-LAST.md` C.2. |
