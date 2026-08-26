# Relatório de evidências — documentação OpenAPI

Levantamento e medição: 2026-08-26, contra o binário construído do `HEAD` de
`feature/wa-noise`, com duas sessões reais de WhatsApp.

**A classificação é sobre o TESTE, não sobre a documentação.** Todas as 141
entradas estão documentadas e visíveis no Swagger; o que varia é quanta prova
existe de que cada uma funciona.

| marca | o que exige |
|---|---|
| ✅ chamada real com efeito confirmado | pedido HTTP real **e** confirmação independente — segunda leitura, transição de estado medida, ou a mensagem vista no cliente |
| 🟡 `200` sem observador independente | pedido HTTP real com resposta de sucesso, sem forma de confirmar o efeito |
| ❌ falhou, com o erro medido | pedido HTTP real que falhou; o erro concreto está na coluna de evidência |
| ⬜ não testada, com o motivo dito | não foi executada, e a razão é específica |

## Onde isto aparece

A marca de cada rota é **aplicada ao título no Swagger UI**, para que quem abre
a página veja a classificação sem ter de vir aqui. A fonte é
`api/openapi/evidencias.tsv`, e o `cmd/openapidoc` é que a cola no `summary` ao
gerar o documento — não está escrita à mão em lado nenhum.

Três guardas mantêm isto honesto:

1. rota documentada sem linha na tabela **faz o gerador falhar**;
2. linha na tabela para rota que já não existe **também**;
3. `TestOpenAPISummariesTrazemMarcaDeEvidencia` recusa a especificação se
   qualquer operação chegar ao binário sem marca.

## Resumo quantitativo

```
Total de endpoints encontrados: 143
  dos quais /docs e /docs/:       2   (a página de documentação; não se documenta a si mesma)
Total a documentar:             141
Total documentado:              141
Cobertura OpenAPI:              100%

Validação:
  OK  chamada real com efeito confirmado: 98
  AMR 200 sem observador independente:    8
  ERR falhou, com o erro medido:          3
  NT  não testada, com o motivo dito:     32

Swagger UI: operacional, verificado na interface
OpenAPI válido: sim — 3.0.3, sem referência quebrada
Endpoints ausentes da documentação: 0
Endpoints duplicados: 0
```

## Por grupo

| Grupo | Rotas | ✅ | 🟡 | ❌ | ⬜ |
|---|---:|---:|---:|---:|---:|
| Administração | 6 | 2 | 0 | 0 | 4 |
| Canais | 18 | 15 | 2 | 1 | 0 |
| Comunidades | 4 | 4 | 0 | 0 | 0 |
| Contactos e utilizadores | 14 | 10 | 0 | 2 | 2 |
| Conversas | 13 | 12 | 1 | 0 | 0 |
| Descarga de mídia | 5 | 5 | 0 | 0 | 0 |
| Envio de mensagens | 16 | 15 | 1 | 0 | 0 |
| Grupos | 18 | 17 | 1 | 0 | 0 |
| Integrações e configuração | 19 | 6 | 0 | 0 | 13 |
| Saúde | 4 | 4 | 0 | 0 | 0 |
| Sessões | 21 | 8 | 0 | 0 | 13 |
| Status | 3 | 0 | 3 | 0 | 0 |
| **Total** | **141** | **98** | **8** | **3** | **32** |

## As três que falharam

| Endpoint | Esperado | Obtido | Erro |
|---|---|---|---|
| `POST /user/block` | `200` | `422` | `upstream_rejected`; no log, `info query returned status 400: bad-request`. Medido nas duas contas, com número real e inexistente, em PN e em LID. Achado **F264**. |
| `POST /user/unblock` | `200` | `422` | idem. `GET /user/blocklist` funciona — só a escrita é recusada. |
| `POST /newsletter/updates` | `200` | `500` | `context deadline exceeded` ao fim de 30,0 s: o servidor do WhatsApp nunca responde. `POST /newsletter/messages` no mesmo canal e segundo devolve `200`. Achado **F265**. |

## Tabela completa

| Grupo | Método | Endpoint | Documentado | Swagger | Teste | Evidência |
|---|---|---|---|---|---|---|
| Administração | `DELETE` | `/admin/users/{id}` | sim | OK | ⬜ | apagaria uma sessao real em uso. |
| Administração | `DELETE` | `/admin/users/{id}/full` | sim | OK | ⬜ | idem, e e irreversivel. |
| Integrações e configuração | `DELETE` | `/hmac/config` | sim | OK | ⬜ | idem. |
| Canais | `DELETE` | `/newsletter/delete` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Integrações e configuração | `DELETE` | `/s3/config` | sim | OK | ⬜ | idem. |
| Sessões | `DELETE` | `/session/hmac/config` | sim | OK | ⬜ | idem. |
| Sessões | `DELETE` | `/session/s3/config` | sim | OK | ⬜ | idem. |
| Integrações e configuração | `DELETE` | `/webhook` | sim | OK | ⬜ | idem. |
| Administração | `GET` | `/admin/users` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Administração | `GET` | `/admin/users/{id}` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `GET` | `/chat/history` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `GET` | `/chat/list` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `GET` | `/group/requestparticipants` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Saúde | `GET` | `/health` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Saúde | `GET` | `/health/live` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Saúde | `GET` | `/health/ready` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Integrações e configuração | `GET` | `/hmac/config` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Integrações e configuração | `GET` | `/labels` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Integrações e configuração | `GET` | `/labels/{id}/chats` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Saúde | `GET` | `/livez` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `GET` | `/newsletter/list` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Integrações e configuração | `GET` | `/s3/config` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `GET` | `/session/connect` | sim | OK | ⬜ | muta estado de ligacao de uma sessao real em uso. |
| Sessões | `GET` | `/session/disconnect` | sim | OK | ⬜ | idem. |
| Sessões | `GET` | `/session/hmac/config` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `GET` | `/session/profile` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `GET` | `/session/profile/full` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `GET` | `/session/qr` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `GET` | `/session/s3/config` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `GET` | `/session/status` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `GET` | `/session/ws` | sim | OK | ⬜ | upgrade WebSocket; fora do alcance de curl e do Try it out. |
| Contactos e utilizadores | `GET` | `/user/blocklist` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `GET` | `/user/contacts` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `GET` | `/user/contacts/last-activity` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `GET` | `/user/lid/{jid}` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `GET` | `/user/privacy` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `GET` | `/user/profile/{jid}` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Integrações e configuração | `GET` | `/webhook` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Integrações e configuração | `GET` | `/webhook/history` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Administração | `POST` | `/admin/users` | sim | OK | ⬜ | criaria uma sessao real no servidor em uso. |
| Integrações e configuração | `POST` | `/call/reject` | sim | OK | ⬜ | exige uma chamada a entrar; nao ha como provocar uma no ambiente. |
| Conversas | `POST` | `/chat/archive` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chat/delete/message` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Descarga de mídia | `POST` | `/chat/downloadaudio` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Descarga de mídia | `POST` | `/chat/downloaddocument` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Descarga de mídia | `POST` | `/chat/downloadimage` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Descarga de mídia | `POST` | `/chat/downloadsticker` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Descarga de mídia | `POST` | `/chat/downloadvideo` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chat/ephemeral` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chat/ephemeral/default` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chat/markread` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chat/mute` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chat/pin` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chat/presence` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chat/react` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Conversas | `POST` | `/chat/request-unavailable-message` | sim | OK | 🟡 | 200; o reenvio depende do par e nao ha como observa-lo daqui. |
| Envio de mensagens | `POST` | `/chat/send/audio` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chat/send/buttons` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chat/send/carousel` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chat/send/contact` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chat/send/document` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chat/send/edit` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chat/send/forward` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chat/send/image` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chat/send/list` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chat/send/location` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chat/send/poll` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chat/send/pollvote` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chat/send/sticker` | sim | OK | 🟡 | 200 e a mensagem chegou, mas o WebP e sintetico e o cliente desenha bolha vazia. |
| Envio de mensagens | `POST` | `/chat/send/template` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chat/send/text` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Envio de mensagens | `POST` | `/chat/send/video` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Comunidades | `POST` | `/community/link` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Comunidades | `POST` | `/community/participants` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Comunidades | `POST` | `/community/subgroups` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Comunidades | `POST` | `/community/unlink` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/announce` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/create` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/ephemeral` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/info` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/inviteinfo` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/invitelink` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/join` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/joinapprovalmode` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/leave` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/list` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/locked` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/name` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/photo` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/photo/remove` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/topic` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/updateparticipants` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Grupos | `POST` | `/group/updaterequestparticipants` | sim | OK | 🟡 | 200 sem fila de pedidos pendente para observar o efeito. |
| Integrações e configuração | `POST` | `/hmac/config` | sim | OK | ⬜ | escreveria a chave HMAC da sessao em uso. |
| Integrações e configuração | `POST` | `/hmac/configure` | sim | OK | ⬜ | idem. |
| Conversas | `POST` | `/message/star` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletter/admin-invite` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletter/admin-invite/accept` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletter/admin-invite/revoke` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletter/change-owner` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletter/create` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletter/demote` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletter/follow` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletter/info` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletter/info-invite` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletter/mark-viewed` | sim | OK | 🟡 | 200 com data:null; nao ha leitura que confirme a marcacao. |
| Canais | `POST` | `/newsletter/messages` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletter/mute` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletter/react` | sim | OK | 🟡 | 200 com data:null; o canal de teste nao tinha mensagem para observar a reacao. |
| Canais | `POST` | `/newsletter/subscribe` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletter/unfollow` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Canais | `POST` | `/newsletter/updates` | sim | OK | ❌ | 500 ao fim de 30,0 s — context deadline exceeded; o servidor nunca responde (F265). |
| Integrações e configuração | `POST` | `/proxy/set` | sim | OK | ⬜ | idem. |
| Integrações e configuração | `POST` | `/s3/config` | sim | OK | ⬜ | escreveria credenciais de armazenamento na sessao em uso. |
| Integrações e configuração | `POST` | `/s3/configure` | sim | OK | ⬜ | idem. |
| Integrações e configuração | `POST` | `/s3/test` | sim | OK | ⬜ | faria round-trip real contra um bucket externo. |
| Sessões | `POST` | `/session/history` | sim | OK | ⬜ | altera configuracao de gravacao de historico de uma sessao real. |
| Sessões | `POST` | `/session/hmac/config` | sim | OK | ⬜ | idem. |
| Sessões | `POST` | `/session/logout` | sim | OK | ⬜ | derrubaria a sessao real em uso nesta sessao de trabalho. |
| Sessões | `POST` | `/session/pairphone` | sim | OK | ⬜ | iniciaria emparelhamento de um numero real. |
| Sessões | `POST` | `/session/proxy` | sim | OK | ⬜ | mudaria a rota de saida da sessao em uso. |
| Sessões | `POST` | `/session/s3/config` | sim | OK | ⬜ | idem ao /s3/config. |
| Sessões | `POST` | `/session/s3/test` | sim | OK | ⬜ | idem ao /s3/test. |
| Status | `POST` | `/status/set/audio` | sim | OK | 🟡 | idem — herdada da bateria de 2026-08-26. |
| Status | `POST` | `/status/set/image` | sim | OK | 🟡 | 200 com message_id; a lista de destinatarios resolve para 2 numa conta Business (F256). |
| Status | `POST` | `/status/set/video` | sim | OK | 🟡 | idem — herdada da bateria de 2026-08-26. |
| Contactos e utilizadores | `POST` | `/user/avatar` | sim | OK | ⬜ | alteraria o avatar da conta — proibido nesta sessao pelo utilizador. |
| Contactos e utilizadores | `POST` | `/user/block` | sim | OK | ❌ | 422 upstream_rejected; WhatsApp devolve 400 bad-request. Medido nas duas contas, PN e LID (F264). |
| Contactos e utilizadores | `POST` | `/user/check` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `POST` | `/user/contacts/sync` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Sessões | `POST` | `/user/history/sync` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `POST` | `/user/info` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `POST` | `/user/presence` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `POST` | `/user/presence/subscribe` | sim | OK | ✅ | chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente. |
| Contactos e utilizadores | `POST` | `/user/privacy` | sim | OK | ⬜ | alteraria definicoes de privacidade da conta. |
| Sessões | `POST` | `/user/status` | sim | OK | ⬜ | alteraria o recado da conta. |
| Contactos e utilizadores | `POST` | `/user/unblock` | sim | OK | ❌ | 422 upstream_rejected, idem (F264). |
| Integrações e configuração | `POST` | `/webhook` | sim | OK | ⬜ | escreveria a configuracao de webhook da sessao em uso. |
| Integrações e configuração | `POST` | `/webhook/history` | sim | OK | ⬜ | mesmo manipulador do anterior. |
| Administração | `PUT` | `/admin/users/{id}` | sim | OK | ⬜ | alteraria uma sessao real em uso. |
| Integrações e configuração | `PUT` | `/webhook` | sim | OK | ⬜ | idem. |
