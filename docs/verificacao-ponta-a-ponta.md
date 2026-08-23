# Verificação de ponta a ponta — pareamento e envio real

**Estado**: procedimento **ensaiado até ao QR** em 2026-08-20, contra o binário
real e um banco limpo. Os passos 1 a 5 foram executados e corrigidos com o que
a execução mostrou. Os passos 6 e 7 exigem o humano: ler o QR e confirmar a
chegada da mensagem.

> **Duas correções que só apareceram por ensaiar.** A primeira versão deste
> documento estava errada em dois pontos que teriam custado a sessão:
> mandava usar o cabeçalho `Authorization` nas rotas de sessão (é **`token`**;
> `Authorization` só vale para `/admin/*`), e dizia que o QR vinha como texto
> puro (vem como **`data:image/png;base64,...`**, imagem pronta a renderizar).
> Entregar sem ensaiar teria reproduzido o defeito que esta sessão passou o dia
> a recusar dos executores: afirmar sem medir.

**Escopo desta primeira prova (decisão do canal, 2026-08-20)**: o MÍNIMO VIÁVEL
— subir, criar usuário, parear, enviar **um** texto, confirmar chegada. Não é
falta de ambição: se algo falhar, falha num ponto identificável. Provar as treze
rotas de uma vez transforma isto numa maratona onde não se sabe qual passo
quebrou.

## As três decisões que moldam o procedimento

| decisão | escolha | por quê |
|---|---|---|
| **destino** | um **segundo número** do humano, NÃO auto-envio | o WhatsApp trata "mensagem para si mesmo" de forma especial; auto-envio enfraqueceria a prova do caminho normal entre dois JIDs |
| **ambiente** | banco **novo e limpo** | isola pareamento + envio. O banco existente e a migração da F163 são teste SEPARADO, depois — misturar os dois confunde causas |
| **o que conta como prova** | a mensagem **aparecer no aparelho destino**, com o conteúdo certo | `200` + `message_id` é evidência INTERMEDIÁRIA. Esta sessão documentou três casos em que a API responde 200 sem ter feito nada (F108, F135, F176) |

## Pré-requisitos

O arranque **falha fechado** sem a chave de encriptação — decisão da F169. Isto
é deliberado: uma chave gerada a cada arranque tornaria indecifrável todo
segredo já guardado.

```bash
export WA_API_GLOBAL_ENCRYPTION_KEY="<32 bytes que você guarde>"   # 16, 24 ou 32
export WA_API_ADMIN_TOKEN="<um token de admin à sua escolha>"
```

Se `WA_API_ADMIN_TOKEN` não for definido, o processo **gera um** e escreve-o num
arquivo `admin_token` com permissão `0600` no diretório de dados — ele NÃO
aparece no log (F169). Defini-lo à mão evita ter de o ir buscar.

## Passo 1 — construir e subir, com banco limpo

```bash
cd /Users/albuquerque/Documents/projetos/profile-github/worktrees/wa-api-wa-noise
make build

DATA_DIR=$(mktemp -d /tmp/wa-e2e-XXXX)     # banco NOVO, isolado
./wa-api -datadir "$DATA_DIR" -port 8080 -loglevel debug
```

Deixe a correr num terminal próprio. O `-loglevel debug` importa: é por ele que
se vê o que o SDK está a fazer se algo falhar.

## Passo 2 — criar o usuário

```bash
curl -s -X POST localhost:8080/admin/users \
  -H "Authorization: $WA_API_ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"e2e","token":"e2e-token-local"}'
```

Devolve `200` com o `id` do usuário criado. **Ensaiado — funciona.**

**Os dois cabeçalhos são DIFERENTES, e confundi-los dá 401:**

| rotas | cabeçalho |
|---|---|
| `/admin/*` | `Authorization: <admin token>` |
| todas as outras | `token: <token do usuário>` |

O `token` do usuário é o que autentica as chamadas seguintes — não é segredo de
produção, é local e descartável com o banco.

## Passo 3 — disparar a conexão e obter o QR

```bash
curl -s "localhost:8080/session/connect" -H "token: e2e-token-local"
sleep 4
curl -s "localhost:8080/session/qr"      -H "token: e2e-token-local"
```

**Ensaiado — funciona.** O `sleep` importa: pedir o QR imediatamente devolve
vazio, porque a conexão é disparada em goroutine.

`/session/qr` devolve `{"QRCode":"data:image/png;base64,iVBORw0KGgo..."}` — uma
**imagem PNG embutida**, não texto. Para a ver:

```bash
curl -s "localhost:8080/session/qr" -H "token: e2e-token-local" \
  | python3 -c "import sys,json,base64;d=json.load(sys.stdin)['data']['QRCode'];open('/tmp/qr.png','wb').write(base64.b64decode(d.split(',',1)[1]))"
open /tmp/qr.png
```

`devui/sessions.html` **não é servido** nesta configuração (404 medido) — use o
comando acima.

> **Atenção ao `/session/connect`**: ele responde `200 {"status":"connecting"}`
> **antes** de saber o resultado, e continua a responder isso mesmo quando nada
> conecta — é a F176/F108, ainda aberta. **Não tome o 200 como confirmação.**
> Use `/session/status`.

## Passo 4 — ler o QR *(HUMANO)*

Ler no WhatsApp do telemóvel: **Aparelhos conectados → Conectar aparelho**.

O QR **expira**; se demorar, repetir o passo 3.

## Passo 5 — confirmar o pareamento antes de enviar

```bash
curl -s "localhost:8080/session/status" -H "token: e2e-token-local"
```

**Ensaiado.** A resposta traz `connected` e `loggedIn` SEPARADOS, e a distinção
é o que importa aqui:

- `connected: true, loggedIn: false` → o socket subiu, **o pareamento ainda NÃO
  aconteceu**. Foi este o estado no ensaio, antes de qualquer leitura de QR.
- `loggedIn: true` → pareado. **É este o sinal para avançar.**

**Não avance com `loggedIn: false`.** Enviar aí produz um erro que parece defeito
de envio e é só sessão não pareada — desperdiça a rodada e confunde o
diagnóstico.

## Passo 6 — enviar UMA mensagem de texto

```bash
curl -s -X POST localhost:8080/chat/send/text \
  -H "token: e2e-token-local" \
  -H 'Content-Type: application/json' \
  -d '{"Phone":"<SEGUNDO NÚMERO, com país e DDD>","Body":"prova de ponta wa-api"}'
```

Esperado: `200` com `message_id` e `timestamp`.

## Passo 7 — a prova de verdade *(HUMANO)*

**Confirmar que a mensagem chegou ao aparelho destino, com o texto certo.**

É este o passo que decide. Os anteriores são preparação — o `200` do passo 6 é
evidência intermediária, não sucesso.

## Se falhar, onde olhar

| sintoma | provável causa |
|---|---|
| **401 em qualquer rota de sessão/chat** | cabeçalho errado: é `token:`, não `Authorization:` — foi o erro da primeira versão deste documento |
| processo não sobe | falta `WA_API_GLOBAL_ENCRYPTION_KEY` — **ensaiado**: a mensagem nomeia a variável e explica por que a chave nunca é gerada |
| `/session/qr` devolve vazio | pedido cedo demais; a conexão é disparada em goroutine — esperar ~4 s e repetir |
| `/session/connect` dá 200 mas `/session/status` nunca conecta | F176/F108 — a rota mente; a causa real está no log |
| envio dá erro de sessão | pareamento não completou; repetir do passo 3 |
| `200` no envio e a mensagem **não chega** | **é o achado mais valioso possível** — a cadeia está verde e o WhatsApp não entregou. Guardar o log completo do passo 6 |

## Depois — só se esta prova passar limpa

O canal autorizou avançar para `/chat/list` e `/chat/history`, que validam o
caminho de **leitura**. `/chat/list` interessa em especial: a F173 mostrou que
ela existe na API e não está exposta no stdio.

**Não ampliar antes de o mínimo passar** — ampliar sobre incerteza acumula
causas em vez de as isolar.
