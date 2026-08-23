# Estado da iniciativa `wa-headless`

Consolidação de 2026-08-19, pedida pela orquestração depois de a matriz de
paridade fechar. É um retrato: o que existe, o que está PROVADO, o que está
aberto e por quê, e o que depende de decisão humana.

Números vieram do repositório, não de memória. **Atualizados em 2026-08-22 (fim
do dia)**, porque um documento de estado que envelhece em silêncio vira citação
errada — que é o defeito registrado no H29.

> **E este documento cometeu o próprio defeito que documenta.** Entre 20/08 e
> 21/08 ele ficou parado dizendo 263 testes e 12 pacotes enquanto o repositório
> ia para 843 e 35. Foi descoberto na reinicialização do protocolo ORCA de 21/08,
> que compara estado documentado com estado real — e ficou três ciclos em
> triagem, porque corrigir de lado é como se expande escopo em silêncio. Está
> registrado aqui em vez de apagado: um retrato que envelheceu é evidência de
> como ele envelhece.

| | 19/08 | 20/08 | 21/08 | **22/08 (agora)** |
|---|---:|---:|---:|---:|
| testes | 254 | 263 | 843 | **1043** |
| pacotes | 12 | 12 | 35 | **42** |
| capacidades | 6 | 6 | 28 | **35** |
| achados no HOUSEKEEP | 29 | 33 | 105 | **145** |
| commits tocando o módulo | — | — | 202 | **338** (136 só em 22/08) |

### O placar da paridade, que é o número que importa

| estado | 21/08 | **22/08** |
|---|---:|---:|
| `PROVEN` | 41 (19%) | **106 (48%)** |
| `PARTIAL` | 52 | 56 |
| `BLOCKED` | 3 | **49** |
| `INTENTIONAL_DIFFERENCE` | 2 | 6 |
| `MISSING` | **139 (63%)** | **3 (1%)** |

**O salto de `BLOCKED` não é derrota: é o oposto.** Em 21/08 a maioria das
linhas dizia `MISSING` — "não atacado" —, que é honesto enquanto ninguém olhou
e vira mentira depois. Hoje cada linha aberta diz POR QUE está aberta, com
medição. `MISSING` só sobrevive onde o caminho existe e não foi percorrido:
três linhas.

O salto não é inflação de contagem: entre 20/08 e 21/08 a iniciativa mudou de
alvo. Deixou de ser "as seis capacidades do piso" e passou a ser o **contrato de
220 itens** do `LEDGER-WWEBJS.md`, com barramento de eventos, ciclo de vida de
sessão e famílias inteiras novas (pedidos de grupo, agenda, chamadas, enquetes,
status, canais, comércio).

## Placar de paridade — a métrica que vale hoje

Fonte: `LEDGER-WWEBJS.md`, travado por quatro gates em `gate_ledger_test.go`.

| estado | itens | fração |
|---|---:|---:|
| `PROVEN` | 52 | 24% |
| `PARTIAL` | 46 | 21% |
| `BLOCKED` | 3 | 1% |
| `INTENTIONAL_DIFFERENCE` | 2 | 0% |
| `MISSING` | 117 | 53% |
| **total** | **220** | |

A Fase 1 fecha em **0 MISSING e 0 PARTIAL**. `INTENTIONAL_DIFFERENCE` não pode
ser usado para encolher escopo.

---

## 1. O que o módulo faz hoje

| camada | pacote | responsabilidade |
|---|---|---|
| driver | `engine/` | único que importa `chromedp`; lançar, parar, abas, prazos, perfil |
| página | `spa/` | classificar, inventário de módulos, liveness, assentamento |
| sessão | `core/` | `StartSession` → READY verificado, ou `*BootFailure` classificado |
| retenção | `runtime/` | `Holder` — uma sessão entre comandos, posse, pid |
| capacidades | `capabilities/` | as seis da paridade |
| observabilidade | `observability/` | `OpLog`, com política de redação |

## 2. Paridade com o `whatsapp-web.js`

> **O alvo declarado (2026-08-20)**: implementar neste projeto as
> funcionalidades do `whatsapp-web.js`, fazendo **engenharia reversa de JS para
> Go sobre a SPA**. As seis capacidades abaixo são o piso dessa paridade — o que
> o `wa-noise` não cobre pelo protocolo — e não o teto.
>
> A H34/H35 mostrou como cada método portado tende a custar: o wwebjs dá o NOME
> do módulo e a ORDEM das chamadas, mas nem os nomes nem o comportamento
> sobrevivem intactos a este build. Quatro módulos da lista dele não existem
> aqui; `queryWidExists` existe mas ele a chama no lugar errado; `_serialized`
> da mensagem é `NULL`. **Portar é medir, não traduzir.**

Seis capacidades, não catorze: as outras oito o `wa-noise` já serve pelo
protocolo (`PARIDADE-WWEBJS.md` §2–3). Todas com prova contra a SPA real.

| capacidade | onde | prova real |
|---|---|---|
| `livenessCheck` | `capabilities/liveness` | pior latência **1 ms** em 5 amostras |
| `getBrowserPid` | `runtime.Holder.BrowserPID` | recusa o pid morto (`SIGKILL` de fora) |
| `refreshOwner` | `capabilities/owner` | PN e LID presentes, `display_name` NULL |
| `onMessageMeta` | `capabilities/messagemeta` | entrega ao vivo em 54 s, 58 reposições ignoradas |
| `fetchMessages` | `capabilities/fetchmessages` | 340 carregados, filtro por chat casa |
| `backupNow` | `capabilities/backup` | restaura com identidade presente; custo medido nos dois perfis (§6.6) |
| `sendText` | `capabilities/send` | **laço fechado real**: conta A envia, conta B recebe o MESMO id em 1 s |
| `listContacts` | `capabilities/contacts` | roster ao vivo **944 linhas → 544 pessoas**, 398 fundidas |
| `fetchContactAvatar` | `capabilities/avatar` | 12 contatos ao vivo: 4 com foto, 8 sem, **0 falhas** |
| `onContact` | `capabilities/contacts` (`subscribe.go`) | 6 buscas de avatar → **7 eventos `change`**, 0 `add` |
| `primeContactRoster` | `capabilities/contacts` (`prime.go`) | refresh de **42 s**, com pós-condição falsificável (`ran=true`); liga `lid→phone`, não acrescenta pessoa |

### Divergências CONSCIENTES do `wwebjs` (§6 da paridade)

1. **PN e LID separados** — eles colapsam em `pn || lid` e perdem qual respondeu.
2. **`display_name` UNVERIFIED** — o getter existe e responde `NULL` neste build.
3. **`msg.id._serialized` é NULL aqui** — eles dependem dele; nós expomos as partes.
4. **`fetchMessages` lê o CARREGADO**, não pede ao servidor. Declarado, com
   `Loaded` como denominador e `Truncated()`.
5. **`sendText` resolve a identidade ANTES de abrir o chat** — o wwebjs só chama
   `queryWidExists` no `getNumberId`, e por isso as issues dele (#3834, #5750)
   morrem em `No LID for user`. Neste build, 397 de 399 mensagens vivem sob
   `@lid`: resolver primeiro não é otimização, é a condição de funcionar (H34).
6. **`sendText` VERIFICA a pós-condição** — o wwebjs devolve assim que a página
   aceita a chamada. Aqui o envio só retorna depois de a mensagem aparecer na
   coleção, contra a identidade RESOLVIDA. É a invariante 14, e foi ela que
   expôs o defeito da verificação em vez de escondê-lo.
7. **`listContacts` FUNDE as duas linhas da mesma pessoa** — o roster carrega
   390 pessoas duas vezes (uma `@c.us`, uma `@lid`), e a ligação só existe no
   sentido `lid → phone`. Devolver as linhas cruas daria 944 contatos para 545
   pessoas. Ver H39.
8. **`listContacts` promete `pushname`, não `name`** — `getName` responde para
   1 de 944 e `getShortName` para nenhum neste perfil. Prometer "nome" seria
   prometer vazio.
9. **`fetchContactAvatar` trata ausência como resposta, não como erro** — 2 de
   12 contatos não têm foto, e a página responde normalmente com os campos de
   URL ausentes. Devolver erro ali diria "a busca falhou" onde a verdade é "não
   há foto". Ver H40.
10. **`fetchContactAvatar` pergunta ao SERVIDOR, não ao cache** — o
    `ProfilePicThumbCollection` tinha 68 modelos para 544 pessoas, e só 33 com
    URL. Servir dali responderia "sem avatar" para quase toda a agenda.
11. **`onContact` escuta `change`, não `add`** — o `messagemeta` liga em `add` e
    funciona para mensagens; para contatos, `add` disparou ZERO vezes em 90 s e
    `change` disparou 7. Um roster muda por linha atualizada. Copiar a palavra
    do irmão instalaria limpo e não entregaria nada. Ver H43.
12. **`primeContactRoster` é REFRESH, não fetch** — e é a única das catorze sem
    equivalente no wwebjs, então nasceu inteira de medição. Não há lacuna de
    pertencimento (391 de 391 contatos referenciados pelos chats já estavam no
    roster); o que a chamada move é a ligação `lid → phone`, e ela custa 42 s.
    A pós-condição é contra DANO (roster que encolhe), não contra ausência de
    melhora. Ver H45.

### Além das catorze — expansão funcional

A matriz de paridade está fechada. O que vem agora não é paridade, é o objetivo
declarado do projeto: métodos principais de envio e chat.

| capacidade | onde | prova real |
|---|---|---|
| `sendMedia` | `capabilities/send` (`media.go`) | conta A envia PNG, conta B recebe o **MESMO id** com `type=image` em 3 s |
| `sendMedia` (documento) | idem, flag `AsDocument` | os MESMOS bytes dão `image` e `document` |
| resolução de GRUPO | `capabilities/send` (`resolve.go`) | grupo resolve sem passar pela identidade de usuário — provado **sem enviar** |
| `createGroup` | `capabilities/group` | criado com 2 participantes; segunda chamada **reutiliza** (`created=false`) |
| envio a GRUPO | `capabilities/send` | texto enviado ao grupo de laboratório e **verificado** |
| presença (anúncio) | `capabilities/presence` | implementado, 10 testes — **sem pós-condição local, por natureza** |
| presença (observação) | `capabilities/presence` | implementada e **NÃO PROVADA**: `isSubscribed` não se mantém (H50) |
| `listChats` | `capabilities/chats` | **384 conversas, 384 com título**, 2 grupos, 122 com não-lidas |
| `markRead` | `capabilities/chats` (`markread.go`) | 9 testes, 4 controles; caminho real **não exercitado ao vivo** (H52) |
| `react` (add) | `capabilities/react` | **provado**: `hasReaction` false→true em 0,5 s |
| `react` (remove) | `capabilities/react` | funciona; **não verificável na sessão** — `Result.Verified` diz isso (H53) |
| `reply` | `capabilities/send` (`reply.go`) | provado ao vivo; pós-condição de citação com **controle negativo AO VIVO** (H54) |
| `archive`/`pin` | `capabilities/chatstate` | **ciclo completo provado ao vivo**; o app recusa pedido REDUNDANTE, e a guarda evita (H55) |
| `revoke` | `capabilities/revoke` | primeira capacidade DESTRUTIVA; direito consultado na página, `as=sender` provado (H56) |
| link de convite | `capabilities/group` (`invite.go`) | assinatura RESOLVIDA (a metadata é o argumento); falta a busca do código, que **trava** (H57) |

### O que este módulo consegue e não consegue provar

Três itens ficaram implementados e NÃO PROVADOS ao vivo, e não é coincidência:

| item | por quê |
|---|---|
| legenda de mídia (H47) | é conteúdo; provar quebraria a invariante 12 |
| observação de presença (H50) | `isSubscribed` não se mantém; resta privacidade |
| reconhecimento de leitura (H52) | o não lido semeado não apareceu em 90 s |

São todos EFEITOS DE SAÍDA cuja evidência mora fora deste processo. A regra que
sai daí: **um módulo que dirige uma SPA prova bem o que ele LÊ e depende de
terceiros para o que ele ESCREVE.** Onde a prova não existe, o teste PULA com a
razão escrita — nunca passa em silêncio.

**Fatoração que a presença forçou (H50)**: o que `send` e `presence` compartilham
é a RESOLUÇÃO DE IDENTIDADE, não a obtenção do chat — porque enviar quer criar
conversa quando não existe, e anunciar que se está digitando NUNCA pode criar
uma. A parte comum virou `spa.ResolveIdentityExpr`, no pacote de conhecimento de
página, e os dois a usam para fins opostos sem que um herde o efeito colateral do
outro. Fatorar no lugar errado teria dado a `presence` um efeito que ela não pode
ter.

**A regra, corrigida depois da H57.** Quatro capacidades ensinaram que
`reading '<campo>' of undefined` significava "passei o id, queriam o modelo"
(H40, H46, H49). A quinta ocorrência tinha a mesma FORMA e outra causa: eu havia
passado o DONO quando queriam a PARTE — `iAmAdmin` é método em
`groupMetadata.participants`, então quem faltava era `participants`, não o campo.

A regra que sobrevive às duas: **o erro diz QUAL OBJETO falta, e o nome do campo
diz ONDE procurá-lo.** Não é "sempre passe o modelo" — é ler onde aquele campo
mora. JavaScript não distingue os dois casos, e por isso a forma engana.

~~**Aberto e dito**: o ENVIO para grupo ainda não foi provado — só a resolução.
Provar exigiria um grupo onde se possa mandar mensagem sem incomodar ninguém,
isto é, um criado entre as duas contas de laboratório, o que depende de uma
capacidade de criar grupo que ainda não existe (H48).~~ **Fechado** pela H49: o
grupo de laboratório existe, o envio foi provado, e `Ensure` é idempotente por
assunto para que nenhuma execução deixe outro grupo para trás.

**Não verificável aqui**: a entrega de LEGENDA. A invariante 12 torna o módulo
metadata-only e uma legenda é conteúdo; provar exigiria ler o corpo. A
capacidade aceita e repassa, e o teste ao vivo diz em voz alta que não verifica
(H47).

**Divergência consciente**: duas das quatro assinaturas contrariam o palpite
óbvio — `sendToChat` recebe UM objeto `{chat, earlyUpload, options}`, e o módulo
de opaque data é `WAWebMediaOpaqueData` porque `WAWebOpaqueData` é NULL aqui.
Ambas foram LIDAS da fonte antes de existir código, o que evitou a quinta
correção cega desta camada (H46).

## 3. Invariantes travadas em teste

Além das do `HANDOFF §6`, três nasceram nesta semana:

- **15 · o tempo de vida de uma sessão termina no `Stop`** — duas rotas de falha
  distintas (`context canceled` e `deadline exceeded`), cada uma com controle.
- **um profile = um dono** — e o controle negativo mostrou que há **duas
  camadas**: a posse em processo e o próprio lock do Chrome. Só a primeira falha
  rápido e com causa nomeada.
- **nada lê texto de página** — portão de módulo por AST, com isenção que
  **decai** se o teste que a justifica sumir.

## 3.1 Endurecimento sob concorrência (2026-08-19)

Toda capacidade foi provada **sozinha**. O produto não vai usá-las sozinhas: um
timer de liveness bate enquanto um comando busca mensagens enquanto uma
assinatura drena. *"Cada uma funciona"* não é *"elas funcionam juntas"*, e este
módulo passou a semana descobrindo que a falha interessante mora na combinação.

| teste | escala | resultado |
|---|---|---|
| `TestConcurrentCapabilityCallsOnOneSession` | 8 chamadores × 6 iterações, fixture local | **PEAK OVERLAP 8**, latência de pior caso 2 ms → **19 ms** sob sobreposição |
| `TestRealSPAConcurrentCapabilities` | 5 rodadas × 4 capacidades, SPA real | **PEAK OVERLAP 20**, identidade estável |

**Os dois MEDEM A PRÓPRIA SOBREPOSIÇÃO**, e isso não é ornamento: um teste de
concorrência cujos chamadores nunca coincidem prova **serialização**, não
segurança, e o `-race` não tem o que detectar. Com ida e volta de 2 ms isso era
possibilidade real, não pedantismo. Ambos falham se o pico ficar abaixo de 2.

**A asserção que só a concorrência permite**: a identidade do dono tem de voltar
IGUAL em todas as leituras simultâneas. Uma sessão não muda de conta no meio da
corrida, então qualquer variação seria uma resposta chegando ao chamador errado —
e um teste sequencial não consegue nem formular isso.

**O que ficou provado**: a afirmação que o `spa.Monitor` fazia no próprio
comentário — *"seguro para uso concorrente: um timer de sonda e um caminho de
comando podem ambos perguntar"* — deixou de ser prosa. E o custo da concorrência
virou número: **19 ms** de pior caso contra 2 ms sequencial.

## 3.2 Retenção de UMA HORA sob carga (2026-08-20)

`TestRealSPALongHoldUnderLoad`, 60 amostras contra o perfil pareado, com
liveness, owner, fetch e drain rodando a cada minuto.

```
liveness not-alive: 0 | identity absent: 0 | reinstalls: 0 | dropped: 13
latency: mediana 2ms, pior 96ms (a PRIMEIRA amostra, logo após o boot)
RSS: first=1740MB last=760MB min=655MB max=1740MB — deriva 0,44x
processos: 9 em 59 amostras, 10 em uma
```

**A hipótese que motivou medir era VAZAMENTO, e o dado aponta o contrário.**
A memória cai monotonicamente do pico de boot e assenta em torno de 700-800 MB;
a contagem de processos não cresce. Dois eixos independentes concordam, e nenhum
dos dois foi projetado para provar isso — vieram junto porque a amostragem os
incluía.

**Os 13 descartes são todos da rajada do minuto 1** (H25), e nenhuma amostra
posterior descartou. A assinatura seguiu entregando em volume baixo — ~23
eventos esparsos ao longo dos 59 minutos restantes —, o que resolve por
evidência a ambiguidade do H24: assinatura viva, não apenas instalada.

**O que este número NÃO autoriza.** É tentador dividir 16 GB por 800 MB e
declarar capacidade. Não faço isso aqui por três motivos: a `runtime/doc.go` já
avisa que os números do spike são um PISO; esta é UMA sessão, e nada mediu a
interação entre sessões concorrentes no mesmo host; e "sob carga" aqui são as
NOSSAS sondas, com a conta ociosa — tráfego de entrada sustentado é outra
medição, e precisaria de uma conta que ninguém tem.

**O que ele autoriza**: dizer que uma sessão segurada por uma hora **não
degrada** — não perde identidade, não perde a assinatura, não acumula memória
nem processo, e para limpo no fim.

## 3.3 Sessões CONCORRENTES, e a primeira afirmação herdada a ser falsificada

Até 2026-08-20 este repositório **nunca tinha rodado dois browsers ao mesmo
tempo**. A afirmação de `runtime/doc.go` e da ADR-0006 — *474–790 MB por sessão,
escalando linearmente até três* — vinha do spike e nunca fora exercitada. E ela
é carga: é o que põe um host de 16 GB *"na faixa de dezenas de sessões"*.

| sessões | total | por sessão | processos | pior latência |
|---:|---:|---:|---:|---:|
| 1 | 1023 MB | 1023 MB | 9 | 0 ms |
| 2 | 2004 MB | 1002 MB | 18 | 0 ms |
| 3 | 2600 MB | **866 MB** | 31 | 1 ms |

**A FORMA se confirma, e melhor que o prometido**: o custo por sessão CAI para
0,85× com três concorrentes, e nenhuma sessão já rodando parou de responder
quando outra subiu. Medi TODAS as sessões vivas a cada onda, não só a nova —
porque a afirmação é sobre o que acontece com as que já estão lá.

**O NÚMERO se falsifica**: 1023 MB contra o teto de 790 MB, **30% acima**, na
mesma condição pré-login e com os **mesmos 9 processos**. Mais memória na mesma
topologia, o que descarta a explicação fácil.

**A consequência foi APLICADA, não só registrada**: o `runtime/doc.go` passou a
carregar a re-medição e a aritmética que ela muda — **~15 sessões em 16 GB** no
lugar de ~20.

**O que segue sem medição, e é o que decide capacidade de verdade**: sessões
**PAREADAS** concorrentes. Não foi feito de propósito — os dois perfis pareados
desta máquina são da MESMA conta, e dois dispositivos ativos podem fazer o
WhatsApp invalidar um. Precisa de duas contas, e isso é decisão do humano.

## 4. Aberto, e por quê

| # | por que segue aberto |
|---|---|
| H3 | pendência de OUTRA sessão (documentos do `disparazaap`) |
| H8 | `Contains` na checagem de host; apertar **muda comportamento** |
| H9 | pré-existente, fora do escopo desta iniciativa |
| H10 | **de propósito** — "o perfil nunca encolhe" é falso e está registrado |
| H11 | leitura de coluna do M6; documental |
| H15 | envelope de `C` sem expressão executável — depende de consumidor que **não existe** (H17 mediu) |
| H16 | **não fechável por amostragem**: mutação numa janela de 24 h escapa de qualquer varredura sobre 1,8×10¹⁹ ns. A garantia é estrutural |
| H18 | `BootFailure` sem PID — aguardava consumidor; hoje há seis capacidades, então é o mais maduro para reabrir |
| H25 | teto de 500 no buffer **sem medição** — precisa de conta de volume real |

| H32 | auditoria de constantes **ABANDONADA** — varredura de texto não lê estrutura de código, e a alternativa (AST) custa mais que o achado justifica |
| H33 | **MEDIDO**: linearidade confirmada, teto de 790 MB falsificado. Segue aberto porque sessões PAREADAS concorrentes precisam de uma segunda conta |

**Nenhum deles é guarda sem teste.** Essa categoria zerou com o H27.

### Achados de 2026-08-20, todos de auditoria sobre o próprio trabalho

- **H30** — os quatro dublês das capacidades **ignoravam o `ctx`**, e a produção
  não ignora. Uma capacidade chamada com contexto cancelado errava em produção e
  **passava** nos testes: o chamador que já desistiu recebia dado *fabricado*.
- **H31** — enumerando as 72 exportadas contra os testes, achei
  `liveness.NewWithMonitor`: um construtor que **inventei** para um chamador que
  nunca existiu. API exportada é promessa; removida.
- **H32** — a auditoria de constantes foi abandonada, e o commit dela saiu com o
  portão **vermelho** porque eu rodei o gate dentro de um *pipe* e o `&&` viu o
  status do `tail`. **Gate dentro de pipe não é gate.**
- **H18** — `BootFailure` ganhou o PID, para **correlação e nunca ação** — a
  fronteira que o separa do H23. Dois controles negativos **não morderam**: a
  ordem de leitura do pid (afirmação minha, falsa) e a guarda da fronteira, que
  **removi** em vez de manter, porque teste que não morde é garantia falsa.
- **H30/H31** — ver acima. Junto com o H18, os três vieram de **auditar o próprio
  trabalho**, não de escrever código novo.
- **H33** — a primeira afirmação HERDADA que este módulo falsificou com número.

## 5. Depende do humano

**Revisto em 22/08.** Quatro dos itens antigos saíram porque a sessão dupla
conta-A/conta-B (H135) os resolveu sozinha: o par agora acorda junto, então
provar a direção `in`, observar evento de participante e exercitar convite de
admin deixaram de precisar de gente.

O que resta é o que exige mesmo um humano:

1. **`git push`** — commits parados no worktree.
2. **Merge para `feature/macbook-lucas`** — 34 conflitos, **21 implementações
   RIVAIS** de `lease`/`dispatch`/`cluster`. Escolher errado apaga trabalho.
3. **Trocar a foto de perfil da conta-A** — visível a 944 contatos de uma conta
   comercial real. Trava `setProfilePicture` e `deleteProfilePicture`. A
   orquestração foi consultada em 22/08 (decisão 61) e concordou que é
   escalação legítima: *"foto e status têm alcance externo real"*.
4. **Postar um status** — mesmo alcance, mesma decisão. Trava
   `revokeStatusMessage`, que só pode revogar o que existe.
5. **Reset do perfil de laboratório** — pareado; trava os testes de QR.
6. **Conta de volume real** — calibra o H25.
7. **CAP-07 `sendText`** — fora da matriz; decisão de produto.

## 5.1 A ordem de descoberta importa mais que uma bateria a mais

A H41 é o caso: o `listContacts` saiu com medição prévia, nove testes, cinco
controles negativos e prova ao vivo com as três somas fechando — e ainda assim
embarcou uma linha de sistema como se fosse pessoa. A medição perguntou
"quantas pessoas?" e nunca "isto é uma pessoa?".

Quem fez a pergunta certa foi a capacidade SEGUINTE, ao usar a saída da anterior
como entrada: o pedido de avatar para aquela linha travou 30 s sem responder e
sem lançar. **Encadear capacidades em prova de integração vale mais que mais uma
bateria sobre a mesma.**

Corolário prático, também da H41: um teste ao vivo deve CONTAR os desfechos, não
abortar no primeiro. `t.Fatalf` no primeiro contato teria escondido se o
problema era aquele contato ou o caminho todo — e os dois exigem conserto
diferente. Foi `present=3 absent=8 failed=1` que deu o diagnóstico.

**Segundo corolário, da H43**: quando a prova depende de um evento acontecer,
PROVOQUE o evento. Esperar o roster mudar sozinho funcionaria — ele muda 7 vezes
em 90 s — mas um teste que depende de a conta alheia estar movimentada falha numa
tarde quieta. Como 24 dos 46 eventos vinham de `profilePicThumb`, e o
`fetchContactAvatar` escreve esses campos, a capacidade de avatar virou o
estímulo. É a mesma ideia do parágrafo acima vista do outro lado: encadear
capacidades não só encontra defeitos, também torna as provas determinísticas.

**Quinto — um detector que não pode falhar não é detector (H45)**. O
`primeContactRoster` tratava "nada mudou" como sucesso, o que torna
indistinguíveis *"já estava atualizado"* e *"o sync nunca rodou"*. A saída não
foi argumentar: foi procurar uma marca observável, descartar DOIS candidatos por
medição, e achar a terceira diffando todo o `localStorage` por hash. E então
controlá-la — 45 s ociosos provando que ela não deriva sozinha. Sem esse
controle, "mudou depois que eu chamei" seria coincidência com cara de prova.

**Quarto, e é o mais desconfortável (H45)**: um instrumento pode passar em todos
os seus próprios testes e ainda estar medindo a coisa errada. O `primeContactRoster`
contava NOMES e reportou `changed=false` para um refresh que religou 23 linhas
`@lid` ao telefone — mudança que o `List` viu na hora (544 pessoas viraram 521
sobre as mesmas 944 linhas) e que o meu `Snapshot` não tinha campo para ver.
Quem denunciou foi a prova AO VIVO, ao imprimir o roster depois. Testes de dublê
não podiam pegar isso: eles só sabem os campos que eu decidi medir.

**Terceiro, e é sobre AMBIGUIDADE (H43)**: "nada disparou" não distingue "este
build não tem esse evento" de "ficou quieto". A sonda passou a disparar um evento
privado em si mesma e conferir que o próprio handler o viu. Sem esse auto-teste,
todos os zeros medidos seriam ilegíveis.

## 6. O que esta semana ensinou, em uma frase cada

- **O dublê não é mais permissivo que a produção — é mais bem-comportado num
  eixo em que ninguém olha.** Velocidade, tempo de vida do contexto, existência
  de socket. A pergunta é *em que EIXO meu dublê é melhor que o mundo?*
- **Este teste passaria se a ação que ele pede não tivesse acontecido?** Se sim,
  ele não mede a ação. Pegou o teste de entrega e o de boot não pareado.
- **Um instrumento que não distingue dois estados não pode decidir entre eles.**
  Quatro instrumentos cegos consertados, três quebrados por mim no mesmo dia.
- **Controle negativo que não morde não prova nada** — e sete não morderam na
  primeira tentativa.
- **A verdura da suíte não distingue "coberto" de "coberto onde importa".** A
  auditoria achou a lacuna na capacidade nº 1.
- **Medir e depois ler a medição com a hipótese na cabeça é não ter medido.**

## 7. O que 22/08 ensinou

Um dia inteiro numa frase cada, escolhidas por terem MUDADO decisão, não por
soarem bem.

1. **Uma medição que dá zero precisa de controle positivo antes de virar
   conclusão** (H114). Buscar `"a"` em 395 mensagens deu zero e quase virou "a
   busca não funciona"; um termo real deu 20.

2. **Quando a regra mora no script, o teste tem de olhar para o script.** Quatro
   controles negativos não morderam num só dia porque o dublê fornecia o valor
   que a asserção examinava — o teste media o parser, não a regra.

3. **Ler o nome na referência é o primeiro passo, não o último** (H134, H137).
   Ela chama nomes que este build não tem. Quando falha, ENUMERE o módulo: três
   tentativas custaram três execuções ao vivo, a enumeração custou uma.

4. **A resposta pode estar certa e a pergunta errada** (H140). As treze
   ausências registradas eram todas verdadeiras; meus erros foram escolher o
   módulo errado para perguntar.

5. **Produzir o fato é a saída do impasse "nunca visto acontecer"** (H119, H121,
   H131, H135). Trocar o assunto do grupo, votar numa enquete, responder uma
   mensagem, fazer a outra conta sair — quatro linhas fecharam assim.

6. **"A chamada não lançou" nunca é a prova** (H133, H136, H137, H139). A
   revogação de convite foi provada pelo ACEITE FALHAR; a transferência de posse,
   pela membership lida do outro lado.

7. **Existe uma terceira resposta entre sim e não** (H133). Quando o oráculo não
   existe, `ErrUnverifiable` diz "não dá para saber" em vez de afirmar falha —
   que seria inventar conhecimento.

8. **Verificar sobra é IR OLHAR, das duas pontas** (H139). Minha limpeza
   retornava sucesso e deixava seis modelos obsoletos do outro lado.

9. **Uma causa costuma estar por baixo de várias linhas** (H138). `findImpl` e
   `markFetchStart` ausentes explicam assinar, silenciar e buscar mensagens de
   canal: um método voltando destrava cinco linhas.

10. **`MISSING` mente por inércia** (H141). Depois que a evidência é colhida, o
    rótulo tem de acompanhar — senão o ledger conta itens em vez de orientar
    trabalho.

## Fase 2 — endurecimento de produção (decisão 65, 2026-08-22)

Enunciado recebido da orquestração, verbatim:

> **Fase 2 é ENDURECIMENTO DE PRODUÇÃO do `wa-headless`**: fechar primeiro as
> dívidas internas acionáveis e depois provar reconexão, perfil sujo,
> long-running, concorrência/multi-sessão, carga, limites de CPU/RAM, teardown,
> event-bus, recuperação e observabilidade sem sucesso silencioso; **não inclui
> HTTP, que pertence ao `wa-api`**.
>
> Ela **encerra quando** todas as invariantes operacionais críticas estiverem
> provadas **sob falha e carga**, sem leaks/orphans/races, com recursos
> *bounded*, recuperação determinística e **nenhum achado acionável de
> severidade alta aberto**. Só depois disso o foco passa para integração ampla no
> `wa-api` ou para `wa-noise`.

### O que isso muda no método

A Fase 1 foi medida contra uma REFERÊNCIA — o ledger de paridade. A Fase 2 é
medida contra **falha e carga**, e o `CLAUDE.md` já tem a regra que governa isso,
escrita depois do pool de despacho da F86: *meça o cenário em que o mecanismo
COBRA o preço, não só aquele em que ele paga*. A pergunta de cada item passa a
ser **qual entrada faz esta proteção virar o problema**.

A invariante que o projeto já enunciou continua valendo e agora é o centro:

> Nada que espere por relógio ou por par morto pode ocupar slot limitado.

### Primeira parada: as dívidas internas acionáveis

São as escaladas durante a Fase 1 e ainda não decididas:

1. **Identidade crua na superfície (H151)** — três capacidades dão resposta bem
   formada e ERRADA para jid de telefone num build LID-first, cada uma de um
   jeito. Não é bug em três lugares: é uma decisão que ninguém tomou sobre quem
   resolve identidade, o chamador ou a capacidade. Três opções com custos
   diferentes estão registradas na H151.
2. **`chats.Clear` sem pós-condição (H166)** — `Emptied` carrega `MessagesBefore`
   e nenhum "depois", então um `Clear` que não apagasse nada devolveria o mesmo
   valor de sucesso. Mudar de "nunca falha" para "pode falhar" é mudança de
   contrato.
3. **`addressbook.DeviceCount` aceita jid não resolvido (H148)** — caso
   particular do item 1, registrado antes de o padrão ser visto.

A assimetria de portas da `ChatCollection` — só `change`, sem `remove` — **já foi
fechada** na H173.

### Balanço da Fase 2 — 2026-08-22

Medido contra o critério da decisão 65, cláusula por cláusula.

| cláusula | evidência | achado |
|---|---|---|
| sem leaks/orphans | teardown 3 ciclos: 13 goroutines → 2, 9 chromes → 0 (H174); 9 min: goroutines 10→10, RSS 727→638MB (H185) | nenhum |
| sem races | 25 de 26 capacidades corrigidas; 80 leituras concorrentes, 0 trocas (H177–H181) | **1 severidade alta, FECHADO** |
| recursos bounded | buffer da página 512 com contador, guarda estrutural e de fronteira; globais 0 antes/depois de 1000 chamadas (H183, H184) | nenhum |
| sob carga | 1000 chamadas: 1,553s, p50 40ms, p95 233ms, 0 erros (H183) | nenhum |
| recuperação determinística | chamada em voo com SIGKILL: 2,017s, `TargetGoneError`, sem vazar; `ErrSessionDied` depois (H182, H183) | nenhum |
| sem sucesso silencioso | 49 escritas auditadas seguindo delegação: zero mentem (H184) | nenhum |

**Números de capacidade que não existiam escritos:**

- sessão **ociosa**: 6% de um núcleo (~16 por núcleo) e 600–700MB de RSS
- sob carga saturante: 12 trabalhadores → 1,76 núcleo, **sub-linear**

O sub-linear é a informação de projeto: o gargalo é a **conexão CDP única**, que
serializa. Mais vazão por máquina se faz com mais **sessões**, não com mais
concorrência dentro de uma.

### FASE 2 ENCERRADA — decisão 70

> **SIM, a Fase 2 fecha; o único gap crítico restante, recuperação completa de
> perfil sujo, fica como `BLOCKED_EXTERNAL` já autorizado pela 68, e nenhuma
> outra invariante crítica falta medir.**

**Em aberto, e só isto:**

1. `BLOCKED_EXTERNAL` — recuperação de **perfil sujo** ponta a ponta. Exige matar
   o navegador num perfil pareado, cujo pior caso é repareamento por um humano
   com o telefone. O mecanismo foi medido com perfil temporário; o caminho
   completo não, e por decisão (68) não será.
2. Duas observações de desenho registradas e não corrigidas: a honestidade do
   sinalizador `Verified` é **disponível mas não imposta**, e o contador de issues
   do lint saltou de 267 para 472 (issues pré-existentes).

## Fase 3 — integração ampla no wa-api (decisão 71, 2026-08-22)

Enunciado recebido, verbatim:

> **Próximo foco é integração ampla no `wa-api`; fecha quando as 35 capabilities
> forem alcançáveis pela fronteira correta, com seleção de engine, contratos
> equivalentes e testes cross-adapter, sem capability órfã.**

### O ponto de partida, MEDIDO

**0 de 35.** Não "algumas faltando" — nenhuma:

- `internal/wa-headless/main.go`, que é a FACHADA e o único caminho de import
  permitido, tem **31 linhas e zero símbolos exportados**. O próprio doc dele diz
  que a fachada ainda está vazia.
- `pkg/infra/wa-headless/` tem três arquivos e **todos são `doc.go`**, inclusive
  em `client/` e `registry/`.

Eu havia relatado à orquestração que o `pkg/` "não expõe todas"; medi depois e
corrigi o número com eles antes de começar. A diferença entre "algumas faltando"
e "nenhuma" muda o tamanho do trabalho, e um enunciado dimensionado sobre o
número errado seria pior que nenhum enunciado.

### A regra que governa o trabalho, e ela já está escrita

Do doc da fachada:

> Nada fora desta árvore importa outra coisa senão este arquivo. Quando um
> símbolo falta, a correção é **acrescentar a linha na fachada**, nunca alcançar
> por trás dela.

E do doc de `pkg/infra/wa-headless`:

> O que vive aqui é esperado satisfazer os MESMOS ports que
> `pkg/infra/wa-noise` satisfaz onde um caso de uso não deveria se importar com
> qual transporte está atrás. Onde uma capacidade existe em só um dos dois, essa
> assimetria é decisão de produto e pertence a um ADR, **não a uma diferença
> silenciosa entre dois adaptadores**.

Isso já responde três das cinco cláusulas do critério: *fronteira correta* é a
fachada, *contratos equivalentes* são os ports do `wa-noise`, e *sem capability
órfã* é a proibição de assimetria silenciosa.


## A primeira fatia da Fase 3 não foi código: foi descobrir que "equivalente" não é "de nome parecido"

O plano era provar o padrão ponta a ponta pelo menor port, `JIDResolver`. O
mapeamento óbvio seria `JIDResolver → capabilities/lookup.NumberID`, que
resolve identidade perguntando à página.

Fui ler o adaptador do `wa-noise` antes de escrever, e ele **ignora o
contexto**: `ResolveJID(_ context.Context, raw string)` é parsing puro, sem E/S.
Implementar o headless com `lookup` teria posto **rede atrás de um contrato
puro**, fazendo todo chamador pagar ida-e-volta — exatamente o que a decisão 66
recusou ao proibir rede oculta dentro de leitor.

Fica a regra, porque ela vale para os 35: **contrato equivalente é sobre a
NATUREZA do contrato — puro ou de transporte — e não sobre a capability de nome
parecido.** Dos adaptadores do `wa-noise`, 10 ignoram o contexto e 166 o usam;
a fronteira entre os dois grupos é onde o mapeamento mecânico erra.

### Decisão 72, e a premissa que eu tinha errada

Levei à orquestração a pergunta de onde mora uma regra que não é de transporte,
com três opções. A resposta foi **(b): extrair para um pacote compartilhado
independente de transporte, migrando os dois adaptadores em commit isolado**.

Eu havia relatado que o parser era o `types.ParseJID` vendorizado. **Estava
errado**: `ResolveJID` usa um `ParseJID` LOCAL e nosso, em
`pkg/infra/wa-noise/mapping/jid/parse.go`. A regra pura já era código wa-api.
Corrigi a premissa com a orquestração antes de seguir — a escolha não mudou,
o custo sim.

### O que a medição achou de lado: F101

Medir o comportamento real antes de reescrevê-lo produziu um panic:
`ParseJID("")` morre em `arg[0]`, e a porta que promete `(JID, error)` entrega
um crash. Enumerei os 11 chamadores um a um, e um está aberto de fora
(`participants:[""]` em `/group/create`). Está no `HOUSEKEEP.md` da raiz como
**F101, corrigido**, por decisão 73 — que mandou sanear ANTES de extrair,
justamente para o defeito não ganhar duas casas.

O controle negativo ensinou mais que o teste: sem a guarda o handler responde
**200**, não 500, porque o dublê `JIDResolver` aceita a string vazia de bom
grado. A suíte de rota inteira fica verde com o defeito no lugar. É a armadilha
nº 1 do `ARMADILHAS.md` outra vez, e é a prova de que o teste no porto real não
era redundante com o teste de rota.

## Decisão 74 — não existe forma canônica compartilhada, e isso muda a Fase 3

Ao procurar o que extrair, medi o que cada transporte chama de canônico. **Eles
não concordam**, e não de um jeito cosmético:

| | número nu vira | identidade de pessoa |
| --- | --- | --- |
| socket (`wa-noise`) | `5511…@s.whatsapp.net` | `s.whatsapp.net` / `lid` |
| página (`wa-headless`) | — | `c.us` / `lid` |

O `types` vendorizado do socket chama `c.us` de **`LegacyUserServer`** — o que
o socket considera legado é o namespace **corrente** da SPA que dirigimos. E
`c.us` não está morto no socket: é o sufixo de consulta do USync
(`capabilities/user/info.go:46`) e o blocklist normaliza por ele.

A consequência atinge a decisão 71 inteira: `ResolveJID("5511999999999")` deve
devolver `…@s.whatsapp.net` num adaptador e `…@c.us` no outro para ser
utilizável. **Mesma porta, mesma entrada, saída corretamente DIFERENTE** — e um
`domain.JID` produzido por um adaptador está silenciosamente errado se consumido
pelo outro. Hoje isso não explode porque só existe um adaptador; a Fase 3 cria
o segundo.

Isso derrubou a minha própria proposta de extrair um PARSER compartilhado: não
há canonicalização a compartilhar. A orquestração decidiu (**74**): `domain.JID`
passa a **carregar o namespace explicitamente**, compartilhando só normalização
de entrada e vocabulário, com **cada adaptador dono do próprio sufixo canônico**.

Implementado em `pkg/domain/jid_namespace.go` como MÉTODOS sobre o tipo
nomeado, e não como struct: `domain.JID` aparece em 221 sítios não-teste de 31
arquivos, e a versão aditiva é subconjunto estrito — se depois se quiser a
struct, o vocabulário já está lá.

O teste que importa não é o do vocabulário: é o que **amarra o vocabulário às
constantes REAIS de cada transporte** (`types.DefaultUserServer`,
`types.LegacyUserServer`, `types.HiddenUserServer`), em vez de repetir as
strings à mão. Repetir a string só provaria que sei copiar; ler a constante faz
o teste acusar quando a produção mudar.

## Decisão 75 — a Fase 3 precisa de N browsers, e medir isso contrariou o nosso próprio código

O registry que mapeia `txtID` → sessão esbarra numa assimetria que o socket não
tem: **cada sessão headless precisa do seu próprio `ProfileDir` e da sua própria
porta de debug**. Dois browsers não compartilham nenhum dos dois.

Antes de escrever o alocador de portas, fui medir. O `BuildFlags` exigia a
porta, com a justificativa escrita ao lado:

> `DebuggingPort is required; without it there is no CDP endpoint and no way to
> stop the browser cleanly`

**A justificativa estava errada.** Com `--remote-debugging-port=0` o Chromium
sobe, escolhe porta efêmera e escreve `<ProfileDir>/DevToolsActivePort` com
duas linhas. Medido em 2026-08-22, Chrome 151.0.7922.170, macOS 15.6, perfil
temporário descartado depois:

```
55077
/devtools/browser/954157ae-38a1-4ef0-b7d5-f03f3c1f7d03
```

### A medição estava INCOMPLETA, e a suíte é que disse

A primeira medição usou porta 0 e concluiu "o Chromium publica o arquivo".
Implementei em cima disso, e **oito testes de integração falharam** — eles
fixam porta. Fui medir o caso fixado, com o conjunto canônico completo:

| `--remote-debugging-port` | escreve `DevToolsActivePort`? | responde HTTP? |
| --- | --- | --- |
| `0` | **sim** | sim |
| fixada | **NÃO** | sim |

Com porta explícita o Chromium não escreve o arquivo de todo. Não é uma
preferência de implementação: **qual caminho existe é decidido pelo Chromium**,
e por isso o launcher tem dois. O do perfil só existe no caso efêmero — que é o
caso que o registry vai usar, e é o único com garantia de identidade.

Fixar porta passa a ser o chamador assumindo o risco explicitamente, que é o
que um override deve ser.

Isto também matou um teste MEU: eu havia escrito "porta fixada discorda do
arquivo publicado", que é um estado que o Chrome real nunca produz. Travá-lo
seria a armadilha nº 1 — dublê com uma forma que a produção não tem. A regra do
projeto de que a medição derruba a hipótese vale inclusive quando a hipótese já
virou código e teste verde.

### E a segunda metade é pior que a primeira

O `awaitEndpoint` fazia GET em `127.0.0.1:PORT/json/version` e **aceitava
qualquer browser que respondesse**. Nada amarrava a resposta ao processo que
lançamos nem ao perfil que pedimos.

Com um browser por vez, inofensivo. Com N, o modo de falha não é "falha ao
subir" — é a **sessão B anexar-se ao browser da sessão A**. Neste stack, isso é
dirigir a conta de WhatsApp errada, em silêncio. O helper de porta dos testes é
bind-`:0`-e-fecha, TOCTOU clássico: duas sessões arrancando juntas podem receber
o mesmo número.

A porta nunca foi a identidade. **O diretório de perfil é.**

### O que a referência deu, e o que ela não deu

O ENTENDIMENTO veio do puppeteer, que o whatsapp-web.js usa: ele lê o
`DevToolsActivePort` do user-data-dir em vez de confiar na porta pedida. O
COMPORTAMENTO foi medido aqui — pela mesma razão de sempre, a de que quatro
nomes de módulo do wwebjs já não existiram no nosso build.

### O buraco que a própria correção abriria

Um `DevToolsActivePort` obsoleto reproduziria o defeito que a mudança remove: o
Chromium apaga o arquivo ao sair limpo, mas um browser que morreu o deixa para
trás, apontando para uma porta agora livre — ou pior, para uma que outro browser
já tomou. Por isso o arquivo é **apagado antes do arranque**: o que se lê depois
só pode ter sido escrito pelo processo que lançamos.

É a Regra 4 do CLAUDE.md aplicada a si mesma — o conserto do conserto também é
um mecanismo.

### O controle negativo que demonstra em vez de afirmar

Removida a limpeza, o teste não falha por uma asserção abstrata: ele mostra o
launcher conectando-se a `ws://127.0.0.1:40001/devtools/browser/STALE`, o
endpoint de outra execução. É o "browser errado" acontecendo dentro de um teste.

O antigo teste do "ws vazio" foi substituído pela corrida REAL — o leitor que
chega no meio da escrita e vê só a linha da porta. E o dublê passou a PUBLICAR o
endpoint no perfil, no formato medido contra o Chrome real; um dublê com outra
forma provaria apenas que o parser lê o que o dublê escreve.

## Decisão 76 — o registry nasce COM teto, e o valor sai da medição

O `runtime.Holder` documenta que adiou de propósito "teto de capacidade,
política de reciclagem, pool de qualquer espécie", citando a Regra 1 do
`CLAUDE.md`. O registry `txtID → sessão` é esse pool — e aqui um detentor não é
uma goroutine: é **um Chrome inteiro**.

A orquestração decidiu (**76**): nasce com teto, mas o valor só depois de medir
custo ocioso e arranque simultâneo, incluindo fila, timeout e liberação de slot.

### O que foi medido

Chrome 151.0.7922.170, macOS 15.6, conjunto canônico completo de flags, perfis
**temporários** descartados a cada rodada. RSS somado de todos os processos do
browser (o RSS conta memória compartilhada mais de uma vez, então é teto, não
piso).

| cenário | por browser | arranque até o endpoint |
| --- | --- | --- |
| `about:blank`, N=1 | ~620 MB | 0,3 s |
| `about:blank`, N=4 | ~520 MB | 0,65 s |
| `about:blank`, N=8 | ~490 MB | 0,96 s |
| SPA real (página de QR), N=1 | ~675 MB | — |
| SPA real, N=2 | ~650 MB | — |
| SPA real, N=4 | ~653 MB | — |

Três leituras, e duas eu não teria adivinhado:

1. **A SPA custa pouco acima do `about:blank`** — ~55 MB. O caro é o processo
   de browser em si, não o bundle. Um teto calculado sobre "quanto pesa a
   aplicação" erraria a conta inteira.
2. **O custo por browser CAI com N** (620 → 490 MB de 1 para 8), porque o RSS
   conta as páginas compartilhadas em cada processo. Medir um só browser e
   multiplicar superestima.
3. **O arranque até o endpoint é sub-segundo mesmo com 8 simultâneos** e não
   degrada. Ou seja, o detentor de slot NÃO é o arranque do processo — é a
   montagem da SPA, exatamente onde o comentário do `Holder` já apontava.

### O que NÃO foi medido, e por quê

**A sessão PAREADA.** O número acima é de uma página de QR, não de uma sessão
com 384 conversas e 399 mensagens carregadas, que é estritamente mais pesada.
O perfil pareado chega às sondas por `WA_SEND_FROM_PROFILE`, que não está
definida nesta sessão — e procurar um perfil pareado no disco é procurar
material de credencial, que não se faz por conta própria.

**Fila, timeout e liberação de slot** são propriedades do mecanismo que ainda
não existe. Medi-las agora seria medir a hipótese, não o mecanismo — elas vêm
depois de o registry existir, e a medição tem de ser a do cenário onde o teto
COBRA o preço (Regra 2), não a daquele onde ele ajuda.

## Decisão 78 — o pedágio de existir um consumidor, e a régua que não servia

O registry da 76 ficou verde sob `-race` com quatro controles negativos, e
ainda assim o gate ficou **vermelho** — por um motivo que não era o registry.

`min_eligible` saltou 571 → 660, `func_coverage` caiu 697 → 603, `errpath` caiu
858 → 808. Provei a causa em vez de a deduzir, removendo e repondo o consumidor
da fachada:

| | `eligible` | `func` | `errpath` |
| --- | --- | --- | --- |
| sem consumidor da fachada | 571 | 697 | 858 |
| com consumidor da fachada | 660 | 603 | 808 |

O `packages.Visit` do `cmd/logcov` inclui qualquer pacote transitivamente
importado sob `wa-api/`. **O simples ato de fiar o stack headless em `pkg/`
arrastou a árvore inteira para o denominador.** Nenhum sítio deixou de logar.

### A régua estava errada, não a árvore

As 85 funções que entraram descobertas não estão sem observabilidade. Esta
árvore observa por duas formas que a régua da camada de aplicação não via:

```go
BootFailure{Stage: StageOwnership, Cause: err}    // o erro CARREGA onde falhou
runner.Do(ctx, OpBoot, label+"/await-endpoint")   // a operação fica no OpLog
```

Um estágio tipado diz **mais** que uma linha de log: chega intacto a quem decide
se aquilo é erro, e é essa camada que loga. É o padrão "adapter não loga, use
case loga" que o `.log-coverage-baseline` já aceitou dezenas de vezes, agora em
escala de biblioteca.

A orquestração recusou as duas saídas fáceis e escolheu a cara (**78**): ensinar
a régua, preservando denominador e ratchets. Excluir `internal/wa-headless/`
seria encolher a base para embelezar o número — o `internal/wa-noise/` está
excluído por ser terceiro vendorizado, e este é código nosso.

### Três coisas que a implementação ensinou

**A primeira versão da regra rejeitava 163 dos 169 sítios.** Ela exigia rótulo
literal puro, e o idioma real da árvore é `label + "/kick"`. A regra media a
minha suposição sobre o código em vez do código.

**Operação rastreada conta como `Info`, não `Warn`**, e é deliberado: rastrear
prova que a operação ACONTECEU, não que alguém classificou uma falha. Como
`Warn`, toda função que chama o rastreador pareceria ter coberto os seus erros
— a confiança falsa que a métrica existe para não dar.

**Dois dos três controles negativos não morderam.** O repositório não tem
literal com `Stage` sem `Cause`, nem outro `.Do` de três argumentos, então
afrouxar a regra nesses pontos passava com o golden intacto. Propriedade que
nenhum teste segura não está travada por o código a expressar — escrevi testes
unitários para elas. E ao repetir o controle A ele **quebrou o build**
(`hasCause declared and not used`) em vez de falhar: a armadilha nº 3 do
`ARMADILHAS.md` em pessoa, e ajustei até compilar E falhar.

### O que continua descoberto, e por quê

Conferido com `-list-uncovered`, não deduzido: engine (39), spa (11), events
(8), runtime (7), observability (5), core (4), liveness (2). A maior parte são
construtores e **validadores puros** — `BuildFlags`, `AppendFlags`,
`ClearSessionSuspect` — que devolvem erro com mensagem completa e acionável, não
tocam porta nenhuma, e cujo chamador é que decide se aquilo é erro. Mesmo caso
já aceito para `clusterModeConfigurado` e `validarStackDoModo`.

### Aviso de escopo: o pedágio não acabou

A fachada hoje importa `core`, `engine`, `runtime` e `spa`. **As 35 capabilities
ainda NÃO estão no denominador** — medido: dos 169 sítios de operação rastreada
da árvore, só 9 estão em pacotes medidos. Fiá-las traz o resto.

## Decisão 77 — a invariante virou dois contadores

O inventário da 76 expôs o detentor perigoso: uma sessão em QR espera por um
**humano**, o que é pior que esperar por um relógio. Com teto 4, quatro sessões
não pareadas seguram o pool inteiro e matam de fome as pareadas.

A orquestração decidiu (**77**): pareamento fica **fora** do pool operacional,
em quota própria com prazo explícito. Espera humana nunca ocupa slot de sessão
pareada.

O que isso muda de verdade: a invariante do projeto — *nada que espere por
relógio ou por par morto pode ocupar slot limitado* — deixou de ser uma frase
num documento e passou a ser **dois contadores**. Espera humana não tem como
tocar num slot pareado porque não é contada no mesmo lugar.

### A armadilha que só apareceu ao desenhar, e que é a Regra 2

A quota de pareamento **não pode** ser o caminho por onde toda sessão nova
passa. Reiniciar o processo com N sessões já pareadas restauraria todas por ali
e bateria numa quota deliberadamente pequena — o teto protetor viraria a
indisponibilidade.

É literalmente a pergunta da Regra 2: *qual entrada faz esta proteção virar o
problema?* Resposta: o restart. Por isso o `Kind` é **explícito** no `Acquire` e
nunca inferido, e há um teste que falha exatamente nesse cenário.

### Duas decisões menores que evitam vazamento

`Promote` com o pool operacional cheio **falha e mantém a sessão** na quota de
pareamento. Uma promoção que largasse a entrada em silêncio vazaria um browser
vivo sem slot nenhum a contá-lo.

`Expired` **relata, não age**. Parar um browser fala CDP e leva segundos; fazê-lo
sob o lock faria todo `Acquire` esperar por um desligamento alheio — a mesma
razão pela qual `Release` para fora do lock.

### Controles negativos executados: quatro

Um deles voltou a quebrar o build (`cutoff` declarado e não usado) e foi
ajustado até compilar E falhar — a armadilha nº 3 pela segunda vez na mesma
sessão. O mais informativo é o quarto: trocar o relógio injetado por
`time.Now()` faz o teste do prazo falhar, o que prova que ele mede a REGRA e não
o relógio da máquina. Um teste de expiração que dorme prova apenas que dormir
funciona.

## O mapa da Fase 3, medido — e a contagem estava errada

A decisão 71 diz "as 35 capabilities alcançáveis pela fronteira correta". Ao
fazer a primeira, fui medir o conjunto todo em vez de o descobrir uma a uma, e a
contagem não fecha assim.

São 35 ports e 35 capabilities, mas **não são a mesma coisa**. Medindo pelo que
distingue um port de transporte — a presença de `txtID string`, que é o
endereçamento de sessão:

| | quantidade |
| --- | --- |
| ports declarados | 35 |
| **ports de TRANSPORTE** (com `txtID`) | **18** |
| ports de infraestrutura (`Session*`, `Storage`, `UserRepository`, `Logger`, …) | 17 |
| capabilities headless | 35 |

Os 18 de transporte:

```
AppStateSyncer      BlocklistManager    CallRejecter        ChatArchiver
ChatMessenger       ContactDirectory    GroupDirectory      GroupLifecycle
GroupRequests       GroupSettings       MessageComposer     NewsletterReader
PresenceController  PrivacyManager      ProfileAccessProvider
SessionController   SessionGuard        UnavailableMessageRequester
```

Deles, hoje: **`ChatArchiver` satisfeito**, `SessionGuard` embutido em todos, e
`UnavailableMessageRequester` **recusado por natureza** — pedir reenvio de
mensagem indecifrável não existe para quem dirige a página.

### Por que a contagem importa

Um port cobre várias capabilities (`GroupSettings` toca `group` e `groupreq`), e
há capabilities sem port nenhum (`liveness`, `messagemeta`, `owner` servem o
próprio stack, não a aplicação). Medir progresso por "capabilities alcançáveis"
conta a coisa errada nas duas direções.

O critério mensurável é: **cada port de transporte satisfeito, ou recusado com
motivo escrito.** É verificável pelo compilador — a asserção
`var _ appport.X = (*Adapter)(nil)` — e a recusa fica travada em teste, como a
da decisão 80.

## Decisão 81 — o critério de encerramento passou a ser verificável

A orquestração aceitou a correção de contagem (**81**): a Fase 3 fecha pelos
**18 ports de transporte**, cada um satisfeito ou recusado **com teste**, mais
composição e cross-adapter verificados.

O que isso mudou na prática é que o critério deixou de ser uma frase e passou a
ser um dispositivo.

### O inventário DESCOBRE os ports; não os repete

`phase3_inventory_test.go` lê `pkg/application/contracts` com o parser do Go e
extrai os ports que carregam `txtID string` — o endereçamento de sessão, que é o
que distingue operar sobre uma sessão de infraestrutura como `Storage` ou
`Logger`. Uma lista escrita à mão só provaria que alguém a escreveu à mão.

Ele falha nas TRÊS direções, e os três controles negativos foram executados:

```
port de transporte NOVO sem classificação
  → port de transporte "PortNovoDeTeste" não está no inventário da Fase 3

recusa sem motivo escrito
  → port "BlocklistManager" não satisfeito e SEM motivo escrito

tabela que ficou para trás do código
  → o inventário classifica "PortQueNaoExiste", que já não é um port de transporte
```

### Cross-adapter: a premissa da 71 deixou de ser esperança

`crossadapter_test.go` é o único lugar do repositório que enxerga os DOIS
adaptadores ao mesmo tempo, e é de propósito. A premissa da decisão 71 é que eles
são transportes alternativos para a mesma intenção de produto — e uma premissa
que ninguém verifica é uma esperança.

Ele trava três coisas:

1. os dois satisfazem `ChatArchiver`, então um caso de uso aceita qualquer um
   sem saber qual está atrás;
2. o socket satisfaz `ChatOperations` inteira e a página **não** — a assimetria
   real, e não uma versão dela escrita à mão;
3. as duas grafias de JID **divergem** e **significam o mesmo**. Se algum dia
   convergirem, `ToPageJID` vira código morto que ninguém sabe remover — e o
   teste falha dizendo isso, em vez de o deixar apodrecer.

### Estado

`ChatArchiver` satisfeito. `SessionGuard` embutido. `UnavailableMessageRequester`
recusado por natureza, com motivo no inventário. **Restam 15**, cada um agora
com uma linha que falha até ser resolvida.

## PresenceAnnouncer — e a SEGUNDA categoria de recusa

Segundo port de transporte satisfeito, e sem precisar de decisão nova: o padrão
da 80 já cobria o caso, e a evidência veio do LEDGER em vez de discussão.

`PresenceController` quebrou em `PresenceAnnouncer` (anunciar o que ESTA sessão
faz) e `PresenceSubscriber` (pedir as atualizações de OUTRA pessoa). Os três
casos de uso passaram a pedir a metade que usam.

### As duas recusas não são a mesma coisa, e a distinção é informação

| port | recusa | o que significa |
| --- | --- | --- |
| `UnavailableMessageRequester` | **ausência de sentido** | pede reenvio de mensagem que não pôde ser DECIFRADA; quem dirige a página não decifra nada. Nunca vai ser implementado. |
| `PresenceSubscriber` | **dependência humana MEDIDA** | a H144 pôs as duas contas acordadas e a assinatura nunca chegou a `subscribed` em 45 s, com `isMyContact:false isAddressBookContact:false`. O vínculo de agenda cria-se NO TELEFONE. Passa a funcionar no dia em que um humano salvar o contato. |

Tratá-las como "pendente" perderia essa diferença — e convidaria alguém a gastar
uma sessão a tentar contornar a segunda, que é exatamente o que o LEDGER existe
para impedir.

O teste que trava a segunda faz mais do que falhar: ele **encaminha**. A mensagem
diz que, se a assinatura passou a funcionar, *a H144 precisa de ser revista
primeiro* — em vez de deixar alguém apagar o teste por o achar desatualizado.

### Detalhe do adaptador que vale a regra

O port aceita `state` como string LIVRE, porque o upstream nunca a validou.
Repassá-la transformaria um typo do chamador numa chamada a função de página que
não existe, então o adaptador mapeia contra um conjunto FECHADO — e aceita
também o vocabulário do socket (`typing`, `stopped`, `audio`), para que um
chamador escrito contra o outro transporte não quebre ao trocar.

**Estado: 2 de 18 ports satisfeitos, 2 recusados com motivo medido, 14 restantes.**

## BlocklistManager — e a TERCEIRA categoria: assimetria de DADO

Terceiro port satisfeito, e o primeiro **por inteiro** — os três verbos estão
`PROVEN` no LEDGER (block e unblock na H59, a leitura na H146). Nenhuma
assimetria de capacidade.

Mas apareceu outra coisa, que não é capacidade e sim **dado**:

O socket versiona a blocklist com um `DHash` que o servidor manda. A página não
expõe equivalente — entrega a coleção, não a versão dela. Preencher com um valor
inventado seria dado bem-formado e FALSO, e um chamador que comparasse dois deles
concluiria "não mudou" a partir de duas listas diferentes.

Vazio é a resposta honesta. O problema é que vazio **parece esquecimento**, então
virou constante nomeada com o motivo E um teste — sem ele, alguém lê o `""` como
lacuna e "corrige". A mensagem do teste encaminha: *se a página passou a expor uma
versão, isso é mudança de capacidade e precisa de medição, não de um valor novo
aqui*.

### As três categorias de divergência, agora nomeadas

| categoria | exemplo | o que fazer |
| --- | --- | --- |
| **capacidade sem sentido** | `UnavailableMessageRequester` | recusar o port; nunca vai existir |
| **capacidade bloqueada por humano** | `PresenceSubscriber` (H144) | recusar o port com a medição; volta quando alguém agir no telefone |
| **dado ausente** | `DHash` da blocklist | valor honesto (vazio), constante nomeada, teste que impede o preenchimento |

A terceira é a mais fácil de errar, porque o compilador aceita qualquer valor.

**Estado: 3 de 18 ports satisfeitos, 2 recusados com motivo medido, 13 restantes.**
