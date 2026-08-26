# Contrato arquitetural do wa-api

**O que este documento é**: as decisões globais que toda rota tem de respeitar,
e o registo honesto de onde a API de hoje ainda não as respeita.

**O que ele não é**: uma descrição do que gostaríamos que a API fosse. Cada
regra abaixo diz o **estado medido** e, quando diverge do alvo, diz que a
divergência existe em vez de a esconder. Uma norma que descreve um sistema que
não existe é pior que nenhuma: leva o consumidor a escrever código contra ela.

Medições de 2026-08-26, contra o binário do `HEAD`, com duas sessões reais.

---

## 0. A ordem de confiança

Quando duas fontes discordarem, esta é a ordem:

```
comportamento real observado  >  implementação  >  validadores  >  testes  >  OpenAPI
```

Se o OpenAPI discorda do resto, **investiga-se**. Não se corrige o OpenAPI para
parecer certo. Esta sessão já apanhou dez divergências assim (F267), e todas
eram do documento, não do código.

---

## 1. Nomes de recurso — singular e plural

**Alvo**: coleções no plural, singleton no singular.

**Estado medido**, por primeiro segmento de caminho:

| segmento | rotas | forma | é coleção? |
|---|---:|---|---|
| `/chat` | 33 | singular | sim — devia ser `/chats` |
| `/group` | 18 | singular | sim — devia ser `/groups` |
| `/newsletter` | 18 | singular | sim — devia ser `/newsletters` |
| `/user` | 16 | singular | sim — devia ser `/users` |
| `/session` | 14 | singular | **não** — é a sessão do token; singleton legítimo |
| `/community` | 4 | singular | sim — devia ser `/communities` |
| `/message` | 1 | singular | sim — devia ser `/messages` |
| `/labels` | 2 | **plural** | sim — correcto |
| `/admin/users` | 3 | **plural** | sim — correcto |
| `/health`, `/livez` | 4 | singular | singleton legítimo |
| `/s3`, `/hmac`, `/webhook`, `/proxy` | 8 | singular | singleton da sessão — legítimo |
| `/status` | 3 | singular | coleção de publicações; ambíguo |
| `/call` | 1 | singular | operação, não recurso |

**Violações reais**: 6 famílias (`chat`, `group`, `newsletter`, `user`,
`community`, `message`) usam singular para coleção, e duas (`labels`,
`admin/users`) usam plural. **A API é inconsistente consigo mesma**, e isso é
medido, não opinião.

**Decisão**: o alvo canónico fica registado, e a migração **não é feita agora**.
Renomear 90 rotas é uma mudança de contrato que parte todo cliente existente.
O caminho está desenhado na secção 20 (versionamento) e o achado registado como
**F269** no `HOUSEKEEP.md`.

**Para rotas NOVAS, a regra vale desde já**: plural para coleção.

### 1.1 Singleton — quando o singular é correcto

Singular só quando existe **um** do recurso naquele contexto:

```
/health          o estado deste processo
/livez           a vivacidade deste processo
/session/status  a sessão deste token — o token JÁ identifica qual
/webhook         o webhook desta sessão
/s3/config       a configuração de armazenamento desta sessão
```

`/session/*` é o caso mais defensável de toda a API: o token no cabeçalho
**é** o identificador da sessão, logo `/sessions/{id}/status` teria o
identificador duas vezes, e as duas cópias poderiam discordar.

### 1.2 Sub-recursos concatenados

**Medido**, caminhos que transportam relação como palavra colada:

| hoje | relação que esconde |
|---|---|
| `POST /group/requestparticipants` | `GET /groups/{id}/join-requests` |
| `POST /group/updaterequestparticipants` | `POST /groups/{id}/join-requests/{action}` |
| `POST /group/joinapprovalmode` | `PATCH /groups/{id}/settings` |
| `POST /group/updateparticipants` | `POST /groups/{id}/participants` |
| `POST /chat/send/text` | `POST /chats/{id}/messages` |
| `POST /newsletter/admin-invite` | `POST /newsletters/{id}/admin-invites` |
| `POST /community/link` | `PUT /communities/{id}/subgroups/{group_id}` |

**Sete famílias**, e o padrão é o mesmo: o identificador do recurso-pai vai no
CORPO em vez do caminho, e o verbo vira substantivo. Registado em **F269**.

---

## 2. Glossário oficial

Vinculante para tudo o que for novo. O que já existe fica, e as divergências
estão na coluna da direita.

| conceito | nome oficial | divergências vivas |
|---|---|---|
| utilizador/sessão | `user` / `users` | — |
| conversa | `chat` / `chats` | `Phone` é usado como destino em 15 rotas |
| grupo | `group` / `groups` | `Group`, `groupJID`, `GroupJID`, `groupjid` — **quatro grafias** |
| comunidade | `community` / `communities` | `communityJID` |
| canal | `newsletter` / `newsletters` | `jid` |
| mensagem | `message` / `messages` | `Id`, `MessageID`, `message_id`, `StanzaId` |
| participante | `participant` / `participants` | `Phone` (array) |
| pedido de entrada | `join_request` / `join-requests` | `requestparticipants` |
| identificador de mensagem | `message_id` | `Id`, `MessageID`, `PollMessageId` |
| JID de grupo | `group_jid` | `groupJID`, `GroupJID`, `groupjid`, `Group` |
| JID de canal | `newsletter_jid` | `jid` |
| URL de proxy | `proxy_url` | — |
| destino universal | `chat` | aceite em TODA rota com destinatário (F225) |

**Convenção de caixa**: `snake_case` para tudo o que for novo.

**Estado medido**: a API mistura `snake_case` (`message_id`, `proxy_url`),
`camelCase` (`groupJID`, `inviteLink`, `mediaDelivery`) e `PascalCase`
(`Phone`, `Body`, `Action`, `Sections`). **As três convivem, por vezes no mesmo
corpo.** Não se migra agora, pela mesma razão da secção 1.

**O que salva o consumidor hoje**: o `encoding/json` do Go casa nomes
**ignorando maiúsculas**. `groupjid`, `groupJID` e `GroupJID` são o mesmo campo.
Isto **não** vale entre palavras diferentes (`code` ≠ `inviteLink`), que é onde
os erros reais acontecem.

---

## 3. Semântica de presença — o vocabulário obrigatório

Todo campo, em pedido e em resposta, tem de responder a **cinco** perguntas.
Não são sinónimos:

| estado | JSON | pergunta |
|---|---|---|
| **ausente** | `{}` | o campo pode ser omitido? |
| **nulo** | `{"name": null}` | aceita `null` explícito? |
| **vazio** | `{"name": ""}` | aceita cadeia vazia? |
| **só espaços** | `{"name": "   "}` | aceita apenas brancos? |
| **preenchido** | `{"name": "Equipa comercial"}` | o caso normal |

**`undefined` não existe em JSON.** Onde um SDK JavaScript envia `undefined`, a
serialização **omite** a propriedade — documente-se como *ausente*, nunca como
`undefined`.

### 3.1 Como isto se escreve no OpenAPI

```yaml
name:
  type: string
  description: |
    Nome do grupo.

    | estado | aceite? | efeito |
    |---|---|---|
    | ausente | não | `400 missing_name` |
    | `null` | não | `400 missing_name` — **indistinguível de ausente** |
    | `""` | não | `400 missing_name` — **indistinguível de ausente** |
    | `"   "` | **sim** | aceite tal e qual; **não há `trim`** |
    | preenchido | sim | — |
  minLength: 1
  maxLength: 100
  nullable: false
  example: Equipa comercial
```

A tabela é obrigatória sempre que qualquer linha dela for surpreendente. Para
um campo trivialmente obrigatório, a frase basta.

### 3.2 A regra do `trim`

**Medido**: o wa-api **não faz `trim`** em campo nenhum. A validação típica é
`if req.Campo == ""`, logo `"   "` passa. Onde isso importar, a descrição
tem de dizer — e dizer que é o comportamento actual, não o desejável.

---

## 4. `null` versus ausência, e o buraco do Go

**Medido, e é a armadilha central desta API.**

Em Go, um campo `string` que não venha no JSON fica `""`. Um campo `*string`
que não venha fica `nil`. **A API usa quase só tipos não-ponteiro**, logo:

```
campo ausente  ==  campo com ""  ==  campo com null (quando o tipo aceita)
```

são **indistinguíveis** na maioria das rotas. Isso é aceitável em rotas de
criação, onde ausente e vazio significam ambos "não foi dado". **Não é
aceitável em actualização parcial** — ver secção 6.

### 4.1 O que foi medido, e desmentiu a minha primeira leitura

Eu tinha escrito que `null` num campo `string` daria
`400 could_not_decode_payload`. **Não dá.** O `encoding/json` do Go trata
`null` para um tipo não-ponteiro como **nada a fazer**: o campo fica com o
valor zero, exactamente como se não tivesse vindo.

Medido, `POST /chat/send/text`, quatro pedidos no mesmo minuto:

```
{"Phone":"…"}                  -> 400 missing_body
{"Phone":"…","Body":null}      -> 400 missing_body     <- MESMO resultado
{"Phone":"…","Body":""}        -> 400 missing_body     <- MESMO resultado
{"Phone":"…","Body":"   "}     -> 200, mensagem ENVIADA
{"Phone":"…","Body":123}       -> 400 could_not_decode_payload
```

**Três dos cinco estados colapsam num só.** Ausente, `null` e `""` são
literalmente o mesmo pedido para o servidor. Só o tipo errado é distinguido.

E o quarto é a surpresa: **`"   "` é enviado**. Uma mensagem de três espaços
chega ao destinatário.

### 4.2 O caso perigoso — `null` num booleano

```
POST /chat/mute {"jid":"…","mute":null}  ->  200 "Chat unmuted"
```

`null` num `bool` não-ponteiro vira **`false`**, em silêncio, e a operação faz o
CONTRÁRIO do que um leitor distraído esperaria de "não decidi". É o mesmo
mecanismo da F268, noutro tipo.

**Regra que daqui sai**: um `bool` cujo `false` seja uma acção — e não um
padrão inerte — **tem de ser ponteiro**, para que ausente e `false` se
distingam.

### 4.3 Arrays não são validados por omissão

```
POST /user/check {"phone":null}   ->  200 {"data":null}
POST /user/check {"phone":[]}     ->  200 {"data":null}
POST /user/check {"phone":[""]}   ->  200 {"data":null}
```

As três formas de "lista vazia" devolvem **sucesso com `data: null`**. A rota
não exige `minItems`, e responde `200` a um pedido que não pede nada.
`data: null` não é `data: []` — quem iterar sobre a resposta sem verificar
parte. Registado em **F270**.

**Onde a API usa ponteiro**, e portanto distingue:

| campo | tipo | porque o ponteiro importa |
|---|---|---|
| `mute_duration` | `*time.Duration` | `0` significa **para sempre**; ausente também. Ver F268 |
| `Latitude` / `Longitude` | `*float64` | `0` é uma coordenada válida (Golfo da Guiné) |
| `ptt` | `*bool` | `false` é "não é nota de voz"; ausente é "decide tu" |
| `ForwardingScore` | `*uint32` | `0` é "não encaminhada"; ausente usa o padrão `1` |
| `webhookUseProxy` | `*bool` | `false` é explícito; ausente herda o gravado |

**Sempre que o valor zero do tipo tiver significado próprio, o campo TEM de ser
ponteiro.** Não sê-lo é o defeito da F268: `duration` mal escrito → `nil` →
`0` → "para sempre", com `200`.

---

## 5. Campos desconhecidos

**Medido**:

```
POST /user/check {"phone":["554192421234"],"campo_que_nao_existe":"lixo"}
  -> 200, o campo é IGNORADO em silêncio
```

Vale para **toda** a API: `domain.DecodeRequest` usa `encoding/json` sem
`DisallowUnknownFields`.

**Consequência real, já medida**: um erro de digitação num nome de campo não dá
erro — dá o valor zero. Quando o valor zero é válido e significa outra coisa, o
erro vira uma escolha silenciosa (F268).

**Decisão**: o alvo é **recusar** campo desconhecido com `400`. Não é aplicado
agora — passaria a `400` pedidos que hoje funcionam, o que é mudança de
contrato. Registado em **F268**, ponto 2.

**Enquanto não for**: `additionalProperties: false` **não** é escrito nos
esquemas, porque seria falso. Cada esquema de pedido diz na descrição que
campos desconhecidos são ignorados.

---

## 6. Actualização parcial

**Medido**: a API **não tem `PATCH`**. Tem `PUT /webhook` e
`PUT /admin/users/{id}`, e ambos são substituição, não fusão.

Para qualquer rota de actualização parcial que venha a existir, a semântica é
obrigatoriamente esta, e escrita na descrição:

```
campo ausente  -> preserva o valor actual
campo = null   -> remove o valor
campo = ""     -> define vazio (NÃO é o mesmo que remover)
campo = false  -> define false
campo = 0      -> define zero
```

E o DTO **tem de usar ponteiros**, senão a distinção não existe no servidor.

---

## 7. Enums

**Medido**: valor fora do enum é **recusado com `400` e código próprio**.

```
POST /user/contacts/sync {"mode":"valor_invalido"}
  -> 400 invalid_sync_mode
     "mode must be one of: if_unsynced, incremental, full"
```

**Isto é o oposto do que acontece com campo desconhecido** — e a diferença é
deliberada: o campo existe, logo o valor é validado.

Todo enum documentado tem de trazer: todos os valores, sensibilidade a
maiúsculas, valor por omissão, e o que acontece com valor desconhecido.

---

## 8. Números

Para cada número: mínimo, máximo, se zero é permitido **e o que significa**, se
negativo é permitido, unidade, e valor por omissão.

**A regra que esta API aprendeu à força**: nunca deixar ambíguo o que `0`
significa. Em `mute_duration`, `0` é **para sempre** — não é "desligado" nem
"ausente" nem "sem limite". Onde `0` tiver significado, a descrição diz qual.

**Unidades medidas**, e são inconsistentes entre si:

| campo | unidade |
|---|---|
| `mute_duration` | **nanossegundos** (inteiro) |
| `duration` (efémeras) | **texto** de duração (`"24h"`) |
| `retention_days` | dias |
| `count` | número de mensagens |
| `Seconds` (áudio) | segundos |

Duas formas para a mesma ideia, em rotas vizinhas. Registado em **F269**.

---

## 9. Booleanos

`false` **nunca** pode ser tratado como ausência. Onde a distinção importar, o
tipo é `*bool` — ver secção 4.

---

## 10. Arrays

Para cada array: pode ser omitido, pode ser `null`, pode ser `[]`, mínimo,
máximo, duplicados, ordem significativa, e **o que acontece a um item
inválido**.

**A regra que esta API aprendeu**: descarte silencioso de item tem de estar na
descrição. Em `POST /chat/send/buttons`, um botão sem `title` é **descartado
sem erro**; se todos caírem, aí sim é `400 no_valid_buttons`. Quem envia três
botões e recebe dois não é avisado.

---

## 11. Objectos

Para cada objecto: `{}` é válido? propriedades obrigatórias? campos
desconhecidos (secção 5)? `null` é aceite? há fusão?

---

## 12. Esquemas de pedido e de resposta são separados

Não se reutiliza o mesmo esquema para leitura e escrita quando a semântica
difere. Um campo que só volta (`id`, `created_at`, `status`) é `readOnly`; um
que só entra (`token`, `secret_key`) é `writeOnly`.

**Medido**: `POST /admin/users` devolve o `token` uma vez, na criação, e
`GET /admin/users` devolve `token: ""` sempre. **O campo existe nos dois e
significa coisas diferentes** — está documentado assim.

---

## 13. Campos sensíveis

`token`, `secret_key`, `access_key`, `hmac_key`, `Authorization` são segredos.

- `writeOnly: true` onde só entram.
- **Nunca** um valor real nos exemplos. Travado por
  `TestOpenAPINaoTrazSegredoReal` — a página `/docs` é servida **sem
  autenticação**, logo um segredo no exemplo é um segredo publicado.
- Nunca em log. `extractRequestToken` já o garante: regista quem mandou e para
  onde, nunca o valor.

---

## 14. Contrato de erro

**Medido**: existem **duas** formas em produção.

**A forma canónica**, para tudo o que passa pela taxonomia `apperr`:

```json
{"code": 400, "error": {"code": "missing_chat", "message": "missing chat in payload"}, "success": false}
```

**A forma antiga**, em 14 pontos do código (F266):

```json
{"code": 400, "error": "bad request", "success": false}
```

`error` é uma **string**. Onze desses pontos vêm de `rejectMissingField`, logo
**toda** recusa de campo obrigatório das rotas de grupo tem este formato.

**Decisão**: a forma canónica é a única para código novo. A antiga está
documentada rota a rota com `ErroTextoSimples`, para que ninguém escreva
`error.code` e parta. Correcção registada em **F266**.

### 14.1 O que nunca sai no corpo de erro

**Medido e travado por teste** (`TestRespondJSONNaoVazaDetalheDeErroInterno`):
um erro que não seja `*apperr.AppError` **nunca** tem o seu texto serializado.
`RespondJSON` cai em `genericErrorMessage`, que devolve só o texto do status.

Consequência: **não há stack trace, caminho de ficheiro, consulta SQL nem
segredo em resposta nenhuma** — em ambiente nenhum. O `panic` completo vai para
o log, que é onde serve.

**Não há bloco `debug` na resposta, e não foi acrescentado.** Acrescentá-lo
significaria criar um caminho de fuga que hoje não existe, dependente de uma
variável de ambiente para não disparar. A propriedade actual — *nunca vaza,
independentemente de configuração* — é mais forte que *vaza só quando a
configuração diz que pode*. Registado como decisão consciente.

### 14.2 Erro esperado versus inesperado

| classe | exemplos | corpo |
|---|---|---|
| esperado | `missing_chat`, `unauthorized`, `invalid_action`, `upstream_rejected` | `error.code` + `error.message` |
| inesperado | `panic`, falha de banco, `nil pointer` | `error` genérico do status, e nada mais |

### 14.3 Erro por campo

**Medido**: a API **não** tem `error.details[]`. Cada recusa devolve **um**
código, o do primeiro campo que falhou. Um pedido com três campos em falta
revela um de cada vez.

É limitação real e está registada em **F269**. Documentada em vez de fingida.

---

## 15. Rastreio

**Medido**: `hlog.RequestIDHandler("req_id", "Request-Id")`.

- Cabeçalho de resposta: **`Request-Id`** — presente, incluindo em erro.
- Nos logs: campo `req_id`.
- **No corpo do erro: ausente.**

O consumidor consegue correlacionar pelo cabeçalho. Pôr o mesmo valor no corpo
seria mais cómodo e é o alvo; registado em **F269**.

Não é `X-Request-Id`. O prefixo `X-` está desaconselhado desde a RFC 6648, e
renomear um cabeçalho que os clientes já leem é mudança de contrato.

---

## 16. Observabilidade

**Medido**: todo pedido produz uma linha estruturada com `req_id`, `method`,
`route` (padrão, não o caminho preenchido), `status`, `duration_ms`, `outcome`,
`size`, `user_agent`, `ip` e `userid`.

Segredos nunca são registados.

---

## 17. Tempos limite

**Medido**: `waclient.RequestTimeout` = **30 segundos** por operação contra o
WhatsApp. É o que produz o `500` de `POST /newsletter/updates` (F265) — o
servidor nunca responde e o prazo esgota.

Nenhuma chamada externa fica sem prazo.

---

## 18. Repetições e idempotência

**Medido**: **não há** `Idempotency-Key` em rota nenhuma.

Chamar `POST /chat/send/text` duas vezes envia **duas** mensagens. Toda rota de
envio é assim, e a documentação diz.

O campo `Id` (cliente) existe em todas as rotas de envio e é o identificador
que o cliente escolhe, mas **não é** chave de idempotência: não foi medido que
o servidor o use para deduplicar. Registado em **F269** como candidato.

**Repetição**: só de rotas de leitura. Uma rota de envio que devolva `500` pode
ter enviado — repetir cegamente duplica.

---

## 19. Paginação

**Medido**, sem paginação nenhuma:

| rota | devolveu | com |
|---|---:|---|
| `GET /user/contacts` | **61 459 bytes** | 1266 contactos |
| `GET /chat/list` | 7 144 bytes | — |
| `POST /group/list` | — | todos os grupos |
| `GET /newsletter/list` | 1 654 bytes | — |

`GET /chat/history` é a **única** com limite (`limit`), e mesmo essa não tem
cursor.

`GET /user/contacts` cresce com a agenda do utilizador e não tem tecto. É o
candidato mais claro a paginação. Alvo, para quando existir:

```json
{"data": [], "pagination": {"next_cursor": "…", "has_more": true}}
```

Cursor, e não deslocamento: a colecção muda debaixo do leitor. Registado em
**F269**.

---

## 20. Versionamento e depreciação

**Medido**: **não há** prefixo de versão. As rotas são servidas na raiz.

Isto é o que torna a secção 1 impossível de aplicar hoje: sem `/v1`, renomear
`/user` para `/users` parte todo cliente no momento do deploy.

**Alvo**: introduzir `/v1` como sinónimo das rotas actuais, sem as remover;
publicar `/v2` com os nomes canónicos; marcar `/v1` `deprecated: true` com data.
Registado em **F269**.

**Aliases já existentes**, medidos, e todos com o par novo/antigo servido pelo
mesmo manipulador:

| antigo | novo | nota |
|---|---|---|
| `POST /s3/config` | `POST /s3/configure` | F251 |
| `POST /hmac/config` | `POST /hmac/configure` | F251 |
| `POST /webhook/history` | `POST /session/history` | F252 — o nome antigo **engana** |
| `/s3/*`, `/hmac/*` | `/session/s3/*`, `/session/hmac/*` | — |
| `/livez` | `/health/live` | mantido: é o que os manifestos apontam |

Nenhum está marcado `deprecated` no OpenAPI, porque nenhum foi **decidido**
como depreciado — marcar sem decisão seria anunciar remoção que ninguém
planeou.

---

## 21. Limitação de ritmo

**Medido**: **não há** rate limiting na fronteira HTTP, e nenhuma rota devolve
`429`.

Há controlo de saída (`golang.org/x/time/rate` no envio para o WhatsApp), que
protege a conta, não o servidor.

Não se inventa um limite arbitrário. Registado em **F269** como decisão a tomar
com o dono do produto, porque o número certo depende do perfil de uso.

---

## 22. Códigos de estado

**Medido**, o que a API realmente usa:

```
101  1     200 140    400 131    401 138    403   4
404 11     409   8    422  31    500 136    501   1
```

**Nunca usados**: `201`, `202`, `204`, `410`, `412`, `415`, `429`, `502`,
`503`, `504`.

Em particular:

- **`POST /group/create` e `POST /newsletter/create` devolvem `200`, não
  `201`** — criam recurso e não devolvem `Location`.
- **`POST /admin/users` devolve `200`, não `201`.**
- Nenhuma operação assíncrona devolve `202`, e há pelo menos uma que o
  justificaria (`POST /user/history/sync`, que pede ao par e responde antes de
  o histórico chegar).

**Decisão**: não se muda status de rota existente. Mudar `200` para `201`
parte todo cliente que compare `=== 200`. Alvo registado em **F269**; para
rotas novas, o código semanticamente correcto desde o início.

---

## 23. Datas e horas

**Medido**, e há **três** formatos em uso:

| campo | forma | exemplo |
|---|---|---|
| `timestamp` (envio) | inteiro Unix, segundos | `1787747900` |
| `timestamp` (saúde) | RFC 3339 em UTC | `2026-08-26T13:17:14Z` |
| `creation_time` (canal) | **texto** com um inteiro | `"1787746245"` |

O terceiro é o pior: um número embrulhado em aspas, vindo do próprio WhatsApp.
Documentado como é. Registado em **F269**.

Para campo novo: RFC 3339 em UTC, `format: date-time`.

---

## 24. Identificadores

| tipo | forma | exemplo |
|---|---|---|
| id de sessão | 32 hex | `918e37366f27e1125ee0482a793267e1` |
| id de mensagem | hex maiúsculo, prefixo `3EB0` | `3EB0A4B2AFD45E625C0917` |
| JID de contacto (PN) | `<número>@s.whatsapp.net` | `554192421234@s.whatsapp.net` |
| JID de contacto (LID) | `<opaco>@lid` | `90937376170214@lid` |
| JID de grupo | `<número>@g.us` | `120363411669320145@g.us` |
| JID de canal | `<número>@newsletter` | `120363411025775186@newsletter` |
| convite de canal | 24 alfanuméricos | `0029Vb8NIrODZ4LhzncjL82W` |
| convite de grupo | 22 alfanuméricos | `IVccRoDVSbpHKNkgNxyZx4` |

**PN e LID não são intermutáveis.** Onde a rota exigir uma das formas, a
descrição diz qual — e a resolução PN→LID que a API faz é **direccional**
(F228/F233c): converter LID→PN parte o pedido.

---

## 25. Tipos de conteúdo

**Medido**: toda a API é `application/json`, na entrada e na saída. **Não há
`multipart/form-data`** — a mídia entra em **base64**, dentro do JSON, como URI
de dados (`data:image/png;base64,…`) ou base64 cru.

Logo **não** se usa `format: binary` em lado nenhum: seria falso. O campo é
`type: string` com a forma documentada.

**A excepção medida**: `POST /group/photo` aceita **só** base64 cru, sem
prefixo `data:` — divergente de `/chat/send/image`, que aceita o URI. Registado
em **F267**.

**`415 Unsupported Media Type` não é devolvido.** Um `Content-Type` errado com
JSON válido no corpo passa.

---

## 26. Segurança da própria página

`/docs` é servida **sem autenticação**, deliberadamente: não carrega credencial
nem dado, e exigir token para ler como obter um token seria circular.

Consequências assumidas:

- **`Try it out` está ligado.** Quem abrir a página pode chamar a API — mas só
  com um token que já tenha. Sem `Authorize`, toda chamada dá `401`.
- **Nenhum segredo nos exemplos**, travado por teste.
- **Antes de expor a página na Internet**, a decisão a tomar é se as rotas
  `/admin` devem aparecer: elas governam a criação de sessões. Registado em
  **F269**.

---

## 27. Como isto é verificado

| propriedade | gate |
|---|---|
| toda rota registada está na spec | `TestOpenAPICobreTodasAsRotasRegistadas` |
| a spec não inventa rotas | `TestOpenAPINaoInventaRotas` |
| tag, summary, descrição e responses | `TestOpenAPIOperacoesEstaoCompletas` |
| marca de evidência em todo summary | `TestOpenAPISummariesTrazemMarcaDeEvidencia` |
| nenhuma credencial real na spec | `TestOpenAPINaoTrazSegredoReal` |
| o documento embutido está em dia | `TestOpenAPIGeradoEstaAtualizado` |
| erro interno nunca vaza no corpo | `TestRespondJSONNaoVazaDetalheDeErroInterno` |
| a taxonomia não é apagada pela guarda acima | `TestRespondJSONPreservaOErroDaTaxonomia` |

Cada um destes teve **controlo negativo executado** — reintroduzi o defeito e
confirmei que o teste falha com mensagem útil.
