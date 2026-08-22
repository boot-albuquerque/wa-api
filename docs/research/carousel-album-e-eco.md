# Carrossel: ALBUM_IMAGE não renderiza, eco do remetente incompatível

**Data**: 2026-08-22
**Autor**: investigação automatizada, fontes citadas inline
**Referência**: HOUSEKEEP F211

---

## 1. Resumo dos factos fotografados

| id | CarouselCardType | destinatário | remetente |
|---|---|---|---|
| `3EB0C101192480B874E881` | `HSCROLL_CARDS` | renderiza (2 cartões, botões) | **incompatível** |
| `3EB00FB1FB8952B80F8AA1` | `ALBUM_IMAGE` | **incompatível** | **incompatível** |

Ambas devolveram HTTP 200 com `message_id`. Mesma árvore protobuf, diferindo
apenas no enum `CarouselCardType` (campo 3 de `CarouselMessage`).

---

## 2. ALBUM_IMAGE — por que o cliente recusa

### 2.1. O que o proto define

```protobuf
// waE2E/WAWebProtobufsE2E.proto:361-372
message InteractiveMessage {
  message CarouselMessage {
    enum CarouselCardType {
      UNKNOWN = 0;
      HSCROLL_CARDS = 1;
      ALBUM_IMAGE = 2;
    }
    repeated InteractiveMessage cards = 1;
    optional int32 messageVersion = 2;
    optional CarouselCardType carouselCardType = 3;
  }
  // ...
  oneof interactiveMessage {
    ShopMessage shopStorefrontMessage = 4;
    CollectionMessage collectionMessage = 5;
    NativeFlowMessage nativeFlowMessage = 6;
    CarouselMessage carouselMessage = 7;
  }
}
```

O enum `ALBUM_IMAGE` existe no proto desde pelo menos a versão que o Baileys
upstream (`WhiskeySockets/Baileys`, branch `master`) publica. Existe também no
`wppconnect-team/wa-proto`. Não foi removido — é um tipo válido no wire.

### 2.2. O que os forks fazem (e não fazem)

**vreden/baileys** (o fork que mais implementa carousel):
- `lib/Utils/messages.js:1264-1267` — monta `carouselMessage: { cards: slides }`
  **sem** setar `messageVersion` nem `carouselCardType`.
- Portanto, o vreden envia sempre `CarouselCardType = 0` (`UNKNOWN`), que o
  cliente interpreta como `HSCROLL_CARDS` (o padrão do proto é 0 = UNKNOWN,
  mas na prática o cliente trata como horizontal scroll).

**itsliaaa/baileys** — exporta o tipo mas não tem código de envio de carousel
no README; confirma apenas que `CarouselCardType` existe como enum.

**zqdevelopers/zq_baileys_helper** — converte "cards" para `buttonsMessage`,
NÃO usa `carouselMessage` de todo. Confirma na doc: "official WhatsApp carousel
templates require business-approved templates".

**Evolution API** (v2.4.0-rc1) — adicionou `POST /message/sendCarousel` usando
`interactiveMessage + carouselMessage`. As release notes NÃO mencionam
`ALBUM_IMAGE`.

**evolution-go** (issue #59) — testou 4 variantes de carousel, todas
renderizaram. NÃO especificou qual `CarouselCardType` usou. Todos os
`/send/button` falharam no consumer (contradiz a nossa medição — ver
ARMADILHAS #26).

### 2.3. Hipóteses sobre ALBUM_IMAGE, ordenadas por probabilidade

Nenhuma destas hipóteses está CONFIRMADA — são candidatas a testar em campo.

**H1 (mais provável) — ALBUM_IMAGE proíbe NativeFlowMessage nos cartões.**
O nome "album de imagens" sugere que o tipo foi desenhado para álbuns visuais
puros (como o `albumMessage` do campo 83, que é imagens/vídeos agrupados sem
interatividade). Os nossos cartões têm `oneof interactiveMessage =
NativeFlowMessage_` com botões — pode ser que `ALBUM_IMAGE` exija que os
cartões tenham APENAS header de imagem, sem `nativeFlowMessage`.

Evidência indireta: o `AlbumMessage` (campo 83 do `Message`) é um tipo de
mensagem separado que agrupa media sem interatividade; `ALBUM_IMAGE` dentro de
`CarouselMessage` seria o equivalente dentro da superfície interativa, mas
mantendo a restrição de "só mídia".

**H2 — ALBUM_IMAGE exige messageVersion diferente.**
O nosso código usa `carouselMessageVersion = 1` (que é o mesmo de
`nativeFlowMessageVersion`). É possível que `ALBUM_IMAGE` exija `messageVersion
= 2` ou outro valor.

**H3 — ALBUM_IMAGE exige mínimo de cartões diferente.**
Enviámos 2 cartões. É possível que álbum exija >= 3 (como álbuns de foto do
WhatsApp, que agrupam a partir de 3 imagens).

**H4 — ALBUM_IMAGE simplesmente não é suportado em clientes de consumidor.**
O enum pode existir no proto para uso futuro ou para contas Business com
templates aprovados. Nenhum fork que encontrei usa `ALBUM_IMAGE`
explicitamente; todos usam `HSCROLL_CARDS` ou `UNKNOWN` (que cai em
`HSCROLL_CARDS`).

### 2.4. O que NÃO é a causa

- **Não é problema de biz nodes**: os mesmos `bizNativeFlowNodes()` são usados
  para ambos os tipos, e o `HSCROLL_CARDS` renderiza no destinatário.
- **Não é problema de upload de imagem**: as mesmas imagens funcionam com
  `HSCROLL_CARDS`.
- **Não é problema de protobuf malformado**: o servidor aceita (200 com ID).

---

## 3. Eco do remetente — por que o próprio dispositivo mostra "incompatível"

### 3.1. Mecanismo de entrega ao remetente

No protocolo multi-device do WhatsApp, o envio de uma DM funciona assim
(verificado no código de `send/encrypt.go:87-94` e
`send/node_build.go:214-239`):

1. A mensagem original (`plaintext`) vai para os dispositivos do DESTINATÁRIO.
2. Uma versão embrulhada em `DeviceSentMessage` (`dsmPlaintext`) vai para os
   OUTROS dispositivos do REMETENTE (companions).
3. O dispositivo que ENVIOU (o nosso wa-noise) é PULADO (`encrypt.go:90-91`):
   `if jid == ownJID || jid == ownLID { continue }`.

O dispositivo que enviou (wa-noise, companion device) não recebe a mensagem de
volta por este caminho. Ele já SABE o que enviou.

O telemóvel primário do WhatsApp (que é um dos "outros dispositivos do
remetente") recebe `DeviceSentMessage { message: { interactiveMessage: { ... }
} }` e precisa renderizá-la.

### 3.2. Hipótese original: viewOnce

A hipótese da F211 era que embrulhar em `viewOnceMessageV2` resolveria o eco.
**Esta hipótese está PARCIALMENTE REFUTADA** pelas seguintes evidências:

1. **Evolution API REMOVEU o wrapper viewOnceMessage** na v2.4.0-rc1:
   > "Button rendering fixed on WhatsApp Web/Desktop/iOS/Android — removed the
   > viewOnceMessage wrapper that prevented buttons from rendering."
   Fonte: release notes do `evolution-foundation/evolution-api`, v2.4.0-rc1.

2. **O vreden/baileys NÃO usa viewOnce por padrão** para interactive messages
   (`lib/Utils/messages.js:1294-1296`). O viewOnce é aplicado OPCIONALMENTE
   quando o chamador seta `message.viewOnce = true`.

3. **O zq_baileys_helper usa `documentWithCaptionMessage`**, não `viewOnce`:
   ```javascript
   // helpers/buttons.js:478-486
   function patchMessageForMdIfRequired(message) {
     const requiresPatch = !!(
       message.buttonsMessage ||
       message.listMessage ||
       message.interactiveMessage
     );
     if (requiresPatch) {
       return {
         documentWithCaptionMessage: {
           message: { ...message }
         }
       };
     }
   }
   ```
   O comentário diz: "Mirrors the legacy patch used in whaileys to ensure
   proper rendering."

### 3.3. O embrulho documentWithCaptionMessage

O nosso código JÁ usa `documentWithCaptionMessage` para listas
(`messenger_list.go:116-120`):

```go
msg := &waE2E.Message{
    DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
        Message: &waE2E.Message{ListMessage: listMsg},
    },
}
```

O nosso `SendButtons` e `SendCarousel` NÃO usam nenhum embrulho — colocam
`InteractiveMessage` direto no `Message.interactiveMessage` (campo 45).

O helper zq faz o embrulho `documentWithCaptionMessage` para TODOS os tipos
interactivos (buttons, list, interactive). Isto sugere que o embrulho pode ser
necessário para a renderização em multi-device (eco no remetente).

### 3.4. Como o wa-noise processa embrulhos

O `msgattrs.GetButtonTypeFromMessage` (em
`protocol/msgattrs/message.go:112-132`) recursivamente desembrulha
`viewOnceMessage`, `viewOnceMessageV2` e `ephemeralMessage` para encontrar o
tipo de botão — mas NÃO desembrulha `documentWithCaptionMessage`.

Contudo, para `InteractiveMessage` (nativeFlow / carousel), a função NÃO retorna
nenhum tipo de botão (não há case para `InteractiveMessage`). Os biz nodes
são injectados via `AdditionalNodes` que o nosso código passa explicitamente.
Portanto, ESTA FUNÇÃO não é relevante para o problema.

### 3.5. Hipótese revista sobre o eco

O embrulho `documentWithCaptionMessage` é o candidato mais forte para resolver
o eco no remetente, NÃO o `viewOnceMessage`. Evidências:

- O zq_baileys_helper usa `documentWithCaptionMessage` especificamente para
  melhorar a renderização multi-device.
- O nosso `SendList` já usa este embrulho e (a confirmar) as listas
  renderizam no remetente.
- A Evolution API REMOVEU viewOnce porque QUEBRAVA a renderização.

**MAS: não confirmei se o problema de eco afeta também os botões (SendButtons)
ou só o carousel.** Isto é crítico: se os botões SEM embrulho renderizam no
remetente, então o embrulho não é a causa do problema de eco no carousel, e a
causa está em outro lugar (talvez no facto de ser um carouselMessage
especificamente).

---

## 4. Aplicabilidade a nós — botões e listas renderizam sem embrulho?

### 4.1. O que sabemos

| superfície | embrulho | destinatário | remetente |
|---|---|---|---|
| SendButtons | nenhum (`Message.interactiveMessage`) | **renderiza** (fotografado) | **NÃO MEDIDO** |
| SendList | `documentWithCaptionMessage` | **renderiza** (fotografado) | **NÃO MEDIDO** |
| SendCarousel HSCROLL | nenhum (`Message.interactiveMessage`) | **renderiza** (fotografado) | **incompatível** (fotografado) |
| SendCarousel ALBUM | nenhum (`Message.interactiveMessage`) | **incompatível** (fotografado) | **incompatível** (fotografado) |

### 4.2. O que NÃO sabemos (e que a matriz de sondas deve resolver)

1. **SendButtons renderiza no remetente?** Se sim, o problema de eco é
   específico de `carouselMessage`, não de `interactiveMessage` em geral.
2. **SendList renderiza no remetente?** Se sim, o embrulho
   `documentWithCaptionMessage` pode ser a diferença.
3. **SendCarousel com embrulho `documentWithCaptionMessage` renderiza no
   remetente?** Teste direto da hipótese.
4. **SendCarousel ALBUM_IMAGE sem botões renderiza no destinatário?** Teste
   direto da H1.

---

## 5. Fontes consultadas

### Código-fonte examinado (com caminho e linha)

| ficheiro | linhas | o que foi verificado |
|---|---|---|
| `pkg/infra/wa-noise/adapters/chat/messenger_carousel.go` | 1-139 | estrutura do envio, uso de `bizNativeFlowNodes()` |
| `pkg/infra/wa-noise/adapters/chat/messenger_buttons.go` | 1-230 | envio de botões, mesmos biz nodes |
| `pkg/infra/wa-noise/adapters/chat/messenger_list.go` | 1-133 | embrulho `documentWithCaptionMessage` |
| `internal/wa-noise/protocol/proto/waE2E/WAWebProtobufsE2E.proto` | 361-452, 1710-1730 | definição de InteractiveMessage, viewOnce, documentWithCaption |
| `internal/wa-noise/protocol/msgattrs/message.go` | 112-153 | GetButtonTypeFromMessage (não reconhece InteractiveMessage) |
| `internal/wa-noise/capabilities/send/node_build.go` | 86-137, 214-239 | MessageContent (biz nodes) e MarshalMessage (DSM) |
| `internal/wa-noise/capabilities/send/encrypt.go` | 87-94 | DSM enviado a companions, próprio dispositivo pulado |

### Código-fonte de terceiros examinado

| repositório | ficheiro | o que foi verificado |
|---|---|---|
| `WhiskeySockets/Baileys` (upstream) | `WAProto/WAProto.proto` | CarouselMessage proto idêntico ao nosso |
| `WhiskeySockets/Baileys` (upstream) | `src/Utils/messages.ts` | NÃO tem código de envio de carousel |
| `WhiskeySockets/Baileys` (upstream) | `src/Socket/messages-send.ts` | NÃO injecta biz nodes para interactive |
| `vreden/baileys` (fork) | `lib/Utils/messages.js:1205-1296` | carousel SEM messageVersion, SEM carouselCardType |
| `vreden/baileys` (fork) | `lib/Socket/messages-send.js:917-980` | desembrulha viewOnce para gerar biz nodes |
| `zqdevelopers/zq_baileys_helper` | `helpers/buttons.js:462-486` | embrulho `documentWithCaptionMessage` para MD |
| `evolution-foundation/evolution-api` | release notes v2.4.0-rc1 | REMOVEU viewOnce, adicionou biz via additionalNodes |

### Issues e discussões consultadas

| fonte | conclusão |
|---|---|
| `evolution-foundation/evolution-go#59` | carousel renderiza no consumer, botões não — CONTRADIZ nossa medição (ARMADILHAS #26) |
| `WhiskeySockets/Baileys#2239` | interactive buttons em discussão |
| `WhiskeySockets/Baileys#531` | viewOnce quebra getTypeMessage |

### O que NÃO encontrei

- Nenhum fork usa explicitamente `ALBUM_IMAGE`. Todos usam `HSCROLL_CARDS` ou
  omitem o campo (que resulta em `UNKNOWN = 0`).
- Nenhuma documentação oficial do WhatsApp sobre `ALBUM_IMAGE`.
- Nenhuma issue ou commit descrevendo o eco do remetente para carousel
  especificamente.
- Não encontrei evidência de que `viewOnceMessageV2` resolva o eco — pelo
  contrário, a Evolution API diz que viewOnce QUEBRA a renderização.

---

## MATRIZ DE SONDAS A ENVIAR

Cada sonda varia UM eixo. Todas devem ser fotografadas nos DOIS telemóveis
(remetente e destinatário). A ordem é da mais informativa à menos.

### Sonda 1 — eco dos botões (baseline)

**Objectivo**: determinar se o eco é problema de `carouselMessage` ou de
`interactiveMessage` em geral.

**O que enviar**: SendButtons normal (o que já funciona no destinatário).
**O que variar**: nada — é baseline.
**O que fotografar**: o telemóvel do REMETENTE.
**O que o resultado prova**:
- Se renderiza no remetente → o eco é específico de carousel, não de
  interactive em geral. A causa está na montagem do `CarouselMessage`.
- Se NÃO renderiza → o eco afecta TODA mensagem `interactiveMessage` sem
  embrulho, e o embrulho é a solução para todas.

### Sonda 2 — eco da lista (baseline com embrulho)

**Objectivo**: confirmar que o embrulho `documentWithCaptionMessage` faz o eco
funcionar.

**O que enviar**: SendList normal (já usa `documentWithCaptionMessage`).
**O que fotografar**: telemóvel do REMETENTE.
**O que o resultado prova**:
- Se renderiza no remetente → confirma que `documentWithCaptionMessage` resolve
  o eco. A correção para carousel é aplicar o mesmo embrulho.
- Se NÃO renderiza → `documentWithCaptionMessage` não é suficiente para o eco,
  e a causa é outra.

### Sonda 3 — carousel HSCROLL_CARDS com embrulho documentWithCaptionMessage

**Objectivo**: testar se o embrulho resolve o eco para carousel.

**O que enviar**: o mesmo carousel HSCROLL_CARDS da sonda original, mas
embrulhado:
```go
msg := &waE2E.Message{
    DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
        Message: &waE2E.Message{InteractiveMessage: interactive},
    },
}
```
**O que fotografar**: AMBOS os telemóveis.
**O que o resultado prova**:
- Se renderiza em AMBOS → `documentWithCaptionMessage` é a correção para o eco
  E mantém a renderização no destinatário.
- Se renderiza no remetente mas NÃO no destinatário → o embrulho quebra o
  destinatário — é um trade-off, não uma correção.
- Se NÃO renderiza em nenhum → o embrulho não é a solução.

### Sonda 4 — carousel ALBUM_IMAGE sem botões (cartões só com imagem)

**Objectivo**: testar H1 (ALBUM_IMAGE proíbe nativeFlowMessage).

**O que enviar**: carousel com `ALBUM_IMAGE`, mesmas imagens, mas os cartões
NÃO têm `nativeFlowMessage` — só header com imagem. Cada cartão:
```go
out := &waE2E.InteractiveMessage{
    Header: header,  // com imagem
    Body:   &waE2E.InteractiveMessage_Body{Text: proto.String(card.Body)},
    // SEM InteractiveMessage oneof (nenhum nativeFlowMessage)
}
```
**O que fotografar**: AMBOS os telemóveis.
**O que o resultado prova**:
- Se renderiza no destinatário → H1 confirmada: `ALBUM_IMAGE` proíbe
  `nativeFlowMessage`. Álbum é para imagens puras.
- Se NÃO renderiza → H1 refutada. A causa é outra (H2, H3, H4).

### Sonda 5 — carousel ALBUM_IMAGE com messageVersion = 2

**Objectivo**: testar H2 (messageVersion diferente).

**O que enviar**: carousel com `ALBUM_IMAGE`, cartões com botões (como o
original), mas `messageVersion = 2` no `CarouselMessage`.
**O que fotografar**: AMBOS os telemóveis.
**O que o resultado prova**:
- Se renderiza → H2 confirmada: `ALBUM_IMAGE` exige versão diferente.
- Se NÃO renderiza → H2 refutada.

### Sonda 6 — carousel ALBUM_IMAGE com >= 3 cartões, sem botões

**Objectivo**: testar H3 (mínimo de cartões) combinado com H1.

**O que enviar**: carousel com `ALBUM_IMAGE`, 4 cartões, cada um só com header
de imagem e body (sem nativeFlowMessage), `messageVersion = 1`.
**O que fotografar**: AMBOS os telemóveis.
**O que o resultado prova**:
- Se renderiza → confirma que é a combinação de "só imagens" + "mais cartões".
- Se NÃO renderiza → `ALBUM_IMAGE` provavelmente não é suportado no consumer
  (H4).

### Sonda 7 — carousel HSCROLL_CARDS com viewOnceMessageV2

**Objectivo**: descartar definitivamente o viewOnce como solução.

**O que enviar**: carousel HSCROLL_CARDS embrulhado em `viewOnceMessageV2`:
```go
msg := &waE2E.Message{
    ViewOnceMessageV2: &waE2E.FutureProofMessage{
        Message: &waE2E.Message{InteractiveMessage: interactive},
    },
}
```
**O que fotografar**: AMBOS os telemóveis.
**O que o resultado prova**:
- Se renderiza em ambos → viewOnce funciona (contradiz Evolution API).
- Se NÃO renderiza no destinatário → confirma que viewOnce QUEBRA a
  renderização, como a Evolution API descobriu.

### Ordem de execução

```
1. Sonda 1 (SendButtons eco)      — baseline, sem código novo
2. Sonda 2 (SendList eco)         — baseline, sem código novo
3. Sonda 3 (HSCROLL + docCaption) — precisa de sonda ad-hoc
4. Sonda 4 (ALBUM sem botões)     — precisa de sonda ad-hoc
5. Sonda 7 (HSCROLL + viewOnce)   — precisa de sonda ad-hoc
6. Sonda 5 (ALBUM msgVersion=2)   — só se Sonda 4 falhar
7. Sonda 6 (ALBUM >= 3 cartões)   — só se Sonda 4 falhar
```

As sondas 1 e 2 não precisam de código novo — são os endpoints existentes. As
sondas 3, 4, 5, 6 e 7 precisam de uma rota de sonda descartável (como a que
gerou os dados da F211).

---

## Conclusões provisórias

1. **ALBUM_IMAGE**: nenhum fork que encontrei o usa. O tipo existe no proto mas
   nenhuma implementação o exerce. A hipótese mais provável é que `ALBUM_IMAGE`
   é incompatível com `nativeFlowMessage` nos cartões (H1). A segunda mais
   provável é que simplesmente não é suportado no consumer (H4). Precisa de
   sonda em campo.

2. **Eco do remetente**: a hipótese viewOnce está PARCIALMENTE REFUTADA. A
   Evolution API removeu viewOnce porque quebrava renderização. O candidato
   mais forte é `documentWithCaptionMessage`, que o zq_baileys_helper usa para
   "improve MD compatibility" e que o nosso SendList já usa. Precisa de sonda
   em campo.

3. **A hipótese de que os dois sintomas são o MESMO mecanismo está
   ENFRAQUECIDA.** O eco afecta HSCROLL_CARDS (que renderiza no destinatário),
   enquanto ALBUM_IMAGE não renderiza em NENHUM lado. Se a Sonda 1 mostrar que
   botões renderizam no remetente, são mecanismos DIFERENTES:
   - ALBUM_IMAGE é um tipo de carrossel não suportado (ou com restrições de
     conteúdo).
   - Eco é um problema de embrulho da mensagem para multi-device.
