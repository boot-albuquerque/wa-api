# Extrator de query IDs do `mex`

## Quando usar

Uma operação de `mex` (newsletter, e outras superfícies GraphQL) começa a
responder:

```
graphql error: 400 Bad Request (CRITICAL)
```

enquanto as operações vizinhas continuam a funcionar. Isso é o sintoma de um
`query_id` retirado pelo WhatsApp.

## Como

`extract.js` corre na consola do `web.whatsapp.com` autenticado. Só lê: procura
nos bundles JS da própria página os nomes `WAWebMex*Mutation.graphql` e o ID
numérico adjacente. Não envia nada.

Os IDs não se conseguem ler do tráfego — o `mex` viaja dentro do WebSocket
cifrado por Noise. O bundle é servido em claro, e é de lá que o Baileys e o
whatsmeow também os tiram.

Os valores vão para
`internal/wa-noise/capabilities/newsletter/queryids.go`.

## Três coisas que esta sessão ensinou (2026-08-25, F233)

**Não copie IDs de outro cliente sem os conferir aqui.** Os do Baileys para
`demote` e `change owner` estavam velhos e davam o `400`; o do `delete` batia
certo. Fontes de aparência idêntica não são intermutáveis.

**Um `400` nem sempre é o ID.** Depois de trocar pelos IDs reais do bundle, o
`400` persistiu — a causa era a forma do JID (`user_id` tem de ser LID, não
PN). Troque o ID, meça, e se o erro não mudar procure noutro sítio.

**Não "conserte" um ID que funciona.** O nosso `follow` (`9926858900719341`)
funciona em campo, e o bundle traz `24404358912487870` para a mesma operação. O
WhatsApp parece manter gerações antigas vivas.

## Verificado a correr

2026-08-25, contra a sessão autenticada: **63 operações** extraídas, e os IDs
de newsletter batem com os medidos em campo (`30062808666639665` delete,
`9880997548630971` demote, `9546742745432473` change owner).

Confirmou-se também a ressalva da janela: `ChangeNewsletterOwner` devolveu
DOIS IDs (`27856697387325731 | 9546742745432473`), porque a janela apanha a
operação vizinha. Quando isso acontecer, use a secção abaixo para desempatar.

## Como confirmar um candidato

Dispare a operação contra o servidor real. O ID errado responde `400 Bad
Request`; o certo responde com o erro *da própria operação* — `405 Not
Allowed`, `401 Not Authorized`, ou sucesso. É essa mudança de erro que
distingue, não o código HTTP da nossa API.
