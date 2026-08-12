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
