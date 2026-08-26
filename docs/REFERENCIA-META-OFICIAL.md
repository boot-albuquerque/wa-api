# Referência oficial — WhatsApp Business Platform (Meta)

**Levantado a 2026-08-26**, das páginas oficiais da Meta. Cada linha desta
página tem URL verificado; o que **não** foi verificado está dito como tal.

## Por que este documento existe

O `wa-api` **não** usa a API oficial da Meta. Ele fala o protocolo do WhatsApp
Web directamente, através do fork em `internal/wa-noise`. São duas coisas
diferentes, e confundi-las custa caro:

| | wa-api (este projecto) | Cloud API (Meta) |
|---|---|---|
| como fala | protocolo WhatsApp Web, Noise + Signal | HTTP sobre o Graph API |
| que conta usa | uma conta de WhatsApp emparelhada, como um telemóvel | um número registado numa WABA |
| quem autoriza | ninguém — é a conta do utilizador | a Meta, com revisão do negócio |
| custo por mensagem | nenhum | tabelado, por conversa |
| janela de 24 h | não existe | existe, e fora dela só templates aprovados |
| templates | não existem | obrigatórios para iniciar conversa |
| estabilidade | depende do protocolo não mudar | contrato versionado |

**A finalidade deste ficheiro é a comparação futura**, e eventualmente uma
integração. Não é uma promessa de que ela vai acontecer.

## Índice da referência oficial

A Meta reorganizou a documentação: os caminhos antigos
`/docs/whatsapp/cloud-api/...` redireccionam para
`/documentation/business-messaging/whatsapp/...`. Ambos respondem hoje; os
segundos são os actuais.

### Nó: número de telefone do negócio

| API | endpoint | URL |
|---|---|---|
| Message API | `POST /{version}/{phone-number-id}/messages` | [ref](https://developers.facebook.com/documentation/business-messaging/whatsapp/reference/whatsapp-business-phone-number/message-api) |
| Media Upload API | `POST /{version}/{phone-number-id}/media` | [ref](https://developers.facebook.com/documentation/business-messaging/whatsapp/reference/whatsapp-business-phone-number/media-upload-api) |
| Media API | `GET /{version}/{media-id}` · `DELETE /{version}/{media-id}` | [ref](https://developers.facebook.com/documentation/business-messaging/whatsapp/reference/media/media-api) |
| Phone Number API | *(não verifiquei os métodos)* | [ref](https://developers.facebook.com/documentation/business-messaging/whatsapp/reference/whatsapp-business-phone-number/phone-number-api) |
| Register API | *(não verifiquei os métodos)* | [ref](https://developers.facebook.com/documentation/business-messaging/whatsapp/reference/whatsapp-business-phone-number/register-api) |

### Nó: WhatsApp Business Account (WABA)

| API | URL |
|---|---|
| WhatsApp Business Account API | [ref](https://developers.facebook.com/documentation/business-messaging/whatsapp/reference/whatsapp-business-account/whatsapp-business-account-api) |
| Phone Number Management API | [ref](https://developers.facebook.com/documentation/business-messaging/whatsapp/reference/whatsapp-business-account/phone-number-management-api) |
| Subscribed Apps API | [ref](https://developers.facebook.com/documentation/business-messaging/whatsapp/reference/whatsapp-business-account/subscribed-apps-api) |

### Webhooks e guias

| assunto | URL |
|---|---|
| Webhooks — visão geral | [ref](https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/overview) |
| Webhooks — payload de `messages` | [ref](https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/reference/messages) |
| Templates — fundamentos | [ref](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates/overview) |
| Preços | [ref](https://developers.facebook.com/documentation/business-messaging/whatsapp/pricing) |
| Changelog | [ref](https://developers.facebook.com/documentation/business-messaging/whatsapp/changelog) |
| Get started | [ref](https://developers.facebook.com/documentation/business-messaging/whatsapp/get-started) |

## O ponto que mais difere: **uma** rota contra 141

A Cloud API envia **tudo** por um só endpoint:

```
POST /{version}/{phone-number-id}/messages
{
  "messaging_product": "whatsapp",
  "recipient_type": "individual",
  "to": "<NÚMERO>",
  "type": "<TIPO>",
  "<TIPO>": { … }
}
```

O tipo vai no corpo, em `type`, e o objecto correspondente tem de existir com
o mesmo nome. O `wa-api` faz o oposto: **o tipo vai no caminho** —
`/chats/send/text`, `/chats/send/image`, `/chats/send/poll`.

Nenhuma das duas está errada, e a diferença tem consequência prática:

- **Um endpoint** dá um contrato pequeno e um corpo grande e polimórfico. Um
  cliente novo aprende uma rota; em troca, a validação do que é obrigatório
  depende do valor de `type`, e o OpenAPI descreve isso com `oneOf`.
- **Um endpoint por tipo** dá contratos pequenos e específicos. É o que
  permite a esta API dizer, por rota, exactamente que campos são obrigatórios
  — que é o que a auditoria de contrato desta série produziu.

Tipos que a Cloud API aceita em `type`, verificados na referência da Message
API: texto, imagem, áudio, documento, vídeo, autocolante, contactos,
localização, template, interactive e reaction. O `interactive.type` aceita
`button`, `list`, `product`, `product_list`, `catalog_message`,
`call_permission_request` e `flow`.

## O alvo de cobertura de mensagens

Esta é a árvore que este projecto deve alcançar. Cada folha traz o estado
**medido a 2026-08-26**, não o pretendido.

| marca | significa |
|---|---|
| ✅ | existe, com rota própria, e foi exercitada em campo |
| 🟡 | existe, mas em forma parcial ou por outro caminho — a coluna diz qual |
| 📥 | sabemos **receber** e classificar; **não** sabemos enviar |
| ❌ | não existe |

```
Messaging
├── ✅ send_text            POST /chats/send/text
├── ✅ send_image           POST /chats/send/image
├── ✅ send_audio           POST /chats/send/audio
├── 🟡 send_voice           a MESMA rota, com "ptt": true
├── ✅ send_video           POST /chats/send/video
├── ✅ send_document        POST /chats/send/document
├── ✅ send_sticker         POST /chats/send/sticker
├── ✅ send_location        POST /chats/send/location
├── 🟡 send_contacts        POST /chats/send/contact — aceita ARRAY, nome no singular
├── ✅ send_reaction        POST /chats/react
│
├── Interactive
│   ├── ✅ send_reply_buttons    POST /chats/send/buttons
│   ├── ✅ send_list             POST /chats/send/list
│   ├── ❌ send_single_product
│   ├── ❌ send_multi_product
│   ├── ❌ send_catalog
│   ├── ❌ send_flow
│   ├── 📥 send_order_details    classificamos OrderMessage à chegada
│   └── ❌ send_order_status
│
├── Templates
│   ├── ✅ send_template          POST /chats/send/template
│   ├── 🟡 send_template_buttons  o MESMO endpoint, campo `Buttons`
│   ├── ❌ send_catalog_template
│   ├── ❌ send_flow_template
│   └── 🟡 send_carousel_template POST /chats/send/carousel — é `InteractiveMessage`
│                                 com `CarouselMessage`, NÃO um template aprovado
│
└── Message Operations
    ├── ✅ reply_to_message   campo `ReplyTo` em TODAS as rotas de envio
    └── ✅ mark_read          POST /chats/markread
```

**Contagem: 12 ✅, 4 🟡, 1 📥, 6 ❌** — de 23 folhas.

### O que cada estado significa aqui, com precisão

**🟡 `send_voice`** — não falta nada: `POST /chats/send/audio` aceita `ptt`, e
o valor por omissão é **`true`** (`pkg/domain/message.go:170-187`), logo o
comportamento padrão já é nota de voz. Só não há rota separada.

**🟡 `send_contacts`** — a rota chama-se `contact` no singular mas aceita um
array. É inconsistência de NOME, não de capacidade. Registada em F269.

**🟡 `send_template_buttons`** — o `POST /chats/send/template` já leva
`Buttons` com tipos `url`, `call` e `quickreply`. Repare que os tipos aceites
diferem dos de `/chats/send/buttons` — e essa divergência está documentada
como armadilha.

**🟡 `send_carousel_template`** — e esta é a distinção mais importante da
árvore. O nosso carrossel é um `InteractiveMessage` com
`CarouselMessage_HSCROLL_CARDS` (`messenger_carousel.go:66`), construído no
momento. Um **carousel template** da Meta é outra coisa: um modelo submetido e
aprovado antes de existir conversa. **Não é o mesmo recurso com nome
diferente.**

**📥 `send_order_details`** — sabemos **receber**: `message_classify.go:146`
classifica `OrderMessage` e `ProductMessage` à chegada. Enviar é outro
trabalho, e nenhum dos dois tem construtor no nosso lado.

### Os seis ❌, e o que cada um exige

Nenhum é "uma rota a mais". Todos dependem de infraestrutura que este projecto
não tem:

| folha | o que falta |
|---|---|
| `send_single_product` · `send_multi_product` · `send_catalog` | um **catálogo** associado à conta, com produtos de ID conhecido. O `wa-noise` tem a capability `catalog`, e ela **não está ligada** a rota nenhuma — é uma das 62 da F237 |
| `send_flow` · `send_flow_template` | **WhatsApp Flows** é recurso da plataforma Meta, definido e publicado no painel dela. Atenção ao falso positivo: há 59 ocorrências de `Flow` em `pkg/`, e são **todas** `NativeFlowButton` — o mecanismo interno dos botões, sem relação com Flows |
| `send_catalog_template` | template aprovado **e** catálogo. Depende dos dois anteriores |
| `send_order_status` | não há construtor, e o fluxo de encomenda pressupõe catálogo |

### O que isto revela sobre a fronteira com a Cloud API

As seis lacunas concentram-se **exactamente** onde a Cloud API é forte:
catálogo, produtos, encomendas e Flows são recursos da plataforma comercial da
Meta, não do protocolo do WhatsApp Web.

É o simétrico das ~60 rotas que só nós temos — grupos, comunidades, canais,
status. **As duas superfícies são quase complementares**, e é isso que torna a
integração futura interessante em vez de redundante: não é escolher uma, é
usar cada uma onde ela tem alcance.

**O que fica por determinar**: se o protocolo do WhatsApp Web permite enviar
produto, catálogo e encomenda de todo. A capability `catalog` do `wa-noise`
sugere que a leitura é possível; **o envio não foi investigado**. Antes de
planear qualquer uma das seis, é essa a medição a fazer — e as três
referências do `CLAUDE.md` (Baileys, Evolution, whatsapp-web.js) são onde
procurar, porque uma resposta negativa delas também é informação.

## Comparação com o que temos — **por fazer**

Uma comparação a sério exige levantar, endpoint a endpoint, o que cada lado
faz e o que só um deles faz. Isso **não foi feito** e não está aqui.

O que se sabe hoje, sem levantamento:

| capacidade | wa-api | Cloud API |
|---|---|---|
| enviar texto e mídia | sim | sim |
| botões e listas | sim | sim (`interactive`) |
| carrossel | sim (medido) | não verificado |
| enquetes | sim | **não verificado** — a Cloud API não a lista nos tipos |
| grupos | sim (18 rotas) | **não** — a Cloud API não gere grupos |
| comunidades | sim | **não** |
| canais/newsletters | sim (18 rotas) | **não** |
| status/stories | sim | **não** |
| presença e "a escrever" | sim | parcial (typing indicators) |
| templates aprovados | **não** | sim, e obrigatórios fora das 24 h |
| Flows | **não** | sim |
| catálogo e produtos | leitura | sim |

**As linhas com "não verificado" são exactamente isso.** Preenchê-las é o
trabalho da comparação, e inventá-las agora tornaria este documento pior que
inútil.

## O que uma integração futura teria de resolver

Registado para não se redescobrir:

1. **Identidade.** O `wa-api` trabalha com JID (`@s.whatsapp.net`, `@lid`,
   `@g.us`, `@newsletter`). A Cloud API trabalha com número E.164 e IDs do
   Graph. Não há correspondência para grupo, comunidade ou canal — porque a
   Cloud API não os tem.
2. **A janela de 24 horas.** Metade das rotas de envio deste projecto não teria
   equivalente utilizável fora dela sem um template aprovado.
3. **Custo.** Enviar passa a ter preço por conversa. Qualquer rota que hoje se
   chama em laço deixa de ser gratuita.
4. **Webhooks.** Os dois lados entregam eventos, com formas diferentes. O
   `pkg/infra/wa-noise/registry/webhook` teria de ganhar um segundo formato,
   ou um adaptador.
5. **O que simplesmente não existe do outro lado.** Grupos, comunidades,
   canais e status são cerca de **60 das 141 rotas** deste projecto. Uma
   integração não os substitui — no máximo coexiste com eles.

## Nota de método

As páginas da Meta são renderizadas por JavaScript, e um `fetch` do índice
devolve só o rodapé de navegação. As tabelas acima foram montadas de páginas
individuais, que devolvem conteúdo. **Onde escrevi "não verifiquei", foi
porque não abri a página** — e não porque a informação não exista.

Ao actualizar este ficheiro, verifique o `changelog` primeiro: a Meta versiona
o Graph API e retira versões antigas com aviso.
