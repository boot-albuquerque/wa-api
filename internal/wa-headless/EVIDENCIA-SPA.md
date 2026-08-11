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
