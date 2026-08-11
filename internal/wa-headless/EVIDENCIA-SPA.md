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

**`__x_wid` está AUSENTE.** É a identidade do dono, e ela só materializa com
sessão — o que a torna o melhor candidato disponível a "o motor pode agir".

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
READY; `__x_wid`, `__x_meReadyTriggered` e `socket_state` são falsos sem
sessão, então **podem** discriminar.

**Não estabelece**: que qualquer um deles fica verdadeiro *quando* a sessão
existe, nem *quando* em relação ao `#pane-side`. Isso é o LOOP B1.4, e exige
perfil pareado. Nenhuma conclusão sobre READY foi tirada daqui.
