# Endpoints do wa-api — inventário e comparação

**Levantamento**: 2026-08-24, contra `feature/wa-noise`.
**Total**: 117 rotas registadas em `pkg/bootstrap/wiring_routes.go`.

Reproduzir a lista:

```
grep -oE 'registry\.Register\("(/[a-zA-Z0-9/{}._-]+)"' pkg/bootstrap/wiring_routes.go \
  | sed 's/registry.Register("//;s/"//' | sort
```

## Aviso de método

As colunas de **concorrentes** são conhecimento geral, **não medição**. Não
corri Cloud API, Evolution nem Baileys. Onde a diferença decidir alguma coisa,
verifique antes de agir.

As colunas do **wa-api** são medidas (rota registada = existe). Mas existir não
é o mesmo que estar provado no cliente — ver a secção "Verificação em campo".

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
>
> Cuidado ao diagnosticar (F230): a tabela pode ter dezenas de milhares de
> linhas vindas da SINCRONIZAÇÃO mesmo com `history = 0`, porque esse caminho
> não aplica a mesma trava. **Histórico cheio não significa histórico ligado** —
> foi essa a conclusão errada que atrasou o diagnóstico da F227.

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

### Descarga de média (5)

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
| POST | `/chat/delete` | apagar conversa |
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
| GET | `/session/qr` | código QR de pareamento |
| POST | `/session/pairphone` | parear por código de telefone |
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

## newsletter (canais) — 12 rotas

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

---

## status (stories) — 4 rotas

| método | rota | o que faz |
|---|---|---|
| POST | `/status/set/text` | publicar status de texto |
| POST | `/status/set/image` | publicar status com imagem (CAP-51) |
| POST | `/status/set/video` | publicar status com vídeo (CAP-51) |
| POST | `/status/set/audio` | publicar status com áudio (CAP-51) |

---

## Infraestrutura — 19 rotas

| método | rota | o que faz |
|---|---|---|
| GET/POST/PUT/DELETE | `/webhook` | configurar webhook da sessão |
| GET/POST | `/webhook/history` | histórico de entregas |
| GET | `/s3/config` | ler configuração S3 global |
| POST | `/s3/configure` | definir configuração S3 global |
| DELETE | `/s3/config` | remover |
| POST | `/s3/test` | testar |
| GET | `/hmac/config` | ler configuração HMAC |
| POST | `/hmac/configure` | definir |
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
| **wa-api** | serviço HTTP sobre fork Go do protocolo WA Web | 117 rotas REST |
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
| **canais (newsletter)** | ✅ 12 rotas | ⚠️ parcial | ✅ | ❌ |
| **comunidades** | ❌ | ⚠️ | ⚠️ | ⚠️ |
| contactos, bloqueio, privacidade | ✅ | ✅ | ✅ | ✅ |
| **status / stories** | ✅ 4 tipos | ✅ | ✅ | ✅ |
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

## Verificação em campo

Rota registada e gate verde **não provam** que o cliente desenha a mensagem.
Neste projeto isso divergiu três vezes: carrossel, álbum e PIX devolveram todos
`200` com `message_id`, e só o carrossel renderizou.

| capability | gates | funcional |
|---|---|---|
| Botões, lista | ✅ | ✅ fotografado |
| Carrossel HSCROLL | ✅ | ✅ fotografado, 6 direções, iOS e Android |
| Reply-to | ✅ | ✅ fotografado, com par de controlo (F222) |
| Menções | ✅ | ✅ confirmado pelo utilizador |
| Encaminhar | ✅ | ✅ confirmado pelo utilizador |
| Votar em enquete | ✅ | ✅ confirmado pelo utilizador |
| Mensagens temporárias | ✅ | ✅ confirmado pelo utilizador |
| Status com média | ✅ | ✅ confirmado pelo utilizador |
| Fixar conversa | ✅ | ✅ **200 em campo** (`pin` e `unpin`) |
| Silenciar conversa | ✅ | ❌ **409 em campo** — F223 |
| Favoritar mensagem | ✅ | ❌ **409 em campo** — F223 |

**Toda a superfície de ENVIO está funcional.** Das três capabilities de
app-state, só o `pin` funciona — as outras duas estão bloqueadas pela F223.

Nota de proveniência, porque a distinção custou caro a este projeto: as três
primeiras linhas foram medidas por fotografia do telemóvel durante a
implementação, com par de controlo onde havia hipótese a refutar. As cinco
últimas são **confirmação do utilizador** (2026-08-24), não medição minha —
registado assim para que ninguém as leia como prova de campo que eu não
recolhi.
