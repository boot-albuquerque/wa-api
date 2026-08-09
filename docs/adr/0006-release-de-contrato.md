# ADR-0006: release de contrato — uma janela, quatro mudanças

- **Status**: proposed — duas decisões em aberto, marcadas abaixo
- **Data**: 2026-08-09
- **Relacionado**: F66, F68, F75, F97 e F100 em `HOUSEKEEP.md`

## Contexto

Quatro achados pendentes mudam o que um cliente observa: F75 (token na query
string), F97 etapa 2 (remover o `OR token = $1`), F66 (67 validações que
respondem 500 em vez de 400) e F68 (o evento `QR` despachado com dois formatos
de payload).

Tratados como quatro itens de backlog, eles custam **quatro** janelas de
migração, quatro avisos a todo cliente e quatro chances de alguém não migrar.
Tratados como uma release, custam uma.

Essa é a decisão deste ADR: eles não são quatro decisões, são uma.

### O que mudou desde que foram registrados

Duas coisas, e as duas alteram o plano:

**A F97 está bloqueada pela F100.** A "etapa 1" descrita na F97 — parar de
gravar o token em texto claro — abre acesso **sem credencial**, medido: com a
coluna em branco, uma requisição sem token nenhum autentica como aquele usuário.
A guarda que recusa token vazio já entrou (`fef8316`), mas os outros dois
pré-requisitos não. Enquanto não entrarem, a F97 não começa.

**A F75 não é o que o nome sugere.** Não dá para "remover o token da query
string" e pronto: a API `WebSocket` do navegador **não permite header customizado
no handshake** — é limitação da especificação, não do nosso código. Um cliente
web só autentica em `/session/ws` por query string, cookie ou subprotocolo. A
remoção uniforme que foi anunciada é impossível de cumprir sem quebrar todo
painel de navegador.

## Decisão

### Fase 0 — invisível ao cliente, entra a qualquer momento

Nada aqui muda resposta, formato ou contrato. Pode entrar em commits separados,
sem janela e sem aviso.

| item | o que | por que antes |
|---|---|---|
| F100 (2) | rechavear o `UserInfoCache` por `userID` em vez de token | usar credencial como chave de cache é o que amarra o cache ao texto claro; sem isso, branquear a coluna torna o webhook inerte (F70 de volta) |
| F100 (3) | tirar o token do caminho de religação (`connectOnStartup` → `Attach`) | ele lê `users.token` do banco; com a coluna em branco, religa com token vazio |
| F97 (1) | o `INSERT` para de gravar o texto claro | só é seguro DEPOIS dos dois acima |

A ordem entre eles não é preferência: inverter deixa o sistema quebrado no meio.

### Fase 1 — a release, com janela e aviso

Tudo abaixo sai junto, numa versão só.

#### F97 etapa 2 — remover `OR token = $1` e dropar a coluna

Invalida qualquer linha antiga que só tenha texto claro. É por isso que precisa
de janela: entre a Fase 0 e esta, todo usuário ativo precisa ter `token_hash`
preenchido. A Fase 0 garante isso para usuários novos; para os antigos, uma
migração de preenchimento (calcular o hash a partir da coluna em claro, enquanto
ela ainda existe) resolve sem exigir nada do cliente.

**Sem essa migração de preenchimento, a etapa 2 desloga quem nunca reautenticou.**

#### F66 — 500 vira 400 nas 67 validações

`Category.HTTPStatus()` já existe e já tem teste; falta ligá-lo. A F83 e a F93 já
demonstraram o padrão em dois endpoints.

Risco real: baixo. Quebra só cliente que trate 5xx como retentável e 4xx como
final — e para esses a mudança é uma MELHORA (parar de repetir um payload que
nunca vai ser aceito). Vai na release por ser mudança de contrato observável,
não por ser perigosa.

#### F68 — unificar o payload do evento `QR`

> **DECISÃO EM ABERTO (1 de 2).**
> **(a)** um `type` só, sempre com `code`, e com `qrCodeBase64`/`expiresAt`
> quando houver — menos disruptivo para quem já consome.
> **(b)** dois `type` distintos (`QR` e `QRCode`) — mais honesto, e o roteamento
> do cliente passa a ser explícito em vez de depender de campo presente.
>
> **Recomendo (a)**: quem consome hoje já lida com o formato rico; a mudança
> some para a maioria e some de vez com o formato pobre.

#### F75 — token na query string

> **DECISÃO EM ABERTO (2 de 2).** As três saídas, e nenhuma é indolor:
>
> **(a) Manter a query só em `/session/ws`, remover do resto.** Simples e
> honesto. Custo: o token continua aparecendo em log de acesso e histórico de
> navegador NAQUELA rota — e o log de fronteira registra a URL, então isso é
> vazamento real, não teórico. Só vale acompanhado de redação do parâmetro no
> log.
>
> **(b) Token no subprotocolo** (`new WebSocket(url, [token])`, servidor lendo
> `Sec-WebSocket-Protocol`). Tira o token da URL de verdade. Custo: todo cliente
> WebSocket muda, inclusive os seus.
>
> **(c) Cookie de sessão.** Resolve o handshake, mas introduz superfície de CSRF
> que hoje não existe. Não recomendo.
>
> **Recomendo (a) com redação de log AGORA, e (b) como destino declarado.** (a)
> cabe nesta release sem exigir mudança de cliente; (b) exige, e misturar as duas
> coisas numa janela só faz a migração falhar por excesso de frentes.

## Consequências

**O anúncio muda de texto.** O que está prometido hoje é "o token por query
string será rejeitado na próxima release". Isso não vai acontecer para
`/session/ws`, e manter a promessa como está fará clientes web quebrarem
achando que estavam em dia. O aviso de depreciação precisa passar a dizer
*exceto `/session/ws`* antes da release, não junto com ela.

**A Fase 0 precisa estar em produção antes**, não no mesmo deploy. É ela que
garante que todo usuário ativo tenha `token_hash`; medir isso é uma consulta
(`SELECT COUNT(*) FROM users WHERE token_hash IS NULL`) e ela precisa devolver
zero antes de a Fase 1 sair.

**Nada aqui é reversível por rollback simples.** Dropar a coluna `token` é
irreversível sem restaurar backup. A ordem — preencher, verificar zero, só
então dropar — é o que substitui a possibilidade de voltar atrás.

## Follow-ups

- Redação do token nos logs de fronteira, se a F75 for por (a).
- Migração de preenchimento de `token_hash` para linhas antigas.
- Consulta de verificação (`token_hash IS NULL` = 0) como critério de entrada da
  Fase 1.
