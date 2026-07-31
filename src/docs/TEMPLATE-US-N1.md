# Template US-N1: Migrar Handler para Clean Arch (Progressivo)

> **Objetivo:** Template reutilizável para migrar qualquer handler de `handlers.go` (package main)
> para o padrão Clean Arch estabelecido em `internal/`. Aplicar **apenas quando o handler for
> estendido** com novas funcionalidades (HMAC, idempotência, retries, novos campos).

## Pré-requisitos

- [x] Build: `go build ./...` verde
- [x] Testes `internal/`: 25/25 passam
- [x] Ports existentes: `ClientProvider`, `Logger`, `ProfileDataAccess`, `DBAccess`, `StoragePort`, `MessagingPort`
- [x] `custom_handlers.go` com wiring centralizado
- [x] `custom_routes.go` com `HandlerRegistry`

## Passo 1: Domain Entity / Request DTO

**Onde:** `internal/application/usecase/<nome>.go` (junto com o usecase)

**Regra:** Request DTOs vivem no mesmo arquivo do usecase que os consome. Entities de domínio
(`domain/`) são para conceitos que transitam entre camadas. DTOs de request HTTP são específicos
do usecase e não devem vazar para `domain/`.

```go
// send_message.go
type SendMessageRequest struct {
    Phone       string          `json:"Phone"`
    Body        string          `json:"Body"`
    LinkPreview bool            `json:"LinkPreview,omitempty"`
    ID          string          `json:"Id,omitempty"`
    ContextInfo waE2E.ContextInfo `json:"ContextInfo,omitempty"`
}

type SendMessageResult struct {
    MessageID string `json:"message_id"`
    Status    string `json:"status"`
}
```

## Passo 2: Port (se necessário)

**Onde:** `internal/application/port/`

**Regra:** Só criar nova port se o handler usa uma dependência ainda não abstraída. Reutilizar ports existentes sempre que possível:
- `port.ClientProvider` → acesso ao cliente whatsmeow
- `port.Logger` → logging
- `port.DBAccess` → banco de dados
- `port.StoragePort` → storage (S3, mídia)

Para SendMessage: **Zero novas ports.** Reusa `ClientProvider` + `Logger`.

## Passo 3: Usecase

**Onde:** `internal/application/usecase/<nome>.go`

**Regra:** Usecase recebe ports via constructor. Método `Execute(ctx, txtID, req) (Result, error)`.
Lógica de negócio extraída do handler original (validação, envio, resposta).
**NÃO referencia `clientManager` global, `s.Respond()`, ou `s.db`.**

```go
type SendMessageUseCase struct {
    clientProvider port.ClientProvider
    logger         port.Logger
}

func NewSendMessageUseCase(cp port.ClientProvider, l port.Logger) *SendMessageUseCase {
    return &SendMessageUseCase{clientProvider: cp, logger: l}
}

func (uc *SendMessageUseCase) Execute(ctx context.Context, txtID string, req SendMessageRequest) (*SendMessageResult, error) {
    // 1. Validar campos
    // 2. Obter cliente whatsmeow
    // 3. Enviar mensagem
    // 4. Retornar resultado
}
```

## Passo 4: Handler HTTP

**Onde:** `internal/interfaces/http/handlers/<dominio>_handler.go`

**Regra:** Handler extrai params do contexto (`port.UserInfoKey`), decodifica body JSON,
chama usecase, responde via `RespondJSON()` (helper em `internal/interfaces/http/response.go`).
**NÃO usa `s.Respond()` — o helper `RespondJSON()` provê o mesmo envelope `{code, data, error}`.**

```go
type SendMessageHandler struct {
    usecase *usecase.SendMessageUseCase
}

func (h *SendMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // 1. Extrair txtID do contexto
    // 2. Decodificar request body
    // 3. Chamar usecase.Execute(ctx, txtID, req)
    // 4. Responder via RespondJSON(w, statusCode, data, err)
}
```

### Contrato do `RespondJSON()`

```go
// RespondJSON escreve uma resposta JSON com envelope {code, data, error}
// compatível com o formato de s.Respond() do upstream.
func RespondJSON(w http.ResponseWriter, statusCode int, data interface{}, err error) {
    // Se err != nil, responde com {"code": statusCode, "error": "<mensagem genérica>"}
    // Se err == nil, responde com {"code": statusCode, "data": {...}}
}
```

## Passo 5: Wrapper em handlers.go

**Regra:** Substituir o corpo da função original por wrapper de 3 linhas que delega ao handler Clean Arch.
**NÃO alterar a assinatura da função** — o router em `routes.go` continua apontando para cá.

```go
// handlers.go — ANTES (100+ linhas)
func (s *server) SendMessage() http.HandlerFunc {
    type textStruct struct { ... }
    return func(w http.ResponseWriter, r *http.Request) {
        // ... 100+ linhas de lógica
    }
}

// handlers.go — DEPOIS (wrapper de 1 linha)
func (s *server) SendMessage() http.HandlerFunc {
    return customHandlerSet.Message.SendMessage.ServeHTTP
}
```

## Passo 6: Wiring em custom_handlers.go

```go
// Adicionar ao customHandlers struct
type customHandlers struct {
    Profile *customhttp.ProfileHandler
    Message *MessageHandlers  // NOVO
}

type MessageHandlers struct {
    SendMessage *handlers.SendMessageHandler
    SendImage   *handlers.SendImageHandler
}

// Em initCustomHandlers():
sendMessageUC := usecase.NewSendMessageUseCase(clientProvider, logger)
sendImageUC := usecase.NewSendImageUseCase(clientProvider, logger)
messageHandlers := &MessageHandlers{
    SendMessage: handlers.NewSendMessageHandler(sendMessageUC),
    SendImage:   handlers.NewSendImageHandler(sendImageUC),
}
```

## Passo 7: Testes

**Usecase:** Unit test com mocks de `ClientProvider` + `Logger` (reusa `testutil.MockClientProvider`).
**Handler:** httptest com mock usecase (padrão `profile_handler_test.go`).

```go
func TestSendMessageUseCase_Execute_Success(t *testing.T) { ... }
func TestSendMessageUseCase_Execute_NoSession(t *testing.T) { ... }
func TestSendMessageHandler_ValidRequest_200(t *testing.T) { ... }
```

## Exemplo Concreto: SendMessage

Ver `internal/application/usecase/send_message.go` (implementação real).

## Checklist de Migração

- [ ] Build verde: `go build ./...`
- [ ] Testes passam: `go test ./internal/...`
- [ ] Wrapper ≤5 linhas: `awk` no handler original
- [ ] Handler Clean Arch usa `RespondJSON()`, não `s.Respond()`
- [ ] Usecase não referencia `clientManager` global
- [ ] Wiring em `custom_handlers.go` atualizado
- [ ] Rota em `routes.go` não precisa mudar (wrapper preserva assinatura)

## Deadlines

- **Q2 2027:** Todos os handlers do domínio de mensagens (14 handlers: SendMessage, SendImage,
  SendDocument, SendAudio, SendVideo, SendSticker, SendContact, SendLocation, SendButtons,
  SendList, SendPoll, DeleteMessage, SendEditMessage, SendTemplate) migrados para Clean Arch.
- **Revisão trimestral:** A cada planning review, avaliar progresso e ajustar deadline.
- **Se deadline passar sem migração:** Estabilizar o padrão híbrido como decisão arquitetural explícita
  (ADR update), removendo a expectativa de migração futura.
