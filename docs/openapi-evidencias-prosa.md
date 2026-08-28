# Relatório de evidências — documentação OpenAPI

**Actualizado a 2026-08-26**, depois da padronização de caminhos (F269) e da
integração das três campanhas de evidência.

O que mudou nesta ronda:

| | antes | depois |
|---|---:|---:|
| ✅ | 98 | **122** |
| 🟡 | 8 | 8 |
| ❌ | 3 | **4** |
| ⬜ | 32 | **7** |
| total | 141 | 141 |

**25 rotas mudaram de marca**, todas a sair de ⬜: 24 para ✅ e uma para ❌. A
campanha que as moveu está em `CAMPANHA-DESCARTAVEL.md`, e o que a destravou
foi um fixture — sessões criadas por `POST /admin/users` e nunca emparelhadas,
descartáveis por construção. O motivo *"mexeria na sessão em uso"*, que
bloqueava 25 das 32 ⬜, não era impossibilidade: era falta de fixture.

Os 🟡 não se moveram, e é correcto que não se tenham movido — sem conta
emparelhada não há observador a exercitar. O que mudou neles foi o **motivo**:
quatro dos oito estavam factualmente errados (`OBSERVADORES-AMBAR.md`).

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
  OK  chamada real com efeito confirmado: 117
  AMR sucesso sem observador independente: 8
  ERR falhou, com o erro medido:          4
  NT  não testada, com o motivo dito:     8
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

## As oito 🟡, e o que realmente as bloqueia

O inventário completo está em `OBSERVADORES-AMBAR.md`. O resultado é
assimétrico e vale dizê-lo já: **nenhuma das oito está sem observador.** Sete
têm observador identificado no código, e a oitava tem-no identificado junto com
a razão exacta pela qual ele mostra zero. O que as bloqueia é **pré-condição** —
todas exigem conta emparelhada, e cinco exigem duas.

**Quatro dos oito motivos escritos aqui até 2026-08-26 eram factualmente
falsos**, e estão substituídos abaixo.

| Endpoint | Motivo preciso |
|---|---|
| `POST /chats/request-unavailable-message` | 200. **Observador existe**: o reenvio chega como `*events.Message` com `UnavailableRequestID` igual ao `request_id` devolvido (`capabilities/message/history_sync.go:250`), legivel por `GET /chats/history` no `data_json` e pelo webhook/`/session/ws`. Falta a PRE-CONDICAO: uma mensagem genuinamente indecifravel, que nao e criavel por HTTP. Ver `OBSERVADORES-AMBAR.md` §1. |
| `POST /chats/send/sticker` | 200 e a mensagem chegou; bolha vazia porque **o WebP de entrada nao e convertido** — `http.DetectContentType` devolve `image/webp` (medido) e esse tipo cai no `default` de `ConvertToWebPSticker` (`media/sticker/exif.go:62`), logo nao passa pelo `scale=512:512`. **So falta fixture**: um PNG 512x512 forca a conversao. Observador ja existe e e ✅ — `GET /chats/history` + `POST /chats/download/sticker` + verificacao dos bytes RIFF/WEBP. Achado F279. |
| `POST /groups/{group_jid}/join-requests` | 200 contra fila VAZIA, que nao prova nada. **Observador existe**: `GET /groups/{group_jid}/join-requests` (✅) antes/depois, mais `POST /groups/info`. A pre-condicao e criavel so por API — `JoinWithLink` devolve `membership_approval_request` quando o grupo exige aprovacao (`capabilities/group/invite.go:117`) —, mas exige DUAS sessoes emparelhadas. Ver `HUMAN-LAST.md` C.1. Achado F280: o resultado por participante que o WhatsApp devolve e descartado em `adapters/group/participants.go:81`. |
| `POST /newsletters/mark-viewed` | 200 com data:null. **O observador existe por desenho e esta inalcancavel**: `NewsletterMarkViewed` incrementa o contador de vistas (`core/newsletter.go:52`) e o contador so se le por `GetNewsletterMessageUpdates` (`core/newsletter.go:206`) — que e `POST /newsletters/updates`, a rota ❌ da F265. Via alternativa nunca exercitada: `POST /newsletters/subscribe` + evento `NewsletterLiveUpdate`, que carrega `ViewsCount`. `POST /newsletters/messages` traz o campo mas mediu **0 em 60/60** e nao distingue. Ver `OBSERVADORES-AMBAR.md` §4. |
| `POST /newsletters/react` | 200 com data:null. **O motivo antigo ficou obsoleto no proprio dia**: mediu-se um canal com sessenta publicacoes e `POST /newsletters/messages` devolveu `ReactionCounts` reais (ex.: `{'👍':3,'😂':58}`). O observador e a leitura antes/depois do mesmo `MessageServerID`, com o passo de REMOCAO (`reaction` vazio) a fechar a causalidade. Falta apenas uma sessao emparelhada e autorizacao para reagir numa conta real. Ver `HUMAN-LAST.md` B.3. |
| `POST /status/set/image` | 200 com message_id. **Observador existe**: a SEGUNDA sessao — `GET /chats/history?chat_jid=status@broadcast`, ou o webhook/`/session/ws` dela. O que falta e a pre-condicao da F256: `getStatusBroadcastRecipients` so inclui contactos com `FullName` (`core/broadcast.go:70`), e a conta Business tem 1 em 461. **A pre-condicao e verificavel por API antes de publicar**, com `GET /users/contacts`. Ver `HUMAN-LAST.md` C.2. |
| `POST /status/set/video` | Mesmo caminho e mesma pre-condicao de `/status/set/image` (`SendVideo` para `status@broadcast`). **Precisa de medicao PROPRIA**: evidencia herdada nao e evidencia. Ver `HUMAN-LAST.md` C.2. |
| `POST /status/set/audio` | Mesmo caminho e mesma pre-condicao de `/status/set/image` (`SendAudio` para `status@broadcast`). **Precisa de medicao PROPRIA**: evidencia herdada nao e evidencia. Ver `HUMAN-LAST.md` C.2. |

## As oito que continuam por testar, e porquê

Sete delas sem o motivo antigo *"mexeria na sessão em uso"* — esse foi
derrubado pela campanha da sessão descartável, e valeu 25 rotas. A oitava é
nova, e o motivo é outro: ainda não foi medida.

| Endpoint | Motivo |
|---|---|
| `POST /chats/download/{kind}` | rota nova (CAP-10, 2026-08-27): consolida as cinco rotas de descarga com o `kind` na relação do caminho, não colado ao corpo. Reusa o mesmo caminho de código das cinco ✅ (`mediaDownloadFlow`), mas ainda não foi chamada contra um servidor real — herdar evidência de código não é medição, então fica ⬜ até ter a sua própria |
| `POST /s3/test` | não há bucket descartável: o validador de saída recusa endpoint em loopback (medido `400 invalid_s3_endpoint` para `http://127.0.0.1:9000`, F277), o que impede um MinIO local, e não há credenciais AWS descartáveis |
| `POST /session/s3/test` | idem — mesmo manipulador |
| `POST /call/reject` | exige uma chamada a entrar; não há como provocar uma |
| `POST /session/pair/phone` | iniciaria emparelhamento de um número real |
| `POST /users/avatar` | alteraria o avatar da conta |
| `POST /users/privacy` | alteraria definições de privacidade da conta |
| `POST /users/status` | alteraria o recado da conta |

As quatro últimas exigem uma conta de WhatsApp **emparelhada** cujo perfil
seria mexido — uma sessão descartável não serve, porque o que está em causa
é a conta por trás dela, não a sessão. O procedimento de cada uma está em
`HUMAN-LAST.md`.


<!-- GERADO:TABELA-COMPLETA -->
