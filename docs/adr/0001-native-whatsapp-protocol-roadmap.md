# ADR-0001: Reimplementação nativa e gradual do protocolo binário do WhatsApp

- **Status**: accepted (roadmap de longo prazo, execução incremental) —
  **amendado parcialmente por [ADR-0002](0002-vendorizar-whatsmeow-em-vez-de-reimplementar.md)**
  (2026-08-06): a rejeição de "fork completo do whatsmeow" e o critério
  "não vale reimplementar Signal/Noise do zero" foram revistos à luz de
  a licença MPL-2.0 permitir vendorizar o núcleo do `whatsmeow`
  (`binary`/`socket`/`store`/`types`/`proto`) e o módulo `libsignal`
  quase sem alteração, reescrevendo só a camada de orquestração de alto
  nível. A filosofia geral deste ADR (migração incremental, ports antes
  de troca de implementação, zero regressão observável) continua válida.
- **Data**: 2026-08-06

## Contexto

O `wa-api` é construído sobre o [`whatsmeow`](https://go.mau.fi/whatsmeow), uma
biblioteca Go que implementa o protocolo do WhatsApp Web/Multi-Device: o
transporte binário `waBinary` (nós `<iq>`/`<message>` codificados, não HTTP
nem protobuf de topo), a criptografia de sessão (Signal/Noise) e a
serialização protobuf específica de cada tipo de payload (`waE2E`,
`waHistorySync`, `waSyncAction` etc.).

Depender de uma lib de terceiros pra isso é a escolha certa por padrão — não
é razoável reimplementar Signal/Noise do zero por capricho. Mas na prática já
sentimos o custo dessa dependência de duas formas:

1. **O `whatsmeow` não aceita correções nossas com facilidade.** Quando
   encontramos uma lacuna ou comportamento que não serve o `wa-api`, o
   caminho normal (abrir PR upstream, esperar merge, publicar release, subir
   `go.mod`) é lento ou trava. Hoje `go.mod` já fixa um pseudo-commit
   (`v0.0.0-20260516102357-8d3700152a69`), não uma tag — sintoma de que já
   dependemos de estado do `whatsmeow` que não necessariamente vira release
   estável no ritmo que precisamos.
2. **A lib expõe só o que o autor decidiu expor, na forma que ele decidiu
   expor.** O caso concreto que motivou este ADR: `GetProfilePictureInfo`
   (`whatsmeow/user.go:517`) suporta busca condicional — você passa
   `ExistingID` (o `pic.ID` da última foto que já se tem) e, se a foto não
   mudou, o servidor responde rápido com "não mudou" (código 304) em vez de
   reenviar tudo. **Ninguém na nossa pipeline usa isso:**
   - `domain.GetAvatarRequest` (wa-api) só tem `Phone`/`Preview`, sem campo
     pra um ID anterior;
   - `user_adapters.go` sempre chama `GetProfilePictureInfo` com
     `ExistingID: ""`;
   - `app-core` (repo `disparazaap`, `upsert-avatar.ts`) só persiste
     `avatar_url`/`avatar_synced_at` — **descarta o `pic.ID`** que o
     `whatsmeow` devolve a cada fetch bem-sucedido.

   Resultado: **todo fetch de avatar é sempre um fetch completo**, mesmo pra
   contatos cuja foto não mudou desde o último ciclo. Isso consome o cap
   `avatar_fetch` (rate-limited, autoridade do `app-core`) à toa a cada
   rodada de batch — é a explicação mais provável de o batch de avatar
   render pouco por rodada (ver `wa.avatar.batch.done` com `fetched: 0`
   observado em dev, 2026-08-06).

   Esse caso específico **não exige reimplementar nada** — é threadar um
   campo que o `whatsmeow` já expõe (`ExistingID`) através de
   `GetAvatarRequest` → `user_adapters.go` → contrato HTTP → `wa-worker` →
   schema do `app-core` (`avatar_id` além de `avatar_url`). Fica registrado
   aqui como **evidência do padrão maior**: a lib expõe uma capacidade, mas
   como ninguém na nossa cadeia pediu por ela no design original, ela nunca
   chegou a ser usada — e корrigir isso depende só de mudar código nosso, não
   de esperar o `whatsmeow`.

O padrão que nos preocupa é o oposto: quando a lacuna está **dentro** do
`whatsmeow` (um bug, uma falha de mapeamento de erro, um caso do protocolo
que ele não cobre), estamos reféns do ritmo de aceitação de PR upstream.

## Decisão

Adotar como **objetivo de longo prazo, perseguido incrementalmente** a
reimplementação nativa do protocolo binário do WhatsApp — `waBinary` (nós
IQ), a camada de criptografia de sessão, e a serialização dos payloads que
mais nos interessam — dentro do próprio `wa-api`, reduzindo a dependência do
`whatsmeow` peça por peça, começando pelas superfícies onde:

1. já identificamos lacuna concreta e recorrente (como o caso do avatar
   acima), e
2. o custo de reimplementar é baixo comparado ao custo de esperar upstream.

**Não é um rewrite.** O `whatsmeow` continua sendo a base até que cada peça
nativa esteja madura e testada em produção. A migração é **superfície por
superfície**, nunca big-bang:

- Cada peça nativa vive atrás do mesmo *port* (`appport.ContactDirectory` e
  afins) que os adapters `whatsmeow/*` já implementam hoje — trocar a
  implementação por baixo não deve exigir tocar em use case ou handler.
- Critério de partida pra "vale a pena nativizar esta superfície": (a) já
  sentimos dor real nela (bug, lacuna, ou latência de correção upstream), e
  (b) o escopo é pequeno o suficiente pra testar isoladamente contra tráfego
  real antes de substituir o caminho `whatsmeow`.
- Nenhuma superfície native migra sem a mesma cobertura de testes (unit +
  contract) que o `whatsmeow` tem hoje via os testes existentes do adapter.

### Primeira candidata concreta (não iniciada)

**Perfil/avatar** (`w:profile:picture` IQ) é a superfície mais simples pra
começar: request/response pequenos, sem stream de mensagens em tempo real,
já mapeados em `user_adapters.go`. Antes de qualquer reimplementação nativa
aqui, o passo imediato e de baixo risco é o já descrito no Contexto: threadar
`ExistingID`/`avatar_id` pelas camadas existentes usando a API que o
`whatsmeow` já oferece. Isso resolve o sintoma (fetch sempre completo) sem
exigir nenhum trabalho de protocolo nativo — fica registrado como o próximo
passo antes de decidir se vale nativizar essa IQ específica.

## Racional

- **Correções em código nosso não esperam merge alheio.** O ritmo de
  release do `whatsmeow` upstream não é algo que controlamos; peças nativas
  sob nosso controle eliminam essa dependência crítica de tempo.
- **Superfície por superfície é reversível.** Cada port abstrai a troca —
  se uma peça nativa se provar pior que o `whatsmeow` em produção, a
  reversão é trocar o adapter de volta, não desfazer um rewrite.
- **O caso do avatar mostra o padrão sem forçar prematuramente a
  reimplementação binária** — a maioria das lacunas que vamos encontrar
  provavelmente é "a lib expõe, ninguém threadou" (fix rápido, nosso lado),
  não "a lib não expõe" (motivo real pra nativizar). Vale medir caso a caso
  antes de gastar esforço em protocolo binário.

## Evidência complementar: análise do `wuzapi`

Pra checar se as dores descritas no Contexto são peculiaridade do `wa-api` ou
padrão do ecossistema `whatsmeow`, foi analisado o
[`wuzapi`](https://github.com/asternic/wuzapi) — outro projeto Go open-source
que também expõe o `whatsmeow` via API HTTP, mais maduro/populoso que o
`wa-api` (~15.3k linhas, análise em 2026-08-06, repo local em
`~/Documents/projetos/github/wuzapi`). Seis apontamentos relevantes:

1. **Arquitetura sem abstração sobre a lib.** `clients.go:10-40` define um
   `ClientManager` (`sync.RWMutex` + `map[userID]*whatsmeow.Client`) e
   `handlers.go` chama o SDK `whatsmeow` **diretamente dentro dos handlers**
   HTTP (76 ocorrências de `clientManager.GetWhatsmeowClient(txtid).X(...)`),
   sem port/adapter separando use case de infraestrutura. Contraste com o
   `wa-api`, que já isola o `whatsmeow` atrás de ports
   (`appport.ContactDirectory` e afins) — arquitetura que este ADR pretende
   preservar ao nativizar superfícies.

2. **`ExistingID` também não é usado.** `handlers.go:3516-3580`, função
   `GetAvatar()`:
   ```go
   existingID := ""
   pic, err = clientManager.GetWhatsmeowClient(txtid).GetProfilePictureInfo(
       context.Background(), jid,
       &whatsmeow.GetProfilePictureParams{Preview: t.Preview, ExistingID: existingID},
   )
   ```
   `existingID` é zerado na hora e nunca preenchido a partir de estado
   anterior; não há cache/persistência de `pic.ID` em nenhum arquivo do
   projeto (`handlers.go`, `wmiau.go`, `clients.go`, `db.go`, `helpers.go`) —
   a única gravação é um log de telemetria (`handlers.go:3570`), descartado
   em seguida. **Confirma que o caso do avatar (ver Contexto) é padrão do
   ecossistema, não peculiaridade do `wa-api`** — dois projetos
   independentes cometeram a mesma omissão, o que é evidência mais forte de
   "a lib expõe, ninguém threadou" do que uma observação isolada teria sido.
   Ponto onde o `wa-api` já está à frente do `wuzapi`: distinção entre
   sem-foto/privacidade e erro real (commit `905d35b`) — o `wuzapi` trata
   ausência de avatar como 500 genérico.

3. **Erros sem tradução semântica.** Sem camada de abstração, erros do SDK
   viram HTTP 400/500 genéricos na maioria dos handlers — reforça o valor de
   manter os ports no `wa-api` mesmo nas peças que continuarem no
   `whatsmeow`.

4. **Mesma pseudo-versão pinada.** `go.mod` do `wuzapi` fixa
   `go.mau.fi/whatsmeow v0.0.0-20260516102357-8d3700152a69` — **o mesmo
   pseudo-commit exato** que o `wa-api` usa hoje. Evidência direta de que
   não há tag estável recente disponível no upstream; corrobora o argumento
   do Contexto de que essa é uma limitação estrutural do `whatsmeow`, não um
   sintoma de manutenção do `wa-api`.

5. **Workaround próprio existe, mas fora do escopo do protocolo binário.**
   `clients.go:14-20` mantém um cache in-memory (`pollOptions
   map[string]map[string][]string`) de texto plano das opções de enquete,
   pra resolver os hashes SHA-256 que o `whatsmeow` entrega nos eventos de
   voto — contorno de aplicação, não de protocolo. Não encontrado nenhum
   workaround que exigisse reimplementar `waBinary`/criptografia; reforça
   que nativizar protocolo deve continuar sendo exceção, não ponto de
   partida.

6. **Eventos**: dispatch centralizado em um switch grande
   (`myEventHandler`, `wmiau.go:696`) roteando por tipo de evento
   `whatsmeow` pra webhook/RabbitMQ/stdio — mesmo padrão observado no
   `wa-api`, sem achado que sugira lacuna na lib.

**Conclusão da análise**: nenhum achado no `wuzapi` motiva acelerar a
reimplementação nativa de protocolo — pelo contrário, reforça que a maior
parte da dor observável no ecossistema é "capacidade exposta e não usada"
(itens 2 e 4), resolvível com código de aplicação, e que a decisão deste ADR
de tratar reimplementação binária como último recurso, não ponto de partida,
está alinhada com a experiência de outro projeto maduro sobre a mesma lib.

## Limitação conhecida: `GetAllContacts` nunca devolve grupos

Achado durante o trabalho de listagem/ordenação de contatos no `disparazaap`
(2026-08-06), registrado aqui porque é o tipo exato de trade-off que este ADR
existe para rastrear: uma limitação que hoje **nos beneficia por acidente**,
mas pode virar **bloqueio real** no dia em que precisarmos de dado de grupo.

### O que descobrimos

`GetAllContacts` (`whatsmeow/store/sqlstore/store.go:797`) lê exclusivamente
da tabela `whatsmeow_contacts`:

```sql
SELECT their_jid, first_name, full_name, push_name, business_name, redacted_phone
  FROM whatsmeow_contacts WHERE our_jid=$1
```

Essa tabela é alimentada só por eventos `contact_action` do app-state sync —
a agenda 1:1 do usuário. **Grupos nunca entram nela**, por design do
`whatsmeow`: metadados de grupo (membros, admin, nome) vivem numa estrutura
inteiramente separada (`GetGroupInfo`/`GetJoinedGroups`), não em
`whatsmeow_contacts`. Não é um bug nem uma omissão nossa — é a lib decidindo
que "contato" e "grupo" são conceitos diferentes desde a base, e só expõe
`their_jid` do primeiro.

### Por que isso nos beneficia agora

O `wa-api` usa `GetAllContacts` como fonte de `GET /user/contacts`
(`user_adapters.go`), que o `wa-worker` consome pra sincronizar a agenda do
tenant no `disparazaap`. A tela de contatos do produto é sobre conversas
privadas com clientes — grupo nunca deveria aparecer ali. Como o `whatsmeow`
já filtra isso na origem, **não precisamos de filtro nosso** nesse caminho
específico (o `wa-api` não escreve nenhuma lógica de "ignore grupo" — a lib
simplesmente nunca oferece o dado).

### Por que isso pode virar problema

O dia em que o produto precisar de QUALQUER informação de grupo via este
provider — lista de grupos que o número participa, membros, nome/admin de um
grupo, mensagens de um grupo — `GetAllContacts` **não serve**, e não há como
fazer servir (não é um parâmetro que falta, é uma tabela que não tem o dado).
Seria necessário:

1. Portas novas sobre `GetGroupInfo`/`GetJoinedGroups` (ou equivalente) do
   `whatsmeow` — capacidade que a lib TEM, só não é usada hoje (mesmo padrão
   do caso do avatar: "a lib expõe, ninguém threadou" ainda não se aplica
   aqui porque ninguém pediu).
2. Ports/domain novos no `wa-api` (`Group`, não `Contact`) — grupo não é o
   mesmo agregado, misturar os dois no mesmo endpoint reintroduziria a
   ambiguidade que hoje não existe.
3. Nenhuma reimplementação de protocolo binário exigida por isso — é
   inteiramente superfície de API já coberta pelo SDK, mesma categoria do
   critério de partida deste ADR (evidência de dor real primeiro, nativizar
   só se a lib genuinamente não suportar).

### Ação registrada, não tomada

Este ADR não cria trabalho de grupo agora — não há demanda de produto pra
isso hoje. Fica documentado para que, quando essa demanda aparecer, quem
investigar não perca tempo redescobrindo que `GetAllContacts` é a fonte
errada — a resposta já está aqui: portas novas sobre a API de grupos do
`whatsmeow`, não sobre `ContactDirectory`.

## Alternativas descartadas

- **Fork completo do `whatsmeow` agora**: alto custo de manutenção
  imediato (toda a superfície de protocolo, criptografia incluída) sem
  evidência suficiente ainda de que a maioria das lacunas exige isso — o
  caso do avatar já mostra que parte da dor é uso incompleto da lib, não
  limitação dela. **Revisto pelo ADR-0002**: a premissa de que fork exige
  reimplementar criptografia do zero está incorreta — MPL-2.0 permite
  vendorizar `binary`/`socket`/`libsignal`/`store` quase sem alteração,
  reduzindo o fork a reescrever só a camada de orquestração de alto
  nível. Ver ADR-0002 para o racional atualizado.
- **Esperar passivamente por PRs upstream**: já demonstrado que trava o
  ritmo (pseudo-versão pinada em `go.mod` em vez de release estável).
- **Reimplementação big-bang do protocolo**: risco alto demais — criptografia
  de sessão e o transporte binário são superfícies onde um bug é
  silencioso e caro (sessões que não conectam, mensagens perdidas). Migração
  incremental por superfície reduz o raio de explosão de qualquer erro.

## Consequências

- Cada nova lacuna encontrada num adapter `whatsmeow/*` deve, antes de virar
  "esperar PR upstream", ser avaliada contra as duas perguntas do critério de
  partida (dor real + escopo pequeno o bastante pra isolar).
- Este ADR não cria trabalho imediato — a primeira ação concreta é o fix de
  `ExistingID`/`avatar_id` (fora do escopo deste documento, ver Contexto),
  não uma reimplementação de protocolo.
- Revisar este ADR quando a primeira superfície nativa (se houver) estiver
  em produção, pra validar se o critério de partida se provou útil na
  prática.
- A superfície de sessão (pareamento, transporte, eventos) já tem port
  pronto pra receber uma implementação nativa, resultado do plano
  `.omc/plans/native-multisession-architecture.md`: `SessionProvider`/
  `Session` (`pkg/application/contracts/session_provider.go`) cobrem
  `Pair`/`Connect`/`Disconnect`/`Logout`/`SetProxy`/`Subscribe`, com
  `SessionEvent` (`pkg/application/contracts/session_event.go`) e
  `PairingEvent` (`pkg/application/contracts/pairing_event.go`) tipando os 6
  eventos de sessão/transporte e o fluxo de QR sem vazar nenhum tipo de
  `go.mau.fi/whatsmeow` na assinatura pública do port. A orquestração (grava
  em `users`, webhook, S3, retry) permanece em `SessionOrchestrator`,
  agnóstica de provider, e não precisa ser tocada quando a primeira peça
  nativa chegar — só a implementação de `SessionProvider` muda. Eventos de
  domínio (mensagem, presença, grupo, histórico) ficam fora deste port,
  continuam em `myEventHandler`/`eventhandler_*.go`.
- Os outros três ports criados junto (`SessionRegistry`,
  `SessionEventDispatcher`, `SessionAttachHook`) **não** fazem parte da
  superfície candidata a nativização: existem só para preservar a direção de
  dependência (`pkg/application` e `pkg/infra` nunca importam
  `pkg/bootstrap`), e continuam implementados por `ClientManager`/
  `pkg/bootstrap` independentemente de a sessão vir do `whatsmeow` ou de
  código nativo. Quem for avaliar a nativização da sessão deve olhar apenas
  para `SessionProvider`/`Session`.
