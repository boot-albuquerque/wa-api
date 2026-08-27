# O gate de nomenclatura da fronteira HTTP

Este documento descreve o **mecanismo de deteção**, não a norma em si — a
norma é `docs/HTTP-DTO-CONVENTIONS.md` §8 (chaves JSON) e §9-10 (como
testar, o gate arquitetural de `RespondJSON`). Leia aquele primeiro. Este
ficheiro existe para responder uma pergunta diferente: **como sei que a
norma continua a valer amanhã?**

A resposta são três testes Go permanentes, cada um cobrindo uma superfície
que as outras duas não alcançam. Nenhum deles corrige nada — os três só
detetam, e falham em cima do que encontram. Corrigir é trabalho de quem for
dono da família de rotas.

## As três superfícies

```
                    ┌─────────────────────────┐
                    │  docs/HTTP-DTO-          │
                    │  CONVENTIONS.md §8        │
                    │  (a norma)                │
                    └────────────┬─────────────┘
                                  │
        ┌─────────────────────────┼─────────────────────────┐
        │                         │                         │
        ▼                         ▼                         ▼
┌───────────────┐       ┌──────────────────┐      ┌──────────────────┐
│ caminho de URL │       │ especificação     │      │ corpo JSON        │
│ registado      │       │ OpenAPI gerada    │      │ servido de verdade│
│                │       │ (embutida)        │      │                    │
│ naming_paths_  │       │ naming_openapi_   │      │ naming_gate_live_ │
│ gate_test.go   │       │ gate_test.go       │      │ test.go            │
│ (pkg/bootstrap)│       │ (pkg/bootstrap)    │      │ (handlers)         │
└───────────────┘       └──────────────────┘      └──────────────────┘
```

Um documento pode prometer a forma certa enquanto o código serve outra — e
um caminho de URL pode estar certo mesmo quando o corpo que ele devolve não
está. As três superfícies divergem de formas independentes, então nenhuma
delas sozinha basta.

### 1. Caminhos de URL — `pkg/bootstrap/naming_paths_gate_test.go`

Enumera toda rota registada via `bootstrap.Routes(Deps{})` — a mesma função
que `cmd/listroutes` usa, e portanto a mesma árvore `gorilla/mux` que a
produção monta. Três testes:

- **`TestPathsAreLowercaseKebabCase`** — todo segmento não-parâmetro é
  minúsculo; se tiver mais de uma palavra, é kebab-case.
- **`TestPathParamsAreSnakeCase`** — todo `{parâmetro}` é snake_case
  (`{group_jid}`, não `{groupId}`).
- **`TestPathSegmentsAreNotConcatenatedCompounds`** — o meio que um regex
  sozinho não cobre. `requestparticipants` e `communities` são **ambas**
  sequências minúsculas sem separador — um regex não distingue "duas
  palavras coladas" de "uma palavra real". Este teste resolve isso com duas
  listas curadas à mão, no próprio ficheiro:

  - `knownBadConcatenatedSegments` — compostos conhecidos que FALHAM mesmo
    tendo forma válida. O valor de cada entrada é a correção sugerida.
  - `allowlistedSingleWordSegments` — palavras de uma peça só, confirmadas,
    que NÃO devem voltar a ser sinalizadas.

  Um segmento novo, minúsculo, com 8+ caracteres, que não estiver em
  nenhuma das duas listas **falha o teste com uma mensagem que diz
  exatamente isso** — é assim que o gate não fica cego à próxima rota.
  Adicionar a uma lista é uma alegação ("verifiquei, isto não é duas
  palavras coladas"); justifique no commit, não só no comentário.

### 2. Especificação OpenAPI — `pkg/bootstrap/naming_openapi_gate_test.go`

Percorre `pkg/presentation/http/apidocs/openapi.yaml` **gerado e embutido**
(via `especificacao(t)`, o mesmo parser que `openapi_contrato_test.go` já
usa) — não os fragmentos fonte em `api/openapi/{base,paths,schemas}/`,
porque `go run ./cmd/openapidoc` pode perder ou corromper um fragmento no
caminho até ao documento final, e checar a fonte não pegaria essa classe de
defeito.

Um `walkOpenAPIDoc` recursivo visita **toda** ocorrência de `properties`,
`enum`, `example`/`examples` e `name` de parâmetro, esteja ela sob
`components.schemas` ou inline num pedido/resposta/parâmetro — quatro
testes:

- `TestOpenAPISchemaPropertyNamesAreCanonical`
- `TestOpenAPIEnumValuesAreCanonical` — com a exclusão documentada em
  `nonCodeEnumValue` para as quatro formas que NÃO são códigos: texto livre
  com espaço, `***` de mascaramento, literal de duração (`0`, `24h`, `7d`,
  `90d`), e a cadeia vazia `''` como membro documentado (não omissão).
- `TestOpenAPIExampleKeysAreCanonical` — reutiliza a MESMA regra de chave
  (`contracttest.IsCanonicalKey`) contra o payload literal de cada exemplo,
  recursivamente.
- `TestOpenAPIParameterNamesAreCanonical` — nomes de parâmetro de
  caminho/query; exclui `in: header`, porque um cabeçalho HTTP segue a
  convenção de cabeçalho (`Authorization`), não o alfabeto de chave JSON.

### 3. Corpo JSON ao vivo — `pkg/presentation/http/handlers/naming_gate_live_test.go`

A superfície que as outras duas não alcançam: a especificação pode estar
certa e o handler ainda servir a forma antiga. Uma rota por família
(sessão, grupos, mensagens, utilizadores, admin, canais), montada no
`*mux.Router` real — nunca o manipulador nu (`ARMADILHAS.md` #2) —,
autenticada, contra o helper partilhado que já existia desde a fundação
DTO:

```go
import "wa-api/pkg/presentation/http/contracttest"

contracttest.AssertPublicJSONUsesCanonicalNaming(t, body)
```

Este ficheiro não recria esse helper — reutiliza-o, o mesmo que
`handler_group_info_contract_test.go` usa como implementação de
referência, e o mesmo que qualquer teste de contrato por rota (§9 de
`docs/HTTP-DTO-CONVENTIONS.md`) deve usar.

## Como correr

Os três ficheiros são testes Go normais, dentro de `go test ./...` — que é
o que `make test` e `make check` já executam. Nenhuma alteração ao
`Makefile` foi necessária para os ligar ao CI.

```
go test ./pkg/bootstrap/... -run 'TestPaths|TestOpenAPI'
go test ./pkg/presentation/http/handlers/... -run TestLiveNaming_
```

## Estado medido em 2026-08-27 (antes das seis migrações fundirem)

Ver `HOUSEKEEP.md`, entrada **F297**, para a tabela completa
passa/falha por teste e a evidência de cada achado — incluindo os cinco
compostos concatenados (`downloadimage`, `downloadvideo`, `downloadaudio`,
`downloaddocument`, `downloadsticker`) encontrados ao construir este gate e
não registados em nenhuma sessão anterior, e a pergunta em aberto sobre
dicionários de chave dinâmica (JID, emoji) nos esquemas `Roster`,
`InfoUtilizadores` e `MensagemCanal`.

É esperado que os três gates reportem falhas reais até as worktrees-irmãs
fundirem as migrações de sessão, utilizadores, mensagens, canais e admin —
grupos e mensagens (a rota `/chat/send/contact` e afins) já passam. Uma
falha aqui não é um defeito do gate: é o gate a fazer o que foi construído
para fazer.
