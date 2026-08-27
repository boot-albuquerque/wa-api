# Como o whatsmeow gera os arquivos `proto/`

> Pesquisa sobre a origem e o pipeline de geração dos arquivos protobuf usados
> pelo projeto [tulir/whatsmeow](https://github.com/tulir/whatsmeow), do qual
> `wa-api` depende (ver [ADR-0002](adr/0002-vendorizar-whatsmeow-em-vez-de-reimplementar.md)).
> Relevante para entender de onde vêm `proto/waE2E`, `proto/waCommon` etc. e
> como/quando eles são atualizados a montante.

## Resumo

Os arquivos `.pb.go` em `proto/` **não são escritos à mão** e **não usam
`buf`**. São gerados por um pipeline próprio do mantenedor (Tulir Asokan):

```
bundle JS do WhatsApp Web (protos.js)
        │
        ▼  proto/parse-proto.js  (Node.js)
   arquivos .proto (texto)
        │
        ▼  protoc --go_out=... (protoc-gen-go)
   arquivos .pb.go (Go)
```

## 1. Origem do schema: engenharia reversa do WhatsApp Web

Os `.proto` são extraídos diretamente do bundle JavaScript que o WhatsApp Web
carrega no navegador — não de uma spec pública. Evidência: `proto/.gitignore`
exclui os dumps brutos de entrada do controle de versão:

```
protos.js
EBMinosWasm
EBWasm
KTWasm
VestaWasm
waArmadilloLocallyTransformedMessage
MDCoreSyncProtobuf
```

Esses arquivos (o bundle JS principal + alguns módulos WASM, usados para os
esquemas do Armadillo/integração com Instagram) precisam ser baixados
manualmente do app web antes de rodar o gerador.

### Como esse `.gitignore` evoluiu

O arquivo não nasceu com essa lista completa — foi crescendo conforme o
mantenedor precisou extrair schemas de novos subsistemas do WhatsApp Web.
Histórico completo (`gh api "repos/tulir/whatsmeow/commits?path=proto/.gitignore"`),
com o conteúdo do arquivo em cada revisão:

| Commit | Data | Conteúdo do `proto/.gitignore` |
|---|---|---|
| `74c49f5a` | 2024-05-21 | `js` (arquivo de entrada ainda sem nome definitivo) |
| `64bc969f` | 2024-06-02 | `protos.js` (renomeado para o nome usado até hoje em `generate.sh`) |
| `6661da30` | 2026-05-04 | `+ EBMinosWasm`, `EBWasm`, `KTWasm`, `VestaWasm`, `waArmadilloLocallyTransformedMessage` — adicionados juntos num commit rotineiro de atualização (`proto: update to v1038709472`), que também introduziu os pacotes `waAICommon`, `waWa6`, `waWeb` |
| `eaa388b4` | 2026-06-16 | `+ MDCoreSyncProtobuf` |

Ou seja: cada entrada nova corresponde a um módulo WASM/JS adicional que o
WhatsApp Web passou a expor e que o extrator (`parse-proto.js`, privado desde
`3d63c6fc`) precisou ingerir para gerar os `.proto` de um novo subsistema.
Interpretação dos nomes (não confirmada oficialmente, apenas inferida pelo
contexto do commit e por convenções conhecidas do WhatsApp/Meta):

- **`waArmadilloLocallyTransformedMessage`** — claramente ligado ao
  **Armadillo**, o formato de mensagem unificado da Meta compartilhado entre
  WhatsApp, Instagram e Messenger (o próprio whatsmeow tem pacotes
  `waArmadillo*` e `instamadillo*` gerados a partir disso).
- **`KTWasm`** — provável referência a **Key Transparency**, feature de
  verificação de chaves de criptografia ponta-a-ponta que WhatsApp/Signal
  documentam publicamente; roda como módulo WASM no cliente web.
- **`MDCoreSyncProtobuf`** — provável referência a **MultiDevice Core Sync**
  (sincronização de estado entre dispositivos vinculados).
- **`EBWasm` / `EBMinosWasm` / `VestaWasm`** — sem confirmação pública; são
  nomes de codinome interno da Meta para módulos WASM do WhatsApp Web, sem
  ocorrência em outras partes do código-fonte do whatsmeow (não aparecem em
  `parse-proto.js` nem em nenhum outro arquivo do repositório, além do
  `.gitignore`), então seu propósito exato só é conhecido pelo mantenedor.

## 2. Scripts do pipeline

Os scripts originais foram removidos do repositório público no commit
[`3d63c6fc`](https://github.com/tulir/whatsmeow/commit/3d63c6fcc1a7db114358782118f017e14200fac)
("proto: remove parse script", 2024-08-21), mas o conteúdo foi recuperado do
diff desse commit.

**`proto/generate.sh`** (10 linhas):

```bash
#!/bin/bash
cd $(dirname $0)
set -euo pipefail
if [[ ! -f "protos.js" ]]; then
	echo "Please download the WhatsApp JavaScript modules with protobuf schemas into protos.js first"
	exit 1
fi
node parse-proto.js
protoc --go_out=. --go_opt=paths=source_relative --go_opt=embed_raw=true */*.proto
pre-commit run -a
```

**`proto/parse-proto.js`** (~469 linhas) — script Node.js que:

- Carrega `protos.js` (dump bruto do bundle de módulos protobuf do WhatsApp Web).
- Emula o module-loader próprio do WhatsApp Web (`global.__d = defineModule`,
  `global.window = {}`) para avaliar as definições de schema em processo.
- Aplica mapas de renomeação de dependências (ex.: `WAProtocol.pb → WACommon.pb`)
  e regras de dedup/ignore (ex.: descarta variantes duplicadas como
  `MAWArmadillo*TableSchema`).
- Percorre o grafo de objetos resultante e reemite texto `.proto` via funções
  como `protoifyMessage` / `protoifyEnum`.

Depois disso, `protoc` compila os `.proto` para Go com `protoc-gen-go` padrão
— **não** `buf`.

## 3. Ferramentas e versões confirmadas

Cabeçalho de `proto/waCommon/WACommon.pb.go`:

```go
// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.11
// 	protoc        v6.33.5
// source: waCommon/WACommon.proto
```

Isso bate com a exigência de `google.golang.org/protobuf v1.36.11` no
`go.mod` do whatsmeow.

Marcadores de arquivo gerado em `.gitattributes`:

```
*.pb.go linguist-generated=true
*.pb.raw binary linguist-generated=true
internals.go linguist-generated=true
```

## 4. Evolução histórica (commits-chave)

| Commit | Data | Significado |
|---|---|---|
| [`74c49f5a`](https://github.com/tulir/whatsmeow/commit/74c49f5a) | 2024-05-21 | *"Update to web version 2.3000.1013652801 and break protobufs"* — refatoração que quebrou o pacote monolítico antigo `binary/proto` nos pacotes modulares atuais (`waE2E`, `waCommon`, `waAdv`, ...). Mensagem do commit: *"This includes redoing the protobuf extractor and redirecting legacy binary/proto package to the new packages which are shared with FB Messenger mode."* Tocou 152 arquivos. |
| [`3d63c6fc`](https://github.com/tulir/whatsmeow/commit/3d63c6fcc1a7db114358782118f017e14200fac) | 2024-08-21 | *"proto: remove parse script"* — `generate.sh`/`parse-proto.js` removidos do repo público. A geração continua existindo (fora do repo/privada), já que os commits `proto: update to vNNNNNNNNNN` seguem aparecendo regularmente. |
| `201d8734` | 2025-11-15 | *"proto: remove remaining legacy getter proxies"* — limpeza adicional da camada de compatibilidade `binary/proto`. |
| `7ae702b1` | 2026-02-19 | *"proto: add decoders for raven messages"* — exemplo de código manual adicionado sobre o código gerado. |
| (recorrente) | semanal, até `e229058e` em 2026-08-04 | Commits `proto: update to vNNNNNNNNNN`, todos por Tulir Asokan, onde o número é a versão de build interna do WhatsApp Web — o mantenedor reexecuta o extrator (agora privado) a cada release nova do WhatsApp Web. |

### Camada de compatibilidade legada

`binary/proto/legacy.go` e `binary/proto/doc.go` ainda existem como shims
finos que reexportam símbolos dos novos pacotes modulares, por exemplo:

```go
PeerDataOperationRequestType_GENERATE_LINK_PREVIEW = waE2E.PeerDataOperationRequestType_GENERATE_LINK_PREVIEW
```

## 5. Estrutura atual de `proto/`

Um subdiretório por pacote protobuf — `waE2E`, `waCommon`, `waAdv`,
`waSyncAction`, `waHistorySync`, família `waArmadillo*`, família
`instamadillo*`, `waConsumerApplication` etc. — cada um com um `.proto` fonte
e o `.pb.go` gerado correspondente. Apenas `extra.go` (código de suporte
escrito à mão, ex.: interface `MessageApplicationSub` que unifica tipos de
mensagem Armadillo/Consumer) e `.gitignore` não são gerados.

Não existe alvo de `Makefile` para regenerar protobufs na raiz do repositório
whatsmeow. O CI (`.github/workflows/go.yml`) só builda, testa e roda
`pre-commit` — não regenera protobufs.

## Implicações para o `wa-api`

- **Não editar manualmente** nada em `proto/` de uma cópia vendorizada do
  whatsmeow — são arquivos gerados e serão sobrescritos na próxima
  atualização.
- Atualizações de protocolo do WhatsApp chegam via `go get -u` da versão do
  whatsmeow (ou atualização do vendor), não via regeneração local — o
  pipeline gerador é privado do mantenedor upstream.
- Se um campo/mensagem novo do WhatsApp for necessário antes de existir uma
  release do whatsmeow com suporte, a única via é aguardar um commit
  `proto: update to vNNNNNNNNNN` upstream ou abrir issue/PR lá.

## Opções para reduzir a dependência manual do upstream

Levantamento de 2026-08-26, feito para responder "como podemos gerar/atualizar
os `.proto` do Noise sem depender do Tulir fazer isso manualmente?". Nenhuma
das quatro foi implementada ainda — registradas aqui para exploração futura,
cada uma com o trade-off que a distingue.

### Opção 1 — Extrator próprio (reimplementar o pipeline privado)

Reconstruir `parse-proto.js` (recuperável do diff de `3d63c6fc`, ver seção
acima) e automatizar a etapa que hoje é manual: baixar `protos.js` do bundle
JS do WhatsApp Web via browser instrumentado (Playwright/Puppeteer), rodar o
parser, compilar com `protoc-gen-go`.

- **Vantagem**: independência total do cronograma do Tulir; captura schema
  novo no mesmo dia em que a Meta publica.
  **Custo**: manter um extrator JS/Node que emula o module-loader do
  WhatsApp Web — o mesmo tipo de manutenção reversa que o Tulir já faz, só
  que duplicada por nós. Quebra toda vez que a Meta muda o bundler ou o
  formato de `protos.js`.

### Opção 2 — Diff automatizado de versão + gatilho de extração

Um cron/CI que baixa periodicamente o bundle JS do WhatsApp Web, lê a versão
de build exposta nele (o mesmo número usado nos commits `proto: update to
vNNNNNNNNNN`), compara com a última versão sincronizada por nós, e só
dispara a Opção 1 quando detecta mudança.

- **Vantagem**: evita rodar o extrator caro a cada execução; sinaliza
  proativamente quando há novidade, antes mesmo de o Tulir publicar.
  **Custo**: ainda depende da Opção 1 existir por baixo — é uma otimização de
  gatilho, não uma alternativa a ela.

### Opção 3 — Geração Go a partir do `.proto` já existente

Uma vez com o `.proto` atualizado (por qualquer via), `protoc-gen-go` já
resolve a geração dos `.pb.go` de forma 100% automatizável — é o mesmo passo
que o `generate.sh` do whatsmeow já fazia antes de ser removido do repo
público. Não é alternativa às opções 1/2/4: é o passo final comum a todas,
o único elo do pipeline que não depende de engenharia reversa.

### Opção 4 — Espelhar o `proto/` do whatsmeow (vendoring automatizado)

Em vez de extrair schema do WhatsApp Web diretamente, um workflow que watch
o repositório upstream (`tulir/whatsmeow`) por novos commits `proto: update
to vNNNNNNNNNN` em `proto/`, e abre PR automático de vendoring no `wa-api`
quando detecta um.

- **Vantagem**: barato — não precisa reimplementar extração de schema, só
  monitorar commits. Consistente com o fato de já vendorizarmos
  `internal/wa-noise` (ver [ADR-0002](adr/0002-vendorizar-whatsmeow-em-vez-de-reimplementar.md)).
  **Custo**: não elimina a dependência do upstream, troca "depender de uma
  pessoa lembrar" por "depender de um repo+CI lembrar" — continuamos atrás do
  Tulir no tempo, nunca à frente.

### Comparação

| Opção | Elimina dependência do Tulir? | Custo de manutenção | Prazo de reação |
|---|---|---|---|
| 1. Extrator próprio | Sim | Alto (duplica engenharia reversa) | Mesmo dia da release da Meta |
| 2. Diff de versão + gatilho | Só combinada com a 1 | Baixo (é só o gatilho) | Detecção imediata, extração conforme opção 1 |
| 3. `protoc-gen-go` a partir do `.proto` | N/A (passo comum, não alternativa) | Nenhum (já automatizável hoje) | Instantâneo, dado o `.proto` |
| 4. Espelhar `proto/` do upstream | Não | Muito baixo | Atraso = cadência do Tulir (~4-8 dias) |

## Fontes

- https://github.com/tulir/whatsmeow/commit/3d63c6fcc1a7db114358782118f017e14200fac.patch (conteúdo recuperado de `generate.sh` e `parse-proto.js`)
- `gh api repos/tulir/whatsmeow/commits?path=proto` (histórico de commits)
- `gh api repos/tulir/whatsmeow/commits/74c49f5a` e `/3d63c6fc` (diffs)
- `gh api repos/tulir/whatsmeow/contents/proto/...` (listagens de diretório, `.gitignore`, cabeçalhos de arquivos gerados)
- `gh api repos/tulir/whatsmeow/contents/go.mod`, `.gitattributes`, `.github/workflows/go.yml`
