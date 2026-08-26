# Production Readiness Scorecard — wa-api

**Data**: 2026-08-26, revisto depois da padronização de caminhos. **Medido**, não estimado: cada número desta página vem de
um comando cuja saída está registada nos commits desta série.

## Como ler este documento

O eixo é **implementação → contrato explícito → testes → evidência → auditoria
→ produção**. É deliberadamente agnóstico de tecnologia: as perguntas valem
para qualquer API HTTP, e as respostas é que são deste projeto.

A regra de preenchimento é uma só:

> Uma linha só fica ✅ se houver um **comando que a prove** e que falhe quando
> a propriedade se perder. Sem gate, é 🟡 — "está feito hoje" não é o mesmo que
> "continuará feito".

Um scorecard que se auto-atribui verdes é um instrumento de conforto. Este tem
🟡 e ❌ de propósito, e a coluna que interessa é a última.

---

## Resumo

| estágio | ✅ | 🟡 | ❌ |
|---|---:|---:|---:|
| 1. Implementação | 3 | 2 | 1 |
| 2. Contrato explícito | 10 | 3 | 2 |
| 3. Testes | 5 | 1 | 1 |
| 4. Evidência | 4 | 1 | 0 |
| 5. Auditoria | 5 | 0 | 0 |
| 6. Produção | 2 | 4 | 4 |
| **Total** | **29** | **11** | **8** |

**A leitura honesta**: o contrato e a evidência estão fortes; a **prontidão
operacional não está**. Uma API pode ser perfeitamente documentada e continuar
imprópria para escala — é o caso aqui, e os ❌ do estágio 6 dizem porquê.

---

## 1. Implementação

| propriedade | estado | prova / lacuna |
|---|---|---|
| Todas as rotas registadas são servidas | ✅ | `TestOpenAPICobreTodasAsRotasRegistadas` — enumera de `Routes(Deps{})`, a mesma função que serve o processo |
| Erro interno nunca vaza no corpo | ✅ | `TestRespondJSONNaoVazaDetalheDeErroInterno`, com par de controlo. Sem stack trace, caminho, SQL ou segredo — **em ambiente nenhum** |
| Campo desconhecido é observável | ✅ | log `unknown field "<nome>"` em 48 sítios de descodificação; `WA_API_STRICT_UNKNOWN_FIELDS` recusa |
| Ordem de validação previsível | 🟡 | medida rota a rota e documentada; **não há regra** que a garanta ao acrescentar um campo |
| Valor zero com significado usa ponteiro | 🟡 | regra escrita (contrato §4.5) e aplicada em `mute_chat.go`; **não há gate** que a imponha |
| Erro por campo (`error.details[]`) | ❌ | não existe. A API devolve **um** código, o do primeiro campo que falha. Um pedido com três campos errados revela-os um de cada vez |

## 2. Contrato explícito

| propriedade | estado | prova / lacuna |
|---|---|---|
| Cobertura OpenAPI | ✅ | **232 de 232** operações (234 rotas − 2 de `/docs`, excluídas de propósito) |
| Toda operação tem tag, título e descrição | ✅ | `TestOpenAPIOperacoesEstaoCompletas` |
| **Toda propriedade tem semântica de presença** | ✅ | **694 de 694** com `description` **e** `nullable`. `TestTodaPropriedadeDeclaraNulabilidade` |
| Enums dizem o que acontece fora da lista | ✅ | `TestTodoEnumDizOQueAconteceComValorDesconhecido` |
| Exemplos coerentes com o tipo e com o esquema | ✅ | `TestContratoExemplosSaoValidosContraOTipo` + `TestContratoExemploDeErroBateComOEsquema` (125 exemplos) |
| Códigos de erro documentados existem no código | ✅ | `TestContratoCodigosDeErroExistemNoCodigo` (112 códigos) |
| Nenhuma credencial real na especificação | ✅ | `TestOpenAPINaoTrazSegredoReal` |
| Segredos marcados `writeOnly` | ✅ | 13 propriedades; 69 `readOnly` |
| Envelope de erro único | 🟡 | **duas** formas em produção: `error` objecto e `error` string (F266, 14 pontos). Ambas documentadas, o que impede o cliente de partir — mas é um contrato com duas caras |
| Nomes consistentes | 🟡 | glossário escrito e vinculante para o novo; o existente mistura `snake_case`, `camelCase` e `PascalCase`, e `group_jid` tem **quatro** grafias (F269) |
| Prosa da documentação verificada | 🟡 | os gates leem estrutura, não texto. Oito menções de um código inexistente sobreviveram em descrições até serem apanhadas à mão (ARMADILHAS #29) |
| Singular/plural coerente | ✅ | **91 formas canónicas** registadas a 26/08, com gate que recusa família de colecção sem canónica. As antigas continuam a funcionar, marcadas `deprecated` |
| Relação no caminho, não no nome | ✅ | 9 rotas reestruturadas com o identificador no caminho e o método a dizer a operação; `TestCaminhosCanonicosCumpremARegra` recusa a relação colada |
| Códigos de estado semânticos | ❌ | `201`, `202`, `204`, `410`, `412`, `415`, `429`, `502`, `503`, `504` **nunca** são usados. `POST /group/create` cria recurso e devolve `200` sem `Location` |
| `additionalProperties: false` | ❌ | não declarado, e **corretamente**: seria falso enquanto o padrão for aceitar. Ligar o modo estrito é decisão de versão |

## 3. Testes

| propriedade | estado | prova / lacuna |
|---|---|---|
| Especificação está em dia com as fontes | ✅ | `TestOpenAPIGeradoEstaAtualizado` |
| Respostas reais batem com o documentado | ✅ | `TestContratoRespostaDeRecusaBateComOEsquema` — **232 operações exercitadas** contra o router |
| Os testes de contrato não podem ficar cegos | ✅ | piso mínimo em cada um; falham se exercitarem menos do que o esperado |
| Controlo negativo em cada gate novo | ✅ | executado e colado nos commits. Duas vezes o primeiro não valeu por partir o build — refeito com mutação que compila (ARMADILHAS #4) |
| Suite passa | ✅ | 116 pacotes verdes |
| Caminho de SUCESSO coberto por contract test | 🟡 | os contract tests exercitam a **recusa** — é o único caminho percorrível sem conta ligada. Está dito no comentário do ficheiro em vez de implícito |
| `make check` totalmente verde | ❌ | uma falha, `internal/wa-headless`, **herdada** e anterior a esta série. Fora do âmbito por instrução |

## 4. Evidência

| propriedade | estado | prova / lacuna |
|---|---|---|
| Classificação por rota, visível no título | ✅ | ✅🟡❌⬜ no `summary` de cada operação, aplicada de `evidencias.tsv` pelo gerador |
| Nenhuma rota sem classificação | ✅ | `TestOpenAPISummariesTrazemMarcaDeEvidencia`; **232 de 232** |
| Tabela não pode desactualizar-se | ✅ | rota nova sem linha **falha o gerador**; linha órfã **também** — a falha por excesso é a silenciosa |
| Distinção ✅ / 🟡 respeitada | ✅ | 🟡 é a recusa deliberada de chamar confirmado o que só devolveu `2xx`. **178 ✅, 13 🟡, 6 ❌, 35 ⬜** |
| Cobertura de efeito confirmado | 🟡 | **178 de 232**. As 35 ⬜ não são esquecimento: escreveriam configuração em uso, derrubariam sessões, ou alterariam a conta — cada uma com o motivo escrito |

## 5. Auditoria

| propriedade | estado | prova |
|---|---|---|
| Achados registados com medição | ✅ | **F256** a **F271** no `HOUSEKEEP.md`, com comando e saída |
| Defeitos não mascarados na documentação | ✅ | as 3 rotas que falham estão documentadas **como falhando**, com o erro medido |
| Correcções não alteram contrato sem aval | ✅ | as 3 de comportamento só mudam respostas que **já falhavam**; a padronização de caminhos é puramente ADITIVA — nenhuma rota antiga mudou |
| Erros do auditor também registados | ✅ | 5 meus nesta série: contador mais grosseiro que o gate, acusação injusta a um executor, F267 a apanhar-me, e duas mutações que partiram o build |
| Armadilhas catalogadas | ✅ | 53 entradas em `ARMADILHAS.md`, cada uma com a evidência que a produziu |
| Padronização não parte cliente | ✅ | 143 rotas antigas continuam a responder; medido `GET /chat/list` e `GET /chats/list` a devolverem ambos `200` |

## 6. Produção

**É aqui que a API não está pronta**, e nenhuma quantidade de documentação o
resolve.

| propriedade | estado | prova / lacuna |
|---|---|---|
| Rastreio por pedido | ✅ | cabeçalho `Request-Id` em toda resposta, incluindo erro; `req_id` em todos os logs |
| Logs estruturados | ✅ | `req_id`, `method`, `route` normalizada, `status`, `duration_ms`, `outcome`, `size`. Segredos nunca registados |
| `request_id` no corpo do erro | 🟡 | só no cabeçalho. Correlacionável, mas o cliente tem de o ler de outro sítio |
| Prazos em dependência externa | 🟡 | 30 s por operação contra o WhatsApp, uniforme. **Não é configurável por rota**, e é o que produz o `500` de `/newsletter/updates` |
| Sondas separadas | 🟡 | `/livez` e `/health/ready` distinguem vivacidade de prontidão. Mas `/health` — a funcional — está **atrás de token**, o que a torna inútil para um balanceador |
| **Paginação** | ❌ | **nenhuma colecção é paginada**. `GET /user/contacts` devolveu **61 459 bytes** com 1266 contactos, sem limite nem cursor. Cresce com a agenda do utilizador |
| **Versionamento** | 🟡 | continua sem `/v1`. Mas a padronização de 26/08 mostrou que **alias lado a lado** resolve tudo o que é ADITIVO — um caminho novo coexiste com o antigo, sem versão. `/v1` continua pré-requisito para o que **não pode coexistir**: mudar o status code de uma rota, ou unificar o envelope de erro |
| **Idempotência** | ❌ | não há `Idempotency-Key`. `POST /chat/send/text` chamado duas vezes envia **duas** mensagens; um `500` pode ter enviado |
| **Limitação de ritmo** | ❌ | nenhuma rota devolve `429`. O `x/time/rate` que existe protege a CONTA no envio, não o servidor |
| **Concorrência** | ❌ | sem `ETag`, `If-Match` ou versão. Duas escritas simultâneas ao mesmo recurso perdem uma, em silêncio |

---

## O que falta, por ordem de dependência

A ordem não é de importância — é de **dependência**. Fazer fora de ordem
duplica trabalho:

1. ~~**`/v1`** como pré-requisito do naming.~~ **Feito de outra forma**: a
   padronização de 26/08 registou as formas canónicas **ao lado** das antigas,
   sem versão. Descobriu-se que o alias resolve tudo o que é ADITIVO — um
   caminho novo pode coexistir com o antigo. O que `/v1` ainda é
   pré-requisito é para o que **não pode coexistir**: mudar o status code de
   uma rota, ou unificar o envelope de erro.
2. **Paginação em `/user/contacts`**, `/chat/list` e `/group/list`. É o único
   ❌ que degrada **sozinho**, com o crescimento dos dados do utilizador.
3. **`429` e limitação de ritmo** — o número certo depende do perfil de uso e é
   decisão de produto, não de engenharia.
4. **Idempotência nas rotas de envio**, com `Idempotency-Key`.
5. **`/v2`** com naming canónico, códigos de estado semânticos, envelope de
   erro único, `error.details[]` e `additionalProperties: false`. Tudo o que
   parte contrato, de uma vez, num sítio onde partir é legítimo.

Os pontos 1 a 4 são compatíveis com todos os clientes actuais. O ponto 5 não é,
e é por isso que vem depois do 1.

## Duas coisas que este scorecard NÃO afirma

- **Que a API funciona.** Afirma que 178 operações tiveram efeito confirmado,
  13 responderam sem observador, 6 falham e 35 não foram exercitadas. A
  diferença entre isto e "funciona" é o assunto inteiro da coluna de evidência.

  E note-se de onde vem o salto de 98 para 178: **não** de mais medição, mas de
  as rotas terem duplicado com a padronização. As 91 canónicas herdam a prova
  da antiga que substituem, porque são o mesmo manipulador noutro caminho. Um
  número maior que não representa mais trabalho de verificação — e dizê-lo é o
  que impede a tabela de parecer melhor do que é.
- **Que os gates cobrem tudo.** Eles leem estrutura, não prosa
  (ARMADILHAS #29), e verificam o repositório, não o binário em execução
  (ARMADILHAS #27). As duas limitações estão escritas nos próprios gates.
