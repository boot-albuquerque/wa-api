# Evidência do SPA real — medições congeladas

Tudo aqui foi **observado** em `https://web.whatsapp.com/`, não deduzido do
`whatsapp-web.js` nem de memória. O `wwebjs` é mapa de onde procurar e fonte de
nomes internos; **não** é verdade automática sobre formato, invariante,
comportamento, ciclo de vida ou erro. O que a página faz é o que vale.

Reproduzir: `WA_HEADLESS_REAL_SPA=1 go test -run TestRealSPA ./internal/headless/ -v`
(a sonda vive em `realspa_test.go` e é pulada por padrão).

**Zero PII neste arquivo.** As medições são booleanos, contagens, nomes de tag e
valores de `data-testid` — que são ganchos da própria Meta, não conteúdo de
usuário. O payload do QR (`[data-ref]`) é credencial pelos segundos em que vive:
foi registrado como **booleano de presença** e o valor nunca foi lido.

---

## M1 — Boot de perfil VAZIO até a tela de QR

**Data**: 2026-08-11 · **Ambiente**: Google Chrome 151.0.7922.76, macOS arm64,
`--headless=new`, perfil novo em `.lab/test-account-profile`, UA da versão
instalada (`Chrome/151.0.0.0`, sem token `HeadlessChrome`).

**Classificação final: `LOGIN_REQUIRED`** — correta.

| t | nós no DOM | `#pane-side` | `canvas[aria-label*="Scan"]` | canvases | `[data-ref]` | `#app` |
|---|--:|---|---|--:|---|--:|
| 0s | 211 | não | não | 0 | não | 1 |
| 9s | 340 | não | não | 0 | não | 1 |
| **15s** | 347 | não | **sim** | **1** | **sim** | 1 |

`stopped_via=browser.close`. Execução inteira: 17 s.

### M1.1 — O seletor de QR que já tínhamos está CORRETO

O `aria-label` medido é, literalmente:

```
Scan this QR code to link a device!
```

O `canvas[aria-label*="Scan"]` casa. **Verificado, não presumido** — era a
pergunta que o LOOP B1.2 existia para responder.

### M1.2 — Mas ele é frágil, e agora dá para dizer por quê

O `aria-label` é **texto de interface em inglês**. Numa conta cujo idioma seja
outro, ele muda: o mesmo elemento em português traria "Ler o código QR". O
seletor não está errado — está **preso a uma localidade**, e a conta de teste
por acaso está em inglês.

A página expõe um gancho independente de idioma, da própria Meta:

```
data-testid="link-device-qr-code"
```

Ele aparece **exatamente** quando o canvas aparece: ausente em t=0s e t=9s,
presente em t=15s, junto do primeiro canvas.

### M1.3 — Existe um estado intermediário que eu não conhecia

Aos **9 segundos** a página já está montada — 340 nós, `#app` presente — e já
carrega os `data-testid` da tela de vinculação:

```
link-device-qrcode-alt-linking-help
link-device-qrcode-alt-linking-hint
link-device-qrcode-alt-linking-tc
loading-spinner
```

…mas **não tem canvas nenhum**. É a tela de QR ainda carregando o código.

Hoje esse estado cai em `OTHER`, porque não tem `#pane-side` nem canvas, está no
host certo e tem texto. Isso é honesto — não é `UNRESPONSIVE`, a página
respondeu — mas é **indistinguível de "página que não reconhecemos"**, e as duas
pedem coisas opostas de quem chama: uma quer esperar, a outra quer investigar.

**Consequência para o CAP-05**: o ciclo de vida do QR tem pelo menos três
estados, não dois. `loading-spinner` + os `link-device-*` sem canvas é
"vinculação carregando"; com canvas é "QR pronto para leitura".

### M1.4 — Inventário completo de `data-testid` na tela de QR

```
wa-logo                              wa-wordmark
wa-square-icon                       wa-brand-arrow-out
wa-brand-arrow-right                 lock-outline
info-refreshed                       loading-spinner
link-device-qrcode-alt-linking-help  link-device-qrcode-alt-linking-hint
link-device-qrcode-alt-linking-tc    link-device-qr-code
```

Onze aparecem já aos 9 s; o `link-device-qr-code` é o décimo segundo e chega com
o canvas.

### M1.5 — O que M1 NÃO estabelece

- **Nada sobre `#pane-side`.** Nenhuma sessão pareada foi aberta, então o
  seletor de `READY` continua sem verificação. É o LOOP B1.4.
- **Nada sobre estabilidade dos `data-testid`** entre builds da Meta. Um
  `data-testid` é mais estável que texto em inglês, não é contrato.
- **Nada sobre outros idiomas.** A fragilidade do `aria-label` está
  argumentada pela forma (é texto de UI), não medida numa conta em português.
- **Os ~15 s até o QR** são UMA amostra, nesta máquina, nesta rede. Não é p50
  nem p95.

---

## M2 — O que já é verdade SEM sessão: candidatos a READY, eliminados

**Data**: 2026-08-11 · Mesmo ambiente do M1, perfil de laboratório **não
pareado** (medido, não suposto — ver M2.3).

Este bloco existe porque a pergunta do LOOP B1.4 — *`#pane-side` significa
READY?* — só tem resposta se houver um sinal que seja **falso sem sessão**. Um
sinal verdadeiro na tela de login não distingue nada.

### M2.1 — O inventário de módulos NÃO é sinal de prontidão

| sinal | quando fica verdadeiro na tela de LOGIN |
|---|---|
| `window.require` presente | **T+0,01s** |
| os 8 módulos do `RequiredAtStartup` resolvem | **T+0,01s** |

Os dois estão de pé antes de qualquer pareamento. **Desqualificados.**

Isso também explica o defeito do primeiro instrumento que escrevi: ele aceitava
`ref` como prova de conexão viva — e `ref` é o campo do **próprio QR**. Ficava
verdadeiro justamente na tela que deveria excluir. Dublê permissivo demais, só
que num instrumento de medição (ARMADILHAS §1).

### M2.2 — As chaves do `WAWebConnModel` sem pareamento

```
$1  $2  __changes  __fired  __initialized  __x_blockStoreAdds  __x_id
__x_isSMB  __x_meReadyTriggered  __x_platformField  __x_ref  __x_refExpiry
__x_refTTL  __x_smbTos  __x_stale  _uiObservers  collection  listenId
mirror  parent  revisionNumber
```

`WAWebSocketModel`: `__x_backoffGeneration  __x_isIncognito
__x_launchGeneration  __x_stale  __x_state  __x_stream  …`

**`__x_wid` está AUSENTE.** ~~É a identidade do dono, e ela só materializa com
sessão — o que a torna o melhor candidato disponível a "o motor pode agir".~~

> **CORRIGIDO em 2026-08-12 pelo M3.2.** A ausência é real; a explicação estava
> errada. `__x_wid` **não existe nesta build**, nem no perfil pareado — logo não
> "materializa com sessão" e nunca poderia ser candidato a nada. O erro foi
> inferir causa a partir de uma ausência num único perfil: sem o pareado, "ainda
> não apareceu" e "não existe" são indistinguíveis. A identidade do dono mora em
> `WAWebUserPrefsMeUser` (M3.1/M3.2).

Só **nomes de chave** foram lidos. Nome de chave é esquema da Meta; valor
seria dado da conta.

### M2.3 — A máquina de estados do socket é observável, e ela se nomeia

Amostragem a cada 250 ms, perfil não pareado:

| T | `socket_state` | `wid` | `meReady` |
|---|---|---|---|
| 0,01s | *(vazio)* | não | não |
| 5,30s | `OPENING` | não | não |
| 5,83s | `PAIRING` | não | não |
| 6,08s | **`UNPAIRED`** | não | não |

A Meta expõe o próprio veredito. `UNPAIRED` é dela, não interpretação nossa —
e é **prova direta de que este perfil não tem sessão**, independente do QR na
tela.

### M2.4 — O que M2 estabelece e o que não

**Estabelece**: `window.require` e o inventário de módulos não servem como
READY; ~~`__x_wid`,~~ `__x_meReadyTriggered` e `socket_state` são falsos sem
sessão, então **podem** discriminar.

**Não estabelece**: que qualquer um deles fica verdadeiro *quando* a sessão
existe, nem *quando* em relação ao `#pane-side`. Isso é o LOOP B1.4, e exige
perfil pareado. Nenhuma conclusão sobre READY foi tirada daqui.

> **CORRIGIDO em 2026-08-12 pelo M3.** `__x_wid` sai da lista: o campo não
> existe nesta build (M3.2). O "não estabelece" acima estava certo e é
> justamente o que o M3 foi medir — com o perfil pareado, e com a identidade
> lida onde o `wwebjs` a lê.

---

## M3 — Onde a identidade do dono realmente mora

**Data**: 2026-08-12 · Chrome 151.0.7922.76, macOS arm64, `--headless=new`, UA da
versão instalada. **Dois perfis, o MESMO instrumento**
(`TestRealSPAOwnerIdentityShape`):

| perfil | caminho | estado |
|---|---|---|
| **não pareado** (controle) | `.lab/test-account-profile` | sem sessão (M2.3) |
| **pareado** | `scripts/chromium-study/wa-session/profile` | sessão viva, medida |

Só leitura, sempre `headless`, encerramento por `stopped_via=browser.close` em
**todas** as execuções — as três originais e as cinco da rodada de correção.
Tamanho e contagem de arquivos do perfil pareado, checkpoint por corrida:

```
396M/2271 → 405M/2287 → 417M/2332 → 424M/2353   (M3 original, 3 corridas)
423M/2367 → 424M/2373 → 424M/2374               (correção: timeline, identity shape)
436M/2459 → 437M/2463 → 437M/2467               (correção: 2 controles negativos + confirmação)
```

**Valor final: 437M / 2467 arquivos**, sobre 8 corridas no total. **Cresceu** —
sincronização saudável; perfil que encolhe seria sinal de alarme.

Uma ressalva sobre ler estes números: a contagem de ARQUIVOS é o sinal estável,
o tamanho não. O `du -sh` arredonda e o LevelDB compacta, então bytes podem cair
alguns KB enquanto a contagem sobe (aconteceu duas vezes aqui: 424M→423M com
+14 arquivos, e 447904K→447900K com +4). Isso é ruído de bloco, não o sinal de
corrupção — a assinatura de corrupção é queda GRANDE com colapso da contagem.

Este bloco existe porque a primeira sonda de prontidão media o lugar errado, e
por isso **o M2.4 não podia ter sido respondido**: ela comparava `#pane-side`
contra `WAWebConnModel.__x_wid`, e esse campo **não existe nesta build**.

### M3.1 — O `wwebjs` não lê a identidade do `WAWebConnModel`

Referência lida (`whatsapp-web.js` **1.34.7**, a versão que o produto roda, em
`services/wa-worker/node_modules/whatsapp-web.js/`):

```js
// src/Client.js:351-364 — como o ClientInfo obtém o dono
wid: window.require('WAWebUserPrefsMeUser').getMaybeMePnUser()
  || window.require('WAWebUserPrefsMeUser').getMaybeMeLidUser()
```

O mesmo módulo aparece em `src/util/Injected/Utils.js:420-424` (remetente da
mensagem) e `:1267-1269` (rejeição de chamada). No caminho do `ClientInfo` a
identidade **não** sai do `WAWebConnModel` — o `Conn.serialize()` é espalhado no
objeto e o `wid` é **sobrescrito** logo em seguida pela leitura do
`UserPrefsMeUser`.

**Correção de uma afirmação absoluta.** Uma revisão independente apontou que a
versão anterior deste bloco dizia *"em nenhum ponto da release a identidade sai
do `WAWebConnModel`"*, e isso é **falso**. Conferido na build instalada, existe
exatamente **uma** leitura sobrevivente:

```js
// src/util/Injected/Utils.js:1254 — dentro de window.WWebJS.getProductMetadata
let sellerId = window.require('WAWebConnModel').Conn.wid;
```

É a única ocorrência de `Conn.wid` na release inteira (`grep -rn "Conn\.wid" src/`),
alcançada só por `src/structures/Product.js:56`, no catálogo de negócios. Todas
as outras leituras do `WAWebConnModel` na release pedem **outra coisa**: bateria
(`ClientInfo.js:65`, `Client.js:1068`), plataforma (`Client.js:2926`,
`Utils.js:386-387`), `canSetMyPushname` (`Client.js:1960`) e o alias do
`AuthStore`.

O contraexemplo **reforça** o achado em vez de enfraquecê-lo. Como `Conn.wid`
não existe nesta build — medido nos dois perfis, e agora também pelo acessador
(M3.2) — esse `sellerId` sai `undefined` e o `queryProduct` recebe um vendedor
vazio. É um caminho **quebrado nesta build**, não uma alternativa viável: se
fosse exercitado com alguma frequência, teria virado issue. Ou seja, é
corroboração independente de que a Meta removeu o campo — e é exatamente por
isso que o `Client.js` **sobrescreve** o `wid` do `Conn.serialize()` logo depois
de espalhá-lo.

Vale como lição de método: uma afirmação universal ("em nenhum ponto") custa um
`grep` para verificar e custa a credibilidade do documento inteiro quando não é
verificada. O achado real nunca precisou dela.

O mapa apontava para outro lugar o tempo todo. A página confirmou.

### M3.2 — O que a página mostrou nos dois perfis

Chaves do `WAWebConnModel` (só NOMES):

```
não pareado: … __x_meReadyTriggered __x_platformField __x_ref __x_refExpiry
             __x_refTTL __x_smbTos __x_stale …
pareado:     … __x_meReadyTriggered __x_platformField __x_refExpiry
             __x_smbTos __x_stale …
```

Duas leituras, ambas medidas:

1. **`__x_wid` está ausente nos DOIS.** Não é "a sessão não sabe quem é": o campo
   não existe nesta build. A conclusão do M2.2 — *"`__x_wid` é o melhor candidato
   disponível"* — estava **errada**, e foi errada por inferência a partir de uma
   ausência. Ausência num perfil sem sessão não distingue "ainda não materializou"
   de "não existe"; só o perfil pareado separa os dois casos.
2. `__x_ref`/`__x_refTTL` (os campos do QR) existem **apenas** no não pareado. É
   confirmação independente de que o perfil pareado não está na tela de login.

**O acessador, medido separadamente — e o motivo de ter sido preciso medi-lo.**
A listagem acima usa `Object.keys()` e a sonda de prontidão lia `c.__x_wid`.
**Os dois enxergam só propriedades próprias e enumeráveis.** Um *getter* de
protótipo `Conn.wid` — delegando ao `UserPrefsMeUser`, por exemplo — seria
invisível para ambos e ainda assim manteria o `Utils.js:1254` funcionando. Este
mesmo arquivo de teste já depende dessa dualidade na direção oposta: lê
`sk.__x_state` onde o `wwebjs` lê `Socket.state`.

Então a pergunta passou a ser feita **das duas formas**, em booleano coagido
dentro da página:

| perfil | `Conn.__x_wid` (campo próprio) | `Conn.wid` (acessador) |
|---|---|---|
| não pareado | `false` | `false` |
| **pareado** | `false` | `false` |

Ambas as formas negativas nos dois perfis. A afirmação "o campo não existe nesta
build" passa a se apoiar no acessador também, e não só na ausência da chave.
Continua **registrado e nunca asserido**: é o falsificador do achado no dia em
que a Meta repuser o campo.

Exports do `WAWebUserPrefsMeUser` — idênticos nos dois perfis:

```
clearGetMaybeMeLidUserCache clearGetMaybeMePnUserCache getMaybeMeDeviceId
getMaybeMeDeviceLid getMaybeMeDevicePn getMaybeMeDisplayName getMaybeMeLidUser
getMaybeMePnUser getMeDeviceForOutgoingPeerMessage getMeDeviceLidOrThrow
getMeDeviceOrThrow getMeDevicePnOrThrow_DO_NOT_USE getMeDeviceWids
getMeDisplayNameOrThrow getMeLidUserOrThrow getMePnUserOrThrow_DO_NOT_USE
getMeUserMatchingAddressingModeOrThrow getMeUserOrThrow
getMyPrimaryForOutgoingPeerMessage getUnknownId isMeAccount isMeDevice
isMePnUser isMePrimary isMeUserRestored isSerializedWidMe setMe
setMeDisplayName setMeLid setUnknownId
```

O veredito dos acessadores — **o sinal discrimina**:

| acessador | não pareado | pareado |
|---|---|---|
| `getMaybeMePnUser()` | `EMPTY` por 75 s inteiros | **`PRESENT` em T+0,01s** |
| `getMaybeMeLidUser()` | `EMPTY` por 75 s inteiros | **`PRESENT` em T+0,01s** |
| `getMaybeMeUser()` / `getMe()` / `getMeUser()` | `ABSENT` | `ABSENT` |

O objeto devolvido tem as chaves `_serialized server user`. **Só nomes de chave
foram lidos**; o valor é a identidade telefônica da conta e não foi lido,
registrado nem usado em asserção em nenhum momento.

### M3.3 — A identidade é PERSISTIDA, e isso muda o que ela prova

Linha do tempo do perfil pareado, amostragem a cada 250 ms
(`TestRealSPAReadinessTimeline`, já corrigido):

```
T+ 0.01s nodes=210  pane=false modules=8/8 identity=true  connWid=false meReady=false socket=
T+ 5.32s nodes=302  pane=false modules=8/8 identity=true  connWid=false meReady=true  socket=OPENING
T+ 5.82s nodes=302  pane=false modules=8/8 identity=true  connWid=false meReady=true  socket=CONNECTED
T+ 7.40s nodes=2589 pane=true  modules=8/8 identity=true  connWid=false meReady=true  socket=CONNECTED
```

| sinal | instante |
|---|---|
| identidade do dono | **T+0,01s** |
| inventário de módulos | T+0,01s |
| `meReadyTriggered` | T+5,32s |
| socket `CONNECTED` | T+5,82s |
| **`#pane-side`** | **T+7,40s** |

A identidade volta **antes de o socket sequer abrir**, porque sai de um store de
preferências do usuário, não da conexão. Consequência que não se adivinharia da
mesa: **ela prova que o perfil está PAREADO, não que a sessão está viva.** Uma
máquina offline desde a semana passada responderia igual de rápido. É a mesma
desqualificação do inventário de módulos no M2.1 — só que descoberta medindo.

### M3.4 — LOOP B1.4: `#pane-side` é marcador precoce?

**Não, nesta medição — é o ÚLTIMO dos cinco sinais.** Identidade, inventário,
`meReadyTriggered` e socket `CONNECTED` já estão de pé quando o painel aparece,
com 1,58 s de folga entre `CONNECTED` (T+5,82s) e o painel (T+7,40s).

Isto **não** promove `#pane-side` a marcador de prontidão. Estabelece só que,
neste boot, ele não chega antes dos sinais de sessão viva. A ordem inversa — a
que o classificador teria de temer — não foi observada.

### M3.5 — O que M3 estabelece e o que NÃO estabelece

**Estabelece:**

- a identidade do dono vive em `WAWebUserPrefsMeUser`, via `getMaybeMePnUser()`
  ou `getMaybeMeLidUser()`, e **discrimina** (falsa no não pareado por 75 s,
  verdadeira no pareado);
- `WAWebConnModel.__x_wid` **não existe** nesta build, nos dois perfis. O M2.2
  ficou com diagnóstico errado e está corrigido aqui;
- nesta corrida, `#pane-side` chega depois de todos os sinais de sessão.

**Não estabelece:**

- que a identidade signifique "a sessão pode agir" — ela é persistida, logo é
  sinal de PAREAMENTO. Um READY honesto precisa de `meReadyTriggered` e/ou do
  estado do socket, e o corte entre os dois **não foi medido**;
- nada sobre reconexão, expiração de sessão ou sessão revogada no servidor: só
  o boot foi observado;
- que os instantes sejam representativos. São **uma** amostra por perfil, nesta
  máquina e nesta rede. O painel apareceu em T+7,40s aqui e em T+15,61s numa
  corrida anterior do mesmo perfil — mais que o dobro. Não é p50 nem p95.

### M3.6 — Controle negativo EXECUTADO

A sonda foi reapontada para o defeito original (identidade lida de
`WAWebConnModel.__x_wid`) e rodada contra o perfil pareado:

```
T+  0.01s nodes=212  pane=false modules=8/8 identity=false connWid=false meReady=false socket=
T+  6.31s nodes=302  pane=false modules=8/8 identity=false connWid=false meReady=true  socket=OPENING
T+  6.81s nodes=302  pane=false modules=8/8 identity=false connWid=false meReady=true  socket=CONNECTED
T+  8.38s nodes=2603 pane=true  modules=8/8 identity=false connWid=false meReady=true  socket=CONNECTED
  owner identity at         NEVER
EARLY_MARKER: #pane-side appeared but the owner identity never did — the
classifier would report READY for a session that cannot act
--- FAIL: TestRealSPAReadinessTimeline (93.28s)
```

Falha com mensagem de asserção real, e gasta os 90 s inteiros de orçamento —
exatamente o comportamento do instrumento quebrado. Revertido antes do commit.

> **Nota (2026-08-12, rodada de correção).** O texto da asserção colado acima —
> *"the classifier would report READY for a session that cannot act"* — foi
> **reescrito** desde então, porque contradizia o próprio M3.3: a identidade é
> persistida, logo prova PAREAMENTO, não capacidade de agir. A colagem fica como
> registro fiel do que aquela corrida imprimiu; a mensagem atual fala em "sem o
> único sinal que mostra que o perfil está PAREADO".

### M3.7 — A correção da própria correção: a sonda repetia o defeito que consertou

Uma validação independente dos commits `2b776b1` e `17d951c` voltou com PASS e
cinco correções. A que importa está aqui, e ela é um caso-escola da **Regra 4**
do `CLAUDE.md` (*"o conserto do conserto também é um mecanismo"*).

**O defeito.** O `record()` parava a amostragem quando `pane && modules &&
identity` estivessem de pé. Os três são alcançáveis **sem a sessão jamais falar
com o servidor**: a identidade é persistida (M3.3), o inventário resolve na tela
de login (M2.1) e o painel renderiza do cache. Num perfil **pareado mas
offline**, portanto, o laço quebrava no instante em que o painel aparecia,
`connected` só era observado por acaso antes disso, e o teste imprimia
`SAFE_READY_MARKER` e **passava** com `socket CONNECTED at never`.

Ou seja: **um veredito alcançável por classe de perfil** — estruturalmente o
mesmo defeito da sonda `__x_wid` que este instrumento substituiu. O instrumento
não tinha internalizado o próprio achado.

**A correção, e por que gatear em vez de só asserir.** As duas opções eram
asserir `marks.connected != 0` no fim, ou **gatear a parada** no sinal. Gatear é
estritamente melhor: se apenas assertisse, um boot em que o painel chegasse
antes do socket seria reprovado por um sinal que chegaria um segundo depois —
falso negativo criado pela própria proteção. Gateando, o laço **espera** até o
orçamento. Os dois juntos é o que foi feito: o gate torna a espera honesta, e o
`t.Fatalf` transforma o estouro de orçamento em veredito.

**Custo no caminho saudável: zero, medido.** `CONNECTED` chega antes do painel
nas três corridas boas desta rodada, então a parada continua disparando no
painel:

| corrida | `CONNECTED` | `#pane-side` | duração |
|---|---|---|---|
| 1 | T+6,86s | T+8,35s | 11,36s |
| 2 (confirmação pós-revert) | T+7,07s | T+8,37s | 11,67s |

**Controle negativo EXECUTADO — duas pernas, porque uma não provaria o
contraste.** O sinal de socket foi tornado inalcançável (leitura apontada para
`sk.__x_stateNEGCONTROL`, campo inexistente), simulando o boot pareado-offline,
e rodado contra o **perfil pareado**.

*Perna A — com o gate novo: FALHA.*

```
  T+  0.05s nodes=195  pane=false modules=8/8 identity=true  connWid=false connWidGet=false meReady=false socket=
  T+  6.34s nodes=302  pane=false modules=8/8 identity=true  connWid=false connWidGet=false meReady=true  socket=
  T+  8.40s nodes=2593 pane=true  modules=8/8 identity=true  connWid=false connWidGet=false meReady=true  socket=
  #pane-side at             T+8.40s
  socket CONNECTED  at      NEVER
the socket never reported CONNECTED within 1m30s. This profile is paired — the
identity and the pane say so — but the session never reached the server, so
there is no boot here to compare instants in. A pane rendered from cache is not
readiness.
--- FAIL: TestRealSPAReadinessTimeline (93.07s)
```

*Perna B — MESMA mutação, gate antigo (`pane && modules && identity`): PASSA.*

```
  T+  7.32s nodes=2740 pane=true  modules=8/8 identity=true  connWid=false connWidGet=false meReady=true  socket=
  #pane-side at             T+7.32s
  socket CONNECTED  at      NEVER
SAFE_READY_MARKER on the identity axis: the owner identity was already there
when #pane-side appeared (identity T+0.01s <= pane T+7.32s + tolerance)
--- PASS: TestRealSPAReadinessTimeline (10.64s)
```

A perna B é o que torna o controle uma prova em vez de uma asserção sobre si
mesmo: **o mesmo defeito, no mesmo perfil, no mesmo minuto**, passa com o gate
antigo e falha com o novo. E a perna A gasta os 90 s inteiros do orçamento, que
é o comportamento correto de um perfil que nunca conecta. Ambas as mutações
foram revertidas antes do commit, e a corrida de confirmação (tabela acima,
linha 2) rodou sobre o código exatamente como ele vai ser commitado.

**O que isto acrescenta ao M3.5.** Continua valendo que os instantes não são p50
nem p95: o painel apareceu em T+7,40s, T+15,61s, T+8,35s e T+8,37s no MESMO
perfil, em corridas diferentes. Quatro amostras não são uma distribuição.

---

## M4 — Sessão que PERDE O SERVIDOR: o que muda, e o que não muda

**Data**: 2026-08-12 · **Loop**: 04.3A · **Ambiente**: Google Chrome
151.0.7922.76, macOS arm64, `--headless=new`, perfil **pareado**
(`scripts/chromium-study/wa-session/profile` via `WA_HEADLESS_PROFILE_DIR`),
`stopped_via=browser.close` nas duas pernas.

Tudo o que o M1–M3 mediu é **boot**. O próprio M3.5 diz: *"nada sobre
reconexão, expiração de sessão ou sessão revogada no servidor: só o boot foi
observado."* E o limite declarado da CAP-04 é que a sonda de liveness
*"descarta o modo de falha medido; não descarta UI montada sobre socket
morto"*.

**UI montada sobre socket morto é exatamente o caso não testado.** A única
forma honesta de olhar para ele é PRODUZI-LO.

### M4.1 — O método: cortar a rede da página, não fingir que cortou

A conexão foi severada pela emulação do próprio browser (`Network.enable` +
`Network.emulateNetworkConditionsByRule` com condição global `offline` +
`Network.overrideNetworkState`), encapsulada em
`engine.Tab.SetNetworkOffline`. É o que acontece quando a tampa do notebook
fecha.

**Não é logout, não é sinal, não é revogação.** Nada aqui desvincula a conta,
nada escreve no perfil, e a rede é restaurada antes de o browser descer — o
perfil nunca fica meio-severado.

Os **dois** comandos são necessários, e isto vale registrar porque a
documentação do protocolo entrega metade da queda em cada um:

| comando | o que faz | o que NÃO faz |
|---|---|---|
| `emulateNetworkConditionsByRule` | derruba as REQUISIÇÕES | não mexe em `navigator` — a aplicação continua se achando online |
| `overrideNetworkState` | vira `navigator.onLine` e dispara o handler de offline | sozinho, as requisições continuam passando |

Só um dos dois mediria a nossa emulação, não a sessão.

> **RESSALVA (LOOP 04.3B).** A frase acima vale para "imitar uma queda de rede
> completa", que era o objetivo desta perna. Ela **não** vale como regra geral, e
> foi o que quase custou a CAP-04: usar `emulateNetworkConditionsByRule`
> SOZINHO é a única forma de perguntar se a aplicação percebe **por conta
> própria**, porque as falhas que a produção enfrenta — rota em buraco negro,
> upstream morto, portal cativo, servidor pendurado — deixam `navigator.onLine`
> **verdadeiro** e não disparam evento nenhum. Ver M5.

O instrumento foi provado **antes** de ser apontado para a conta, contra
Chrome real e um servidor local:
`TestBrowserChainSeversAndRestoresThePageNetwork` verifica `navigator.onLine`,
o veredito do `fetch` e — a asserção que importa — que **o servidor não recebeu
a requisição** durante o corte.

Amostragem a cada **1 s**. Isto é mais grosso que o tick de 250 ms do M3, e de
propósito: o **F-20** trata de uma janela de ~500 ms que 250 ms não resolve, e
aqui nada está nessa escala — transição de rede e socket desistindo são
segundos a dezenas de segundos. Cada tick custa três round-trips.

### M4.2 — O que o socket faz: sai de `CONNECTED` em ~3 s, para `OPENING`

```
  [baseline] T+  0.00s online=true  pane=true  identity=true  meReady=true  socket=CONNECTED  nodes=2742  probe=APP_READY  liveness=true /APP_READY
NETWORK SEVERED at T+8.11s
  [severed ] T+  8.11s online=false pane=true  identity=true  meReady=true  socket=CONNECTED  nodes=2911  probe=APP_READY  liveness=true /APP_READY
  [severed ] T+ 10.19s online=false pane=true  identity=true  meReady=true  socket=OPENING    nodes=2925  probe=APP_READY  liveness=true /APP_READY
SEVERED WINDOW — 90 samples over 91s (0 with the signals unread), from the steady state at T+7.10s
```

| sinal | quando mudou, a partir do estado estável |
|---|---|
| `navigator.onLine` → `false` | +1,0 s |
| socket sai de `CONNECTED` → **`OPENING`** | **+3,1 s** |
| `#pane-side` → false | **NUNCA** (90 s) |
| identidade do dono → false | **NUNCA** (90 s) |
| `meReadyTriggered` → false | **NUNCA** (90 s) |
| `spa.Probe` sai de `APP_READY` | **NUNCA** (90 s) |
| liveness reporta NÃO vivo | **NUNCA** (90 s) |

O socket **reage**. O valor para onde ele vai é `OPENING` — **o mesmo estado do
boot normal** (M3.3: `OPENING` em T+5,32s, `CONNECTED` em T+5,82s). Ele ficou em
`OPENING` pelos 90 s inteiros, tentando reconectar.

> **CORREÇÃO (2026-08-12, LOOP 04.3B).** A versão anterior deste parágrafo dizia
> que o socket "reage em três segundos, **sem depender de keepalive**". A parte
> depois da vírgula **não estava medida** e está errada. Esta perna dispara os
> DOIS comandos, e o `overrideNetworkState` faz o navegador emitir o evento
> `offline` na página — então a saída rápida podia ser reação ao EVENTO, não
> percepção da queda. A perna não consegue separar as duas coisas, porque
> dispara as duas.
>
> Pior: o corte não fecha o socket. `TestBrowserChainSeversTheWebSocketTransport`
> mede, contra servidor local que conta quadros nos dois sentidos, que a
> emulação **engole os quadros em silêncio** — `readyState` fica `1` (OPEN),
> `onclose` nunca dispara. Logo a saída do `CONNECTED` é decisão do próprio SPA,
> e a pergunta "qual das duas?" era obrigatória.
>
> A M5 mediu. **São dois mecanismos, com tempos de ordem diferente:** o evento
> compra ~1,4–3,1 s; a percepção própria leva ~34 s. O "três segundos" desta
> perna é o EVENTO. Sem keepalive nenhum, é ~34 s.

Consequência que não se adivinharia: **"socket ≠ `CONNECTED`" não distingue
"está subindo" de "perdeu o servidor".** As duas situações têm o mesmo valor de
enum; o que as separa é **quanto tempo** o estado dura. Um liveness que leia só
o valor instantâneo tem de escolher entre matar sessão que está subindo e
manter sessão morta.

### M4.3 — O achado: a sonda de liveness atual NÃO percebe

`spa.Monitor.Check` respondeu **`Alive=true`, `Class=APP_READY`** em todas as
90 amostras com a rede cortada. `spa.Probe` respondeu `APP_READY` nas 90.

Isto **não** é defeito da implementação: ela faz exatamente o que o seu próprio
comentário do `livenessScript` diz que faz — prova que o renderer executa
e que a aplicação está montada. O comentário já declarava que "o que ela ainda
não descarta é UI montada sobre socket morto".

A diferença é que agora isso é **fato medido**, e não limite escrito por
prudência. É o risco de fase 6 na sua forma nova: numa frota com standby e
reciclagem, esta sessão seria contada como capacidade e receberia trabalho.

### M4.4 — `#pane-side` e identidade: prova concreta, não mais raciocínio

Os dois ficaram **verdadeiros pelos 90 s inteiros** com o servidor inalcançável.
O painel está renderizado e nada o desmonta; a identidade sai de um store de
preferências (M3.3) e a rede não a alcança.

Isto **promove o F-18 de raciocínio de boot para fato medido**: nenhum dos dois
pode ser sinal de liveness. Antes se sabia que eles chegam cedo demais; agora se
sabe que eles **permanecem** depois que a sessão perde o servidor.

### M4.5 — Recuperação: 2–3 s

```
NETWORK RESTORED at T+98.71s
  [recovery] T+ 98.71s online=true  pane=true  identity=true  meReady=true  socket=OPENING    probe=APP_READY
  [recovery] T+101.71s online=true  pane=true  identity=true  meReady=true  socket=CONNECTED  probe=APP_READY
  socket back to CONNECTED        +3.0s after the reference (T+101.71s)
  liveness alive again            n/a — it never stopped saying alive
```

A sessão voltou sozinha, sem QR, sem renavegação, sem reinício do browser. Duas
corridas do mesmo perfil deram +2,0 s e +3,0 s — num corte de 90 s. **Não** diz
nada sobre cortes longos nem sobre expiração.

### M4.6 — Controle negativo EXECUTADO: a MESMA sonda, a MESMA janela, sem cortar

Sem esta perna, "o sinal mudou" não é atribuível ao corte — poderia ser deriva
do próprio sinal. É o mesmo instrumento, no mesmo perfil, na mesma corrida do
`go test`, com a mesma janela de 90 s — mas **sequencialmente, com ~3 min entre
uma perna e outra** (severada 173,49s, controle 108,20s, na ordem em que
aparecem abaixo), cada uma com o seu próprio boot. A versão anterior desta
frase dizia "no mesmo minuto", o que é mais forte do que os tempos das pernas
permitem afirmar; o que a comparação de fato controla é máquina, rede, perfil e
instrumento, não simultaneidade.

```
=== RUN   TestRealSPALivenessUnderSeveredNetwork/control
    READY: pane T+7.34s · identity T+0.00s · meReady T+5.30s · socket CONNECTED T+5.81s
  [baseline] T+  0.00s online=true  pane=true  identity=true  meReady=true  socket=CONNECTED  nodes=2838  probe=APP_READY  liveness=true /APP_READY
  [control ] T+  8.07s online=true  pane=true  identity=true  meReady=true  socket=CONNECTED  nodes=2917  probe=APP_READY  liveness=true /APP_READY
    CONTROL WINDOW — 90 samples over 90s (0 with the signals unread), from the steady state at T+7.06s
      navigator.onLine went false     NEVER
      socket left CONNECTED           NEVER
      #pane-side went false           NEVER
      owner identity went false       NEVER
      meReadyTriggered went false     NEVER
      spa.Probe left APP_READY        NEVER
      liveness reported NOT alive     NEVER
      at the end of the window: socket="CONNECTED" pane=true identity=true probe=APP_READY liveness=true/APP_READY
    CONTROL: every signal held still over the same window with the network untouched
--- PASS: TestRealSPALivenessUnderSeveredNetwork (281.69s)
    --- PASS: TestRealSPALivenessUnderSeveredNetwork/severed (173.49s)
    --- PASS: TestRealSPALivenessUnderSeveredNetwork/control (108.20s)
```

O socket ficou em `CONNECTED` pelos 90 s. **A saída para `OPENING` na perna
severada é atribuível ao corte**, e não a instabilidade da rede da máquina nem
a comportamento espontâneo do SPA.

A perna severada também tem a sua própria precondição travada em código: se
`navigator.onLine` não ficasse falso, o teste **falha** dizendo que o corte
nunca chegou à página — porque aí "nada mudou" seria afirmação sobre a nossa
emulação, não sobre a sessão.

Contagem de arquivos do perfil pareado, na corrida colada acima: **2527 antes →
2557 depois**. Cresceu, como o F-18 prevê; não encolheu em momento nenhum
(2467 → 2524 → 2527 → 2557 ao longo das corridas do dia).

### M4.7 — O que M4 estabelece e o que NÃO estabelece

**Estabelece:**

- `WAWebSocketModel.Socket.__x_state` **reage** à perda do servidor, em ~3 s,
  saindo de `CONNECTED`;
- o valor para onde ele vai é `OPENING`, que é **indistinguível do boot** —
  logo o estado instantâneo não basta, é preciso duração;
- `#pane-side`, identidade do dono e `meReadyTriggered` permanecem
  **verdadeiros** com o servidor inalcançável, por 90 s. Nenhum deles é sinal de
  liveness (F-18, agora medido);
- `spa.Monitor.Check` e `spa.Probe` reportam a sessão **saudável** durante todo
  o corte. O limite declarado da CAP-04 está **confirmado em campo**;
- restaurada a rede, a sessão volta a `CONNECTED` em 2–3 s sozinha.

**Não estabelece:**

- **nada sobre sessão revogada, deslogada ou expirada no servidor.** Um corte de
  rede é o caso mais benigno da família: o servidor continua existindo e
  aceitando o mesmo credential. Revogação é outra medição, e não foi feita —
  fazê-la destruiria o ativo;
- **nada sobre cortes longos.** A janela foi de 90 s. Não se sabe se, em 10
  minutos, o SPA desiste, cai para outro estado, mostra QR ou fica em `OPENING`
  para sempre;
- **onde fica o corte de duração** que separa "subindo" de "morto". A medição
  diz que o corte existe e que é necessário; não diz o número. Isso é medição
  própria, com boots e severações repetidos;
- que os instantes sejam representativos. A saída do `CONNECTED` **com os dois
  comandos** deu, contando todas as corridas conhecidas deste corte:

  | corrida | saída do `CONNECTED` |
  |---|---|
  | 04.3A, três corridas | +3,1 s · +3,0 s · +3,1 s |
  | validação independente | **+2,1 s** |
  | 04.3B, controle negativo B (janela de 6 s) | **+1,4 s** |
  | 04.3B, perna severada reexecutada | +3,0 s |

  A faixa real é **+1,4 s a +3,1 s**, não "+3 s estável" como a versão anterior
  desta linha dizia — ela só tinha as três primeiras. Sete amostras continuam
  não sendo distribuição, e **a grade de amostragem é de 1 s**: nenhum destes
  números tem resolução melhor que isso, e a diferença entre +1,4 s e +3,1 s é
  de duas casas da grade. A volta variou mais: +2,0 s e +3,0 s no 04.3A, e
  +4,1 s, +5,0 s e +6,0 s nas três corridas do 04.3B — mas essas voltam de um
  socket que passou ~55 s em `OPENING`, não ~87 s, e o corte lá era só de
  transporte.

---

## M5 — O SPA percebe a queda SOZINHO? (corte só de transporte)

**Data**: 2026-08-12 · **Loop**: 04.3B · **Ambiente**: Google Chrome
151.0.7922.76, macOS arm64, `--headless=new`, perfil **pareado**
(`scripts/chromium-study/wa-session/profile` via `WA_HEADLESS_PROFILE_DIR`),
`stopped_via=browser.close` em todas as corridas.

### M5.1 — Por que esta medição era obrigatória

A M4 mediu com os DOIS comandos. A validação independente do `437316e` provou,
com instrumento próprio, que o corte chega mesmo ao transporte WebSocket — e
achou, no mesmo experimento, que **o Chrome não fecha o socket**: os quadros são
engolidos em silêncio, `readyState` fica `1` (OPEN) e `onclose` nunca dispara.

Isso muda a leitura da M4 inteira. Se o socket não é derrubado pelo browser, a
saída do `CONNECTED` em ~3 s foi **decisão do próprio SPA** — e havia duas
causas possíveis que a perna severada não consegue separar, porque dispara as
duas ao mesmo tempo:

- **(a)** o evento `offline` do DOM, que o `overrideNetworkState` faz o
  navegador emitir;
- **(b)** o SPA notando, por conta própria, que o seu tráfego parou.

A diferença decide a CAP-04 inteira. As falhas que a produção enfrenta —
**rota em buraco negro, upstream morto, portal cativo, servidor pendurado** —
deixam `navigator.onLine` **verdadeiro** e não disparam evento nenhum. Se o SPA
só reagisse ao evento, o socket **não seria discriminador em produção**: a
sessão ficaria em `CONNECTED` indefinidamente com o servidor morto, e a CAP-04
ficaria sem sinal algum, já que os sinais de tela caíram no F-18.

### M5.2 — O método: cortar os bytes sem avisar a página

`engine.Tab.SetTransportOffline` manda **só** o
`Network.emulateNetworkConditionsByRule`. Os bytes morrem;
`navigator.onLine` fica `true`; nenhum evento é disparado. É o corte mais
**estrito**, não o mais fraco: é a única forma em que "a aplicação percebeu"
significa que ela percebeu **sozinha**.

O corte é o mesmo objeto que a M4 usou pela metade — não é instrumento novo. E
o que ele faz com o WebSocket está travado em teste próprio (M5.6).

### M5.3 — A resposta

**O SPA percebe a queda SOZINHO — e leva ~34 s, não ~3 s.**

```
  READY: pane T+7.50s · identity T+0.02s · meReady T+5.58s · socket CONNECTED T+6.08s
  [baseline] T+  0.02s online=true  pane=true  qr=false identity=true  meReady=true  socket=CONNECTED  nodes=2707  probe=APP_READY  liveness=true /APP_READY
NETWORK SEVERED (transport-only) at T+8.32s
REACHABILITY — with the network untouched: OK · with the transport cut: FAIL
  [transport-only] T+  8.43s online=true  pane=true  identity=true  meReady=true  socket=CONNECTED  nodes=2948  probe=APP_READY  liveness=true /APP_READY
  [transport-only] T+ 42.54s online=true  pane=true  identity=true  meReady=true  socket=OPENING    nodes=2963  probe=APP_READY  liveness=true /APP_READY
DOM NETWORK EVENTS over the window: offline=0 online=0
TRANSPORT-ONLY WINDOW — 90 samples over 91s (0 with the signals unread), from the steady state at T+7.11s
  navigator.onLine went false     NEVER
  socket left CONNECTED           +35.4s after the reference (T+42.54s) (to "OPENING")
  #pane-side went false           NEVER
  owner identity went false       NEVER
  meReadyTriggered went false     NEVER
  spa.Probe left APP_READY        NEVER
  liveness reported NOT alive     NEVER
ANSWER: the SPA detects the stall ITSELF.
```

Três corridas, mesmo perfil, mesma máquina, mesma rede:

| corrida | corte em | saída do `CONNECTED` | desde o estado estável | desde o CORTE | eventos `offline` | volta ao `CONNECTED` |
|---|---|---|---|---|---|---|
| 1 | T+8,32s | T+42,54s | +35,4 s | **+34,2 s** | 0 | +6,0 s |
| 2 | T+8,50s | T+41,72s | +34,5 s | **+33,2 s** | 0 | +4,1 s |
| 3 | T+8,36s | T+42,58s | +35,4 s | **+34,2 s** | 0 | +5,0 s |

**Contado a partir do corte a estabilidade é de 1 s**, que é a própria grade de
amostragem. Três amostras não são distribuição, mas a ordem de grandeza é
inequívoca: **dezenas de segundos, não segundos.**

### M5.4 — Os dois mecanismos, com os números lado a lado

| o que o corte faz | evento `offline` | saída do `CONNECTED` |
|---|---|---|
| **os dois comandos** (M4) | dispara (medido: `offline=1`) | +1,4 s a +3,1 s |
| **só transporte** (M5) | **não dispara** (medido: `offline=0`) | **+33,2 s a +34,2 s** |

São **dois caminhos diferentes** no SPA, com uma ordem de grandeza entre eles. O
número que a M4 publicou é o do atalho — o que só existe porque a nossa emulação
se anunciou. O número que vale em produção é o outro.

Isto **não se adivinharia**: a hipótese de escrivaninha era "ou o SPA percebe, e
os 3 s valem, ou não percebe, e não há sinal". A resposta foi uma terceira,
que é boa notícia com um preço: **o sinal existe e é honesto, mas custa ~34 s
de latência de detecção.**

Os ~34 s cheiram a temporizador — keepalive, timeout de ping, prazo de resposta.
**Isso é leitura, não medição**: nada aqui olhou para o tráfego do SPA e nada
identificou o mecanismo. O que está medido é o INTERVALO, e é dele que a CAP-04
precisa.

### M5.5 — A precondição desta perna, e por que ela é sólida

A perna severada trava a sua precondição em `navigator.onLine` ficar falso —
que é exatamente o que esta perna **não** faz. Precisava de outra, e uma perna
sem precondição mede silenciosamente nada. São quatro checagens, em dois pares:

**O CORTE CHEGOU** (checagem positiva, `reportTransportOnlyVerdict`):

1. um `fetch` da própria página para um asset estático da origem do SPA
   responde **`OK` antes** do corte — isto é o controle interno: sonda que não
   consegue dizer `OK` jamais provaria nada dizendo `FAIL`;
2. o mesmo `fetch` responde **`FAIL` com a emulação ativa**.

O `fetch` mede HTTP, e o sinal em questão é WebSocket. A ponte **não é
suposição**: `TestBrowserChainSeversTheWebSocketTransport` (M5.6) prova, contra
servidor local, que este mesmo comando engole quadros de WebSocket nos dois
sentidos. Detalhes que fazem a sonda funcionar: a URL leva *cache-buster*
(acerto no cache HTTP ou no cache do service worker responderia `OK` com o
transporte morto), e o veredito é sobre **alcançabilidade**, não sobre status —
`fetch` resolve em 404 igual a 200 e só rejeita quando a requisição não pôde
ser feita, que é exatamente a distinção necessária.

**NADA ANUNCIOU O CORTE** (as duas que impedem a perna de virar a perna da M4):

3. `navigator.onLine` ficou **verdadeiro** pela janela inteira;
4. a página contou **zero** eventos `offline`, por listeners instalados antes do
   corte. O contador é monotônico, então zero no fim é zero o tempo todo.

A (4) é observação DIRETA do mecanismo; inferir "nenhum evento disparou" a
partir do `navigator.onLine` seria inferência. E o contador não é decoração: o
controle negativo B (M5.7) leu `offline=1`, provando que ele **conta** quando há
o que contar.

### M5.6 — O instrumento provado contra o transporte que importa

A prova do corte que existia era contra `fetch` HTTP, nunca contra WebSocket —
o transporte sobre o qual toda a M4 e toda a M5 se apoiam. Um Chrome que
aplicasse a emulação a `fetch` e deixasse um WebSocket já aberto continuar
entregando quadros transformaria tudo isto em achado sobre a nossa emulação, e
nada notaria.

`TestBrowserChainSeversTheWebSocketTransport` fecha o buraco: servidor WS local,
quadros contados **nos dois sentidos**, para os **dois** formatos de corte.

```
--- PASS: TestBrowserChainSeversTheWebSocketTransport (13.92s)
    --- PASS: .../full (6.20s)
    --- PASS: .../transport-only (6.16s)
    full: severed window — page sent 10, server received 0, page received 0, readyState=1 closed=false
    transport-only: severed window — page sent 10, server received 0, page received 0, readyState=1 closed=false
```

Ele mede **ENTREGA**, nunca intenção: a página continua chamando `send()` a
janela inteira (`page sent 10`), e é isso que torna o zero atribuível ao
transporte e não a um produtor que parou.

E confirma, dentro deste repositório, o que a validação independente achou:
**`readyState=1`, `closed=false`** — o Chrome não fecha o socket, engole os
quadros. É o fato que tornou a M5 obrigatória.

### M5.7 — Controles negativos EXECUTADOS

Colados em `EXECUCAO.md`, LOOP 04.3B. Em resumo: a mutação "o corte não
acontece" faz a precondição (1)+(2) matar a corrida
(`the transport cut never landed: the reachability probe still answered "OK"`),
e a mutação "esta perna usa `SetNetworkOffline`" faz a precondição (3) matar a
corrida (`navigator.onLine went false at T+8.56s; this leg exists to leave it
alone`) — essa segunda lendo, de quebra, `offline=1`, que é o controle positivo
do contador de eventos. No teste de WebSocket, não severar faz as duas
asserções falharem (`server received 10 frames`, `page received 10 frames`).
Todas as mutações foram revertidas antes do commit.

### M5.8 — O que M5 estabelece e o que NÃO estabelece

**Estabelece:**

- o SPA **percebe sozinho** que o transporte morreu, sem `navigator.onLine` e
  sem evento `offline`. **`WAWebSocketModel.Socket.__x_state` é discriminador
  para as falhas que a produção enfrenta**, e não artefato da nossa emulação —
  a CAP-04 tem fundação;
- o **preço** dessa fundação: **~33–34 s** de latência de detecção, uma ordem de
  grandeza acima dos ~3 s que a M4 publicou. Qualquer prazo de liveness ou de
  reciclagem tem de caber acima disto;
- a saída de ~1,4–3,1 s da M4 é reação ao **evento**, e não à queda. Corrigido
  na M4.2;
- a emulação **não fecha** o WebSocket em nenhum dos dois formatos de corte:
  quadros engolidos, `readyState` OPEN, `onclose` nunca (M5.6);
- o F-18 continua de pé, agora também sob corte silencioso: `#pane-side`,
  identidade do dono e `meReadyTriggered` ficaram **verdadeiros** pelos 90 s;
- o F-21 continua de pé: `spa.Monitor.Check` e `spa.Probe` responderam
  **saudável** nas 90 amostras das três corridas.

**Não estabelece:**

- **qual** mecanismo dá os ~34 s. Ninguém olhou o tráfego; "keepalive" é leitura
  plausível, não medição;
- **onde fica o corte de duração** que separa "subindo" de "morto". Continua sem
  número — e agora com uma restrição a mais, porque o limiar tem de ser maior
  que os ~34 s de detecção mais o tempo de `OPENING` de um boot saudável;
- **nada sobre cortes longos**: a janela foi de 90 s, dos quais ~55 s em
  `OPENING`;
- **nada sobre sessão revogada, deslogada ou expirada.** Continua sendo outra
  medição, e continua sendo a que destruiria o ativo;
- que os instantes sejam representativos: três corridas, uma máquina, uma rede,
  grade de 1 s.

### M5.9 — As outras duas pernas, reexecutadas com o instrumento novo

O contador de eventos entrou nas TRÊS pernas, então as outras duas foram
reexecutadas para que nenhuma asserção nova ficasse sem exercício:

```
severed   offline=1  socket left CONNECTED +3.0s   liveness ALIVE nas 90        PASS (171.03s)
control   offline=0  socket left CONNECTED NEVER   todo sinal parado            PASS (110.12s)
```

A perna severada confirma pela sétima vez o F-21 (`Alive=true`/`APP_READY` a
janela inteira) e dá a sétima amostra da saída rápida, +3,0 s. E o par
`offline=1` / `offline=0` nas duas pernas é a demonstração mais curta da M5:
**o mesmo instrumento, o mesmo perfil, a mesma janela — só muda quem avisa a
página, e o tempo de reação muda de 3 s para 34 s.**

**Contagem de arquivos do perfil pareado**: **2572 → 2686** ao longo das sete
corridas do loop (2572 → 2635 → 2646 → 2656 → 2675 → 2680 → 2682 → 2686).
Cresceu em todas; não encolheu em nenhuma. `stopped_via=browser.close` em todas.

---

## M6 — Quanto tempo o socket fica em `OPENING` num boot SAUDÁVEL (LOOP 04.3D, metade)

**Data**: 2026-08-12 · Perfil pareado, só leitura, headless, sem envio.
Instrumento: `TestRealSPAReadinessTimeline`, amostragem de 250 ms.

### M6.1 — Por que esta medição

O M5 fechou que o SPA percebe a queda sozinho, mas o valor de queda é `OPENING`
— **o mesmo estado do boot**. O valor instantâneo não separa "subindo" de
"perdeu o servidor"; só a DURAÇÃO separa. Sem ela, um liveness que leia o socket
escolhe entre matar sessão que está subindo e manter sessão morta.

### M6.2 — Os números

Cinco boots consecutivos do perfil pareado, na mesma máquina, em ~10 minutos:

| amostra | `meReadyTriggered` | socket `CONNECTED` | janela em `OPENING` | `#pane-side` |
|---|---|---|---|---|
| 1 | 5,04s | 5,54s | **0,50s** | 7,03s |
| 2 | 6,29s | 6,79s | **0,50s** | 8,40s |
| 3 | 6,30s | 6,57s | **0,27s** | 8,28s |
| 4 | 6,31s | 6,81s | **0,50s** | 8,31s |
| 5 | 6,30s | 6,80s | **0,50s** | 8,35s |

Histórico, mesma medida no M3.3: 5,32s → 5,82s = **0,50s**. Seis amostras ao
todo, todas **≤ 0,50s**.

Higiene: `stopped_via=browser.close` em 5/5, perfil **2686 → 2781 arquivos**,
cresceu em todas, encolheu em nenhuma.

### M6.3 — A separação, e por que ela é grande

```
boot saudável, em OPENING       0,27 – 0,50 s
sob corte, para SAIR de CONNECTED   33,2 – 34,2 s   (M5)
```

Cerca de **70×** entre os dois. Qualquer corte entre ~2 s e ~30 s separaria as
duas populações medidas — o que é confortável demais para ser aceito sem a
ressalva do M6.4.

### M6.4 — O que esta medição NÃO estabelece, e é o essencial

**Isto não é uma distribuição do fenômeno. São seis amostras de UM estado.**

Os `meReadyTriggered` das amostras 2–5 caem em 6,29 / 6,30 / 6,31 / 6,30 s — um
aperto de 20 ms. Um agrupamento assim, em corridas seguidas na mesma máquina e
na mesma rede, não é evidência de estabilidade do fenômeno: é evidência de que
**uma condição só foi amostrada cinco vezes**. Vale aqui a regra do projeto — se
a medição só confirma o que já se achava e nenhum resultado possível a
enfraqueceria, ela provavelmente não mediu o que interessa.

E o que interessa para o corte é a **cauda superior**, não a mediana. A pergunta
que decide é: *qual boot lento vira falso positivo?* Ela continua sem resposta.

Há indício direto de que a cauda existe e é longa: o `#pane-side` apareceu em
**7,40 s** numa corrida do M3.3 e em **15,61 s** noutra do MESMO perfil — mais
que o dobro. Se a janela de `OPENING` escalar com a lentidão do boot como o
painel escala, 0,5 s pode virar segundos sob CPU disputada ou rede ruim, e é aí
que o corte é decidido.

**Duas coisas faltam, e nenhuma é opinião:**

1. **A perna que deveria PIORAR** — boot sob CPU disputada e sob rede
   degradada, que é onde o mecanismo cobra o preço. Sem ela mediu-se a
   hipótese, não o mecanismo.
2. **A duração em `OPENING` SOB CORTE** — o M5 mediu o instante da SAÍDA de
   `CONNECTED`, não por quanto tempo o socket permanece em `OPENING` depois
   disso. A janela do M5 foi de 90 s e o comportamento além dela é o item
   "corte longo" do `Next`.

**Portanto o corte NÃO sai desta medição**, e propor um número agora seria
escolha de mesa com aparência de dado. O que o M6 entrega é a metade saudável da
distribuição, medida, e o piso de 34 s do M5 do outro lado.

### M6.5 — Um limite do INSTRUMENTO, não do fenômeno

Os valores caem em 0,27 s e 0,50 s porque a amostragem é de 250 ms: são **um ou
dois ticks**. O valor verdadeiro está em algum ponto abaixo de 0,5 s e este
instrumento não consegue dizer onde — é a **F-20** aparecendo nesta medida
também. Para a decisão do corte isso não atrapalha (a separação é de 70×), mas
qualquer afirmação mais fina que "menos de meio segundo" seria quantização
apresentada como medida.

---

## M7 — A cauda: `OPENING` num boot saudável sob CPU disputada e rede degradada (LOOP 04.3E)

**Data**: 2026-08-12/13 · **Ambiente**: Google Chrome 151.0.7922.109, macOS
**arm64**, 10 núcleos, Go 1.26.0, `--headless=new`, perfil **pareado**
(`scripts/chromium-study/wa-session/profile` via `WA_HEADLESS_PROFILE_DIR`),
só leitura, sem envio. Instrumento: `TestRealSPABootUnderStress`, **21 boots
numa única execução**, amostragem de **50 ms**.

Comando:

```
WA_HEADLESS_REAL_SPA=1 \
WA_HEADLESS_PROFILE_DIR=<repo>/scripts/chromium-study/wa-session/profile \
go test -run TestRealSPABootUnderStress ./internal/headless/ -v -timeout 120m
```

### M7.1 — Os três resultados, nomeados ANTES da corrida

Escritos antes de qualquer boot, e repetidos no comentário de cabeçalho do
próprio teste para que não pudessem ser ajustados depois:

- **CONFIRMA** "a janela escala com a lentidão do boot": o máximo das pernas
  estressadas sobe materialmente acima do das não estressadas, **e sobe com o
  nível** de degradação em vez de aleatoriamente.
- **ENFRAQUECE**: o máximo estressado sobe só para a mesma ordem do não
  estressado, ou sobe sem relação com o nível.
- **FALSIFICA**: o máximo estressado fica **igual ou abaixo** do não estressado
  **com a contenção comprovadamente aplicada** — isto é, com o boot em volta
  medidamente mais lento e com dilatação e RTT mostrando fome real. Uma janela
  que não se mexe enquanto tudo à volta se mexe é uma janela limitada por outra
  coisa que não esta máquina, que é a **hipótese ALTERNATIVA**: um handshake do
  lado do servidor.

O resultado foi **os dois**, cada um num eixo — o que nenhuma das duas hipóteses
previa sozinha. Ver M7.4.

### M7.2 — O método

**Um instrumento, não dois.** É o mesmo `sampleReadiness` que produziu o M6,
com o tick virando parâmetro. `readinessTick` continua em 250 ms e o
`TestRealSPAReadinessTimeline` continua chamando com ele: **os números do M6
não foram rebaselinados**. A ponte entre as duas medições é a perna
`unstressed` do M7, intercalada com as outras e amostrando o mesmo fenômeno.

**Resolução, declarada (F-20).** O M7 amostra a 50 ms, **quatro vezes mais
fino** que o M6, cujos 0,27/0,50 s eram um ou dois ticks (M6.5). Não é promessa:
cada amostra custa uma ida e volta, então o espaçamento REAL é medido e
reportado por perna, e é ele que vale como resolução — ver M7.5.

**Duas âncoras, porque o M6 tinha uma só e não disse que era escolha.** O M6
publicou "janela em `OPENING`" como `connected − meReady`. O M7 reporta essa —
`openingM6`, para poder comparar — e também a direta, `primeira amostra em
OPENING → CONNECTED`. As duas andam juntas, e a direta é **50–70 ms menor em 18
dos 21 boots**; em três boots de CPU da rodada 1 a diferença chega a **80, 110 e
280 ms**, e os 280 ms de `cpu-1x` r1 são **42% da janela daquele boot** (0,66 s)
— que é justamente o máximo de todas as pernas de CPU, o número que a
falsificação do eixo CPU usa. Os três *outliers* estão todos onde o espaçamento
observado foi pior, o que é o que se esperaria de um artefato de resolução e não
de uma propriedade das âncoras. Nenhuma conclusão do M7 vira sob a âncora direta
— verificado linha a linha no N2a-EVAL, e sob ela a falsificação da CPU fica
**mais forte** (0,49 s contra 0,68 s, em vez de 0,66 contra 0,73).

**Intercalação dentro de UMA execução, na mesma máquina.** Comparar execuções
separadas mediria também o estado da máquina. A perna não estressada abre cada
rodada — é a ponte com o M6 e quer o mesmo lugar sempre — e as seis restantes
**rodam por rotação**, para que uma máquina que aqueça ou derive ao longo da
execução não some o mesmo deslocamento à mesma perna três vezes. A ordem
executada, colada da corrida:

```
r1/unstressed -> r1/cpu-0.5x -> r1/cpu-1x -> r1/cpu-2x -> r1/net-mild ->
r1/net-moderate -> r1/net-heavy -> r2/unstressed -> r2/cpu-1x -> r2/cpu-2x ->
r2/net-mild -> r2/net-moderate -> r2/net-heavy -> r2/cpu-0.5x -> r3/unstressed ->
r3/cpu-2x -> r3/net-mild -> r3/net-moderate -> r3/net-heavy -> r3/cpu-0.5x ->
r3/cpu-1x
```

**Os eixos variam UM DE CADA VEZ**, não cruzados. Um cruzamento completo seriam
9 condições e 27 boots do perfil pareado, e a fase 4C mediu que boots repetidos
são como uma sessão pareada se degrada; um de cada vez também **atribui** o
movimento ao eixo que se mexeu.

**A CPU disputada é real, e quantificada em vez de afirmada.** São processos do
sistema operacional girando (`/bin/sh -c 'while :; do :; done'`), não
goroutines: goroutines disputariam primeiro o `GOMAXPROCS` deste processo e
poderiam matar de fome o amostrador deixando o browser relativamente em paz —
uma caricatura que não esfomeia o que está sendo medido não mede nada. Três
níveis contra os 10 núcleos reais: **5, 10 e 20** processos. O quanto pegou sai
em três números por boot: a **dilatação** de um laço de CPU fixo do processo de
teste contra a mesma medida sem carga, o `vm.loadavg` do kernel e o **RTT da
própria sonda**, que é a mesma pergunta feita ao renderizador enquanto o boot
acontece.

**A rede degradada NÃO desliga a página, e isso é estrutural antes de ser
medido.** O 04.3A já pagou por isto: `overrideNetworkState` faz o navegador
**disparar um evento `offline`**, e o M5.4 mediu o preço — o SPA reage ao
ANÚNCIO em 1,4–3,1 s e ao travamento real em 33,2–34,2 s. Uma medição de rede
lenta que anunciasse uma queda estaria medindo o anúncio.

Por isso o método novo é um tipo que **não consegue expressar queda**:
`engine.NetworkDegradation` não tem campo `Offline`, e `degradedConditions`
escreve `false` no único ponto de construção. `overrideNetworkState` nunca é
enviado neste caminho. Três níveis:

| nível | latência adicionada | download | upload |
|---|---|---|---|
| `net-mild` | 150 ms | 1,5 MB/s | 512 KB/s |
| `net-moderate` | 400 ms | 500 KB/s | 200 KB/s |
| `net-heavy` | 900 ms | 200 KB/s | 100 KB/s |

A degradação entra **ANTES da navegação**: a pergunta é sobre um boot LENTO, e
uma degradação ligada depois de o bundle ter chegado seria degradação de nada.

### M7.3 — Os números. O MÁXIMO primeiro, porque é ele que decide

**O maior tempo em `OPENING` medido em 21 boots foi 1,36 s** — perna
`net-heavy`, rodada 1. Contra os **33,2 s** que o M5 mediu para o socket SAIR
de `CONNECTED` com o servidor perdido, a razão é **24,4×**.

Por perna, `n=3` cada. `p95` é por **posto mais próximo**: com `n=3` o p95 **É o
máximo**, e está escrito assim em vez de disfarçado com interpolação, que
inventaria um valor nunca observado.

| perna | n | **máx** (âncora M6) | p95 | mediana | máx (direta) | pior espaçamento | dilatação média |
|---|---|---|---|---|---|---|---|
| `unstressed` | 3 | **0,73 s** | 0,73 s | 0,49 s | 0,68 s | 0,49 s | 0,98× |
| `cpu-0.5x` (5 proc) | 3 | **0,51 s** | 0,51 s | 0,51 s | 0,46 s | 0,32 s | 1,56× |
| `cpu-1x` (10 proc) | 3 | **0,66 s** | 0,66 s | 0,55 s | 0,49 s | 1,41 s | 2,27× |
| `cpu-2x` (20 proc) | 3 | **0,50 s** | 0,50 s | 0,49 s | 0,44 s | 5,11 s | 9,61× |
| `net-mild` | 3 | **0,70 s** | 0,70 s | 0,53 s | 0,64 s | 0,31 s | 0,95× |
| `net-moderate` | 3 | **0,76 s** | 0,76 s | 0,75 s | 0,70 s | 0,30 s | 0,95× |
| `net-heavy` | 3 | **1,36 s** | 1,36 s | 1,30 s | 1,31 s | 0,30 s | 0,94× |

Agrupado: **não estressado n=3, máx 0,73 s** · **estressado n=18, máx 1,36 s**.

Os 21 boots, um por linha, com a dilatação que cada um realmente sofreu:

```
perna         rodada  proc  dilatação  meReady    CONNECTED  janela(M6)  janela(direta)
unstressed    r1      0      1,01×     T+5,29s    T+5,78s    0,49s       0,44s
cpu-0.5x      r1      5      1,32×     T+6,16s    T+6,67s    0,51s       0,40s
cpu-1x        r1     10      2,36×     T+5,38s    T+6,05s    0,66s       0,38s
cpu-2x        r1     20     22,86×     T+5,96s    T+6,41s    0,46s       0,38s
net-mild      r1      0      0,97×     T+5,20s    T+5,90s    0,70s       0,64s
net-moderate  r1      0      0,96×     T+6,16s    T+6,92s    0,76s       0,70s
net-heavy     r1      0      0,94×     T+6,08s    T+7,44s    1,36s       1,31s
unstressed    r2      0      0,95×     T+5,16s    T+5,62s    0,46s       0,40s
cpu-1x        r2     10      2,12×     T+6,22s    T+6,68s    0,46s       0,41s
cpu-2x        r2     20      2,82×     T+5,77s    T+6,27s    0,50s       0,44s
net-mild      r2      0      0,93×     T+6,17s    T+6,66s    0,49s       0,44s
net-moderate  r2      0      0,94×     T+6,15s    T+6,90s    0,75s       0,70s
net-heavy     r2      0      0,94×     T+12,79s   T+14,08s   1,30s       1,24s
cpu-0.5x      r2      5      1,13×     T+6,14s    T+6,65s    0,51s       0,46s
unstressed    r3      0      0,97×     T+5,14s    T+5,86s    0,73s       0,68s
cpu-2x        r3     20      3,13×     T+8,00s    T+8,49s    0,49s       0,42s
net-mild      r3      0      0,95×     T+5,08s    T+5,61s    0,53s       0,48s
net-moderate  r3      0      0,94×     T+5,17s    T+5,93s    0,75s       0,70s
net-heavy     r3      0      0,94×     T+6,09s    T+7,38s    1,28s       1,23s
cpu-0.5x      r3      5      2,23×     T+6,13s    T+6,64s    0,51s       0,44s
cpu-1x        r3     10      2,32×     T+5,47s    T+6,02s    0,55s       0,49s
```

Higiene: `stopped_via=browser.close` em **21/21**, e `SingletonLock` **ausente**
depois da parada limpa em **21/21**. A contagem de arquivos do perfil NÃO é
usada como sinal de higiene aqui — a **H10** mediu "o perfil nunca encolhe"
**FALSA** para um boot só (457 → 456 numa parada limpa), então seria observável
falso.

### M7.4 — A resposta, e ela é dividida por eixo

**A CPU não move a janela. A rede move.** Nenhuma das duas hipóteses previa
isso; cada uma acertou metade.

**Eixo CPU — a hipótese primária está FALSIFICADA, pelo critério escrito antes
da corrida.** A perna `cpu-2x` chegou a **22,86×** de dilatação num boot — o
processo de teste levando 22 vezes mais para o mesmo laço — e produziu **0,46 s**
de janela, **abaixo** dos 0,73 s da não estressada. O máximo de TODAS as pernas
de CPU (0,66 s) fica abaixo do máximo da não estressada (0,73 s). A contenção
não é alegada: ela **mexeu tudo em volta** — o `#pane-side` foi de T+7,3s para
T+10,2s, o RTT da sonda de 1 ms mediano para 2,66 s de pico. Tudo se mexeu
menos a janela.

**Eixo rede — a hipótese primária está CONFIRMADA, e monotonicamente.**
0,70 → 0,76 → **1,36 s** conforme a latência vai de 150 a 400 a 900 ms. Sobe
**com o nível**, que era exatamente a condição escrita antes.

**O que junta os dois é a evidência mais forte da corrida: a janela responde ao
eixo LATÊNCIA e não responde à lentidão do boot.** E isso não repousa num boot
só — repousa nas correlações sobre os 21 boots, calculadas no N2a-EVAL:

```
r(latência adicionada, janela), 21 boots        = +0,952   <- o eixo que move
r(dilatação de CPU, janela), 21 boots           = −0,234
r(dilatação de CPU, janela), só pernas CPU (n=9)= −0,347   <- sinal NEGATIVO
r(nº de burners, janela), só pernas CPU (n=9)   = −0,282
```

O sinal **negativo** dentro das pernas de CPU é o ponto: quanto mais esfomeada a
CPU, **menor** a janela, o oposto da hipótese primária. E dentro de cada perna,
livre do confundimento entre pernas, `r(meReady, janela)` não tem sinal
consistente em nenhuma delas.

**Dois boots que mostram isso um a um, e são um "eu não teria adivinhado":**

- **`cpu-2x` r3**: `meReady` em T+8,00 s, **2,2 s mais lento** que os irmãos da
  mesma perna (T+5,96 e T+5,77) → janela de **0,49 s**, exatamente no meio dos
  0,46 e 0,50 deles. Mesma demonstração, num eixo diferente.
- **`unstressed` r3**: o `meReady` mais **RÁPIDO** da sua perna (T+5,14 s)
  produziu a **MAIOR** janela não estressada (0,73 s), enquanto o `unstressed` r2,
  no mesmo patamar de lentidão (T+5,16 s), produziu a **MENOR** (0,46 s). Direção
  oposta à hipótese, com dois boots indistinguíveis em lentidão.

O boot `net-heavy` da rodada 2 — o mais lento das 21 corridas, `meReady` em
**T+12,79 s** contra T+5–6 s nos outros, e ainda assim janela de **1,30 s**,
entre os 1,36 e 1,28 dos irmãos — é a mesma demonstração e continua verdadeiro.
Mas ele **não** é o apoio da claim, e o N2a-EVAL mostrou por quê: é um ponto de
alta alavancagem, e no sentido contrário ao que serviria. Retirá-lo leva
`r(meReady, janela)` de **+0,454 para −0,009** — ou seja, o único ponto que
produzia qualquer correlação positiva aparente era ele, por confundimento de
perna e não por sinal. Além disso a lentidão dele é da fase **pré-`meReady`**,
que sob 200 KB/s é limitada por **vazão**, enquanto a janela é pós-`meReady` e
limitada por **latência**: são gargalos diferentes, então "boot lento, janela
normal" é menos surpreendente nele do que nos dois boots acima.

A janela não segue a lentidão do boot. Ela segue a **latência de rede**,
especificamente. Isso é o que se esperaria de um **handshake com número fixo de
idas e voltas**: o custo é `k × RTT` e não depende da CPU local nem de quanto o
resto do boot demorou. É a hipótese ALTERNATIVA, mas com uma emenda que ela não
tinha: o handshake é limitado pelo servidor **no número de trocas**, não no
tempo — então RTT alto o infla, mesmo que a CPU não.

**Isto é leitura, não medição.** Ninguém olhou o tráfego e nada aqui identificou
o mecanismo, exatamente como no M5.4 com os ~34 s. O que está MEDIDO é que a
janela responde à latência e não responde à CPU.

### M7.5 — A resolução do instrumento, medida em vez de declarada

O tick pedido é 50 ms. O espaçamento **observado** entre amostras foi mediano de
**52 ms** em todas as 21 corridas — o tick segurou na mediana. O pior
espaçamento de cada corrida é outra história, e é onde a honestidade custa:

| perna | pior espaçamento | janela medida |
|---|---|---|
| `unstressed` | 0,30–0,49 s | 0,46–0,73 s |
| `cpu-0.5x` | 0,30–0,32 s | 0,51 s |
| `cpu-1x` | 0,59–1,41 s | 0,46–0,66 s |
| `cpu-2x` | **2,71–5,11 s** | 0,46–0,50 s |
| `net-*` | 0,30–0,31 s | 0,49–1,36 s |

Na perna `cpu-2x` o **pior tick é dez vezes maior que a janela que ele mede**.
Isso tem de ser dito, e tem de ser dito o que faz com a conclusão:

- **o erro do amostrador não é unidirecional, e dizer que é seria a versão
  confortável.** A janela é a diferença de duas marcas, ambas enviesadas para
  TARDE: `Ŵ = W + δc − δm`. Se um buraco cobre o **FIM**, a janela infla; se
  cobre o **INÍCIO**, ela **encolhe**, e encolhe para um valor pequeno mas **não
  nulo** — exatamente indistinguível de 0,46 s. O engolimento TOTAL (`Ŵ ≈ 0`,
  `meReady` e `CONNECTED` na mesma amostra) é o caso fácil; o **parcial** é o que
  se disfarça de dado bom, e foi ele que o texto anterior desta seção não
  descartou;
- **o que descarta o engolimento do início é o delta entre as DUAS âncoras**, e
  ele está na tabela do M7.3: `openingFirst − meReady` deu **80 / 60 / 70 ms**
  nas três corridas de `cpu-2x`. Um delta desse tamanho **exige** amostras
  separadas por ~70 ms ENTRE `meReady` e a primeira vista de `OPENING` — logo os
  buracos de 2,71–5,11 s estavam em **outro ponto da linha do tempo** (o
  candidato óbvio é a primeira sonda contra uma página fria com 20 burners), e
  não na vizinhança das marcas. Se um buraco as tivesse engolido, as duas âncoras
  teriam colapsado na **mesma** amostra e o delta seria **zero**. **É este
  número, e não o sentido do enviesamento, que sustenta a falsificação.** Apoio
  independente: `meReady` cai em T+5,08–6,22 s em 20 dos 21 boots, e um
  deslocamento de segundos na marca de início exigiria um `meReady` verdadeiro em
  ~T+1 s, incompatível com o bundle do SPA ter de carregar antes;
- as pernas de rede, que são as que produziram o **máximo**, tiveram pior
  espaçamento de **0,30 s** contra janela de **1,36 s**. O número que decide é o
  medido com a melhor resolução da corrida.

### M7.6 — O que o M7 entrega sobre o corte: o limite INFERIOR, e só ele

| | |
|---|---|
| pior janela de boot saudável medida (21 boots, 7 condições) | **1,36 s** |
| latência de DETECÇÃO do M5 (a mais RÁPIDA das três) | **33,2 s** |
| razão entre as duas | 24,4× |

**O que o M7 entrega sobre o corte é o limite INFERIOR e só ele**: `C > 1,36 s`
na faixa medida (até 900 ms de latência adicionada). O limite **SUPERIOR** é a
permanência em `OPENING` sob corte, que é o N2b e continua **não medido** (M7.8
item 6).

O tempo total até declarar uma sessão morta seria **`33,2 s + C`**: os 33,2 s do
M5 são latência de **DETECÇÃO** — o instante de SAIR de `CONNECTED` — e
**somam-se** ao corte em vez de o limitarem. Uma duração DENTRO de `OPENING` e
uma latência que a PRECEDE são grandezas de eixos diferentes, e não competem.

Por isso **a razão de 24,4× NÃO é um limite superior para o corte**. Ela é
contexto orçamentário: diz que a folga para escolher `C` é pequena perto do
custo de detecção que já se paga, e nada além disso. A formulação anterior desta
seção — "a cauda invade o piso de detecção? não" — era **malformada**, e o mesmo
erro estava congelado no comentário de `detectionFloor` em `realspa_test.go`;
ambos corrigidos no N2a (ver `HOUSEKEEP.md` H12).

**A resposta tem alcance, e o alcance é a faixa medida**: 900 ms de latência
adicionada. A janela responde à latência de forma clara e o M7 não mediu enlaces
de RTT plurissegundo — satélite, celular congestionado, portal cativo lento.
Extrapolar a curva de três pontos até lá seria exatamente a escolha de mesa que
o M6.4 recusou. O que está medido é: **até 900 ms de latência adicionada, o
limite inferior do corte é 1,36 s.**

### M7.7 — Os controles do confundidor, EXECUTADOS, nos dois sentidos

**Lado negativo — zero em todas as corridas degradadas.** `offline=0` e
`online=0` nas **21** corridas, e `navigator.onLine` **nunca** foi falso em
nenhuma das ~2.500 amostras. Duas testemunhas independentes, porque inferir
"nenhum evento disparou" a partir do `navigator.onLine` seria inferência.

Antes de ser medido, é **estrutural**: `overrideNetworkState` não é enviado
neste caminho, e `NetworkDegradation` não tem como expressar queda. O guarda
disso é `TestDegradedConditionsCannotExpressAnOutage`, e ele **morde** —
controle negativo executado, com `Offline` virado para `true`:

```
=== RUN   TestDegradedConditionsCannotExpressAnOutage
    network_test.go:42: degradedConditions({Latency:0s DownloadBytesPerSecond:0
    UploadBytesPerSecond:0}) built an OUTAGE: Offline=true. A degraded network is
    a network that is THERE — with the outage announced the SPA reacts in ~3s and
    without it in ~34s (EVIDENCIA-SPA.md M5.4), so this flag decides what the
    measurement is about
--- FAIL: TestDegradedConditionsCannotExpressAnOutage (0.00s)
```

A mutação foi revertida antes do commit.

**Lado positivo — o contador CONTA.** Um listener que nunca contou nada não
prova nada lendo zero. No último boot da execução, **depois** de a janela
daquele boot ter sido cronometrada e o contador lido, o mesmo listener, na mesma
instância de página, recebeu um evento de verdade via `SetNetworkOffline`:

```
POSITIVE CONTROL: the same listener that just read offline=0 is now given a genuine event
POSITIVE CONTROL PASSED: offline 0 -> 1, online 0. The counter counts, so the
zeros above are measurements and not silence.
```

É mais forte que o controle do M5.7, que era outra corrida: aqui é o **mesmo
listener que acabara de ler zero**. E não custou boot nenhum ao perfil pareado.

### M7.8 — O que esta medição NÃO estabelece

**1. Não é uma distribuição, e cai na MESMA armadilha do M6.4 dentro de cada
perna.** As três amostras de `net-heavy` deram 1,36/1,30/1,28 s — 80 ms de
aperto. As de `cpu-0.5x` deram 0,51/0,51/0,51 s. Isso é, de novo, **uma condição
amostrada três vezes**. O que o M7 produziu é uma **curva de resposta entre
condições**, não a cauda de uma distribuição dentro de uma condição. A pergunta
"qual boot lento vira falso positivo" está respondida **para as condições que eu
impus**, e a variância natural do fenômeno numa condição fixa continua
desconhecida.

**2. Não mediu latência acima de 900 ms**, que é justamente onde o único eixo
que move a janela continuaria movendo. É a limitação que dá alcance ao "não" do
M7.6.

**3. Não identificou o mecanismo.** "Handshake de k idas e voltas" é leitura
plausível da resposta à latência, como "keepalive" era leitura plausível dos
~34 s no M5.4. Ninguém olhou o tráfego.

**4. Não mediu o eixo CPU com resolução à altura da janela.** Ver M7.5: em
`cpu-2x` o **pior** espaçamento é 10× a janela. A conclusão sobrevive porque a
resolução **na vizinhança das marcas** foi de ~70 ms, e isso está **medido**: é o
delta `openingFirst − meReady` de 80/60/70 ms nas três corridas de `cpu-2x`, que
só pode existir se houve amostras separadas por ~70 ms entre as duas âncoras.
**Não** sobrevive porque o erro apontasse para o outro lado — não aponta: um
buraco sobre a marca de INÍCIO encolhe a janela, e o texto anterior desta linha
afirmava uma unidirecionalidade que os dados não dão.

**5. Não mediu os eixos CRUZADOS.** CPU esfomeada *e* rede degradada ao mesmo
tempo não foi amostrado — foi um de cada vez, por 21 boots contra 27. Se houver
interação entre os dois, ela não está aqui.

**6. Não mediu a PERMANÊNCIA em `OPENING` sob corte**, que é a outra metade que
o M6.4 nomeou e continua aberta: o M5 mediu o instante da SAÍDA de `CONNECTED`,
não por quanto tempo o socket fica em `OPENING` depois disso.

**7. Uma máquina, uma rede, um perfil, 21 boots.** A rede base é a desta casa;
a "latência adicionada" soma-se a um RTT de base que não foi caracterizado.

**8. O controle positivo do confundidor rodou numa perna NÃO degradada.**
`lastBoot` sai de `rotatedConditions(conditions, 3)`, e a rotação põe `r3/cpu-1x`
no fim — uma perna sem degradação de rede. Então o que ficou provado é que **o
código do sentinela conta**, na mesma instância de página que acabara de reportar
zero; que ele contava nas 20 instâncias anteriores é generalização por código
idêntico, não medição. Combinado com o apoio **estrutural** (`overrideNetworkState`
nunca é enviado no caminho de degradação — `SetNetworkDegraded` → `applyConditions`,
verificado no código) é suficiente, e por isso a limitação fica **registrada em vez
de fechada**: fechá-la de vez custaria escolher `lastBoot` numa perna `net-*`, o
que exige uma **re-medição completa de 21 boots** do perfil pareado por um ganho
marginal — e o controle do confundidor já rodou nos dois sentidos (M7.7). Decisão
do N2a: não re-executar o SPA.

**9. Também sobre esta medição: o M7 não publicou os brutos.** As amostras
individuais — em particular o **espaçamento observado imediatamente ao redor de
`meReady` e de `CONNECTED` em cada boot** — não estão nesta evidência. O argumento
do M7.5 sobre o engolimento parcial é uma **inferência a partir do delta entre
âncoras**, não a leitura direta do espaçamento local. Publicar esse intervalo para
os três boots de `cpu-2x` fecharia a questão diretamente, e **não exige o perfil
pareado**: sai da re-impressão dos logs da corrida, se estiverem guardados.

**10. O corte NÃO sai daqui, e não era para sair.** Este nó é medição. O que ele
entrega é a segunda perna: a metade saudável do M6, agora com a cauda amostrada
sob adversidade, e a informação — que não estava em lugar nenhum — de que **o
eixo que importa para essa cauda é a latência de rede, não a CPU**.

---

## M8 — A PERMANÊNCIA em `OPENING` sob corte: o limite SUPERIOR (N2b)

**Data**: 2026-08-13 · **Ambiente**: Google Chrome 151.0.7922.109, macOS 15.6,
**arm64**, 10 núcleos, Go 1.26.0, `--headless=new`, perfil **pareado**
(`scripts/chromium-study/wa-session/profile` via `WA_HEADLESS_PROFILE_DIR`), só
leitura, sem envio. Instrumento: `TestRealSPAOpeningPersistenceUnderLongCut`,
**5 boots em duas execuções**, amostragem de **1 s** do lado Go e de **100 ms**
dentro da página.

Comandos, os dois:

```
# execução A — as quatro pernas
WA_HEADLESS_REAL_SPA=1 \
WA_HEADLESS_PROFILE_DIR=<repo>/scripts/chromium-study/wa-session/profile \
go test -run TestRealSPAOpeningPersistenceUnderLongCut ./internal/headless/ -v -timeout 150m

# execução B — a REPOSIÇÃO da perna que a rede da casa desqualificou (M8.6)
WA_HEADLESS_REAL_SPA=1 \
WA_HEADLESS_PROFILE_DIR=<repo>/scripts/chromium-study/wa-session/profile \
go test -run 'TestRealSPAOpeningPersistenceUnderLongCut/long-cut-2$' ./internal/headless/ -v -timeout 60m
```

### M8.1 — Por que esta era a metade que faltava

O M5 mediu **latência de DETECÇÃO**: 33,2–34,2 s do corte até o socket **SAIR**
de `CONNECTED`, dentro de uma janela de 90 s. Ele nunca mediu o que o socket faz
DEPOIS — o próprio M5.8 registra "nada sobre cortes longos", e os ~55 s de
`OPENING` daquela janela são tudo que ela chegou a ver.

O M7 entregou o limite **INFERIOR** do corte: `C > 1,36 s`, a pior janela de
`OPENING` de um boot saudável sob adversidade. **Nada limitava `C` por cima.**

E o limite superior não é detalhe de orçamento: um corte `C` sobre
tempo-em-`OPENING` **só dispara se o socket ainda estiver em `OPENING`** quando
`C` expira. Se o SPA desiste, tenta por outro estado, mostra QR ou volta sozinho
para `CONNECTED`, então `C` tem **teto** — e pior: o observável que o corte vigia
pode **sumir por baixo dele**, e o corte teria de tratar essa transição como
sinal próprio.

### M8.2 — Os três resultados, nomeados ANTES da corrida

Escritos antes de qualquer boot e repetidos no comentário de cabeçalho do
próprio teste, para que não pudessem ser ajustados depois:

- **CONFIRMA** a hipótese primária (laço de retentativa sem estado terminal,
  logo sem limite superior prático dentro da janela): o socket entra em
  `OPENING` depois do corte e **ainda está em `OPENING`** quando a janela fecha,
  em TODAS as corridas, sem estado intermediário — e o gravador da página, dez
  vezes mais fino que o amostrador Go, não mostra transição que o amostrador
  tenha perdido.
- **ENFRAQUECE**: o socket sai de `OPENING` em algumas corridas e não em outras,
  ou oscila entre `OPENING` e outra coisa sem assentar. O observável é então
  instável, e um corte teria de tratar a própria oscilação como o seu sinal.
- **FALSIFICA**: o socket sai de `OPENING` num instante reprodutível em todas as
  corridas, para um estado do qual não volta. Esse instante **É** um limite
  superior para `C`, e é a hipótese ALTERNATIVA.

O resultado foi **CONFIRMA**, em 3 de 3 corridas válidas. Ver M8.4.

### M8.3 — O método

**O corte é o da M5, não um terceiro.** `engine.Tab.SetTransportOffline` — o
caminho de **OUTAGE**. Os bytes morrem, `navigator.onLine` fica `true`, nenhum
evento é disparado. Não é o caminho de **DEGRADAÇÃO** do M7:
`engine.NetworkDegradation` não consegue expressar queda por construção, e as
duas coisas são objetos separados exatamente por isso. O amostrador é o
`watchSession` do M5 e a sonda de sinais é a do M6/M7 — nenhum harness novo.

**A janela é de 12 minutos a partir do corte, e o número é escolhido, não
herdado.** Os 90 s do M5 são o que deixou esta pergunta aberta, e o item
`corte longo` do `EXECUCAO.md` nomeia "o que o SPA faz depois de dez minutos sem
servidor". A detecção sozinha come ~31–42 s dessa janela, então 12 min a partir
do corte deixam ~11,3 min de `OPENING` para observar — passa dos dez minutos com
folga, e a folga existe porque **uma janela que termina exatamente no instante
interessante não distingue "nunca saiu" de "saiu logo depois de a gente parar de
olhar"**.

**Duas resoluções, e as duas MEDIDAS em vez de declaradas (F-20).** O amostrador
Go roda a 1 s, três idas e voltas por tique. A 1 s ele **não resolve** um estado
pelo qual o socket passasse por 300 ms, e a ausência disso sairia como "nenhuma
transição" — que é a resposta ERRADA, não a resposta grosseira. Por isso entrou
um **gravador dentro da página**, a 100 ms, que não custa ida e volta nenhuma e é
**lido** uma vez por tique do Go. Ele não substitui a linha do tempo do Go: é a
testemunha que diz se ela perdeu alguma coisa. Os dois leem a MESMA expressão
(`socketStateReadJS`), extraída para um único lugar — duas grafias da mesma
leitura divergiriam de forma invisível, e a divergência apareceria justamente
como "transição que o amostrador perdeu".

**Ordem INTERCALADA na execução A.** A execução inteira leva ~53 min. Um controle
tomado antes de todos os cortes ou depois de todos ficaria num extremo do que a
máquina e a rede fizeram nesse tempo. Ele é o **segundo** dos quatro, e as
corridas de corte têm vizinhos dos dois lados.

**A recuperação é medida nas TRÊS corridas de corte**, não em uma. É o lado do
FALSO POSITIVO da escolha: uma sessão que TERIA se recuperado é exatamente o que
um `C` mal escolhido mata. O orçamento de espera é de 120 s e não os 60 s do M5,
porque um orçamento calibrado no caso curto que expirasse reportaria "a sessão
nunca voltou" quando o que houve foi "paramos de esperar".

### M8.4 — Os números

**Em 3 de 3 corridas válidas o socket entrou em `OPENING` e AINDA ESTAVA em
`OPENING` quando a janela fechou, 12 minutos depois do corte.** Nenhum estado
intermediário, nenhuma oscilação, nenhum QR, nenhum retorno espontâneo.

| corrida | exec | corte em | saiu do `CONNECTED` | **detecção** | **tempo em `OPENING`** | estado seguinte | eventos `offline` | recuperação |
|---|---|---|---|---|---|---|---|---|
| `long-cut-1` | A | T+8,29s | T+42,51s | **+34,2 s** | **≥ 685,48 s** | **nenhum** | 0 | **+2,01 s** |
| `long-cut-2` | B | T+8,43s | T+39,53s | **+31,1 s** | **≥ 688,38 s** | **nenhum** | 0 | **+3,02 s** |
| `long-cut-3` | A | T+8,29s | T+50,55s | **+42,3 s** | **≥ 677,69 s** | **nenhum** | 0 | **+5,02 s** |

O `≥` é literal e não é hedge: a corrida acabou com o socket ainda em `OPENING`,
então o número é o que a JANELA viu, não a duração do estado. E o início real é
ainda mais cedo que a coluna sugere — o gravador da página viu `OPENING` de 0,10
a 0,57 s antes do amostrador Go (M8.5).

Linha do tempo completa de uma corrida, colada, que é a forma de todas as três:

```
LONG-CUT-1 WINDOW — 719 samples over 720s (0 with the signals unread), cut at T+8.29s
  observed spacing: median 1.00s, worst 1.07s (requested tick 1.00s)
  SOCKET TIMELINE (Go sampler, one line per contiguous state)
    T+    8.39s .. T+   41.51s  "CONNECTED"   33.11s over 34 samples
    T+   42.51s .. T+  727.98s  "OPENING"     685.48s over 685 samples
PAGE-SIDE TRAIL — 2 transitions in 7211 ticks at 100ms (capped=false)
    T+    0.01s -> "CONNECTED"
    T+   42.41s -> "OPENING"
  DOM NETWORK EVENTS over the window: offline=0 online=0
  STAY: STILL in OPENING at T+727.98s — entered T+42.51s (34.2s after the cut),
        held AT LEAST 685.48s. This is a statement about the window, not about eternity
TRANSPORT RESTORED at T+728.99s, after 720.00s of outage
RECOVERY — 120 samples over 120.00s (0 with the signals unread)
  socket back to CONNECTED  +2.0s after the reference (T+731.00s)
    T+  728.99s .. T+  729.99s  "OPENING"
    T+  731.00s .. T+  848.56s  "CONNECTED"
```

**A resposta, explícita: NÃO existe limite superior para `C` DENTRO DA JANELA
MEDIDA.** O socket não tem estado terminal sob corte nos 12 minutos observados;
não há teto abaixo de **677,69 s** (11,3 min). Combinado com o M7:

| | |
|---|---|
| limite INFERIOR (M7, pior boot saudável sob adversidade) | **C > 1,36 s** |
| limite SUPERIOR (M8, dentro da janela de 12 min) | **nenhum abaixo de 677,69 s** |
| latência de DETECÇÃO que se SOMA (M5 + M8, ver M8.7) | **31,1–42,3 s** |

O tempo total até declarar uma sessão morta continua sendo **`detecção + C`**,
não `C`. **A faixa de escolha para `C` é larguíssima**, e o que a aperta não é o
teto — é o custo de detecção que já se paga antes de `C` começar a contar, e o
falso positivo do outro lado (M8.7).

**Recuperação: doze minutos de queda NÃO degradam a volta.** +2,01 / +3,02 /
+5,02 s, dentro da faixa que o M4.5 mediu para corte curto (2–3 s) e o M5 para
90 s (4,1–6,0 s). Isto é um "eu não teria adivinhado": a hipótese de escrivaninha
razoável era que um socket doze minutos em retentativa voltasse com *backoff*
longo, e ele volta como se nada tivesse acontecido. **É o lado do falso positivo,
e ele é caro**: uma sessão cortada por 12 minutos ainda estava a ~3 s de voltar
sozinha.

### M8.5 — A resolução do instrumento, medida nos dois níveis

O tique pedido do lado Go é 1 s. O espaçamento **observado** foi **mediano de
1,00 s** nas cinco janelas, com pior caso de **1,01 a 1,13 s** — o tique segurou,
e nenhuma amostra teve os sinais não lidos (`0 with the signals unread` em 5/5).

O que o gravador da página acrescenta é a única coisa que o amostrador Go não
podia dizer sobre si mesmo:

| corrida | `OPENING` visto pelo gravador (100 ms) | pelo amostrador Go (1 s) | **atraso do amostrador** | transições vistas pelo gravador |
|---|---|---|---|---|
| `long-cut-1` | T+42,41s | T+42,51s | **0,10 s** | 2 em 7211 tiques |
| `long-cut-2` (B) | T+38,96s | T+39,53s | **0,57 s** | 2 em 7169 tiques |
| `long-cut-3` | T+50,02s | T+50,55s | **0,53 s** | 2 em 7172 tiques |
| `long-control` | — (nunca saiu de `CONNECTED`) | — | — | 1 em 7180 tiques |

Duas leituras, e as duas importam:

1. **O amostrador Go atrasa de 0,10 a 0,57 s — sempre menos que um tique**, que é
   exatamente o que se esperaria e agora está MEDIDO em vez de suposto;
2. **o gravador não achou NENHUMA transição que o amostrador tenha perdido.** Duas
   transições por janela de corte (`CONNECTED` no instante da instalação,
   `OPENING` depois), contra os dois estados contíguos que o amostrador reportou.
   Em ~7.180 tiques de 100 ms por janela, **o socket não passou por nenhum
   estado intermediário**. É esta linha que fecha a armadilha do F-20 aqui: a
   ausência de transição é medida com resolução de 100 ms, não inferida de uma
   grade de 1 s.

O cap de 2.000 entradas do gravador **não foi atingido em nenhuma janela**
(`capped=false` em 5/5), então nenhuma trilha está truncada.

### M8.6 — Os controles, todos EXECUTADOS

**Controle NEGATIVO (o obrigatório): a perna sem corte.** Mesma janela, mesmo
amostrador, mesma sonda, rede intocada:

```
LONG-CONTROL WINDOW — 717 samples over 720s (0 with the signals unread)
  SOCKET TIMELINE (Go sampler, one line per contiguous state)
    T+    8.10s .. T+  727.71s  "CONNECTED"   719.61s over 717 samples
PAGE-SIDE TRAIL — 1 transitions in 7180 ticks at 100ms (capped=false)
  DOM NETWORK EVENTS over the window: offline=0 online=0
  STAY: the socket NEVER entered OPENING
CONTROL: the socket held CONNECTED for the whole 720.00s window with the network
untouched, and the same reader reported OPENING on the cut legs. The reader discriminates.
```

É ele que torna as três corridas de corte legíveis: **o mesmo leitor, na mesma
execução, reportou `CONNECTED` por 719,61 s quando não havia corte e `OPENING`
por ~680 s quando havia.** Sem isto, "ficou em `OPENING`" não se distinguiria de
um instrumento que reporta `OPENING` sempre.

**O CORTE CHEGOU**, nas três: `REACHABILITY — untouched: OK · with the transport
cut: FAIL`, com o `OK` de antes servindo de controle interno — sonda que não
consegue dizer `OK` jamais provaria nada dizendo `FAIL`.

**NADA ANUNCIOU O CORTE**: `offline=0` e `online=0` nas três corridas válidas e
no controle, e `navigator.onLine` **nunca** foi falso nelas.

**Controle POSITIVO do contador, na forma do M7.7** — o mesmo listener que
acabara de ler zero, na mesma instância de página, depois de a janela estar
cronometrada:

```
POSITIVE CONTROL: the same listener that just read offline=0 is now given a genuine event
POSITIVE CONTROL PASSED: offline 0 -> 1, online 0 -> 0. The counter counts, so the
zeros above are measurements and not silence
```

**E um controle que o ambiente rodou sozinho, sem ser encomendado.** A `long-cut-2`
da execução A foi **DESQUALIFICADA pelo próprio instrumento**: a rede da casa
caiu de verdade no meio da janela, e a precondição matou a perna com o número na
mão:

```
  [long-cut-2] T+267.14s online=false ... socket=OPENING
  [long-cut-2] T+295.21s online=true  ... socket=OPENING
  DOM NETWORK EVENTS over the window: offline=1 online=1
navigator.onLine went false at T+267.14s; the blackhole cut exists to leave it alone,
so its whole premise is gone and this timeline cannot be read as self-detection
--- FAIL: TestRealSPAOpeningPersistenceUnderLongCut/long-cut-2 (739.49s)
```

Isso vale por três coisas. É a prova de que **a guarda MORDE contra um evento
real que ninguém encenou** — controle negativo de campo, não de laboratório. É um
**segundo controle positivo do contador**, independente do encomendado:
`offline=1 online=1` num evento genuíno. E o dado da perna, embora inadmissível
para a pergunta do M8, é ele próprio informativo: **mesmo com um `offline` de
verdade no meio, o socket continuou em `OPENING`** e chegou a T+728,19s ainda
nele. A perna foi **reposta** na execução B em vez de aproveitada — a
desqualificação é da premissa, e premissa não se remenda.

### M8.7 — O que o M8 entrega sobre o corte, e a correção que ele traz ao M5

**Entrega o limite SUPERIOR, e ele é a ausência de um**: dentro de 12 minutos, o
socket sob corte não tem estado terminal. `C` está livre para ficar em qualquer
ponto acima do piso do M7 (`C > 1,36 s`) e abaixo dos ~11 min medidos, **sem que
o observável desapareça por baixo dele**.

**O que APERTA a escolha não é o teto — são as duas outras grandezas**, e as duas
saem desta corrida:

- **detecção**, que se SOMA: `detecção + C`. E o M8 **alarga a faixa que o M5
  publicou**. O M5 mediu 33,2–34,2 s em três corridas e a estabilidade de 1 s
  parecia ser a própria grade; o M8 mediu **31,1 / 34,2 / 42,3 s** em três
  corridas válidas (mais 41,3 s na perna desqualificada), com o mesmo perfil, a
  mesma máquina e a mesma grade de 1 s. **A faixa real é mais larga: 31,1–42,3 s,
  e o máximo é 8,1 s acima do máximo do M5.** Não é contradição — são seis
  amostras contra três, e a sexta caiu fora da faixa das três primeiras. O que
  cai é a leitura de que os ~34 s eram um número apertado;
- **falso positivo**, do outro lado: a recuperação depois de 12 minutos de queda
  leva **2,01–5,02 s**. Um `C` escolhido perto do piso mata sessões que estavam a
  três segundos de voltar.

**A moldura dos 24,4× continua fora, e o M8 não a traz de volta.** 33,2 s é
latência de detecção, soma-se; e o próprio número virou faixa.

### M8.8 — O que o M8 NÃO estabelece

**1. Não diz o que acontece depois de 12 minutos.** "Ainda em `OPENING` em
T+727,98s" é uma afirmação sobre a JANELA. Se há um estado terminal em 20 min, em
uma hora ou em um dia, o M8 não o viu e não o exclui. O texto do instrumento
recusa, por construção, imprimir isto como "para sempre".

**2. Três corridas em DUAS execuções, não uma.** A `long-cut-2` válida veio de
uma execução separada, porque a rede da casa desqualificou a original. Comparar
execuções separadas mede também o estado da máquina (a lição do M7.2), e a perna
reposta não teve o controle ao lado no mesmo processo. O que a torna comparável
mesmo assim: mesmo binário, mesmo perfil, mesma máquina, mesmo dia, e ela é a
corrida com a **menor** detecção e a **maior** permanência das três — ou seja,
não é ela que sustenta a conclusão.

**3. Uma condição amostrada três vezes — a armadilha do M6.4/M7.8-1, pela
terceira vez nesta capacidade.** As três corridas são o MESMO cenário: corte
total, imediato, com a rede de casa saudável. Não foi amostrado corte
intermitente, corte parcial, servidor que responde devagar, portal cativo, nem
corte aplicado durante o boot em vez de sobre sessão estável. A permanência é
"sem teto" **para este corte**, não para toda adversidade.

**4. Não identificou o mecanismo.** "Laço de retentativa sem estado terminal" é
LEITURA da linha do tempo, não medição. Ninguém olhou o tráfego, ninguém leu o
código do SPA, e nenhum temporizador foi identificado — exatamente como
"keepalive" era leitura no M5.4 e "handshake de k idas e voltas" no M7.4.

**5. Não mediu sessão revogada, deslogada ou expirada.** Continua sendo outra
medição, e continua sendo a que destruiria o ativo. Um socket em `OPENING` porque
a REDE morreu e um socket em `OPENING` porque a SESSÃO morreu são
indistinguíveis nesta evidência, e essa distinção é o que a CAP-04 vai precisar.

**6. Não mediu a recuperação depois de mais de 12 minutos**, nem a partir de um
estado que não fosse `OPENING`.

**7. A resolução da AUSÊNCIA de transição é 100 ms, não zero.** Um estado pelo
qual o socket passasse por menos de 100 ms escaparia às duas grades. O gravador
reduz a janela cega em 10×; não a fecha.

**8. Uma máquina, uma rede, um perfil, 5 boots.** E a rede de casa provou, nesta
mesma corrida, que não é estável — o que reforça o item 2 em vez de o suavizar.

**9. O corte NÃO sai daqui, e não era para sair.** Este nó é medição. Quando o
corte for implementado, tempo, prazos e sequências têm de ser dependências
injetáveis para que o teste seja determinístico, e o caminho de liveness não pode
ganhar *fallback* silencioso: **toda morte de sessão sai com causa classificada,
nunca erro genérico** (`HANDOFF-INICIATIVA.md` §6, numerada **11**).

### M8.9 — Higiene

`stopped_via=browser.close` em **5/5** boots e `SingletonLock` **ausente** depois
da parada limpa em **5/5** (4 na execução A, 1 na B). A contagem de arquivos do
perfil **não** é usada como sinal — a **H10** a mediu falsa para um boot só.
Nenhum envio, nenhuma leitura de conteúdo de mensagem, nenhum dado da conta em
log: a sonda reporta booleanos, contagens e o enum de estado do socket.
