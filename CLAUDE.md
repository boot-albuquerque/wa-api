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
- **Contrato de resposta**: envelope, status e forma do corpo quando o
  achado for de fronteira HTTP.
- **Determinismo**: quando houver ordenação, mapa ou concorrência, rode a
  asserção várias vezes — ordem de mapa em Go é aleatória por desenho.

A verificação em produção NÃO substitui o teste: ela prova que funciona
hoje, o teste impede que pare de funcionar amanhã. Quando as duas existirem,
registre as duas na entrada.

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
