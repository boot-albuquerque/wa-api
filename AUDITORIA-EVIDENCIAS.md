# Auditoria independente da classificação de evidência

**Data**: 2026-08-26
**Base auditada**: `2d965350` (`docs: sincroniza os cinco documentos com a padronizacao de caminhos`)
**Postura**: hostil. O objectivo foi **falsificar** cada classificação, não confirmá-la.

---

## 0. A base mudou durante a auditoria, e isso é o primeiro achado

O enunciado da auditoria dizia que o HEAD era `fea7e6e` e que o `evidencias.tsv`
"já não tem 141 linhas". **As duas afirmações são falsas**, e vale a pena
registá-lo porque a auditoria começou a medir a coisa errada:

| afirmação do enunciado | medido | veredicto |
|---|---|---|
| `evidencias.tsv` no HEAD já não tem 141 linhas | tem exactamente 141 (153 − 12 de cabeçalho) | **falsa** |
| o relatório está desactualizado face ao próprio HEAD | em `fea7e6e` bate com o tsv marca a marca | **falsa** |
| o trabalho por commitar em `wa-api-wa-noise` é o remate que falta | esse worktree está **limpo**, dois commits à frente | **falsa** |

O que existe de verdade:

```
fea7e6e  feat(rotas): padronizacao de caminhos          <- base original da auditoria
9a6b3ff  docs(rotas): as 91 canonicas no OpenAPI...     <- o remate
2d96535  docs: sincroniza os cinco documentos...        <- a sincronizacao dos docs
```

`fea7e6e` **é** um commit incompleto — parte gates e só é reparado dois commits
depois — mas não há trabalho perdido. Ver F284 no `HOUSEKEEP.md`.

A auditoria foi refeita sobre `2d96535` por *fast-forward* (`git merge
--ff-only feature/wa-noise`), que não escreve nada no worktree alheio. Auditar
`fea7e6e` teria produzido um gate que passa por vacuidade: nesse commit a
legenda ainda batia com o tsv, e a divergência real não existiria para ser
apanhada.

---

## 1. Tabela de reconciliação, fonte a fonte

Medido em `2d96535`, **antes** da correcção desta auditoria:

| fonte | operações | ✅ | 🟡 | ❌ | ⬜ | como foi medido |
|---|---:|---:|---:|---:|---:|---|
| `api/openapi/evidencias.tsv` | 232 | 178 | 13 | 6 | 35 | contagem da coluna 3 |
| spec embutida — marca de cada `summary` | 232 | 178 | 13 | 6 | 35 | YAML parseado, rota a rota |
| **legenda de `info.description`** | **141** | **98** | **8** | **3** | **32** | **DIVERGE — números de antes das canónicas** |
| `docs/OPENAPI-EVIDENCIAS.md` — resumo | 232 | 178 | 13 | 6 | 35 | bloco `Validação:` |
| `docs/OPENAPI-EVIDENCIAS.md` — por grupo | 232 | 178 | 13 | 6 | 35 | soma das 12 linhas |
| router (`go run ./cmd/listroutes`) | 234 | — | — | — | — | 232 + `/docs` + `/docs/` |

**Quatro das cinco fontes batem exactamente.** A que diverge é a pior possível:
a legenda é o que `/docs` serve a quem consome a API. Ela dizia, em três sítios:

- "As **141** estão documentadas" → são 232
- "Medição de 2026-08-26 […]: **98 ✅, 8 🟡, 3 ❌, 32 ⬜**" → 178 / 13 / 6 / 35
- "As **32** por testar não são esquecimento" → são 35

**Toda a suíte `pkg/bootstrap` passava com estes três números errados**
(`go test ./pkg/bootstrap/ -count=1` → `ok`, exit 0). Nenhum gate reconciliava
a legenda com a tabela. Corrigido nesta auditoria, e travado pelo gate novo.

### Uma armadilha de medição, registada porque quase inverteu a conclusão

O primeiro instrumento desta auditoria foi `grep` ao `openapi.yaml`, e ele
contou **zero 🟡** — o que parecia provar que oito operações perdiam a marca na
geração. Era falso. O emoji 🟡 é `U+1F7E1`, **fora do plano básico**, e o
serializador YAML escapa-o para `"\U0001F7E1"`, enquanto ✅ (`U+2705`), ❌
(`U+274C`) e ⬜ (`U+2B1C`) ficam literais por serem BMP.

O gerador estava certo; o instrumento é que era mais pobre que o formato. Por
isso o gate novo **parseia YAML** e nunca faz *grep* ao ficheiro. Está
comentado no código para que ninguém o "simplifique" de volta.

---

## 2. A qualidade das ✅ — o maior achado

### 2.1 As 98 ✅ têm todas a MESMA frase. Não a maioria: **todas**.

No último commit em que o relatório ainda tinha coluna de evidência (`fea7e6e`),
agrupando o texto de evidência das 98 ✅:

```
  98  chamada real com resposta e efeito confirmado por segunda leitura ou pelo cliente.
```

**98 genéricas, 0 específicas.** Uma única frase-modelo, repetida 98 vezes.

Isto não é evidência: é uma **afirmação agregada**. Não diz qual segunda
leitura, nem o que foi lido, nem que valor mudou, nem quando. Uma ✅ cuja prova
é uma frase-modelo é **indistinguível de uma ✅ por decreto** — e é
indistinguível *por construção*, porque a frase é idêntica para uma rota de
leitura trivial e para uma operação de escrita irreversível.

### 2.2 A assimetria é o que torna isto grave

As outras três marcas **documentam-se a si próprias**:

| marca | total | com evidência específica | genérica / "idem" | % específica |
|---|---:|---:|---:|---:|
| ❌ | 3 | 3 | 0 | **100%** |
| 🟡 | 8 | 6 | 2 | **75%** |
| ⬜ | 32 | 21 | 11 | **66%** |
| ✅ | 98 | **0** | 98 | **0%** |

Exemplos do que as outras marcas registam, e a ✅ não:

- ❌ — `500 ao fim de 30,0 s — context deadline exceeded; o servidor nunca responde (F265).`
- 🟡 — `200 e a mensagem chegou, mas o WebP e sintetico e o cliente desenha bolha vazia.`
- ⬜ — `alteraria o avatar da conta — proibido nesta sessao pelo utilizador.`

**A marca que carrega a afirmação mais forte é a única que não traz prova
nenhuma.** Isto inverte o ónus da prova exactamente ao contrário do que a
disciplina exige: as recusas estão fundamentadas e as aprovações não.

### 2.3 Não afirmo que as ✅ estejam erradas — afirmo que são inauditáveis

Não tenho acesso às sessões de WhatsApp usadas na medição e **não posso provar
que uma ✅ concreta seja falsa**. O achado é mais específico e mais defensável:
do registo escrito, é **impossível distinguir** uma ✅ medida de uma ✅
assumida. A frase-modelo destrói essa distinção para as 98.

### 2.4 A coluna de evidência foi APAGADA em `2d96535`

E o estado hoje é pior que o de `fea7e6e`. O commit `2d96535` regenerou o
relatório e, ao fazê-lo, trocou a última coluna da tabela completa:

```
@fea7e6e : | Grupo | Método | Endpoint | Documentado | Swagger | Teste | Evidência |
@2d96535 : | Grupo | Método | Caminho  | Forma       | Teste   | Título |
```

A coluna **Evidência** deixou de existir. Com ela desapareceram os 21 motivos
específicos das ⬜ e os 6 das 🟡 — a única parte do registo que tinha conteúdo
medido rota a rota:

```
$ grep -c 'apagaria uma sessao\|derrubaria\|alteraria o avatar' docs/OPENAPI-EVIDENCIAS.md
0
```

E **duas promessas ficaram a apontar para o vazio**:

- `evidencias.tsv`, cabeçalho: `⬜ não executada, com o motivo dito em docs/OPENAPI-EVIDENCIAS.md`
- `base.yaml`, legenda: `O detalhe rota a rota está em `docs/OPENAPI-EVIDENCIAS.md`.`

Um commit de documentação que **melhorou os números e destruiu a prova**. Ver
F283 no `HOUSEKEEP.md`. Hoje, em `2d96535`, a contagem honesta é **0 de 232
operações com evidência registada**.

---

## 3. ✅ estruturalmente impossíveis de confirmar por segunda leitura

Método: para cada ✅ de escrita, procurei na própria especificação a leitura que
poderia confirmá-la. Facto medido que sustenta a lista:

```
$ grep -oE '^\s+(archived|pinned|muted|starred|is_muted|unread_count):' openapi.yaml
(nenhum resultado)
```

`GET /chat/list` devolve **apenas** `is_group`, `jid`, `last_activity`, `name`,
`phone`, `push_name`. Não existe, em toda a API, campo que exponha arquivo,
fixação, silenciamento, leitura ou favorito.

**Dez operações ✅ (vinte, contando a forma canónica) não têm leitura de volta
em lado nenhum da API:**

| # | rota | por que a segunda leitura é impossível |
|---|---|---|
| 1 | `POST /chat/archive` | nenhum campo de arquivo em nenhuma resposta |
| 2 | `POST /chat/pin` | nenhum campo de fixação |
| 3 | `POST /chat/mute` | nenhum campo de silenciamento |
| 4 | `POST /chat/markread` | nenhum contador de não-lidas |
| 5 | `POST /message/star` | nenhum campo de favorito |
| 6 | `POST /chat/presence` | indicador de escrita: efémero por desenho |
| 7 | `POST /user/presence` | não há leitura da própria presença |
| 8 | `POST /user/presence/subscribe` | o efeito são eventos futuros, não estado legível |
| 9 | `POST /chat/ephemeral` | não há leitura do temporizador de conversa 1:1 |
| 10 | `POST /chat/ephemeral/default` | não há leitura do padrão da conta |

Para estas, o ramo "segunda leitura" da definição de ✅ é **provadamente
inaplicável**. A classificação só pode assentar no ramo "ou pelo cliente" — que
ninguém registou. **São as candidatas mais fortes a 🟡.**

### 3.1 Duas incoerências internas, pela norma do próprio documento

Não é opinião minha: são pares com a mesma estrutura de observabilidade e
marcas diferentes.

| ✅ | 🟡 gémea | motivo registado na 🟡 |
|---|---|---|
| `POST /chat/markread` | `POST /newsletter/mark-viewed` | *"200 com data:null; não há leitura que confirme a marcação."* |
| `POST /chat/presence` | `POST /newsletter/mark-viewed` | idem — ambos sinalizam sem estado legível |

A razão escrita para a 🟡 de `newsletter/mark-viewed` aplica-se, **palavra por
palavra**, a `chat/markread`. Uma das duas está mal classificada.

### 3.2 Candidatas que testei e DESCARTEI

A disciplina exige dizer também o que não se confirmou. Três suspeitas minhas
caíram quando fui verificar:

- `POST /chat/react` — **descartada**. `GET /chat/history` devolve `data_json`
  com a mensagem em bruto, onde a reacção é observável.
- `POST /user/contacts/sync` — **descartada**. `GET /user/contacts` permite a
  segunda leitura.
- `POST /user/history/sync` — **descartada**. `GET /chat/history` idem.

---

## 4. Veredicto sobre a herança das canónicas

### 4.1 A herança é literal, e verificável

Cruzando `caminhos.tsv` (91 pares) com `evidencias.tsv`:

```
=== iguais: 91 ===   (zero divergências)
```

**As 91 canónicas têm exactamente a marca da forma antiga.** Para as 81 que só
mudaram de caminho, isto é defensável: `RegisterCanonicalAliases` reutiliza o
`entrada.handler` da rota antiga — é literalmente o mesmo manipulador, logo a
mesma prova. Verifiquei no código, não no comentário.

### 4.2 Para as reestruturadas, a herança NÃO é justificada — e os números não fecham

Nas rotas com parâmetro no caminho há **código novo** entre o cliente e o
manipulador: `injectPathParams` (`pkg/presentation/http/canonico.go:78`). A
marca herdada atravessa esse código sem o ter exercitado.

O commit `9a6b3ff` afirma que "as **nove** reestruturadas foram RE-MEDIDAS".
**Três números diferentes descrevem o mesmo conjunto:**

| fonte | diz | medido |
|---|---:|---|
| mensagem de `9a6b3ff` | **9** | re-medidas ("oito responderam; a nona deu 400") |
| `canonico.go:57` — `strings.Contains(CanonicalPath, "{")` | **12** | operações embrulhadas pelo adaptador |
| canónicas com `{group_jid}` ou `{community_jid}` | **10** | operações que precisam mesmo da injecção |
| `docs/ENDPOINTS.md:566` — secção "As **nove** que mudaram de forma" | **9** no título, **12** na tabela | a tabela contradiz o próprio título |

**Consequências, medidas:**

1. **Pelo menos uma das 10 rotas que atravessam o adaptador nunca foi
   re-medida.** O registo documenta 9 medições (8 + a do `400 invalid_phone`)
   para 10 rotas que precisam delas. A marca dessa rota é **herança disfarçada
   de medição** — exactamente o que esta auditoria foi procurar.

2. **Não é possível dizer QUAL**, porque nenhuma fonte enumera as nove. É a
   falha de "afirmação agregada" outra vez: um número sem o conjunto não é
   auditável. Este é o achado, não uma limitação da auditoria.

3. **Duas rotas são embrulhadas sem precisarem.** `GET /users/lid/{jid}` e
   `GET /users/profile/{jid}` já tinham o parâmetro no caminho na forma antiga
   — nada sai do corpo. Ainda assim `injectPathParams` corre nelas, porque a
   condição é "o caminho contém `{`". Como `bodyFieldForPathParam` só conhece
   `group_jid` e `community_jid`, o parâmetro `jid` não é injectado: o adaptador
   lê o corpo, não injecta nada, e **reescreve o corpo como `{}`**. Funciona
   por acidente — o manipulador lê o `{jid}` de `mux.Vars` —, mas é código a
   correr onde não devia. Ver F285 no `HOUSEKEEP.md`.

4. **A tabela de ENDPOINTS.md diz "nove" e lista doze**, incluindo essas duas,
   sob a frase "Nestas o identificador **sai do corpo** e vai para o caminho" —
   que é falsa para elas.

**Veredicto**: a herança está *tecnicamente correcta* para as 81 que só mudaram
de caminho e *não está estabelecida* para as reestruturadas. A afirmação
"foram RE-MEDIDAS" é verdadeira para pelo menos 8 e falsa ou não provada para
pelo menos 1, e o registo não permite saber quais.

---

## 5. Violações da regra de transição

**Não há violações, porque não há transições.** Este é o achado, não uma
absolvição.

```
$ git log --oneline -- api/openapi/evidencias.tsv
b0b8323 docs(openapi): marca de evidencia no titulo de cada endpoint
9a6b3ff docs(rotas): as 91 canonicas no OpenAPI e no ENDPOINTS.md
```

- `b0b8323` **cria** a tabela já com 98 ✅ / 8 🟡 / 3 ❌ / 32 ⬜ (153 linhas, `1 file changed, 153 insertions(+)`).
- `9a6b3ff` **acrescenta** as 91 canónicas, todas por herança.
- Comparando as 141 linhas antigas entre `fea7e6e` e `2d96535`:
  `rotas antigas ainda presentes: 141`, **zero transições**.

Ou seja: **nenhuma marca deste repositório foi alguma vez promovida.** Todas
nasceram classificadas num único commit, ou foram copiadas de uma linha vizinha
por tabela. A regra "é proibido promover testando apenas o caminho de erro"
nunca teve oportunidade de ser violada — e também nunca foi exercida, o que
significa que **não há prova de que alguma ✅ tenha passado por ela**.

Combinado com §2.1 — as 98 ✅ nasceram no mesmo commit, com a mesma frase — a
leitura honesta é: as ✅ são uma **classificação inicial não revista**, não o
resultado de um processo de promoção.

---

## 6. O gate de reconciliação

`pkg/bootstrap/openapi_reconciliation_test.go`, cinco testes:

| teste | o que trava |
|---|---|
| `TestEvidenceMarkConstantsMatchGenerator` | as constantes das marcas não divergirem da lista que o gate antigo usa |
| `TestEvidenceTableMatchesSpec` | tsv × spec **rota a rota** (não só o total) |
| `TestEvidenceLegendMatchesTable` | a legenda de `info.description` × tsv — **a divergência medida** |
| `TestEvidenceReportMatchesSpec` | relatório × spec **por grupo**, e o Total × soma das parcelas |
| `TestEvidenceReportSummaryMatchesTable` | bloco `Validação:` do relatório × tsv |

Duas decisões de desenho vêm directamente de regras deste projecto:

- **Rota a rota, não só o total.** Uma rota a subir de 🟡 a ✅ e outra a descer
  fechariam o total com o defeito lá dentro. O gate compara chaves.
- **Total contra a soma das parcelas**, além de contra a fonte. O Total é uma
  afirmação agregada e é conferido contra os membros, nunca aceite como mais
  uma linha.

### 6.1 Controlos negativos — EXECUTADOS

**Controlo A** — inverter uma marca em `evidencias.tsv` (`POST /chat/pin` ✅ → 🟡), sem regenerar:

```
EXIT_APOS_MUTACAO=1
--- FAIL: TestEvidenceTableMatchesSpec
    marca divergente — POST /chat/pin: tabela 🟡, especificação ✅
--- FAIL: TestEvidenceLegendMatchesTable
    legenda diz 178 ✅, mas evidencias.tsv tem 177
    legenda diz 13 🟡, mas evidencias.tsv tem 14
--- FAIL: TestEvidenceReportMatchesSpec
    linha Total diz 178 ✅, mas evidencias.tsv tem 177
--- FAIL: TestEvidenceReportSummaryMatchesTable
    resumo diz OK 178, mas evidencias.tsv tem 177 ✅
```

Uma marca trocada acende **quatro** gates e **nomeia a rota**. Revertido.

**Controlo B** — mexer **só** na tabela por grupo do relatório (`Grupos`: 34 ✅ → 33, 2 🟡 → 3):

```
EXIT_APOS_MUTACAO=1
--- FAIL: TestEvidenceReportMatchesSpec
    grupo "Grupos": relatório diz 33 ✅, especificação tem 34
    grupo "Grupos": relatório diz 3 🟡, especificação tem 2
    linha Total diz 178 ✅, mas as parcelas somam 177
    linha Total diz 13 🟡, mas as parcelas somam 14
```

Apanhado **duas vezes** — pelo grupo e pela soma. Revertido.

**Controlo C** — mexer na legenda em `base.yaml` (178 → 179) e regenerar:

```
EXIT_APOS_MUTACAO=1
--- FAIL: TestEvidenceLegendMatchesTable
    legenda diz 179 ✅, mas evidencias.tsv tem 178
```

Revertido.

**Controlo D — o mais forte, e não foi encenado**: na sua primeira execução, o
gate falhou contra o repositório **tal como estava**, com seis erros, e
identificou a divergência real da legenda (§1). Um gate cujo primeiro acto é
apanhar um defeito verdadeiro não precisa de prova de que morde.

Depois da correcção: `go test ./pkg/bootstrap/ -count=1` → `ok`, exit **0**.

---

## 7. Resumo dos achados

| # | achado | gravidade | estado |
|---|---|---|---|
| 1 | Legenda servida em `/docs` com números de antes das canónicas (141/98/8/3/32 vs 232/178/13/6/35), com toda a suíte verde | **alta** — é o que o consumidor lê | **corrigido** + gate |
| 2 | 98 de 98 ✅ com a mesma frase-modelo; 0 com evidência específica | **alta** — as ✅ são inauditáveis | F282, parcial |
| 3 | Coluna "Evidência" apagada em `2d96535`; hoje 0 de 232 operações têm evidência registada, e duas promessas apontam ao vazio | **alta** | F283 |
| 4 | 10 rotas atravessam código novo, 9 medições registadas, nenhuma fonte enumera quais | média | F286 |
| 5 | 10 ✅ sem qualquer leitura de volta na API; 2 incoerentes com a 🟡 gémea | média | F282 |
| 6 | `fea7e6e` parte gates e só é reparado dois commits depois (perigo de bisect) | média | F284 |
| 7 | `injectPathParams` corre em 2 rotas que não precisam e reescreve o corpo como `{}` | baixa | F285 |
| 8 | `ENDPOINTS.md` diz "nove" e lista doze, sob uma frase falsa para duas delas | baixa | F286 |
| 9 | Zero transições em toda a história da tabela: nenhuma ✅ foi promovida | contexto | **deixou de valer** — ver nota da integração |

### O que NÃO consegui medir, e porquê

- **Se alguma ✅ concreta é falsa.** Não tenho as sessões de WhatsApp. Provei que
  são inauditáveis a partir do registo, não que estejam erradas.
- **Qual das 10 rotas do adaptador ficou sem re-medição.** Nenhuma fonte
  enumera as nove; é precisamente o achado #4.
- **`TestGolden` e `cmd/logcov` em `fea7e6e`.** Medi directamente, antes do
  *fast-forward*, `TestOpenAPICobreTodasAsRotasRegistadas` (91 rotas sem
  entrada) e `TestRegisteredHTTPRoutesHaveStdioEntry`. Os outros dois vêm da
  medição de outro worker e estão citados como tal no `HOUSEKEEP.md` (F284).
- **O falso positivo do `orphan-browser-check`.** O enunciado avisava que ele
  acusaria como órfão um browser cujo dono está vivo, e mandava não correr
  `make orphan-browser-clean`. Não corri. Mas no `make check` desta sessão o
  passo saiu limpo — `orphan-browser-check: nenhum browser de teste
  sobrevivente.` **Não reproduzi o defeito**, e por isso não o registei como
  achado numerado: a condição parece depender de haver outra suíte a correr em
  paralelo no momento exacto.

---

## 8. Recomendações

1. **Substituir a frase-modelo por evidência real, ou baixar a marca.** Uma ✅
   deve trazer o que foi lido de volta e o valor observado. Enquanto isso não
   existir, as 10 rotas de §3 são 🟡 por definição.
2. **Repor a coluna de evidência** no relatório gerado — os motivos das ⬜ e das
   🟡 estão em `fea7e6e` e são recuperáveis do git.
3. **Enumerar as reestruturadas** e re-medir as 10, uma por uma, com o
   resultado ao lado do nome.
4. **Fazer da evidência um campo da tabela**, não prosa num documento gerado. O
   que não é gerado da fonte única volta a divergir — foi essa a causa
   estrutural do achado #1.

---

## Nota da integração — 2026-08-26

Esta auditoria mediu `2d965350`, onde o contrato tinha **232 operações** porque
documentava as formas antigas ao lado das canónicas. `3a0b48b4` removeu as
antigas do contrato, e a base sobre a qual esta auditoria foi integrada tem
**141 operações, um nome por capacidade**.

O que sobrevive à mudança de base, e o que não:

| achado | sobrevive? | porquê |
|---|---|---|
| F281 (legenda divergente) | **sim** — o gate foi portado | ele não conhece número nenhum: lê a fonte única e exige que as outras três a espelhem |
| F282 (as ✅ inauditáveis) | **sim, atenuado** | 43 das 141 têm hoje observador concreto na coluna Evidência; as 98 restantes continuam com a frase-modelo |
| F283 (coluna de evidência apagada) | **corrigido** | a coluna voltou à tabela completa |
| F284 (`fea7e6e` parte gates) | sim | é facto histórico sobre um commit que continua no histórico |
| F285, F286, F287 | sim | tocam código e documentos que `3a0b48b4` não alterou |
| F288 (`make check` já falhava) | sim | continua a falhar, e é a falha pré-existente conhecida |

**O achado #9 deixou de valer, e é a melhor notícia desta integração.** Ele
dizia que a tabela nunca tinha registado uma transição: nenhuma rota alguma vez
subiu de marca. A campanha da sessão descartável produziu **25** — 24 ⬜ → ✅ e
uma ⬜ → ❌ — todas com observador independente e registadas rota a rota em
`CAMPANHA-DESCARTAVEL.md`.

**Os números desta auditoria são os de `2d965350` e ficam como estão** — reescrevê-los
para o estado de hoje apagaria a medição que produziu o achado. O estado actual
está em `docs/OPENAPI-EVIDENCIAS.md` e na legenda de `api/openapi/base.yaml`:
**141 operações — 122 ✅, 8 🟡, 4 ❌, 7 ⬜**.
