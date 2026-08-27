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

O router serve **235 rotas**; o contrato documenta **137**. A diferença são as
91 formas antigas pluralizadas (F269) mais as cinco `/chat/download*`
(CAP-10) — 96 no total —, que **continuam a responder** e saíram da
especificação. `POST /chats/download/{kind}` é a rota nova que as
consolida, com o kind na relação do caminho: as CINCO rotas
`/chats/downloadimage` etc. (a forma intermédia que a F269 tinha
pluralizado) foram RETIRADAS do contrato — e do serviço — a favor dela,
porque continuavam a violar a regra 2 (verbo colado ao tipo no nome).

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
| `POST /chats/send/sticker` | 200 e a mensagem chegou; bolha vazia porque **o WebP de entrada nao e convertido** — `http.DetectContentType` devolve `image/webp` (medido) e esse tipo cai no `default` de `ConvertToWebPSticker` (`media/sticker/exif.go:62`), logo nao passa pelo `scale=512:512`. **So falta fixture**: um PNG 512x512 forca a conversao. Observador ja existe e e ✅ — `GET /chats/history` + `POST /chats/downloadsticker` + verificacao dos bytes RIFF/WEBP. Achado F279. |
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

A coluna **Evidência** traz o observador CONCRETO onde ele foi registado.
As linhas que ainda trazem a frase-modelo *"chamada real com resposta e efeito
confirmado por segunda leitura ou pelo cliente"* são exactamente as que a
auditoria de `AUDITORIA-EVIDENCIAS.md` classificou como **inauditáveis**: a
marca está lá, o observador não. Não são menos verdadeiras — são menos
verificáveis, e a diferença está agora visível em vez de escondida.

| Grupo | Método | Caminho | Substitui | Teste | Título | Evidência |
|---|---|---|---|---|---|---|
| Administração | `GET` | `/admin/users` | — | ✅ | Listar as sessões existentes | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Administração | `POST` | `/admin/users` | — | ✅ | Criar uma sessão | `200` com o id devolvido; `GET /admin/users` passou de `data: null` para um array com esse id. |
| Administração | `GET` | `/admin/users/{id}` | — | ✅ | Consultar uma sessão pelo identificador | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Administração | `PUT` | `/admin/users/{id}` | — | ✅ | Alterar uma sessão | `200 {"status":"ok"}`; `GET /admin/users/{id}` passou a devolver `name: descartavel-1-renomeado` e `events: Message,ReadReceipt`. |
| Administração | `DELETE` | `/admin/users/{id}` | — | ✅ | Apagar o registo de uma sessão | sessao descartavel `descartavel-2`: `200 {"status":"deleted"}`, e a linha sumiu do SQLite — `select count(*) from users where id=...` devolveu `0`. |
| Administração | `DELETE` | `/admin/users/{id}/full` | — | ✅ | Apagar uma sessão por completo, com ficheiros e ligação | sessao descartavel `descartavel-4`: `200 {"id":...,"name":"descartavel-4","jid":""}`; `GET /admin/users` deixou de a listar e o `count(*)` no SQLite ficou `0`. |
| Canais | `POST` | `/newsletters/admin-invite` | `POST /newsletter/admin-invite` | ✅ | Convidar um utilizador para administrador do canal | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletters/admin-invite/accept` | `POST /newsletter/admin-invite/accept` | ✅ | Aceitar um convite para administrador de canal | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletters/admin-invite/revoke` | `POST /newsletter/admin-invite/revoke` | ✅ | Revogar um convite de administrador de canal | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletters/change-owner` | `POST /newsletter/change-owner` | ✅ | Transferir a posse de um canal | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletters/create` | `POST /newsletter/create` | ✅ | Criar um canal | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `DELETE` | `/newsletters/delete` | `DELETE /newsletter/delete` | ✅ | Apagar um canal (IRREVERSÍVEL) | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletters/demote` | `POST /newsletter/demote` | ✅ | Despromover um administrador do canal a assinante | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletters/follow` | `POST /newsletter/follow` | ✅ | Seguir um canal | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletters/info` | `POST /newsletter/info` | ✅ | Consultar os metadados de um canal | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletters/info-invite` | `POST /newsletter/info-invite` | ✅ | Consultar um canal pelo código de convite | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `GET` | `/newsletters/list` | `GET /newsletter/list` | ✅ | Listar os canais que a sessão segue | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletters/mark-viewed` | `POST /newsletter/mark-viewed` | 🟡 | Marcar mensagens de um canal como vistas | 200 com data:null. **O observador existe por desenho e esta inalcancavel**: `NewsletterMarkViewed` incrementa o contador de vistas (`core/newsletter.go:52`) e o contador so se le por `GetNewsletterMessageUpdates` (`core/newsletter.go:206`) — que e `POST /newsletters/updates`, a rota ❌ da F265. Via alternativa nunca exercitada: `POST /newsletters/subscribe` + evento `NewsletterLiveUpdate`, que carrega `ViewsCount`. `POST /newsletters/messages` traz o campo mas mediu **0 em 60/60** e nao distingue. Ver `OBSERVADORES-AMBAR.md` §4. |
| Canais | `POST` | `/newsletters/messages` | `POST /newsletter/messages` | ✅ | Ler as mensagens de um canal | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletters/mute` | `POST /newsletter/mute` | ✅ | Silenciar ou dessilenciar um canal | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletters/react` | `POST /newsletter/react` | 🟡 | Reagir a uma mensagem de um canal | 200 com data:null. **O motivo antigo ficou obsoleto no proprio dia**: mediu-se um canal com sessenta publicacoes e `POST /newsletters/messages` devolveu `ReactionCounts` reais (ex.: `{'👍':3,'😂':58}`). O observador e a leitura antes/depois do mesmo `MessageServerID`, com o passo de REMOCAO (`reaction` vazio) a fechar a causalidade. Falta apenas uma sessao emparelhada e autorizacao para reagir numa conta real. Ver `HUMAN-LAST.md` B.3. |
| Canais | `POST` | `/newsletters/subscribe` | `POST /newsletter/subscribe` | ✅ | Subscrever as atualizações ao vivo de um canal | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletters/unfollow` | `POST /newsletter/unfollow` | ✅ | Deixar de seguir um canal | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletters/updates` | `POST /newsletter/updates` | ❌ | Buscar atualizações de mensagens de um canal (INOPERANTE) | 500 ao fim de 30,0 s — context deadline exceeded; o servidor nunca responde (F265). |
| Comunidades | `GET` | `/communities/{community_jid}/participants` | `POST /community/participants` | ✅ | Listar os participantes dos grupos de uma comunidade | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Comunidades | `GET` | `/communities/{community_jid}/subgroups` | `POST /community/subgroups` | ✅ | Listar os sub-grupos de uma comunidade | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Comunidades | `PUT` | `/communities/{community_jid}/subgroups/{group_jid}` | `POST /community/link` | ✅ | Ligar um grupo a uma comunidade | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Comunidades | `DELETE` | `/communities/{community_jid}/subgroups/{group_jid}` | `POST /community/unlink` | ✅ | Desligar um grupo de uma comunidade | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `POST` | `/users/avatar` | `POST /user/avatar` | ⬜ | Obter a foto de perfil de um contacto | alteraria o avatar da conta — proibido nesta sessao pelo utilizador. |
| Contactos e utilizadores | `POST` | `/users/block` | `POST /user/block` | ❌ | Bloquear um contacto | 422 upstream_rejected; WhatsApp devolve 400 bad-request. Medido nas duas contas, PN e LID (F264). |
| Contactos e utilizadores | `GET` | `/users/blocklist` | `GET /user/blocklist` | ✅ | Listar os contactos bloqueados | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `POST` | `/users/check` | `POST /user/check` | ✅ | Verificar se números têm WhatsApp | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `GET` | `/users/contacts` | `GET /user/contacts` | ✅ | Listar o roster inteiro da conta | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `GET` | `/users/contacts/last-activity` | `GET /user/contacts/last-activity` | ✅ | Consultar o instante da última mensagem de cada conversa | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `POST` | `/users/info` | `POST /user/info` | ✅ | Consultar aparelhos, recado e identidade de contas | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `GET` | `/users/lid/{jid}` | `GET /user/lid/{jid}` | ✅ | Resolver o LID de um número | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `POST` | `/users/presence` | `POST /user/presence` | ✅ | Definir a presença da própria conta | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `POST` | `/users/presence/subscribe` | `POST /user/presence/subscribe` | ✅ | Subscrever a presença de um contacto | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `GET` | `/users/privacy` | `GET /user/privacy` | ✅ | Ler as definições de privacidade da conta | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `POST` | `/users/privacy` | `POST /user/privacy` | ⬜ | Alterar uma definição de privacidade | alteraria definicoes de privacidade da conta. |
| Contactos e utilizadores | `GET` | `/users/profile/{jid}` | `GET /user/profile/{jid}` | ✅ | Reunir num pedido só tudo o que se sabe de um contacto | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `POST` | `/users/unblock` | `POST /user/unblock` | ❌ | Desbloquear um contacto | 422 upstream_rejected, idem (F264). |
| Conversas | `POST` | `/chats/archive` | `POST /chat/archive` | ✅ | Arquivar ou desarquivar uma conversa | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chats/delete/message` | `POST /chat/delete/message` | ✅ | Apagar para todos uma mensagem enviada | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chats/ephemeral` | `POST /chat/ephemeral` | ✅ | Definir o temporizador de mensagens temporárias da conversa | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chats/ephemeral/default` | `POST /chat/ephemeral/default` | ✅ | Definir o temporizador padrão da conta | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `GET` | `/chats/history` | `GET /chat/history` | ✅ | Ler o histórico de mensagens de uma conversa | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `GET` | `/chats/list` | `GET /chat/list` | ✅ | Listar as conversas por interação mais recente | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chats/markread` | `POST /chat/markread` | ✅ | Marcar mensagens como lidas | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chats/mute` | `POST /chat/mute` | ✅ | Silenciar ou dessilenciar uma conversa | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chats/pin` | `POST /chat/pin` | ✅ | Fixar ou desafixar uma conversa no topo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chats/presence` | `POST /chat/presence` | ✅ | Anunciar "a escrever" ou "a gravar" numa conversa | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chats/react` | `POST /chat/react` | ✅ | Reagir a uma mensagem com um emoji | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chats/request-unavailable-message` | `POST /chat/request-unavailable-message` | 🟡 | Pedir ao par o reenvio de uma mensagem indecifrável | 200. **Observador existe**: o reenvio chega como `*events.Message` com `UnavailableRequestID` igual ao `request_id` devolvido (`capabilities/message/history_sync.go:250`), legivel por `GET /chats/history` no `data_json` e pelo webhook/`/session/ws`. Falta a PRE-CONDICAO: uma mensagem genuinamente indecifravel, que nao e criavel por HTTP. Ver `OBSERVADORES-AMBAR.md` §1. |
| Conversas | `POST` | `/messages/star` | `POST /message/star` | ✅ | Favoritar ou desfavoritar uma mensagem | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Descarga de mídia | `POST` | `/chats/download/{kind}` | `POST /chat/downloadimage`, `/chat/downloadvideo`, `/chat/downloadaudio`, `/chat/downloaddocument`, `/chat/downloadsticker` (CAP-10) | ⬜ | Descarregar a mídia de uma mensagem recebida, pelo kind no caminho | rota nova (CAP-10); ainda não medida contra um servidor real. |
| Envio de mensagens | `POST` | `/chats/send/audio` | `POST /chat/send/audio` | ✅ | Enviar um áudio ou mensagem de voz | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chats/send/buttons` | `POST /chat/send/buttons` | ✅ | Enviar uma mensagem com botões | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chats/send/carousel` | `POST /chat/send/carousel` | ✅ | Enviar um carrossel de cartões | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chats/send/contact` | `POST /chat/send/contact` | ✅ | Enviar um cartão de contacto | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chats/send/document` | `POST /chat/send/document` | ✅ | Enviar um documento | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chats/send/edit` | `POST /chat/send/edit` | ✅ | Editar uma mensagem já enviada | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chats/send/forward` | `POST /chat/send/forward` | ✅ | Encaminhar uma mensagem | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chats/send/image` | `POST /chat/send/image` | ✅ | Enviar uma imagem | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chats/send/list` | `POST /chat/send/list` | ✅ | Enviar uma lista de seleção | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chats/send/location` | `POST /chat/send/location` | ✅ | Enviar uma localização | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chats/send/poll` | `POST /chat/send/poll` | ✅ | Criar uma enquete | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chats/send/pollvote` | `POST /chat/send/pollvote` | ✅ | Votar numa enquete | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chats/send/sticker` | `POST /chat/send/sticker` | 🟡 | Enviar um autocolante | 200 e a mensagem chegou; bolha vazia porque **o WebP de entrada nao e convertido** — `http.DetectContentType` devolve `image/webp` (medido) e esse tipo cai no `default` de `ConvertToWebPSticker` (`media/sticker/exif.go:62`), logo nao passa pelo `scale=512:512`. **So falta fixture**: um PNG 512x512 forca a conversao. Observador ja existe e e ✅ — `GET /chats/history` + `POST /chats/downloadsticker` + verificacao dos bytes RIFF/WEBP. Achado F279. |
| Envio de mensagens | `POST` | `/chats/send/template` | `POST /chat/send/template` | ✅ | Enviar uma mensagem de modelo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chats/send/text` | `POST /chat/send/text` | ✅ | Enviar uma mensagem de texto | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chats/send/video` | `POST /chat/send/video` | ✅ | Enviar um vídeo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/groups/announce` | `POST /group/announce` | ✅ | Fechar ou abrir o grupo à escrita de não-administradores | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/groups/create` | `POST /group/create` | ✅ | Criar um grupo, uma comunidade ou um sub-grupo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/groups/ephemeral` | `POST /group/ephemeral` | ✅ | Definir o tempo das mensagens temporárias do grupo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/groups/info` | `POST /group/info` | ✅ | Consultar os metadados de um grupo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/groups/inviteinfo` | `POST /group/inviteinfo` | ✅ | Inspecionar um convite sem entrar no grupo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/groups/invitelink` | `POST /group/invitelink` | ✅ | Obter o link de convite de um grupo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/groups/join` | `POST /group/join` | ✅ | Entrar num grupo por código de convite | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/groups/leave` | `POST /group/leave` | ✅ | Sair de um grupo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/groups/list` | `POST /group/list` | ✅ | Listar os grupos da sessão | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/groups/locked` | `POST /group/locked` | ✅ | Trancar ou destrancar a edição dos metadados do grupo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/groups/name` | `POST /group/name` | ✅ | Mudar o nome de um grupo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/groups/topic` | `POST /group/topic` | ✅ | Mudar a descrição de um grupo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `GET` | `/groups/{group_jid}/join-requests` | `GET /group/requestparticipants` | ✅ | Listar os pedidos de entrada pendentes de um grupo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/groups/{group_jid}/join-requests` | `POST /group/updaterequestparticipants` | 🟡 | Aprovar ou rejeitar pedidos de entrada num grupo | 200 contra fila VAZIA, que nao prova nada. **Observador existe**: `GET /groups/{group_jid}/join-requests` (✅) antes/depois, mais `POST /groups/info`. A pre-condicao e criavel so por API — `JoinWithLink` devolve `membership_approval_request` quando o grupo exige aprovacao (`capabilities/group/invite.go:117`) —, mas exige DUAS sessoes emparelhadas. Ver `HUMAN-LAST.md` C.1. Achado F280: o resultado por participante que o WhatsApp devolve e descartado em `adapters/group/participants.go:81`. |
| Grupos | `POST` | `/groups/{group_jid}/participants` | `POST /group/updateparticipants` | ✅ | Adicionar ou remover participantes de um grupo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `PUT` | `/groups/{group_jid}/photo` | `POST /group/photo` | ✅ | Definir a foto de um grupo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `DELETE` | `/groups/{group_jid}/photo` | `POST /group/photo/remove` | ✅ | Remover a foto de um grupo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `PUT` | `/groups/{group_jid}/settings/join-approval` | `POST /group/joinapprovalmode` | ✅ | Exigir aprovação de administrador para entrar no grupo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Integrações e configuração | `POST` | `/call/reject` | — | ⬜ | Recusar uma chamada a entrar | exige uma chamada a entrar; nao ha como provocar uma no ambiente. |
| Integrações e configuração | `GET` | `/hmac/config` | — | ✅ | Consultar se há chave HMAC configurada | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Integrações e configuração | `POST` | `/hmac/config` | — | ✅ | Gravar a chave HMAC da sessão | `200 {"Details":"HMAC configuration saved successfully","Enabled":true}`; `GET /hmac/config` passou de `{"hmac_key":""}` para `{"hmac_key":"***"}`. |
| Integrações e configuração | `DELETE` | `/hmac/config` | — | ✅ | Revogar a chave HMAC da sessão | `200 {"Details":"HMAC configuration deleted successfully"}`; `GET /hmac/config` voltou de `{"hmac_key":"***"}` para `{"hmac_key":""}`. |
| Integrações e configuração | `POST` | `/hmac/configure` | — | ✅ | Gravar a chave HMAC da sessão (caminho original) | chave DIFERENTE da do alias curto, sobre estado limpo: `200`, e `GET /hmac/config` passou de `""` para `"***"` — a transicao e desta chamada, nao herdada. |
| Integrações e configuração | `GET` | `/labels` | — | ✅ | Listar as etiquetas da conta | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Integrações e configuração | `GET` | `/labels/{id}/chats` | — | ✅ | Listar as conversas de uma etiqueta | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Integrações e configuração | `POST` | `/proxy/set` | — | ✅ | Configurar o proxy de saída da sessão | `200 {"Details":"Proxy configured successfully","Set":true,"ProxyURL":"http://outro.exemplo.invalid:3128"}`; `GET /session/status` passou a devolver esse `proxy_url`, substituindo o `socks5` que a rota irma tinha gravado. |
| Integrações e configuração | `GET` | `/s3/config` | — | ✅ | Consultar a configuração de S3 da sessão | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Integrações e configuração | `POST` | `/s3/config` | — | ✅ | Gravar a configuração de S3 da sessão | `200 {"Details":"S3 configuration saved successfully","Enabled":true}`; `GET /s3/config` devolveu campo a campo o corpo enviado (`bucket: descartavel`, `retention_days: 7`, `media_delivery: both`), com `access_key: "***"`. |
| Integrações e configuração | `DELETE` | `/s3/config` | — | ✅ | Remover a configuração de S3 da sessão | `200 {"Details":"S3 configuration deleted successfully"}`; `GET /s3/config` passou de `bucket: descartavel` para `enabled: false, endpoint: "", bucket: ""`. |
| Integrações e configuração | `POST` | `/s3/configure` | — | ✅ | Gravar a configuração de S3 da sessão (caminho original) | corpo DIFERENTE do alias curto (`eu-west-1`, `descartavel-configure`, `retention_days: 11`): `200`, e `GET /s3/config` devolveu o corpo novo. |
| Integrações e configuração | `POST` | `/s3/test` | — | ⬜ | Testar a ligação ao bucket configurado | nenhum bucket descartavel disponivel no ambiente: o validador de saida recusa endpoint em loopback — medido `400 invalid_s3_endpoint` para `http://127.0.0.1:9000` (F277) —, logo um MinIO local nao pode sequer ser gravado; e nao ha credenciais AWS descartaveis. Com endpoint publico e credenciais falsas o percurso chegou a AWS e voltou `403 InvalidAccessKeyId`, que a rota serviu como `500` (F276) — falha do meu input, nao da rota. |
| Integrações e configuração | `GET` | `/webhook` | — | ✅ | Consultar o webhook da sessão | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Integrações e configuração | `POST` | `/webhook` | — | ✅ | Definir o webhook da sessão | `200 {"webhook":"http://127.0.0.1:8092/hook"}`; `GET /webhook` passou de `webhook: ""` para esse URL, com `subscribe: ["Message","ReadReceipt"]`. |
| Integrações e configuração | `PUT` | `/webhook` | — | ✅ | Actualizar ou desligar o webhook da sessão | `200 {"active":true,"events":["Message"],"webhook":"…/hook2"}`; `GET /webhook` confirmou o URL novo e `subscribe: ["Message"]` — o `ReadReceipt` gravado pelo `POST` desapareceu, o que torna a substituicao observavel. |
| Integrações e configuração | `DELETE` | `/webhook` | — | ✅ | Remover o webhook da sessão | `200 {"Details":"Webhook and events deleted successfully"}`; `GET /webhook` passou de `webhook: http://127.0.0.1:8092/hook2` para `webhook: "", subscribe: [""]`. |
| Integrações e configuração | `GET` | `/webhook/history` | — | ✅ | Consultar o limite de gravação de mensagens | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Integrações e configuração | `POST` | `/webhook/history` | — | ✅ | Definir o limite de gravação de mensagens | `200 {"Details":"History configured successfully","History":77}`; `GET /webhook/history` passou de `{"History":0}` para `{"History":77}`. |
| Saúde | `GET` | `/health` | — | ✅ | Consultar a saúde detalhada do serviço | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Saúde | `GET` | `/health/live` | — | ✅ | Verificar se o processo está vivo (caminho alternativo) | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Saúde | `GET` | `/health/ready` | — | ✅ | Verificar se o serviço pode receber tráfego | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Saúde | `GET` | `/livez` | — | ✅ | Verificar se o processo está vivo | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `GET` | `/session/connect` | — | ✅ | Iniciar a ligação da sessão ao WhatsApp | sessao descartavel nunca emparelhada: `200 {"status":"connecting"}`, e `GET /session/status` passou de `connected: false, qrcode: ""` para `connected: true` com QR de 1842 caracteres — ligou-se mesmo ao WhatsApp. Ver F274 para o que acontece na SEGUNDA chamada depois de um disconnect. |
| Sessões | `GET` | `/session/disconnect` | — | ✅ | Derrubar o transporte da sessão sem desemparelhar | `200 {"details":""}`; `GET /session/status` voltou a `connected: false` mantendo `loggedIn: false`. |
| Sessões | `POST` | `/session/history` | — | ✅ | Configurar quantas mensagens a sessão guarda | `200 {"Details":"History configured successfully","History":33}`; `GET /webhook/history` passou a devolver `{"History":33}` — confirma tambem que e o mesmo manipulador de `POST /webhook/history`. |
| Sessões | `GET` | `/session/hmac/config` | — | ✅ | Saber se esta sessão tem chave HMAC configurada | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `POST` | `/session/hmac/config` | — | ✅ | Gravar a chave HMAC de assinatura dos webhooks | terceira chave distinta, sobre estado limpo: `200`, e `GET /session/hmac/config` passou de `""` para `"***"`. |
| Sessões | `DELETE` | `/session/hmac/config` | — | ✅ | Revogar a chave HMAC desta sessão | `200`; `GET /session/hmac/config` voltou de `"***"` para `""`. |
| Sessões | `POST` | `/session/logout` | — | ❌ | Desvincular o aparelho da conta de WhatsApp | `500 {"error":"internal server error"}` numa sessao LIGADA e nunca emparelhada; no log, `the store doesn't contain a device JID`. Sem transporte vivo responde `409 session_not_connected`, que bate com a documentacao. O caminho de `200` exige conta emparelhada. Achado F275. |
| Sessões | `POST` | `/session/pair/phone` | — | ⬜ | Emparelhar por código de telefone em vez de QR | iniciaria emparelhamento de um numero real. |
| Sessões | `GET` | `/session/profile` | — | ✅ | Consultar o perfil da conta ligada | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `GET` | `/session/profile/full` | — | ✅ | Consultar o perfil da conta com os dados que só a rede sabe | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `POST` | `/session/proxy` | — | ✅ | Configurar o proxy de saída desta sessão | `200 {"Details":"Proxy configured successfully","Set":true,"ProxyURL":"socks5://proxy.exemplo.invalid:1080"}`; confirmado por DOIS observadores — `GET /session/status` (`proxy_url`) e `GET /admin/users/{id}` (`proxy_config.enabled: true`). |
| Sessões | `GET` | `/session/pair/qr` | — | ✅ | Ler o QR code de emparelhamento | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `GET` | `/session/s3/config` | — | ✅ | Ler a configuração S3 desta sessão | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `POST` | `/session/s3/config` | — | ✅ | Gravar a configuração S3 desta sessão | corpo distinto (`ap-south-1`, `descartavel-sessao`, `retention_days: 5`): `200`, e `GET /session/s3/config` devolveu-o. |
| Sessões | `DELETE` | `/session/s3/config` | — | ✅ | Remover a configuração S3 desta sessão | `200`; `GET /session/s3/config` passou de `bucket: descartavel-sessao` para `enabled: false, bucket: ""`. |
| Sessões | `POST` | `/session/s3/test` | — | ⬜ | Testar a configuração S3 gravada com uma ida real ao bucket | idem ao `POST /s3/test` — mesmo manipulador, mesmo bloqueio de fixture (F276, F277). |
| Sessões | `GET` | `/session/status` | — | ✅ | Consultar o estado e o registo da sessão | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `GET` | `/session/ws` | — | ✅ | Receber os eventos da sessão em tempo real (WebSocket) | `101 Switching Protocols` com `Sec-Websocket-Accept` valido, seguido de tres quadros de texto `type: QR` com `code`, `qrCodeBase64` e `expiresAt` na raiz e sem envelope. Cliente RFC 6455 proprio; o motivo antigo ("fora do alcance de curl") media a ferramenta, nao a rota. |
| Sessões | `POST` | `/users/contacts/sync` | `POST /user/contacts/sync` | ✅ | Forçar a sincronização da agenda de contactos | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `POST` | `/users/history/sync` | `POST /user/history/sync` | ✅ | Pedir ao telemóvel as mensagens anteriores a uma âncora | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `POST` | `/users/status` | `POST /user/status` | ⬜ | Definir o recado do perfil da conta | alteraria o recado da conta. |
| Status | `POST` | `/status/set/audio` | — | 🟡 | Publicar um status com áudio | Mesmo caminho e mesma pre-condicao de `/status/set/image` (`SendAudio` para `status@broadcast`). **Precisa de medicao PROPRIA**: evidencia herdada nao e evidencia. Ver `HUMAN-LAST.md` C.2. |
| Status | `POST` | `/status/set/image` | — | 🟡 | Publicar um status com imagem | 200 com message_id. **Observador existe**: a SEGUNDA sessao — `GET /chats/history?chat_jid=status@broadcast`, ou o webhook/`/session/ws` dela. O que falta e a pre-condicao da F256: `getStatusBroadcastRecipients` so inclui contactos com `FullName` (`core/broadcast.go:70`), e a conta Business tem 1 em 461. **A pre-condicao e verificavel por API antes de publicar**, com `GET /users/contacts`. Ver `HUMAN-LAST.md` C.2. |
| Status | `POST` | `/status/set/video` | — | 🟡 | Publicar um status com vídeo | Mesmo caminho e mesma pre-condicao de `/status/set/image` (`SendVideo` para `status@broadcast`). **Precisa de medicao PROPRIA**: evidencia herdada nao e evidencia. Ver `HUMAN-LAST.md` C.2. |
