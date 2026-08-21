# Armadilhas deste projeto

Catálogo de defeitos que **já passaram por revisão e por testes verdes** neste
repositório. Não é teoria: cada entrada aconteceu, tem a evidência medida e o
que a expôs.

Leia antes de mexer em identidade (LID/PN), rotas HTTP, junção de dados ou
qualquer coisa que grave no banco. O padrão comum de quase todas é o mesmo:
**o teste concordava com o defeito**.

---

## 1. Dublê DIVERGENTE da produção — esconde defeito, ou abençoa código morto

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

### O título dizia "mais permissivo", e isso deixava metade da armadilha de fora

Corrigido em 2026-08-20, depois de eu cair na metade que não estava escrita.

Um dublê **mais permissivo** aceita o que a produção recusa, e o efeito é
esconder um defeito. Foram os três casos da tabela acima, e é a forma que a
formulação antiga cobria.

Mas um dublê pode divergir por ser **mais SIMPLES**, e aí o efeito é outro e
pior de detetar: ele **abençoa código morto** — cria confiança em código que a
produção nunca executa.

**O caso** (HOUSEKEEP F188). O nosso `/chat/send/list` embrulha a lista em
`DocumentWithCaptionMessage`, então escrevi um ajudante para a desembrulhar na
recepção, e um teste que montava o evento **à mão**, atribuindo `Message`
diretamente. Só que a produção chama `events.Message.UnwrapRaw()`, que **já
desembrulhou** aquele invólucro antes de o evento chegar ao classificador. O
ajudante nunca alcançava a sua segunda linha.

O que passou verde com código morto no meio:

- o teste;
- o **controlo negativo** — que mordeu, mas num caminho imaginário;
- o `make check`;
- e a **verificação em campo**, porque a lista gravou de facto: pelo ramo do
  topo, não pelo que eu tinha escrito para ela.

**Por que escapa**: a formulação "mais permissivo" faz procurar um dublê que
aceita DEMAIS. Este aceitava de menos — construía um objeto mais pobre que o
real. Ninguém à procura de permissividade olha para lá.

**Regra acrescentada**: quando o objeto do teste passa por uma TRANSFORMAÇÃO no
caminho de produção — desembrulho, normalização, decoração, hidratação —, o
dublê tem de passar pela mesma transformação. Construir o objeto no estado final
que se imagina é adivinhar; construí-lo no estado inicial e deixar a produção
transformá-lo é medir.

Para `events.Message` neste repositório, isso significa: monta-se em
`RawMessage` e chama-se `UnwrapRaw()`. **Nunca** se atribui `Message`
diretamente.

**Como pegar esta metade**: pergunte, de cada ramo que o teste exercita, *quantas
vezes a produção passa aqui?* Se a resposta honesta for "não sei", o dublê não
está a imitar o caminho — está a imitar o destino.

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
  sufixo** (`90937376170214`, não `90937376170214@lid`). Com o sufixo
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

## 19. Você testa o caminho que escreveu, não o caminho que o usuário percorre

O D2 (posse de sessão) foi instrumentado no `connectOnStartup`, e os testes
seguiram o código: 9 unitários sob `-race`, 6 de integração contra Postgres
real, 3 controles negativos, `make check` verde.

O caminho que o usuário percorre é outro. `/session/connect` — que o painel e
todo pareamento novo usam — iniciava sessão **sem reivindicar posse nenhuma**.
Medido com duas sessões vivas:

```
teste-d2    connected=1  posse: MacBook-Pro-de-Lucas.local-93206
teste-d2-b  connected=1  posse: SEM LEASE          <- pareada pelo painel
```

Sob N réplicas, isso é o desastre que o mecanismo existe para impedir: a
réplica seguinte encontra o lease livre, toma, conecta a mesma sessão, e o
WhatsApp mata uma das duas para sempre.

**Regra**: depois de instrumentar um caminho, liste TODOS os pontos de entrada
daquela operação e verifique cada um. Aqui eram dois — arranque e API — e o
segundo é o que 100% dos usuários usam. "Passou nos testes" mede o código
escrito; só o exercício do fluxo real mede o produto.

**Como pegar barato**: exercite pela interface que o usuário usa, não pela
função que você acabou de editar. Uma requisição pelo painel achou em segundos
o que 18 testes verdes não acharam.

---

## 20. O conserto do conserto é um mecanismo novo — e a regra existir não basta

A política anti-regressão deste repositório (`CLAUDE.md`) tem uma regra 4
explícita: *o conserto do conserto também é um mecanismo; aplique as três
regras anteriores a ele*.

Escrevi essa regra pela manhã, ao corrigir a F88. Algumas horas depois,
corrigindo a posse de sessão, criei a F96 — reivindicar a posse ANTES de
materializar a sessão (decisão correta, evita registries sujos) sem liberar
quando a sessão não sobe. Resultado medido:

```
t=6s   teste-runtime | connected=0 | expira_em=12s
t=12s  teste-runtime | connected=0 | expira_em=11s
t=18s  teste-runtime | connected=0 | expira_em=15s   <- renovou
```

Posse viva, renovada a cada 5s, para uma sessão que nunca existiu — e nenhuma
outra réplica poderia assumir aquele usuário, jamais.

**Regra**: a política não se aplica sozinha. Ao terminar QUALQUER correção,
releia a lista de regras antes de considerar o trabalho pronto — não durante a
revisão, que é tarde. A pergunta operacional é: *o que este conserto passou a
segurar, e quem devolve?*

**Corolário — correções de recurso vêm em pares.** Quem adquire precisa de quem
libera, e os dois caminhos precisam de teste em direções OPOSTAS: não liberar
reintroduz o vazamento; liberar demais entrega o recurso em uso. Os dois
controles negativos desta correção falharam em mensagens distintas, e é isso
que prova que o par está correto.

---

## 21. O shell também é um instrumento, e ele mente diferente em cada casca

Três vezes numa sessão, uma peculiaridade de shell produziu resultado que
parecia dado:

| o que eu escrevi | o que aconteceu | o que quase concluí |
|---|---|---|
| `git grep -E '\bfoo\b'` | `\b` não é suportado ali | "não existe no repo" |
| `timeout 120 go test ...` | `timeout` não existe no macOS | "o teste ainda pendura" |
| `env $E cmd` com `E="A=1 B=2"` | **zsh não faz word-splitting**: virou UM argumento | "as réplicas se comportaram assim" |

O terceiro é o pior: as duas réplicas subiram sem NENHUMA variável de banco,
caíram em SQLite e uma rodou em modo `single` — e o experimento produziu uma
tabela de resultados plausível sobre um cenário que não existiu.

**Regra**: todo experimento precisa de um CONTROLE que prove que o cenário foi
montado, antes de olhar qualquer resultado. No caso das réplicas, o controle é
uma linha:

```
A: sqlite_fallback=0 | ownership=1
B: sqlite_fallback=0 | ownership=1
```

Sem ele, não há como distinguir "o mecanismo se comportou assim" de "o
mecanismo nem foi exercitado".

**Corolário**: em zsh, use atribuições explícitas antes do comando
(`A=1 B=2 cmd`) em vez de expandir uma variável com vários pares. E desconfie
de saída vazia: pode ser `command not found` engolido por um `grep` na
sequência.

---

## 22. `git checkout <arquivo>` desfaz o controle negativo — e a correção junto

O controle negativo tem uma forma fixa: quebre a correção de propósito,
confirme que o teste falha, restaure. É o terceiro passo que morde.

Hoje, ao validar a F98, mutei a correção com um script e restaurei com:

```sh
git checkout pkg/application/session/orchestrator.go
```

O teste falhou como esperado, o controle "passou", e eu segui adiante. Só que
`git checkout` restaura o arquivo para o **HEAD**, não para o estado anterior à
mutação — e a correção ainda não estava commitada. O comando apagou a mutação e
a correção no mesmo gesto.

O que torna isso perigoso não é o erro, é o **silêncio** dele. O build seguiu
verde: a correção removida não quebra compilação, e os testes que a cobrem só
seriam rodados de novo mais tarde. Se eu tivesse commitado logo em seguida, o
commit teria a mensagem, os testes, o registro em `HOUSEKEEP.md` — e **não teria
a correção**. Um commit que documenta em detalhe um conserto que não existe é
pior que nenhum commit, porque ninguém volta a olhar.

**Regra**: para restaurar depois de um controle negativo, use uma cópia feita
ANTES da mutação, nunca o git:

```sh
cp arquivo.go /tmp/arquivo.go.bak     # antes
...muta, roda o teste, confirma a falha...
cp /tmp/arquivo.go.bak arquivo.go     # depois
```

`git checkout`, `git stash`, `git restore` — todos falam com o HEAD, e o HEAD
não sabe nada de trabalho não commitado. Só sirvam para restaurar o que já está
commitado.

**Verificação barata que fecha o buraco**: depois de restaurar, confirme que a
correção voltou, com um `grep` na linha que a implementa — não com o build.

```sh
grep -c "releaseOwnership(userID)" pkg/application/session/orchestrator.go
```

É a mesma lição da armadilha 4 por outro ângulo: o controle negativo é código
também, e o passo de restaurar precisa da mesma desconfiança que o passo de
mutar.

**Reincidência, no mesmo dia, duas horas depois de escrever isto acima.** Ao
validar o D6 rodei `git checkout pkg/bootstrap/lease.go` para desfazer a
segunda mutação, e apaguei o `HeartbeatStalled` que ainda não estava commitado.
O que mudou foi só o desfecho: eu tinha posto o `grep` de verificação logo
depois do restore, ele respondeu `0`, e o buraco se fechou em segundos em vez
de virar um commit mentiroso.

Vale registrar porque é a armadilha 20 acontecendo com a própria armadilha 22:
**escrever a regra não impede o erro — o passo de verificação, sim.** Uma regra
é uma intenção; um comando que roda é um mecanismo. Só o segundo funciona
quando você está no meio de outra coisa.

---

## 23. Recomendação sobre risco também é afirmação — e ninguém a revisa

Ao listar os achados pendentes, recomendei ao usuário fazer a etapa 1 da F97
("parar de gravar o token em texto claro") **primeiro, porque é segura e não
quebra cliente**. Ele aprovou com base nisso.

Era falso. A etapa 1 abre acesso sem credencial: a consulta de autenticação
casa por `token = $1`, uma requisição sem token produz `$1 = ""`, e branquear a
coluna faz qualquer requisição anônima autenticar como aquele usuário. Medido
em dois comandos, e o controle negativo devolve **200 OK** para uma requisição
sem token nenhum.

De onde veio a recomendação errada: eu repeti o que a entrada da F97 afirmava.
A entrada dizia "a autenticação já aceita o hash", o que é verdade e é
irrelevante — o problema não é o hash não funcionar, é o texto claro continuar
sendo um caminho de casamento. **Eu não abri `auth.go` antes de recomendar.**

O que torna esse erro pior que um commit errado:

| | commit errado | conselho errado |
|---|---|---|
| passa por revisão | sim (diff, testes, gate) | não |
| deixa rastro | sim (histórico) | quase nenhum |
| quem paga | quem revisa | quem decidiu confiando |

Um plano aprovado vira escopo, e escopo vira trabalho executado sem
reavaliação. A recomendação é o ponto de MAIOR alavancagem e o de MENOR
escrutínio.

**Regra**: antes de classificar uma mudança como "segura", "pequena" ou "não
quebra cliente", leia o caminho que ela toca — não o que a documentação diz
sobre ele. Se o custo de ler for alto, diga que não verificou, com essas
palavras: *"a entrada afirma X; não confirmei no código"*. Incerteza declarada
é utilizável; confiança emprestada de um documento antigo, não.

**Corolário**: entrada de `HOUSEKEEP.md` é hipótese datada, não fato corrente.
A F97 foi escrita horas antes e já estava errada sobre a própria consequência.
Quando ela vira plano, o texto precisa ser reconferido contra o código —
exatamente como se veio de outra pessoa. É o mesmo princípio da armadilha 20 (o
conserto do conserto é um mecanismo novo) aplicado a documento em vez de código.

---

## 24. Teste unitário recebe a dependência pronta; produção a monta depois

Os dez testes do D6 passavam. A sonda estava certa, o relatório estava certo, os
controles negativos falhavam nas duas direções. Subi na bancada em modo `multi`
e o corpo veio assim:

```json
{"status":"ready","checks":{"database":"ok"}}
```

Sem `session_ownership`. A checagem não falhou — ela **não existia**. E o pior:
o relatório ficou idêntico ao de um processo `single` saudável, que é
exatamente o formato esperado quando não há posse a coordenar. Nada acusaria.

A causa é ordem de montagem:

```go
s.routes()               // main.go:409 — monta o roteador, e com ele a sonda
setupSessionOwnership(s) // main.go:413 — instala s.Leases QUATRO LINHAS DEPOIS
```

A sonda recebia `*leaseManager` por valor, então capturava `nil` para sempre.

**Por que nenhum teste pegou**: todos entregam o manager já construído —
`buildReadinessProbe(fakePinger{}, manager)`. Um teste que recebe a dependência
pronta testa o COMPORTAMENTO da unidade e nunca a ORDEM em que ela é montada.
São duas propriedades diferentes, e a segunda só aparece no processo inteiro.

**Regra**: quando um componente lê uma dependência que é instalada em outro
ponto do arranque, o teste tem de exercitar a JANELA — construir com a
dependência ausente, instalá-la depois, e verificar que o componente passou a
enxergá-la:

```go
var installed *leaseManager
probe := buildReadinessProbe(fakePinger{}, func() *leaseManager { return installed })
// ... afirma que a checagem NAO aparece ...
installed = newLeaseManager(...)   // setupSessionOwnership roda aqui
// ... afirma que a checagem PASSOU a aparecer ...
```

**Corolário de projeto**: prefira getter a valor sempre que a ordem de
inicialização não for obviamente garantida. `func() *T` custa uma indireção e
transfere a resolução para o instante em que a resposta é conhecível. Valor
capturado cedo é uma decisão tomada antes de haver informação.

**E o sinal de alerta**: desconfie de saída que parece SAUDÁVEL por acidente. Um
relatório sem a checagem é indistinguível de um relatório onde a checagem não se
aplica — a ausência não grita. Foi por isso que só a bancada pegou, e só porque
eu conhecia o modo em que o processo estava rodando.

---

## 25. Controle negativo que NÃO falha é defeito no teste, não prova do código

O controle negativo tem uma leitura óbvia — "quebrei a correção e o teste
acusou, logo o teste presta" — e uma leitura que quase ninguém faz: **e quando
eu quebro a correção e o teste continua verde?**

A tentação é anotar "esse caminho não estava coberto" e seguir. Está errado. O
teste EXISTE e tem um nome que afirma cobrir aquilo. Verde com o mecanismo
removido significa que ele passa por outro motivo — e a partir dali ele protege
nada enquanto anuncia que protege.

Aconteceu hoje, na fiação do outbox. `TestOutboxWiring_VarreduraReivindicaOVencido`
dizia, em comentário meu: *"Sem cliente HTTP provisionado, tentarWebhook desiste
e liquida a linha"*. Removi a liquidação daquele ramo e o teste passou. O log
explicou:

```
[warn]  falha ao reler a chave HMAC do usuario para uma entrega pendente
[error] nao foi possivel recuperar a chave HMAC; descartando
```

O usuário de teste nunca fora inserido em `users`. A entrada era descartada por
falta de chave **antes de chegar ao caminho de entrega**. O teste percorria um
caminho e o comentário descrevia outro — armadilha 19, com a diferença de que
aqui ele passava *por acidente*, não por omissão.

O conserto não foi ajustar a asserção: foi dividir em dois testes que percorrem
cada um o seu caminho, e inserir o usuário no que precisa dele. Os dois
comportamentos são reais e merecem cobertura; o que não podia continuar era um
teste afirmando cobrir os dois.

**Regra**: mutação aplicada + teste verde = investigar o TESTE, imediatamente.
Não anotar como lacuna, não seguir em frente. E confirme sempre que a mutação
foi de fato aplicada antes de interpretar o resultado — um `assert` que aborta o
script deixa o teste rodar sem mutação nenhuma, e o verde parece um controle que
passou. Foi o que aconteceu na primeira tentativa desta mesma verificação: a
âncora ocorria três vezes, o script morreu, e o `ok` que sobrou não significava
nada.

**Corolário**: prefira controles que falhem por um MOTIVO específico e legível
na mensagem. "quisera falhar" não distingue "o mecanismo sumiu" de "o teste
nunca chegou lá".

**Segunda ocorrência, na F87, com outra causa — e por isso vale registrar.** O
teste media o tempo que o produtor gastava, mas o cronômetro começava DEPOIS de
enfileirar o evento lento:

```go
enqueueSessionEvent(userID, func() { time.Sleep(lento) })  // custo acontece aqui
inicio := time.Now()                                       // ...e o relogio comeca aqui
```

Com a fila, o enfileiramento é instantâneo. Sem a fila, ele bloqueia por dois
segundos — mas ANTES do `time.Now()`. Os dois mundos produziam o mesmo número, e
o controle negativo passava. Mover uma linha para cima fez o controle acusar
`2.002241625s`.

A forma geral: **um teste que mede DEPOIS do efeito não mede o efeito.** Vale
para tempo, para contador e para estado — e é irmão do erro de ler
`users.connected` antes do logout (F93), onde a medição estava certa e o
INSTANTE estava errado.

## 13. Teste verde prova que o código está certo, não que está A CORRER

Em 2026-08-21 a correção do QR (F192) esteve horas com o rótulo "corrigida,
oito controlos negativos executados, `make check` verde" enquanto o utilizador
via o defeito intacto. Nada estava errado no diagnóstico nem no código: o
processo que servia o `:8099` tinha arrancado **12 horas antes** do conserto.

```
processo a servir :8099   arrancado  Aug 20 21:32
orchestrator.go (o fix)   modificado Aug 21 09:51
```

O worker verificou em campo — e a verificação era verdadeira —, mas numa
instância que ele próprio levantou. Entre "os testes passam" e "o utilizador
vê funcionar" existe um passo, compilar e reiniciar, que **não tem gate neste
repositório**.

Duas regras que saem daqui:

1. **Antes de dizer que um comportamento observável está corrigido, confirme
   qual binário está a servir.** `ps -o lstart= -p $(lsof -ti:PORTA)` responde
   em um comando. Se a hora de arranque for anterior à do ficheiro que
   corrigiu, você está a olhar para código antigo.

2. **"Verificado em campo" sem dizer EM QUE INSTÂNCIA não é verificação.**
   Registe porta, hora de arranque do binário e o commit. A F192 registou as
   medições do defeito com hora e porta, mas a prova do conserto ficou só no
   transcript do worker e desapareceu com o worktree — restou uma afirmação sem
   evidência recuperável.

Corolário para quem despacha workers: o worker corrige e valida no worktree
DELE. Implantar no ambiente partilhado é trabalho de quem integra, e tem de
estar dito no packet ou feito na integração — senão não é feito por ninguém.
