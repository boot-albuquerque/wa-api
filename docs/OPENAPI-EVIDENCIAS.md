# Relatório de evidências — documentação OpenAPI

**Actualizado a 2026-08-26**, depois da padronização de caminhos (F269).

## O contrato descreve um nome por operação

O router serve **234 rotas**; o contrato documenta **141**. A diferença são as
91 formas antigas, que **continuam a responder** e saíram da especificação.

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
Rotas servidas pelo router:     234
  documentadas (canónicas):     141
  antigas, fora do contrato:     91   (continuam a responder)
  /docs e /docs/:                 2

Operações documentadas:         141
Cobertura do contrato:          100%
Caminhos distintos:             122
Esquemas:                       165
Propriedades com semântica:     694 de 694

Validação:
  OK  chamada real com efeito confirmado: 98
  AMR sucesso sem observador independente: 8
  ERR falhou, com o erro medido:          3
  NT  não testada, com o motivo dito:     32
```

## Por grupo

| Grupo | Operações | ✅ | 🟡 | ❌ | ⬜ |
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

## As três que falham

| operação | erro medido |
|---|---|
| `POST /users/block` | `422 upstream_rejected`; o WhatsApp responde `400 bad-request`. Medido em duas contas, PN e LID (F264) |
| `POST /users/unblock` | idem. `GET /users/blocklist` funciona — só a escrita é recusada |
| `POST /newsletters/updates` | `500` ao fim de 30,0 s: o servidor do WhatsApp nunca responde (F265) |

## Tabela completa

| Grupo | Método | Caminho | Substitui | Teste | Título |
|---|---|---|---|---|---|
| Administração | `GET` | `/admin/users` | — | ✅ | Listar as sessões existentes |
| Administração | `POST` | `/admin/users` | — | ⬜ | Criar uma sessão |
| Administração | `DELETE` | `/admin/users/{id}` | — | ⬜ | Apagar o registo de uma sessão |
| Administração | `GET` | `/admin/users/{id}` | — | ✅ | Consultar uma sessão pelo identificador |
| Administração | `PUT` | `/admin/users/{id}` | — | ⬜ | Alterar uma sessão |
| Administração | `DELETE` | `/admin/users/{id}/full` | — | ⬜ | Apagar uma sessão por completo, com ficheiros e ligação |
| Canais | `POST` | `/newsletters/admin-invite` | `POST /newsletter/admin-invite` | ✅ | Convidar um utilizador para administrador do canal |
| Canais | `POST` | `/newsletters/admin-invite/accept` | `POST /newsletter/admin-invite/accept` | ✅ | Aceitar um convite para administrador de canal |
| Canais | `POST` | `/newsletters/admin-invite/revoke` | `POST /newsletter/admin-invite/revoke` | ✅ | Revogar um convite de administrador de canal |
| Canais | `POST` | `/newsletters/change-owner` | `POST /newsletter/change-owner` | ✅ | Transferir a posse de um canal |
| Canais | `POST` | `/newsletters/create` | `POST /newsletter/create` | ✅ | Criar um canal |
| Canais | `DELETE` | `/newsletters/delete` | `DELETE /newsletter/delete` | ✅ | Apagar um canal (IRREVERSÍVEL) |
| Canais | `POST` | `/newsletters/demote` | `POST /newsletter/demote` | ✅ | Despromover um administrador do canal a assinante |
| Canais | `POST` | `/newsletters/follow` | `POST /newsletter/follow` | ✅ | Seguir um canal |
| Canais | `POST` | `/newsletters/info` | `POST /newsletter/info` | ✅ | Consultar os metadados de um canal |
| Canais | `POST` | `/newsletters/info-invite` | `POST /newsletter/info-invite` | ✅ | Consultar um canal pelo código de convite |
| Canais | `GET` | `/newsletters/list` | `GET /newsletter/list` | ✅ | Listar os canais que a sessão segue |
| Canais | `POST` | `/newsletters/mark-viewed` | `POST /newsletter/mark-viewed` | 🟡 | Marcar mensagens de um canal como vistas |
| Canais | `POST` | `/newsletters/messages` | `POST /newsletter/messages` | ✅ | Ler as mensagens de um canal |
| Canais | `POST` | `/newsletters/mute` | `POST /newsletter/mute` | ✅ | Silenciar ou dessilenciar um canal |
| Canais | `POST` | `/newsletters/react` | `POST /newsletter/react` | 🟡 | Reagir a uma mensagem de um canal |
| Canais | `POST` | `/newsletters/subscribe` | `POST /newsletter/subscribe` | ✅ | Subscrever as atualizações ao vivo de um canal |
| Canais | `POST` | `/newsletters/unfollow` | `POST /newsletter/unfollow` | ✅ | Deixar de seguir um canal |
| Canais | `POST` | `/newsletters/updates` | `POST /newsletter/updates` | ❌ | Buscar atualizações de mensagens de um canal (INOPERANTE) |
| Comunidades | `GET` | `/communities/{community_jid}/participants` | `POST /community/participants` | ✅ | Listar os participantes dos grupos de uma comunidade |
| Comunidades | `GET` | `/communities/{community_jid}/subgroups` | `POST /community/subgroups` | ✅ | Listar os sub-grupos de uma comunidade |
| Comunidades | `DELETE` | `/communities/{community_jid}/subgroups/{group_jid}` | `POST /community/unlink` | ✅ | Desligar um grupo de uma comunidade |
| Comunidades | `PUT` | `/communities/{community_jid}/subgroups/{group_jid}` | `POST /community/link` | ✅ | Ligar um grupo a uma comunidade |
| Contactos e utilizadores | `POST` | `/users/avatar` | `POST /user/avatar` | ⬜ | Obter a foto de perfil de um contacto |
| Contactos e utilizadores | `POST` | `/users/block` | `POST /user/block` | ❌ | Bloquear um contacto |
| Contactos e utilizadores | `GET` | `/users/blocklist` | `GET /user/blocklist` | ✅ | Listar os contactos bloqueados |
| Contactos e utilizadores | `POST` | `/users/check` | `POST /user/check` | ✅ | Verificar se números têm WhatsApp |
| Contactos e utilizadores | `GET` | `/users/contacts` | `GET /user/contacts` | ✅ | Listar o roster inteiro da conta |
| Contactos e utilizadores | `GET` | `/users/contacts/last-activity` | `GET /user/contacts/last-activity` | ✅ | Consultar o instante da última mensagem de cada conversa |
| Contactos e utilizadores | `POST` | `/users/info` | `POST /user/info` | ✅ | Consultar aparelhos, recado e identidade de contas |
| Contactos e utilizadores | `GET` | `/users/lid/{jid}` | `GET /user/lid/{jid}` | ✅ | Resolver o LID de um número |
| Contactos e utilizadores | `POST` | `/users/presence` | `POST /user/presence` | ✅ | Definir a presença da própria conta |
| Contactos e utilizadores | `POST` | `/users/presence/subscribe` | `POST /user/presence/subscribe` | ✅ | Subscrever a presença de um contacto |
| Contactos e utilizadores | `GET` | `/users/privacy` | `GET /user/privacy` | ✅ | Ler as definições de privacidade da conta |
| Contactos e utilizadores | `POST` | `/users/privacy` | `POST /user/privacy` | ⬜ | Alterar uma definição de privacidade |
| Contactos e utilizadores | `GET` | `/users/profile/{jid}` | `GET /user/profile/{jid}` | ✅ | Reunir num pedido só tudo o que se sabe de um contacto |
| Contactos e utilizadores | `POST` | `/users/unblock` | `POST /user/unblock` | ❌ | Desbloquear um contacto |
| Conversas | `POST` | `/chats/archive` | `POST /chat/archive` | ✅ | Arquivar ou desarquivar uma conversa |
| Conversas | `POST` | `/chats/delete/message` | `POST /chat/delete/message` | ✅ | Apagar para todos uma mensagem enviada |
| Conversas | `POST` | `/chats/ephemeral` | `POST /chat/ephemeral` | ✅ | Definir o temporizador de mensagens temporárias da conversa |
| Conversas | `POST` | `/chats/ephemeral/default` | `POST /chat/ephemeral/default` | ✅ | Definir o temporizador padrão da conta |
| Conversas | `GET` | `/chats/history` | `GET /chat/history` | ✅ | Ler o histórico de mensagens de uma conversa |
| Conversas | `GET` | `/chats/list` | `GET /chat/list` | ✅ | Listar as conversas por interação mais recente |
| Conversas | `POST` | `/chats/markread` | `POST /chat/markread` | ✅ | Marcar mensagens como lidas |
| Conversas | `POST` | `/chats/mute` | `POST /chat/mute` | ✅ | Silenciar ou dessilenciar uma conversa |
| Conversas | `POST` | `/chats/pin` | `POST /chat/pin` | ✅ | Fixar ou desafixar uma conversa no topo |
| Conversas | `POST` | `/chats/presence` | `POST /chat/presence` | ✅ | Anunciar "a escrever" ou "a gravar" numa conversa |
| Conversas | `POST` | `/chats/react` | `POST /chat/react` | ✅ | Reagir a uma mensagem com um emoji |
| Conversas | `POST` | `/chats/request-unavailable-message` | `POST /chat/request-unavailable-message` | 🟡 | Pedir ao par o reenvio de uma mensagem indecifrável |
| Conversas | `POST` | `/messages/star` | `POST /message/star` | ✅ | Favoritar ou desfavoritar uma mensagem |
| Descarga de mídia | `POST` | `/chats/downloadaudio` | `POST /chat/downloadaudio` | ✅ | Descarregar o áudio de uma mensagem recebida |
| Descarga de mídia | `POST` | `/chats/downloaddocument` | `POST /chat/downloaddocument` | ✅ | Descarregar o documento de uma mensagem recebida |
| Descarga de mídia | `POST` | `/chats/downloadimage` | `POST /chat/downloadimage` | ✅ | Descarregar a imagem de uma mensagem recebida |
| Descarga de mídia | `POST` | `/chats/downloadsticker` | `POST /chat/downloadsticker` | ✅ | Descarregar o autocolante de uma mensagem recebida |
| Descarga de mídia | `POST` | `/chats/downloadvideo` | `POST /chat/downloadvideo` | ✅ | Descarregar o vídeo de uma mensagem recebida |
| Envio de mensagens | `POST` | `/chats/send/audio` | `POST /chat/send/audio` | ✅ | Enviar um áudio ou mensagem de voz |
| Envio de mensagens | `POST` | `/chats/send/buttons` | `POST /chat/send/buttons` | ✅ | Enviar uma mensagem com botões |
| Envio de mensagens | `POST` | `/chats/send/carousel` | `POST /chat/send/carousel` | ✅ | Enviar um carrossel de cartões |
| Envio de mensagens | `POST` | `/chats/send/contact` | `POST /chat/send/contact` | ✅ | Enviar um cartão de contacto |
| Envio de mensagens | `POST` | `/chats/send/document` | `POST /chat/send/document` | ✅ | Enviar um documento |
| Envio de mensagens | `POST` | `/chats/send/edit` | `POST /chat/send/edit` | ✅ | Editar uma mensagem já enviada |
| Envio de mensagens | `POST` | `/chats/send/forward` | `POST /chat/send/forward` | ✅ | Encaminhar uma mensagem |
| Envio de mensagens | `POST` | `/chats/send/image` | `POST /chat/send/image` | ✅ | Enviar uma imagem |
| Envio de mensagens | `POST` | `/chats/send/list` | `POST /chat/send/list` | ✅ | Enviar uma lista de seleção |
| Envio de mensagens | `POST` | `/chats/send/location` | `POST /chat/send/location` | ✅ | Enviar uma localização |
| Envio de mensagens | `POST` | `/chats/send/poll` | `POST /chat/send/poll` | ✅ | Criar uma enquete |
| Envio de mensagens | `POST` | `/chats/send/pollvote` | `POST /chat/send/pollvote` | ✅ | Votar numa enquete |
| Envio de mensagens | `POST` | `/chats/send/sticker` | `POST /chat/send/sticker` | 🟡 | Enviar um autocolante |
| Envio de mensagens | `POST` | `/chats/send/template` | `POST /chat/send/template` | ✅ | Enviar uma mensagem de modelo |
| Envio de mensagens | `POST` | `/chats/send/text` | `POST /chat/send/text` | ✅ | Enviar uma mensagem de texto |
| Envio de mensagens | `POST` | `/chats/send/video` | `POST /chat/send/video` | ✅ | Enviar um vídeo |
| Grupos | `POST` | `/groups/announce` | `POST /group/announce` | ✅ | Fechar ou abrir o grupo à escrita de não-administradores |
| Grupos | `POST` | `/groups/create` | `POST /group/create` | ✅ | Criar um grupo, uma comunidade ou um sub-grupo |
| Grupos | `POST` | `/groups/ephemeral` | `POST /group/ephemeral` | ✅ | Definir o tempo das mensagens temporárias do grupo |
| Grupos | `POST` | `/groups/info` | `POST /group/info` | ✅ | Consultar os metadados de um grupo |
| Grupos | `POST` | `/groups/inviteinfo` | `POST /group/inviteinfo` | ✅ | Inspecionar um convite sem entrar no grupo |
| Grupos | `POST` | `/groups/invitelink` | `POST /group/invitelink` | ✅ | Obter o link de convite de um grupo |
| Grupos | `POST` | `/groups/join` | `POST /group/join` | ✅ | Entrar num grupo por código de convite |
| Grupos | `POST` | `/groups/leave` | `POST /group/leave` | ✅ | Sair de um grupo |
| Grupos | `POST` | `/groups/list` | `POST /group/list` | ✅ | Listar os grupos da sessão |
| Grupos | `POST` | `/groups/locked` | `POST /group/locked` | ✅ | Trancar ou destrancar a edição dos metadados do grupo |
| Grupos | `POST` | `/groups/name` | `POST /group/name` | ✅ | Mudar o nome de um grupo |
| Grupos | `POST` | `/groups/topic` | `POST /group/topic` | ✅ | Mudar a descrição de um grupo |
| Grupos | `GET` | `/groups/{group_jid}/join-requests` | `GET /group/requestparticipants` | ✅ | Listar os pedidos de entrada pendentes de um grupo |
| Grupos | `POST` | `/groups/{group_jid}/join-requests` | `POST /group/updaterequestparticipants` | 🟡 | Aprovar ou rejeitar pedidos de entrada num grupo |
| Grupos | `POST` | `/groups/{group_jid}/participants` | `POST /group/updateparticipants` | ✅ | Adicionar ou remover participantes de um grupo |
| Grupos | `DELETE` | `/groups/{group_jid}/photo` | `POST /group/photo/remove` | ✅ | Remover a foto de um grupo |
| Grupos | `PUT` | `/groups/{group_jid}/photo` | `POST /group/photo` | ✅ | Definir a foto de um grupo |
| Grupos | `PUT` | `/groups/{group_jid}/settings/join-approval` | `POST /group/joinapprovalmode` | ✅ | Exigir aprovação de administrador para entrar no grupo |
| Integrações e configuração | `POST` | `/call/reject` | — | ⬜ | Recusar uma chamada a entrar |
| Integrações e configuração | `DELETE` | `/hmac/config` | — | ⬜ | Revogar a chave HMAC da sessão |
| Integrações e configuração | `GET` | `/hmac/config` | — | ✅ | Consultar se há chave HMAC configurada |
| Integrações e configuração | `POST` | `/hmac/config` | — | ⬜ | Gravar a chave HMAC da sessão |
| Integrações e configuração | `POST` | `/hmac/configure` | — | ⬜ | Gravar a chave HMAC da sessão (caminho original) |
| Integrações e configuração | `GET` | `/labels` | — | ✅ | Listar as etiquetas da conta |
| Integrações e configuração | `GET` | `/labels/{id}/chats` | — | ✅ | Listar as conversas de uma etiqueta |
| Integrações e configuração | `POST` | `/proxy/set` | — | ⬜ | Configurar o proxy de saída da sessão |
| Integrações e configuração | `DELETE` | `/s3/config` | — | ⬜ | Remover a configuração de S3 da sessão |
| Integrações e configuração | `GET` | `/s3/config` | — | ✅ | Consultar a configuração de S3 da sessão |
| Integrações e configuração | `POST` | `/s3/config` | — | ⬜ | Gravar a configuração de S3 da sessão |
| Integrações e configuração | `POST` | `/s3/configure` | — | ⬜ | Gravar a configuração de S3 da sessão (caminho original) |
| Integrações e configuração | `POST` | `/s3/test` | — | ⬜ | Testar a ligação ao bucket configurado |
| Integrações e configuração | `DELETE` | `/webhook` | — | ⬜ | Remover o webhook da sessão |
| Integrações e configuração | `GET` | `/webhook` | — | ✅ | Consultar o webhook da sessão |
| Integrações e configuração | `POST` | `/webhook` | — | ⬜ | Definir o webhook da sessão |
| Integrações e configuração | `PUT` | `/webhook` | — | ⬜ | Actualizar ou desligar o webhook da sessão |
| Integrações e configuração | `GET` | `/webhook/history` | — | ✅ | Consultar o limite de gravação de mensagens |
| Integrações e configuração | `POST` | `/webhook/history` | — | ⬜ | Definir o limite de gravação de mensagens |
| Saúde | `GET` | `/health` | — | ✅ | Consultar a saúde detalhada do serviço |
| Saúde | `GET` | `/health/live` | — | ✅ | Verificar se o processo está vivo (caminho alternativo) |
| Saúde | `GET` | `/health/ready` | — | ✅ | Verificar se o serviço pode receber tráfego |
| Saúde | `GET` | `/livez` | — | ✅ | Verificar se o processo está vivo |
| Sessões | `GET` | `/session/connect` | — | ⬜ | Iniciar a ligação da sessão ao WhatsApp |
| Sessões | `GET` | `/session/disconnect` | — | ⬜ | Derrubar o transporte da sessão sem desemparelhar |
| Sessões | `POST` | `/session/history` | — | ⬜ | Configurar quantas mensagens a sessão guarda |
| Sessões | `DELETE` | `/session/hmac/config` | — | ⬜ | Revogar a chave HMAC desta sessão |
| Sessões | `GET` | `/session/hmac/config` | — | ✅ | Saber se esta sessão tem chave HMAC configurada |
| Sessões | `POST` | `/session/hmac/config` | — | ⬜ | Gravar a chave HMAC de assinatura dos webhooks |
| Sessões | `POST` | `/session/logout` | — | ⬜ | Desvincular o aparelho da conta de WhatsApp |
| Sessões | `POST` | `/session/pairphone` | — | ⬜ | Emparelhar por código de telefone em vez de QR |
| Sessões | `GET` | `/session/profile` | — | ✅ | Consultar o perfil da conta ligada |
| Sessões | `GET` | `/session/profile/full` | — | ✅ | Consultar o perfil da conta com os dados que só a rede sabe |
| Sessões | `POST` | `/session/proxy` | — | ⬜ | Configurar o proxy de saída desta sessão |
| Sessões | `GET` | `/session/qr` | — | ✅ | Ler o QR code de emparelhamento |
| Sessões | `DELETE` | `/session/s3/config` | — | ⬜ | Remover a configuração S3 desta sessão |
| Sessões | `GET` | `/session/s3/config` | — | ✅ | Ler a configuração S3 desta sessão |
| Sessões | `POST` | `/session/s3/config` | — | ⬜ | Gravar a configuração S3 desta sessão |
| Sessões | `POST` | `/session/s3/test` | — | ⬜ | Testar a configuração S3 gravada com uma ida real ao bucket |
| Sessões | `GET` | `/session/status` | — | ✅ | Consultar o estado e o registo da sessão |
| Sessões | `GET` | `/session/ws` | — | ⬜ | Receber os eventos da sessão em tempo real (WebSocket) |
| Sessões | `POST` | `/users/contacts/sync` | `POST /user/contacts/sync` | ✅ | Forçar a sincronização da agenda de contactos |
| Sessões | `POST` | `/users/history/sync` | `POST /user/history/sync` | ✅ | Pedir ao telemóvel as mensagens anteriores a uma âncora |
| Sessões | `POST` | `/users/status` | `POST /user/status` | ⬜ | Definir o recado do perfil da conta |
| Status | `POST` | `/status/set/audio` | — | 🟡 | Publicar um status com áudio |
| Status | `POST` | `/status/set/image` | — | 🟡 | Publicar um status com imagem |
| Status | `POST` | `/status/set/video` | — | 🟡 | Publicar um status com vídeo |
