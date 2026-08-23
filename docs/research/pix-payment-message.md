# Pesquisa: botão de pagamento PIX via WhatsApp

Data: 2026-08-22
Escopo: descobrir o esquema exato do `buttonParamsJSON` para pagamento PIX

---

## 1. Existem DUAS vias completamente distintas

A investigação revelou que "PIX no WhatsApp" NÃO é uma coisa só. São duas
vias com protocolos, pré-requisitos e capacidades diferentes:

### Via A — Cloud API (`order_details` + `review_and_pay`)

É a via OFICIAL da Meta. Usa a WhatsApp Cloud API (HTTP REST), não o
protocolo web/multidevice.

- **Tipo de mensagem**: `interactive`, subtipo `order_details`
- **Nome do botão na action**: `review_and_pay`
- **Requer**: WABA (WhatsApp Business Account) brasileiro, catálogo de
  produtos Meta, PSP (Payment Service Provider) integrado, e a Payments API
  habilitada pela Meta (closed beta / acesso seletivo)
- **Renderiza**: cartão de pedido nativo com itens, valores, e botão "Revisar
  e Pagar" que abre código PIX / link de pagamento / boleto

### Via B — Web API (`NativeFlowButton` com `payment_info`)

É a via NÃO-oficial, usada por forks do Baileys e por projetos como o nosso
que falam o protocolo web/multidevice direto. Usa o mesmo mecanismo de
`NativeFlowMessage` + `NativeFlowButton` que já usamos para `quick_reply`,
`cta_url`, `cta_call`, `cta_copy`.

- **Nome do botão**: `payment_info`
- **Requer**: a mensagem ser montada corretamente no protobuf + stanza BIZ.
  Em princípio NÃO exige WABA — usa o mesmo canal que nossos botões atuais.
- **Renderiza**: botão de pagamento PIX nativo no app do destinatário, com
  dados do recebedor (nome do comerciante, chave PIX)

---

## 2. Via A — Cloud API: esquema completo do `order_details`

**Proveniência**: documentação oficial da Meta (`developers.facebook.com
/documentation/business-messaging/whatsapp/payments/payments-br/`),
confirmada por documentação de BSPs (CM.com, Sinch, 360dialog).

### JSON da mensagem interativa (envio direto, não template)

```json
{
  "messaging_product": "whatsapp",
  "recipient_type": "individual",
  "to": "<PHONE_NUMBER>",
  "type": "interactive",
  "interactive": {
    "type": "order_details",
    "header": {
      "type": "image",
      "image": { "link": "https://example.com/image.jpg" }
    },
    "body": { "text": "Descrição do pedido" },
    "footer": { "text": "Texto opcional de rodapé" },
    "action": {
      "name": "review_and_pay",
      "parameters": {
        "reference_id": "<UNIQUE_REF>",
        "type": "digital-goods",
        "payment_type": "br",
        "payment_settings": [
          {
            "type": "pix_dynamic_code",
            "pix_dynamic_code": {
              "code": "<PIX_QRCODE_STRING>",
              "merchant_name": "<NOME_DO_TITULAR>",
              "key": "<CHAVE_PIX>",
              "key_type": "CNPJ"
            }
          }
        ],
        "currency": "BRL",
        "total_amount": {
          "value": 50000,
          "offset": 100
        },
        "order": {
          "status": "pending",
          "items": [
            {
              "retailer_id": "SKU001",
              "name": "Produto",
              "amount": { "value": 50000, "offset": 100 },
              "quantity": 1
            }
          ],
          "subtotal": { "value": 50000, "offset": 100 },
          "tax": { "value": 0, "offset": 100, "description": "" },
          "shipping": { "value": 0, "offset": 100 },
          "discount": { "value": 0, "offset": 100 }
        }
      }
    }
  }
}
```

### Campos do `parameters` (via A)

| Campo | Tipo | Obrigatório | Descrição | Proveniência |
|---|---|---|---|---|
| `reference_id` | string | SIM | ID único do pedido, max 60 chars, alfanumérico + `_-." | Meta docs |
| `type` | string | SIM | `"digital-goods"` ou `"physical-goods"` | Meta docs |
| `payment_type` | string | SIM | `"br"` (Brasil) | Meta docs |
| `payment_settings` | array | NÃO | Array de métodos de pagamento (até 3) | Meta docs |
| `currency` | string | SIM | `"BRL"` | Meta docs |
| `total_amount` | object | SIM | `{value: int, offset: 100}`. Valor 50000 com offset 100 = R$500,00 | Meta docs |
| `order` | object | NÃO | Itens detalhados do pedido | Meta docs |

### Tipos de `payment_settings` (via A)

| Tipo | Campos | Proveniência |
|---|---|---|
| `pix_dynamic_code` | `code` (string QR), `merchant_name`, `key`, `key_type` (CNPJ confirmado; outros inferidos) | Meta docs |
| `payment_link` | `uri` (URL do link de pagamento) | Meta docs |
| `boleto` | `digitable_line` (linha digitável) | Meta docs |
| `offsite_card_pay` | `last_four_digits`, `credential_id` | CM.com docs |

### Pré-requisitos da via A

1. **WABA brasileiro** (WhatsApp Business Account) — **obrigatório**
2. **Catálogo de produtos Meta** vinculado ao WABA — **obrigatório**
3. **PSP integrado** — o negócio gera o código PIX e reconcilia pagamento
4. **Payments API habilitada** — feature em closed beta / acesso seletivo
5. **Número do cliente brasileiro** — Meta rejeita se não for BR

**Proveniência**: Meta docs + CM.com docs + 360dialog docs

---

## 3. Via B — Web API: esquema do `buttonParamsJSON` para `payment_info`

**Proveniência**: fork Itsukichann/Baileys (README.md, seção "Buttons
Interactive Message PIX"), confirmado em forks yemo-dev/baileys,
innovatorssoft/Baileys, e Luna-botv6/luna-baileys. Também referenciado na
issue #1050 do EvolutionAPI/evolution-api e no commit 27633aa da Evolution
API v2.3.7.

### buttonParamsJSON para `payment_info`

```json
{
  "payment_settings": [
    {
      "type": "pix_static_code",
      "pix_static_code": {
        "merchant_name": "Nome do Recebedor",
        "key": "email@example.com",
        "key_type": "EMAIL"
      }
    }
  ]
}
```

### Campos do `payment_settings[0].pix_static_code`

| Campo | Tipo | Obrigatório | Valores conhecidos | Proveniência |
|---|---|---|---|---|
| `merchant_name` | string | SIM (inferido) | Nome livre do recebedor | Baileys forks (código) |
| `key` | string | SIM (inferido) | Chave PIX do recebedor | Baileys forks (código) |
| `key_type` | string | SIM (inferido) | `"PHONE"`, `"EMAIL"`, `"CPF"`, `"EVP"` | Baileys forks (código) |

**ATENÇÃO**: "obrigatório" acima é INFERIDO dos exemplos dos forks, NÃO de
documentação oficial. Não existe documentação oficial da Meta para esta via.

### Como se integra no protobuf (nosso código)

O botão `payment_info` usa EXATAMENTE a mesma estrutura protobuf que nossos
botões atuais:

```
InteractiveMessage
  └─ NativeFlowMessage
       ├─ Buttons[]
       │    ├─ Name:             "payment_info"
       │    └─ ButtonParamsJSON:  (string JSON acima)
       └─ MessageVersion:       1
```

E precisa do MESMO nó BIZ no stanza:

```
biz
  └─ interactive (type="native_flow", v="1")
       └─ native_flow (v="9", name="mixed")
```

Isto é exatamente o que `messenger_buttons.go` já faz para os quatro tipos
existentes. Acrescentar `payment_info` seria adicionar um quinto `case` no
switch de `nativeFlowButtons` e uma constante `nativeFlowNamePaymentInfo`.

### Diferença entre `pix_static_code` e `pix_dynamic_code`

| Aspecto | `pix_static_code` | `pix_dynamic_code` |
|---|---|---|
| Via | Web API (NativeFlowButton) | Cloud API (order_details) |
| Contém valor? | NÃO — só identifica o recebedor | SIM — inclui `code` (QR) com valor embutido |
| Confirmação de pagamento? | NÃO — o app só mostra dados para copiar | SIM — webhook de status do pedido |
| Requer WABA? | NÃO (inferido) | SIM |

---

## 4. Via B (complemento): `review_and_pay` via Web API

Os mesmos forks documentam TAMBÉM um botão `review_and_pay` via
NativeFlowButton (Web API, não Cloud API). O JSON é mais rico:

```json
{
  "currency": "IDR",
  "payment_configuration": "",
  "payment_type": "",
  "total_amount": { "value": "999999999", "offset": "100" },
  "reference_id": "45XXXXX",
  "type": "physical-goods",
  "payment_method": "confirm",
  "payment_status": "captured",
  "payment_timestamp": 1234567890,
  "order": {
    "status": "completed",
    "description": "",
    "subtotal": { "value": "0", "offset": "100" },
    "order_type": "PAYMENT_REQUEST",
    "items": [{
      "retailer_id": "your_retailer_id",
      "name": "Product Name",
      "amount": { "value": "999999999", "offset": "100" },
      "quantity": "1"
    }]
  },
  "additional_note": "Thank you",
  "native_payment_methods": [],
  "share_payment_status": false
}
```

**Proveniência**: Baileys forks (Itsukichann, yemo-dev, innovatorssoft).

**NOTA IMPORTANTE**: este exemplo usa `"IDR"` (rupia indonésia) e
`payment_configuration: ""` vazio. NÃO há exemplo confirmado com `"BRL"` e
PIX nesta via. O campo `payment_configuration` é um ID emitido pela Meta
quando se configura um gateway de pagamento no Business Manager — sem ele,
é provável que o servidor ignore ou rejeite. Esta via parece destinada a
mercados onde WhatsApp Pay está ativo (Índia, Singapura), NÃO ao Brasil.

---

## 5. Terceira via: `wa_payment_transaction_details`

Documentado nos mesmos forks:

```json
{
  "name": "wa_payment_transaction_details",
  "buttonParamsJson": {
    "transaction_id": "12345848"
  }
}
```

**Proveniência**: Baileys forks.

Este botão mostra o HISTÓRICO de uma transação já realizada. Não inicia
pagamento. Irrelevante para o nosso caso.

---

## 6. O que o servidor faz com JSON inválido

Não encontrei documentação oficial sobre o comportamento de erro para
`payment_info` via Web API. Evidências indiretas:

- O projeto do 99freelas (`#731423` e `#732202`) relata que mensagens
  chegam com HTTP 200 e recibo de entrega, mas **NÃO renderizam** no app
  do destinatário quando o `messageVersion` está ausente ou o
  `messageContextInfo` conflita. Isso sugere que o servidor ACEITA o
  protobuf mas o cliente NÃO desenha o botão.

- Para a Cloud API (via A), a Meta documenta que payloads inválidos
  retornam erro na API HTTP. Os códigos não estão na documentação pública
  que encontrei.

- O projeto do 99freelas foi **concluído** (freelancer Otávio Q., fev/2026),
  o que sugere que o problema de rendering foi resolvido — mas não há
  detalhes públicos de como.

---

## 7. Exige conta business?

### Via A (Cloud API + order_details)
**SIM, obrigatoriamente.** Requer WABA brasileiro com catálogo Meta e
Payments API habilitada. Fonte: documentação oficial da Meta.

### Via B (Web API + payment_info)
**NÃO HÁ EVIDÊNCIA DE EXIGÊNCIA**, mas também NÃO há confirmação de que
funciona com conta pessoal. Os forks do Baileys não distinguem — usam o
mesmo `sendMessage` de contas pessoais e business. O projeto do 99freelas
não menciona tipo de conta.

A Evolution API, no commit 27633aa, trata `payment_info` como mensagem
RECEBIDA (parsing no Chatwoot), não como mensagem enviada — o que sugere
que o uso real é predominantemente por contas business que enviam via
Cloud API, e a Evolution só RECEBE e exibe.

**Risco**: mesmo que o protobuf seja aceito, o servidor pode validar se a
conta remetente tem permissão para enviar botões de pagamento. Sem teste
real, isto é desconhecido.

---

## 8. Precisa de chave PIX registada, merchant onboarding, ou ID da Meta?

### Via A (Cloud API)
SIM para tudo:
- **Chave PIX**: o negócio precisa gerar o código PIX dinâmico via PSP
- **Merchant onboarding**: WABA + catálogo + Payments API (closed beta)
- **ID da Meta**: implícito no WABA e no `payment_configuration`

### Via B (Web API + payment_info com pix_static_code)
**NÃO** — o `pix_static_code` leva a chave PIX diretamente no JSON. Não
há `payment_configuration` nem referência a ID da Meta. O app do
destinatário apenas EXIBE os dados do recebedor para que o usuário copie
e pague manualmente no banco. Não há circuito de confirmação.

---

## VEREDICTO

1. **Dá para enviar PIX das nossas contas de teste HOJE?**
   **Não sei com certeza.** A via B (`payment_info` + `pix_static_code`) usa
   exatamente o mesmo mecanismo protobuf dos nossos botões atuais e não exige
   WABA, mas nenhum dos projetos consultados confirma publicamente que funciona
   com conta pessoal — e o projeto do 99freelas relatou problemas de rendering
   que podem ou não ter sido resolvidos. A via A (Cloud API) está DESCARTADA
   porque exige WABA com Payments API.

2. **Se sim, qual é o JSON?**
   O candidato mais provável para teste é:
   ```json
   {
     "name": "payment_info",
     "buttonParamsJSON": "{\"payment_settings\":[{\"type\":\"pix_static_code\",\"pix_static_code\":{\"merchant_name\":\"Nome Teste\",\"key\":\"email@teste.com\",\"key_type\":\"EMAIL\"}}]}"
   }
   ```
   Montado como `NativeFlowButton` com `MessageVersion: 1`, dentro de
   `InteractiveMessage.NativeFlowMessage`, acompanhado do nó BIZ — EXATAMENTE
   como `SendButtons` em `messenger_buttons.go` já faz para os outros tipos.
   **Este JSON é de fork de terceiros e NÃO foi validado por nós.**

3. **O que falta?**
   - **Teste real**: enviar o botão `payment_info` de uma conta de teste e
     verificar se (a) o servidor aceita, (b) o botão renderiza no app do
     destinatário, (c) a interação funciona. Este é o ÚNICO caminho para
     saber se funciona — nenhuma documentação responde.
   - **Decisão de produto**: o `pix_static_code` NÃO processa pagamento —
     apenas mostra dados do recebedor. O usuário precisa abrir o app do
     banco e pagar manualmente. Se o requisito é pagamento com confirmação
     automática, só a via A (Cloud API + WABA) serve, e isso está fora do
     nosso alcance hoje.
