# wa-api — instruções para agentes

## Registro de achados incidentais (internal/wa-noise/HOUSEKEEP.md)

Sempre que, durante uma sessão de trabalho, você encontrar um bug, gap,
comportamento incorreto ou dívida técnica que **não faça parte do escopo
da tarefa atual**, registre em `internal/wa-noise/HOUSEKEEP.md` antes de
encerrar a sessão — mesmo que decida não corrigir.

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
sem perguntar primeiro — registre no HOUSEKEEP e pergunte ao usuário
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
