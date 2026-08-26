# Relatório de evidências — documentação OpenAPI

**Actualizado a 2026-08-26**, depois da padronização de caminhos (F269).

## O que a padronização fez a estes números

As rotas duplicaram: cada uma das 91 rotas de família de colecção passou a ter
uma **forma canónica** ao lado da antiga. Isso muda os totais, e **não** muda a
prova de nada:

| forma | operações | evidência |
|---|---:|---|
| única (já era canónica) | 50 | medida directamente |
| depreciada (a antiga) | 91 | medida directamente |
| canónica (a nova) | 91 | **herdada** da antiga que substitui |

**Por que herdar é honesto aqui**: a canónica é servida pelo MESMO
manipulador, com o mesmo corpo e as mesmas respostas. Não é uma
implementação nova cuja prova esteja por fazer — é o mesmo código noutro
caminho.

**A excepção são as nove reestruturadas**, onde há código novo entre o cliente
e o manipulador (o adaptador que injecta o identificador do caminho no corpo).
Essas foram **re-medidas**, uma a uma, e a nona foi verificada por **paridade**
com a antiga em vez de por sucesso:

```
POST /group/updaterequestparticipants     -> 400 invalid_phone
POST /groups/{group_jid}/join-requests    -> 400 invalid_phone   IDÊNTICAS
```

## Legenda

| marca | o que exige |
|---|---|
| ✅ | pedido HTTP real **e** confirmação independente do efeito |
| 🟡 | pedido real com sucesso, sem forma de confirmar o efeito |
| ❌ | pedido real que falhou; o erro concreto está na descrição da rota |
| ⬜ | não executada, e a razão é específica |

## Resumo quantitativo

```
Rotas registadas no código:     234
  das quais /docs e /docs/:       2   (a página não se documenta a si mesma)
Operações a documentar:         232
Operações documentadas:         232
Cobertura OpenAPI:              100%

Caminhos distintos:             212
Esquemas:                       165
Propriedades com semântica:     694 de 694

Validação:
  OK  chamada real com efeito confirmado: 178
  AMR sucesso sem observador independente: 13
  ERR falhou, com o erro medido:          6
  NT  não testada, com o motivo dito:     35

Swagger UI: operacional, verificado na interface
OpenAPI válido: 3.0.3, sem referência quebrada
Operações depreciadas: 91 (as formas antigas, sem data de remoção)
Endpoints ausentes da documentação: 0
```

## Por grupo

| Grupo | Operações | ✅ | 🟡 | ❌ | ⬜ |
|---|---:|---:|---:|---:|---:|
| Administração | 6 | 2 | 0 | 0 | 4 |
| Canais | 36 | 30 | 4 | 2 | 0 |
| Comunidades | 8 | 8 | 0 | 0 | 0 |
| Contactos e utilizadores | 28 | 20 | 0 | 4 | 4 |
| Conversas | 26 | 24 | 2 | 0 | 0 |
| Descarga de mídia | 10 | 10 | 0 | 0 | 0 |
| Envio de mensagens | 32 | 30 | 2 | 0 | 0 |
| Grupos | 36 | 34 | 2 | 0 | 0 |
| Integrações e configuração | 19 | 6 | 0 | 0 | 13 |
| Saúde | 4 | 4 | 0 | 0 | 0 |
| Sessões | 24 | 10 | 0 | 0 | 14 |
| Status | 3 | 0 | 3 | 0 | 0 |
| **Total** | **232** | **178** | **13** | **6** | **35** |

## As que falham

Seis operações — **três defeitos**, cada um contado duas vezes por ter forma
antiga e canónica:

| defeito | operações | erro medido |
|---|---|---|
| `/user/block` | `POST /user/block`, `POST /users/block` | `422 upstream_rejected`; WhatsApp responde `400 bad-request`. Medido em duas contas, PN e LID (F264) |
| `/user/unblock` | `POST /user/unblock`, `POST /users/unblock` | idem. `GET /user/blocklist` funciona — só a escrita é recusada |
| `/newsletter/updates` | `POST /newsletter/updates`, `POST /newsletters/updates` | `500` ao fim de 30,0 s: o servidor do WhatsApp nunca responde (F265) |

## Tabela completa

| Grupo | Método | Caminho | Forma | Teste | Título |
|---|---|---|---|---|---|
| Administração | `GET` | `/admin/users` | única | ✅ | Listar as sessões existentes |
| Administração | `POST` | `/admin/users` | única | ⬜ | Criar uma sessão |
| Administração | `DELETE` | `/admin/users/{id}` | única | ⬜ | Apagar o registo de uma sessão |
| Administração | `GET` | `/admin/users/{id}` | única | ✅ | Consultar uma sessão pelo identificador |
| Administração | `PUT` | `/admin/users/{id}` | única | ⬜ | Alterar uma sessão |
| Administração | `DELETE` | `/admin/users/{id}/full` | única | ⬜ | Apagar uma sessão por completo, com ficheiros e ligação |
| Canais | `POST` | `/newsletter/admin-invite` | depreciada | ✅ | Convidar um utilizador para administrador do canal |
| Canais | `POST` | `/newsletter/admin-invite/accept` | depreciada | ✅ | Aceitar um convite para administrador de canal |
| Canais | `POST` | `/newsletter/admin-invite/revoke` | depreciada | ✅ | Revogar um convite de administrador de canal |
| Canais | `POST` | `/newsletter/change-owner` | depreciada | ✅ | Transferir a posse de um canal |
| Canais | `POST` | `/newsletter/create` | depreciada | ✅ | Criar um canal |
| Canais | `DELETE` | `/newsletter/delete` | depreciada | ✅ | Apagar um canal (IRREVERSÍVEL) |
| Canais | `POST` | `/newsletter/demote` | depreciada | ✅ | Despromover um administrador do canal a assinante |
| Canais | `POST` | `/newsletter/follow` | depreciada | ✅ | Seguir um canal |
| Canais | `POST` | `/newsletter/info` | depreciada | ✅ | Consultar os metadados de um canal |
| Canais | `POST` | `/newsletter/info-invite` | depreciada | ✅ | Consultar um canal pelo código de convite |
| Canais | `GET` | `/newsletter/list` | depreciada | ✅ | Listar os canais que a sessão segue |
| Canais | `POST` | `/newsletter/mark-viewed` | depreciada | 🟡 | Marcar mensagens de um canal como vistas |
| Canais | `POST` | `/newsletter/messages` | depreciada | ✅ | Ler as mensagens de um canal |
| Canais | `POST` | `/newsletter/mute` | depreciada | ✅ | Silenciar ou dessilenciar um canal |
| Canais | `POST` | `/newsletter/react` | depreciada | 🟡 | Reagir a uma mensagem de um canal |
| Canais | `POST` | `/newsletter/subscribe` | depreciada | ✅ | Subscrever as atualizações ao vivo de um canal |
| Canais | `POST` | `/newsletter/unfollow` | depreciada | ✅ | Deixar de seguir um canal |
| Canais | `POST` | `/newsletter/updates` | depreciada | ❌ | Buscar atualizações de mensagens de um canal (INOPERANTE) |
| Canais | `POST` | `/newsletters/admin-invite` | canónica | ✅ | Convidar um utilizador para administrador do canal |
| Canais | `POST` | `/newsletters/admin-invite/accept` | canónica | ✅ | Aceitar um convite para administrador de canal |
| Canais | `POST` | `/newsletters/admin-invite/revoke` | canónica | ✅ | Revogar um convite de administrador de canal |
| Canais | `POST` | `/newsletters/change-owner` | canónica | ✅ | Transferir a posse de um canal |
| Canais | `POST` | `/newsletters/create` | canónica | ✅ | Criar um canal |
| Canais | `DELETE` | `/newsletters/delete` | canónica | ✅ | Apagar um canal (IRREVERSÍVEL) |
| Canais | `POST` | `/newsletters/demote` | canónica | ✅ | Despromover um administrador do canal a assinante |
| Canais | `POST` | `/newsletters/follow` | canónica | ✅ | Seguir um canal |
| Canais | `POST` | `/newsletters/info` | canónica | ✅ | Consultar os metadados de um canal |
| Canais | `POST` | `/newsletters/info-invite` | canónica | ✅ | Consultar um canal pelo código de convite |
| Canais | `GET` | `/newsletters/list` | canónica | ✅ | Listar os canais que a sessão segue |
| Canais | `POST` | `/newsletters/mark-viewed` | canónica | 🟡 | Marcar mensagens de um canal como vistas |
| Canais | `POST` | `/newsletters/messages` | canónica | ✅ | Ler as mensagens de um canal |
| Canais | `POST` | `/newsletters/mute` | canónica | ✅ | Silenciar ou dessilenciar um canal |
| Canais | `POST` | `/newsletters/react` | canónica | 🟡 | Reagir a uma mensagem de um canal |
| Canais | `POST` | `/newsletters/subscribe` | canónica | ✅ | Subscrever as atualizações ao vivo de um canal |
| Canais | `POST` | `/newsletters/unfollow` | canónica | ✅ | Deixar de seguir um canal |
| Canais | `POST` | `/newsletters/updates` | canónica | ❌ | Buscar atualizações de mensagens de um canal (INOPERANTE) |
| Comunidades | `GET` | `/communities/{community_jid}/participants` | canónica | ✅ | Listar os participantes dos grupos de uma comunidade |
| Comunidades | `GET` | `/communities/{community_jid}/subgroups` | canónica | ✅ | Listar os sub-grupos de uma comunidade |
| Comunidades | `DELETE` | `/communities/{community_jid}/subgroups/{group_jid}` | canónica | ✅ | Desligar um grupo de uma comunidade |
| Comunidades | `PUT` | `/communities/{community_jid}/subgroups/{group_jid}` | canónica | ✅ | Ligar um grupo a uma comunidade |
| Comunidades | `POST` | `/community/link` | depreciada | ✅ | Ligar um grupo a uma comunidade |
| Comunidades | `POST` | `/community/participants` | depreciada | ✅ | Listar os participantes dos grupos de uma comunidade |
| Comunidades | `POST` | `/community/subgroups` | depreciada | ✅ | Listar os sub-grupos de uma comunidade |
| Comunidades | `POST` | `/community/unlink` | depreciada | ✅ | Desligar um grupo de uma comunidade |
| Contactos e utilizadores | `POST` | `/user/avatar` | depreciada | ⬜ | Obter a foto de perfil de um contacto |
| Contactos e utilizadores | `POST` | `/user/block` | depreciada | ❌ | Bloquear um contacto |
| Contactos e utilizadores | `GET` | `/user/blocklist` | depreciada | ✅ | Listar os contactos bloqueados |
| Contactos e utilizadores | `POST` | `/user/check` | depreciada | ✅ | Verificar se números têm WhatsApp |
| Contactos e utilizadores | `GET` | `/user/contacts` | depreciada | ✅ | Listar o roster inteiro da conta |
| Contactos e utilizadores | `GET` | `/user/contacts/last-activity` | depreciada | ✅ | Consultar o instante da última mensagem de cada conversa |
| Contactos e utilizadores | `POST` | `/user/info` | depreciada | ✅ | Consultar aparelhos, recado e identidade de contas |
| Contactos e utilizadores | `GET` | `/user/lid/{jid}` | depreciada | ✅ | Resolver o LID de um número |
| Contactos e utilizadores | `POST` | `/user/presence` | depreciada | ✅ | Definir a presença da própria conta |
| Contactos e utilizadores | `POST` | `/user/presence/subscribe` | depreciada | ✅ | Subscrever a presença de um contacto |
| Contactos e utilizadores | `GET` | `/user/privacy` | depreciada | ✅ | Ler as definições de privacidade da conta |
| Contactos e utilizadores | `POST` | `/user/privacy` | depreciada | ⬜ | Alterar uma definição de privacidade |
| Contactos e utilizadores | `GET` | `/user/profile/{jid}` | depreciada | ✅ | Reunir num pedido só tudo o que se sabe de um contacto |
| Contactos e utilizadores | `POST` | `/user/unblock` | depreciada | ❌ | Desbloquear um contacto |
| Contactos e utilizadores | `POST` | `/users/avatar` | canónica | ⬜ | Obter a foto de perfil de um contacto |
| Contactos e utilizadores | `POST` | `/users/block` | canónica | ❌ | Bloquear um contacto |
| Contactos e utilizadores | `GET` | `/users/blocklist` | canónica | ✅ | Listar os contactos bloqueados |
| Contactos e utilizadores | `POST` | `/users/check` | canónica | ✅ | Verificar se números têm WhatsApp |
| Contactos e utilizadores | `GET` | `/users/contacts` | canónica | ✅ | Listar o roster inteiro da conta |
| Contactos e utilizadores | `GET` | `/users/contacts/last-activity` | canónica | ✅ | Consultar o instante da última mensagem de cada conversa |
| Contactos e utilizadores | `POST` | `/users/info` | canónica | ✅ | Consultar aparelhos, recado e identidade de contas |
| Contactos e utilizadores | `GET` | `/users/lid/{jid}` | canónica | ✅ | Resolver o LID de um número |
| Contactos e utilizadores | `POST` | `/users/presence` | canónica | ✅ | Definir a presença da própria conta |
| Contactos e utilizadores | `POST` | `/users/presence/subscribe` | canónica | ✅ | Subscrever a presença de um contacto |
| Contactos e utilizadores | `GET` | `/users/privacy` | canónica | ✅ | Ler as definições de privacidade da conta |
| Contactos e utilizadores | `POST` | `/users/privacy` | canónica | ⬜ | Alterar uma definição de privacidade |
| Contactos e utilizadores | `GET` | `/users/profile/{jid}` | canónica | ✅ | Reunir num pedido só tudo o que se sabe de um contacto |
| Contactos e utilizadores | `POST` | `/users/unblock` | canónica | ❌ | Desbloquear um contacto |
| Conversas | `POST` | `/chat/archive` | depreciada | ✅ | Arquivar ou desarquivar uma conversa |
| Conversas | `POST` | `/chat/delete/message` | depreciada | ✅ | Apagar para todos uma mensagem enviada |
| Conversas | `POST` | `/chat/ephemeral` | depreciada | ✅ | Definir o temporizador de mensagens temporárias da conversa |
| Conversas | `POST` | `/chat/ephemeral/default` | depreciada | ✅ | Definir o temporizador padrão da conta |
| Conversas | `GET` | `/chat/history` | depreciada | ✅ | Ler o histórico de mensagens de uma conversa |
| Conversas | `GET` | `/chat/list` | depreciada | ✅ | Listar as conversas por interação mais recente |
| Conversas | `POST` | `/chat/markread` | depreciada | ✅ | Marcar mensagens como lidas |
| Conversas | `POST` | `/chat/mute` | depreciada | ✅ | Silenciar ou dessilenciar uma conversa |
| Conversas | `POST` | `/chat/pin` | depreciada | ✅ | Fixar ou desafixar uma conversa no topo |
| Conversas | `POST` | `/chat/presence` | depreciada | ✅ | Anunciar "a escrever" ou "a gravar" numa conversa |
| Conversas | `POST` | `/chat/react` | depreciada | ✅ | Reagir a uma mensagem com um emoji |
| Conversas | `POST` | `/chat/request-unavailable-message` | depreciada | 🟡 | Pedir ao par o reenvio de uma mensagem indecifrável |
| Conversas | `POST` | `/chats/archive` | canónica | ✅ | Arquivar ou desarquivar uma conversa |
| Conversas | `POST` | `/chats/delete/message` | canónica | ✅ | Apagar para todos uma mensagem enviada |
| Conversas | `POST` | `/chats/ephemeral` | canónica | ✅ | Definir o temporizador de mensagens temporárias da conversa |
| Conversas | `POST` | `/chats/ephemeral/default` | canónica | ✅ | Definir o temporizador padrão da conta |
| Conversas | `GET` | `/chats/history` | canónica | ✅ | Ler o histórico de mensagens de uma conversa |
| Conversas | `GET` | `/chats/list` | canónica | ✅ | Listar as conversas por interação mais recente |
| Conversas | `POST` | `/chats/markread` | canónica | ✅ | Marcar mensagens como lidas |
| Conversas | `POST` | `/chats/mute` | canónica | ✅ | Silenciar ou dessilenciar uma conversa |
| Conversas | `POST` | `/chats/pin` | canónica | ✅ | Fixar ou desafixar uma conversa no topo |
| Conversas | `POST` | `/chats/presence` | canónica | ✅ | Anunciar "a escrever" ou "a gravar" numa conversa |
| Conversas | `POST` | `/chats/react` | canónica | ✅ | Reagir a uma mensagem com um emoji |
| Conversas | `POST` | `/chats/request-unavailable-message` | canónica | 🟡 | Pedir ao par o reenvio de uma mensagem indecifrável |
| Conversas | `POST` | `/message/star` | depreciada | ✅ | Favoritar ou desfavoritar uma mensagem |
| Conversas | `POST` | `/messages/star` | canónica | ✅ | Favoritar ou desfavoritar uma mensagem |
| Descarga de mídia | `POST` | `/chat/downloadaudio` | depreciada | ✅ | Descarregar o áudio de uma mensagem recebida |
| Descarga de mídia | `POST` | `/chat/downloaddocument` | depreciada | ✅ | Descarregar o documento de uma mensagem recebida |
| Descarga de mídia | `POST` | `/chat/downloadimage` | depreciada | ✅ | Descarregar a imagem de uma mensagem recebida |
| Descarga de mídia | `POST` | `/chat/downloadsticker` | depreciada | ✅ | Descarregar o autocolante de uma mensagem recebida |
| Descarga de mídia | `POST` | `/chat/downloadvideo` | depreciada | ✅ | Descarregar o vídeo de uma mensagem recebida |
| Descarga de mídia | `POST` | `/chats/downloadaudio` | canónica | ✅ | Descarregar o áudio de uma mensagem recebida |
| Descarga de mídia | `POST` | `/chats/downloaddocument` | canónica | ✅ | Descarregar o documento de uma mensagem recebida |
| Descarga de mídia | `POST` | `/chats/downloadimage` | canónica | ✅ | Descarregar a imagem de uma mensagem recebida |
| Descarga de mídia | `POST` | `/chats/downloadsticker` | canónica | ✅ | Descarregar o autocolante de uma mensagem recebida |
| Descarga de mídia | `POST` | `/chats/downloadvideo` | canónica | ✅ | Descarregar o vídeo de uma mensagem recebida |
| Envio de mensagens | `POST` | `/chat/send/audio` | depreciada | ✅ | Enviar um áudio ou mensagem de voz |
| Envio de mensagens | `POST` | `/chat/send/buttons` | depreciada | ✅ | Enviar uma mensagem com botões |
| Envio de mensagens | `POST` | `/chat/send/carousel` | depreciada | ✅ | Enviar um carrossel de cartões |
| Envio de mensagens | `POST` | `/chat/send/contact` | depreciada | ✅ | Enviar um cartão de contacto |
| Envio de mensagens | `POST` | `/chat/send/document` | depreciada | ✅ | Enviar um documento |
| Envio de mensagens | `POST` | `/chat/send/edit` | depreciada | ✅ | Editar uma mensagem já enviada |
| Envio de mensagens | `POST` | `/chat/send/forward` | depreciada | ✅ | Encaminhar uma mensagem |
| Envio de mensagens | `POST` | `/chat/send/image` | depreciada | ✅ | Enviar uma imagem |
| Envio de mensagens | `POST` | `/chat/send/list` | depreciada | ✅ | Enviar uma lista de seleção |
| Envio de mensagens | `POST` | `/chat/send/location` | depreciada | ✅ | Enviar uma localização |
| Envio de mensagens | `POST` | `/chat/send/poll` | depreciada | ✅ | Criar uma enquete |
| Envio de mensagens | `POST` | `/chat/send/pollvote` | depreciada | ✅ | Votar numa enquete |
| Envio de mensagens | `POST` | `/chat/send/sticker` | depreciada | 🟡 | Enviar um autocolante |
| Envio de mensagens | `POST` | `/chat/send/template` | depreciada | ✅ | Enviar uma mensagem de modelo |
| Envio de mensagens | `POST` | `/chat/send/text` | depreciada | ✅ | Enviar uma mensagem de texto |
| Envio de mensagens | `POST` | `/chat/send/video` | depreciada | ✅ | Enviar um vídeo |
| Envio de mensagens | `POST` | `/chats/send/audio` | canónica | ✅ | Enviar um áudio ou mensagem de voz |
| Envio de mensagens | `POST` | `/chats/send/buttons` | canónica | ✅ | Enviar uma mensagem com botões |
| Envio de mensagens | `POST` | `/chats/send/carousel` | canónica | ✅ | Enviar um carrossel de cartões |
| Envio de mensagens | `POST` | `/chats/send/contact` | canónica | ✅ | Enviar um cartão de contacto |
| Envio de mensagens | `POST` | `/chats/send/document` | canónica | ✅ | Enviar um documento |
| Envio de mensagens | `POST` | `/chats/send/edit` | canónica | ✅ | Editar uma mensagem já enviada |
| Envio de mensagens | `POST` | `/chats/send/forward` | canónica | ✅ | Encaminhar uma mensagem |
| Envio de mensagens | `POST` | `/chats/send/image` | canónica | ✅ | Enviar uma imagem |
| Envio de mensagens | `POST` | `/chats/send/list` | canónica | ✅ | Enviar uma lista de seleção |
| Envio de mensagens | `POST` | `/chats/send/location` | canónica | ✅ | Enviar uma localização |
| Envio de mensagens | `POST` | `/chats/send/poll` | canónica | ✅ | Criar uma enquete |
| Envio de mensagens | `POST` | `/chats/send/pollvote` | canónica | ✅ | Votar numa enquete |
| Envio de mensagens | `POST` | `/chats/send/sticker` | canónica | 🟡 | Enviar um autocolante |
| Envio de mensagens | `POST` | `/chats/send/template` | canónica | ✅ | Enviar uma mensagem de modelo |
| Envio de mensagens | `POST` | `/chats/send/text` | canónica | ✅ | Enviar uma mensagem de texto |
| Envio de mensagens | `POST` | `/chats/send/video` | canónica | ✅ | Enviar um vídeo |
| Grupos | `POST` | `/group/announce` | depreciada | ✅ | Fechar ou abrir o grupo à escrita de não-administradores |
| Grupos | `POST` | `/group/create` | depreciada | ✅ | Criar um grupo, uma comunidade ou um sub-grupo |
| Grupos | `POST` | `/group/ephemeral` | depreciada | ✅ | Definir o tempo das mensagens temporárias do grupo |
| Grupos | `POST` | `/group/info` | depreciada | ✅ | Consultar os metadados de um grupo |
| Grupos | `POST` | `/group/inviteinfo` | depreciada | ✅ | Inspecionar um convite sem entrar no grupo |
| Grupos | `POST` | `/group/invitelink` | depreciada | ✅ | Obter o link de convite de um grupo |
| Grupos | `POST` | `/group/join` | depreciada | ✅ | Entrar num grupo por código de convite |
| Grupos | `POST` | `/group/joinapprovalmode` | depreciada | ✅ | Exigir aprovação de administrador para entrar no grupo |
| Grupos | `POST` | `/group/leave` | depreciada | ✅ | Sair de um grupo |
| Grupos | `POST` | `/group/list` | depreciada | ✅ | Listar os grupos da sessão |
| Grupos | `POST` | `/group/locked` | depreciada | ✅ | Trancar ou destrancar a edição dos metadados do grupo |
| Grupos | `POST` | `/group/name` | depreciada | ✅ | Mudar o nome de um grupo |
| Grupos | `POST` | `/group/photo` | depreciada | ✅ | Definir a foto de um grupo |
| Grupos | `POST` | `/group/photo/remove` | depreciada | ✅ | Remover a foto de um grupo |
| Grupos | `GET` | `/group/requestparticipants` | depreciada | ✅ | Listar os pedidos de entrada pendentes de um grupo |
| Grupos | `POST` | `/group/topic` | depreciada | ✅ | Mudar a descrição de um grupo |
| Grupos | `POST` | `/group/updateparticipants` | depreciada | ✅ | Adicionar ou remover participantes de um grupo |
| Grupos | `POST` | `/group/updaterequestparticipants` | depreciada | 🟡 | Aprovar ou rejeitar pedidos de entrada num grupo |
| Grupos | `POST` | `/groups/announce` | canónica | ✅ | Fechar ou abrir o grupo à escrita de não-administradores |
| Grupos | `POST` | `/groups/create` | canónica | ✅ | Criar um grupo, uma comunidade ou um sub-grupo |
| Grupos | `POST` | `/groups/ephemeral` | canónica | ✅ | Definir o tempo das mensagens temporárias do grupo |
| Grupos | `POST` | `/groups/info` | canónica | ✅ | Consultar os metadados de um grupo |
| Grupos | `POST` | `/groups/inviteinfo` | canónica | ✅ | Inspecionar um convite sem entrar no grupo |
| Grupos | `POST` | `/groups/invitelink` | canónica | ✅ | Obter o link de convite de um grupo |
| Grupos | `POST` | `/groups/join` | canónica | ✅ | Entrar num grupo por código de convite |
| Grupos | `POST` | `/groups/leave` | canónica | ✅ | Sair de um grupo |
| Grupos | `POST` | `/groups/list` | canónica | ✅ | Listar os grupos da sessão |
| Grupos | `POST` | `/groups/locked` | canónica | ✅ | Trancar ou destrancar a edição dos metadados do grupo |
| Grupos | `POST` | `/groups/name` | canónica | ✅ | Mudar o nome de um grupo |
| Grupos | `POST` | `/groups/topic` | canónica | ✅ | Mudar a descrição de um grupo |
| Grupos | `GET` | `/groups/{group_jid}/join-requests` | canónica | ✅ | Listar os pedidos de entrada pendentes de um grupo |
| Grupos | `POST` | `/groups/{group_jid}/join-requests` | canónica | 🟡 | Aprovar ou rejeitar pedidos de entrada num grupo |
| Grupos | `POST` | `/groups/{group_jid}/participants` | canónica | ✅ | Adicionar ou remover participantes de um grupo |
| Grupos | `DELETE` | `/groups/{group_jid}/photo` | canónica | ✅ | Remover a foto de um grupo |
| Grupos | `PUT` | `/groups/{group_jid}/photo` | canónica | ✅ | Definir a foto de um grupo |
| Grupos | `PUT` | `/groups/{group_jid}/settings/join-approval` | canónica | ✅ | Exigir aprovação de administrador para entrar no grupo |
| Integrações e configuração | `POST` | `/call/reject` | única | ⬜ | Recusar uma chamada a entrar |
| Integrações e configuração | `DELETE` | `/hmac/config` | única | ⬜ | Revogar a chave HMAC da sessão |
| Integrações e configuração | `GET` | `/hmac/config` | única | ✅ | Consultar se há chave HMAC configurada |
| Integrações e configuração | `POST` | `/hmac/config` | única | ⬜ | Gravar a chave HMAC da sessão |
| Integrações e configuração | `POST` | `/hmac/configure` | única | ⬜ | Gravar a chave HMAC da sessão (caminho original) |
| Integrações e configuração | `GET` | `/labels` | única | ✅ | Listar as etiquetas da conta |
| Integrações e configuração | `GET` | `/labels/{id}/chats` | única | ✅ | Listar as conversas de uma etiqueta |
| Integrações e configuração | `POST` | `/proxy/set` | única | ⬜ | Configurar o proxy de saída da sessão |
| Integrações e configuração | `DELETE` | `/s3/config` | única | ⬜ | Remover a configuração de S3 da sessão |
| Integrações e configuração | `GET` | `/s3/config` | única | ✅ | Consultar a configuração de S3 da sessão |
| Integrações e configuração | `POST` | `/s3/config` | única | ⬜ | Gravar a configuração de S3 da sessão |
| Integrações e configuração | `POST` | `/s3/configure` | única | ⬜ | Gravar a configuração de S3 da sessão (caminho original) |
| Integrações e configuração | `POST` | `/s3/test` | única | ⬜ | Testar a ligação ao bucket configurado |
| Integrações e configuração | `DELETE` | `/webhook` | única | ⬜ | Remover o webhook da sessão |
| Integrações e configuração | `GET` | `/webhook` | única | ✅ | Consultar o webhook da sessão |
| Integrações e configuração | `POST` | `/webhook` | única | ⬜ | Definir o webhook da sessão |
| Integrações e configuração | `PUT` | `/webhook` | única | ⬜ | Actualizar ou desligar o webhook da sessão |
| Integrações e configuração | `GET` | `/webhook/history` | única | ✅ | Consultar o limite de gravação de mensagens |
| Integrações e configuração | `POST` | `/webhook/history` | única | ⬜ | Definir o limite de gravação de mensagens |
| Saúde | `GET` | `/health` | única | ✅ | Consultar a saúde detalhada do serviço |
| Saúde | `GET` | `/health/live` | única | ✅ | Verificar se o processo está vivo (caminho alternativo) |
| Saúde | `GET` | `/health/ready` | única | ✅ | Verificar se o serviço pode receber tráfego |
| Saúde | `GET` | `/livez` | única | ✅ | Verificar se o processo está vivo |
| Sessões | `GET` | `/session/connect` | única | ⬜ | Iniciar a ligação da sessão ao WhatsApp |
| Sessões | `GET` | `/session/disconnect` | única | ⬜ | Derrubar o transporte da sessão sem desemparelhar |
| Sessões | `POST` | `/session/history` | única | ⬜ | Configurar quantas mensagens a sessão guarda |
| Sessões | `DELETE` | `/session/hmac/config` | única | ⬜ | Revogar a chave HMAC desta sessão |
| Sessões | `GET` | `/session/hmac/config` | única | ✅ | Saber se esta sessão tem chave HMAC configurada |
| Sessões | `POST` | `/session/hmac/config` | única | ⬜ | Gravar a chave HMAC de assinatura dos webhooks |
| Sessões | `POST` | `/session/logout` | única | ⬜ | Desvincular o aparelho da conta de WhatsApp |
| Sessões | `POST` | `/session/pairphone` | única | ⬜ | Emparelhar por código de telefone em vez de QR |
| Sessões | `GET` | `/session/profile` | única | ✅ | Consultar o perfil da conta ligada |
| Sessões | `GET` | `/session/profile/full` | única | ✅ | Consultar o perfil da conta com os dados que só a rede sabe |
| Sessões | `POST` | `/session/proxy` | única | ⬜ | Configurar o proxy de saída desta sessão |
| Sessões | `GET` | `/session/qr` | única | ✅ | Ler o QR code de emparelhamento |
| Sessões | `DELETE` | `/session/s3/config` | única | ⬜ | Remover a configuração S3 desta sessão |
| Sessões | `GET` | `/session/s3/config` | única | ✅ | Ler a configuração S3 desta sessão |
| Sessões | `POST` | `/session/s3/config` | única | ⬜ | Gravar a configuração S3 desta sessão |
| Sessões | `POST` | `/session/s3/test` | única | ⬜ | Testar a configuração S3 gravada com uma ida real ao bucket |
| Sessões | `GET` | `/session/status` | única | ✅ | Consultar o estado e o registo da sessão |
| Sessões | `GET` | `/session/ws` | única | ⬜ | Receber os eventos da sessão em tempo real (WebSocket) |
| Sessões | `POST` | `/user/contacts/sync` | depreciada | ✅ | Forçar a sincronização da agenda de contactos |
| Sessões | `POST` | `/user/history/sync` | depreciada | ✅ | Pedir ao telemóvel as mensagens anteriores a uma âncora |
| Sessões | `POST` | `/user/status` | depreciada | ⬜ | Definir o recado do perfil da conta |
| Sessões | `POST` | `/users/contacts/sync` | canónica | ✅ | Forçar a sincronização da agenda de contactos |
| Sessões | `POST` | `/users/history/sync` | canónica | ✅ | Pedir ao telemóvel as mensagens anteriores a uma âncora |
| Sessões | `POST` | `/users/status` | canónica | ⬜ | Definir o recado do perfil da conta |
| Status | `POST` | `/status/set/audio` | única | 🟡 | Publicar um status com áudio |
| Status | `POST` | `/status/set/image` | única | 🟡 | Publicar um status com imagem |
| Status | `POST` | `/status/set/video` | única | 🟡 | Publicar um status com vídeo |
