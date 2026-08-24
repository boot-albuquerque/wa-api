# Endpoints do wa-api — inventário e comparação

**Levantamento**: 2026-08-24, contra `feature/wa-noise`.
**Total**: 115 rotas registadas em `pkg/bootstrap/wiring_routes.go`.

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

## chat — 32 rotas

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
| POST | `/chat/send/pollvote` | **votar** numa enquete (CAP-48) |
| POST | `/chat/send/buttons` | botões interativos (fluxo nativo) |
| POST | `/chat/send/list` | lista de seleção |
| POST | `/chat/send/carousel` | carrossel HSCROLL_CARDS (CAP-46) |
| POST | `/chat/send/template` | mensagem de template |
| POST | `/chat/send/forward` | encaminhar, com `IsForwarded`/`ForwardingScore` (CAP-49) |

Todas as rotas de envio aceitam **reply-to** (`ReplyTo`) desde a CAP-46; as
oito com texto visível aceitam **menções** (`MentionedJid`) desde a CAP-47.

### Edição e remoção (2)

| método | rota | o que faz |
|---|---|---|
| POST | `/chat/send/edit` | editar mensagem enviada |
| POST | `/chat/delete/message` | revogar mensagem enviada |

### Descarga de média (5)

| método | rota | o que faz |
|---|---|---|
| POST | `/chat/downloadimage` | baixa e decifra imagem recebida |
| POST | `/chat/downloadvideo` | idem, vídeo |
| POST | `/chat/downloadaudio` | idem, áudio |
| POST | `/chat/downloaddocument` | idem, documento |
| POST | `/chat/downloadsticker` | idem, autocolante |

### Gestão de conversa (10)

| método | rota | o que faz |
|---|---|---|
| GET | `/chat/list` | lista as conversas |
| GET | `/chat/history` | histórico de mensagens de uma conversa |
| POST | `/chat/archive` | arquivar / desarquivar |
| POST | `/chat/delete` | apagar conversa |
| POST | `/chat/markread` | marcar como lida |
| POST | `/chat/presence` | "a escrever" / "a gravar" |
| POST | `/chat/react` | reagir com emoji |
| POST | `/chat/ephemeral` | temporizador de mensagens temporárias da conversa (CAP-50) |
| POST | `/chat/ephemeral/default` | temporizador padrão da conta (CAP-50) |
| POST | `/chat/request-unavailable-message` | pedir reenvio de mensagem indisponível |

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

| capacidade | wa-api | Cloud API | Evolution | Baileys |
|---|---|---|---|---|
| Envio de texto e média | ✅ | ✅ | ✅ | ✅ |
| Botões e lista | ✅ | ⚠️ só via template aprovado | ✅ | ✅ |
| **Carrossel** | ✅ provado em campo | ❌ | ⚠️ | ⚠️ proto cru |
| **Reply-to e menções** | ✅ | ✅ | ✅ | ✅ |
| **Encaminhar** | ✅ | ⚠️ | ✅ | ✅ |
| **Votar em enquete** | ✅ | ❌ | ⚠️ | ✅ |
| Editar / apagar / reagir | ✅ | ⚠️ parcial | ✅ | ✅ |
| **Grupos** (18 rotas) | ✅ | ❌ **não existe** | ✅ | ✅ |
| **Canais** (12 rotas) | ✅ | ❌ | ⚠️ parcial | ✅ |
| **Status / stories** | ✅ 4 tipos | ❌ | ⚠️ | ✅ |
| Contactos, bloqueio, privacidade | ✅ | ❌ | ✅ | ✅ |
| Presença e recibos | ✅ | ⚠️ limitado | ✅ | ✅ |
| Mensagens temporárias | ✅ conversa + padrão | ⚠️ | ⚠️ | ✅ |
| Etiquetas | ✅ | ❌ | ⚠️ | ✅ |
| Rejeitar chamada | ✅ | ❌ | ⚠️ | ✅ |
| Webhook, S3, HMAC, proxy | ✅ | ✅ só webhook | ✅ | ➖ é biblioteca |
| Catálogo / produtos | ❌ | ✅ | ⚠️ | ⚠️ |
| Flows | ❌ | ✅ | ❌ | ❌ |
| Pagamento | ❌ | ✅ WABA | ❌ | ❌ |

**A vantagem estrutural sobre a Cloud API são grupos e canais**: não existem na
API oficial. Quem precisa de bot em grupo não tem alternativa oficial.

---

## O que falta — a lista completa

Seis itens, e mais nada. Todo o resto da superfície está entregue.

### Falta no próprio protocolo (4)

Nenhum destes existe em `internal/wa-noise/core`. Não é lacuna de rota: o fork
não tem o primitivo, e implementá-los é trabalho de protocolo.

| capacidade | símbolo ausente | nota |
|---|---|---|
| **Fixar mensagem** | `PinInChat` | Baileys tem; nós teríamos de construir |
| **Favoritar (star)** | `StarMessage` | é estado local do cliente; confirmar se viaja no wire |
| **Silenciar conversa** | `MuteChat` | provavelmente app-state, não mensagem — outro mecanismo |
| **Comunidades** | uma única menção no fork | praticamente ausente; exige levantamento próprio |

Reproduzir a ausência:

```
for k in PinInChat StarMessage MuteChat Community; do
  echo -n "$k: "; grep -rl "$k" internal/wa-noise/core/*.go | wc -l
done
```

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

**Toda a superfície de envio está funcional.**

Nota de proveniência, porque a distinção custou caro a este projeto: as três
primeiras linhas foram medidas por fotografia do telemóvel durante a
implementação, com par de controlo onde havia hipótese a refutar. As cinco
últimas são **confirmação do utilizador** (2026-08-24), não medição minha —
registado assim para que ninguém as leia como prova de campo que eu não
recolhi.
