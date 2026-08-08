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
