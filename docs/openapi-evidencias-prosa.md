# Relatório de evidências — documentação OpenAPI

**Actualizado a 2026-08-28**, depois da campanha F239/F282 (Fases 0-9,
HOUSEKEEP F344-F354) e de uma ronda de destrave ad hoc pedida pelo usuário
no mesmo dia, usando as sessões reais `envia`/`recebe`.

Contagem actual (137 rotas documentadas): **129 ✅, 4 🟡, 4 ❌, 0 ⬜.**
**Todas as 137 rotas já foram executadas pelo menos uma vez.**

A campanha F239/F282 não mudou marca nenhuma — só adicionou evidência
específica às 93 rotas que ainda tinham a frase-modelo genérica. A ronda de
destrave de 2026-08-28 (pedido explícito do usuário, "vamos destravar esses
que não precise da minha ação humana", seguida de "vamos seguir com os
próximos que posso estar ajudando" e da ajuda pessoal salvando um contacto
no telefone) moveu 5 rotas de 🟡/⬜ para ✅ (`POST /chats/download/{kind}`,
`POST /groups/{group_jid}/join-requests`, `POST /newsletters/react`,
`POST /chats/send/sticker` — com ajuda do usuário autorizando
`brew install ffmpeg-full`, F357 —, e `POST /status/set/image` — com ajuda
do usuário salvando `recebe` como contacto nomeado, F256) e investigou a
fundo duas 🟡 que continuaram 🟡 com causa agora DETERMINADA em vez de
aberta: `POST /newsletters/mark-viewed` (F356, não destravável nesta base)
e `POST /status/set/video`+`POST /status/set/audio` (F358, achado
incidental novo — ver abaixo).

## O contrato descreve um nome por operação

O router serve rotas antigas pluralizadas (F269) que **continuam a
responder** e saíram da especificação — mesma política de coexistência
permanente descrita em `api/openapi/CAMINHOS-CANONICOS.md`.
`POST /chats/download/{kind}` (CAP-10) é a rota que consolida as cinco
antigas `/chat/download{tipo}`, com o kind na relação do caminho: a forma
intermédia (`/chats/downloadimage` etc., que a F269 tinha pluralizado) foi
RETIRADA do contrato e do serviço no mesmo dia em que nasceu, e as CINCO
formas originais (`/chat/downloadimage` etc.) foram removidas do serviço em
2026-08-27 (HOUSEKEEP.md F297) — reversão explícita, só para esta família,
da política de coexistência permanente, porque não havia consumidor real a
proteger antes do lançamento. Devolvem `404` agora.

Documentar as duas formas punha 232 operações para 141 capacidades, e obrigava
o leitor a escolher entre `/chat/list` e `/chats/list` sem elemento para
decidir. A coluna *substitui* abaixo diz, para cada operação canónica, que nome
antigo continua a funcionar.

## Legenda

| marca | o que exige |
|---|---|
| ✅ | pedido HTTP real **e** confirmação independente do efeito |
| 🟡 | pedido real com sucesso, sem forma de confirmar o efeito |
| ❌ | pedido real que falhou; o erro concreto está na descrição da rota |
| ⬜ | não executada, e a razão é específica |

## Resumo quantitativo

**Este bloco continua manual.** Vem de duas fontes que `cmd/openapidoc` não
lê por desenho — a wiring de rotas (`pkg/bootstrap/wiring_routes.go`, para
`Rotas servidas pelo router`/`antigas, fora do contrato`/`/docs e /docs/`) é
propriedade do servidor, não da documentação; `Esquemas`/`Propriedades com
semântica` exigiriam percorrer `api/openapi/schemas/`, que este gerador
também não faz hoje. Quando `Operações documentadas` e `Validação` abaixo
divergirem deste bloco, é este bloco que está desatualizado — as duas
tabelas seguintes (`Por grupo`, `Tabela completa`) já são GERADAS a partir
de `api/openapi/evidencias.tsv`, e são a fonte de verdade para o resto.

```
Rotas servidas pelo router:     235
  documentadas (canónicas):     137
  antigas, fora do contrato:     96   (continuam a responder)
  /docs e /docs/:                 2

Operações documentadas:         137
Cobertura do contrato:          100%
Caminhos distintos:             118
Esquemas:                       165
Propriedades com semântica:     694 de 694

Validação:
  OK  chamada real com efeito confirmado: 129
  AMR sucesso sem observador independente: 4
  ERR falhou, com o erro medido:          4
  NT  não testada, com o motivo dito:     0
```


<!-- GERADO:POR-GRUPO -->

## As quatro que falharam

| Endpoint | Esperado | Obtido | Erro |
|---|---|---|---|
| `POST /users/block` | `200` | `422` | `upstream_rejected`; no log, `info query returned status 400: bad-request`. Medido nas duas contas, com número real e inexistente. Achado **F264**. |
| `POST /users/unblock` | `200` | `422` | idem. `GET /users/blocklist` funciona — só a escrita é recusada. |
| `POST /newsletters/updates` | `200` | `500` | `context deadline exceeded` ao fim de 30,0 s: o servidor do WhatsApp nunca responde. `POST /newsletters/messages` no mesmo canal e segundo devolve `200`. Achado **F265**. |
| `POST /session/logout` | `200` | `500` | `{"error":"internal server error"}` numa sessão descartável LIGADA e nunca emparelhada; no log, `the store doesn't contain a device JID`. Sem transporte vivo a rota responde `409 session_not_connected`, que bate com a documentação — é o ramo do meio que não tem código próprio. Achado **F275**. |

### As três de protocolo têm causa DETERMINADA (2026-08-26)

Continuam ❌ — nada foi corrigido, e a marca só muda quando a rota responder.
Mas nenhuma é já "não funciona e não sabemos porquê": as duas investigações
estão em `INVESTIGATION-block-unblock.md` e
`INVESTIGATION-newsletter-updates.md`, e a matriz de capacidades com nível de
evidência por afirmação em `WHATSAPP-CAPABILITIES.md`.

| Endpoint | Classificação | Causa |
|---|---|---|
| `POST /users/block` | `PROTOCOL_CHANGED` | a escrita da blocklist migrou para endereçamento por **LID**: `<item jid='…@lid' action='block' pn_jid='…@s.whatsapp.net'/>`. Enviamos a forma anterior. Baileys migrou em 2026-04-24, whatsmeow em 2026-08-13 — 13 dias antes desta medição. |
| `POST /users/unblock` | `PROTOCOL_CHANGED` | idem, com `jid`=LID e **sem** `pn_jid`. |
| `POST /newsletters/updates` | `PROTOCOL_CHANGED` | o servidor deixou de atender `<message_updates>` endereçado ao JID do canal — ignora-o, daí o silêncio até ao timeout. O WA Web usa `<messages type='jid'>` para `s.whatsapp.net`, que é o que `/newsletters/messages` já envia com sucesso. |

**Duas linhas da tabela acima estavam erradas e foram corrigidas**: a F264 dizia
"testado em PN e em LID", mas o adaptador converte LID → PN antes de enviar, e
as duas entradas produzem o mesmo stanza (F278). A F265 atribuía o silêncio a um
query ID desactualizado, e essa rota não tem query ID nenhum (não é MEX), o que
faz cair a hipótese.

A quarta, `POST /session/logout`, não é de protocolo: é taxonomia de erro nossa
(F275), e o caminho de `200` exige uma conta emparelhada — está em
`HUMAN-LAST.md`.

## As quatro 🟡, e o que realmente as bloqueia

O inventário completo está em `OBSERVADORES-AMBAR.md`. **Quatro saíram
desta lista em 2026-08-28** (`POST /groups/{group_jid}/join-requests`,
`POST /newsletters/react`, `POST /chats/send/sticker` e
`POST /status/set/image`, movidas para ✅ — ver "Destrave de 2026-08-28"
abaixo). Das quatro que restam, `request-unavailable-message` exige
conta emparelhada, `mark-viewed` tem causa determinada e não é destravável
nesta base, e `status/set/video`/`status/set/audio` têm um achado
incidental NOVO (F358): a mesma pré-condição de `status/set/image` já foi
satisfeita, mas o observador mostra uma entrega incompleta específica de
vídeo/áudio, não uma falta de pré-condição.

| Endpoint | Motivo preciso |
|---|---|
| `POST /chats/request-unavailable-message` | 200. **Observador existe**: o reenvio chega como `*events.Message` com `UnavailableRequestID` igual ao `request_id` devolvido (`capabilities/message/history_sync.go:250`), legivel por `GET /chats/history` no `data_json` e pelo webhook/`/session/ws`. Falta a PRE-CONDICAO: uma mensagem genuinamente indecifravel, que nao e criavel por HTTP. Ver `OBSERVADORES-AMBAR.md` §1. |
| `POST /newsletters/mark-viewed` | 200 com data:null. **INVESTIGADO A FUNDO em 2026-08-28, com um listener que PROVOU funcionar**: o mesmo WebSocket que recebeu, em segundos, o evento `NewsletterLiveUpdate` de uma reação de `recebe`, esperou 45s por um evento depois de `mark-viewed` — zero. Mais forte: `POST /newsletters/messages` (sem WebSocket nenhum) confirmou `view_count:0` antes e depois, enquanto `reactions` no MESMO objeto mostrava a contagem real. Não é bug de entrega — a entrega funciona, provado. É o `view_count` nunca incrementar do lado do WhatsApp para uma marcação feita por API, possivelmente por exigir renderização por cliente real (hipótese, não confirmável sem o código deles). **Não há ação humana nem de código nesta base que destrave isto.** Ver `OBSERVADORES-AMBAR.md` §4. |
| `POST /status/set/video` | **MEDIÇÃO PRÓPRIA feita em 2026-08-28** (não herdada) — pré-condição de `FullName` satisfeita (ver "Destrave de 2026-08-28" abaixo). `200`, `envia` grava o `videoMessage` completo na própria história. Mas o WebSocket de `recebe`, em DUAS janelas de 90s (reproduzido), só recebeu o `senderKeyDistributionMessage` (preâmbulo Signal) — nunca o `videoMessage`. Achado incidental **F358**: diferente de `image`, que entregou completo em segundos. |
| `POST /status/set/audio` | **MEDIÇÃO PRÓPRIA feita em 2026-08-28** — mesmo padrão de `/status/set/video`: `200`, mas o WebSocket de `recebe` (90s) só recebeu o preâmbulo, nunca o `audioMessage`. Mesmo achado incidental **F358**. |

## Nenhuma rota fica por testar

As oito que ainda restavam saíram desta lista em 2026-08-28:
`POST /chats/download/{kind}`; `POST /users/privacy` e
`POST /users/status` (permissão explícita do usuário, "sim, pode fazer no
envia", ciclo completo mudar→confirmar→reverter); `POST
/session/pair/phone` (terceiro número descartável fornecido pelo
usuário); `POST /s3/test` e `POST /session/s3/test` (bucket B2 real,
chave dedicada isolada das buckets de produção do usuário); `POST
/call/reject` (chamada real do usuário, capturada e recusada ao vivo); e
`POST /users/avatar`, a última — que nem precisou de permissão nenhuma no
fim, porque a premissa que a mantinha na lista estava ERRADA (ver
"Destrave de 2026-08-28" abaixo, F363).

**Todas as 137 rotas documentadas já foram exercitadas pelo menos uma
vez.** O que resta como 🟡 (4) e ❌ (4) tem causa determinada — não é
"nunca medido", é "medido, e é isto que acontece".

## Destrave de 2026-08-28

A pedido do usuário ("vamos destravar esses que não precise da minha ação
humana", depois "vamos seguir com os próximos que posso estar ajudando"),
catorze rotas foram re-testadas ao vivo (`envia`/`recebe`, uma sessão
descartável nova pareada com um terceiro número real, um bucket B2 real
do usuário, e uma chamada de voz real do usuário), doze delas movidas
para ✅ — fechando as 137 rotas do contrato:

- **`POST /chats/download/{kind}`** → ✅. Chamada com os sete campos de uma
  mensagem de imagem real já em `GET /chats/history`: `200`, imagem
  decifrada corretamente.
- **`POST /groups/{group_jid}/join-requests`** → ✅. A evidência já existia
  desde a Fase 6 (F350) na linha irmã `GET .../join-requests`, mas esta
  linha (o `POST` de decisão) tinha ficado com a marca antiga por um lapso
  da campanha — corrigido, sem nova medição necessária.
- **`POST /newsletters/react`** → ✅. Canal descartável, `recebe` reage
  👍 a uma mensagem de `envia`: `reactions` passa de `[]` para
  `[{emoji:"👍",count:1}]` e volta a `[]` ao remover — os três estados
  fecham a causalidade.
- **`POST /chats/send/sticker`** → ✅ (F357). Bloqueado inicialmente por
  `ffmpeg` local quebrado (`libx265.215.dylib` em falta); depois de
  `brew reinstall ffmpeg` (autorizado pelo usuário), um segundo bloqueio
  apareceu — o `ffmpeg` padrão do Homebrew não inclui o encoder `libwebp`
  — resolvido com `brew install ffmpeg-full` (47 dependências extras,
  keg-only, `brew unlink ffmpeg && brew link ffmpeg-full`, autorizado pelo
  usuário). Com PNG 512×512 real: `200`, `GET /chats/history` + `POST
  /chats/download/sticker` confirmaram bytes `RIFF`…`WEBP` válidos de
  512×512.
- **`POST /newsletters/mark-viewed`** → continua 🟡, mas agora com causa
  DEFINITIVA (ver tabela acima): a entrega por WebSocket foi provada
  funcional com um evento real de reação, e mesmo assim `mark-viewed` não
  produz nem esse evento nem uma mudança de `view_count` observável por
  polling. Não é um "ainda não investigado" — é "investigado, e o efeito
  genuinamente não existe do lado do WhatsApp para este caminho".
- **`POST /status/set/image`** → ✅. O usuário salvou `recebe` como
  contacto com nome no telefone de `envia` (pré-condição da F256,
  confirmada via `GET /users/contacts`: `full_name` preenchido). Publicado
  um JPEG real: `200`, e o WebSocket de `recebe` recebeu o `imageMessage`
  completo em segundos — `mimetype`, `caption` e `message_id` batendo.
- **`POST /status/set/video`** e **`POST /status/set/audio`** → continuam
  🟡, mas com medição própria feita e um achado incidental novo (**F358**):
  a mesma pré-condição de `image` foi satisfeita, `200` em ambas, `envia`
  grava o conteúdo completo na própria história — mas o WebSocket de
  `recebe`, testado em janelas de 90s (reproduzido para vídeo), só recebeu
  o `senderKeyDistributionMessage` (o preâmbulo do protocolo Signal), nunca
  o `videoMessage`/`audioMessage` em si. Diferente de `image`, que entregou
  completo. Não é falta de pré-condição — é a mesma pré-condição, resultado
  diferente conforme o tipo de mídia.
- **`POST /users/privacy`** → ✅. Permissão explícita do usuário ("sim,
  pode fazer no envia"). `readreceipts` (estado inicial `all`) → `none` via
  `POST`, confirmado por `GET /users/privacy`; revertido para `all` no
  mesmo ciclo, confirmado de novo. A conta não ficou alterada ao final.
- **`POST /users/status`** → ✅. Mesma permissão. Recado original `"conta
  de testes"` → texto de teste via `POST`, confirmado por
  `GET /session/profile/full` (`user_info[0].status`); revertido ao texto
  original no mesmo ciclo, confirmado de novo.
- **`POST /session/pair/phone`** → ✅. Usuário forneceu um TERCEIRO número
  descartável (não registrado neste documento, é dado pessoal). Sessão
  nova criada só para o teste, `GET /session/connect` +
  `POST /session/pair/phone` → `linking_code`. Primeiro código expirou
  (janela ~2min) antes do usuário digitar; segundo código digitado a
  tempo — `GET /session/status` confirmou `connected:true,
  logged_in:true`, `jid` batendo com o número. Sessão desconectada e
  apagada ao final; `envia`/`recebe` intactas.
- **`POST /s3/test`** e **`POST /session/s3/test`** → ✅. Usuário
  forneceu um bucket B2 (Backblaze) real, com chave dedicada isolada das
  buckets de produção. `POST /s3/config` com endpoint/bucket/credenciais
  reais → `200`; `POST /s3/test` → `200 {connected:true, details:"S3
  connection test successful", bucket:"...", region:"us-west-004"}` —
  conexão real confirmada. `GET /session/s3/config` já refletia a mesma
  configuração antes de eu tocar nela, confirmando que `/session/s3/*` e
  `/s3/*` são o mesmo manipulador; `POST /session/s3/test` devolveu o
  mesmo resultado. Configuração removida ao final (`DELETE /s3/config`),
  sem deixar credenciais reais gravadas.
- **`POST /call/reject`** → ✅. Usuário fez uma chamada de voz real para
  `recebe`. Um listener automático no `/session/ws` de `recebe` capturou
  o evento `CallOffer` (`From`/`CallID` reais) e disparou
  `POST /call/reject` dentro da janela — a rota exige isso, o `call_id`
  não é inventável. `200 {"details":"Call rejected","call_id":"..."}`,
  com o `call_id` batendo exatamente com o do evento capturado.
- **`POST /users/avatar`** → ✅ (F363). Ao investigar como testar com
  permissão do usuário, descobri que a premissa que a mantinha na lista
  ("alteraria o avatar da conta") estava **errada**: é a mesma rota que
  `GET /users/avatar` documenta como leitura (`GetAvatarUseCase`,
  `pkg/application/usecase/user/get_avatar.go` — sem nenhum caminho de
  escrita), e o próprio ficheiro OpenAPI já dizia "Esta rota LÊ". Chamada
  com o número de `envia`: `200 {id:"214830039", url:...}` — o `id` bate
  exatamente com `avatar_id` de `GET /session/profile`. Chamada com o
  número de `recebe`: `403 forbidden` (foto escondida por privacidade,
  comportamento documentado). Nenhuma conta foi alterada — a rota nunca
  precisou de permissão nenhuma, só de alguém ler o código em vez de
  herdar a suposição.

<!-- GERADO:TABELA-COMPLETA -->
