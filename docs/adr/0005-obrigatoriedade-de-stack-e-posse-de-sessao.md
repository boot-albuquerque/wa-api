# ADR-0005: obrigatoriedade de stack, posse de sessão e degradação por capacidade

- **Status**: proposed
- **Data**: 2026-08-08
- **Relacionado**: F86, F87, F88, F89 em `HOUSEKEEP.md`. **Amenda** a decisão
  registrada na F88 de que retry durável exigiria RabbitMQ — ver D3.

## Contexto

O alvo passa a ser execução em N pods (k8s/k3s), com um requisito adicional
explícito: em cenário catastrófico o sistema roda **só com SQLite e um pod**, e
ainda assim tem de permanecer funcional.

Isso não é uma escolha entre duas arquiteturas. São duas pontas de uma mesma
escala, e o desenho tem de valer nas duas sem trocar de semântica no caminho.

### O que já foi MEDIDO (F89, 2026-08-08)

Ambiente isolado, Postgres em `infra/compose.yaml`, sessão descartável pareada
por QR:

| medida | resultado |
|---|---|
| processo novo retoma a sessão sem QR | **sim, 6/6** |
| tempo do arranque até a sessão operacional | **1,16s a 2,29s** |
| duas réplicas na mesma sessão | **1** `StreamReplaced`, sem laço |
| logout por conflito | **0** — credenciais preservadas |
| o perdedor se recupera? | **não**, nem 45s depois de o rival morrer |
| `users.connected` durante o zumbi | continua **1** |

Três consequências que o desenho tem de absorver:

1. **Retomar sessão é barato** (1–2s). Coordenação não precisa ser rápida: um
   lease em Postgres é folgado. Se custasse 30s, a conclusão seria outra.
2. **O modo de falha do conflito é silencioso**, não barulhento. O perdedor
   fica com o processo vivo, HTTP respondendo e liveness verde — e a sessão
   morta, sem nunca tentar reconectar. O banco continua afirmando
   `connected=1`.
3. **Degradação silenciosa já mordeu.** A instância de produção roda em SQLite
   por causa do fallback automático de `pkg/infra/db/connection.go:81`, que
   dispara quando as variáveis de Postgres estão PARCIALMENTE definidas. Ela
   registra um `warn` e segue. Em multi-pod isso seria cada réplica com um
   banco próprio, todas se achando donas de tudo.

## Decisão

### Princípio: capacidade escala com a infraestrutura, função não

O núcleo — manter sessões do WhatsApp e entregar eventos — **nunca exige mais
que SQLite e um pod**. Tudo acima disso compra ESCALA, não função. Nenhum
componente opcional pode ser pré-requisito de algo que o produto promete.

O corolário é o que responde à pergunta "como continua funcional no cenário
catastrófico": ele continua funcional porque o modo de um pod é um **modo de
primeira classe, com garantia imposta**, e não um acidente de configuração.

### D1 — Modo de cluster é explícito e não é inferido

`WA_API_CLUSTER_MODE` = `single` | `multi`. Sem padrão silencioso.

- Em **`multi`**, Postgres é obrigatório. O fallback para SQLite vira **erro
  fatal de arranque**, não `warn`. Uma variável esquecida no manifesto tem de
  derrubar o pod, não produzir uma réplica com banco próprio.
- Em **`single`**, o processo adquire um **lock exclusivo local** (arquivo no
  diretório de dados). Um segundo processo apontando para o mesmo SQLite
  **recusa subir**, com mensagem dizendo por quê.

O objetivo do lock local não é performance: é impedir o zumbi silencioso
medido no Exp 1. Hoje nada impede um segundo processo, e o resultado é uma
sessão morta que ninguém percebe.

### D2 — Posse de sessão por lease com TTL, só no modo `multi`

Tabela `session_leases(user_id PK, owner_id, expires_at)`.

- **Reivindicar**: `INSERT ... ON CONFLICT (user_id) DO UPDATE ... WHERE
  session_leases.expires_at < now()`. Quem conseguir a linha é o dono.
- **Renovar**: heartbeat estende `expires_at` enquanto a sessão estiver viva.
- **Liberar**: no desligamento gracioso, apaga a linha — assim o failover não
  espera o TTL no caso comum.

**Lease com TTL, e não advisory lock**, apesar de o advisory lock ser mais
elegante (morre junto com a conexão). O motivo é previsibilidade: a liberação
do advisory lock depende de o Postgres PERCEBER que a conexão morreu, e num
kill abrupto isso fica à mercê do keepalive do TCP, que em alguns sistemas é
medido em horas. TTL é um número que nós controlamos.

**Dimensionamento a partir da medição**: retomada leva 1,2–2,3s, então o TTL é
que domina o failover. TTL de 15s com heartbeat de 5s dá margem de 3× contra
pausa de GC e soluço de rede, com failover de pior caso ≈ 15s + 2s ≈ **17s**.
Ambos configuráveis. TTL menor acelera o failover e aumenta o risco de despejo
falso — o ponto exato ainda **não foi medido** e não deve ser chutado.

### D3 — Durabilidade de entrega é OUTBOX, não broker

**Isto amenda a F88**, que registrou que retry durável exigiria RabbitMQ e um
consumidor. Está errado: durabilidade exige um **armazenamento transacional**,
e SQLite é um.

Tabela `webhook_outbox(id, user_id, url, payload, tentativa, due_at, status)`.
O despacho grava a intenção antes de tentar; o retry atualiza `due_at`; o
sucesso apaga a linha. No arranque, o processo carrega as linhas vencidas e
retoma.

Isso dá **sobrevivência a restart em qualquer configuração**, inclusive na
catastrófica — que é exatamente o que o requisito pede. Em `multi`, as linhas
são reivindicadas com `FOR UPDATE SKIP LOCKED`, o mesmo mecanismo, sem código
diferente por tier.

Consequência a aceitar conscientemente: o payload (conteúdo de mensagens
reais) passa a repousar em disco até ser entregue. Hoje o retry vive só em
memória. Vale para SQLite e para Postgres igualmente, e exige política de
retenção — a linha some quando entrega ou quando esgota, e o esgotamento
continua indo para o caminho terminal.

**Estado em 2026-08-09: metade implementada, e a metade que falta está
desenhada aqui para não se perder.**

PRONTO e testado (`pkg/infra/db/webhook_outbox.go`, migração 15):

- tabela nos dois dialetos — em SQLite ela NÃO é inerte, ao contrário da de
  posse: o cenário catastrófico é justamente onde durabilidade de entrega
  precisa funcionar;
- `Enqueue` / `Reschedule` / `Delete` / `ClaimDue` / `PendingCount`;
- reivindicação por EMPURRÃO de `due_at`, na mesma transação da leitura. Não há
  estado "em processamento" separado, de propósito: um estado assim fica PRESO
  quando o processo morre entre marcar e entregar, e exigiria um varredor para
  destravá-lo. Empurrar o prazo faz ele vencer sozinho.
- a chave HMAC **não** é persistida: já vive em `users.hmac_key` e é relida por
  `user_id`. Duplicar segredo em outra tabela multiplica a superfície de
  vazamento sem comprar nada.

**FIAÇÃO PRONTA e validada em bancada (2026-08-09)**, com `kill -9` — queda, não
desligamento gracioso:

```
processo vivo, varredura de pe
kill -9                                  <- queda abrupta
insere 2 entregas pendentes (com o processo FORA)
   pendentes com o processo morto: 2
sobe de novo
   t=+2s  pendentes=0
   log: "retomando entregas pendentes do outbox" entregas=2
```

Antes desta mudança, as duas teriam sumido com o processo, sem log e sem
contagem. Migração 15 aplicada e conferida nos DOIS dialetos (Postgres em 15 com
`to_regclass` não-nulo; SQLite em 15 com a tabela presente).

O que a bancada NÃO cobriu, e fica explícito: a entrega HTTP em si. O cliente
HTTP é provisionado junto com a sessão, então uma entrega retomada para usuário
sem sessão para em "HTTP client is nil" e a linha é liquidada — que é o
comportamento correto, mas significa que o laço completo até um destino real
exige um dispositivo pareado. Os testes unitários cobrem o resto.

O desenho, para quem for ler o código:

1. `callHookWithHmac` grava a intenção ANTES da primeira tentativa e tenta na
   hora — a primeira entrega não ganha latência de varredura.
2. Sucesso e caminho terminal apagam a linha.
3. Falha com orçamento restante faz `Reschedule`, e **quem executa a
   retentativa passa a ser a varredura**, não o `time.AfterFunc` da F88.

O item 3 é a decisão de projeto que importa: **o outbox vira o ÚNICO
agendador.** Manter os dois (timer em memória + varredura) criaria corrida
entre eles — os dois disparariam perto de `due_at` e o cliente receberia em
duplicata. Uma fonte de verdade, e o custo é latência de até um intervalo de
varredura numa retentativa que já espera 30s ou mais.

Efeito colateral bem-vindo: o orçamento de bytes pendentes da F88
(`retryBytesPendentes`) deixa de ser necessário para o retry. Ele existia porque
os payloads pendentes viviam em MEMÓRIA; com o outbox eles vivem em disco, e o
teto passa a ser o disco, que é observável e não derruba o processo.

### D4 — RabbitMQ é distribuição, não durabilidade

Com o outbox, o broker deixa de ser necessário para não perder entrega. O que
ele acrescenta:

- tirar a entrega do pod dono, distribuindo entre réplicas;
- absorver rajada fora do processo;
- integrar com consumidores externos que já existam.

Quando ausente, o pod dono entrega a partir do próprio outbox. **Nenhuma
funcionalidade some** — muda quem faz o trabalho.

### D5 — Redis: quando NÃO usar, que é o caso comum

Resolvida a posse (D2), o WebSocket entre réplicas se resolve **roteando pelo
dono**: a sessão já está fixada num pod, então `/session/ws` daquele usuário vai
para aquele pod. Isso não precisa de pub/sub.

Portanto: **não usar Redis só porque ele está disponível.** Cada dependência
usada é um modo de falha novo, e aqui não há ganho correspondente.

Redis entra quando houver **medição** mostrando uma destas duas coisas:

1. o heartbeat do lease sobrecarregando o Postgres (muitos pods × muitas
   sessões); ou
2. roteamento por dono inviável na camada de ingress do ambiente real, o que
   tornaria o fan-out por pub/sub a alternativa.

Até que exista esse número, Redis fica **fora**. Ver "Medir antes de projetar"
em `CLAUDE.md`.

### D6 — Saúde separa "processo de pé" de "sessão de pé"

O Exp 1 mostrou processo saudável com sessão morta e o banco dizendo
`connected=1`. Logo:

- `users.connected` é **intenção** (o usuário quer estar conectado), e continua
  sendo isso;
- o **estado observado** da sessão é coisa distinta e precisa existir;
- `/health/live` responde pelo processo; `/health/ready` responde por "este pod
  está servindo as sessões que ele diz possuir".

Sem essa separação, qualquer readiness probe mente, e o k8s manda tráfego para
um pod que não serve aquela sessão.

**Implementado em 2026-08-09** (`pkg/bootstrap/health.go`), com uma divergência
deliberada do parágrafo acima, registrada aqui porque quem ler o código depois
vai notar a diferença:

- `/health/live` responde pelo processo. `/livez` continua valendo como alias —
  é para onde o `HEALTHCHECK` do Dockerfile aponta hoje, e renomear uma sonda
  por baixo de um deployment em execução deixa todo contêiner insalubre no
  mesmo instante.
- `/health/ready` reprova quando o pod **não consegue servir nada**: banco sem
  resposta, ou heartbeat de posse parado (este só existe em `multi`; em
  `single` a checagem some do relatório em vez de aparecer como "ok", porque
  checagem que sempre passa ensina a ser ignorada).

O que **não** entrou: reprovar readiness porque UMA sessão morreu. Lido ao pé
da letra, "servindo as sessões que ele diz possuir" derrubaria o pod inteiro
por causa de uma sessão — cortando as outras noventa e nove que ele serve bem.
A pergunta original é por SESSÃO e a sonda é por POD: ela não tem como dizer
"mande o usuário A para outro lugar e continue me mandando o B". Isso é
roteamento por dono (D5), não readiness.

Enquanto o D5 não existe, a divisão honesta é: readiness reprova o que impede o
pod de servir; divergência por sessão é **reportada**, não fatal. A F98 já
removeu a fonte principal dessa divergência — lease de sessão que não existe
mais volta a ser devolvido em vez de renovado para sempre.

Validado em bancada, com o Postgres derrubado por `docker stop`:

```
live   {"status":"ok"}                                                    HTTP 200
ready  {"status":"not_ready","checks":{"database":"unreachable",
                                       "session_ownership":"ok"}}         HTTP 503
```

As duas sondas discordando ao mesmo tempo é a prova do D6 — uma sonda só, ou
duas que sempre concordam, é o defeito que o Exp 1 mediu. Recuperou em ~3s
depois de o banco voltar.

A bancada também pegou o que os dez testes unitários não pegaram: `s.routes()`
monta o roteador (e a sonda) QUATRO LINHAS antes de `setupSessionOwnership`
instalar o `leaseManager`. Recebendo o manager por valor, a sonda capturava
`nil` para sempre e a checagem de posse simplesmente não aparecia em `multi` —
com um relatório idêntico ao de um `single` saudável, que é o pior formato
possível para um defeito assumir. Corrigido lendo por getter, e travado por um
teste que exercita a janela (ARMADILHAS 24).

O corpo da sonda leva código de motivo, nunca o erro do driver: ela é
não-autenticada (o kubelet não carrega token) e erro de conexão carrega host,
porta e usuário. O detalhe vai para o log, que é autenticado. Resposta é **503**
e não 500 — o pod está temporariamente incapaz de servir, e 5xx-como-bug seria
lido como "reinicie este processo" quando a causa costuma ser dependência que
volta.

### D7 — Degradação é sempre alta, nunca silenciosa

No arranque, o processo registra um relatório de capacidades: o que está
ligado, o que não está, e **por quê**. Combinação impossível não degrada:
recusa subir (D1).

## Tabela de obrigatoriedade de stack

| capacidade | SQLite 1 pod | + Postgres | + RabbitMQ | + Redis |
|---|---|---|---|---|
| manter sessões e receber eventos | **sim** | sim | sim | sim |
| entregar webhook (1ª tentativa) | **sim** | sim | sim | sim |
| retry com backoff e jitter | **sim** | sim | sim | sim |
| **retry sobrevive a restart** | **sim** (outbox) | sim | sim | sim |
| WebSocket ao vivo | **sim** (local) | sim | sim | sim |
| N pods | não (recusa) | **sim** (lease) | sim | sim |
| WS com cliente em pod diferente | n/a | **sim** (rota por dono) | sim | sim |
| entrega distribuída entre pods | não | não | **sim** | sim |
| consumidor externo da fila | não | não | **sim** | sim |

| componente | obrigatório quando | NÃO usar quando | por quê |
|---|---|---|---|
| SQLite | sempre que não houver Postgres | modo `multi` | não coordena N processos; em `multi` é erro fatal, não fallback |
| Postgres | modo `multi` | 1 pod sem necessidade de escala | é o que torna a posse possível; abaixo disso é peso sem ganho |
| RabbitMQ | nunca obrigatório | não houver consumidor nem N pods | é distribuição; durabilidade já vem do outbox |
| Redis | nunca obrigatório | **por padrão** | sem medição que justifique, é modo de falha sem ganho |

## Consequências

**Boas.** O cenário catastrófico é um modo suportado e testável, não um
acidente. Durabilidade deixa de depender de broker. Redis e RabbitMQ viram
otimizações honestas, com critério de entrada por medição. E os dois modos de
falha silenciosos medidos — fallback de banco e zumbi de sessão — passam a ser
recusa de arranque e sinal de saúde.

**Custos.** Uma tabela de outbox nova e o conteúdo de mensagens em disco.
Um lease a manter, com heartbeat e o risco de despejo falso sob pausa longa.
Roteamento por dono exige que a camada de ingress consiga rotear por
usuário — se não conseguir, D5 muda e o Redis entra.

**Não resolvido aqui.** O TTL ideal do lease; o comportamento sob partição de
rede (dois pods se achando donos, que o TTL mitiga mas não elimina); e a
política de retenção do outbox.

## Sequência sugerida

1. **D1** (modo explícito, fallback fatal, lock local em `single`) — é o que
   impede o zumbi hoje e não depende de nada.
2. **D6** (saúde separada) — sem isso não dá para observar os demais.
3. **D3** (outbox) — vale nos dois extremos, e substitui o retry em memória.
4. **D2** (lease) — habilita `multi`.
5. **Roteamento por dono** e só então reavaliar **D5**.

## Perguntas em aberto para validação

- Quanto tempo o Postgres leva para liberar um lease de um pod morto de forma
  abrupta, sob TTL de 15s? Medir com kill -9, não com desligamento gracioso.
- O despejo falso acontece sob que pausa? Medir com o processo suspenso
  (`SIGSTOP`) por mais que o TTL.
- Com dois pods e roteamento por dono, o WS reconecta sozinho quando a sessão
  troca de dono?
