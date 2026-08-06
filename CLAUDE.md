# wa-api — instruções do projeto

## Registro de achados incidentais (HOUSEKEEP.md)

Sempre que, durante uma sessão de trabalho, você encontrar um bug, gap,
comportamento incorreto ou dívida técnica que **não faça parte do escopo
da tarefa atual**, registre em `HOUSEKEEP.md` (raiz do repo) antes de
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
claramente escopo de uma tarefa em andamento — `HOUSEKEEP.md` é para o que
foi descoberto de lado, não para o trabalho principal.

Não "corrija de graça" bugs pré-existentes fora do escopo da tarefa atual
sem perguntar primeiro — registre em `HOUSEKEEP.md` e pergunte ao usuário
se quer que a correção seja feita agora ou fique pendente.
