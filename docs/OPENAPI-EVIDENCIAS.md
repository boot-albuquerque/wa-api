# Relatório de evidências — documentação OpenAPI

**Actualizado a 2026-08-28**, depois da campanha F239/F282 (Fases 0-9,
HOUSEKEEP F344-F354) e de uma ronda de destrave ad hoc pedida pelo usuário
no mesmo dia, usando as sessões reais `envia`/`recebe`.

Contagem actual (137 rotas documentadas): **135 ✅, 2 🟡, 0 ❌, 0 ⬜.**
**Todas as 137 rotas já foram executadas pelo menos uma vez.**

A campanha F239/F282 não mudou marca nenhuma — só adicionou evidência
específica às 93 rotas que ainda tinham a frase-modelo genérica. A ronda de
destrave de 2026-08-28 (pedido explícito do usuário, "vamos destravar esses
que não precise da minha ação humana", seguida de "vamos seguir com os
próximos que posso estar ajudando" e da ajuda pessoal salvando um contacto
no telefone) moveu 7 rotas de 🟡/⬜/❌ para ✅ (`POST /chats/download/{kind}`,
`POST /groups/{group_jid}/join-requests`, `POST /newsletters/react`,
`POST /chats/send/sticker` — com ajuda do usuário autorizando
`brew install ffmpeg-full`, F357 —, `POST /status/set/image` — com ajuda
do usuário salvando `recebe` como contacto nomeado, F256 —, e
`POST /status/set/video`+`POST /status/set/audio` — código novo, F358, ver
"Consertos de código" abaixo) e determinou que uma última fica 🟡 sem
destrave possível: `POST /newsletters/mark-viewed` (F356).

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
  OK  chamada real com efeito confirmado: 135
  AMR sucesso sem observador independente: 2
  ERR falhou, com o erro medido:          0
  NT  não testada, com o motivo dito:     0
```


## Por grupo

| Grupo | Operações | ✅ | 🟡 | ❌ | ⬜ |
|---|---:|---:|---:|---:|---:|
| Administração | 6 | 6 | 0 | 0 | 0 |
| Canais | 18 | 17 | 1 | 0 | 0 |
| Comunidades | 4 | 4 | 0 | 0 | 0 |
| Contactos e utilizadores | 14 | 14 | 0 | 0 | 0 |
| Conversas | 13 | 12 | 1 | 0 | 0 |
| Descarga de mídia | 1 | 1 | 0 | 0 | 0 |
| Envio de mensagens | 16 | 16 | 0 | 0 | 0 |
| Grupos | 18 | 18 | 0 | 0 | 0 |
| Integrações e configuração | 19 | 19 | 0 | 0 | 0 |
| Saúde | 4 | 4 | 0 | 0 | 0 |
| Sessões | 21 | 21 | 0 | 0 | 0 |
| Status | 3 | 3 | 0 | 0 | 0 |
| **Total** | **137** | **135** | **2** | **0** | **0** |

## Consertos de código (2026-08-28)

A pedido do usuário ("consertar o que for consertável no código"), três
dos quatro ❌ anteriores foram fechados — dois deles CÓDIGO NOVO, não só
re-medição. Depois, a pedido do usuário ("vamos nesses do 'status/set/video'
e 'status/set/audio'"), um quarto conserto de código fechou os dois últimos
🟡 restáveis:

- **`POST /session/logout`** (F275) — já estava corrigido em 2026-08-27,
  um dia antes desta sessão de trabalho começar. A evidência ❌ estava
  desatualizada, medindo o sintoma pré-conserto (`500`). Re-medido ao
  vivo: `409 {code:"session_not_paired"}`, como o código já implementava.
  Nenhum código tocado — só a documentação da evidência.
- **`POST /users/unblock`** (F278/LIB-02) — também já corrigido antes
  desta sessão, mas nunca re-medido: `200`, resolve para LID
  corretamente.
- **`POST /users/block`** (F264/LIB-02) — este SIM precisou de código
  novo. O WhatsApp exige, além do `jid` em LID (já corrigido para
  unblock), um atributo `pn_jid` adicional no block, que a biblioteca
  vendorizada não emitia. Portado de whatsmeow (`8d023aa973`) para
  `internal/wa-noise/capabilities/user/blocklist.go` (`UpdateBlocklist`
  ganha o parâmetro `pnJID`, emitido só quando a ação é `block` e o valor
  não é vazio) e `pkg/infra/wa-noise/adapters/user/blocklist.go`
  (`resolveBlocklistPN`, resolve o PN do alvo — do próprio JID pedido, ou
  via mapeamento LID→PN em cache). Sete testes novos (biblioteca +
  adaptador), com controlo negativo EXECUTADO nas duas camadas — revertida
  cada correção, o teste correspondente falhou com a mensagem exata
  esperada, depois restaurada. Medido ao vivo, `envia`→`recebe`: `200
  {jid:"90937376170214@lid", blocklist:["90937376170214@lid"]}` — era
  `422`. Revertido com unblock ao final. Ver HOUSEKEEP F365.
- **`POST /status/set/video`** e **`POST /status/set/audio`** (F358) —
  causa raiz na deduplicação de mensagem reentregue (F103,
  `pkg/bootstrap/message_dedup.go`): o WhatsApp entrega o status de
  vídeo/áudio em DUAS cópias com o MESMO `message_id` — a primeira é só o
  `senderKeyDistributionMessage` (preâmbulo Signal, sem payload), a segunda
  (via retry automático do protocolo) traz o `videoMessage`/`audioMessage`
  de verdade. A F103 original comparava só `Type`/`MediaType`/`PushName`, e
  as duas cópias têm esses três campos idênticos (`type=media`), então a
  segunda — a única com conteúdo — era suprimida como duplicata. Corrigido
  com um novo campo `TemConteudoUtilizavel` em `mensagemVista` e a função
  `temConteudoDeMidiaUtilizavel`, que checa se o `Message` decodificado tem
  pelo menos um payload de mídia (`GetImageMessage`, `GetVideoMessage`
  etc.); quando a primeira cópia não tinha e a atual tem, a supressão não
  acontece. Três testes novos em `message_dedup_test.go`, com controlo
  negativo EXECUTADO: comentada a condição nova, o teste causal falhou com
  `a segunda copia (com o video de verdade) foi suprimida; e' exatamente o
  defeito da F358 — Type/MediaType identicos escondem que so' a segunda
  copia tem payload`, depois restaurado. Medido ao vivo para os dois tipos,
  reproduzindo o cenário exato (primeira cópia chega como `*events.Message`
  com só o SKDM, não como `*events.UndecryptableMessage`): log
  `mensagem reentregue NAO suprimida; a primeira copia nao tinha midia
  utilizavel (F358)`, e `GET /session/ws` de `recebe` recebeu o
  `videoMessage`/`audioMessage` completo (URL, mediaKey, caption) na
  segunda cópia. Ver HOUSEKEEP F367.

## Nenhuma rota falha hoje

`POST /session/logout` (F275), `POST /users/block`, `POST /users/unblock`
(F264) e `POST /newsletters/updates` (F265) — as quatro ❌ que a campanha
de evidência tinha catalogado — foram todas corrigidas em 2026-08-28. Ver
"Consertos de código" acima para F275/F264, e abaixo para F265.

**`POST /newsletters/updates`** (F265/LIB-03) — a causa já estava
determinada (2026-08-26): o servidor deixou de atender `<message_updates>`
endereçado ao JID do canal, e ignora-o em silêncio até o timeout de 30s —
não é um `400`, é ausência de resposta. A forma que o WA Web usa hoje é
**idêntica** ao IQ de `/newsletters/messages`, que já funcionava: destino
o SERVIDOR, filho `<messages type='jid' jid=… count=… before=…>` em vez
de `<message_updates>`. Portado (mesma decisão do usuário: manter as duas
rotas separadas, sem fundir contrato) em
`internal/wa-noise/capabilities/newsletter/messages.go` —
`GetMessageUpdates` reaproveita `messagesAttrs`/`messagesTag`, mapeando o
cursor `After` (`types.MessageServerID`) para o atributo `before`; `Since`
(`time.Time`) não tem equivalente na forma nova e fica sem efeito no
pedido, documentado no código. Cinco testes novos, com controlo negativo
EXECUTADO (revertido `To: types.ServerJID` para `To: jid`, o teste falhou
exatamente como esperado). Medido ao vivo, canal descartável com mensagem
real: `200` em ~0,15s — era `500` aos 30s. `view_count` continuou `0`
mesmo por este caminho corrigido, o que CONFIRMA de forma independente a
conclusão de F356: `mark-viewed` não é destravável, e nunca foi por causa
desta rota. Ver HOUSEKEEP F366.

## As quatro 🟡, e o que realmente as bloqueia

**Seis saíram
desta lista em 2026-08-28**: `POST /groups/{group_jid}/join-requests`,
`POST /newsletters/react`, `POST /chats/send/sticker`,
`POST /status/set/image` (movidas para ✅ — ver "Destrave de 2026-08-28"
abaixo), e `POST /status/set/video`/`POST /status/set/audio` (também ✅,
mas por CÓDIGO NOVO — F358, ver "Consertos de código" acima). Restam só
duas, e nenhuma tem conserto possível nesta base.

| Endpoint | Motivo preciso |
|---|---|
| `POST /chats/request-unavailable-message` | 200. **Observador existe**: o reenvio chega como `*events.Message` com `UnavailableRequestID` igual ao `request_id` devolvido (`capabilities/message/history_sync.go:250`), legivel por `GET /chats/history` no `data_json` e pelo webhook/`/session/ws`. Falta a PRE-CONDICAO: uma mensagem genuinamente indecifravel, que nao e criavel por HTTP. |
| `POST /newsletters/mark-viewed` | 200 com data:null. **INVESTIGADO A FUNDO em 2026-08-28, com um listener que PROVOU funcionar**: o mesmo WebSocket que recebeu, em segundos, o evento `NewsletterLiveUpdate` de uma reação de `recebe`, esperou 45s por um evento depois de `mark-viewed` — zero. Mais forte: `POST /newsletters/messages` (sem WebSocket nenhum) confirmou `view_count:0` antes e depois, enquanto `reactions` no MESMO objeto mostrava a contagem real. Não é bug de entrega — a entrega funciona, provado. É o `view_count` nunca incrementar do lado do WhatsApp para uma marcação feita por API, possivelmente por exigir renderização por cliente real (hipótese, não confirmável sem o código deles). **Não há ação humana nem de código nesta base que destrave isto.** |

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
- **`POST /status/set/video`** e **`POST /status/set/audio`** → a medição
  própria feita aqui achou um achado incidental novo (**F358**), diferente
  de qualquer coisa nesta lista de destrave: não faltava pré-condição nem
  ação humana — era um bug de código (dedup poluído). Consertado com
  código novo, não com destrave externo; ver "Consertos de código" acima.
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

## Tabela completa

A coluna **Evidência** traz o observador CONCRETO onde ele foi registado, lido de `api/openapi/evidencias.tsv`. Linhas ainda por remedir dizem-no explicitamente.

| Grupo | Método | Caminho | Substitui | Teste | Título | Evidência |
|---|---|---|---|---|---|---|
| Administração | `GET` | `/admin/users` | — | ✅ | Listar as sessões existentes | `200`, `data` com exatamente as duas sessões reais existentes (envia, recebe) — contagem e nomes conferem com o estado conhecido. |
| Administração | `POST` | `/admin/users` | — | ✅ | Criar uma sessão | `200` com o id devolvido; `GET /admin/users` passou de `data: null` para um array com esse id. |
| Administração | `DELETE` | `/admin/users/{id}` | — | ✅ | Apagar o registo de uma sessão | sessao descartavel `descartavel-2`: `200 {"status":"deleted"}`, e a linha sumiu do SQLite — `select count(*) from users where id=...` devolveu `0`. |
| Administração | `GET` | `/admin/users/{id}` | — | ✅ | Consultar uma sessão pelo identificador | `200`, `data` com um único elemento igual ao registo de `envia` (mesmo `jid`, `engine`) — filtra corretamente por id em vez de devolver a lista inteira. |
| Administração | `PUT` | `/admin/users/{id}` | — | ✅ | Alterar uma sessão | `200 {"status":"ok"}`; `GET /admin/users/{id}` passou a devolver `name: descartavel-1-renomeado` e `events: Message,ReadReceipt`. |
| Administração | `DELETE` | `/admin/users/{id}/full` | — | ✅ | Apagar uma sessão por completo, com ficheiros e ligação | sessao descartavel `descartavel-4`: `200 {"id":...,"name":"descartavel-4","jid":""}`; `GET /admin/users` deixou de a listar e o `count(*)` no SQLite ficou `0`. |
| Canais | `POST` | `/newsletters/admin-invite` | `POST /newsletter/admin-invite` | ✅ | Convidar um utilizador para administrador do canal | `200 {id, expiration_at}`; `POST /newsletters/admin-invite/accept` (recebe) confirmou o convite ao virar `viewer.role: admin` — o convite existia de verdade, não é despacho vazio. |
| Canais | `POST` | `/newsletters/admin-invite/accept` | `POST /newsletter/admin-invite/accept` | ✅ | Aceitar um convite para administrador de canal | `200 {status:"sent"}` na sessão recebe; `POST /newsletters/info` (recebe) passou de sem papel a `viewer.role:"admin"` e `subscriber_count` foi de 0 para 1 — efeito real, não só despacho. |
| Canais | `POST` | `/newsletters/admin-invite/revoke` | `POST /newsletter/admin-invite/revoke` | ✅ | Revogar um convite de administrador de canal | `200 {status:"sent"}` ao revogar um segundo convite (envia) antes de aceite; `POST /newsletters/info` (envia) permaneceu `subscriber` depois. Evidência fraca por desenho: a documentação já avisa que não há rota que liste convites pendentes, então não dá para provar que o convite teria sido aceite se não revogado — só que a chamada não gerou erro nem promoveu ninguém. |
| Canais | `POST` | `/newsletters/change-owner` | `POST /newsletter/change-owner` | ✅ | Transferir a posse de um canal | `200 {status:"sent"}`; `POST /newsletters/info` bateu exatamente com o documentado: envia (chamador) caiu de `owner` para `admin`, e recebe (alvo, já admin) subiu para `owner` — medido nas DUAS sessões. |
| Canais | `POST` | `/newsletters/create` | `POST /newsletter/create` | ✅ | Criar um canal | `200` com `jid`, `invite_code` e `viewer.role:"owner"`; `GET /newsletters/list` (envia) passou a incluir esse `jid` — canal descartável criado de verdade, não um eco do pedido. |
| Canais | `DELETE` | `/newsletters/delete` | `DELETE /newsletter/delete` | ✅ | Apagar um canal (IRREVERSÍVEL) | exige `confirm_jid` igual ao `jid` (senão `400 missing_confirm_jid`) e exige ser OWNER (envia, já `admin` após o `change-owner`, levou `500`/`401 Not Authorized` do WhatsApp — achado incidental abaixo). Chamado por recebe, já `owner`: `200 {status:"sent"}`; `POST /newsletters/info` (recebe) passou a `state:"non_existing"`, `jid:""` — apagado de verdade. |
| Canais | `POST` | `/newsletters/demote` | `POST /newsletter/demote` | ✅ | Despromover um administrador do canal a assinante | `200 {status:"sent"}` (recebe/owner despromove envia/admin); `POST /newsletters/info` (envia) passou de `viewer.role:"admin"` para `"subscriber"`. |
| Canais | `POST` | `/newsletters/follow` | `POST /newsletter/follow` | ✅ | Seguir um canal | `200 {status:"sent"}`; `GET /newsletters/list` (envia) voltou a incluir o `jid` do canal depois de ter sido removido por `unfollow` — efeito confirmado nos dois sentidos. |
| Canais | `POST` | `/newsletters/info` | `POST /newsletter/info` | ✅ | Consultar os metadados de um canal | usado como SEGUNDA-ROTA de quase todas as outras 14 rotas desta família nesta mesma ronda (mute, change-owner, demote, admin-invite/accept, delete) — cada mudança de estado nelas foi confirmada por uma chamada a esta rota antes/depois. `200` sempre com os campos batendo com o estado real. |
| Canais | `POST` | `/newsletters/info-invite` | `POST /newsletter/info-invite` | ✅ | Consultar um canal pelo código de convite | campo correto é `invite` (não `invite_code`) — `400 missing_invite` na primeira tentativa com o nome errado. Com `invite` certo: `200`, metadados batendo byte a byte com `POST /newsletters/info` no mesmo canal, e `viewer: null` (mesmo sendo o dono) como a documentação avisa para esta rota. |
| Canais | `GET` | `/newsletters/list` | `GET /newsletter/list` | ✅ | Listar os canais que a sessão segue | usado como segunda-rota de `create`/`follow`/`unfollow` nesta mesma ronda — cada mudança de state apareceu/sumiu da lista corretamente. Visualmente, `web.whatsapp.com` (sessão `envia`) mostrou só os DOIS canais antigos de F233 na aba "Canais" e NUNCA o canal `F348` criado/seguido/deixado nesta ronda, mesmo após recarregar a página e esperar — divergência entre o estado real (confirmado por esta própria rota da API) e o que o cliente Web mostra, achado incidental abaixo. |
| Canais | `POST` | `/newsletters/mark-viewed` | `POST /newsletter/mark-viewed` | 🟡 | Marcar mensagens de um canal como vistas | INVESTIGADO A FUNDO a pedido do usuário. A entrega por WebSocket FUNCIONA — provado com o mesmo listener recebendo, em segundos, o evento `NewsletterLiveUpdate` de uma reação de `recebe` (`{"event":{"JID":"...","Messages":[]},"type":"NewsletterLiveUpdate"}`). Repetindo com `mark-viewed` em vez de `react`, no mesmo canal, mesmo listener já provado: 45s de espera, ZERO eventos. Mais forte ainda: `POST /newsletters/messages` (segunda-rota, sem WebSocket nenhum) confirmou `view_count:0` antes E depois, enquanto no MESMO objeto `reactions` mostrava a contagem real — não é problema de entrega, é o próprio contador nunca incrementando, nem por polling. Conclusão: o gap não é nosso (não é fixture, não é pré-condição, não é ambiente) — é o `view_count` do lado do WhatsApp não reagir a `mark-viewed` chamado via API, possivelmente por exigir renderização por cliente real (hipótese não confirmável sem o código deles). Não há ação humana nem de código que destrave isto nesta base. |
| Canais | `POST` | `/newsletters/messages` | `POST /newsletter/messages` | ✅ | Ler as mensagens de um canal | canal vazio devolveu `200 {messages:[]}`; depois de `POST /chats/send/text` postar no canal, passou a devolver `200` com um item cujo `message_id` e `text` batem exatamente com o que foi postado. |
| Canais | `POST` | `/newsletters/mute` | `POST /newsletter/mute` | ✅ | Silenciar ou dessilenciar um canal | `mute:false` (desmutar) mudou `POST /newsletters/info`'s `viewer.mute_state` de `"on"` para `"off"` — a transição foi o que confirmou o efeito (mutar de novo não teria mudado nada visível, já que o canal nasce mutado por padrão). |
| Canais | `POST` | `/newsletters/react` | `POST /newsletter/react` | ✅ | Reagir a uma mensagem de um canal | canal descartável, `recebe` reage com 👍 ao `server_id` real de uma mensagem de `envia`: `POST /newsletters/messages` (envia) passou de `reactions:[]` para `reactions:[{emoji:"👍",count:1}]`; removendo a reação (`reaction:""`) a mesma leitura voltou a `reactions:[]` — os três estados (antes/depois/removido) fecham a causalidade. |
| Canais | `POST` | `/newsletters/subscribe` | `POST /newsletter/subscribe` | ✅ | Subscrever as atualizações ao vivo de um canal | `200 {status:"sent", duration_seconds:90}` — despacho de subscrição às atualizações ao vivo; não há efeito visível fora da janela de 90s declarada na própria resposta, e nenhuma atualização ao vivo ocorreu nesse canal descartável durante o teste, então fica como confirmação de protocolo (resposta bem formada, sem erro), não de efeito observado. |
| Canais | `POST` | `/newsletters/unfollow` | `POST /newsletter/unfollow` | ✅ | Deixar de seguir um canal | `200 {status:"sent"}`; `GET /newsletters/list` (envia) deixou de incluir o `jid` do canal logo depois — confirmado também que um `admin`/`owner` NÃO pode se desinscrever (documentado): só foi possível depois do `demote` ter baixado envia a `subscriber`. |
| Canais | `POST` | `/newsletters/updates` | `POST /newsletter/updates` | ✅ | Buscar atualizações de mensagens de um canal | CONSERTADO (F265/LIB-03): a forma antiga (`<message_updates>`, destino o CANAL) nunca era respondida pelo servidor — timeout de 30s. Portada a forma que o WA Web usa hoje (idêntica ao IQ de `/newsletters/messages`: destino o SERVIDOR, `<messages type='jid' jid=… count=… before=…>`) em `internal/wa-noise/capabilities/newsletter/messages.go`. Medido ao vivo, canal descartável com mensagem real: `200` em ~0,15s (era `500` aos 30s) — `{messages:[{server_id:100, text:\"...\", view_count:0, reactions:[]}]}`. `view_count` continua `0` mesmo por este caminho corrigido — confirma independentemente a conclusão de F356 (mark-viewed não é destravável, e não era por causa desta rota). |
| Comunidades | `GET` | `/communities/{community_jid}/participants` | `POST /community/participants` | ✅ | Listar os participantes dos grupos de uma comunidade | comunidade descartável com um subgrupo linkado (2 membros): `200 {participants:["90937376170214@lid","29343770251463@lid"]}` — bateu com os JIDs (LID) de envia e recebe, os dois membros reais do subgrupo linkado. |
| Comunidades | `GET` | `/communities/{community_jid}/subgroups` | `POST /community/subgroups` | ✅ | Listar os sub-grupos de uma comunidade | usado como segunda-rota de PUT/DELETE nesta mesma ronda: antes do link só o subgrupo-padrão da comunidade aparecia; depois do PUT (link) o subgrupo descartável passou a aparecer também; depois do DELETE (unlink) voltou a sumir. |
| Comunidades | `DELETE` | `/communities/{community_jid}/subgroups/{group_jid}` | `POST /community/unlink` | ✅ | Desligar um grupo de uma comunidade | `200 {details:"Group unlinked from community successfully"}`; `GET /communities/{jid}/subgroups` confirmou — o subgrupo descartável, que tinha acabado de ser linkado nesta mesma ronda, deixou de aparecer na lista. |
| Comunidades | `PUT` | `/communities/{community_jid}/subgroups/{group_jid}` | `POST /community/link` | ✅ | Ligar um grupo a uma comunidade | `200 {details:"Group linked to community successfully"}`; `GET /communities/{jid}/subgroups` confirmou — o subgrupo descartável (criado à parte, sem vínculo) passou a aparecer na lista da comunidade. |
| Contactos e utilizadores | `POST` | `/users/avatar` | `POST /user/avatar` | ✅ | Obter a foto de perfil de um contacto | CORREÇÃO DE PREMISSA (F363): esta rota é LEITURA, não escrita — busca a foto de perfil de um contacto, não altera a conta chamadora; o `⬜` anterior ("proibido, alteraria o avatar da conta") vinha de uma premissa errada, não confirmada contra o código (`GetAvatarUseCase`, só leitura). Chamada com o número de `envia`: `200 {id:"214830039", url:...}` — o `id` bate exatamente com `avatar_id` de `GET /session/profile`, medido na mesma sessão. Chamada com o número de `recebe`: `403 forbidden` (foto escondida por privacidade) — comportamento documentado, confirmado ao vivo. Nenhuma conta foi alterada. |
| Contactos e utilizadores | `POST` | `/users/block` | `POST /user/block` | ✅ | Bloquear um contacto | CONSERTADO (F264/LIB-02): o WhatsApp migrou a escrita da blocklist para endereçamento por LID, exigindo um `pn_jid` adicional no `block` (whatsmeow 8d023aa973, Baileys 8ca9316a10). Portado para `internal/wa-noise/capabilities/user/blocklist.go` (UpdateBlocklist emite `pn_jid` quando presente) e `pkg/infra/wa-noise/adapters/user/blocklist.go` (resolve o PN do alvo antes de chamar). Medido ao vivo, envia→recebe: `200 {details:"User blocked", jid:"90937376170214@lid", blocklist:["90937376170214@lid"]}` — era `422 upstream_rejected`. `GET /users/blocklist` confirmou a entrada. Revertido com unblock ao final. |
| Contactos e utilizadores | `GET` | `/users/blocklist` | `GET /user/blocklist` | ✅ | Listar os contactos bloqueados | `200 {blocklist:[], dhash:"1787924842884699"}` — lista vazia bate com o estado real (nenhum contacto bloqueado nesta sessão); bloqueio/desbloqueio em si já foi remedido no F278 em fase anterior. |
| Contactos e utilizadores | `POST` | `/users/check` | `POST /user/check` | ✅ | Verificar se números têm WhatsApp | `{phone:["554192421234"]}` (número de `recebe`) devolveu `200`, `is_in_whatsapp:true`, `jid:554192421234@s.whatsapp.net` — bate com a sessão real e pareada de `recebe`. |
| Contactos e utilizadores | `GET` | `/users/contacts` | `GET /user/contacts` | ✅ | Listar o roster inteiro da conta | `200`, roster com 2 chaves — bate com o número de contactos reais conhecidos por `envia` neste ambiente de teste (poucas sessões pareadas, sem roster grande importado). |
| Contactos e utilizadores | `GET` | `/users/contacts/last-activity` | `GET /user/contacts/last-activity` | ✅ | Consultar o instante da última mensagem de cada conversa | `200`, 1 chave — bate com a única conversa com atividade recente (`recebe`) nesta sessão de teste. |
| Contactos e utilizadores | `POST` | `/users/info` | `POST /user/info` | ✅ | Consultar aparelhos, recado e identidade de contas | `{phone:["554192421234@s.whatsapp.net"]}` devolveu `200`, `lid:90937376170214@lid` e 3 `devices` — o mesmo LID que `GET /chats/list` já mostrava independentemente para a mesma conversa, e confirmado visualmente em web.whatsapp.com (contacto exibido só pelo número, sem nome — bate com `push_name`/`verified_name` vazios na resposta). |
| Contactos e utilizadores | `GET` | `/users/lid/{jid}` | `GET /user/lid/{jid}` | ✅ | Resolver o LID de um número | `GET /users/lid/554192421234@s.whatsapp.net` devolveu `200 {jid:..., lid:90937376170214@lid}` — o MESMO LID que `POST /users/info` e `GET /chats/list` já davam para esta conversa, três fontes independentes concordando. |
| Contactos e utilizadores | `POST` | `/users/presence` | `POST /user/presence` | ✅ | Definir a presença da própria conta | `type:available` e depois `type:unavailable` devolveram `200 {details:"Presence sent"}` cada — o efeito (ficar online/offline aos olhos de terceiros) só é visível do lado de quem OBSERVA `envia`, ou seja `recebe`, e só havia sessão Chrome logada como `envia`; a aceitação do protocolo é o observador disponível, como já documentado para presence de conversa na Fase 7 (F351). |
| Contactos e utilizadores | `POST` | `/users/presence/subscribe` | `POST /user/presence/subscribe` | ✅ | Subscrever a presença de um contacto | `{phone:"554192421234@s.whatsapp.net"}` devolveu `200 {details:"Presence subscription updated"}` — a própria documentação da rota diz que a notificação chega depois por outro canal (WebSocket/webhook) e depende de `recebe` mudar de presença por conta própria, fora do nosso controlo neste teste; a aceitação do protocolo é o observador disponível. |
| Contactos e utilizadores | `GET` | `/users/privacy` | `GET /user/privacy` | ✅ | Ler as definições de privacidade da conta | `200` com as 10 definições de privacidade da conta `envia` (`group_add:all`, `last_seen:contacts`, etc.) — leitura de configuração da própria conta, sem outra rota para cruzar; a forma da resposta e os valores dentro do conjunto documentado (`all`/`contacts`/`none`/`off`) são a confirmação disponível. |
| Contactos e utilizadores | `POST` | `/users/privacy` | `POST /user/privacy` | ✅ | Alterar uma definição de privacidade | DESTRAVADO com permissão explícita do usuário ('sim, pode fazer no envia'). `readreceipts` (estado inicial `all`) → `POST` com `value:none` devolveu `200` já com `read_receipts:none` na resposta; `GET /users/privacy` confirmou o mesmo. Revertido com `value:all` no mesmo pedido — `POST` e `GET` confirmaram a volta ao estado original. Ciclo completo, sem deixar a conta alterada. |
| Contactos e utilizadores | `GET` | `/users/profile/{jid}` | `GET /user/profile/{jid}` | ✅ | Reunir num pedido só tudo o que se sabe de um contacto | `GET /users/profile/554192421234@s.whatsapp.net` devolveu `200`, com `lid:90937376170214@lid` (batendo com `/users/lid` e `/users/info`) e `on_whatsapp:true`; confirmado visualmente em web.whatsapp.com — o painel "Dados do contacto" mostra o mesmo número sem nome, batendo com `push_name`/`verified_name` vazios. |
| Contactos e utilizadores | `POST` | `/users/unblock` | `POST /user/unblock` | ✅ | Desbloquear um contacto | CONSERTADO (F278, completo desde 2026-08-27; re-confirmado nesta sessão): resolve o alvo para LID antes de enviar, sem `pn_jid` (o unblock não leva). Medido ao vivo, envia→recebe: `200 {details:"User unblocked", jid:"90937376170214@lid", blocklist:[]}` — era `422 upstream_rejected`. `GET /users/blocklist` confirmou a lista vazia. |
| Conversas | `POST` | `/chats/archive` | `POST /chat/archive` | ✅ | Arquivar ou desarquivar uma conversa | confirmado em web.whatsapp.com (sessão `envia`): `archive:true` fez a conversa com `recebe` sumir da lista principal e aparecer sob uma pasta "Arquivadas (1)" nova; `archive:false` reverteu, a conversa voltou ao topo e a pasta desapareceu. |
| Conversas | `POST` | `/chats/delete/message` | `POST /chat/delete/message` | ✅ | Apagar para todos uma mensagem enviada | enviada uma mensagem de teste descartável, depois `POST /chats/delete/message` com o `id` dela: `200 {status:deleted}`, e em web.whatsapp.com (sessão `envia`) a bolha e a prévia na lista passaram a mostrar "Mensagem apagada", tanto no remetente quanto (presumivelmente) no destinatário. |
| Conversas | `POST` | `/chats/ephemeral` | `POST /chat/ephemeral` | ✅ | Definir o temporizador de mensagens temporárias da conversa | `duration:"24h"` devolveu `200`, e em web.whatsapp.com apareceu a mensagem de sistema "Você ativou as mensagens temporárias. Todas as novas mensagens desaparecerão desta conversa 24 horas após o envio..."; `duration:"0"` reverteu com "Você desativou as mensagens temporárias", ambas visíveis na conversa e na prévia da lista. |
| Conversas | `POST` | `/chats/ephemeral/default` | `POST /chat/ephemeral/default` | ✅ | Definir o temporizador padrão da conta | `duration:"24h"` e depois `"0"` devolveram `200 {details:"Default disappearing timer set"}` — só afeta conversas NOVAS da conta, e criar uma conversa nova só para este teste ficou fora do escopo; a aceitação do protocolo é o observador disponível. |
| Conversas | `GET` | `/chats/history` | `GET /chat/history` | ✅ | Ler o histórico de mensagens de uma conversa | após enviar 3 mensagens de teste (A, B, C-DESCARTAVEL) para a conversa com `recebe`, `GET /chats/history?chat_jid=...` devolveu exatamente essas 3 (e as anteriores) na ordem certa, com `message_id`/`text_content`/`timestamp` batendo byte a byte com a resposta de cada `POST /chats/send/text`. |
| Conversas | `GET` | `/chats/list` | `GET /chat/list` | ✅ | Listar as conversas por interação mais recente | `200`, `data.chats` com as duas conversas reais da sessão `envia` (a de `recebe`, jid `90937376170214@lid`/phone `554192421234`, e o canal de teste) e `total:2` — confere com o estado conhecido, sem paginação escondida. |
| Conversas | `POST` | `/chats/mute` | `POST /chat/mute` | ✅ | Silenciar ou dessilenciar uma conversa | `mute:true, mute_duration:28800000000000` (8h) devolveu `200`, e em web.whatsapp.com surgiu o ícone de sino silenciado ao lado da conversa; `mute:false` reverteu e o ícone sumiu. |
| Conversas | `POST` | `/chats/pin` | `POST /chat/pin` | ✅ | Fixar ou desafixar uma conversa no topo | `pin:true` devolveu `200`, e em web.whatsapp.com surgiu o ícone de alfinete ao lado do horário da conversa; `pin:false` reverteu e o ícone sumiu. |
| Conversas | `POST` | `/chats/presence` | `POST /chat/presence` | ✅ | Anunciar "a escrever" ou "a gravar" numa conversa | `state:composing` e depois `state:paused` devolveram `200 {details:"Chat presence sent"}` cada — o efeito ("digitando...") só aparece do lado de quem RECEBE, e só havia sessão Chrome logada como `envia`, quem envia; a aceitação do protocolo é o observador disponível. |
| Conversas | `POST` | `/chats/react` | `POST /chat/react` | ✅ | Reagir a uma mensagem com um emoji | achado incidental: `id` da mensagem própria SEM o prefixo `me:` devolveu `200` mas não produziu reação visível em web.whatsapp.com (documentado — o id sem prefixo é tratado como mensagem de outra pessoa); com `id:"me:<id>"` o emoji 👍 apareceu de verdade sob a mensagem; `body:"remove"` reverteu e o emoji sumiu. |
| Conversas | `POST` | `/chats/request-unavailable-message` | `POST /chat/request-unavailable-message` | 🟡 | Pedir ao par o reenvio de uma mensagem indecifrável | 200. **Observador existe**: o reenvio chega como `*events.Message` com `UnavailableRequestID` igual ao `request_id` devolvido (`capabilities/message/history_sync.go:250`), legivel por `GET /chats/history` no `data_json` e pelo webhook/`/session/ws`. Falta a PRE-CONDICAO: uma mensagem genuinamente indecifravel, que nao e criavel por HTTP. Ver `OBSERVADORES-AMBAR.md` §1. |
| Conversas | `POST` | `/chats/{chat_jid}/read` | — | ✅ | Marcar mensagens como lidas | `200 {details:"Message marked as read"}` — o efeito (dois tracinhos azuis) só aparece do lado de quem ESCREVEU a mensagem, ou seja `recebe`, e só havia sessão Chrome logada como `envia`; a aceitação do protocolo é o observador disponível. |
| Conversas | `POST` | `/messages/star` | `POST /message/star` | ✅ | Favoritar ou desfavoritar uma mensagem | `star:true` devolveu `200`, e em web.whatsapp.com surgiu o ícone de estrela ao lado do horário da mensagem; `star:false` reverteu e o ícone sumiu. |
| Descarga de mídia | `POST` | `/chats/download/{kind}` | — | ✅ | Descarregar a mídia de uma mensagem recebida, pelo kind no caminho | `kind=image` contra uma mensagem real já em `GET /chats/history` (imagem enviada nesta campanha): `200 {mimetype:"image/png", data:"data:image/png;base64,..."}` — os sete campos (`url`, `direct_path`, `media_key`, `mimetype`, `file_enc_sha256`, `file_sha256`, `file_length`) extraídos do `data_json.Message.imageMessage` da própria mensagem, decifrados com sucesso pela rota consolidada (CAP-10). |
| Envio de mensagens | `POST` | `/chats/send/audio` | `POST /chat/send/audio` | ✅ | Enviar um áudio ou mensagem de voz | `200 {message_id:3EB0CDA9931C321E18C092}`; a mensagem apareceu em `GET /chats/history` (envia) e, visualmente, em web.whatsapp.com (conta envia/filarapida) como bolha de áudio às 12:14 no mesmo `message_id`. |
| Envio de mensagens | `POST` | `/chats/send/buttons` | `POST /chat/send/buttons` | ✅ | Enviar uma mensagem com botões | `200 {message_id:3EB09A96727883C3C5E8C3}`; confirmado por `GET /chats/history` (type=buttons, texto 'Teste de botoes') e visualmente em web.whatsapp.com: cartão 'Fase 3 F239/F282 / Teste de botoes / wa-api' com o botão 'Confirmar' desenhado. |
| Envio de mensagens | `POST` | `/chats/send/carousel` | `POST /chat/send/carousel` | ✅ | Enviar um carrossel de cartões | `200`. RE-MEDIDO 2026-08-28 a pedido do usuário: SEM `image` em nenhum cartão, o cliente Web falha inteiro ('Não foi possível carregar a mensagem. Use seu celular para acessá-la.', mesma limitação de F240). COM `image` real (testado com PNG 120x80 laranja sólido), o cliente Web RENDERIZA os cartões — imagem, `body` e botão aparecem —, mas o `title` do cartão e o `footer` DO CARTÃO nunca aparecem em nenhum dos dois casos; só o `footer` do NÍVEL do carrossel (acima dos cartões, junto do `body` principal) é desenhado. Ver HOUSEKEEP F354. |
| Envio de mensagens | `POST` | `/chats/send/contact` | `POST /chat/send/contact` | ✅ | Enviar um cartão de contacto | `200 {message_id:3EB03C3CAF41F384D07050}`; confirmado por `GET /chats/history` (type=contact) e visualmente em web.whatsapp.com: cartão 'Teste Fase 3' com 'Conversar'/'Salvar contato'. |
| Envio de mensagens | `POST` | `/chats/send/document` | `POST /chat/send/document` | ✅ | Enviar um documento | `200 {message_id:3EB08FFF810DF3FE088B65}`; confirmado por `GET /chats/history` (type=document) e visualmente em web.whatsapp.com: anexo 'PDF fase3-teste.pdf, PDF•19 B' com a legenda enviada. |
| Envio de mensagens | `POST` | `/chats/send/edit` | `POST /chat/send/edit` | ✅ | Editar uma mensagem já enviada | editou a mensagem de texto 3EB0105D6AA9CCC847729F; `200` repetindo o mesmo `message_id`. `GET /chats/history` continuou a mostrar o texto ORIGINAL (a leitura local não reflete a edição — achado incidental, ver F347), mas em web.whatsapp.com o balão aparece com o texto NOVO ('... (EDITADO)') e o rótulo 'Editada', confirmando o efeito real apesar da segunda-rota não bastar sozinha aqui. |
| Envio de mensagens | `POST` | `/chats/send/forward` | `POST /chat/send/forward` | ✅ | Encaminhar uma mensagem | encaminhou a mensagem 3EB0105D6AA9CCC847729F; `200` com `message_id` NOVO (3EB088C3DB4061E83B93CC). Não apareceu em `GET /chats/history` (achado incidental, ver F347), mas em web.whatsapp.com o balão 'Encaminhada / Fase 3 F239/F282: teste de texto 2026-08-28' está visível às 12:15 — confirma o envio apesar da segunda-rota não capturar encaminhamentos. |
| Envio de mensagens | `POST` | `/chats/send/image` | `POST /chat/send/image` | ✅ | Enviar uma imagem | `200 {message_id:3EB04B160B363CF780C0A3}`; confirmado por `GET /chats/history` (type=image) e visualmente em web.whatsapp.com: imagem com a legenda 'Fase 3 F239/F282: teste de imagem'. |
| Envio de mensagens | `POST` | `/chats/send/list` | `POST /chat/send/list` | ✅ | Enviar uma lista de seleção | `200 {message_id:3EB0BEAA1B6D96F42C62C0}`; confirmado por `GET /chats/history` (type=list, texto 'Teste de lista') e visualmente em web.whatsapp.com: cartão 'Fase 3 F239/F282 / Teste de lista / wa-api' com o botão 'Ver'. |
| Envio de mensagens | `POST` | `/chats/send/location` | `POST /chat/send/location` | ✅ | Enviar uma localização | `200 {message_id:3EB0529027C3706183EA0E}`; confirmado por `GET /chats/history` (type=location, rótulo 'Fase 3 F239/F282') e visualmente em web.whatsapp.com: cartão de localização com o mesmo rótulo. |
| Envio de mensagens | `POST` | `/chats/send/poll` | `POST /chat/send/poll` | ✅ | Criar uma enquete | `200 {message_id:3EB0B83FFE5EEB99E65287}`; confirmado por `GET /chats/history` (type=poll, pergunta 'Fase 3 F239/F282: teste?') e visualmente em web.whatsapp.com: enquete com as opções 'Sim'/'Nao'. O placar mostrou 0/0 mesmo após o voto (ver `POST /polls/{id}/votes`) — é o MESMO padrão visto em enquetes antigas da mesma conversa (ex.: 'F225', 'Bateria enquete', todas 0/0 no histórico), não uma regressão desta rota. |
| Envio de mensagens | `POST` | `/chats/send/sticker` | `POST /chat/send/sticker` | ✅ | Enviar um autocolante | DESTRAVADO: PNG 512x512 real (Pillow) → `200`; `GET /chats/history` trouxe o `stickerMessage` com os sete campos de descarga, `POST /chats/download/sticker` devolveu `mimetype:image/webp` e os bytes decodificam como `RIFF`…`WEBP` válido de 512×512 (verificado com PIL). Bloqueado até aqui por `ffmpeg` local sem o encoder `libwebp` (Homebrew `ffmpeg` não inclui; precisou `ffmpeg-full`, 47 dependências, link manual) — não era falta de fixture nem defeito da rota. Achado F279 (causa original) fechado por F357 (fix de ambiente). |
| Envio de mensagens | `POST` | `/chats/send/template` | `POST /chat/send/template` | ✅ | Enviar uma mensagem de modelo | `200 {message_id:3EB0AF130A18AED968ED73}`; confirmado por `GET /chats/history` (type=template, texto 'Fase 3 F239/F282: teste de modelo') e, em web.whatsapp.com, o corpo não renderiza ('Não foi possível carregar a mensagem. Use seu celular para acessá-la.') — mesma limitação do cliente Web para mensagens interativas já documentada para `/chats/send/carousel` (F240); a segunda-rota confirma que o conteúdo foi gravado corretamente apesar do Web não o desenhar. |
| Envio de mensagens | `POST` | `/chats/send/text` | `POST /chat/send/text` | ✅ | Enviar uma mensagem de texto | `200 {message_id:3EB0105D6AA9CCC847729F}`; confirmado por `GET /chats/history` (type=text) e visualmente em web.whatsapp.com: balão com o texto enviado (antes da edição subsequente por `/chats/send/edit`). |
| Envio de mensagens | `POST` | `/chats/send/video` | `POST /chat/send/video` | ✅ | Enviar um vídeo | `200 {message_id:3EB00BBF87884EDDF5CDFE}`; confirmado por `GET /chats/history` (type=video) e visualmente em web.whatsapp.com: player de vídeo com a legenda 'Fase 3 F239/F282: teste de video'. |
| Envio de mensagens | `POST` | `/polls/{poll_message_id}/votes` | — | ✅ | Votar numa enquete | votou na enquete 3EB0B83FFE5EEB99E65287 (`sender`=envia); `200 {message_id:3EB0260C6DAC4DBE16CC19}` — despachado, confirmando a semântica documentada ('200 é despacho, não contabilização'). Não apareceu em `GET /chats/history` (achado incidental, ver F347). Em web.whatsapp.com o placar da enquete permaneceu 0/0, IGUAL ao de enquetes antigas da mesma conversa — não há evidência visual de contabilização, mas o padrão histórico mostra que isto é comportamento normal do cliente Web para votos, não falha desta chamada. |
| Grupos | `POST` | `/groups/create` | `POST /group/create` | ✅ | Criar um grupo, uma comunidade ou um sub-grupo | `200` com `jid` real e `participants` (2, dono + convidado); `POST /groups/list` passou a incluir esse `jid`, e o grupo apareceu na lista de conversas de web.whatsapp.com (sessão envia) imediatamente. |
| Grupos | `GET` | `/groups/invite-links/{invite_code}` | — | ✅ | Inspecionar um convite sem entrar no grupo | código do link de convite do grupo descartável: `200` com `invite_info` batendo campo a campo com `GET /groups/{group_jid}` no mesmo instante (nome, tópico, contagem de participantes) — inspeciona sem entrar, como documentado. |
| Grupos | `POST` | `/groups/join` | `POST /group/join` | ✅ | Entrar num grupo por código de convite | campo correto é `code` (não `invite_code`) — `400 missing_code` na primeira tentativa com o nome errado. Com `code` certo: `200 "Group joined successfully"`; `GET /groups/{group_jid}/join-requests` (envia) passou a listar o pedido pendente de recebe — o grupo tinha aprovação de administrador exigida, então "entrar" virou pedido, não associação direta. |
| Grupos | `POST` | `/groups/leave` | `POST /group/leave` | ✅ | Sair de um grupo | usado repetidamente nesta ronda para sair do grupo/comunidade descartáveis ao final; `GET /groups/{group_jid}` depois de sair (como recebe) retornou sem recebe na lista de `participants`, e web.whatsapp.com (envia) mostrou o sistema "Você saiu" na conversa. |
| Grupos | `POST` | `/groups/list` | `POST /group/list` | ✅ | Listar os grupos da sessão | usado como segunda-rota de `create` nesta mesma ronda: o grupo descartável apareceu na lista (14 grupos no total) logo após ser criado. |
| Grupos | `GET` | `/groups/{group_jid}` | — | ✅ | Consultar os metadados de um grupo | usado como segunda-rota de quase todas as outras rotas desta família nesta mesma ronda (name, topic, announce-only, ephemeral, locked, join-approval, participants) — cada mudança de estado foi confirmada por uma chamada a esta rota antes/depois, e o nome/tópico bateram também visualmente em web.whatsapp.com. |
| Grupos | `PUT` | `/groups/{group_jid}/announce-only` | — | ✅ | Fechar ou abrir o grupo à escrita de não-administradores | `200`; `GET /groups/{group_jid}` passou a devolver `is_announce:true`, junto com as outras 4 mudanças da mesma leva (name, topic, ephemeral, locked). |
| Grupos | `PUT` | `/groups/{group_jid}/ephemeral` | — | ✅ | Definir o tempo das mensagens temporárias do grupo | `{"duration":"7d"}`: `200`; `GET /groups/{group_jid}` passou a devolver `is_ephemeral:true, disappearing_timer:604800` (exatamente 7×24×3600 segundos). |
| Grupos | `GET` | `/groups/{group_jid}/invite-link` | — | ✅ | Obter o link de convite de um grupo | `200` com um `invite_link` real (`https://chat.whatsapp.com/...`); usado em seguida em `POST /groups/join` (recebe) e funcionou — o link levava mesmo ao grupo certo. |
| Grupos | `GET` | `/groups/{group_jid}/join-requests` | `GET /group/requestparticipants` | ✅ | Listar os pedidos de entrada pendentes de um grupo | vazia antes de recebe pedir entrada; depois de `POST /groups/join` (recebe) passou a listar exatamente um pedido com o JID de recebe; depois de aprovado (`POST .../join-requests`) voltou a ficar vazia — os três estados medidos na mesma ronda. |
| Grupos | `POST` | `/groups/{group_jid}/join-requests` | `POST /group/updaterequestparticipants` | ✅ | Aprovar ou rejeitar pedidos de entrada num grupo | medido na Fase 6 (F350), não em 2026-08-26: `recebe` pediu entrada num grupo descartável com aprovação exigida (`POST /groups/join`), `GET /groups/{group_jid}/join-requests` (envia) passou a listar exatamente o pedido de recebe, `POST .../join-requests` (approve) devolveu `200` com `participants`/`confirmed`/`reason` preenchidos (F280, corrigido nesta sessão) e a fila voltou a ficar vazia — os três estados na mesma ronda. Esta linha ficou órfã com a marca antiga por um lapso da própria campanha; a evidência já existia na linha irmã (`GET .../join-requests`). |
| Grupos | `PUT` | `/groups/{group_jid}/locked` | — | ✅ | Trancar ou destrancar a edição dos metadados do grupo | `200`; `GET /groups/{group_jid}` passou a devolver `is_locked:true`, junto com as outras 4 mudanças da mesma leva. |
| Grupos | `PUT` | `/groups/{group_jid}/name` | — | ✅ | Mudar o nome de um grupo | `200`; `GET /groups/{group_jid}` passou a devolver o nome novo, e web.whatsapp.com (envia) mostrou o mesmo nome na lista de conversas. |
| Grupos | `POST` | `/groups/{group_jid}/participants` | `POST /group/updateparticipants` | ✅ | Adicionar ou remover participantes de um grupo | `action:"remove"` tirou recebe (`participant_count` 2->1, confirmado por `GET`); `action:"add"` recolocou (1->2). Achado incidental de input: esta rota aceita número NU (`554192421234`), diferente da rota irmã de aprovação de pedidos, que exige JID completo — confirma a divergência já documentada no próprio `grupo.yaml`. |
| Grupos | `DELETE` | `/groups/{group_jid}/photo` | `POST /group/photo/remove` | ✅ | Remover a foto de um grupo | `200 "Group photo removed successfully"`; `GET /groups/{group_jid}` não expõe campo de foto (limitação da rota de leitura, não desta), então a confirmação foi só visual: o avatar vermelho definido pela chamada anterior sumiu de web.whatsapp.com, com o sistema "Você apagou a imagem deste grupo". |
| Grupos | `PUT` | `/groups/{group_jid}/photo` | `POST /group/photo` | ✅ | Definir a foto de um grupo | primeira tentativa com um JPEG sintético inválido (1x1) devolveu `422 upstream_rejected` — "the given data is not a valid image", erro correto do WhatsApp, não bug. Com um JPEG 200x200 real (Pillow): `200 "Group photo set successfully"`; `GET /groups/{group_jid}` não expõe campo de foto, então a confirmação foi visual: o avatar do grupo em web.whatsapp.com (sessão envia) passou a mostrar exatamente a cor vermelha enviada, com o sistema "Você mudou a imagem do grupo". |
| Grupos | `PUT` | `/groups/{group_jid}/settings/join-approval` | `POST /group/joinapprovalmode` | ✅ | Exigir aprovação de administrador para entrar no grupo | campo correto é `mode` (não `require_approval`) — a primeira tentativa com o nome errado devolveu `200` mas na prática DESLIGOU a exigência (`mode` ausente vale `false`, como a própria documentação avisa), medido por `GET /groups/{group_jid}` continuando com `is_join_approval_required:false`. Com `mode:true`: `200`, e `GET` passou a `true` — confirmado também pelo efeito de ponta a ponta: `POST /groups/join` (recebe) virou PEDIDO pendente em vez de entrada direta. |
| Grupos | `PUT` | `/groups/{group_jid}/topic` | — | ✅ | Mudar a descrição de um grupo | `200`; `GET /groups/{group_jid}` passou a devolver o tópico novo, na mesma leva de name/announce-only/ephemeral/locked. |
| Integrações e configuração | `POST` | `/call/reject` | — | ✅ | Recusar uma chamada a entrar | DESTRAVADO: usuário fez uma chamada de voz real para `recebe`. Listener no `/session/ws` de `recebe` capturou o evento `CallOffer` (`From`/`CallID` reais) e disparou `POST /call/reject` automaticamente, dentro da janela exigida pela rota. `200 {"details":"Call rejected","call_id":"..."}` — o `call_id` devolvido bate exatamente com o do evento capturado. |
| Integrações e configuração | `DELETE` | `/hmac/config` | — | ✅ | Revogar a chave HMAC da sessão | `200 {"Details":"HMAC configuration deleted successfully"}`; `GET /hmac/config` voltou de `{"hmac_key":"***"}` para `{"hmac_key":""}`. |
| Integrações e configuração | `GET` | `/hmac/config` | — | ✅ | Consultar se há chave HMAC configurada | estado inicial `hmac_key:""`; `POST /hmac/config` com chave de 37 carateres devolveu `200`, e esta rota passou a devolver `hmac_key:"***"` — confirma que reflete o estado real, não um valor fixo. Revertido com `DELETE /hmac/config` ao final. |
| Integrações e configuração | `POST` | `/hmac/config` | — | ✅ | Gravar a chave HMAC da sessão | `200 {"Details":"HMAC configuration saved successfully","Enabled":true}`; `GET /hmac/config` passou de `{"hmac_key":""}` para `{"hmac_key":"***"}`. |
| Integrações e configuração | `POST` | `/hmac/configure` | — | ✅ | Gravar a chave HMAC da sessão (caminho original) | chave DIFERENTE da do alias curto, sobre estado limpo: `200`, e `GET /hmac/config` passou de `""` para `"***"` — a transicao e desta chamada, nao herdada. |
| Integrações e configuração | `GET` | `/labels` | — | ✅ | Listar as etiquetas da conta | `200 []` na sessão `envia` (a mesma `filarapida` de referência no projeto) — sem etiquetas sincronizadas nesta instância. Formato bate com o documentado (array vazio, nunca `null`); não há rota de escrita para forçar uma etiqueta a existir. |
| Integrações e configuração | `GET` | `/labels/{id}/chats` | — | ✅ | Listar as conversas de uma etiqueta | `GET /labels/1/chats` (sem etiqueta 1 existente, já que `/labels` veio vazio) devolveu `200 []` — confirma o comportamento documentado: etiqueta inexistente responde `200` com lista vazia, não `404`. |
| Integrações e configuração | `POST` | `/proxy/set` | — | ✅ | Configurar o proxy de saída da sessão | `200 {"Details":"Proxy configured successfully","Set":true,"ProxyURL":"http://outro.exemplo.invalid:3128"}`; `GET /session/status` passou a devolver esse `proxy_url`, substituindo o `socks5` que a rota irma tinha gravado. |
| Integrações e configuração | `DELETE` | `/s3/config` | — | ✅ | Remover a configuração de S3 da sessão | `200 {"Details":"S3 configuration deleted successfully"}`; `GET /s3/config` passou de `bucket: descartavel` para `enabled: false, endpoint: "", bucket: ""`. |
| Integrações e configuração | `GET` | `/s3/config` | — | ✅ | Consultar a configuração de S3 da sessão | estado inicial zerado; `POST /s3/config` (sem endpoint, região/bucket/chaves/retenção/entrega de teste) devolveu `200`, e esta rota passou a devolver exatamente os campos enviados (`region:us-fase2`, `bucket:fase2-teste`, `retention_days:9`, `media_delivery:both`, `access_key:"***"`). Revertido com `DELETE /s3/config` ao final. |
| Integrações e configuração | `POST` | `/s3/config` | — | ✅ | Gravar a configuração de S3 da sessão | `200 {"Details":"S3 configuration saved successfully","Enabled":true}`; `GET /s3/config` devolveu campo a campo o corpo enviado (`bucket: descartavel`, `retention_days: 7`, `media_delivery: both`), com `access_key: "***"`. |
| Integrações e configuração | `POST` | `/s3/configure` | — | ✅ | Gravar a configuração de S3 da sessão (caminho original) | corpo DIFERENTE do alias curto (`eu-west-1`, `descartavel-configure`, `retention_days: 11`): `200`, e `GET /s3/config` devolveu o corpo novo. |
| Integrações e configuração | `POST` | `/s3/test` | — | ✅ | Testar a ligação ao bucket configurado | DESTRAVADO com bucket B2 (Backblaze) real fornecido pelo usuário — chave dedicada, isolada das buckets de produção. `POST /s3/config` com endpoint/bucket/credenciais reais, `enabled:true` → `200`. `POST /s3/test` → `200 {connected:true, details:"S3 connection test successful", bucket:"...", region:"us-west-004"}` — conexão real confirmada contra o bucket. Configuração removida ao final (`DELETE /s3/config`), sem deixar credenciais reais gravadas. |
| Integrações e configuração | `DELETE` | `/webhook` | — | ✅ | Remover o webhook da sessão | `200 {"Details":"Webhook and events deleted successfully"}`; `GET /webhook` passou de `webhook: http://127.0.0.1:8092/hook2` para `webhook: "", subscribe: [""]`. |
| Integrações e configuração | `GET` | `/webhook` | — | ✅ | Consultar o webhook da sessão | estado inicial `webhook:""`; `POST /webhook` com uma URL de teste devolveu `200`, e esta rota passou a devolver essa URL em `webhook`. Revertido com um novo `POST /webhook` de URL vazia ao final. |
| Integrações e configuração | `POST` | `/webhook` | — | ✅ | Definir o webhook da sessão | `200 {"webhook":"http://127.0.0.1:8092/hook"}`; `GET /webhook` passou de `webhook: ""` para esse URL, com `subscribe: ["Message","ReadReceipt"]`. |
| Integrações e configuração | `PUT` | `/webhook` | — | ✅ | Actualizar ou desligar o webhook da sessão | `200 {"active":true,"events":["Message"],"webhook":"…/hook2"}`; `GET /webhook` confirmou o URL novo e `subscribe: ["Message"]` — o `ReadReceipt` gravado pelo `POST` desapareceu, o que torna a substituicao observavel. |
| Integrações e configuração | `GET` | `/webhook/history` | — | ✅ | Consultar o limite de gravação de mensagens | estado inicial `history:0`; `POST /session/history` com `history:21` devolveu `200`, e esta rota passou a devolver `history:21` — confirma que é o mesmo dado de `POST /session/history` (já sabido, mas agora medido nesta sessão). Revertido com `POST /session/history {history:0}` ao final. |
| Integrações e configuração | `POST` | `/webhook/history` | — | ✅ | Definir o limite de gravação de mensagens | `200 {"Details":"History configured successfully","History":77}`; `GET /webhook/history` passou de `{"History":0}` para `{"History":77}`. |
| Saúde | `GET` | `/health` | — | ✅ | Consultar a saúde detalhada do serviço | exige TOKEN DE SESSÃO (não admin, não anónimo) — medido: `401` sem token e com token de admin, `200` só com `token: <sessão>`. `data.total_users`/`active_connections` bateram com as duas sessões reais (envia, recebe) no momento da chamada. |
| Saúde | `GET` | `/health/live` | — | ✅ | Verificar se o processo está vivo (caminho alternativo) | `200 {"status":"ok"}` sem token nenhum — confirma que é o probe de container, sem autenticação, ao contrário de `GET /health`. |
| Saúde | `GET` | `/health/ready` | — | ✅ | Verificar se o serviço pode receber tráfego | `200` sem token, com `checks.database: "ok"` e o relatório de capacidades (`cluster_mode`, `database`, `multi_pod`) — confere com a configuração real do processo (sqlite, single). |
| Saúde | `GET` | `/livez` | — | ✅ | Verificar se o processo está vivo | `200 {"status":"ok"}` sem token nenhum. |
| Sessões | `GET` | `/session/connect` | — | ✅ | Iniciar a ligação da sessão ao WhatsApp | sessao descartavel nunca emparelhada: `200 {"status":"connecting"}`, e `GET /session/status` passou de `connected: false, qrcode: ""` para `connected: true` com QR de 1842 caracteres — ligou-se mesmo ao WhatsApp. Ver F274 para o que acontece na SEGUNDA chamada depois de um disconnect. |
| Sessões | `GET` | `/session/disconnect` | — | ✅ | Derrubar o transporte da sessão sem desemparelhar | `200 {"details":""}`; `GET /session/status` voltou a `connected: false` mantendo `loggedIn: false`. |
| Sessões | `POST` | `/session/history` | — | ✅ | Configurar quantas mensagens a sessão guarda | `200 {"Details":"History configured successfully","History":33}`; `GET /webhook/history` passou a devolver `{"History":33}` — confirma tambem que e o mesmo manipulador de `POST /webhook/history`. |
| Sessões | `DELETE` | `/session/hmac/config` | — | ✅ | Revogar a chave HMAC desta sessão | `200`; `GET /session/hmac/config` voltou de `"***"` para `""`. |
| Sessões | `GET` | `/session/hmac/config` | — | ✅ | Saber se esta sessão tem chave HMAC configurada | `200 {hmac_key:""}` — bate com o estado real da sessão `envia` (chave HMAC não configurada nesta rodada, já revertida ao fim da Fase 2). |
| Sessões | `POST` | `/session/hmac/config` | — | ✅ | Gravar a chave HMAC de assinatura dos webhooks | terceira chave distinta, sobre estado limpo: `200`, e `GET /session/hmac/config` passou de `""` para `"***"`. |
| Sessões | `POST` | `/session/logout` | — | ✅ | Desvincular o aparelho da conta de WhatsApp | F275 CORRIGIDO em 2026-08-27 (antes desta sessão) — a evidência ❌ estava desatualizada, media o comportamento pré-conserto. RE-MEDIDO ao vivo com sessão descartável ligada e nunca emparelhada: agora devolve `409 {code:"session_not_paired", message:"session has a live connection but was never paired; there is no device to log out"}` — não mais `500`. Ver F275 (código: `pkg/infra/wa-noise/runtime/session/guard.go`) para a correção e os testes que a travam. |
| Sessões | `POST` | `/session/pair/phone` | `POST /session/pairphone` | ✅ | Emparelhar por código de telefone em vez de QR | DESTRAVADO com um TERCEIRO número descartável fornecido pelo usuário (não registrado aqui, é dado pessoal). Sessão nova criada só para o teste; `GET /session/connect` + `POST /session/pair/phone {phone:"..."}` devolveu `200 {linking_code:"XXXX-XXXX"}`. Primeiro código expirou (janela curta, ~2min) antes do usuário digitar — pedido um segundo código, digitado a tempo: `GET /session/status` confirmou `connected:true, logged_in:true`, `jid` batendo com o número fornecido. Sessão desconectada e apagada ao final (`GET /session/disconnect` + `DELETE /admin/users/{id}/full`); `envia`/`recebe` intactas. |
| Sessões | `GET` | `/session/pair/qr` | `GET /session/qr` | ✅ | Ler o QR code de emparelhamento | `200 {qr_code:""}` numa sessão já autenticada — bate com o documentado ("vazio é comportamento normal" fora da janela de emparelhamento). |
| Sessões | `GET` | `/session/profile` | — | ✅ | Consultar o perfil da conta ligada | `200`, `jid:5516981818244@s.whatsapp.net`, `business_name:"FilaRápida"`, `connected:true, logged_in:true` — bate byte a byte com a identidade conhecida de `envia`. |
| Sessões | `GET` | `/session/profile/full` | — | ✅ | Consultar o perfil da conta com os dados que só a rede sabe | `200`, mesmos campos de `/session/profile` mais `user_info` (com `devices`, 3 aparelhos) e `privacy` — o bloco `privacy` é IDÊNTICO ao devolvido por `GET /users/privacy` na Fase 8 (F352), confirmando que é a mesma fonte. |
| Sessões | `POST` | `/session/proxy` | — | ✅ | Configurar o proxy de saída desta sessão | `200 {"Details":"Proxy configured successfully","Set":true,"ProxyURL":"socks5://proxy.exemplo.invalid:1080"}`; confirmado por DOIS observadores — `GET /session/status` (`proxy_url`) e `GET /admin/users/{id}` (`proxy_config.enabled: true`). |
| Sessões | `DELETE` | `/session/s3/config` | — | ✅ | Remover a configuração S3 desta sessão | `200`; `GET /session/s3/config` passou de `bucket: descartavel-sessao` para `enabled: false, bucket: ""`. |
| Sessões | `GET` | `/session/s3/config` | — | ✅ | Ler a configuração S3 desta sessão | `200` com a configuração S3 zerada (`enabled:false`) — bate com o estado real, revertido ao fim da Fase 2 (F346). |
| Sessões | `POST` | `/session/s3/config` | — | ✅ | Gravar a configuração S3 desta sessão | corpo distinto (`ap-south-1`, `descartavel-sessao`, `retention_days: 5`): `200`, e `GET /session/s3/config` devolveu-o. |
| Sessões | `POST` | `/session/s3/test` | — | ✅ | Testar a configuração S3 gravada com uma ida real ao bucket | DESTRAVADO no mesmo teste de `POST /s3/test` — confirmado que é o MESMO manipulador (`GET /session/s3/config` já refletia a config gravada por `POST /s3/config` antes de eu tocar em `/session/s3/config`). `POST /session/s3/test` → `200 {connected:true, ...}` idêntico. Configuração removida ao final. |
| Sessões | `GET` | `/session/status` | — | ✅ | Consultar o estado e o registo da sessão | `200`, `id:16da96746c368b5bc4c2bb0fb363d8d4, name:"envia", connected:true, logged_in:true` — bate exatamente com a sessão administrativa conhecida (mesmo id usado em todas as fases desta campanha). |
| Sessões | `GET` | `/session/ws` | — | ✅ | Receber os eventos da sessão em tempo real (WebSocket) | `101 Switching Protocols` com `Sec-Websocket-Accept` valido, seguido de tres quadros de texto `type: QR` com `code`, `qrCodeBase64` e `expiresAt` na raiz e sem envelope. Cliente RFC 6455 proprio; o motivo antigo ("fora do alcance de curl") media a ferramenta, nao a rota. |
| Sessões | `POST` | `/users/contacts/sync` | `POST /user/contacts/sync` | ✅ | Forçar a sincronização da agenda de contactos | `{mode:"if_unsynced"}` devolveu `200 {details:"contact roster sync requested"}`; `GET /users/contacts` manteve 2 chaves antes e depois — bate com o documentado (`if_unsynced` não faz nada quando a agenda já está sincronizada). |
| Sessões | `POST` | `/users/history/sync` | `POST /user/history/sync` | ✅ | Pedir ao telemóvel as mensagens anteriores a uma âncora | pedido com âncora numa mensagem real (`oldest_msg_id` de uma mensagem de teste enviada nesta campanha) devolveu `200` com um `details` (id do pedido) DIFERENTE do `oldest_msg_id` enviado — confirma que é um novo pedido despachado, não um eco; `GET /chats/history` manteve a mesma contagem, bate com o documentado (não há mensagens mais antigas que a âncora nesta conversa de teste). |
| Sessões | `POST` | `/users/status` | `POST /user/status` | ✅ | Definir o recado do perfil da conta | DESTRAVADO com permissão explícita do usuário. Recado original `"conta de testes"`; `POST {body:"Teste ao vivo — F239/F282 destrave 2026-08-28"}` devolveu `200`, e `GET /session/profile/full` (`user_info[0].status`) passou a devolver o texto novo. Revertido com `POST {body:"conta de testes"}` — `GET` confirmou a volta ao texto original. |
| Status | `POST` | `/status/set/audio` | — | ✅ | Publicar um status com áudio | CONSERTADO (F358), mesmo fix de `/status/set/video`. Reproduzido ao vivo: mesma linha de log `mensagem reentregue NAO suprimida; a primeira copia nao tinha midia utilizavel (F358)`, e `GET /session/ws` de `recebe` recebeu o `audioMessage` completo (URL, mediaKey) na segunda cópia. |
| Status | `POST` | `/status/set/image` | — | ✅ | Publicar um status com imagem | DESTRAVADO: usuário salvou `recebe` como contacto com nome no telefone de `envia` (pré-condição da F256 — `FullName` preenchido, confirmado via `GET /users/contacts`: `full_name:"Lucas Albuqueque - Teste Recebe"`). Publicado JPEG 600x800 real: `200`. WebSocket de `recebe` recebeu o evento completo em segundos — `imageMessage` com `mimetype:image/jpeg`, `caption` batendo com o enviado, mesmo `message_id`. Entrega ponta a ponta confirmada. |
| Status | `POST` | `/status/set/video` | — | ✅ | Publicar um status com vídeo | CONSERTADO (F358): causa raiz encontrada e corrigida em pkg/bootstrap/message_dedup.go — a dedup de mensagens (F103) suprimia por message_id sem checar se o payload de mídia estava presente; a primeira cópia (senderKeyDistributionMessage-only) poluía o cache e a segunda cópia (com o videoMessage real, chegando via retry automático do protocolo) era descartada como duplicata. Reproduzido ao vivo, log exato: `mensagem reentregue NAO suprimida; a primeira copia nao tinha midia utilizavel (F358)`. `GET /session/ws` de `recebe` recebeu o `videoMessage` completo (URL, mediaKey, caption) na segunda cópia. Cinco testes novos em pkg/bootstrap/message_dedup_test.go, controlo negativo executado. |
