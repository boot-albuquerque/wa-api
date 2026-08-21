# Paridade com o `whatsapp-web.js` — a definição executável de "equivalente"

O `HANDOFF-INICIATIVA.md` §1 diz que o `wa-headless` precisa ser
**funcionalmente equivalente ao `whatsapp-web.js`**. Sem esta matriz, isso é a
superfície inteira da biblioteca, que é escopo infinito. Com ela, é uma lista.

Tudo aqui vem de **leitura do código** do `disparazaap`
(`features/macbook-lucas`, árvore limpa, somente leitura), não de memória. Cada
afirmação tem caminho de arquivo.

Fonte primária: `services/wa-worker/src/infrastructure/wa/adapter.ts`.

---

## 1. O achado que muda o plano: a fenda não é o adapter TypeScript

O `wa-api` **já é um `AdapterKind` de primeira classe** e já tem implementação
própria em `services/wa-worker/src/infrastructure/wa/wa-api-adapter.ts`, ligada
em `services/wa-worker/src/index.ts:279` sob a flag `WA_WA_API_ENABLED`
(ADR-0033, fatia 0007).

Esse adapter **não fala com um browser**. Ele fala com o serviço `wa-api` por
HTTP e WebSocket — `GET /session/ws`, `GET /session/status`,
`GET /session/profile`, `GET /user/contacts`, `POST /user/contacts/sync` — e
hoje quem serve essas rotas, deste lado, é o `internal/wa-noise` (whatsmeow).
Confirmado neste repositório em `pkg/presentation/http/handlers/handler_session.go:47`,
`handler_session_ws.go:21`, `handler_session.go:281` e
`pkg/presentation/http/profile_handler.go:42`.

**Consequência direta**: o `wa-headless` não substitui o `wa-api-adapter.ts`.
Ele vira um **segundo motor atrás da mesma superfície HTTP** que o `wa-noise`
já serve. O arquivo TypeScript não muda — a incerteza nº 1 do handoff (§7)
fecha na **leitura B**, e fecha por evidência, não por preferência.

O que isso reorganiza:

- **CAP-09** deixa de ser "construir uma fachada nova" e passa a ser "servir
  rotas que já existem, com o motor novo por trás". O contrato já está escrito
  e já tem consumidor.
- A pergunta "equivalente ao `wwebjs`" vira duas perguntas separáveis:
  **(a)** o que o `wa-api` já entrega hoje pelo `wa-noise`, e **(b)** o que só o
  `wwebjs` entrega, que é onde o `wa-headless` tem de existir.

---

## 2. A matriz

14 métodos: **6 obrigatórios**, **8 opcionais**. Todos os 14 têm call site real
no `services/wa-worker/src/application/runner.ts` — nenhum é declarado e não
usado.

| # | capability | wwebjs | usado pelo produto (call site) | `wa-api` hoje | precisa do `wa-headless`? |
|---|---|:---:|---|:---:|---|
| 1 | `start` | sim `:454` | **obrigatório** · `runner.ts:1204` | sim `:98` | **não** — já servido |
| 2 | `stop` | sim `:848` | **obrigatório** · `runner.ts:1284` | sim `:131` | **não** — já servido |
| 3 | `on` (WaEvent) | sim `:396` | **obrigatório** · `runner.ts:1184` | sim `:142` | **não** — já servido |
| 4 | `onContact` | sim `:400` | **obrigatório** · `runner.ts:1197` | sim `:146` | **não** — já servido |
| 5 | `listContacts` | sim `:908` | **obrigatório** · `runner.ts:2185` | sim `:150` | **não** — já servido |
| 6 | `sendText` | sim `:899` | **obrigatório** · `runner.ts:2891` | sim `:200` | **não** — já servido |
| 7 | `fetchContactAvatar` | sim `:1122` | opcional · `runner.ts:2684`, `:2390` | sim `:208` | **não** — já servido |
| 8 | `primeContactRoster` | **NÃO** | opcional · `runner.ts:2539`, `:2559` | sim `:188` | **não** — o `wwebjs` é que não tem |
| 9 | `fetchMessages` | sim `:872` | opcional · `runner.ts:2806` | **não** | **SIM** |
| 10 | `onMessageMeta` | sim `:404` | opcional · `runner.ts:1199` | **não** | **SIM** |
| 11 | `livenessCheck` | sim `:818` | opcional · `runner.ts:2134` | **não** | **SIM** |
| 12 | `refreshOwner` | sim `:779` | opcional · `runner.ts:2087`, `:2104` | **não** | **SIM** |
| 13 | `getBrowserPid` | sim `:760` | opcional · `runner.ts:1234` | **não** | **SIM** (é browser) |
| 14 | `backupNow` | sim `:835` | opcional · `runner.ts:728` | **não** | **SIM** (perfil em disco) |

Colunas `wwebjs` e `wa-api` referem-se a
`infrastructure/wa/whatsapp-web-js-adapter.ts` e
`infrastructure/wa/wa-api-adapter.ts`, com o número da linha da definição.

O runner protege cada opcional com uma guarda (`if (!adapter.X) return`,
`runner.ts:1230`, `:2129`, `:2680`, `:2791`, `:717`), então a ausência degrada
em silêncio em vez de quebrar — o que também significa que **uma capacidade
faltando não aparece como erro**, só como funcionalidade que sumiu.

---

## 3. O escopo real do `wa-headless`

**Seis capacidades**, não catorze. São exatamente as linhas 9–14: a diferença
entre o que o `wa-noise` já entrega pelo protocolo e o que hoje só existe
dirigindo o SPA.

| ordem | capacidade | por quê primeiro |
|---|---|---|
| 1 | `livenessCheck` | é a invariante 11 do handoff, e o achado do H1 da fase 6 (renderer que não responde com tudo saudável por fora) é literalmente esta capacidade. Nada acima dela é confiável sem ela. |
| 2 | `getBrowserPid` | trivial dado o CAP-02, e o `ChromiumSupervisor` do produto depende dele para registrar o processo |
| 3 | `refreshOwner` | primeira leitura do SPA; exercita o inventário de módulos (CAP-06) num caminho de baixo risco |
| 4 | `onMessageMeta` | metadata-only, invariante **12**; realtime, sem envio |
| 5 | `fetchMessages` | metadata-only; mesma superfície de leitura da anterior |
| 6 | `backupNow` | durabilidade do perfil; depende do lifecycle da CAP-05 estar fechado |

`sendText` **permanece no mapa** (CAP-07 do handoff) apesar de a linha 6 dizer
"já servido": o `wa-api` envia hoje pelo `wa-noise`, e um envio pelo
`wa-headless` só passa a ser necessário quando uma conta rodar no motor de
browser. Ele deixa de ser a primeira capacidade de produto e passa a ser
consequência do CAP-09 — mas o fluxo Resolve → Validate → Act → Verify
continua obrigatório quando chegar, porque a alternativa é sucesso silencioso.

---

## 4. O que fica fora da fatia atual do `WaClientAdapter`

> **CORREÇÃO (2026-08-18, LOOP 04.4).** O título desta seção era "O que NÃO
> implementar", redação que lê como exclusão GLOBAL de escopo do
> `wa-headless`. Isso é falso: o que a matriz prova é **apenas** que estas
> quatro coisas estão `OUT_OF_CURRENT_WA_WORKER_ADAPTER_SLICE` — fora do
> contrato `WaClientAdapter` que o `wa-worker` consome HOJE
> (`adapter.ts`, seção 2 acima). O `internal/wa-headless` é um SDK interno
> com roadmap mais amplo do que essa fatia; paridade de integração ATUAL
> **não é** o mesmo que escopo de capacidade GLOBAL do `wa-headless`. Nenhuma
> destas quatro capacidades foi aberta para implementação por esta correção —
> é mudança de redação e escopo, não de trabalho.

Por evidência desta matriz, contra o contrato atual:

- **grupos, broadcast, status/stories — `NOT_REQUIRED_BY_CURRENT_PRODUCT_CONTRACT`**:
  a interface filtra os três em todo método que os menciona (`adapter.ts:97`,
  `:117`, `:128`) — o produto é 1:1 nesta fatia.
- **mídia no envio — `OUT_OF_CURRENT_WA_WORKER_ADAPTER_SLICE`**:
  `sendText(waJid, body)` é a única superfície de envio declarada no contrato
  atual. Não existe `sendMedia`, `sendImage`, `sendDocument`.
- **corpo de mensagem — `NOT_REQUIRED_BY_CURRENT_PRODUCT_CONTRACT`**:
  `WaMessageMeta` é **metadata-only por invariante** (`adapter.ts:57`, `:116`,
  `:136`; invariante **12** do handoff) nesta fatia. O texto live é outra via
  (core-NATS `live.>`).
- **`primeContactRoster` no motor de browser — `OUT_OF_CURRENT_WA_WORKER_ADAPTER_SLICE`**:
  o próprio `adapter.ts:188` registra que o `wwebjs` não tem equivalente.
  Implementá-lo superaria o `wwebjs`, e o handoff §1 exclui isso do escopo
  ATUAL — não é exclusão permanente do `wa-headless` como SDK.

---

## 5. O que esta matriz NÃO estabelece

- **Como o `wa-api` decide qual motor usar por conta.** Hoje o `AdapterKind` é
  resolvido no `wa-worker` (`index.ts:265`); o `wa-headless` seria uma escolha
  de motor DENTRO do `wa-api`, e essa chave não existe. É desenho do CAP-09.
- **Se as seis capacidades restantes são atingíveis pelo SPA** com o inventário
  de módulos do CAP-06. Nenhuma foi exercitada contra alvo real ainda.
- **O custo.** Nenhuma medição de memória ou tempo por capacidade.

---

_Levantado em 2026-08-11 (CAP-01), contra `disparazaap` em
`features/macbook-lucas`. Se o `adapter.ts` mudar, esta matriz vence — refaça a
busca antes de confiar nela._

---

## 6. Divergências conscientes do `whatsapp-web.js`

O `CLAUDE.md` exige que divergir seja **decisão registrada**, não acidente.
Esta seção existe para isso. Divergir é aceitável; divergir sem saber é como
este projeto reescreve os bugs deles junto.

### 6.1 — `refreshOwner` mantém PN e LID SEPARADOS

**O que o `wwebjs` faz** (`Client.js:351-364`, 1.34.7):

```js
wid: window.require('WAWebUserPrefsMeUser').getMaybeMePnUser()
  || window.require('WAWebUserPrefsMeUser').getMaybeMeLidUser()
```

Um campo só. O `||` escolhe o primeiro que existir e **descarta qual dos dois
respondeu**.

**O que nós fazemos**: `owner.Identity` carrega `PN` e `LID` em campos
distintos, e `Present()` aceita qualquer um dos dois — a mesma condição que o
`wwebjs` trata como "existe wid".

**Por quê**: LID e PN são espaços de nomes diferentes para a mesma pessoa, e
**este produto tem histórico documentado de confundi-los**. Para uma biblioteca
cujos chamadores só exibem o valor, colapsar é barato. Aqui não é: um chamador
que segura o resultado precisa saber se tem identidade de telefone ou LID,
porque os dois não são intercambiáveis ao falar com o servidor.

**O que a divergência assume, e onde isso é verificado**: que os dois chegam
juntos. A M3 mediu assim, e `TestRealSPARefreshOwnerAgainstProduction` falha com
mensagem própria se um dia só um materializar — dizendo que `Present()` continua
correto mas que **esta decisão precisa ser revisitada**, em vez de só quebrar.

### 6.2 — `DisplayName` existe na superfície e **nunca foi visto preenchido**

Medido em 2026-08-19 contra o perfil pareado real:

| getter | resultado |
|---|---|
| `WAWebUserPrefsMeUser.getMaybeMeDisplayName` | **`NULL`** (a função existe) |
| `WAWebUserPrefsMeUser.getMaybeMePnUser` | `object` |
| `WAWebUserPrefsInfoStore.getPushname` | **`GETTER_ABSENT`** |
| `WAWebUserPrefsInfoStore.getMe` | **`GETTER_ABSENT`** |

As duas últimas linhas eram **palpites meus**, e a medição as descartou — que é
o motivo de a regra "medir antes de projetar" existir.

O campo fica na struct porque é onde ele moraria, e o comentário diz que é
**UNVERIFIED**: ninguém deve tratar um `DisplayName` preenchido como garantido,
e um vazio não é evidência de nada sobre a sessão. Se é nulo porque a conta não
tem pushname ou porque este build o guarda noutro lugar continua **UNKNOWN** —
e `TestRealSPADisplayNameShape` é o instrumento que distingue os casos no dia
em que mudar.

### 6.3 — Correção de referência (2026-08-19)

As duas citações de *"invariante 13"* neste documento apontavam para a
invariante errada. No `HANDOFF-INICIATIVA.md` §6 a lista diz:

- **12** — `WaMessageMeta` é metadata-only; zero PII em log
- **13** — reclaim de `Singleton` no boot é obrigatório em contêiner

Metadata-only é a **12**. Corrigido nos dois lugares. Achado ao ir implementar
`onMessageMeta` e conferir contra qual invariante ela seria validada — conferir
é o que fez a discrepância aparecer, e ela estava no documento desde antes
desta sessão.

### 6.4 — `msg.id._serialized` é **NULL** neste build; o `wwebjs` depende dele

Medido em 2026-08-19 contra o perfil pareado real, antes de `onMessageMeta` ser
implementada. O store é `WAWebMsgCollection.MsgCollection` (`on`/`off`/
`getModelsArray`, 384 modelos); `WAWebMsgStore` — palpite meu — **não existe**.

Forma de um modelo de mensagem, só nomes e tipos:

| campo | resultado |
|---|---|
| `id` | objeto, chaves `$1 \| fromMe \| id \| remote` |
| **`id._serialized`** | **`NULL`** |
| `id.id` | `string` |
| `id.remote` | objeto WID (`_serialized`, `server`, `user`) |
| `id.fromMe` | `boolean` |
| `from` / `to` | objeto WID |
| `t` | `number` |
| `type` | `string` |
| `ack` | `number` |
| `author`, `isGroupMsg`, `isNewMsg`, `chat` | `NULL` (conversa 1:1) |

**A divergência**: o `whatsapp-web.js` usa `msg.id._serialized` como
identificador da mensagem. Neste build esse acessor responde **nulo**. Copiar o
CÓDIGO deles produziria um `waMessageId` vazio em toda mensagem — e vazio é
indistinguível de "sem id", que é a classe de defeito que este módulo passou a
semana caçando.

Copiar o ENTENDIMENTO produz outra coisa: o identificador é composto por
`fromMe`, `remote` e `id`, e é isso que o `_serialized` deles concatena.

**Decisão**: expor as PARTES (`ID`, `FromMe`, `Remote`) em vez de sintetizar uma
string. Mesmo raciocínio do §6.1: não perder informação por colapso. Sintetizar
um identificador à mão é como uma stack começa a discordar do servidor sobre
qual mensagem é qual — e se um dia o `_serialized` voltar, ele passa a ser mais
uma fonte a conferir, não a única.

**Como isto foi encontrado**: `Object.keys` no modelo NÃO lista `_serialized`,
e também não listaria um getter de protótipo que funcionasse — foi a lição já
registrada para `WAWebConnModel.Conn.wid`. Perguntar ao acessor **diretamente**,
em vez de confiar na lista de chaves, é o que separou "campo ausente da lista" de
"campo que responde nulo".

### 6.5 — `fetchMessages` lê o que está CARREGADO, não o histórico

O `whatsapp-web.js` expõe `chat.fetchMessages({limit})`, que pode **pedir mais
ao servidor**. A nossa implementação lê `WAWebMsgCollection.getModelsArray()`,
que é o que a SPA já carregou — **340 modelos** medidos contra uma lista de 917
conversas.

Um chamador que pede 100 e recebe 12 está sendo informado do que está
**carregado**, não do que **existe**.

A lacuna é declarada em vez de escondida porque a alternativa — devolver uma
lista curta como se fosse o histórico — é a incompletude silenciosa que este
módulo vem encontrando em todas as formas. Por isso o `Result` carrega
`Loaded` (o denominador) e `Matched`, e `Truncated()` diz quando o limite cortou:
12 de 340 carregados é uma conversa de pouco tráfego; 12 de 12 é uma página que
mal carregou.

Disparar o carregamento sob demanda é fatia futura, e vai precisar de medição
própria: não se sabe qual chamada a SPA usa para isso neste build.

### 6.6 — `backupNow` recusa perfil em uso, e a cópia só vale depois de restaurada

**Medido antes de projetar**: em **10 segundos** de uma sessão viva e **ociosa**,
**11 de 1124** arquivos do perfil mudaram ou nasceram. O Chromium mantém LevelDB
e IndexedDB escrevendo por baixo. Uma cópia tirada com o browser rodando pega
arquivos no meio da escrita — e o resultado não é um backup levemente
desatualizado, é um **corrompido que parece bom** até o dia em que alguém
precisa dele.

Por isso `Backup` **recusa** perfil em uso, e a checagem acontece **antes** de
qualquer escrita: um backup recusado não deixa diretório pela metade para
alguém achar depois e confiar.

**O `SingletonLock` não é copiado.** Ele é um SYMLINK cujo alvo nomeia um
processo (`<host>-<pid>`); levá-lo para dentro da cópia carregaria uma alegação
sobre um processo que não existe no perfil restaurado — exatamente a condição de
lock obsoleto que o `ReclaimProfile` existe para limpar.

**E a parte que dá sentido ao nome**: contagem de arquivos não é backup. O item
11 do briefing diz que uma ação retornando `nil` não é a operação ter tido
sucesso, e em lugar nenhum isso é mais verdadeiro: um perfil corrompido copia
perfeitamente. `TestRealSPABackupRestoresToAWorkingSession` **boota a cópia** e
exige READY **com identidade presente** — a mesma régua do observável da CAP-05.

**Medição corrigida em 2026-08-20**, porque a primeira era uma amostra só e eu
a apresentei como se fosse o custo de um backup. Não era: era o custo de um
backup de conta VELHA, medido com o cache frio.

Dois perfis, mesma máquina, mesmo instante, três rodadas cada:

| perfil | arquivos | tamanho | rodada 1 (fria) | rodada 2 | rodada 3 |
|---|---:|---:|---:|---:|---:|
| recém-pareado | 758 | 80,4 MB | 411 ms | 111 ms | 96 ms |
| pareado há meses | 1188 | 314,1 MB | 1,03 s | 369 ms | 208 ms |

**Duas variáveis, e eu só tinha visto uma.**

1. **O tamanho escala com a IDADE DA CONTA, não com o número de sessões** —
   3,9× entre um perfil recém-pareado e um com meses de histórico. Um perfil
   novo não carrega conversa nenhuma.
2. **A vazão é dominada pelo estado do cache de página**: 195 → 841 MB/s no
   mesmo perfil, entre a primeira e a terceira rodada. A diferença entre frio e
   quente é de 3 a 5×, **maior que a diferença entre os dois perfis**.

**A leitura errada que isto corrige**: *"o backup custa ~1 s"* virou número
citável a partir de UMA amostra fria de UM perfil velho. O certo é: o custo
depende do tamanho do perfil e do estado do cache, e uma amostra única não
distingue os dois. Repetir foi o que separou.

O restauro segue provado: perfil restaurado alcança READY com `identity=PRESENT`
nos dois perfis.

**O perigo que o chamador tem de decidir**: a cópia carrega as MESMAS
credenciais. Dois browsers vivos nos dois diretórios são dois dispositivos numa
conta — no nível do perfil são diretórios diferentes, então nada nesta stack
impede, e o WhatsApp pode invalidar um deles. Por isso `Verify` recebe o boot
como FUNÇÃO do chamador em vez de bootar sozinho: quem chama teve de decidir que
o original está parado, e exigir a função é como essa decisão vira explícita.

---

### 6.7 — `sendText` resolve a identidade ANTES de abrir a conversa

**O que o `wwebjs` faz**: `sendMessage` vai direto a
`findOrCreateLatestChat(wid)` com um WID construído do número. A resolução pelo
servidor existe na biblioteca, em `getNumberId` →
`WAWebQueryExistsJob.queryWidExists`, mas **não é usada no caminho de envio**.

**O que fazemos**: chamamos `queryWidExists` PRIMEIRO e abrimos a conversa com o
`wid` que o servidor devolveu.

**Por que divergimos**: neste build o WID do número não basta. Sem a resolução,
`findOrCreateLatestChat` lança `No LID for user` para qualquer destinatário com
quem nunca se falou. As issues abertas do próprio `wwebjs` (#3834, #5750,
set/2025 a jan/2026) morrem nesse mesmo ponto sem correção — ou seja, copiar o
caminho dele teria reproduzido o defeito dele.

A medição que sustenta a escolha: de 399 modelos em `WAWebMsgCollection`,
**397 sob `@lid`**, 1 sob `c.us`, 1 sob `g.us`. Este build é LID-first, e o
número de telefone é entrada do usuário, não identidade.

O Baileys resolve o mesmo problema com maquinaria muito maior — um
`LIDMappingStore` com fallback para USync — porque fala o protocolo e precisa
manter o mapeamento. Dirigindo a SPA, o mapeamento já está lá; basta perguntar.
**Mesmo entendimento, uma fração da máquina**, que é exatamente o que este
documento pede que se copie.

### 6.8 — `sendText` VERIFICA a pós-condição; o `wwebjs` retorna na aceitação

**O que o `wwebjs` faz**: devolve assim que a página aceita a chamada.

**O que fazemos**: o envio só retorna depois de a mensagem APARECER na coleção
local, filtrada pela identidade resolvida e por um limite de frescor. É a
invariante 14 (o envio lança em falha e nunca devolve sucesso silencioso).

**O que isso comprou, concretamente**: a verificação foi o que expôs o defeito
da própria verificação. Ela comparava com o jid de TELEFONE e por isso nunca
casava — e o `ErrUnverified` resultante levou à sonda que mediu os 397/399.
Sem a pós-condição, o envio teria devolvido sucesso e o erro de identidade
ficaria invisível até alguém notar mensagem faltando. Ver H34.

**Custo medido**: 1–2 ms de espera no caminho feliz (`waited=1ms`, `waited=2ms`
em duas execuções reais). A pós-condição não é cara; a ausência dela é.

### 6.9 — `listContacts` devolve PESSOAS, e o `wwebjs` devolve linhas

**O que o `wwebjs` faz**: `getContacts()` entrega a coleção como está.

**O que fazemos**: fundimos as duas linhas da mesma pessoa e descartamos o que a
página não chama de pessoa.

**Por que divergimos**: medido neste build, 390 pessoas têm DUAS linhas — uma
`@c.us` e uma `@lid` — e a ligação só existe no sentido `lid → phone`. Entregar
as linhas cruas daria 944 contatos para 544 pessoas, e o chamador não teria como
notar, porque as duas linhas parecem válidas.

Há ainda uma linha marcada `isPSA()` pela própria página: sentinela de sistema
que também é `isUser()`. Ela custou a H41 e só apareceu porque a capacidade
seguinte usou esta saída como entrada.

### 6.10 — `fetchContactAvatar`: ausência de foto é RESPOSTA, não erro

**O que o `wwebjs` faz**: `getProfilePicUrl()` devolve `undefined` quando não há
foto, misturando "não tem" com "não deu certo" num único valor vazio.

**O que fazemos**: `Avatar.Present` separa os dois, e só a página recusando
produz erro.

**Por que divergimos**: medido em 12 contatos ao vivo — 10 com foto, 2 sem, 0
exceções, 0 `null`. Quem não tem foto devolve resultado NORMAL com os campos de
URL ausentes. Colapsar isso em vazio faria o chamador tratar 2 de cada 12
pessoas como falha de rede, e tentar de novo para sempre.

**E a chamada não recebe um wid**, o que nenhuma referência diz: ela recebe um
objeto carregando `.id`. Passar o wid lança em `isNewsletter`. A forma veio de
LER a fonte do `profilePicResync` no próprio build — não de documentação, não da
referência.

### 6.11 — `onContact` escuta `change`; o evento do `wwebjs` para contatos não é o mesmo que para mensagens

**O que o `wwebjs` faz**: liga em `add` nas coleções, e é de onde o nosso
`onMessageMeta` tirou a palavra — verificada desde então para MENSAGENS.

**O que fazemos**: para CONTATOS ligamos em `change`, e mantemos `add` ligado por
precaução com o nome do evento viajando em cada evento.

**Por que divergimos**: medido em 90 s no roster ao vivo, `add` disparou **zero**
vezes e `change` **sete**. Dos 35 nomes vistos pelo catch-all, todos eram
`change` ou `change:<campo>`. Um roster muda por linha ATUALIZADA, não inserida.

O risco desta divergência é o que a torna importante: ligar em `add` aqui
instalaria sem erro e entregaria nada — falha sem sintoma, indistinguível de uma
agenda parada.

### 6.12 — `primeContactRoster` não tem baseline no `wwebjs`, e o nome promete demais

**O que o `wwebjs` faz**: nada. É a única das catorze sem equivalente lá, então
não houve o que copiar — nem entendimento, nem nome de módulo, nem issue aberta.

**O que fazemos**: chamamos `WAWebContactSyncBridge.doFullContactSync` e
reportamos a DIFERENÇA entre antes e depois, em vez de afirmar que "preparamos"
o roster.

**Por que a forma é essa**: medido, e o nome não sobrevive à medição. Não há
lacuna de pertencimento — dos 391 contatos que a `ChatCollection` referencia
pela regra da própria página, 391 já estavam no roster de 944. A chamada custa
**42 segundos**, devolve `undefined`, não acrescenta pessoas e não popula nomes
de agenda (`getName` responde para 1 de 944 antes e depois).

O que ela move é a ligação `lid → phone`, que é a única aresta cruzada deste
build e da qual a deduplicação depende. Cada ligação nova é uma pessoa que deixa
de ser contada duas vezes.

**A pós-condição é falsificável, e essa foi a parte difícil.** Como a chamada
devolve `undefined`, sucesso do comando não prova nada, e "o roster já estava
atualizado" era indistinguível de "o sync nunca rodou". A prova é a marca
`contact-sync-refresh-seconds`, que o refresh regenera a cada execução e que um
controle de 45 s ociosos mostrou não derivar sozinha. Com produção alterada para
não chamar o sync, o detector dispara contra a SPA real — e denuncia pelo tempo:
5 ms contra 42 s.

**Classificação**: MELHORIA INTENCIONAL, sem baseline no `wwebjs` — não conta
como item de paridade, e não deve ser lida como tal.

## 7. Passagem de auditoria — as seis, conferidas contra o que a matriz prometia

Feita em 2026-08-19, depois de a sexta capacidade entrar. O alvo parou de mudar,
então auditar passou a valer.

| # | capacidade | onde vive | testes | prova contra a SPA real |
|---|---|---|:---:|---|
| 1 | `livenessCheck` | `capabilities/liveness` | 6 | `TestRealSPALivenessAgainstProduction` |
| 2 | `getBrowserPid` | `runtime.Holder.BrowserPID` | 2 | coberto pelo `SIGKILL` em `runtime/` |
| 3 | `refreshOwner` | `capabilities/owner` | 7 | `TestRealSPARefreshOwnerAgainstProduction` |
| 4 | `onMessageMeta` | `capabilities/messagemeta` | 9 | `...MessageMetaInstalls` + `...MessageMetaDelivery` |
| 5 | `fetchMessages` | `capabilities/fetchmessages` | 8 | `TestRealSPAFetchMessagesAgainstProduction` |
| 6 | `backupNow` | `capabilities/backup` | 7 | `TestRealSPABackupRestoresToAWorkingSession` |

**A auditoria achou uma lacuna, e na capacidade que esta própria matriz ranqueia
em PRIMEIRO lugar**: `liveness` era a única das seis **sem prova contra a página
real**. Todas as outras cinco tinham. Ela foi construída primeiro, quando o
hábito de fechar cada fatia com uma corrida real ainda não estava firme, e
ninguém percebeu porque a suíte estava verde — que é exatamente o motivo de uma
passagem de auditoria existir depois de o alvo parar.

Fechada com `TestRealSPALivenessAgainstProduction`, e ela entregou um número que
não existia: **pior latência de 1 ms em 5 amostras** contra a página real. Isso
importa mais que o PASS — o `spa.Monitor` afirma no comentário ser *"seguro para
chamar de um caminho que segura slot limitado"*, e até aqui essa afirmação era
prosa. O teste falha se a ida e volta passar de 1 s, porque uma sonda de segundos
falsificaria o desenho sem quebrar mais nada.

**O `getBrowserPid` é o único sem teste com portão contra a SPA real, e de
propósito**: o que ele tem de provar é a recusa quando o pid deixa de significar
algo, e isso exige **matar o browser** — barato contra um perfil descartável,
caro contra um perfil que custou um humano com telefone para parear.


---

## §6.13 — bloquear e desbloquear contato

`wwebjs` expõe `Contact.block()` / `Contact.unblock()`, e chama
`window.Store.BlockContact.blockContact` / `unblockContact`. **O nome do módulo
conferiu; as assinaturas não** — lidas do `toString()` deste build:

```
blockContact({bizOptOutArgs, blockEntryPoint, contact, skipCtwa1pdNbfSignal})
unblockContact(contact, blockEntryPoint)
```

**Onde divergimos de propósito**, e a divergência é registrada porque a regra do
`CLAUDE.md` exige:

1. **Nós verificamos.** O `wwebjs` devolve o retorno da chamada. Nós lemos a
   `BlocklistCollection` antes e depois e recusamos com `ErrBlocklistUnchanged`
   quando a lista não se moveu. Um chamador que acredita ter bloqueado alguém
   **para de vigiar essa pessoa**; sucesso silencioso aqui custa mais que na
   maioria das capacidades.
2. **Nós checamos a pré-condição do próprio app.** O bundle carrega um `throw`
   para "block a pn contact without a chat". O `wwebjs` deixa lançar; nós
   devolvemos `ErrNoChatToBlockFrom`.
3. **Redundância é no-op, não erro.** Bloquear quem já está bloqueado devolve
   `AlreadyInState`, pela mesma razão da conversa já arquivada (H55).

**Onde a referência não ajudou**: ela não trata a assimetria das duas
assinaturas, porque em JavaScript passar o objeto errado para `unblockContact`
falha em runtime e não na leitura. Foi a sonda que respondeu — e a lição virou
entrada no `ARMADILHAS.md`.

## §6.14 — a superfície ainda aberta, com módulo já localizado

Uma sonda de enumeração devolveu ação **e** verificação para as quatro
restantes, então nenhuma delas está bloqueada por desconhecimento:

| `wwebjs` | ação neste build | verificação |
|---|---|---|
| `Message.forward` | `WAWebChatForwardMessage.forwardMessages` | id na conversa destino |
| `Message.star` | ~~`WAWebChatSendStarMsgsBridge.sendStarMsgs`~~ **`Cmd.sendStarMsgs`** | ~~`WAWebStarredMsgCollection`~~ **`msg.star`** |
| `Chat.mute` | `WAWebChatMuteBridge.sendConversationMute` | `WAWebMuteGetters.getIsMuted` |
| `Message.edit` | `WAWebSendMessageEditAction.sendMessageEdit` | `WAWebMessageEditUtils.msgTypeSupportsEditing` |

Ler as assinaturas com `String(fn)` **antes** de escrever cada uma é o que a
H58 comprou caro e a H59 comprou barato.

## §6.15 — editar mensagem

`wwebjs` expõe `Message.edit(content, options)` sobre
`window.Store.EditMessage.sendMessageEdit(msg, content, options)`. **A assinatura
conferiu** — é a primeira das cinco últimas em que conferiu — porque a função é
síncrona e o `toString()` a mostra inteira.

**Onde divergimos de propósito:**

1. **Nós perguntamos a guarda antes.** A `sendMessageEdit` rejeita por
   `canEditText`/`canEditCaption` com `"Cannot edit message"`, que não diz se o
   problema é o tipo, a autoria ou a janela. Nós consultamos o mesmo predicado
   primeiro e devolvemos `ErrNotEditable` **com o número da janela** (1200 s).
2. **Nós verificamos as duas metades.** Corpo mudou E `latestEditMsgKey` deixou
   de ser nulo. O `wwebjs` devolve o retorno da chamada. Aplicado-sem-registrar é
   estado real, e `Result.Recorded` o distingue de sucesso.
3. **Texto vazio é recusado.** Esvaziar mensagem é `revoke`, não `edit`, e fazer
   isso por engano através desta capacidade é surpresa destrutiva.

**O que a referência não podia dar**: que `latestEditMsgKey` está DEFINIDO em
mensagens nunca editadas. Isso é fato deste build, medido, e é o que separa uma
pós-condição real de uma que aprova tudo.

### Correção da §6.14 — a linha de favoritar estava errada nas DUAS colunas

A tabela acima foi montada a partir de nomes de módulo devolvidos pela sonda de
enumeração, e favoritar foi a única linha em que o nome enganou nas duas metades.
O experimento (`probe_star_test.go`) mediu:

| coluna | o que a tabela dizia | o que a página faz |
|---|---|---|
| ação | `WAWebChatSendStarMsgsBridge.sendStarMsgs` | **`Cmd.sendStarMsgs(chat, [msg], true)`** |
| verificação | `WAWebStarredMsgCollection` | **`msg.star`** — a coleção **lança** |

A contagem da coleção voltou `-1` antes e depois de um `star` que
demonstravelmente funcionou (`msgStar` foi de `false` para `true`). Uma
pós-condição construída sobre ela teria aprovado tudo, inclusive o fracasso.

Isto é a regra do `CLAUDE.md` sendo aplicada contra o próprio autor: **a medição
contrariou a hipótese, então a hipótese cai** — e a entrada é corrigida em vez de
ficar parecendo resolvida.

## §6.16 — favoritar mensagem

`wwebjs` expõe `Message.star()` / `unstar()`. **O nome do módulo que a busca
sugeria estava errado**, e a verificação também — ver a correção da §6.14 acima.
O que a página faz, medido por experimento:

```
Cmd.sendStarMsgs(chat, [msg], true)      // modelo do chat, ARRAY de modelos
Cmd.sendUnstarMsgs(chat, [msg], true)
```

**Onde divergimos de propósito:**

1. **Nós esperamos.** O `await` resolve antes de `msg.star` virar — medidos
   696 ms de atraso. O `wwebjs` devolve o retorno da chamada, o que na prática
   significa devolver antes de o efeito existir.
2. **Nós distinguimos "não moveu" de "travou"**, porque têm conserto diferente.
3. **Redundância é no-op.**

**Onde a referência não podia ajudar**: a assimetria não é dela — é uma
propriedade deste build, e só um experimento contra a página viva a mostra.

## §6.17 — silenciar conversa

`wwebjs` expõe `Chat.mute(unmuteDate)` / `unmute()`. **A camada que a nossa
enumeração nomeou primeiro era a errada**: `WAWebChatMuteBridge` recebe um objeto
cuja chave é `$MuteImpl3`, artefato de minificação. Nós dirigimos o **modelo**
`Mute`, cujos métodos são síncronos e legíveis.

**Onde divergimos de propósito:**

1. **Nós passamos `sendDevice: true` e testamos que passamos.** É o argumento que
   faz o efeito sair do dispositivo; sem ele o silenciamento é local e toda
   pós-condição continua verde. É a divergência mais importante desta seção.
2. **Nós usamos o conversor da própria página** (`calculateMuteExpiration`),
   inclusive a sentinela de "para sempre", e **reportamos o número escolhido** em
   vez de escondê-lo.
3. **Nós esperamos o modelo virar**, pela mesma razão da §6.16.
4. **`canMute` é consultado antes**, então a recusa nomeia a causa.

## §6.18 — o que resta

| `wwebjs` | estado |
|---|---|
| `Message.forward` | módulo localizado (`WAWebChatForwardMessage`), assinatura async — precisa de experimento |
| `Chat.setSubject` | **entregue** (H64) — `WAWebSetSubjectGroupAction.setGroupSubject` |
| participantes de grupo | **entregue** (H58) — prova ENTRE sessões; este build não confirma na mesma |
| link de convite de grupo | **parcialmente entregue** (H57) — `queryGroupInvite` trava |

## §6.19 — encaminhar mensagem

`wwebjs` expõe `Message.forward(chat)`. A assinatura **não estava legível em
lugar nenhum** — invólucro async, e o modelo do chat não tem método de
encaminhar. Veio do chamador, no bundle:

```
forwardMessagesToChats({msgs, chats, includeCaption, appendedText})
```

**Onde divergimos de propósito:**

1. **Não criamos conversa.** O fluxo do app usa `findOrCreateLatestChat`; abrir
   conversa com alguém para reenviar-lhe algo é ato maior do que encaminhar.
2. **Verificamos a cópia por conjunto de ids**, não por instante — a comparação
   por timestamp já produziu falso positivo neste módulo.
3. **`includeCaption` é escolha do chamador**, não constante: encaminhar mídia
   com e sem legenda são atos diferentes, e escolher por conta própria seria
   escolher o que outras pessoas leem.
4. **O campo `reasons` do erro sobrevive** até o chamador.

## §6.20 — renomear grupo

`wwebjs` expõe `GroupChat.setSubject(subject)`. A assinatura foi legível de
primeira porque o invólucro é síncrono:

```
setGroupSubject(chat, subject = "")
```

**Onde divergimos de propósito:**

1. **Passamos o assunto explicitamente e recusamos vazio.** O default do app é a
   string vazia, que **apaga o nome do grupo**. Renomear para nada não é
   renomear.
2. **Não palpitamos sobre admin.** Renomear é governado por configuração por
   grupo que pode permitir qualquer membro; a recusa da página passa adiante.
3. **A pós-condição é auto-medidora** e reporta o campo que carregou a mudança.
   Neste build é `chat.formattedTitle` — medido, não escolhido.

Com isto, a superfície do whatsapp-web.js está fechada exceto pelos dois
bloqueios nomeados: participantes (H58) e link de convite (H57).

## §6.21 — promover, rebaixar e sair de grupo

`wwebjs` expõe `GroupChat.promoteParticipants` / `demoteParticipants` / `leave`.

| operação | forma neste build |
|---|---|
| promover / rebaixar | `promoteParticipantsJob(group, participants, groupMetadata, isOffline)` — **quatro posicionais**, terceira forma do mesmo módulo |
| sair | `sendExitGroup(chat)` — um argumento, **modelo** |

**Onde divergimos de propósito:**

1. **`Verified` é false para mudança real**, pela razão medida na H58.
2. **Recusamos sair de grupo em que a conta não está.** Um no-op silencioso ali
   deixaria o chamador acreditando ter saído de algo em que nunca entrou.
3. **Sair não tem prova ao vivo, por desenho** — e está escrito em três lugares.
   Conta que sai de grupo que criou não volta sem convite, e o laboratório tem
   duas contas.

## §6.22 — limpar conversa, apagar conversa, nome de exibição

| `wwebjs` | forma neste build | prova ao vivo |
|---|---|---|
| `Chat.clearMessages` | `sendClear(chat, keepStarred)` | **não**, por desenho — destruiria o fixture |
| `Chat.delete` | `sendDelete(chat, syncToDevices=true)` | **não**, idem |
| `Client.setDisplayName` | `setPushname(name, onDone)` | **impossível** — `canSetMyPushname()` é false nesta conta |

**A conta de laboratório é BUSINESS**, medido por `canSetMyPushname` =
`!getIsSMB(this)` = false. Propriedade do fixture, não desta capacidade.

**Divergência**: o app faz `sendExitGroup` e depois `sendDelete` ao apagar um
grupo. Nós apagamos só a conversa — continuar no grupo sem a conversa é estado
real, e sair em silêncio seria decidir pelo chamador.

**Falta**: recado (`about`/status). Os dois módulos existem e **não são
funções** — objetos de chave `"0"`. Medido, registrado, não resolvido.

## §6.23 — baixar mídia

`wwebjs` expõe `Message.downloadMedia()`. A forma veio dos chamadores do app —
o método é async e o `toString()` não mostra nada:

```
downloadAndMaybeDecrypt({signal, downloadQpl, directPath, encFilehash,
                         filehash, mediaKey, mediaKeyTimestamp, type})
```

**Onde divergimos de propósito:**

1. **Verificamos criptograficamente.** `filehash` é o SHA-256 do texto claro, e
   nós o conferimos. É a única pós-condição do módulo que a página não consegue
   falsificar.
2. **Temos teto**, porque os bytes atravessam a fronteira CDP como base64 — um
   terço a mais que o payload — e recusamos pelo tamanho DECLARADO antes de
   baixar, para não gastar a banda de quem enviou.
3. **Reportamos a forma que a página devolveu**, em vez de assumir uma.
4. **`MIME` é dito como alegação de quem enviou**, não tipo farejado.

## §6.24 — grupos em comum (entregue) e enquete (não entregue)

| `wwebjs` | estado |
|---|---|
| `Contact.getCommonGroups` | **entregue** (H68) — leitura, `count=1` esperado e conferido |
| `Client.sendMessage(chat, poll)` | **não entregue** (H69) — medições preservadas |

**Grupos em comum, divergências:** o `null` da página significa "sou eu",
não "nenhum", e nós mantemos a distinção (`ErrIsSelf`); a renderização mostra
contagem, porque a lista de grupos de alguém é um perfil dessa pessoa.

**Enquete, o que ficou medido:** opção é `{name, localId}`, a mensagem carrega
`pollName`/`pollOptions`/`pollSelectableOptionsCount`/`pollContentType`/
`pollType`/`correctOptionIndex`, e `sendPollCreation({poll, chat, quotedMsg,
isWamoSub})` é da API. O que falta é o que `createPollCreationMsgData`
desestrutura.

## §6.25 — recado do contato (leitura)

`wwebjs` expõe `Contact.getAbout()`. Neste build o recado é um *text status*:

```
o("WAWebTextStatusAction").getTextStatus(wid)     // o WID
WAWebTextStatusCollection.TextStatusCollection.find(wid)
```

`getTextStatus` toma o **wid**; seu vizinho `findCommonGroups`, no mesmo pacote,
toma o **modelo**. Não há regra — há leitura.

**Onde divergimos de propósito:**

1. **Consultamos a guarda de recebimento antes.** Recurso desligado e contato sem
   recado são respostas diferentes, e o `wwebjs` devolve a mesma coisa nas duas.
2. **O texto nunca é renderizado**, só o comprimento — em RUNAS.
3. **Dizemos o que a prova ao vivo não provou**: o par tem recado vazio e já em
   cache, então a busca no servidor não foi exercitada.

## §6.26 — estado de entrega (`ack`)

`wwebjs` expõe `Message.getInfo()` — quem recebeu, quem leu. **Neste build isso
não está disponível para uma sessão headless**, e a medição diz por quê: a
`MsgInfoCollection` tem zero modelos contra 368 mensagens enviadas, porque o
detalhe por participante só é buscado quando o app abre a gaveta de informações.

O que existe é `msg.ack`, e é o que entregamos.

**Onde divergimos de propósito:**

1. **O enum vem da página.** Procuramos dentro de `WAWebAck` a entrada cujo valor
   é este ack e usamos o NOME dela — e reportamos de onde veio. Um build que
   renumerar seus estados muda só o campo `Raw`.
2. **`Unknown` é o valor zero.** Um ack que ninguém soube nomear não vira
   "pendente", que seria uma afirmação.
3. **`FromMe` é carregado**, porque o ack de uma mensagem recebida é sobre o que
   ESTA conta reconheceu — pergunta diferente de "eles leram a minha".
