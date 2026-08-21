# Armadilhas deste projeto

Catálogo de defeitos que **já passaram por revisão e por testes verdes** neste
repositório. Não é teoria: cada entrada aconteceu, tem a evidência medida e o
que a expôs.

Leia antes de mexer em identidade (LID/PN), rotas HTTP, junção de dados ou
qualquer coisa que grave no banco. O padrão comum de quase todas é o mesmo:
**o teste concordava com o defeito**.

---

## 1. Dublê mais permissivo que a produção esconde o defeito

O modo de falha mais frequente aqui. O teste passa, a rota real quebra.

**Aconteceu três vezes em 2026-08-08:**

| dublê | o que ele fazia | o que a produção faz | efeito |
|---|---|---|---|
| `ResolveQualifiedJID` fake | qualificava número nu | **rejeita** sem servidor | `/user/profile/5511999` dava erro só em produção |
| `spyPort.ResolveJID` | concatenava `@s.whatsapp.net` cego | preserva servidor existente | `...@s.whatsapp.net@s.whatsapp.net` |
| dublês de `list_chats` | chaveavam tudo no mesmo espaço | histórico mistura `@lid` e `@s.whatsapp.net` | 7.273 nomes no banco, 11 na lista |

**Regra**: quando um dublê imita uma REGRA (parsing, normalização, resolução),
ele tem de imitar a regra real, e o comentário do dublê deve dizer de onde ela
vem — com caminho de arquivo. Se a regra é `mapping/jid/parse.go:16-27`, o
dublê cita isso.

**Como pegar**: escreva um teste que force os dois lados a divergirem. Se o
dublê e a produção concordam sempre, o teste não está medindo a regra.

---

## 2. Testes que codificam o defeito como contrato

Testes verdes que provavam o comportamento errado.

- **F79** — `/session/disconnect` e `/session/logout` devolviam 200 sem
  encerrar nada. A suíte só exercitava a RECUSA da guarda; um use case que
  valida e não age passava com folga. **O caminho de sucesso não era testado
  por ninguém.**
- **F81** — `GET /user/lid/{jid}` dava 400 sempre. O teste servia o handler
  **cru**, sem padrão de rota, e mandava o corpo que o handler lia. Teste
  verde, rota quebrada.
- **F82** — o teste do walog rodava sem banco, e o comentário dizia "sem
  banco, sem HTTP". Aquilo só era verdade **por causa do defeito**: a guarda
  saía antes da escrita.

**Regra**: para todo use case, teste o caminho de SUCESSO, não só a recusa.
Para toda rota, teste pela rota REGISTRADA. Se um teste roda sem uma
dependência que a produção exige, pergunte por que — pode ser o bug.

---

## 3. Comentário que afirma comportamento inexistente

`nomesDoHistorico` dizia: *"a normalização acontece depois, em `montarResumo`,
que tenta as duas formas"*. **Não tentava.** O comentário foi escrito junto do
código, na mesma sessão, pelo mesmo autor.

**Regra**: comentário que descreve o que OUTRA função faz é dívida no
nascimento. Ou o teste prova a afirmação, ou ela não entra. Ao ler um
comentário assim, verifique antes de confiar.

---

## 4. Controle negativo que não compila não prova nada

Reintroduzir o defeito e ver o teste falhar é obrigatório (ver `CLAUDE.md`).
Mas várias tentativas hoje **não dispararam porque a edição quebrou o build** —
e um controle que não roda é indistinguível de um que não morde.

Casos reais:
- trocar embedding anônimo por campo nomeado → quebra promoção de campos →
  não compila. A variante que compila é manter anônimo e **adicionar tag
  JSON**.
- remover a única chamada a um pacote → import órfão → não compila. Remova o
  import junto.

**Regra**: se o controle negativo não produzir uma FALHA DE TESTE com
mensagem, ele não valeu. Ajuste a mutação até compilar e falhar.

---

## 5. Medir, não inferir — e conferir a própria medição

- Concluí que o fallback LID→PN "recuperava **zero**" a partir de um JOIN SQL
  que eu mesmo escrevi errado: `wanoise_lid_map` guarda LID e PN **sem
  sufixo** (`90000000000002`, não `90000000000002@lid`). Com o sufixo
  removido, recuperava 73. **Quase descartei o caminho por causa da minha
  consulta.**
- Reportei "17 de 481" a partir de amostra da API; contra o banco inteiro era
  "261 de 1717".

**Regra**: antes de concluir a partir de uma consulta, valide-a num caso que
você SABE que deveria casar. Uma contagem zero é suspeita, não conclusiva.

---

## 6. LID e PN são o mesmo tipo Go

`types.JID` não distingue telefone de LID — só o `Server` em tempo de
execução (`@s.whatsapp.net` vs `@lid`). O compilador não ajuda.

- **F65** — `GetManyLIDsForPNs` devolvia o mapa INVERTIDO em produção. O
  dublê estava invertido do mesmo jeito, então os dois combinavam e o teste
  ficava verde.
- **F84 (2ª parte)** — tabela de consulta num espaço, chave de busca noutro.

**Regra**: toda tabela de consulta chaveada por JID precisa ser consultada nos
DOIS espaços, ou aliasada para um só (ver `aliasarParaLID`). E todo teste de
resolução tem de fixar a DIREÇÃO: LID de entrada não pode chamar
`GetLIDForPN`, e vice-versa.

---

## 7. `ON CONFLICT DO NOTHING` torna defeito de dado irrecuperável

`message_history` usava `DO NOTHING`. Como o logout não apaga a tabela, um
HistorySync novo traz as MESMAS `message_id` e o insert era descartado
inteiro: **nenhuma instalação existente se curaria ao atualizar**.

A cláusula certa preenche o que falta sem sobrescrever o que existe:

```sql
ON CONFLICT (...) DO UPDATE SET col = EXCLUDED.col
 WHERE EXCLUDED.col <> '' AND (tabela.col IS NULL OR tabela.col = '')
```

**Regra**: ao corrigir um bug que gravou dado errado, pergunte como o dado
JÁ GRAVADO se recupera. Se a resposta for "não recupera", isso é parte do
defeito, não uma limitação aceitável.

---

## 8. Nunca sobrescrever um valor com vazio

A Evolution API ([#2426](https://github.com/EvolutionAPI/evolution-api/issues/2426))
zerava `Contact.pushName` a cada mensagem enviada porque o upsert não tinha
guarda contra vazio — enquanto o caminho de `Chat.name` tinha.

**Regra**: em qualquer upsert de nome/rótulo, a guarda `<> ''` é obrigatória.

---

## 9. Rota se testa pelo roteador, e o roteador aqui é gorilla/mux

`r.PathValue` **não funciona** neste projeto — é do `ServeMux` nativo, e o
router é `gorilla/mux` (`pkg/bootstrap/router.go:237`). Use `mux.Vars(r)`.

A correção óbvia da F81 (`r.PathValue`) estaria igualmente quebrada; foi pega
por um controle negativo dedicado.

**Regra**: teste de rota monta `mux.NewRouter()` com o MESMO padrão de
`wiring_routes.go`.

---

## 10. Endpoints de rede não se chamam em laço

Um laço de **seis** chamadas a `/user/profile` disparou
`429: rate-overlimit` do usync do WhatsApp.

**Regra**: rota que faz chamada de rede por consulta não entra em laço sobre
uma lista. Para enriquecer N itens, ou existe chamada em LOTE (como
`GetJoinedGroups`, que traz todos os nomes de grupo de uma vez) ou o
enriquecimento é sob demanda com cache.

---

## 11. Verificação em produção não é opcional

Os dois defeitos mais graves de 2026-08-08 passaram por revisão, testes
verdes e `make check` — e só apareceram **medindo contra dados reais**:

- 7.273 nomes no banco, 11 na lista.
- 0 de 23.836 mensagens com nome depois de um "fix" que os testes aprovavam.

**Regra**: para defeito de forma de dado ou de junção, o teste unitário prova
que o mecanismo funciona; só a medição em produção prova que ele funciona
**nos dados que existem**. Registre o ANTES antes de mexer — sem linha de
base, o depois não significa nada.

---

## 12. Não sobrescrever arquivo sem olhar o diff

`cp CLAUDE.md AGENTS.md` depois de o próprio script informar que os dois
**diferiam**. A perda foi uma linha (o título) e o git recuperou, mas a
decisão estava errada independentemente do tamanho do estrago.

**Regra**: antes de sobrescrever, `diff`. O aviso de que os arquivos diferem
é o momento de parar, não de continuar.

---

## 13. O seu método de medição também é suspeito

Três varreduras seguidas erraram em 2026-08-08, todas minhas, todas sobre
documentos que estavam corretos:

| o que a varredura disse | por quê | risco |
|---|---|---|
| "LID→PN recupera **zero**" | JOIN comparava `90937@lid` com `90937` — a tabela guarda **sem sufixo** | quase descartei um caminho que recuperava 73 |
| "5 entradas **sem status**" | regex exigia veredito em **negrito**; estavam em texto simples | quase acrescentei status a entradas que já o tinham |
| "4 entradas **sem status**" | regex exigia `**Status**` no início da linha; eram item de lista | quase reescrevi entradas íntegras |

Nos três casos o dado estava certo e a ferramenta errada — e nos três a
conclusão parecia acionável.

**Regra**: antes de agir sobre o resultado de uma varredura, rode-a num caso
que você SABE que deveria casar. Se ele não casar, o problema é a varredura.
Uma contagem **zero** ou um "não encontrado" é sinal de suspeita, não
conclusão.

**Corolário**: quando a varredura contradiz o que o documento aparenta,
abra o documento antes de "corrigi-lo". Documento bem escrito perdendo para
regex ingênua é o caso comum, não o raro.

---

## 14. O instrumento de medida precisa alocar, bloquear e falhar como o original

A entrada 13 diz que a varredura é suspeita. Esta diz que o **harness**
também é — e o modo de falha é pior, porque um harness ruim produz números
que parecem dados.

Medindo o teto de despacho da F86, o primeiro harness usava `time.Sleep` para
simular a entrega. Resultado: heap **idêntico** com e sem teto (4,7MB), o que
levaria à conclusão "goroutine sem teto não custa nada, não faça nada".

`time.Sleep` não aloca. Uma entrega real segura `http.Request`, buffers de
resposta e estado TLS. Refeito com HTTP de verdade contra um servidor lento e
payload de 8KB:

| | goroutines | heap pico |
|---|---|---|
| sem teto | ~4.000 | **~32MB** |
| teto=64 | ~350 | **~13MB** |

A conclusão inverteu. E apareceu de quebra que 800 entregas viram ~4.000
goroutines — o transporte HTTP cria goroutines internas por conexão, coisa
que a leitura do código não mostrava.

**Regra**: antes de confiar num harness, pergunte o que ele NÃO faz que o
original faz. Se o dublê não aloca, não bloqueia em rede e não falha como o
real, ele mede outra coisa — e o número dele é pior que nenhum número,
porque tem aparência de evidência.

**Corolário**: rode a medição pelo menos três vezes. Uma amostra não separa
efeito de ruído, e o heap em particular depende de quando o GC passou.

---

## 15. A propriedade que você documentou não é a que você internalizou

O limitador da F86 tem aquisição **bloqueante**, e eu escrevi isso no
comentário: *"aquisição bloqueante é deliberada — segurá-la por um instante é
exatamente o backpressure que falta"*.

Duas horas depois, meu próprio teste travou por 600s porque emitia despachos
num laço com teto 1: o segundo bloqueou o laço, e o `close()` que liberaria
o primeiro nunca chegou.

E o deadlock revelou o que o comentário não dizia: em produção o chamador é a
goroutine do handler de eventos do SDK. Segurá-la não é backpressure sobre
uma fila — é **parar o processamento da sessão inteira**, inclusive dos
eventos que nem vão para webhook. O "remédio" empurrava o problema para um
lugar pior.

**Regra**: ao introduzir bloqueio, semáforo ou fila, escreva explicitamente
QUEM é o chamador em produção e o que acontece com ele quando o mecanismo
satura. Se a resposta for "não sei", não é decisão de desenho — é acidente
esperando.

**Fixe a propriedade num teste** que falhe se ela mudar, como
`TestLimitador_AquisicaoBloqueiaOChamador`. Comentário não impede regressão;
teste impede.

---

## 16. Espera sem prazo transforma defeito em silêncio

`sync.WaitGroup` só oferece `Wait()` infinito. Num teste que persegue "o
mecanismo travou", isso é o defeito silenciando o próprio teste.

`TestPool_PanicoNaoMataOWorker` (F86) esperava com `panicos.Wait()` nu. Ao
rodar o **controle negativo** — recover encerrando o worker, semântica de
`SafeGo` —, os workers morreram, os pânicos restantes nunca rodaram, e o teste
**pendurou por 300s** em vez de acusar. O controle só provou alguma coisa
depois que a espera ganhou prazo:

```
--- FAIL: TestPool_PanicoNaoMataOWorker (10.00s)
    so' 4 de 12 panicos rodaram: os workers morreram e o pool encolheu
```

`4 de 12` é exatamente o número de workers: cada pânico matou um. A mensagem
com progresso é o que separa "falhou" de "falhou por este motivo".

**Regra**: toda espera num teste de concorrência tem prazo e mensagem com
PROGRESSO (`X de Y`), nunca só "timeout". Um `Wait()` nu parece mais simples e
é pior: ele converte a falha que você quer detectar em travamento, que é o
sintoma mais caro de diagnosticar.

**Corolário — o controle negativo audita o teste, não só o código.** Este
controle não achou defeito no `dispatch.go`; achou defeito no
`dispatch_test.go`. Se o controle não produzir uma FALHA COM MENSAGEM em
tempo hábil, o problema pode ser o teste (ver também a entrada 4, sobre
controle que não compila).

---

## 17. A correção mede onde ela ajuda, e quebra onde ninguém olhou

O pool de despacho da F86 foi medido em três harnesses, testado sob `-race`,
teve três controles negativos e passou no `make check`. E tornou um cenário
**estritamente pior**: um webhook morto passou a segurar um worker por 7,5
minutos, e dois pareamentos simultâneos paravam toda a entrega da instalação —
inclusive WebSocket e RabbitMQ, que não tinham relação com o destino quebrado.

O defeito não estava no código novo. Estava na **interação** dele com um
comportamento antigo e inofensivo: o `time.Sleep` do backoff de retry, que
antes rodava numa goroutine solta.

Por que nenhuma das medições pegou: todas comparavam cenários em que o pool
AJUDA (rajada, memória, goroutines). Nenhuma perguntou onde ele cobra.

**Regra**: para todo mecanismo que limita um recurso, meça também o cenário em
que ele vira o problema. A pergunta que faz o cenário aparecer é *qual entrada
faz esta proteção virar o defeito?* — e a resposta costuma ser "algo que segura
o recurso por muito tempo", não "algo que chega em volume".

**Corolário — enumere os detentores.** Ao converter recurso ilimitado em
limitado, liste tudo que passa a disputá-lo e o pior caso de ocupação de cada
um. A conta é aritmética simples e teria pego este caso em minutos:
`5 tentativas × backoff exponencial de base 30s = 450s por evento`.

**Corolário 2 — o conserto do conserto é um mecanismo novo.** Trocar `Sleep`
por `time.AfterFunc` remove a goroutine, e mantém o payload vivo no timer:
sem teto, seria a mesma memória ilimitada com outra roupa. Por isso o conjunto
de pendentes nasceu limitado por bytes.

A política completa está em `CLAUDE.md`, seção "regressão introduzida pela
PRÓPRIA correção".

---

## 18. Instrumento de medida na suíte padrão mede a máquina, não o mecanismo

Os harnesses de carga da F86 rodavam junto com o resto da suíte. Duas
consequências, e a segunda é pior que a primeira:

**Quebrou o `make check`.** `TestMedicaoCalibracaoOrcamento` leva 132s sozinho
e levou **10m26s** dentro da suíte, estourando o orçamento de 20 min do pacote.
E derrubou junto `cmd/logcov`, que `go test` roda em PARALELO: de 195s para
9m49s, morto por inanição de CPU. Um pacote que não tem relação nenhuma com a
mudança falhou por causa dela.

**E invalidou a própria medição.** Contenção de CPU é exatamente a variável sob
medida. Rodando disputando núcleo com outros pacotes, o número que sai descreve
a máquina naquele instante, não o mecanismo. Pior: passou numa execução e
falhou na seguinte sem nenhuma mudança relevante — instrumento intermitente
produzindo número com cara de dado.

**Regra**: harness de medição não roda na suíte padrão. Guarde atrás de uma
variável de ambiente (`WA_API_MEDICAO=1` neste repo) e não atrás de build tag:
com env var ele continua COMPILANDO no `make check` e não apodrece em silêncio
quando a API muda.

**Corolário**: ao adicionar um teste que leva minutos, pergunte o que mais roda
em paralelo com ele. `go test ./...` paraleliza por pacote, então um teste
faminto de CPU faz um pacote vizinho falhar por timeout — e o diagnóstico
aponta para o lugar errado.

---

## 19. Espera cujo relógio mora na página: uma página parada para o próprio timeout

A armadilha nº 16 é sobre espera **sem** prazo. Esta é pior, porque a espera
**tem** prazo — e ele não vale.

O `chromedp.Poll` aceita `WithPollingTimeout`, e ele parece o prazo da operação.
Não é: **o timer é injetado DENTRO da página**. Se o renderer não estiver
executando JavaScript, o timeout nunca dispara, e a espera fica presa
indefinidamente — **fora da `DeadlinePolicy` sem parecer que está**.

**Evidência.** Uma sonda da Fase 6 travou **duas vezes**, 10 min e depois 25 min,
sempre entre os mesmos dois estágios. Nada explicava: `navigateTarget` está sob
prazo de 30 s, `waitAppReady` sob 15 s, e o `Poll` declarava teto de 180 s.
Somando os piores casos dava 4 minutos. O estágio seguinte foi carimbado em
**t=1501s**.

A causa apareceu ao trocar o `Poll` por laço do lado Go com cada sondagem sob
`Runner.Do`: o silêncio de 24 minutos virou **12 erros registrados no `OpLog`**,
e ficou visível que a página havia parado de responder aos ~6 s.

**É prima da nº 3 da Fase 4C** (`chromedp.Poll` que nunca avaliava a condição
porque `requestAnimationFrame` não dispara em aba de fundo). As duas presumem
que a página coopera — uma para agendar a avaliação, outra para agendar o
próprio prazo. Duas bibliotecas independentes já erraram no primeiro; o segundo
é o mesmo erro um nível acima.

**Regra**: o relógio de qualquer espera remota mora **do lado do controlador**.
Prazo implementado na página é dado, não garantia — vale como otimização, nunca
como limite superior.

**Guarda**: toda espera remota passa pelo `Runner.Do`, que aplica prazo por
classe de operação e registra no `OpLog`. Espera que não passa por lá é espera
que, quando travar, não deixa rastro de onde travou.

**Como isto se detecta**: um estágio carimbado muito além da soma dos prazos
declarados. Se o tempo observado não cabe na aritmética das políticas, existe
uma espera fora delas — e o `OpLog` diz qual foi a última operação registrada.

## ARM — o boot de produção classifica UMA vez, e o dublê era rápido demais para revelar

**Data**: 2026-08-18 · **Descoberta**: LOOP 05.1, primeira corrida de N ciclos
contra o perfil pareado real.

**O defeito**: `internal/wa-headless/core/session.go:303` chama `spa.Probe`
**exatamente uma vez**, imediatamente depois de `tab.Navigate` retornar, e falha
o boot se a classe não for `APP_READY`. Não há laço, espera de assentamento nem
nova tentativa.

```
cycle 1/3: core.StartSession failed after 4.708414333s
  core: boot failed at not_ready (stopped_via=browser.close):
  core: page classified "OTHER", want "APP_READY"
  (snapshot url="https://web.whatsapp.com/" dom_nodes=224)
```

Aos 4,7 s a página tinha **224 nós** — instantâneo tirado no meio da montagem.
Não é QR, não é login-required: essas têm classe própria. É o SPA ainda subindo.

**Por que atravessou todas as defesas.** O `core/session_test.go` tem quatro
provas adversariais, todas contra páginas servidas por `httptest`. Um fixture
local **monta instantaneamente**: quando `Navigate` retorna, `#pane-side` já
está lá. O SPA real não — este mesmo repositório mediu que ele leva segundos
além da navegação (`HANDOFF §F2`: recovery p50 **10,4 s**; `EVIDENCIA-SPA.md`
M3/M6/M7). O dublê não reproduzia o **tempo** da produção, então o defeito ficou
invisível.

É a armadilha nº 1 deste catálogo noutra forma: não é que o dublê fosse mais
permissivo na regra — é que ele era **mais rápido**. A dimensão em que ele
diverge da produção foi a dimensão que escondeu o bug.

**E o mais revelador**: `StartSession` é o **único** lugar do módulo que
classifica uma vez e desiste. Todo o resto do `realspa_test.go` amostra em
janela — 45 s no boot não pareado, `sampleReadiness` com orçamento, o laço de
pareamento. O conhecimento existia no repositório e não atravessou para o
código de produção quando ele foi escrito.

**O que NÃO era**: prazo. O orçamento externo do ciclo era de 60 s. A única
sonda disparou em T+4,7 s, dentro do orçamento, e nunca olhou de novo. Aumentar
o timeout não conserta — falta o laço.

**Quem passou por cima**: quatro provas adversariais, a verificação do Chief
(que reproduziu a ablação de `VerifyInventory` com as próprias mãos) e o
`CAP-05_RESTORATION: PASS` da orquestração. Nenhum deles podia pegar: nenhum
exercitava o caminho de produção contra o SPA real.

**A lição de processo, que é o que esta entrada existe para preservar**: um
caminho de produção validado apenas contra dublê está validado contra as
propriedades que o dublê tem. Quando a propriedade que importa é **temporal**,
o dublê precisa ser lento como a produção — ou a validação precisa acontecer
contra a produção. Aqui a evidência do timing já existia, medida, no mesmo
repositório, e ainda assim não impediu o defeito.

**Correção**: não aplicada nesta sessão — está fora do write set do LOOP 05.1 e
mexer em `session.go` é mudança de mecanismo, não de teste. O caminho é dar ao
`StartSession` um laço de assentamento com orçamento, na forma que o
`sampleReadiness` já usa, e travá-lo com um dublê que **demore** para montar.

**BEFORE_FIX, evidência executada e preservada** (2026-08-18, antes de qualquer
correção, conforme a política deste repositório):

O defeito foi reproduzido num teste rápido contra fixture local. O fixture
`lateMountingPage` serve uma página **sem** o marcador de prontidão no primeiro
paint e injeta `#pane-side` no DOM só depois de `lateMountDelay = 6 s` — atraso
escolhido a partir de número **medido**, não de intuição: a falha de campo
disparou em T+4,708 s. O orçamento dado ao `StartSession` é de 20 s, dentro do
teto do `F2` (app-ready observado até 15,8 s). Ou seja: **um boot que apenas
esperasse teria folga de sobra para passar; só um boot que classifica uma vez
falha.**

```
--- FAIL: TestStartSession_LateMountingSPA_SingleProbeFailsBeforePageFinishesMounting
    StartSession gave up at StageNotReady after only 2.15s, before
    lateMountDelay=6s had elapsed and #pane-side was ever mounted — this is the
    defect under test: session.go:303 calls spa.Probe exactly once, right after
    Navigate returns, and never looks again.
```

Reproduzido pelo Chief. Falha em **2,15 s**, muito abaixo do atraso e do
orçamento: não é `context.DeadlineExceeded`. Todos os testes pré-existentes
continuam verdes.

**A armadilha DENTRO da armadilha, e ela é a parte mais instrutiva.** O primeiro
rascunho do teste afirmava `BootFailure` em `StageNotReady` **como sucesso** — e
**passou** contra o código defeituoso. Isso é o inverso do que um teste de
defeito deve fazer: ele passaria hoje e **falharia depois do conserto**,
travando o bug em vez do comportamento. O executor percebeu sozinho e reescreveu
para afirmar `err == nil`.

Vale como regra própria: **um teste de defeito que passa hoje não é teste de
defeito — é a especificação do bug.** A pergunta que separa os dois é "este
teste vira verde ou vermelho quando o conserto chegar?".

**Nota de fidelidade do dublê**: o snapshot local classifica `REDIRECT` e não
`OTHER` como no campo, porque `Classify` verifica `web.whatsapp.com` na URL e o
`httptest` serve de `127.0.0.1`. As duas classes são `!= APP_READY` e entram no
`StageNotReady` pelo mesmo ramo, então o estágio sob teste não muda — mas fica
registrado que o dublê **não** reproduz a classe exata, só o caminho.

**CORRIGIDA em 2026-08-18** (LOOP 05.2, escopo autorizado pela orquestração
depois da reabertura `PASS → SUPERSEDED → FAIL_REAL_SPA`). A semântica temporal
foi para a camada certa: `spa.WaitForReady` — o `core` passa a saber apenas
*"espere a condição de prontidão, com orçamento"*, e não quantos polls, qual
seletor transitório ou como `OTHER` evolui.

**O orçamento foi verificado, não presumido.** O `OpRecoveryProbe` — aquele
orçamento declarado sem call site que este ciclo já tinha encontrado — foi
checado e **não serve**: vale 2 s, e limita UMA ida e volta, não a espera
inteira; com 2 s o laço desistiria no primeiro poll e o defeito voltaria com
outro nome. Entrou `spa.DefaultSettleBudget = 60 s`, documentado como
**orçamento operacional de espera, nunca SLA**, e explicitamente **não**
derivado das três amostras do F2.

**O achado durante a correção, que é a parte que vale.** A primeira versão
marcou `ClassRedirect` como terminal — decisão que parece óbvia: a página
"saiu" do host esperado. O teste do defeito **quebrou na hora**, e a razão é
que `Classify` devolve `REDIRECT` para qualquer página sem marcador cuja URL
não contenha `web.whatsapp.com` — o que é verdade de **todo** fixture local
deste módulo, porque `httptest` nunca serve daquele host. Marcar `REDIRECT`
como terminal **recriaria o defeito original com outro nome**: falha na
primeira sonda, antes de a página montar.

Ou seja: o teste do defeito pegou a reintrodução do próprio defeito, durante o
conserto dele. É a justificativa mais direta possível para a regra de escrever
o teste ANTES.

**Controle de mutação, executado e reproduzido pelo Chief**: voltando ao
`spa.Probe` de tiro único, o teste do defeito falha de novo em ~1,4 s. O
conserto é o que o faz passar.

**Regressões**: ablação do `VerifyInventory`, os dois testes da marca de
suspeita e os de ownership continuam mordendo. Suíte inteira verde.

**Status**: CORRIGIDA e **provada contra o SPA REAL** (atualizado em 2026-08-18,
LOOP 05.10 — as três ressalvas que este parágrafo carregava caducaram e estavam
enganando quem lesse o status).

O que ele dizia e por que já não vale:
- *"o que segue aberto é a prova contra o SPA REAL"* — feita:
  `TestRealSPANCycleLifecycle`, 3/3 ciclos contra o perfil pareado, READY em
  16,5s / 11,9s / 9,9s, identidade `PRESENT` em todos, parada limpa, sem lock,
  sem órfão.
- *"pendente de autorização de escopo"* — concedida e executada; o laço de
  assentamento vive em `spa.WaitForReady` e é chamado por `core.StartSession`.
- *"o teste está na árvore de trabalho, não commitado"* — commitado desde o
  `72512b1`, com a suíte verde.

O parágrafo original ficou registrado acima em vez de apagado: uma armadilha
cujo status envelheceu em silêncio é ela mesma um exemplo do que este catálogo
existe para pegar.

## ARM — a sonda de identidade do teste de N ciclos funde três estados em `false`

**Data**: 2026-08-18 · **Descoberta**: primeira corrida de N ciclos **depois** da
correção do boot, LOOP 05.2.

**O que aconteceu**: com o conserto no lugar, o ciclo 1 alcançou `READY` em
**16,22 s** — o laço de assentamento funcionou, contra os 4,7 s em que o boot
antigo desistia. Mas o teste parou com `identity_present=false` e a mensagem
"absence of QR is not proof of identity".

**E a identidade estava lá.** Rodei em seguida a sonda já existente
(`TestRealSPAOwnerIdentityShape`, só leitura, mesmo perfil, minutos depois):
`OWNER IDENTITY PRESENT: true`, primeira vista em **T+0,02 s**,
`getMaybeMePnUser() -> PRESENT`, `getMaybeMeLidUser() -> PRESENT`. O perfil
está pareado e íntegro.

**O defeito, em `realspa_test.go`**:

```go
var identityPresent bool
if identityErr == nil {
    if json.Unmarshal([]byte(raw), &shape) == nil {
        identityPresent = shape.HasIdentity
    }
}
```

Uma leitura ÚNICA, com `OpStateProbe` (5 s), e `identityPresent` fica `false`
em **três** situações que nada distingue: a sonda **errou**, o parse **falhou**,
ou a identidade está **de fato ausente**. Erro engolido em silêncio vira
"negativo" — e foi lido como negativo.

**É a mesma classe do defeito que este ciclo acabou de consertar.** O boot
classificava uma vez e desistia; esta sonda lê uma vez e desiste. A vizinha
`TestRealSPAOwnerIdentityShape` faz o certo: amostra por 75 s com tique de 2 s,
e é por isso que ela vê o que a outra não viu. **O conhecimento estava no
arquivo, na função ao lado.**

**E é também a lição da F-27 noutra forma**: um instrumento que não distingue
dois estados não pode ser usado para decidir entre eles.

**Agravante de processo, meu**: eu **elogiei** esse controle no RELATÓRIO #6 —
disse que o validador tinha feito certo ao parar. Ele parou, e a decisão de
parar diante de ambiguidade continua certa; o que estava errado era o
instrumento que produziu a ambiguidade. Um controle rigoroso alimentado por
sonda cega para de rodar pelo motivo errado, e eu não olhei o suficiente para
perceber.

**Correção sugerida**: a leitura de identidade deve (a) distinguir erro de
sonda, erro de parse e ausência real, cada um com sua mensagem; e (b) amostrar
em janela como a `identityShapeBudget`/`identityShapeTick` já fazem, em vez de
ler uma vez. Não aplicada aqui: é mudança no instrumento logo depois de ele ter
produzido um resultado, e trocar instrumento e conclusão no mesmo passo é como
se perde a rastreabilidade.

**Status**: **CORRIGIDA em 2026-08-18 (LOOP 05.3)** — esta entrada esteve como
`DESCOBERTA` enquanto o conserto era feito num passo separado, de propósito,
para não misturar o registro do achado com a troca do instrumento.

O conserto: veredito de quatro estados
(`PRESENT` / `ABSENT` / `PROBE_ERROR` / `PARSE_ERROR`), cada um com a sua
mensagem, e amostragem em janela reutilizando
`identityShapeBudget`/`identityShapeTick` em vez de número novo. Na primeira
corrida sob o instrumento corrigido ele imediatamente pagou: devolveu
`PROBE_ERROR: context canceled` por 76 s, revelando a armadilha seguinte deste
catálogo — a sessão morria com o prazo do próprio boot.

Continua verdadeiro, e é o motivo de a entrada existir: o resultado original
*"N ciclos falhou por identidade ausente"* era **FALSO NEGATIVO** e não deve ser
citado como evidência sobre a sessão.

## ARM — o dublê divergiu da produção no TEMPO DE VIDA DO CONTEXTO

**Data**: 2026-08-18 · **Contexto**: LOOP 05.3, depois que a sonda de identidade
corrigida (armadilha anterior) parou de mentir.

**O que a sonda corrigida revelou**: `PROBE_ERROR: context canceled`, pela
janela inteira de 76 s, num boot que tinha alcançado READY em 10,9 s. Não era
identidade ausente — era a sessão **já morta** quando a primeira sonda rodou. O
`identity_present=false` original era isto, e não uma afirmação sobre a conta.

**A causa, em produção**: `engine.OpenTab` (`engine/tab.go:35`) deriva o
alocador do chromedp do contexto que recebe, e `core.StartSession` passava o
contexto do **boot** do chamador. Logo o tempo de vida da sessão ERA o do prazo
de boot. Um chamador que escreve `defer cancel()` — o que todo código Go correto
escreve — matava a sessão no instante em que o boot retornava. E mesmo sem
cancelar, ela se autodestruiria quando o prazo de boot expirasse.

Um chamador que concede 60 s está pedindo um BOOT de 60 s, não uma SESSÃO de
60 s. O wa-api vai segurar a sessão por horas.

**Por que a suíte inteira era cega**: todos os testes de `core/session_test.go`
chamavam `StartSession(context.Background(), ...)` — um contexto que nunca é
cancelado e nunca expira. O acoplamento não tinha como morder.

**A lição, e é a terceira vez nesta sessão**: o dublê não era mais PERMISSIVO
que a produção. Ele divergia numa dimensão em que ninguém estava olhando.
- Primeiro foi a **velocidade** (fixture `httptest` monta instantaneamente, a
  SPA real leva segundos) — escondeu o boot de tiro único.
- Agora é o **tempo de vida do contexto** (`Background()` nunca morre, o prazo
  de boot de um chamador real morre) — escondeu o acoplamento da sessão.

A pergunta que faz essa classe aparecer não é "meu dublê aceita o que a produção
aceita?", é: **em que EIXO o meu dublê é mais bem-comportado que o mundo?**
Tempo, cancelamento, ordem, concorrência, falha parcial. `context.Background()`
num teste é um dublê de um contexto, e um dublê imortal.

**Correção**: `core.StartSession` abre a aba sob um contexto que o pacote
possui, encerrado só pelo `Stop`. O aborto do boot é preservado por uma goroutine
vigia desmontada no retorno — necessária porque `tab.Navigate` roda sob o
contexto da ABA, nunca sob o `ctx`.

**Controles negativos, e os dois primeiros NÃO morderam** — vale registrar
porque é a armadilha 3 deste catálogo acontecendo em tempo real:
1. Reverter o `OpenTab` para o `ctx` **não compilou** (`sessionCtx` sem uso).
   Não prova nada. Refeito com `_ = sessionCtx`: aí sim FALHOU, com a mensagem
   do defeito.
2. Remover o vigia deixou `TestStartSession_CancelledBootStillAborts` **passar**.
   O teste era cerimônia: a página de teste montava rápido, o aborto vinha do
   `WaitForReady` (que recebe `ctx`) e o `Navigate` nunca chegava a importar.
   Reescrito contra um servidor que só responde depois de 30 s — o único ponto
   em que o vigia é o que carrega o cancelamento. Aí o controle mordeu:
   `StartSession took 31.95s, at or beyond the 30s server delay: it waited the
   navigation out instead of aborting on cancellation`.

**Testes que travam**: `TestStartSession_SessionOutlivesItsBootContext`,
`TestStartSession_CancelledBootStillAborts`.

**Status**: CORRIGIDA. Verificada contra o perfil pareado real: 3/3 ciclos,
identidade `PRESENT` em todos (esperas de 104ms, 99ms, 2,9ms), parada limpa,
`SingletonLock=0`, nenhum órfão.

## ARM — o laço com prazo que pode não rodar NENHUMA vez

**Data**: 2026-08-18 · **Contexto**: LOOP 05.9, enquanto a medição de retenção
longa rodava. Defeito **meu**, introduzido hoje, na função que eu havia acabado
de escrever para consertar um instrumento cego.

**Onde**: `internal/wa-headless/realspa_test.go`,
`sampleIdentityUntilPresent`.

**O código**:

```go
start := time.Now()
last := verdictAbsent
for deadline := start.Add(budget); time.Now().Before(deadline); {
    ...sonda...
}
return last, time.Since(start), lastErr
```

**O problema**: se o orçamento já tiver expirado quando a primeira verificação
roda, o corpo **nunca executa** e a função devolve `verdictAbsent` **sem ter
sondado nada**. Os chamadores leem `ABSENT` como afirmação sobre a SESSÃO. É a
armadilha da sonda cega reconstruída num lugar novo — na função escrita para
consertar a sonda cega.

**Como apareceu**: o teste de retenção chama com `budget = 1ms` de propósito,
porque ali a pergunta é "o que é verdade AGORA" e esperar borraria a linha do
tempo que se está medindo. Com 1 ms, a janela entre `start` e a primeira
verificação deixa de ser desprezível.

**Por que quase passou**: com os orçamentos grandes (75 s do N ciclos) o corpo
sempre roda, então nenhum teste existente morde. O defeito só é alcançável pelo
chamador novo — o mesmo padrão de "só aparece quando um consumidor real
aparece" que se repetiu o dia inteiro.

**Correção**: `probed := false` e `!probed || time.Now().Before(deadline)`.
**Pelo menos uma sonda sempre acontece; o orçamento passa a limitar as
RETENTATIVAS**, que é o que ele sempre quis dizer.

**Sobre a medição em curso quando o defeito foi achado**: ela rodava o binário
antigo. Não foi descartada, e o critério é verificável — o defeito só produz
`ABSENT`, e `PRESENT` **só pode vir de uma sonda que rodou**. Toda amostra
`PRESENT` daquela corrida é válida. Um `ABSENT` ali seria **ambíguo** e não
contaria como achado.

**Travada em teste**: `TestSamplingLoopAlwaysProbesAtLeastOnce`. A decisão do
laço foi extraída para `shouldProbeAgain(probed, now, deadline)` — pura, sem
browser — porque `sampleIdentityUntilPresent` recebe `*core.Session` e não havia
como exercitá-la sem Chrome. O teste **não tem portão**: o defeito não tem nada
a ver com o WhatsApp, e pô-lo atrás do portão significaria verificar a
propriedade só nas corridas raras que têm perfil pareado — que foi exatamente
como ele entrou.

**Controle negativo, executado**: voltar a `return now.Before(deadline)` faz
falhar com *"the sampling loop skips its first probe when the budget has already
elapsed; it would return the zero-value verdict (ABSENT) without having probed"*.
Arquivo restaurado byte-idêntico.

**Status**: CORRIGIDA. A lição que fica: um laço com prazo cujo corpo pode
executar zero vezes precisa dizer, no tipo ou na estrutura, o que devolve
quando não executou — senão devolve o zero-value, e zero-value de veredito é
sempre a resposta mais perigosa possível.

## ARM — o teste que reportou sucesso medindo o ESTADO OPOSTO ao que o nome dele diz

**Data**: 2026-08-19 · **Contexto**: pareamento do perfil de laboratório por QR
ao vivo, a pedido do humano. Defeito causado pela ação, não pré-existente.

**Onde**: `internal/wa-headless/realspa_test.go`,
`TestRealSPAUnpairedBootObservation` (e, por tabela,
`TestRealSPACaptureQRCode` e `TestRealSPALiveQR`).

**O que aconteceu**: o perfil de laboratório é descrito no próprio arquivo como
*"descartável e não pareado por construção"*. Depois de ele ser pareado por um
escaneamento real, o teste que observa o boot **NÃO PAREADO** continuou
**PASSANDO** — e o que ele registrou foi:

```
CLASSIFIED AS: APP_READY (has_pane=true has_qr=false)
--- PASS: TestRealSPAUnpairedBootObservation (11.71s)
```

Ou seja: um teste chamado *observação de boot não pareado* observou um boot
**pareado**, gravou no log a lista de seletores de uma tela de conversas como se
fossem os candidatos de uma tela de QR, e reportou sucesso.

**Por que passou**: ele é um teste de OBSERVAÇÃO — registra, quase não afirma.
As únicas asserções eram "a SPA respondeu" e "a página renderizou algo", e as
duas continuam verdadeiras num boot pareado. **O silêncio era o defeito.**

**A generalização, e é a que vale**: "por construção" não é garantia. O perfil de
laboratório é um DIRETÓRIO; qualquer coisa que o pareie — uma demonstração de QR
ao vivo, uma corrida perdida — vira a premissa do avesso sem avisar ninguém. Um
teste cuja conclusão depende de um estado do mundo tem de **checar esse estado**,
não presumi-lo a partir da própria documentação.

**Correção**: os três testes passam a verificar a pré-condição e falhar
**nomeando a causa**, não o sintoma:

- observação: *"the lab profile is PAIRED (classified APP_READY, has_pane=true):
  this test observes what an UNPAIRED boot looks like ... Reset the lab profile"*
- captura de QR: em vez de *"no QR appeared within 60s"* — que manda o leitor
  caçar seletor quebrado — *"the lab profile is already PAIRED: it restores a
  session instead of showing a QR"*
- QR ao vivo: um perfil já pareado alcança `#pane-side` **na primeira amostra**,
  antes de qualquer QR ter sido mostrado; reportar isso como *"o escaneamento
  completou"* creditaria um evento que não foi observado. Agora recusa.

**Status**: CORRIGIDA. Os três falham contra o perfil pareado, com a causa
nomeada, verificado por execução.

**Consequência aberta**: o perfil de laboratório está PAREADO e esses três
testes não rodam até ele ser resetado. Resetar é apagar um diretório com um
vínculo de dispositivo vivo — decisão do humano, não minha.

## ARM — o teste de entrega passou sem entrega nenhuma

**Data**: 2026-08-19 · **Contexto**: verificação do nome do evento da coleção de
mensagens (H24), com um humano enviando a mensagem. Defeito meu, do mesmo dia
em que escrevi o teste.

**O que o teste reportou**:

```
SEND A MESSAGE to the lab account now; watching for 3m0s
DELIVERED after 0s: Meta(jid=<redacted> id=ACB68C... dir=in type=image
                         at=2026-08-19T02:50:41Z)
EVENT NAME "add" IS CONFIRMED for this build
--- PASS
```

**O que de fato aconteceu**: o carimbo do evento era `02:50:41Z` e o relógio
marcava `22:37Z`. A mensagem tinha **19h46m** de idade. Não era a que o humano
acabara de enviar — era uma **imagem antiga**, reposta pela coleção durante o
carregamento da página.

**Por que passou**: `MsgCollection` emite `add` também ao **repor histórico**, e
não só na entrega ao vivo. Eu tinha um "drain de escorva" para descartar a
reposição, mas ela **continua depois** dele. Qualquer evento servia para o
teste, então ele teria passado **mesmo se ninguém enviasse nada**.

**A frase que eu escrevi no log é o agravante**: *"EVENT NAME 'add' IS CONFIRMED
for this build"*. A parte verdadeira é que `add` dispara. A parte falsa é o
contexto — o teste dizia "envie uma mensagem e eu observo", e o que ele observou
não veio dessa ação.

**A classe, e é a terceira variação dela nesta semana**: um teste que **reporta
sucesso medindo outra coisa** que não a que declara. Antes foi um teste de boot
não pareado observando um boot pareado; agora é um teste de entrega observando
histórico. O padrão comum: **a condição de sucesso era ampla demais para
discriminar a hipótese**.

A pergunta que teria evitado: *este teste passaria se a ação que ele pede não
tivesse acontecido?* Se sim, ele não está medindo a ação.

**Correção**: o frescor vira discriminador. Só conta evento carimbado **depois
do início da janela**, com 5 minutos de tolerância para desvio de relógio — o
carimbo vem do servidor, não deste host, e o que a tolerância precisa excluir é
HISTÓRICO, que tem horas ou dias. A reposição passa a ser contada à parte, e a
mensagem de falha distingue três casos que antes eram um: nada chegou; chegou
só reposição (logo `add` dispara, mas entrega ao vivo segue sem prova); ou
chegou evento fresco.

**Status**: CORRIGIDA e **verificada por nova corrida**, com o humano enviando:

```
(events stamped before 2026-08-19T22:33:19Z are HISTORY REPLAY and are ignored)
DELIVERED after 54s: Meta(jid=<redacted> id=3A7D... dir=out type=chat
                          at=2026-08-19T22:39:11Z)
(ignored 58 replayed history event(s) along the way)
```

**Os 58 são a prova de que o discriminador carrega peso.** Sem ele, qualquer um
daqueles 58 teria feito o teste passar — que é literalmente o que aconteceu na
primeira corrida. Um controle negativo raramente vem tão de graça: a própria
execução corrigida mostra quantas oportunidades de falso positivo existiam.


## ARM — o frescor consertou um teste e ARRUINOU o seguinte

A armadilha anterior termina em triunfo: o frescor era o discriminador que
faltava, e os 58 eventos replicados provaram que ele carregava peso. Esta
começa exatamente daí — porque **a lição foi guardada errada**.

Guardei "use frescor". A lição verdadeira era "use o discriminador que responde
à pergunta que você está fazendo".

No teste de laço fechado (conta A envia, conta B recebe), a pergunta mudou. Não
é mais *"isto chegou agora ou é histórico?"* — é *"isto é a MINHA mensagem?"*.
Frescor não responde a essa. E o teste passou assim:

```
SENT and VERIFIED on the sender: send.Result(id=3EB022B6E694D81AD12A5B at=2026-08-20T19:28:44Z ...)
RECEIVED on the other account:   Meta(... id=2A45804B4DB29280B9F9 dir=in type=image at=2026-08-20T19:28:21Z)
```

Id diferente. `type=image` para um envio de texto. E carimbada **23 segundos
antes do próprio envio**. Verde, e provando nada.

**A conta estava viva.** É essa a diferença entre os dois casos: no teste de
entrega, o único tráfego era o histórico, e frescor bastava para isolar o
presente. Numa conta que recebe mensagens de verdade, o presente está cheio de
gente. O controle negativo determinístico mediu quantas: **3 inbound frescas
alheias em 90 segundos**. A asserção antiga aceitaria qualquer uma das três.

**O discriminador certo estava à mão o tempo todo**: uma mensagem do WhatsApp
mantém o MESMO id nos dois lados. O emissor já devolvia esse id, e o teste já o
imprimia na linha de cima — a prova estava no log, não usada.

**E o primeiro controle negativo não valeu.** Anular a comparação de id com
`if false && ...` fez o teste PASSAR: naquela rodada, a primeira inbound fresca
por acaso foi a certa. Um controle cujo resultado depende de sorte não prova que
a asserção morde. Só a mutação determinística — procurar um id que não pode
existir — produziu a falha.

**As duas regras que sobram:**

1. Quando a pergunta do teste mudar, **reavalie o discriminador**. Herdar o da
   tarefa anterior é como este defeito nasceu: o mecanismo estava certo, e
   estava respondendo a outra pergunta.
2. **Controle negativo não-determinístico é controle nenhum.** Se a mutação pode
   passar por sorte, aperte-a até que a falha seja obrigatória.

---

## Funções irmãs do WhatsApp Web NÃO têm assinaturas simétricas

**Medido duas vezes, em módulos diferentes, em 2026-08-21.**

| par | forma |
|---|---|
| `addParticipantsJob` | `({group, participants, isOffline, reason})` — **um objeto** |
| `removeParticipantsJob` | `(group, participants, timestamp, author, reason, groupMetadata, isOffline)` — **sete posicionais** |
| `blockContact` | `({bizOptOutArgs, blockEntryPoint, contact, skipCtwa1pdNbfSignal})` — **um objeto** |
| `unblockContact` | `(contact, blockEntryPoint)` — **dois posicionais** |

Nos dois casos as duas funções são **exportadas do MESMO módulo**, fazem
operações inversas do mesmo produto, e têm formas de chamada incompatíveis.

**A armadilha é a inferência, não o código.** Ler uma e escrever a outra "por
simetria" é o caminho natural, e falha. Na H58 falhou ao vivo, com uma mensagem
(`reading 'toString'`) que não diz nada sobre assinatura.

**Como evitar, e é barato:** `String(mod.fn)` no console dá o corpo da função
minificada, e os nomes dos parâmetros desestruturados sobrevivem à minificação
porque são propriedades. Uma sonda de 10 segundos
(`internal/wa-headless/probe_block_test.go`) leu as duas assinaturas exatas e a
lista de valores válidos de `BlockEntryPoint` de uma vez. É estritamente mais
barato que uma execução ao vivo falhando.

**Vale também para a leitura por regex no bundle**: o grep achou o `throw`
("trying to block a pn contact without a chat") que virou pré-condição
verificada, mas NÃO conseguiu recortar a assinatura do meio do minificado. As
duas técnicas respondem perguntas diferentes — grep para o comportamento
narrado, `toString()` para a forma da chamada.

**Travado por**: `capabilities/block/block_test.go::TestTheTwoCallsHaveDifferentShapes`
e `capabilities/group/participants_test.go::TestTheTwoSiblingsHaveDifferentShapes`
— ambos falham se alguém "arrumar" uma das metades.

### O limite do `toString()`: funções `async` mostram a casca

Medido em 2026-08-21, na mesma sonda que leu as assinaturas acima.

Função transpilada com `asyncToGenerator` devolve o invólucro, não o corpo:

```js
WAWebChatForwardMessage.forwardMessages    → function v(e){return S.apply(this,arguments)}
WAWebChatMuteBridge.sendConversationMute   → function e(e){return s.apply(this,arguments)}
```

Um `e` só não diz se é objeto, id ou modelo — que é exatamente a informação
pela qual a técnica foi adotada.

**Onde ela FUNCIONA**: função síncrona, e aí é exata e barata —

```js
sendMessageEdit(msg, text, options)   // três posicionais, e o corpo mostra
                                      // que recusa via canEditText/canEditCaption
sendStarMsgs(e, t, n) → d(t, n)       // o PRIMEIRO argumento é ignorado
calculateMuteExpiration(hours)        // horas; Infinity vira sentinela
```

O `sendStarMsgs` é o caso que justifica a leitura sozinho: passar três
argumentos supondo que os três importam funcionaria por acidente, e passar dois
falharia sem dizer por quê.

**A regra composta, das três técnicas medidas:**

| pergunta | técnica |
|---|---|
| a chamada tem que forma? | `String(fn)` — **só se for síncrona** |
| o que o app narra sobre o comportamento? | grep no bundle pelo literal |
| e quando é `async`? | grep pelo corpo do gerador interno; o `toString()` não serve |

## Asserção sobre o PREDICADO não trava a GUARDA

**Medido em 2026-08-21, por um controle negativo que PASSOU.**

A capacidade de edição consulta a guarda do próprio app antes de chamar:

```js
const allowed = Cap.canEditText(msg) || Cap.canEditCaption(msg);
if (!allowed) { park({ ok: false, why: 'NOT_EDITABLE' }); return; }
```

O teste afirmava que o script **continha** `Cap.canEditText(msg)`. O controle
negativo trocou `if (!allowed)` por `if (false)` — e o teste **passou**. O
predicado continuava calculado, continuava no texto, e não controlava mais nada.

**Consultar uma guarda e ignorá-la é exatamente o defeito** que o teste existe
para pegar, e é um defeito plausível: nasce de um `return` removido num refactor,
não de má-fé.

**A regra**: quando o que importa é que uma condição **desvie**, a agulha tem de
nomear o desvio (`if (!allowed) { park(`), não a condição. Isto é parente da
armadilha "guarda que casa com a prosa" — nos dois casos a busca acha algo
verdadeiro sobre o texto e falso sobre o comportamento.

**Encontrado uma vez, corrigido em dois lugares**: o mesmo padrão estava no
teste de pré-condição do bloqueio, escrito no mesmo dia. Buraco achado num teste
é buraco nos irmãos escritos junto — vale reler os do dia.

**Travado por**: `capabilities/edit/edit_test.go::TestTheAppsOwnGateIsAskedFirst`
e `capabilities/block/block_test.go::TestTheAppsOwnPreconditionIsCheckedHere`,
ambos com o controle negativo executado registrado no HOUSEKEEP.

## O `await` da página não é a conclusão

**Medido em 2026-08-21, por uma prova ao vivo que falhou.**

```js
await Cmd.sendStarMsgs(chat, [msg], true);
return { after: !!msg.star };     // ← devolve o valor ANTIGO
```

A promessa resolve antes de o modelo virar. A execução ao vivo devolveu
`before=false after=false` contra um `star` que **funcionou** — o mesmo teste
com espera mediu **696 ms** até o campo mudar. Uma leitura imediata falharia
sempre.

**Por que a sonda tinha passado**: ela dormia 800 ms com `setTimeout`. A sonda
mediu a página; a capacidade mediu a promessa. São perguntas diferentes, e a
sonda não avisa qual das duas respondeu.

**O conserto NÃO é dormir na página.** Isso funcionaria e violaria a invariante
6 — a espera viraria uma duração que nada em Go consegue ver, compor ou
comprimir num teste. O modelo vivo é **estacionado** no estado e o Go relê um
booleano dele a cada volta:

```js
park({ stage: 'settling', before, want, msg });   // o MODELO, não o valor
```

E o script de leitura nunca entrega o modelo ao `JSON.stringify` — lê um campo e
monta a resposta.

**Distinga os dois desfechos.** Orçamento esgotado em `settling` é *"a flag não
moveu"*, não *"a página travou"*: têm conserto diferente, e um erro genérico
esconde qual dos dois aconteceu.

**Onde mais isto vale**: qualquer capacidade cujo efeito seja um campo de
modelo. As H50 e H53 já tinham tropeçado nisto sem que o padrão fosse nomeado.

**Travado por**: `capabilities/star/star_test.go::TestTheAwaitIsNotTheCompletion`
(que assere as DUAS camadas — ver abaixo por quê) e
`::TestAFlagThatNeverSettlesIsUnchangedNotATimeout`.

## O controle negativo tem de mutar a camada que o teste observa

**Medido em 2026-08-21, por dois controles negativos que PASSARAM.**

O teste do parágrafo acima assere que o Go continua lendo enquanto a página diz
`settling`. O controle negativo reescreveu o **script da página** para confiar no
`await` — e o teste **passou**, porque o dublê responde `settling` independente
do que o script diga. O defeito medido em produção vivia no script, e o teste
travava só o lado Go.

O segundo caso foi mais simples e igualmente instrutivo: a mutação removeu a
pós-condição do Go, e eu rodei o teste do caminho de **prazo esgotado**. São
ramos diferentes; nenhum teste cobria o ramo mutado.

**As duas regras que saem daí:**

1. **Se o defeito pode viver no script da página, o teste tem de asserir o
   script** — `stage: 'settling'` presente, `after: !!msg.star` ausente. Dublê
   nenhum substitui isso, porque o dublê não executa o script.
2. **Ao rodar um controle negativo, rode o teste do RAMO que você mutou**, não um
   parente. Um controle negativo apontado para o teste errado é um controle
   negativo que mentiu, e ele mente dizendo "está coberto".

Nas duas vezes o buraco só apareceu porque o controle foi executado. Um controle
negativo escrito e não rodado teria deixado ambos passar.

### E o controle negativo tem de APLICAR

**Medido em 2026-08-21**, no quinto controle da capacidade de silenciar.

A mutação procurava, no fonte Go, o texto do script como ele aparece **na
página**:

```
hours === -1 ? Number.POSITIVE_INFINITY : hours
```

No fonte Go aquilo é concatenação — `hours === ` + strconv.Itoa(Always) + ` ? …`
— então a busca não achou nada, o `replace` não trocou nada, e o teste passou
contra o código **intacto**. O relatório teria dito "controle negativo: passou",
que é exatamente a frase que faz um teste fraco parecer forte.

**A regra**: toda mutação de controle negativo tem de **falhar ruidosamente se
não se aplicar**. Em Python, `assert s.count(old) == 1` antes do `replace`; em
`sed`, confira o diff. Uma mutação silenciosamente vazia é indistinguível de um
teste que não morde, e as duas se relatam com a mesma palavra.

Isto fecha o trio de modos de o controle negativo mentir, todos medidos no mesmo
dia:

| modo | sintoma | conserto |
|---|---|---|
| mutou a camada errada | passa | asserir a camada onde o defeito vive |
| apontou para o teste do ramo vizinho | passa | rodar o teste do ramo mutado |
| **não se aplicou** | passa | asserir que a mutação existe antes de mutar |

## `len()` em Go conta bytes; `.length` em JS conta unidades UTF-16

**Medido em 2026-08-21, por uma prova ao vivo que falhou com o efeito CORRETO
já aplicado.**

O teste do rename de grupo pediu um assunto contendo travessão e comparou o
comprimento devolvido pela página com `len()` do Go:

```
the new subject's length is 72 and 74 was asked for
```

O rename **tinha funcionado**. A asserção é que estava errada — e este é o pior
dos dois desfechos, porque uma asserção errada num teste ao vivo gasta uma
execução inteira para não dizer nada, e a próxima pessoa vai depurar a
capacidade.

O travessão custa 3 bytes e 1 unidade. Emoji fora do BMP custa 4 bytes e **2**
unidades, por ser par substituto.

**A parte que faz disto armadilha e não erro de digitação**: a mesma comparação
estava nos testes ao vivo de **editar** e **encaminhar**, e passava — porque
aquelas strings eram ASCII. Latente, não ausente. Um dia alguém põe um emoji no
texto de teste e três testes quebram por um motivo que não tem nada a ver com o
que eles medem.

**A regra**: qualquer comprimento que atravesse a fronteira Go↔página se compara
em unidades UTF-16. O ajudante é `utf16Len` (`internal/wa-headless/utf16len_test.go`),
e ele tem teste próprio com casos que DIFEREM de `len()` — para que não vire
sinônimo de `len()` num refactor.

**Vale para toda fronteira com JavaScript**, não só esta: índices de `slice`,
posições de menção, limites de truncamento.

## Nem toda mudança é observável pela sessão que a fez

**Medido em 2026-08-21, ao custo de três diagnósticos errados e de um grupo de
laboratório quebrado por uma tarde.**

Mudança de participante de grupo **chega ao servidor** e é invisível para a
sessão que a fez:

| sinal | durante 90 s | verdade (sessão nova) |
|---|---|---|
| `chat.groupMetadata.participants` | contagem antiga | mudada |
| `GroupMetadataCollection.get()` | idem — **mesmo objeto** | idem |
| `MsgCollection` (`gp2/add`, `gp2/remove`) | **nada** | — |

Um `gp2/subject` de rename chega na mesma coleção, então o canal funciona: é
esta notificação que não é entregue aqui.

**A pós-condição mentiu na direção pior** — disse "nada mudou" quando tudo tinha
mudado. Uma pós-condição que erra assim é mais perigosa que nenhuma: ela faz o
chamador desfazer, repetir, ou "consertar" o que já estava certo.

**Duas regras:**

1. **Antes de escrever pós-condição, prove que o sinal MEXE.** Uma leitura que
   devolve o mesmo valor antes e depois pode significar "não aconteceu" ou "não
   dá para ver daqui", e as duas exigem código diferente. O teste é barato:
   provoque a mudança, reinicie a sessão, compare.
2. **Quando não dá para ver daqui, diga isso no TIPO.** `Verified: false` é
   informação; um erro genérico é ruído; um sucesso silencioso é dano.

**E o corolário perigoso**: se a pré-condição lê o mesmo sinal obsoleto, uma
sequência *mudar → desfazer* na mesma sessão vira *mudar → no-op*, e a
restauração reporta sucesso sem restaurar. Passos que se desfazem precisam de
sessões separadas.

### E confirme que o chamador chama a função que VOCÊ vai chamar

**Medido em 2026-08-21, na H69, ao custo de duas execuções ao vivo.**

A técnica "leia o chamador do app" resolveu encaminhar, silenciar e favoritar.
Na enquete ela falhou, e não por culpa dela:

```js
const r = b({correctOptionKey: P, filteredOptions: n, …});
sendPollCreation({poll: r, chat: i, …});
```

Eu tomei a lista de argumentos de `b` como sendo a de
`createPollCreationMsgData`. **`b` é um ajudante local da UI.** O dado de
mensagem que a função da API produz carrega `correctOptionIndex`; o objeto que
copiei diz `correctOptionKey`. Nomes diferentes são funções diferentes.

**O passo que faltava**: ao ler um chamador, confirme que a chamada é para a
função que você vai chamar — pelo nome do módulo no `o("...")`, não pela
proximidade no código. Na mesma linha havia DUAS chamadas: uma para um helper
local (`b`) e uma para a API (`sendPollCreation`). A segunda era boa; a primeira
não era o que eu pensava.

Sintoma típico: `reading '<campo>' of undefined` num campo que existe no objeto
que você montou. Se o campo existe e mesmo assim está indefinido lá dentro, a
função que recebeu não é a que você leu.

## O portão do HOUSEKEEP casa `Status` no meio de um identificador

**Medido em 2026-08-21, na H70.**

`TestHousekeepEntriesAreMachineReadable` procura linhas de status com

```
\*{0,2}Status[^*:]*\*{0,2}:[\s*·—–-]*([^\n*]{3,60})
```

A palavra `Status` aparece dentro de `receiveTextStatusEnabled`, e `[^*:]*`
**atravessa quebras de linha** — então o casamento correu prosa abaixo até o
primeiro `:` (dentro de um bloco de código, três parágrafos adiante) e reportou
como "status" um trecho de saída de teste.

**O sintoma** é o portão reclamando de um "status" que você não escreveu, citando
texto que não é status nenhum.

**O conserto barato**: envolver o identificador em ênfase — `**`identificador`**`
— porque o `*` de fechamento cai dentro do alcance de `[^*:]*` e interrompe o
casamento antes do dois-pontos distante.

**O conserto certo**, se reaparecer: prender o padrão ao início de linha e
proibir quebra de linha no meio. Não foi feito agora porque mexer no portão no
mesmo commit em que ele acusa é como se perde a confiança nele.

**Reapareceu no mesmo dia**, na H66, por `getTextStatus`/`setMyTextStatus` numa
frase seguida de dois-pontos três linhas adiante. Duas ocorrências em horas
mudam o cálculo: o conserto barato virou imposto recorrente sobre quem escrever
"Status" numa entrada.

### CORRIGIDO em 2026-08-21, em commit isolado

```
(?m)^[ \t>*·-]*\*{0,2}Status[^*:\n]*\*{0,2}:[\s*·—–-]*([^\n*]{3,60})
```

Duas mudanças, e ambas foram pagas:

- **`(?m)^…`** — a linha de status começa uma linha. A classe à frente aceita
  indentação, citação e marcador de lista, que é como as linhas reais aparecem.
- **`[^*:\n]*`** — o rótulo não pode correr para outra linha atrás do
  dois-pontos. Era daí que vinha o absurdo.

Os remendos de ênfase foram **removidos**, e há teste que falha se voltarem —
senão o próximo leitor os copia como convenção.

**Os negativos do teste são os casos MEDIDOS**, não inventados:
`receiveTextStatusEnabled` e `getTextStatus` com dois-pontos a parágrafos de
distância. O controle negativo que reverte a âncora reproduz exatamente as duas
capturas que o portão reportou no dia — `"contacts.About(len=0)"` e ` ``` `.

E o teste usa o **mesmo** `housekeepStatusLine` que o portão, não uma cópia:
copiar o padrão faria um dublê incapaz de divergir do original por construção,
que é a primeira armadilha deste arquivo.
