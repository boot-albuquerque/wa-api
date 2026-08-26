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
