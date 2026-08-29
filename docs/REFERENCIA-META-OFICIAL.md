# Referência oficial — WhatsApp Business Platform (Meta)

**Levantado a 2026-08-26**, das páginas oficiais da Meta. Cada linha desta
página tem URL verificado; o que **não** foi verificado está dito como tal.

## Por que este documento existe

O `wa-api` **não** usa a API oficial da Meta. Ele fala o protocolo do WhatsApp
Web directamente, através do fork em `internal/noise`. São duas coisas
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
├── ✅ send_native_carousel POST /chats/send/carousel — NÃO é template (ver nota)
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
│   └── ❌ send_carousel_template  ver a nota abaixo: são DUAS capacidades
│
└── Message Operations
    ├── ✅ reply_to_message   campo `ReplyTo` em TODAS as rotas de envio
    └── ✅ mark_read          POST /chats/markread
```

**Contagem: 13 ✅, 3 🟡, 1 📥, 7 ❌** — de 24 folhas.

A árvore ganhou uma folha e perdeu uma ambiguidade: `send_carousel_template`
era 🟡 por partilhar a palavra "carousel" com o que temos. São **duas
capacidades distintas**, e separá-las mostra que temos uma e não a outra.

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

**✅ `send_native_carousel` e ❌ `send_carousel_template`** — a distinção mais
importante da árvore, e ela obrigou a **dividir uma folha em duas**.

O nosso carrossel é um `InteractiveMessage` com `CarouselMessage_HSCROLL_CARDS`
(`messenger_carousel.go:66`), montado no momento do envio. Um **carousel
template** da Meta é um modelo submetido e **aprovado antes de existir
conversa**.

Uma pesquisa da colecção oficial no Postman **não encontrou** um pedido
independente de "Send Carousel Message", ao contrário do que existe para
botões, lista, produto e Flow — o carrossel aparece como *message template com
múltiplos cards*. Ou seja: o que temos pode ser capacidade do protocolo
WhatsApp Web **sem equivalente directo** na Cloud API.

Enquanto o contrato oficial não for confirmado, ficam como capacidades
separadas. Marcá-las como uma só, em qualquer sentido, seria igualar nomes em
vez de capacidades.

**📥 `send_order_details`** — sabemos **receber**: `message_classify.go:146`
classifica `OrderMessage` e `ProductMessage` à chegada. Enviar é outro
trabalho, e nenhum dos dois tem construtor no nosso lado.

### Os seis ❌, e o que cada um exige

Nenhum é "uma rota a mais". Todos dependem de infraestrutura que este projecto
não tem:

| folha | o que falta |
|---|---|
| `send_single_product` · `send_multi_product` · `send_catalog` | um **catálogo** associado à conta, com produtos de ID conhecido. O `noise` tem a capability `catalog`, e ela **não está ligada** a rota nenhuma — é uma das 62 da F237 |
| `send_flow` · `send_flow_template` | **WhatsApp Flows** é recurso da plataforma Meta, definido e publicado no painel dela. Atenção ao falso positivo: há 59 ocorrências de `Flow` em `pkg/`, e são **todas** `NativeFlowButton` — o mecanismo interno dos botões, sem relação com Flows |
| `send_catalog_template` | template aprovado **e** catálogo. Depende dos dois anteriores |
| `send_order_status` | não há construtor, e o fluxo de encomenda pressupõe catálogo |

## Como a Cloud API modela isto — e o que valida o nosso desenho

Levantamento da colecção oficial da Meta no Postman (2026-08-26). **Onde não
consegui cruzar com `developers.facebook.com`, está dito**: a página de
referência da Message API é renderizada por JavaScript e não expõe os schemas.

### O endpoint é transporte, não capacidade

Quase tudo passa por um só:

```
POST /{version}/{phone_number_id}/messages
```

O que muda é `type` no corpo e, nas interativas, `interactive.type`:

| capacidade | `type` | `interactive.type` |
|---|---|---|
| texto, imagem, áudio, vídeo, documento, sticker, localização, contactos, reação | o próprio nome | — |
| template | `template` | — |
| botões de resposta | `interactive` | `button` |
| lista | `interactive` | `list` |
| produto único | `interactive` | `product` |
| multi-produto | `interactive` | `product_list` |
| catálogo | `interactive` | `catalog_message` |
| Flow | `interactive` | `flow` |
| cobrança | `interactive` | `order_details` |
| estado da encomenda | `interactive` | `order_status` |

**A consequência para o nosso registo de capacidades**: `POST /messages`
existir **não significa** que as capacidades de mensagem estejam
implementadas. Uma tabela que contasse endpoints daria 1 do lado da Meta e 16
do nosso, e não diria nada.

### Três coisas que ela modela melhor do que eu tinha escrito

**1. "Botões" não é uma capacidade — são duas.** `interactive.button` (até 3
botões de resposta, título ≤ 20 caracteres, `id` devolvido no webhook) é
diferente de **botões de template**, que são criados em
`POST /{waba_id}/message_templates` e passam por aprovação, com subtipos
`QUICK_REPLY`, `URL`, `PHONE_NUMBER`, `OTP`, `CATALOG` e `FLOW`.

Isto valida a divergência que já tínhamos medido e documentado como armadilha:
os tipos aceites em `/chats/send/buttons` (`reply`, `cta_url`, `cta_call`,
`copy`) diferem dos de `/chats/send/template` (`quickreply`, `url`, `call`).
**Não era inconsistência nossa — é a diferença entre dois recursos.**

**2. "Template" também são duas**: gerir (`POST`/`GET`/`DELETE`
`/{waba_id}/message_templates`) e enviar (`type: "template"` em `/messages`).
Nós não temos nem uma nem outra — o nosso `/chats/send/template` monta um
`TemplateMessage` do protocolo, sem aprovação prévia.

**3. Pagamentos são regionais.** A colecção traz exemplos específicos de
**Singapura e Índia**. Não é capacidade global, e tratá-la como tal no registo
seria prometer o que não existe no Brasil.

### E duas que confirmam o desenho que já tínhamos

**Resposta não é um tipo de mensagem.** A Meta acrescenta
`context.message_id` ao tipo normal, em vez de ter `reply_text`,
`reply_image`, … É exactamente o nosso `ReplyTo`, presente em **todas** as
rotas de envio. Dois desenhos independentes chegaram à mesma forma.

**O `200` não é entrega.** A resposta devolve um `wamid`; os estados reais —
`sent`, `delivered`, `read`, `failed` — chegam por **webhook**. É o mesmo que
esta série mediu à força: o nosso `200` com `message_id` vale 🟡, e só a
observação independente promove a ✅.

Isso sugere uma melhoria concreta ao nosso modelo de evidência: hoje a
promoção de 🟡 para ✅ é feita por mim, a olhar para o cliente. **Com o
webhook de `delivered` ela poderia ser automática** — e essa é a diferença
entre evidência que se recolhe e evidência que se recebe.

## O nosso carrossel NÃO é `multi_product` nem `catalog_message` — medido no proto

A pergunta era se o `/chats/send/carousel` é equivalente a alguma das
capacidades comerciais da Cloud API. **Não é**, e o protocolo diz-o sem
ambiguidade: são **variantes diferentes do mesmo `oneof`**.

`InteractiveMessage` tem quatro variantes mutuamente exclusivas
(`waE2E`, campo `interactiveMessage`):

| campo | struct | campos que a definem | o que é |
|---|---|---|---|
| 4 `shopStorefrontMessage` | `ShopMessage` | `ID`, `Surface` — `FB`, `IG` ou `WA` | abre a **montra** da loja numa superfície |
| 5 `collectionMessage` | `CollectionMessage` | `BizJID`, `ID` | referencia uma **colecção do catálogo** do negócio |
| 6 `nativeFlowMessage` | `NativeFlowMessage` | `NativeFlowButton[]` | os **botões** — é o que `/chats/send/buttons` usa |
| 7 `carouselMessage` | `CarouselMessage` | `Cards []*InteractiveMessage`, `CarouselCardType` | **cartões deslizáveis**, cada um uma InteractiveMessage completa |

**O que decide a questão** é o tipo de `Cards`: `[]*InteractiveMessage`. Um
cartão do nosso carrossel é uma mensagem interativa inteira — com o seu
cabeçalho, corpo, rodapé e botões próprios. **Não é uma referência a produto.**

As duas capacidades comerciais referenciam catálogo por identificador:
`CollectionMessage` tem `BizJID` + `ID`, `ShopMessage` tem `ID` + `Surface`.
Nenhuma transporta conteúdo — apontam para dados que vivem no catálogo do
negócio.

### O mapeamento, então, é este

| Cloud API | protocolo WhatsApp Web | temos? |
|---|---|---|
| `interactive.product_list` (multi-produto) | `CollectionMessage` | ❌ existe no proto, **não construímos** |
| `interactive.catalog_message` | `ShopMessage` | ❌ existe no proto, **não construímos** |
| `interactive.product` (produto único) | `Header_ProductMessage` (cabeçalho de card) | ❌ |
| `interactive.button` | `NativeFlowMessage` | ✅ `/chats/send/buttons` |
| `interactive.list` | `NativeFlowMessage` com `single_select` | ✅ `/chats/send/list` |
| **sem equivalente conhecido** | `CarouselMessage` | ✅ `/chats/send/carousel` |

**O nosso carrossel continua sem par do lado da Cloud API.** A pesquisa da
colecção Postman não encontrou pedido próprio para ele, e o proto mostra
porquê: é um contentor genérico de cartões interativos, não um recurso
comercial. O *carousel template* da Meta resolve um problema parecido por
outro caminho — modelo aprovado, com cards fixos.

### O detalhe que muda o planeamento

`CarouselMessage.Cards` são `InteractiveMessage`, e `InteractiveMessage.Header`
aceita `Header_ProductMessage`. **Em teoria, um cartão do nosso carrossel pode
ter um produto no cabeçalho** — o que aproximaria o carrossel de um
multi-produto.

O nosso construtor **não faz isso**: `buildCarouselCard`
(`messenger_carousel.go:106-124`) só monta `Header_ImageMessage` a partir de
bytes carregados. Se a montra de produtos vier a ser precisa, este é o ponto
onde ela encaixa — e é mais barato do que implementar `CollectionMessage` de
raiz.

**Não medido, e é o que decide se vale a pena**: se o servidor do WhatsApp
aceita `Header_ProductMessage` vindo de um cliente Web, e se o produto tem de
existir num catálogo aprovado. O proto declarar um campo **não** significa que
o servidor o aceite — esta série já viu isso três vezes.

### Um segundo tipo de carrossel que temos e não usamos

`CarouselCardType` tem **dois** valores válidos: `HSCROLL_CARDS` (1), que
usamos, e `ALBUM_IMAGE` (2), que **não**. O segundo sugere um álbum de imagens
em vez de cartões com botões — e há memória neste repositório de uma tentativa
de álbum (`boot-albuquerque/wa-album-eco`).

Fica registado como candidato: é uma linha de código no construtor, e a
pergunta é se o cliente o desenha diferente.

### O que a pesquisa revelou como lacuna nossa, e eu não tinha visto

A Meta aceita mídia por **URL** e por **`media_id`** (upload prévio). Eu
supus que só aceitávamos base64 — **errado, e medido**: `send_image.go:254`
aceita URL `http(s)`, com protecção contra SSRF, além do URI de dados.

Falta-nos o terceiro: **não há upload prévio com ID reutilizável**. Enviar a
mesma imagem a cem destinatários envia-a cem vezes. Não é defeito — é uma
capacidade que a Cloud API tem e nós não, e fica registada como tal.

**Nuance por confirmar**: a colecção mostra `audio.voice: true` para nota de
voz. Nós usamos `ptt`, com padrão `true`. Parecem o mesmo conceito com nomes
diferentes, **mas não cruzei os dois** — e igualar por semelhança de
significado é o erro que a nota do carrossel acabou de evitar.

### O que isto revela sobre a fronteira com a Cloud API

As seis lacunas concentram-se **exactamente** onde a Cloud API é forte:
catálogo, produtos, encomendas e Flows são recursos da plataforma comercial da
Meta, não do protocolo do WhatsApp Web.

É o simétrico das ~60 rotas que só nós temos — grupos, comunidades, canais,
status. **As duas superfícies são quase complementares**, e é isso que torna a
integração futura interessante em vez de redundante: não é escolher uma, é
usar cada uma onde ela tem alcance.

**O que fica por determinar**: se o protocolo do WhatsApp Web permite enviar
produto, catálogo e encomenda de todo. A capability `catalog` do `noise`
sugere que a leitura é possível; **o envio não foi investigado**. Antes de
planear qualquer uma das seis, é essa a medição a fazer — e as três
referências do `CLAUDE.md` (Baileys, Evolution, whatsapp-web.js) são onde
procurar, porque uma resposta negativa delas também é informação.

## Matriz de capacidades por motor

O projecto tem **dois** motores — `noise` (protocolo WhatsApp Web, o que
serve hoje) e `headless` (dirige a SPA) — e a Cloud API seria um terceiro.
A coluna certa para cada capacidade não é a mesma.

`noise` medido nesta série. `headless` **não medido** — está fora do
âmbito por instrução, e um `?` é mais honesto que uma suposição.

| capacidade | noise | headless | meta_cloud |
|---|---|---|---|
| `send_text` | ✅ | ? | ✅ |
| `send_image` · `send_video` · `send_document` | ✅ | ? | ✅ |
| `send_audio` | ✅ | ? | ✅ |
| `send_voice` | 🟡 `ptt`, padrão `true` | ? | 🟡 `audio.voice` — equivalência **não cruzada** |
| `send_sticker` | ✅ | ? | ✅ |
| `send_location` | ✅ | ? | ✅ |
| `send_contacts` | 🟡 rota no singular, aceita array | ? | ✅ `contacts` |
| `send_reaction` | ✅ | ? | ✅ (falha se a original tiver >30 dias) |
| `send_reply_buttons` | ✅ | ? | ✅ `interactive.button`, máx. 3 |
| `send_list` | ✅ | ? | ✅ `interactive.list`, máx. 10 linhas |
| `send_native_carousel` | ✅ `CarouselMessage`/`HSCROLL_CARDS` | ? | **sem equivalente** — medido no proto |
| `send_single_product` | ❌ `Header_ProductMessage` existe no proto, não construído | ? | ✅ `interactive.product` |
| `send_multi_product` | ❌ `CollectionMessage` existe no proto, não construído | ? | ✅ `product_list`, máx. 30 |
| `send_catalog` | ❌ `ShopMessage` existe no proto, não construído | ? | ✅ `catalog_message` |
| `send_flow` | ❌ | ? | ✅ `interactive.flow` |
| `send_order_details` | 📥 só recebe | ? | 🟡 **regional** (SG, IN) |
| `send_order_status` | ❌ | ? | 🟡 **regional** |
| `send_template` | 🟡 sem aprovação prévia | ? | ✅ com aprovação |
| `send_template_buttons` | 🟡 tipos diferentes de `/buttons` | ? | ✅ 6 subtipos |
| `send_catalog_template` · `send_flow_template` · `send_carousel_template` | ❌ | ? | ✅ |
| **gestão** de templates | ❌ | ? | ✅ `/{waba_id}/message_templates` |
| **gestão** de Flows | ❌ | ? | ✅ `/{waba_id}/flows` |
| upload de mídia com ID reutilizável | ❌ | ? | ✅ |
| `reply_to_message` | ✅ `ReplyTo` | ? | ✅ `context.message_id` |
| `mark_read` | ✅ rota própria | ? | ✅ mesmo endpoint, outro método |
| **grupos** (18 rotas) | ✅ | ? | ❌ |
| **comunidades** (4) | ✅ | ? | ❌ |
| **canais** (18) | ✅ | ? | ❌ |
| **status** (3) | 🟡 F256 | ? | ❌ |
| **enquetes** | ✅ | ? | ❌ não consta dos tipos |

### O que a matriz mostra que a árvore sozinha escondia

**A linha divisória não é "quem tem mais".** É clara e tem nome: tudo o que
depende do **Commerce Manager** e do **painel da Meta** — catálogo, produtos,
encomendas, Flows, templates aprovados — está do lado deles. Tudo o que
depende do **protocolo social** — grupos, comunidades, canais, status,
enquetes — está do nosso.

Não é acidente: são as duas metades do que o WhatsApp é. Uma API de plataforma
comercial não expõe grupos; um cliente de protocolo não tem catálogo aprovado.

**A consequência para o planeamento**: as 7 lacunas da árvore **não se
resolvem escrevendo rotas**. Resolvem-se por integração, ou não se resolvem.
Confundir as duas coisas faria alguém abrir uma tarefa "implementar
send_catalog" que não tem como terminar.

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
   `pkg/infra/noise/registry/webhook` teria de ganhar um segundo formato,
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
