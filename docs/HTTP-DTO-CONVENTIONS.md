# Convenções de DTO da fronteira HTTP

Este documento é a **norma** que as seis migrações por família (sessão,
mensagens, grupos, utilizadores, canais, admin) seguem. Ele não descreve um
plano: descreve o padrão que já está implementado uma vez, com nomes de
ficheiro reais que se podem abrir e copiar.

A implementação de referência é a rota `POST /group/info`. Sempre que este
documento disser "como no exemplo", o exemplo é:

| papel | ficheiro |
|---|---|
| tipo de domínio | `pkg/domain/group_info.go` |
| porta tipada | `pkg/application/contracts/group_ports.go` |
| normalização protocolo→domínio (motor wa-noise) | `pkg/infra/wa-noise/adapters/group/map_group_info.go` |
| normalização protocolo→domínio (motor headless) | `pkg/infra/wa-headless/groupdir/directory.go` |
| DTO de resposta | `pkg/presentation/http/dto/group/group_info.go` |
| apresentador | `pkg/presentation/http/dto/group/presenter.go` |
| manipulador ligado | `pkg/presentation/http/handlers/handler_group.go` |
| teste de contrato por rota | `pkg/presentation/http/handlers/handler_group_info_contract_test.go` |
| helper partilhado de asserção | `pkg/presentation/http/contracttest/naming.go` |
| gate arquitectural | `pkg/presentation/http/handlers/respondjson_ledger_test.go` |

---

## 1. O problema, em uma frase

As structs de `pkg/domain` eram descodificadas e codificadas **directamente**
como corpos HTTP. Isso tinha duas consequências, e só uma delas é de estética:

1. **Nomes errados no fio.** As etiquetas eram Go-idiomáticas (`"GroupJID"`,
   `"Phone"`, `"loggedIn"`, `"QRCode"`) — ou não existiam de todo, caso em que
   o `encoding/json` emite o nome do campo Go tal e qual.
2. **O modelo de domínio ERA o modelo público.** Renomear um campo interno
   partia clientes em silêncio, e nenhum teste o via. Pior: onde a porta
   devolvia `any`, era o MOTOR da sessão que decidia a forma do JSON — o
   `POST /group/info` servia a struct do wa-noise numa sessão e a struct de
   conversa do headless noutra, e nenhuma das duas estava declarada em lado
   nenhum.

A correcção da (1) sem a (2) seria trocar as etiquetas e ficar exactamente com
o mesmo acoplamento.

## 2. O fluxo

```
pedido HTTP
   ↓
DTO de PEDIDO        valida, normaliza, distingue omitido/null/""/false/0/[]/{}
   ↓
entrada de domínio   (comando do caso de uso)
   ↓
caso de uso  →  resultado de domínio
   ↓
APRESENTADOR         mapeamento explícito, campo a campo, escrito à mão
   ↓
DTO de RESPOSTA      etiquetas `json` em snake_case minúsculo
   ↓
resposta HTTP
```

`pkg/domain` continua Go-idiomático por dentro — **PascalCase, sem etiquetas
`json`**. Já não é o formato de fio.

## 3. Onde os DTO vivem

```
pkg/presentation/http/dto/<família>/
    <recurso>.go     os tipos de fio
    presenter.go     os apresentadores (domínio → DTO)
    request.go       os DTO de pedido (DTO → domínio), quando a família os tiver
```

Famílias: `session`, `message`, `group`, `user`, `newsletter`, `admin`.

**Porquê `dto/<família>` e não um único pacote `dto`.** Um pacote só, com ~260
campos, obrigaria a prefixar todo tipo com a família para evitar colisão
(`GroupInfoResponse` vs `NewsletterInfoResponse` são fáceis; `InfoResponse`
seis vezes não é). Com um pacote por família, o prefixo é o nome do pacote, o
que é o que o Go já faz de graça. E dá a cada worker um directório só seu, o
que importa quando seis migrações correm em paralelo.

**Porquê `dto` e não `contract`.** O nome curto aparece em toda importação
qualificada (`dtogroup.PresentGroupInfo`) e o gate arquitectural classifica
pelo prefixo `dto` da expressão. Ver §8.

**Regra de importação, e ela é de sentido único:** o pacote `dto` importa
`pkg/domain`. `pkg/domain`, `pkg/application` e `pkg/infra` **nunca** importam
`dto`. Um tipo de DTO que apareça numa assinatura de caso de uso é o acoplamento
a voltar, ao contrário.

**DTO só na fronteira PÚBLICA.** HTTP, WebSocket, webhook. Uma struct interna
que nunca atravessa um fio não precisa de DTO nenhum, e criar um só custa uma
camada a manter.

## 4. Nomes de tipo

| papel | forma | exemplo |
|---|---|---|
| tipo de domínio | substantivo nu | `domain.GroupInfo`, `domain.GroupParticipant` |
| entrada de caso de uso | `…Input` ou `…Request` de domínio | `domain.GetGroupInfoRequest` |
| resultado de caso de uso | `…Result` | `domain.GetGroupInfoResult` |
| **DTO de resposta** | `…Response` | `dtogroup.GroupInfoResponse` |
| **DTO de pedido** | `…Request` | `dtogroup.SendMessageRequest` |
| **envelope de `data` de uma rota** | `<Operação>Response` | `dtogroup.GetGroupInfoResponse` |

Não há sobreposição ambígua porque o pacote qualifica: `domain.GroupInfo` e
`dtogroup.GroupInfoResponse` nunca se confundem numa linha de código.

**Nunca** um tipo de DTO chamado igual ao de domínio, nem sequer em pacotes
diferentes: `dtogroup.GroupInfo` ao lado de `domain.GroupInfo` é a colisão que
uma revisão não apanha.

## 5. Apresentadores (domínio → DTO)

Uma função exportada **por tipo**, escrita à mão, campo a campo:

```go
func PresentGroupParticipant(p domain.GroupParticipant) GroupParticipantResponse
func PresentGroupInfo(g *domain.GroupInfo) *GroupInfoResponse
func PresentGetGroupInfo(r *domain.GetGroupInfoResult) GetGroupInfoResponse
```

Regras:

- **Nada de reflexão, nada de copiadores genéricos, nada de ida-e-volta por
  `json`.** A propriedade que se está a comprar é um **ERRO DE COMPILAÇÃO**:
  renomeie um campo em `domain.GroupInfo` e `presenter.go` deixa de compilar.
  Um mapeador por reflexão continuaria a compilar e apagaria a chave de todas
  as respostas em silêncio — que é exactamente a falha que esta camada existe
  para impedir.
- **Entrada `nil` apresenta como `nil`.** O chamador não ramifica antes de
  chamar.
- **Colecção vazia é `[]`, nunca `null`.** `make([]T, 0, len(src))` antes do
  laço. Só uma das duas formas se percorre sem verificação.
- **Tempo zero é `null`, nunca `"0001-01-01T00:00:00Z"`.** Essa string é uma
  data real no fio, e um cliente que a analise recebe um grupo criado antes do
  calendário gregoriano em vez de um campo desconhecido. Use o helper
  `presentTime(time.Time) *string`, que devolve RFC 3339 em UTC ou `nil`.
- O apresentador vive **no mesmo pacote** do DTO, em `presenter.go`.

## 6. DTO de pedido (DTO → domínio)

Forma:

```go
// SendTextRequest é o corpo de POST /chats/send/text.
type SendTextRequest struct {
    Phone   string  `json:"phone"`
    Body    string  `json:"body"`
    // Ponteiro: é preciso distinguir "não mandei" de "mandei false".
    Preview *bool   `json:"link_preview"`
}

// Validate recusa o pedido que não pode produzir um comando de domínio.
// Devolve *apperr.AppError para que a fronteira responda 400 com um
// error.code estável, em vez de um 500 genérico.
func (r SendTextRequest) Validate() error

// ToDomain produz a entrada do caso de uso. Só é chamado depois de Validate.
func (r SendTextRequest) ToDomain() domain.SendTextInput
```

### Omitido vs `null` vs zero

Esta é a parte que se erra em silêncio. O `encoding/json` do Go, num campo de
VALOR, trata `ausente`, `null`, `""`, `false` e `0` como **o mesmo estado**: o
zero do tipo. Se a rota precisar de distinguir, o campo tem de ser **ponteiro**
(ou um tipo com `UnmarshalJSON` próprio).

| precisa distinguir? | tipo do campo | como ler |
|---|---|---|
| não — zero é um valor legítimo e "não mandei" também significa zero | `string`, `bool`, `int` | directo |
| sim — "não mandei" ≠ "mandei o zero" | `*string`, `*bool`, `*int` | `nil` = omitido/`null`; apontado = enviado |
| sim, e `null` ≠ omitido | ponteiro + `UnmarshalJSON` que registe a presença da chave | caso raro; justifique em comentário |

**Não use `omitempty` num DTO de RESPOSTA.** Ele faz `""`, `false`, `0` e `[]`
desaparecerem, e um cliente deixa de conseguir distinguir "o grupo não tem
descrição" de "esta chave não vem nesta versão". Emitir sempre é mais barato
que documentar a ausência.

`omitempty` num DTO de PEDIDO é irrelevante (o Go não o lê na descodificação) —
não o escreva, para não sugerir uma semântica que não existe.

### Normalização

O que a rota normalizar tem de o fazer **no DTO de pedido**, não no caso de
uso e não no manipulador. Hoje este repositório **não faz `TrimSpace` em lado
nenhum** (ver `ARMADILHAS.md`), o que significa que `"   "` passa a guarda
`== ""` e morre mais à frente com outro código de erro. Se uma família decidir
normalizar, isso é uma **mudança de comportamento** e vai na sua própria
entrada de `HOUSEKEEP.md`, com o código de erro antes e depois.

## 7. Envelope e taxonomia de erro — a especificação autoritativa

Produzido por `RespondJSON` em `pkg/presentation/http/response.go`. É
**canónico e permanente**, não uma janela de depreciação.

**Sucesso**

```json
{"success": true, "code": 200, "data": { ... }}
```

**Erro**

```json
{"success": false, "code": 400, "error": {"code": "invalid_request", "message": "Requisição inválida."}}
```

Invariantes, e nenhuma delas tem excepção:

1. **`error` é SEMPRE um objecto**, em todo estado HTTP (400/401/403/404/409/
   422/429/500/501/502/503/504/…), para erro tipado e não tipado. Nunca uma
   string. Um cliente pode ler `error.code` sem verificar o tipo. (Havia uma
   segunda forma, com `error` em texto; era a **F266** e foi removida.)
2. **`error.code` é `snake_case` e estável.** É por ele que um cliente
   ramifica; a mensagem pode mudar.
3. **`error.message` é pt-BR, legível, e seguro por construção.** Nunca
   `err.Error()`.
4. **Erro tipado manda no status.** Quando o erro é um `*apperr.AppError`
   (mesmo embrulhado por `%w`), o `statusCode` passado pelo manipulador é
   IGNORADO e o real vem de `err.Category.HTTPStatus()`.
5. **Erro não tipado nunca vaza.** O ramo genérico descarta o texto do erro e
   devolve o par código+mensagem do estado. Travado por
   `TestRespondJSONNaoVazaDetalheDeErroInterno` e por
   `TestRespondJSON_ErrorIsNeverAString`.

### Códigos genéricos por estado

Para o erro que não passou pela taxonomia `apperr`:

| estado | `error.code` | `error.message` |
|---|---|---|
| 400 | `invalid_request` | Requisição inválida. |
| 401 | `unauthorized` | Credenciais ausentes ou inválidas. |
| 403 | `forbidden` | Operação não permitida. |
| 404 | `not_found` | Recurso não encontrado. |
| 405 | `method_not_allowed` | Método HTTP não permitido para este recurso. |
| 409 | `conflict` | A requisição não pode ser atendida no estado atual. |
| 422 | `unprocessable_entity` | A requisição foi recusada pelo destino. |
| 429 | `rate_limited` | Limite de requisições excedido. |
| 501 | `not_implemented` | Recurso não disponível nesta configuração. |
| 502 | `bad_gateway` | Falha ao contactar um serviço externo. |
| 503 | `service_unavailable` | Serviço temporariamente indisponível. |
| 504 | `gateway_timeout` | Tempo esgotado ao contactar um serviço externo. |
| 500 e qualquer outro | `internal_error` | Ocorreu um erro interno. |

O 405 entrou na tabela quando o router deixou de responder `text/plain` a um
método errado. Sem ele, o ramo `default` teria devolvido `internal_error` num
405 — "nós partimos" para um pedido cujo único defeito é o verbo.

**Prefira um código PRÓPRIO ao genérico.** O genérico é a rede de segurança,
não o destino: construa `apperr.New("missing_group_jid", apperr.CategoryValidation, …)`
e o cliente sabe o que corrigir. `invalid_request` só diz "algo estava mal".

### Onde o código nasce, e como se escreve

O código é `snake_case`, pela MESMA regra das chaves (§8), porque é um valor
enumerado que o servidor escreve. Isto é verificado, não confiado:
`TestErrorCodesAreCanonicalSnakeCase`
(`pkg/presentation/http/handlers/error_code_contract_test.go`) percorre `pkg/`
inteiro por AST e recusa qualquer literal de código — em `apperr.New` ou num
literal composto de `AppError` — que não case com a expressão.

**Não derive o código do nome do campo.** O mesmo campo escreve-se `groupJID`
numa rota e `groupjid` noutra, e nenhuma das duas grafias é canónica; derivar
produziria dois códigos para uma condição, ambos inválidos. O código é
explícito no sítio da chamada, como constante nomeada — ver
`pkg/presentation/http/handlers/errors.go`.

**Mensagem pt-BR, causa em inglês, e a causa vai EMBRULHADA.** `apperr.New`
recebe as duas: `Message` é o que vai para o fio, o `err` embrulhado é o que
vai para o log (`AppError.Error()` concatena os dois). Nunca interpole a causa
dentro de `Message` — o texto de um erro de driver traz a instrução SQL e o
alvo da ligação, e o de um descodificador de base64 traz um byte do PEDIDO.
`RespondJSON` nunca serializa a cadeia embrulhada; escrevê-la em `Message`
seria contorná-lo à mão.

### O campo `debug` em desenvolvimento: NÃO foi acrescentado

O plano previa um `error.debug` com `exception`/`stack_trace` apenas em
desenvolvimento. **Não foi implementado, e a decisão é deliberada**, não uma
omissão:

- `api/openapi/CONTRATO-ARQUITETURAL.md` §14.1 já regista a decisão contrária,
  com o argumento: acrescentá-lo criaria um caminho de fuga que hoje **não
  existe**, dependente de uma variável de ambiente para não disparar. A
  propriedade actual — *nunca vaza, em ambiente nenhum* — é mais forte que
  *vaza só quando a variável está desligada*.
- A única bandeira de desenvolvimento deste repositório é
  `devui.EnvEnabled` (`WA_API_DEV_UI`, ver `pkg/presentation/http/devui/devui.go`),
  e ela liga o **painel de teste** e a exposição do token de admin. Pendurar
  fuga de stack trace nela alargaria o raio dessa bandeira em silêncio.

**Seguimento nomeado**, se alguém quiser reabrir: precisa de uma bandeira
PRÓPRIA (não `WA_API_DEV_UI`), de um teste que prove que o campo NÃO aparece
com a bandeira desligada, e de uma entrada em `HOUSEKEEP.md`. Não o acrescente
de passagem numa migração de família.

### Paginação

**Não foi introduzida nenhuma forma de `meta`**, porque nenhuma rota de
listagem hoje pagina: `chat/list`, `group/list`, `newsletter/list` e
`admin/users` devolvem a colecção inteira. Inventar `{"data": [...], "meta":
{...}}` agora seria contrato sem implementação.

Quando a primeira rota paginar, a forma é esta, e vai aqui como norma antes de
ir para o código:

```json
{"success": true, "code": 200,
 "data": {"items": [...], "meta": {"next_cursor": "…", "has_more": true}}}
```

`meta` dentro de `data`, e não ao lado — `data` é a carga útil da rota, e mover
o envelope por causa de uma família partiria as outras 100 rotas.

## 8. Nomes no fio

**Regra**: toda chave de objecto JSON público, recursivamente — objectos
aninhados e objectos dentro de arrays — casa com

```
^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$
```

Isto recusa `GroupJID`, `groupJid`, `group-jid`, `group__jid`, `_group`,
`group_`. Vale para respostas E para corpos de pedido.

Aplica-se também aos **valores enumerados** que o servidor escreve
(`admin_add`, `invalid_request`), não só às chaves.

Afirme-o com o helper partilhado, nunca com uma lista escrita à mão:

```go
import "wa-api/pkg/presentation/http/contracttest"

contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
contracttest.AssertNoKeys(t, rec.Body.Bytes(), "GroupJID", "Participants")
```

Assinaturas:

```go
func IsCanonicalKey(key string) bool
func AssertPublicJSONUsesCanonicalNaming(t TestingT, body []byte)
func AssertNoKeys(t TestingT, body []byte, keys ...string)
```

`TestingT` é uma interface estreita (`Helper`, `Errorf`, `Fatalf`) e não
`*testing.T`, para que o próprio pacote possa provar que os helpers **mordem**
— ver `pkg/presentation/http/contracttest/naming_test.go`. `*testing.T`
satisfá-la sem conversão.

## 9. Como testar uma família migrada

O mínimo, por rota migrada — copie
`pkg/presentation/http/handlers/handler_group_info_contract_test.go`:

1. **Pela ROTA REGISTADA**, num `mux.Router` montado com
   `customhttp.NewHandlerRegistry()`, e não com o manipulador nu. O router é
   `gorilla/mux`; um manipulador montado à mão não exercita método, padrão de
   caminho nem extracção de parâmetro (`ARMADILHAS.md` #2).
2. **Caminho de SUCESSO**, com a sessão autenticada injectada no contexto
   (`appport.UserInfoKey`). Um teste que só exercita a guarda mede a guarda.
3. **`AssertPublicJSONUsesCanonicalNaming`** sobre o corpo.
4. **`AssertNoKeys`** com as chaves ANTIGAS. "A chave nova existe" não prova
   migração: um struct pode carregar as duas.
5. **Valores mapeados**, não só chaves. Uma troca entre dois booleanos vizinhos
   passa em tudo o resto.
6. **Zero e vazio**: tempo desconhecido → `null`; colecção vazia → `[]`.
7. **CONTROLO NEGATIVO EXECUTADO**: reintroduza o defeito (troque uma etiqueta
   para PascalCase, ou volte a passar o valor de domínio a `RespondJSON`),
   confirme que o teste FALHA, cole a saída na entrada de `HOUSEKEEP.md`, e
   reverta. Um teste que passa mas não morde é pior que nenhum.

E o **dublê**: `contractsfake.GroupDirectory` já não devolve `(nil, nil)` no
zero-value — nenhum dos dois adaptadores reais o faz, e um dublê mais simples
que a produção **abençoa código morto** (`ARMADILHAS.md` #1). Quando migrar uma
porta, alinhe o dublê com o que a produção realmente devolve.

## 10. O gate arquitectural

`pkg/presentation/http/handlers/respondjson_ledger_test.go` mantém um
**livro-razão** (`testdata/respondjson_ledger.tsv`) de toda chamada a
`RespondJSON` no pacote de manipuladores, com a expressão serializada e a sua
classificação: `dto`, `nil` ou `pendente`.

Falha em dois casos:

1. o livro-razão deixou de descrever o código — chamada acrescentada, removida
   ou alterada sem actualizar;
2. **o número de `pendente` subiu**. É a catraca: `maxPendingRespondJSONSites`,
   hoje **108**. Baixe-o quando a sua família migrar; nunca o suba.

Actualizar depois de uma migração:

```
go test ./pkg/presentation/http/handlers/ -run TestRespondJSONLedger -update-ledger
# e baixe maxPendingRespondJSONSites para o novo número que o teste reporta
```

### O compromisso, escrito com todas as letras

Um gate PRECISO exigiria `go/types` sobre a árvore inteira para saber o **tipo
estático** do terceiro argumento — `rsp` de domínio e `rsp` de DTO são a mesma
palavra. São 404 sítios de chamada, e resolver tipos em todos custa uma
dependência de `packages.Load` dentro do gate.

O que está implementado classifica pela **forma** da expressão: `nil` é `nil`,
prefixo `dto` é `dto`, o resto é `pendente`. **É por isso que a convenção de
importação `dtogroup`/`dtosession`/… não é estética: é o que torna o gate
verificável.**

O que este gate **NÃO** apanha: um apresentador que devolva o tipo errado, ou
uma variável chamada `dtoAlgo` que não seja DTO nenhum. Esse lado fica coberto
pelo teste de contrato por rota (§9), que afirma os nomes das chaves realmente
servidas — e nenhum tipo errado sobrevive a ele. Os dois juntos cobrem: o gate
apanha a chamada que ninguém classificou, o teste de contrato apanha o valor
errado.

## 11. Corte a seco — o que NÃO se faz

Este projecto **não tem consumidores externos**. Portanto:

- **sem** DTO "legado" ao lado do novo;
- **sem** serialização dupla, nem chaves antiga+nova no mesmo corpo;
- **sem** pares `LegacyResponse`/`CanonicalResponse`;
- **sem** alias depreciados nem cabeçalho de versão.

A etiqueta `json` antiga é **removida ou alterada**, e pronto.

E: **um teste antigo que protege um contrato mau não é um requisito de
compatibilidade.** Actualize-o. Foi o que se fez a
`TestRespondJSON_UntypedError_UsesGenericMessage`, que afirmava
`envelope["error"] == "bad request"` — hoje é
`TestRespondJSON_UntypedError_UsesCanonicalErrorObject` e afirma o objecto.

## 12. Antes / depois

### (a) O tipo de fio deixa de ser o de domínio

```go
// ANTES — pkg/domain/group.go: o domínio ERA o fio
type GetGroupInfoResult struct {
    GroupInfo interface{} `json:"group_info"` // any: a forma dependia do motor
}
```

```go
// DEPOIS — pkg/domain/group.go: sem etiqueta, porque já não é o fio
type GetGroupInfoResult struct {
    GroupInfo *GroupInfo
}
```

### (b) O manipulador apresenta em vez de serializar

```go
// ANTES — pkg/presentation/http/handlers/handler_group.go
rsp, err := h.usecase.Execute(r.Context(), id, req)
...
customhttp.RespondJSON(w, 200, rsp, nil)
```

```go
// DEPOIS
rsp, err := h.usecase.Execute(r.Context(), id, req)
...
customhttp.RespondJSON(w, 200, dtogroup.PresentGetGroupInfo(rsp), nil)
```

### (c) O corpo servido

```jsonc
// ANTES: nomes de campo do Go, porque a struct de protocolo não tem etiquetas
{"success": true, "code": 200, "data": {"group_info": {
  "JID": "120363411669320145@g.us",
  "OwnerJID": "90937376170214@lid",
  "IsAnnounce": false,
  "GroupCreated": "2026-08-26T08:42:47-04:00",
  "Participants": [{"JID": "…", "IsAdmin": false, "Error": 0, "AddRequest": null}]
}}}
```

```jsonc
// DEPOIS
{"success": true, "code": 200, "data": {"group_info": {
  "jid": "120363411669320145@g.us",
  "owner_jid": "90937376170214@lid",
  "is_announce": false,
  "created_at": "2026-08-26T12:42:47Z",
  "participants": [{"jid": "…", "phone_number": "…", "lid": "…",
                    "display_name": "", "is_admin": false, "is_super_admin": false}]
}}}
```

### (d) O erro deixa de ter duas formas

```jsonc
// ANTES: dependia de o erro ter passado pela taxonomia
{"code": 400, "error": {"code": "missing_group_jid", "message": "…"}, "success": false}
{"code": 400, "error": "bad request", "success": false}   // F266
```

```jsonc
// DEPOIS: uma só forma, em todo estado
{"code": 400, "error": {"code": "missing_group_jid", "message": "…"}, "success": false}
{"code": 400, "error": {"code": "invalid_request", "message": "Requisição inválida."}, "success": false}
```

## 13. Documentação da API

A especificação é **gerada**: edite `api/openapi/{base,paths/,schemas/}` e corra
`go run ./cmd/openapidoc`. Nunca edite
`pkg/presentation/http/apidocs/openapi.yaml`.

Quando migrar uma família:

- acrescente o esquema CANÓNICO ao lado do antigo enquanto outras rotas ainda
  usarem o antigo — foi o que se fez com `InfoGrupoCanonico` ao lado de
  `InfoGrupo`, porque `/group/list` e `/group/inviteinfo` ainda não migraram;
- reescreva o `example` da resposta com as chaves novas: há um gate
  (`TestContratoExemploDeErroBateComOEsquema`) que compara exemplo e esquema;
- o `error` de todo exemplo tem de ser objecto com `code` e `message` — a
  tolerância à forma antiga saiu de `envelopeDeErroValido`;
- e confirme que o binário serve o que o ficheiro diz, porque a especificação é
  **embutida**:

  ```
  curl -s localhost:8080/docs/openapi.yaml | cmp - pkg/presentation/http/apidocs/openapi.yaml
  ```
