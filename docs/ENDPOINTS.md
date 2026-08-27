# Endpoints do wa-api — inventário e comparação

**Levantamento**: 2026-08-24, contra `feature/wa-noise`.
**Bateria em campo**: 2026-08-26 — ver "Verificação em campo".
**Total**: **234 entradas de rota** servidas, das quais **141 documentadas**
sobre **122 caminhos distintos**. As outras 91 são as formas antigas: continuam
a responder e saíram do contrato.

O número duplicou a 26/08 com a padronização: 143 rotas antigas, mais 91
formas canónicas registadas ao lado delas. As antigas continuam a funcionar e
estão marcadas `deprecated` no OpenAPI — ver "Caminhos canónicos".

Percurso: 121 no levantamento de 24/08 → 141 com comunidades e convites de
admin → 143 → **234** com a padronização.

**Estado por caminho, contado da tabela de verificação** (não estimado):

| | caminhos |
|---|---|
| ✅ chamada real com efeito confirmado | **98** |
| 🟡 sucesso sem observador independente | **8** |
| ❌ falhou, com o erro medido | **3** — `/users/block`, `/users/unblock`, `/newsletters/updates` |
| ⬜ não testada, com o motivo dito | **32** |
| | **141** |

Reproduzir a lista:

```
grep -oE 'registry\.Register\("(/[a-zA-Z0-9/{}._-]+)"' pkg/bootstrap/wiring_routes.go \
  | sed 's/registry.Register("//;s/"//' | sort
```

> **O alvo de cobertura de mensagens** — as 23 folhas que este projecto deve
> alcançar — está em `docs/REFERENCIA-META-OFICIAL.md`, com o estado medido de
> cada uma: **12 ✅, 4 🟡, 1 📥, 6 ❌**. As seis em falta são catálogo,
> produtos, encomendas e Flows.
>
> **A referência oficial da Meta** está em
> `docs/REFERENCIA-META-OFICIAL.md`, com os URLs verificados. As colunas de
> concorrentes desta página são conhecimento geral e **não** medição; aquele
> ficheiro é o levantamento a sério, e diz explicitamente o que não foi
> verificado.

## Aviso de método

As colunas de **concorrentes** são conhecimento geral, **não medição**. Não
corri Cloud API, Evolution nem Baileys. Onde a diferença decidir alguma coisa,
verifique antes de agir.

As colunas do **wa-api** são medidas (rota registada = existe). Mas existir não
é o mesmo que estar provado no cliente — ver a secção "Verificação em campo".

---

## Envelope de erro (F236)

Toda resposta de erro segue o formato:

```json
{
  "code": 400,
  "error": {
    "code": "missing_chat",
    "message": "missing chat in payload"
  },
  "success": false
}
```

- `code` (topo): status HTTP, espelhado no header.
- `error.code`: código estável, legível por máquina, em `snake_case`.
- `error.message`: descrição legível por humano, segura para mostrar ao utilizador.
- `success`: `false` em todo erro, `true` em sucesso. Campo legado — será removido.

### Códigos de erro de fronteira

Estes cinco são guardas que o handler aplica ANTES de chamar o use case. Se um
deles responder, o WhatsApp nem foi contactado.

| HTTP | error.code | error.message | quando |
|---|---|---|---|
| 401 | `unauthorized` | unauthorized | token ausente ou inválido |
| 400 | `missing_session_id` | missing session id | userinfo sem Id (sessão não criada) |
| 400 | `missing_id` | missing ID | parâmetro de caminho `{id}` ausente |
| 400 | `decode_payload_failed` | could not decode payload | corpo JSON malformado ou ilegível |
| 400 | `missing_jid` | missing jid in path | parâmetro de caminho `{jid}` ausente |

### Códigos de erro de validação

Campos presentes no corpo mas com valor inválido ou ausente. O use case
devolve `*apperr.AppError` com `CategoryValidation`, que a fronteira
traduz em 400.

Os códigos variam por rota — ver os testes de cada handler para a lista
completa. Exemplos comuns: `missing_chat`, `invalid_duration`,
`invalid_limit`.

---

## Campo de destino: `chat` como nome universal (F225)

Todas as rotas que recebem um destinatário no corpo JSON aceitam **`"chat"`**
como nome do campo de destino. O nome legado da rota (`Phone`, `groupJID`,
`jid`, `Group`, `groupjid`, `Chat`, `phone`) continua a funcionar — nada
quebra. Quando **ambos** estiverem presentes no payload, o nome legado ganha.

Exemplos equivalentes:

```bash
# antes (nome específico da rota)
curl -X POST .../chat/send/text -d '{"Phone":"55...@s.whatsapp.net","Body":"oi"}'

# agora (nome universal)
curl -X POST .../chat/send/text -d '{"chat":"55...@s.whatsapp.net","Body":"oi"}'
```

O campo `chat` aceita qualquer JID válido — individual (`@s.whatsapp.net`) ou
grupo (`@g.us`) — conforme a rota espere.

Os nomes antigos **não estão deprecados**. Payloads existentes continuam a
funcionar sem alteração.

---

## chat — 34 rotas

### Envio (15)

| método | rota | o que faz |
|---|---|---|
| POST | `/chat/send/text` | texto, com link preview opcional |
| POST | `/chat/send/image` | imagem com legenda |
| POST | `/chat/send/video` | vídeo com legenda |
| POST | `/chat/send/audio` | áudio; a legenda vai como mensagem separada (F116) |
| POST | `/chat/send/document` | documento com nome de ficheiro |
| POST | `/chat/send/sticker` | autocolante |
| POST | `/chat/send/location` | localização; latitude/longitude são ponteiros (F121) |
| POST | `/chat/send/contact` | cartão de contacto |
| POST | `/chat/send/poll` | criar enquete |
| POST | `/chat/send/pollvote` | **votar** numa enquete (CAP-48, F228) |
| POST | `/chat/send/buttons` | botões interativos (fluxo nativo) |
| POST | `/chat/send/list` | lista de seleção |
| POST | `/chat/send/carousel` | carrossel HSCROLL_CARDS (CAP-46) |
| POST | `/chat/send/template` | mensagem de template |
| POST | `/chat/send/forward` | encaminhar — por conteúdo (CAP-49) ou por chave de mensagem (CAP-55) |

**`/chat/send/forward`** aceita duas formas de payload (CAP-55):

| forma | campos obrigatórios | ForwardingScore | o que acontece |
|---|---|---|---|
| por conteúdo | `Phone` + `Body` | aceite do payload (default 1) | envia texto novo marcado como encaminhado (CAP-49) |
| por chave | `Phone` + `MessageID` | **IGNORADO** — derivado da mensagem original (+1) | reenvia a mensagem guardada (incluindo mídia, sem re-upload) |

Quando `MessageID` está presente, o campo `ForwardingScore` do payload é
ignorado; o score é lido do `ContextInfo` da mensagem original e incrementado
em 1, como o Baileys faz. `Chat` é opcional (reservado para disambiguação
futura). Se a mensagem não existir no histórico, devolve 404.

> **Limitação anterior removida (F227, corrigida 2026-08-25)**: mensagens
> enviadas pela API agora são persistidas no `message_history` com `datajson`
> completo. É possível encaminhar por chave uma mensagem que a API acabou de
> enviar — desde que o utilizador tenha `history > 0` na configuração.
>
> **`history` NÃO vem ligado por omissão.** Ligue com
> `POST /session/history {"history": 100}` antes de contar com isto.
> Com `history = 0` (o valor por omissão), **nenhum** dos dois caminhos —
> tempo real e sincronização — grava em `message_history` (F230, corrigida
> 2026-08-25). Instalações novas não acumulam histórico até a chamada acima.

**`/chat/send/pollvote`** — notas (F228):

- O campo `Sender` do payload (JID do criador da enquete) é **resolvido pelo
  servidor** para a forma de identidade correcta (PN ou LID) antes de encriptar
  o voto. O cliente pode enviar qualquer das duas formas; a resolução é:
  1. Histórico (`message_history.sender_jid`) — forma do wire, autoritativa.
  2. Mapeamento PN→LID via store — fallback quando o histórico não tem a
     mensagem. **Só converte PN→LID, nunca LID→PN.**
  3. Payload tal qual — com warning se for PN (pode falhar MAC).
- O `200` significa **despacho** (o voto foi enviado ao servidor do WhatsApp),
  **não confirmação de contagem**. Não há como distinguir despacho de
  contabilização na resposta — confirmar na interface do WhatsApp.

Todas as rotas de envio aceitam **reply-to** (`ReplyTo`) desde a CAP-46; as
oito com texto visível aceitam **menções** (`MentionedJid`) desde a CAP-47.

### Ações sobre mensagem (3)

| método | rota | o que faz |
|---|---|---|
| POST | `/chat/send/edit` | editar mensagem enviada |
| POST | `/chat/delete/message` | revogar mensagem enviada |
| POST | `/message/star` | favoritar / desfavoritar mensagem (CAP-54) — **ver F223** |

### Descarga de média (5, mais a forma canónica consolidada)

Canónica (CAP-10): `POST /chats/download/{kind}`, `kind` ∈
`image, video, audio, document, sticker`. As cinco abaixo continuam a
responder, mas saíram do contrato.

| método | rota | o que faz |
|---|---|---|
| POST | `/chat/downloadimage` | baixa e decifra imagem recebida |
| POST | `/chat/downloadvideo` | idem, vídeo |
| POST | `/chat/downloadaudio` | idem, áudio |
| POST | `/chat/downloaddocument` | idem, documento |
| POST | `/chat/downloadsticker` | idem, autocolante |

### Gestão de conversa (12)

| método | rota | o que faz |
|---|---|---|
| GET | `/chat/list` | lista as conversas |
| GET | `/chat/history` | histórico de mensagens de uma conversa |
| POST | `/chat/archive` | arquivar / desarquivar |
| POST | `/chat/mute` | silenciar / dessilenciar: 8h, 1 semana, sempre (CAP-52) — **bloqueado, ver F223** |
| ~~POST~~ | ~~`/chat/delete`~~ | **removida (F257)** — era alias de `/chat/delete/message`, não apagava conversa. Usar `/chat/delete/message`. |
| POST | `/chat/markread` | marcar como lida |
| POST | `/chat/presence` | "a escrever" / "a gravar" |
| POST | `/chat/react` | reagir com emoji |
| POST | `/chat/ephemeral` | temporizador de mensagens temporárias da conversa (CAP-50) |
| POST | `/chat/ephemeral/default` | temporizador padrão da conta (CAP-50) |
| POST | `/chat/request-unavailable-message` | pedir reenvio de mensagem indisponível |
| POST | `/chat/pin` | fixar / desafixar conversa no topo (CAP-53) |

---

## group — 18 rotas

| método | rota | o que faz |
|---|---|---|
| POST | `/group/create` | criar grupo com participantes |
| POST | `/group/list` | grupos da sessão |
| POST | `/group/info` | metadados do grupo |
| POST | `/group/join` | entrar por código de convite |
| POST | `/group/leave` | sair do grupo |
| POST | `/group/invitelink` | obter / revogar link de convite |
| POST | `/group/inviteinfo` | inspecionar convite sem entrar |
| POST | `/group/updateparticipants` | adicionar, remover, promover, despromover |
| GET | `/group/requestparticipants` | pedidos de entrada pendentes |
| POST | `/group/updaterequestparticipants` | aprovar / rejeitar pedidos |
| POST | `/group/joinapprovalmode` | exigir aprovação para entrar |
| POST | `/group/name` | mudar o nome |
| POST | `/group/topic` | mudar a descrição |
| POST | `/group/photo` | definir foto |
| POST | `/group/photo/remove` | remover foto |
| POST | `/group/announce` | só administradores enviam |
| POST | `/group/locked` | só administradores editam metadados |
| POST | `/group/ephemeral` | mensagens temporárias do grupo |

---

## user — 16 rotas

| método | rota | o que faz |
|---|---|---|
| POST | `/user/info` | informação de um ou mais números |
| POST | `/user/check` | verificar se o número tem WhatsApp |
| GET | `/user/contacts` | lista de contactos |
| POST | `/user/contacts/sync` | forçar sincronização de contactos |
| GET | `/user/contacts/last-activity` | última atividade conhecida |
| POST | `/user/avatar` | foto de perfil de um contacto |
| GET | `/user/profile/{jid}` | perfil de um contacto |
| POST | `/user/status` | recado ("status" textual) |
| POST | `/user/presence` | definir presença (online / offline) |
| POST | `/user/presence/subscribe` | subscrever presença de um contacto |
| GET | `/user/privacy` | definições de privacidade |
| POST | `/user/block` | bloquear |
| POST | `/user/unblock` | desbloquear |
| GET | `/user/blocklist` | lista de bloqueados |
| GET | `/user/lid/{jid}` | resolver LID ↔ número |
| POST | `/user/history/sync` | pedir sincronização de histórico |

---

## session — 14 rotas

| método | rota | o que faz |
|---|---|---|
| GET | `/session/connect` | ligar a sessão; verifica posse antes de responder (F108) |
| GET | `/session/disconnect` | desligar |
| POST | `/session/logout` | terminar sessão no telemóvel |
| GET | `/session/pair/qr` | código QR de pareamento (canónico; substitui `GET /session/qr`) |
| POST | `/session/pair/phone` | parear por código de telefone (canónico; substitui `POST /session/pairphone`) |
| GET | `/session/status` | estado da sessão |
| GET | `/session/profile` | perfil da própria sessão |
| GET | `/session/profile/full` | perfil completo |
| GET | `/session/ws` | WebSocket de eventos |
| POST | `/session/proxy` | proxy por sessão |
| POST | `/session/history` | configurar limite de histórico |
| POST | `/session/s3/config` | configurar S3 por sessão |
| POST | `/session/s3/test` | testar credenciais S3 |
| POST | `/session/hmac/config` | configurar chave HMAC por sessão |

---

## newsletter (canais) — 15 rotas

| método | rota | o que faz |
|---|---|---|
| POST | `/newsletter/create` | criar canal |
| GET | `/newsletter/list` | canais seguidos |
| POST | `/newsletter/info` | metadados do canal |
| POST | `/newsletter/info-invite` | metadados por convite |
| POST | `/newsletter/follow` | seguir |
| POST | `/newsletter/unfollow` | deixar de seguir |
| POST | `/newsletter/subscribe` | subscrever atualizações ao vivo |
| POST | `/newsletter/updates` | buscar atualizações |
| POST | `/newsletter/messages` | mensagens do canal |
| POST | `/newsletter/mark-viewed` | marcar como visto |
| POST | `/newsletter/react` | reagir a mensagem do canal |
| POST | `/newsletter/mute` | silenciar |
| POST | `/newsletter/demote` | despromover admin a assinante (F233b) |
| POST | `/newsletter/change-owner` | transferir posse do canal (F233b) |
| DELETE | `/newsletter/delete` | **apagar canal — IRREVERSÍVEL** (F233b) |

#### `/newsletter/demote` — despromover admin

Corpo: `{"jid": "<canal>", "userJID": "<admin-a-despromover>"}`.

Transforma um administrador do canal em assinante simples. Exige que o
chamador seja dono do canal.

#### `/newsletter/change-owner` — transferir posse

Corpo: `{"jid": "<canal>", "userJID": "<novo-dono>"}`.

Transfere a posse do canal para outro utilizador. O chamador perde a posse;
o alvo torna-se o novo dono. **Irreversível sem a cooperação do novo dono.**

#### `/newsletter/delete` — apagar canal

**Verificado ponta a ponta em 2026-08-25**: canal real apagado pela API, e a
remoção confirmada por três vias independentes — a lista de canais passou de 1
para 0, o `info` passou a `state: non_existing`, e o link público de convite
passou a `Link de convite inválido`.

Método: **DELETE** (não POST).
Corpo: `{"jid": "<canal>", "confirmJID": "<canal>"}`.

Apaga permanentemente o canal. **IRREVERSÍVEL — o canal e todo o conteúdo
são destruídos.** A confirmação explícita é obrigatória: `confirmJID` tem
de ser idêntico a `jid`. Corpo sem `confirmJID`, ou com valor diferente de
`jid`, devolve 400.

**Query IDs (F233b/c)**: os IDs iniciais vieram do Baileys e **estavam
errados** para `demote` e `change-owner` — davam `400 Bad Request`. Foram
substituídos pelos reais, extraídos do bundle JS do WhatsApp Web
(`WAWebMexDemoteNewsletterAdminJobMutation` → `9880997548630971`,
`WAWebMexChangeNewsletterOwnerJobMutation` → `9546742745432473`); o do
`delete` que veio do Baileys já estava correto.

O WhatsApp **roda estes IDs sem aviso**. Quando uma operação começar a
responder `400 Bad Request (CRITICAL)` e as vizinhas continuarem a funcionar,
reextraia com `scripts/mex-query-ids/` — o README explica o método e as
armadilhas.

### Erros específicos de newsletter

| HTTP | error.code | quando | o que fazer |
|---|---|---|---|
| 403 | `newsletter_admin_cannot_unfollow` | `POST /newsletter/unfollow` quando o chamador é admin/dono do canal | demitir-se de admin antes de deixar de seguir (FAQ WhatsApp: https://faq.whatsapp.com/284188487298437/) |

---

## status (stories) — 3 rotas

| método | rota | o que faz |
|---|---|---|
| POST | `/status/set/image` | publicar status com imagem (CAP-51) |
| POST | `/status/set/video` | publicar status com vídeo (CAP-51) |
| POST | `/status/set/audio` | publicar status com áudio (CAP-51) |

### Quebra de contrato (F229, 2026-08-25)

`POST /status/set/text` foi **removida**. A rota apontava para o mesmo handler
de `POST /user/status` (define o "Recado"/About do perfil) e **não publicava
status nenhum** — quem a chamava julgava publicar uma story de texto e estava
a alterar o perfil. Para definir o recado do perfil, use `POST /user/status`.
Publicar texto como story (status efémero) não é uma capability existente.

---

## Infraestrutura — 19 rotas

| método | rota | o que faz |
|---|---|---|
| GET/POST/PUT/DELETE | `/webhook` | configurar webhook da sessão |
| GET/POST | `/webhook/history` | **limite de gravação de MENSAGENS** (`users.history`) — nome enganador, ver F252 |
| GET | `/s3/config` | ler configuração S3 global |
| POST | `/s3/configure` | definir configuração S3 global |
| POST | `/s3/config` | alias de `/s3/configure` (F251) |
| DELETE | `/s3/config` | remover |
| POST | `/s3/test` | testar |
| GET | `/hmac/config` | ler configuração HMAC |
| POST | `/hmac/configure` | definir |
| POST | `/hmac/config` | alias de `/hmac/configure` (F251) |
| DELETE | `/hmac/config` | remover |
| GET | `/labels` | etiquetas (WhatsApp Business) |
| GET | `/labels/{id}/chats` | conversas de uma etiqueta |
| POST | `/proxy/set` | proxy global |
| POST | `/call/reject` | rejeitar chamada recebida |
| GET | `/health` | saúde do serviço |
| GET | `/` | raiz |

---

## Comparação

**Aviso de proveniência, repetido porque importa**: a coluna **wa-api** é
MEDIDA (contagem de rotas registadas, com o comando no topo deste documento).
As colunas dos concorrentes são **conhecimento geral, não medição** — não corri
Evolution, Baileys nem Open WA. Onde a diferença decidir alguma coisa, meça
antes de agir. Este projeto já se enganou por acreditar numa issue de outro
projeto sem medir (F222).

**O que cada um é**, porque comparar sem isto produz tabelas enganadoras:

| | natureza | acesso |
|---|---|---|
| **wa-api** | serviço HTTP sobre fork Go do protocolo WA Web | 234 rotas REST (143 + 91 canónicas) |
| **Evolution API** | serviço HTTP sobre Baileys | REST + webhooks |
| **Baileys** | **biblioteca** TypeScript | API de programa, não HTTP |
| **Open WA** | automação de **Puppeteer** sobre a SPA do WhatsApp Web | REST (EASY API) ou biblioteca |

A diferença de natureza tem consequência prática: o **Open WA dirige um browser
real**, portanto carrega Chrome e é sensível a mudanças de DOM da SPA. Nós e a
Evolution falamos o protocolo diretamente. O Baileys é biblioteca — comparar
"rotas" com ele é comparar coisas diferentes; a coluna diz se a capacidade
existe, não se há endpoint.

### Envio de mensagem

| capacidade | wa-api | Evolution | Baileys | Open WA |
|---|---|---|---|---|
| texto, imagem, vídeo, áudio, documento, sticker | ✅ | ✅ | ✅ | ✅ |
| localização, contacto | ✅ | ✅ | ✅ | ✅ |
| enquete (criar) | ✅ | ✅ | ✅ | ✅ |
| **votar em enquete** | ✅ | ⚠️ | ✅ | ⚠️ |
| botões, lista | ✅ | ✅ | ✅ | ⚠️ descontinuado |
| **carrossel** | ✅ provado em campo | ⚠️ | ⚠️ proto cru | ❌ |
| template | ✅ | ✅ | ✅ | ⚠️ |
| **reply-to (citar)** | ✅ 15 rotas | ✅ | ✅ | ✅ |
| **menções (@)** | ✅ 8 rotas com texto | ✅ | ✅ | ✅ |
| **encaminhar** | ✅ | ✅ | ✅ | ✅ |
| editar, apagar, reagir | ✅ | ✅ | ✅ | ✅ |

### Conversa e mensagem

| capacidade | wa-api | Evolution | Baileys | Open WA |
|---|---|---|---|---|
| listar conversas, histórico | ✅ | ✅ | ✅ | ✅ |
| arquivar, marcar lida, presença | ✅ | ✅ | ✅ | ✅ |
| **fixar conversa** | ✅ | ⚠️ | ✅ | ✅ |
| **silenciar conversa** | ⚠️ **bloqueado (F223)** | ⚠️ | ✅ | ✅ |
| **favoritar mensagem** | ⚠️ **bloqueado (F223)** | ⚠️ | ✅ | ✅ |
| mensagens temporárias | ✅ conversa + padrão | ⚠️ | ✅ | ⚠️ |
| descarga de média (5 tipos) | ✅ | ✅ | ✅ | ✅ |
| etiquetas (labels) | ✅ | ✅ | ✅ | ✅ |

### Grupos, canais e conta

| capacidade | wa-api | Evolution | Baileys | Open WA |
|---|---|---|---|---|
| **grupos** | ✅ 18 rotas | ✅ | ✅ | ✅ |
| **canais (newsletter)** | ✅ 15 rotas | ⚠️ parcial | ✅ | ❌ |
| **comunidades** | ❌ | ⚠️ | ⚠️ | ⚠️ |
| contactos, bloqueio, privacidade | ✅ | ✅ | ✅ | ✅ |
| **status / stories** | ✅ 3 tipos (F229: texto removido) | ✅ | ✅ | ✅ |
| perfil próprio | ✅ | ✅ | ✅ | ✅ |
| rejeitar chamada | ✅ | ⚠️ | ✅ | ⚠️ |

### Operação

| capacidade | wa-api | Evolution | Baileys | Open WA |
|---|---|---|---|---|
| webhook por sessão | ✅ | ✅ | ➖ é lib | ✅ |
| **assinatura HMAC do webhook** | ✅ 3 rotas | ⚠️ | ➖ | ⚠️ |
| **S3 por sessão** | ✅ 4 rotas | ✅ | ➖ | ⚠️ |
| proxy por sessão | ✅ | ✅ | ✅ | ✅ |
| multi-sessão | ✅ | ✅ | manual | ✅ |
| WebSocket de eventos | ✅ | ✅ | ➖ | ✅ |
| RabbitMQ / SQS | ❌ | ✅ | ❌ | ❌ |
| integrações (Chatwoot, Typebot) | ❌ | ✅ | ❌ | ❌ |

### Leitura honesta desta tabela

**Onde estamos à frente**: canais (12 rotas contra parcial/nenhum), HMAC de
webhook, e o carrossel provado em campo. As mensagens temporárias com
temporizador de conversa E padrão de conta também são mais completas que a
média.

**Onde estamos atrás**: integrações prontas (Chatwoot, Typebot, RabbitMQ) — a
Evolution vive disso e nós não temos nenhuma. E **duas capacidades que os
outros três têm e nós entregámos bloqueadas**: silenciar e favoritar (F223).

**Onde a comparação engana**: o Baileys tem quase tudo porque é biblioteca —
quem o usa escreve o serviço à volta. A comparação justa com ele não é de
funcionalidades, é de esforço para chegar a um serviço operável.

## O que falta — a lista completa

**Três itens.** Todo o resto da superfície está entregue.

### CORREÇÃO 2026-08-24 — três "faltas de protocolo" não eram faltas

Este documento afirmava que **fixar conversa, favoritar e silenciar** faltavam
no protocolo e exigiriam trabalho de raiz. **Estava errado.**

O erro de método: procurei pelos nomes dos MÉTODOS do cliente (`MuteChat`,
`PinInChat`, `StarMessage`) em `internal/wa-noise/core`. O app-state não vive
lá — vive nos construtores de patch, e os três já existiam:

```
appstate/patch_builders_chat.go:16     BuildMute / BuildMuteAbs
appstate/patch_builders_chat.go:57     BuildPin
appstate/patch_builders_message.go:43  BuildStar
```

Nenhum tinha consumidor. Eram **falta de rota**, não falta de protocolo — a
mesma classe do voto em enquete. Entregues nas CAP-52, CAP-53 e CAP-54.

**Lição, porque é reutilizável**: confirmar no código antes de concluir que
algo não existe, e procurar pelo MECANISMO (app-state, mensagem, IQ), não pelo
nome que a capacidade teria.

### Falta no próprio protocolo (1)

| capacidade | estado | nota |
|---|---|---|
| **Comunidades** | uma única menção no fork | praticamente ausente; exige levantamento próprio antes de estimar |

### Bloqueado por defeito do fork (2)

Existem, estão implementadas e testadas, mas **não funcionam em campo**:

| rota | estado |
|---|---|
| `POST /chat/mute` | 409 `app_state_conflict` |
| `POST /message/star` | 409 `app_state_conflict` |

**F223**: patches `regular_high` falham com `mismatching LTHash`. Medido em duas
sessões, com `fullSync` e incremental. Com o estado local apagado, nem o
snapshot do próprio servidor verifica — não é a nossa cópia.

O `POST /chat/pin` funciona porque usa `regular_low`. Quatro tipos de patch
verificam; só o `regular_high` falha, com o mesmo código. **Essa assimetria é a
pista por explorar.**

A resposta é honesta — 409 diz "conflito de estado", não "a API rebentou" —
mas a capability está indisponível.

### Decidido não fazer, com medição em campo (2)

| item | veredito |
|---|---|
| **`ALBUM_IMAGE`** (F221) | três variantes enviadas — com botões, sem botões, `messageVersion=2` — **todas recusadas pelo cliente** nos dois lados. Não é um enum de carrossel: o álbum é `MessageAssociation` com `MEDIA_ALBUM` e `albumParentKey`, mensagens separadas ligadas por chave de pai. Outro mecanismo. |
| **PIX / pagamento** (F213) | `payment_info` + `pix_static_code` **recusado pelo cliente** em conta pessoal. E mesmo a renderizar, `pix_static_code` **não processa pagamento** — mostra os dados e a pessoa paga à mão. Confirmação automática só na Cloud API com WABA. |

Os dois foram refutados por **medição em campo com fotografia**, não por leitura.
Reabrir só com dado novo.

### Fora de alcance

Catálogo, produtos e Flows são superfície exclusiva da Cloud API com WABA.

## Caminhos canónicos — a padronização

**2026-08-26.** As rotas passaram a ter forma canónica, e **as antigas
continuam a funcionar**. A regra está em `api/openapi/CAMINHOS-CANONICOS.md`; a
tabela é `api/openapi/caminhos.tsv`, e é dela que saem tanto as rotas
registadas como a documentação — não há terceira cópia a desactualizar-se.

**91 rotas** ganharam forma canónica. O que fica singular — `/session/*`,
`/health`, `/webhook`, `/s3/*`, `/hmac/*`, `/proxy/set`, `/status/set/*`,
`/labels`, `/admin/users`, `/call/reject` — é singleton ou já era plural, e o
motivo de cada uma está no documento da regra.

**As antigas continuam a ser servidas — e saíram do OpenAPI.** Não há data de
remoção; o que há é a decisão de o contrato descrever **um nome por operação**.
Documentar as duas formas punha 232 operações para 141 capacidades, e obrigava
quem lê a escolher entre `/chat/list` e `/chats/list` sem elemento para decidir
— que é a ambiguidade que esta padronização existe para eliminar.

**Esta tabela é, a partir de agora, a única referência do nome antigo.** Se tem
um cliente a chamar `/chat/list`, ele continua a funcionar; procure aqui a
forma nova quando quiser migrar.

### As nove que mudaram de forma, não só de número

Nestas o identificador sai do corpo e vai para o caminho, e o método passa a
dizer a operação:

| antiga | canónica |
|---|---|
| `GET /group/requestparticipants` | `GET /groups/{group_jid}/join-requests` |
| `GET /user/lid/{jid}` | `GET /users/lid/{jid}` |
| `GET /user/profile/{jid}` | `GET /users/profile/{jid}` |
| `POST /community/link` | `PUT /communities/{community_jid}/subgroups/{group_jid}` |
| `POST /community/participants` | `GET /communities/{community_jid}/participants` |
| `POST /community/subgroups` | `GET /communities/{community_jid}/subgroups` |
| `POST /community/unlink` | `DELETE /communities/{community_jid}/subgroups/{group_jid}` |
| `POST /group/joinapprovalmode` | `PUT /groups/{group_jid}/settings/join-approval` |
| `POST /group/photo` | `PUT /groups/{group_jid}/photo` |
| `POST /group/photo/remove` | `DELETE /groups/{group_jid}/photo` |
| `POST /group/updateparticipants` | `POST /groups/{group_jid}/participants` |
| `POST /group/updaterequestparticipants` | `POST /groups/{group_jid}/join-requests` |

O corpo **continua a ser aceite**: se o identificador vier nos dois sítios, o
corpo ganha. É o que permite migrar um cliente de cada vez.

### As restantes, por família

**`/chat` → `/chats`** (28 rotas)

| antiga | canónica |
|---|---|
| `GET /chat/history` | `GET /chats/history` |
| `GET /chat/list` | `GET /chats/list` |
| `POST /chat/archive` | `POST /chats/archive` |
| `POST /chat/delete/message` | `POST /chats/delete/message` |
| `POST /chat/ephemeral` | `POST /chats/ephemeral` |
| `POST /chat/ephemeral/default` | `POST /chats/ephemeral/default` |
| `POST /chat/markread` | `POST /chats/markread` |
| `POST /chat/mute` | `POST /chats/mute` |
| `POST /chat/pin` | `POST /chats/pin` |
| `POST /chat/presence` | `POST /chats/presence` |
| `POST /chat/react` | `POST /chats/react` |
| `POST /chat/request-unavailable-message` | `POST /chats/request-unavailable-message` |
| `POST /chat/send/audio` | `POST /chats/send/audio` |
| `POST /chat/send/buttons` | `POST /chats/send/buttons` |
| `POST /chat/send/carousel` | `POST /chats/send/carousel` |
| `POST /chat/send/contact` | `POST /chats/send/contact` |
| `POST /chat/send/document` | `POST /chats/send/document` |
| `POST /chat/send/edit` | `POST /chats/send/edit` |
| `POST /chat/send/forward` | `POST /chats/send/forward` |
| `POST /chat/send/image` | `POST /chats/send/image` |
| `POST /chat/send/list` | `POST /chats/send/list` |
| `POST /chat/send/location` | `POST /chats/send/location` |
| `POST /chat/send/poll` | `POST /chats/send/poll` |
| `POST /chat/send/pollvote` | `POST /chats/send/pollvote` |
| `POST /chat/send/sticker` | `POST /chats/send/sticker` |
| `POST /chat/send/template` | `POST /chats/send/template` |
| `POST /chat/send/text` | `POST /chats/send/text` |
| `POST /chat/send/video` | `POST /chats/send/video` |

**`/group` → `/groups`** (12 rotas)

| antiga | canónica |
|---|---|
| `POST /group/announce` | `POST /groups/announce` |
| `POST /group/create` | `POST /groups/create` |
| `POST /group/ephemeral` | `POST /groups/ephemeral` |
| `POST /group/info` | `POST /groups/info` |
| `POST /group/inviteinfo` | `POST /groups/inviteinfo` |
| `POST /group/invitelink` | `POST /groups/invitelink` |
| `POST /group/join` | `POST /groups/join` |
| `POST /group/leave` | `POST /groups/leave` |
| `POST /group/list` | `POST /groups/list` |
| `POST /group/locked` | `POST /groups/locked` |
| `POST /group/name` | `POST /groups/name` |
| `POST /group/topic` | `POST /groups/topic` |

**`/message` → `/messages`** (1 rotas)

| antiga | canónica |
|---|---|
| `POST /message/star` | `POST /messages/star` |

**`/newsletter` → `/newsletters`** (18 rotas)

| antiga | canónica |
|---|---|
| `DELETE /newsletter/delete` | `DELETE /newsletters/delete` |
| `GET /newsletter/list` | `GET /newsletters/list` |
| `POST /newsletter/admin-invite` | `POST /newsletters/admin-invite` |
| `POST /newsletter/admin-invite/accept` | `POST /newsletters/admin-invite/accept` |
| `POST /newsletter/admin-invite/revoke` | `POST /newsletters/admin-invite/revoke` |
| `POST /newsletter/change-owner` | `POST /newsletters/change-owner` |
| `POST /newsletter/create` | `POST /newsletters/create` |
| `POST /newsletter/demote` | `POST /newsletters/demote` |
| `POST /newsletter/follow` | `POST /newsletters/follow` |
| `POST /newsletter/info` | `POST /newsletters/info` |
| `POST /newsletter/info-invite` | `POST /newsletters/info-invite` |
| `POST /newsletter/mark-viewed` | `POST /newsletters/mark-viewed` |
| `POST /newsletter/messages` | `POST /newsletters/messages` |
| `POST /newsletter/mute` | `POST /newsletters/mute` |
| `POST /newsletter/react` | `POST /newsletters/react` |
| `POST /newsletter/subscribe` | `POST /newsletters/subscribe` |
| `POST /newsletter/unfollow` | `POST /newsletters/unfollow` |
| `POST /newsletter/updates` | `POST /newsletters/updates` |

**`/user` → `/users`** (15 rotas)

| antiga | canónica |
|---|---|
| `GET /user/blocklist` | `GET /users/blocklist` |
| `GET /user/contacts` | `GET /users/contacts` |
| `GET /user/contacts/last-activity` | `GET /users/contacts/last-activity` |
| `GET /user/privacy` | `GET /users/privacy` |
| `POST /user/avatar` | `POST /users/avatar` |
| `POST /user/block` | `POST /users/block` |
| `POST /user/check` | `POST /users/check` |
| `POST /user/contacts/sync` | `POST /users/contacts/sync` |
| `POST /user/history/sync` | `POST /users/history/sync` |
| `POST /user/info` | `POST /users/info` |
| `POST /user/presence` | `POST /users/presence` |
| `POST /user/presence/subscribe` | `POST /users/presence/subscribe` |
| `POST /user/privacy` | `POST /users/privacy` |
| `POST /user/status` | `POST /users/status` |
| `POST /user/unblock` | `POST /users/unblock` |

### Fora da tabela: duas renomeações simples e uma consolidação (CAP-10, 2026-08-27)

| antiga | canónica |
|---|---|
| `GET /session/qr` | `GET /session/pair/qr` |
| `POST /session/pairphone` | `POST /session/pair/phone` |

As duas rotas de pareamento (QR e telefone) passam a viver sob `/session/pair/`
— relação explícita em vez de dois nomes soltos que só a documentação
associava.

**Consolidação das cinco rotas de descarga.** As cinco `/chats/download{tipo}`
que a F269 tinha pluralizado (`/chats/downloadimage` etc.) foram **retiradas
do contrato e do serviço** — não são mais servidas — a favor de
`POST /chats/download/{kind}`, com o `kind` (`image`, `video`, `audio`,
`document`, `sticker`) na RELAÇÃO do caminho em vez de colado ao nome. A
pluralização sozinha não bastava: `downloadimage` continuava a violar a
regra 2 de `api/openapi/CAMINHOS-CANONICOS.md` (verbo colado ao tipo). As
CINCO formas originais, singulares (`/chat/downloadimage` etc.), continuam a
responder — coexistência normal — mas não têm mais uma forma canónica
1-para-1: o gate de cobertura reconhece-as como cobertas pela consolidação,
não por uma linha em `caminhos.tsv` (ver `openapi_coverage_test.go` e
`caminhos_canonicos_test.go`, exceção CAP-10). Corpo igual
(`PedidoDescargaDeMidia`, sem `Kind`); um `Kind` no corpo, se vier, não
sobrescreve o `{kind}` do caminho.

---

## Verificação em campo — bateria de 2026-08-26

**Método**: chamadas reais da conta `+55 16 98181-8244` (`filarapida`, Business)
para `+55 41 9242-1234` (`lucas`), contra o binário construído do `HEAD` de
`feature/wa-noise`, com confirmação visual no `web.whatsapp.com` para tudo o
que produz mensagem. Cada corpo abaixo é o corpo **que foi enviado**, não um
corpo derivado da struct — a diferença importa, e a secção
"O que a bateria corrigiu nos meus próprios exemplos" diz porquê.

> **Actualizado a 2026-08-26, depois de três correcções.** As marcas e as
> contagens abaixo **não mudaram** — e é correcto que não tenham mudado: as
> correcções alteraram apenas respostas de RECUSA, e recusa medida não confirma
> efeito. O que mudou foram os códigos de erro de algumas rotas:
>
> | rota | antes | depois |
> |---|---|---|
> | as 14 de canal que exigem `jid` | `500 newsletter_failed` | `400 invalid_newsletter_jid` |
> | `/user/contacts/sync` | `invalid_sync_mode` para tudo | `missing_sync_mode` vs `invalid_sync_mode` |
> | `/user/presence` | `invalid_presence_type` para tudo | `missing_presence_type` vs `invalid_presence_type` |
> | `/user/privacy` | dois códigos | quatro, com a ordem de validação medida |
> | toda a API | campo desconhecido em silêncio | registado no log; recusável com `WA_API_STRICT_UNKNOWN_FIELDS` |

**Legenda**

| marca | significa |
|---|---|
| ✅ | chamada real, `200`, **e** efeito confirmado (visual, ou transição de estado medida) |
| 🟡 | chamada real, `200`, efeito **não** confirmado por observador independente |
| ❌ | chamada real, **falhou** — com o erro medido |
| ⬜ | **não testada**, com o motivo dito |

Todos os exemplos assumem `-H 'token: <TOKEN>' -H 'Content-Type: application/json'`.

---

### chat — envio (15 rotas)

| rota | | corpo enviado |
|---|---|---|
| `POST /chat/send/text` | ✅ | `{"Phone":"554192421234@s.whatsapp.net","Body":"texto"}` |
| `POST /chat/send/image` | ✅ | `{"Phone":"…","Image":"data:image/png;base64,iVBOR…","Caption":"legenda"}` |
| `POST /chat/send/video` | ✅ | `{"Phone":"…","Video":"data:video/mp4;base64,AAAA…","Caption":"legenda"}` |
| `POST /chat/send/audio` | ✅ | `{"Phone":"…","Audio":"data:audio/mp4;base64,AAAA…"}` |
| `POST /chat/send/document` | ✅ | `{"Phone":"…","Document":"data:application/pdf;base64,JVBER…","FileName":"ficheiro.pdf"}` |
| `POST /chat/send/sticker` | 🟡 | `{"Phone":"…","Sticker":"data:image/webp;base64,UklGR…"}` |
| `POST /chat/send/location` | ✅ | `{"Phone":"…","Latitude":-25.4284,"Longitude":-49.2733,"Name":"Curitiba"}` |
| `POST /chat/send/contact` | ✅ | `{"Phone":"…","Name":"Contacto","Vcard":"BEGIN:VCARD\nVERSION:3.0\nFN:Contacto\nTEL;type=CELL;waid=554192421234:+55 41 9242-1234\nEND:VCARD"}` |
| `POST /chat/send/poll` | ✅ | `{"Group":"554192421234@s.whatsapp.net","Header":"Pergunta","Options":["A","B"]}` |
| `POST /chat/send/pollvote` | ✅ | `{"Phone":"…","Sender":"…","PollMessageId":"3EB0…","PollMessageTimestamp":1787748016,"Options":["A"]}` |
| `POST /chat/send/buttons` | ✅ | `{"Phone":"…","Title":"t","Body":"corpo","Footer":"rodapé","Buttons":[{"type":"reply","title":"Sim","id":"s"}]}` |
| `POST /chat/send/list` | ✅ | `{"Phone":"…","ButtonText":"Ver","TopText":"topo","Desc":"corpo","FooterText":"rodapé","Sections":[{"title":"S1","rows":[{"title":"Op1","desc":"d1","RowId":"r1"}]}]}` |
| `POST /chat/send/carousel` | ✅ | `{"Phone":"…","Body":"corpo","Cards":[{"Title":"C1","Body":"b1","Buttons":[{"type":"reply","title":"Ok","id":"o1"}]}]}` |
| `POST /chat/send/template` | ✅ | `{"Phone":"…","Content":"texto","Footer":"rodapé","Buttons":[{"DisplayText":"Abrir","Url":"https://exemplo.com","Type":"url"}]}` |
| `POST /chat/send/forward` | ✅ | por chave: `{"Phone":"…","MessageID":"3EB0…","Chat":"554192421234@s.whatsapp.net"}` — por conteúdo: `{"Phone":"…","Body":"texto"}` |

**Três regras de payload que a bateria mediu**, e que nenhuma leitura das
structs teria dado:

1. **`Buttons` quer `title`, não `displayText`.** Um botão sem `title`/`text`/
   `buttonText` é descartado em silêncio, e se todos forem descartados a rota
   devolve `400 no_valid_buttons`. Os tipos aceites são `reply`, `cta_url`,
   `cta_call`, `copy` — `quickreply` vale em `/chat/send/template` e **não**
   aqui.
2. **`/chat/send/poll` usa `Group`, não `Phone`** — mesmo para enquete enviada
   a um contacto individual. Com `Phone` responde `400 missing_group`.
3. **Vídeo e áudio têm mínimo de bytes**: `256` para vídeo
   (`send_video.go:36`) e `128` para áudio (`send_audio.go:34`). Abaixo disso
   é `400 video_too_small` / `audio_too_small` **antes** de qualquer envio.

**Nota sobre o `sticker` (🟡)**: a rota devolveu `200` e a mensagem chegou, mas
o WebP que usei é sintético e o cliente desenha uma bolha vazia com "25 B".
A rota está exercitada; o autocolante não está provado.

**Nota sobre o carrossel no Web**: o cliente Web desenha *"Não foi possível
carregar a mensagem. Use seu celular para acessá-la."* — é limitação do Web
para tipos interativos, e não falha do envio. Está fotografado a funcionar no
telemóvel (F240).

### chat — ações sobre mensagem (4)

| rota | | corpo enviado |
|---|---|---|
| `POST /chat/send/edit` | ✅ | `{"Phone":"…","Id":"3EB0…","Body":"texto corrigido"}` |
| `POST /chat/react` | ✅ | `{"Phone":"…","Id":"3EB0…","Body":"👍"}` (`"Body":""` remove) |
| `POST /chat/delete/message` | ✅ | `{"Phone":"…","Id":"3EB0…"}` |
| `POST /message/star` | ✅ | `{"chat":"…","sender":"5516981818244@s.whatsapp.net","message_id":"3EB0…","from_me":true,"star":true}` |

`star` exige `sender` — sem ele é `400 missing_sender`. Era `409` até à F223;
com o CAP-54 responde `200`.

### chat — descarga de média (5)

Todas ✅. O corpo é o descritor de média, tirado do `data_json` da mensagem em
`GET /chat/history` (campos `URL`, `directPath`, `mediaKey`, `mimetype`,
`fileEncSha256`, `fileSha256`, `fileLength`):

```json
{"Url":"https://mmg.whatsapp.net/o1/v/t24/…","DirectPath":"/o1/v/t24/…",
 "MediaKey":"JZdDIuwqVp1Z…","Mimetype":"image/png",
 "FileEncSHA256":"…","FileSHA256":"…","FileLength":74}
```

| rota | | medido |
|---|---|---|
| `POST /chat/downloadimage` | ✅ | `image/png`, 122 B de base64 |
| `POST /chat/downloadvideo` | ✅ | `video/mp4`, 2 010 098 B — ida e volta de 1,5 MB real |
| `POST /chat/downloaddocument` | ✅ | `application/pdf`, 48 B |
| `POST /chat/downloadsticker` | ✅ | `image/webp`, 59 B |
| `POST /chat/downloadaudio` | ✅ | `audio/mp4`, 14 130 B |

### chat — gestão de conversa (10)

| rota | | corpo enviado |
|---|---|---|
| `POST /chat/markread` | ✅ | `{"Chat":"…","Id":["3EB0…"]}` |
| `POST /chat/presence` | ✅ | `{"Phone":"…","State":"composing","Media":""}` |
| `POST /chat/archive` | ✅ | `{"jid":"…","archive":true}` |
| `POST /chat/pin` | ✅ | `{"jid":"…","pin":true}` |
| `POST /chat/mute` | ✅ | `{"jid":"…","mute":true,"mute_duration":28800000000000}` |
| `POST /chat/ephemeral` | ✅ | `{"chat":"…","duration":"24h"}` |
| `POST /chat/ephemeral/default` | ✅ | `{"duration":"0"}` |
| `POST /chat/request-unavailable-message` | 🟡 | `{"chat":"…","sender":"…","id":"3EB0…"}` |
| `GET /chat/list` | ✅ | — |
| `GET /chat/history` | ✅ | `?chat_jid=554192421234@s.whatsapp.net&limit=40` |

**`mute_duration` é NANOSSEGUNDOS, não uma string de duração.** É um
`*time.Duration` (`pkg/domain/mute.go:17`), logo `"8h"` devolve
`400 could_not_decode_payload` e `28800000000000` funciona. É a única rota do
inventário com esta forma — `/chat/ephemeral` aceita `"24h"` como texto.

**O parâmetro de `GET /chat/history` chama-se `chat_jid`**, não `chat`
(`handler_chat_history.go:60`). Com `chat` é `400 missing_chat_jid`.

### group — 18 rotas

| rota | | corpo enviado |
|---|---|---|
| `POST /group/create` | ✅ | `{"name":"Grupo","participants":["554192421234"]}` |
| `POST /group/create` (comunidade) | ✅ | `{"name":"Comunidade","is_parent":true}` |
| `POST /group/create` (subgrupo) | 🟡 | `{"name":"Sub","participants":["…"],"linked_parent_jid":"1203…@g.us"}` |
| `POST /group/info` | ✅ | `{"groupJID":"1203…@g.us"}` |
| `POST /group/list` | ✅ | `{}` |
| `POST /group/name` | ✅ | `{"groupJID":"…","name":"Novo nome"}` |
| `POST /group/topic` | ✅ | `{"groupJID":"…","topic":"Novo tópico"}` |
| `POST /group/photo` | ✅ | `{"groupJID":"…","photo":"<base64 CRU, JPEG>"}` |
| `POST /group/photo/remove` | ✅ | `{"groupjid":"…"}` |
| `POST /group/announce` | ✅ | `{"groupJID":"…","announce":true}` |
| `POST /group/locked` | ✅ | `{"groupJID":"…","locked":true}` |
| `POST /group/ephemeral` | ✅ | `{"groupjid":"…","duration":"24h"}` |
| `POST /group/invitelink` | ✅ | `{"groupJID":"…"}` |
| `POST /group/inviteinfo` | ✅ | `{"Code":"IVccRoDVSbpHKNkgNxyZx4"}` |
| `POST /group/join` | ✅ | `{"code":"IVccRoDVSbpHKNkgNxyZx4"}` |
| `POST /group/leave` | ✅ | `{"groupJID":"…"}` |
| `POST /group/updateparticipants` | ✅ | `{"groupJID":"…","Phone":["554192421234"],"Action":"add"}` |
| `POST /group/joinapprovalmode` | ✅ | `{"groupjid":"…","mode":true}` |
| `GET /group/requestparticipants` | ✅ | corpo JSON, apesar de ser `GET`: `{"groupJID":"…"}` |
| `POST /group/updaterequestparticipants` | 🟡 | `{"groupJID":"…","Phone":["554192421234"],"Action":"approve"}` |

**Quatro armadilhas de payload medidas nesta família:**

1. **`/group/photo` quer base64 CRU**, sem o prefixo `data:image/jpeg;base64,`.
   Isolado numa medição com uma só variável: os mesmos nomes de campo e a mesma
   imagem, com prefixo dão `400` e sem prefixo dão `200`. É divergente de
   `/chat/send/image`, que aceita o URI de dados. E a imagem tem de ser
   **JPEG** — PNG produz `422 upstream_rejected`.
2. **`/group/join` quer `code`**, o código nu, não `inviteLink` nem o URL
   completo (`handler_group_mgmt.go:148`). O `GroupJoinRequest` do domínio
   declara `inviteLink` e **não é o que a rota lê**.
3. **`/group/inviteinfo` quer `Code`** — também o código nu, não o link.
4. **`GET /group/requestparticipants` lê o corpo**, não a query string. Um
   `GET` com corpo é servido pelo `curl`, mas quebra clientes que assumem que
   `GET` não o tem.

**`Action` só aceita `add` e `remove`.** `promote` e `demote` devolvem
`400 invalid_action` — ver HOUSEKEEP F263: a biblioteca sabe fazê-lo
(`internal/wa-noise/capabilities/group/participants.go:18-19`), a rota não o
expõe.

### community — 4 rotas (F237)

Todas ✅, com a transição de sub-grupos medida `1 → 2 → 1` e o log de sistema
do grupo de avisos a confirmar no cliente.

| rota | | corpo enviado |
|---|---|---|
| `POST /community/subgroups` | ✅ | `{"communityJID":"1203…@g.us"}` |
| `POST /community/participants` | ✅ | `{"communityJID":"1203…@g.us"}` |
| `POST /community/link` | ✅ | `{"communityJID":"1203…@g.us","groupJID":"1203…@g.us"}` |
| `POST /community/unlink` | ✅ | `{"communityJID":"1203…@g.us","groupJID":"1203…@g.us"}` |

**Comunidade criada por conta WhatsApp Business.** A ajuda oficial diz que não
é possível; é afirmação sobre o CLIENTE. Pelo protocolo funciona — medido, e a
F260 do HOUSEKEEP foi corrigida por causa disto.

### newsletter (canais) — 18 rotas

| rota | | corpo enviado |
|---|---|---|
| `POST /newsletter/create` | ✅ | `{"name":"Canal","description":"descrição"}` |
| `POST /newsletter/info` | ✅ | `{"jid":"1203…@newsletter"}` |
| `POST /newsletter/info-invite` | ✅ | `{"invite":"0029Vb8NIrODZ4LhzncjL82W"}` |
| `GET /newsletter/list` | ✅ | — |
| `POST /newsletter/follow` | ✅ | `{"jid":"1203…@newsletter"}` |
| `POST /newsletter/unfollow` | ✅ | `{"jid":"1203…@newsletter"}` |
| `POST /newsletter/subscribe` | ✅ | `{"jid":"1203…@newsletter"}` |
| `POST /newsletter/mute` | ✅ | `{"jid":"1203…@newsletter","mute":true}` |
| `POST /newsletter/messages` | ✅ | `{"jid":"1203…@newsletter","count":5}` |
| `POST /newsletter/mark-viewed` | 🟡 | `{"jid":"1203…@newsletter","serverIDs":[1]}` |
| `POST /newsletter/react` | 🟡 | `{"jid":"1203…@newsletter","serverID":1,"reaction":"👍"}` |
| `POST /newsletter/admin-invite` | ✅ | `{"jid":"1203…@newsletter","userJID":"90937376170214@lid"}` |
| `POST /newsletter/admin-invite/accept` | ✅ | `{"jid":"1203…@newsletter"}` |
| `POST /newsletter/admin-invite/revoke` | ✅ | `{"jid":"1203…@newsletter","userJID":"…@lid"}` |
| `POST /newsletter/change-owner` | ✅ | `{"jid":"1203…@newsletter","userJID":"…@lid"}` |
| `POST /newsletter/demote` | ✅ | `{"jid":"1203…@newsletter","userJID":"…@lid"}` |
| `DELETE /newsletter/delete` | ✅ | `{"jid":"1203…@newsletter","confirmJID":"1203…@newsletter"}` |
| `POST /newsletter/updates` | ❌ | `{"jid":"1203…@newsletter","count":5}` → **`500` ao fim de 30 s**, `context deadline exceeded`. Ver HOUSEKEEP F265. |

**A cadeia de administração de canal está fechada** (F233), e é a única forma
de chegar a segundo administrador:

```
create → admin-invite → admin-invite/accept → change-owner → demote → delete
 owner      (convite)        role=admin         role=owner    subscriber  non_existing
```

O `userJID` pode ir em PN ou LID — a resolução PN→LID é feita antes do envio
(F233c).

### user — 16 rotas

| rota | | corpo enviado |
|---|---|---|
| `POST /user/check` | ✅ | `{"phone":["554192421234"]}` |
| `POST /user/info` | ✅ | `{"Phone":["554192421234@s.whatsapp.net"]}` |
| `GET /user/contacts` | ✅ | — |
| `GET /user/contacts/last-activity` | ✅ | — |
| `GET /user/privacy` | ✅ | — |
| `GET /user/profile/{jid}` | ✅ | `/user/profile/554192421234@s.whatsapp.net` |
| `GET /user/lid/{jid}` | ✅ | `/user/lid/554192421234@s.whatsapp.net` |
| `GET /user/blocklist` | ✅ | — |
| `POST /user/presence` | ✅ | `{"type":"available"}` |
| `POST /user/presence/subscribe` | ✅ | `{"Phone":"554192421234@s.whatsapp.net"}` |
| `POST /user/contacts/sync` | ✅ | `{"mode":"if_unsynced"}` — só `if_unsynced`, `incremental`, `full`; ausente dá `missing_sync_mode`, valor errado dá `invalid_sync_mode` |
| `POST /user/history/sync` | ✅ | `{"count":5,"chat_jid":"…","oldest_msg_id":"3EB0…","oldest_msg_from_me":true,"oldest_msg_timestamp":1787747900}` |
| `POST /user/block` | ❌ | `{"Phone":"554192421234@s.whatsapp.net"}` → **`422 upstream_rejected`** |
| `POST /user/unblock` | ❌ | idem |
| `POST /user/avatar` | ⬜ | altera o avatar da conta — proibido nesta sessão |
| `POST /user/status` | ⬜ | altera o recado da conta — não testado sem aval |
| `POST /user/privacy` | ⬜ | altera definições da conta — não testado sem aval |

**`/user/block` e `/user/unblock` falham nas DUAS contas**, com PN e com LID:
o WhatsApp devolve `info query returned status 400: bad-request`. Não é
específico do número nem do tipo de conta — ver HOUSEKEEP F264.

### status (stories) — 3 rotas

| rota | | corpo enviado |
|---|---|---|
| `POST /status/set/image` | 🟡 | `{"Image":"data:image/png;base64,…","Caption":"legenda"}` |
| `POST /status/set/video` | 🟡 | `{"Video":"data:video/mp4;base64,…","Caption":"legenda"}` |
| `POST /status/set/audio` | 🟡 | `{"Audio":"data:audio/mp4;base64,…"}` |

**Não há `POST /status/set/text`** — a rota não existe (`404`). O recado de
texto é `POST /user/status`, que é outra coisa.

**Os três `200` são enganadores numa conta Business.** A lista de destinatários
é construída a partir de `contact.FullName`, e uma conta Business recebe os
contactos por `push_name`: 461 contactos produzem **dois** destinatários.
Medido, com o comparativo da conta pessoal (1266 contactos → 165
destinatários). Detalhe em HOUSEKEEP F256.

### infraestrutura — 19 rotas

| rota | | nota |
|---|---|---|
| `GET /health` | ✅ | autenticada |
| `GET /health/live`, `GET /health/ready`, `GET /livez` | ✅ | **sem** autenticação |
| `GET /admin/users`, `GET /admin/users/{id}` | ✅ | `-H 'Authorization: <ADMIN_TOKEN>'`. O token é **regerado a cada arranque** e está em `<datadir>/admin_token` |
| `GET /labels`, `GET /labels/{id}/chats` | ✅ | — |
| `GET /webhook`, `GET /webhook/history` | ✅ | leitura |
| `GET /s3/config`, `GET /session/s3/config` | ✅ | leitura |
| `GET /hmac/config`, `GET /session/hmac/config` | ✅ | leitura |
| `GET /session/status`, `/session/profile`, `/session/profile/full`, `/session/pair/qr` | ✅ | leitura |
| `POST/PUT/DELETE /webhook`, `POST /webhook/history` | ⬜ | escreve configuração — não testado sem aval |
| `POST/DELETE /s3/config`, `POST /s3/configure`, `POST /s3/test`, `POST/DELETE /session/s3/config`, `POST /session/s3/test` | ⬜ | idem |
| `POST/DELETE /hmac/config`, `/hmac/configure` | ⬜ | idem |
| `POST /session/history` | ⬜ | idem |
| `POST /admin/users`, `PUT /admin/users/{id}`, `DELETE /admin/users/{id}`, `DELETE /admin/users/{id}/full` | ⬜ | cria/apaga utilizadores |
| `GET /session/connect`, `/session/disconnect`, `POST /session/logout`, `/session/pair/phone` | ⬜ | derrubaria as sessões vivas |
| `POST /proxy/set`, `POST /session/proxy` | ⬜ | mudaria a rede da sessão |
| `GET /session/ws` | ⬜ | WebSocket, fora do alcance de `curl` |
| `POST /call/reject` | ⬜ | exige chamada a entrar |

---

## O que a bateria corrigiu nos meus próprios exemplos

Esta secção existe porque a primeira passagem da bateria produziu **oito**
`400` que eu tinha escrito como se fossem corpos válidos. Todos vinham de eu
ter derivado o exemplo da struct de domínio em vez de o medir:

| rota | o que eu escrevi | o que a rota quer |
|---|---|---|
| `/chat/send/buttons` | `"displayText"` | `"title"` |
| `/chat/send/carousel` | cartão sem `Buttons` | `Buttons` obrigatório por cartão |
| `/chat/send/poll` | `"Phone"` | `"Group"` |
| `/chat/mute` | `"mute_duration":"8h"` | nanossegundos: `28800000000000` |
| `/message/star` | sem `sender` | `sender` obrigatório |
| `/chat/history` | `?chat=` | `?chat_jid=` |
| `/group/join` | `"inviteLink"` + URL | `"code"` + código nu |
| `/group/inviteinfo` | `"inviteLink"` | `"Code"` |
| `/group/photo` | `data:image/…;base64,` | base64 cru, e JPEG |
| `/user/contacts/sync` | `"delta"` | `if_unsynced` \| `incremental` \| `full` |

**A regra que daqui sai**: a struct do domínio **não é** o contrato da rota.
Onze handlers de grupo declaram structs anónimas próprias
(`handler_group_mgmt.go`), e onde o nome do campo diverge é essa struct que
manda — o `encoding/json` do Go casa nomes ignorando maiúsculas, o que
esconde metade das divergências e deixa as outras (`code` vs `inviteLink`) a
falhar sem pista. Documentar a partir do código-fonte das structs produziria
uma referência errada em dez rotas.

## O envelope de erro não é universal (F266)

O formato da secção "Envelope de erro (F236)" vale para os erros que passam
por `*apperr.AppError`. **Catorze pontos** de `pkg/presentation/http/handlers`
ainda respondem com `fmt.Errorf`, e esses produzem:

```json
{"code":400,"error":"bad request","success":false}
```

com `error` a ser uma **string** e não o objecto `{code,message}`. Onze deles
vêm da mesma função, `rejectMissingField` (`handler_group_mgmt.go:99`), logo
todas as recusas de campo obrigatório das rotas de grupo têm este formato. Um
cliente que leia `error.code` parte aqui.

### `DELETE /newsletter/delete` — irreversível

Exige `confirmJID` **igual** ao `jid` do canal. Sem ele, ou com valor
diferente, devolve `400 missing_confirm_jid` e **não contacta o WhatsApp**.

```bash
curl -X DELETE http://localhost:8080/newsletter/delete \
  -H 'token: <TOKEN>' -H 'Content-Type: application/json' \
  -d '{"jid":"1203…@newsletter","confirmJID":"1203…@newsletter"}'
```

Só o dono pode apagar. Depois de apagado:

- `GET /newsletter/list` deixa de o listar;
- `POST /newsletter/info` devolve `state: non_existing` e `viewer_metadata: null`;
- o link público de convite passa a `Link de convite inválido`.

Não há como desfazer.

> **`/webhook/history` configura o histórico de MENSAGENS** (HOUSEKEEP F252).
> O nome sugere um registo de entregas de webhook; o efeito é sobre a coluna
> `users.history`, a mesma que `POST /session/history` altera e que governa a
> gravação em `message_history` (F227, F230).
>
> As duas rotas são o mesmo handler. O caminho público mantém-se para não
> quebrar clientes existentes — a decisão está registada em
> `wiring_routes.go:133-137`.
>
> Para ler o histórico de mensagens de uma conversa, use `GET /chat/history`,
> que é outro handler (a distinção é travada por
> `TestChatHistoryAndWebhookHistoryAreDistinctHandlers`, criado pela F124).
