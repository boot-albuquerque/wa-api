# wa-api — instruções do projeto

## Registro de achados incidentais (dois HOUSEKEEP)

Sempre que, durante uma sessão de trabalho, você encontrar um bug, gap,
comportamento incorreto ou dívida técnica que **não faça parte do escopo
da tarefa atual**, registre antes de encerrar a sessão — mesmo que decida
não corrigir.

São **dois** arquivos, e a escolha entre eles não é organizacional:

- **`HOUSEKEEP.md`** (raiz) — o wa-api como um todo: `pkg/`, build, gates,
  configuração, rotas HTTP. É código nosso, e só nós corrigimos.
- **`internal/wa-noise/HOUSEKEEP.md`** — a biblioteca vendorizada. Acompanha
  o upstream, e um achado ali é candidato a virar patch ou a sumir num
  rebase.

O achado fica no arquivo de quem **causa** o problema, com referência cruzada
no outro quando atravessar a fronteira.

Cada entrada deve conter, no mínimo:
- **Data** e **contexto**: em que tarefa/feature o achado surgiu.
- **Onde**: arquivo(s) e linha(s) exatos (`caminho/arquivo.go:NN`), com
  trecho de código relevante quando ajudar a localizar.
- **Problema**: descrição concreta do que está errado, com evidência (ex:
  comando que reproduz, saída observada vs esperada).
- **Correção sugerida**: caminho de fix, mesmo que não vá ser aplicado
  agora.
- **Status**: corrigido nesta sessão / não corrigido (e por quê).

Não registre o que já está coberto por ADRs, planos aprovados, ou que é
claramente escopo de uma tarefa em andamento — o HOUSEKEEP é para o que
foi descoberto de lado, não para o trabalho principal.

Não "corrija de graça" bugs pré-existentes fora do escopo da tarefa atual
sem perguntar primeiro — registre no HOUSEKEEP certo e pergunte ao usuário
se quer que a correção seja feita agora ou fique pendente.

## Consultar as implementações de referência antes de resolver

Antes de projetar solução para qualquer problema de protocolo WhatsApp —
identidade LID/PN, roster, avatar, histórico, presença, envio, pareamento —
**consulte primeiro como estes dois projetos resolveram**:

- **Baileys** (`https://github.com/whiskeysockets/Baileys`) — implementação
  TypeScript do protocolo. É a referência mais próxima do wire: o que ele
  faz costuma ser o que o WhatsApp exige, não uma escolha de design.
- **Evolution API** (`https://github.com/evolution-foundation/evolution-api`)
  — API HTTP sobre Baileys, com os mesmos problemas de produto que este
  projeto tem (listar conversas, nomear contatos, servir avatar).
- **whatsapp-web.js** (`https://github.com/pedroslopez/whatsapp-web.js`) — a
  referência que **dirige a mesma SPA que nós**. Para qualquer problema de
  módulo, seletor ou API de página, é a mais próxima do nosso terreno: quando
  o Baileys fala protocolo e nós falamos DOM, é o wwebjs que já esteve
  exatamente onde estamos.

  Consulte-o especialmente para: **nomes de módulo do `window.require`**,
  ordem de chamadas dentro da página, e o que a página exige antes de uma
  operação. E leia as **issues abertas** dele como fonte de primeira classe —
  elas dizem o que está QUEBRADO hoje, que é informação que o código não dá.

### A resposta NEGATIVA também é informação

Regra aprendida em 2026-08-20, no `sendText` (HOUSEKEEP H34):

O envio parou em `No LID for user`. A pergunta natural — *"como o wwebjs
resolve?"* — teve resposta **negativa**: ele **não resolve**. São issues abertas
de set/2025 a jan/2026 (`#3834`, `#5750`), morrendo no mesmo
`findOrCreateLatestChat → toUserLidOrThrow`.

**Isso não é um beco: é um dado.** Significa que copiar o caminho da referência
teria falhado mesmo se tivesse sido copiado, e que a solução tem de vir de outro
lugar. Veio: o Baileys, que fala protocolo, mantém um `LIDMappingStore` com
fallback para **USync**. Nós dirigimos a SPA, então não precisamos do store — só
precisamos **chamar a resolução que a página já tem**, e ela está no
`WAWebQueryExistsJob.queryWidExists`, que o wwebjs usa no `getNumberId` e **não**
antes de enviar. Fazer isso ANTES é a diferença.

Ou seja: **as três referências foram necessárias, e nenhuma sozinha bastava.**
O wwebjs deu o nome do módulo, o Baileys deu o entendimento do que é resolver
identidade, e a issue aberta do wwebjs deu a informação de que aquele caminho
está quebrado. Registre sempre qual referência respondeu o quê — inclusive quando
a resposta for "esta aqui não resolve".

**Quatro módulos que a lista do wwebjs usaria não existem no nosso build**
(`WAWebSendMsg`, `WAWebMsgSend`, `WAWebSendMessage`, `WAWebComposeMessage`).
Copiar a lista falharia em quatro de quatro — o que é a própria razão de a regra
dizer para copiar o ENTENDIMENTO e medir o resto.

Os dois já erraram nos caminhos que estamos prestes a percorrer. Ler o
código atual não basta: **os históricos de commits e as issues são onde a
informação está**, porque explicam por que a solução tem a forma que tem.
Procure especificamente:

- **Issues fechadas** com o sintoma que estamos vendo — costumam trazer a
  causa e o descarte de hipóteses erradas.
- **Commits que revertem ou reescrevem** uma área: a mensagem do segundo
  commit diz o que a primeira tentativa quebrou.
- **Comentários no código** citando comportamento do servidor do WhatsApp —
  são observações de campo que não estão documentadas em lugar nenhum.

Regra prática: se a nossa solução divergir da deles, isso é aceitável, mas
tem de ser **decisão consciente e registrada** (ADR ou HOUSEKEEP) dizendo o
que eles fazem, o que nós fazemos e por que a diferença. Divergir sem saber
que se está divergindo é como este projeto reescreve a roda — e reescreve
os bugs deles junto.

Não copie código: as licenças e a arquitetura são outras. Copie o
ENTENDIMENTO.

## Política anti-regressão para achados do HOUSEKEEP

Todo achado do `HOUSEKEEP.md` que for **corrigido** precisa sair da sessão
com teste que o trave. A entrada só muda para "corrigido" quando isto
estiver feito, e a entrada diz **quais** testes o cobrem.

O mínimo, para qualquer achado:

1. **Teste do defeito**: reproduz a condição exata que foi medida em campo,
   com os valores observados quando houver. Não uma aproximação.
2. **Controle negativo EXECUTADO**: reintroduza o defeito e confirme que o
   teste falha, colando a saída da falha na entrada do HOUSEKEEP. Um teste
   que passa mas não morde é pior que nenhum — dá confiança falsa.
3. **Teste da causa, não só do sintoma**: se o sintoma tem mais de uma
   causa possível, trave a causa. Silenciar o sintoma faria o teste passar
   com o defeito no lugar.

Acrescente, conforme o achado exigir:

- **Ordem e efeito colateral**: quando a correção depende de uma sequência
  (agir só depois de validar, não soltar recurso após falha), teste a
  ORDEM. Inverter chamadas costuma passar em todos os outros testes.
- **Caminho pelo roteador**: defeito de rota se testa pela rota registrada,
  não pelo handler cru. Um handler montado sem padrão de rota não exercita
  extração de parâmetro — foi assim que a F81 sobreviveu.
- **Fronteira medida contra a produção**: dublê mais permissivo que a
  implementação real esconde defeito em vez de revelá-lo. Quando o dublê
  imita uma regra (parsing, normalização, resolução), ele tem de imitar a
  regra REAL, e o comentário do dublê deve dizer de onde ela vem.
- **Fronteira medida contra a produção**: dublê DIVERGENTE da implementação
  real esconde defeito em vez de revelá-lo — ou, quando é mais SIMPLES que ela,
  abençoa código morto. Quando o dublê imita uma regra (parsing, normalização,
  resolução), ele tem de imitar a regra REAL, e o comentário do dublê deve
  dizer de onde ela vem.
- **Contrato de resposta**: envelope, status e forma do corpo quando o
  achado for de fronteira HTTP.
- **Determinismo**: quando houver ordenação, mapa ou concorrência, rode a
  asserção várias vezes — ordem de mapa em Go é aleatória por desenho.

A verificação em produção NÃO substitui o teste: ela prova que funciona
hoje, o teste impede que pare de funcionar amanhã. Quando as duas existirem,
registre as duas na entrada.

### O caso especial: regressão introduzida pela PRÓPRIA correção

A política acima protege o defeito que você foi consertar. Não protege o que o
conserto quebra — e essa foi a falha real de 2026-08-08.

O pool de despacho da F86 foi medido, testado sob `-race`, teve três controles
negativos e passou no `make check`. Ainda assim tornou um cenário
**estritamente pior**: com os padrões de retry que já existiam (5 tentativas,
base 30s, exponencial), a espera do backoff dormia dentro de um worker e um
único evento para um webhook morto segurava um slot por 7,5 minutos. Dois
pareamentos simultâneos saturavam o pool inteiro e paravam também WebSocket,
webhook global e RabbitMQ — canais que nada tinham a ver com o destino
quebrado. Antes do pool, as mesmas esperas eram goroutines soltas dormindo:
feio e inofensivo.

Nada disso apareceu na análise nem na medição, porque **a medição comparou os
cenários em que o mecanismo ajuda**.

**Regra 1 — inventário de detentores.** Toda mudança que converte um recurso
ilimitado em LIMITADO (pool, semáforo, fila, teto, rate limit, pool de
conexões) tem de enumerar, na entrada do HOUSEKEEP, tudo que passa a disputar
esse recurso e o PIOR CASO de ocupação de cada um. Detentor que possa segurar
por tempo longo ou indeterminado é bloqueio, não observação.

**Regra 2 — medir onde deveria PIORAR.** Para todo mecanismo, meça o cenário
em que ele cobra o preço, não só aquele em que ele paga. Se a medição só tem
pernas onde o mecanismo ganha, ela não mediu o mecanismo — mediu a hipótese.
Pergunta que faz o cenário aparecer: *qual entrada faz esta proteção virar o
problema?*

**Regra 3 — a invariante, escrita e travada em teste.** Quando o recurso for
limitado, enuncie a invariante em vez de redescobri-la a cada camada nova.
A deste projeto é:

> Nada que espere por relógio ou por par morto pode ocupar slot limitado.

Com ela escrita, o próximo padrão de resiliência (circuit breaker, bulkhead,
rate limiter) é **auditado contra uma regra**, em vez de ter as interações
descobertas por acidente. Sem ela, cada camada nova custa uma sessão inteira
de investigação.

**Regra 4 — o conserto do conserto também é um mecanismo.** Aplique as três
regras acima a ele. O conserto da F88 trocou `Sleep` por `time.AfterFunc`; um
timer não custa goroutine, mas mantém o payload vivo — trocar "goroutine
dormindo" por "timer pendente" só mudaria ONDE a memória cresce sem limite.
Por isso o conjunto de pendentes nasceu com teto por BYTES, igual ao do pool.

## Idioma do código: identificadores e comentários em inglês (EN-US)

**Regra**, válida para todo código novo e para todo código tocado:

| o quê | idioma |
|---|---|
| identificadores (variável, função, tipo, campo, constante) | **inglês** |
| comentários de código | **inglês** |
| nomes de arquivo e de diretório | **inglês** |
| mensagens de log e de erro | **inglês** |
| documentos do repositório (`HOUSEKEEP.md`, `ARMADILHAS.md`, ADRs) | português — são registro de decisão, não código |
| mensagens de commit | português — idem |

O motivo não é estética: identificador em português obriga quem lê a alternar
de idioma no meio de uma expressão que já mistura palavras-chave em inglês
(`for`, `range`, `err`), e nomes como `renovarUma` ao lado de `RowsAffected`
tornam a leitura mais lenta para qualquer pessoa, inclusive quem fala
português.

**Zero string literal solta.** Toda string que carrega significado — nome de
tabela, tipo de banco, chave de configuração, rótulo de goroutine — vira
constante nomeada no pacote. Literal repetido em dois lugares é o mesmo bug
esperando divergir. Isto já era pilar do projeto (ADR-0004) e passa a ser
verificável: se você escreveu `"postgres"` duas vezes, extraia.

**Ao TOCAR num arquivo antigo**: converta o que você mexeu, não o arquivo
inteiro. Conversão em massa mistura renomeação com mudança de comportamento no
mesmo diff, e aí a revisão não consegue separar as duas — é exatamente o tipo
de mudança em que um defeito passa despercebido.

## Armadilhas conhecidas — leia `ARMADILHAS.md`

`ARMADILHAS.md` (raiz) cataloga defeitos que **já passaram por revisão e por
testes verdes** neste repositório, com a evidência medida de cada um. Leia
antes de mexer em identidade LID/PN, rotas HTTP, junção de dados ou escrita
no banco.

As quatro que mais custaram, resumidas aqui porque valem para toda tarefa:

1. **Dublê mais permissivo que a produção esconde o defeito.** Quando um
   dublê imita uma REGRA (parsing, normalização, resolução), ele tem de
   imitar a regra REAL e citar de onde ela vem, com caminho de arquivo. Se o
   dublê e a produção nunca divergem, o teste não está medindo a regra.
1. **Dublê DIVERGENTE da produção.** Quando um dublê imita uma REGRA
   (parsing, normalização, resolução), ele tem de imitar a regra REAL e citar
   de onde ela vem, com caminho de arquivo. Se o dublê e a produção nunca
   divergem, o teste não está medindo a regra.
   **Divergir não é só ser mais permissivo**: um dublê mais SIMPLES que a
   produção não esconde defeito — ele ABENÇOA CÓDIGO MORTO, e passa por teste,
   controlo negativo, `make check` e verificação em campo. Quando o objeto
   atravessa uma transformação no caminho real (desembrulho, normalização,
   hidratação), o dublê tem de atravessá-la também.

2. **Teste o caminho de SUCESSO, não só a recusa.** Três defeitos deste repo
   viviam atrás de suítes que só exercitavam a guarda. E teste rota pela
   ROTA REGISTRADA — o router é `gorilla/mux`, então `r.PathValue` não
   funciona; use `mux.Vars`.

3. **Controle negativo que não compila não prova nada.** Se a mutação
   quebrar o build em vez de produzir uma falha de teste com mensagem, ela
   não valeu — ajuste até compilar E falhar.

4. **Medição em produção não é opcional para defeito de dado.** Os dois
   piores casos de 2026-08-08 passaram por revisão, testes e `make check`, e
   só apareceram contra dados reais. Registre a linha de base ANTES de
   mexer: sem ela, o "depois" não significa nada.

Quando encontrar uma armadilha nova, acrescente ao catálogo com a evidência
— é o que o torna útil em vez de genérico.

## Medir antes de projetar — especulação não entra no plano

Nenhum plano de correção sai de suposição sobre como o sistema se comporta.
Sai de **teste prático que simula a vida real**, e os números dele é que
viram o plano.

O teste desta regra é simples: **se a medição só confirmou o que você já
achava, provavelmente ela não mediu nada.** Uma medição útil produz pelo
menos um "eu não teria adivinhado".

Três exemplos reais de 2026-08-08, todos do mesmo dia e todos invisíveis a
qualquer raciocínio de escrivaninha:

1. **A amplificação era ~5×, não 1×.** 800 entregas viraram ~4.000
   goroutines — o transporte HTTP cria goroutines internas por conexão. A
   análise estática tinha contado "4 goroutines por evento".
2. **O primeiro harness mediu a coisa errada e teria invertido a decisão.**
   Com `time.Sleep` o heap era idêntico com e sem teto (4,7MB), o que
   sugeria "não faça nada". `Sleep` não aloca; com HTTP real foram 32MB
   contra 13MB. **O instrumento precisa alocar, bloquear e falhar como o
   original.**
3. **O limitador travou o próprio teste.** A aquisição bloqueia o chamador —
   propriedade documentada por mim e não internalizada. Em produção o
   chamador é o handler de eventos do SDK, então o "remédio" empurrava o
   problema para um lugar pior. Só apareceu porque um teste deadlockou.

**Como fazer:**

- **Simule o caminho real, não uma caricatura dele.** E/S de rede se mede com
  E/S de rede; alocação, com payload do tamanho verdadeiro. Se o dublê não
  aloca nem bloqueia como o original, ele mede outra coisa.
- **Compare no mesmo instante e na mesma máquina.** Alterne a variável dentro
  de uma execução; comparar execuções separadas mede também o estado da
  máquina.
- **Repita.** Uma amostra não distingue efeito de ruído — três rodadas já
  mostram se os números são estáveis.
- **Meça o que você agiria a respeito**: pico (não média), memória, latência
  induzida, e a saturação do próprio mecanismo.
- **Registre o ANTES** antes de mexer. Sem linha de base, o depois não
  significa nada.

Quando a medição contrariar a hipótese, **a hipótese cai** — inclusive se
ela já estiver escrita num HOUSEKEEP com número. Corrija a entrada; um
achado com diagnóstico errado é pior que nenhum, porque parece resolvido.
