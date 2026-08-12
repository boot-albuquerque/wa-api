# Evidência do SPA real — medições congeladas

Tudo aqui foi **observado** em `https://web.whatsapp.com/`, não deduzido do
`whatsapp-web.js` nem de memória. O `wwebjs` é mapa de onde procurar e fonte de
nomes internos; **não** é verdade automática sobre formato, invariante,
comportamento, ciclo de vida ou erro. O que a página faz é o que vale.

Reproduzir: `WA_HEADLESS_REAL_SPA=1 go test -run TestRealSPA ./internal/wa-headless/ -v`
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
