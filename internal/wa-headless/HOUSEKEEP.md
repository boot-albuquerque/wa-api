# HOUSEKEEP — wa-headless (biblioteca vendorizada)

Achados do módulo de protocolo em `internal/wa-headless/`, o fork vendorizado.

Achados da aplicação moram na raiz: `HOUSEKEEP.md`. A separação não é
organizacional — o que está aqui acompanha o upstream e é candidato a virar
patch ou a sumir num rebase; o que está lá é nosso e só nós corrigimos.

Um achado que atravessa a fronteira fica no arquivo de quem CAUSA o problema,
com referência cruzada no outro.

O formato de cada entrada, e a política anti-regressão que rege a passagem
para "corrigido", estão em `CLAUDE.md` / `AGENTS.md`.

> **Nota de procedência (2026-08-08):** este arquivo nasceu da divisão do
> antigo `internal/wa-noise/HOUSEKEEP.md`, que registrava o repositório
> inteiro. O índice que ele mantinha no topo foi descartado na divisão — ele
> já trazia uma correção admitindo estar desatualizado em relação às próprias
> entradas, e um índice que mente é pior que a ausência dele. As entradas
> vieram integralmente; nenhuma foi perdida.

## Convenção de status

Toda entrada termina com um `**Status**:` cujo **veredito vem em negrito**,
para que uma varredura mecânica o encontre. Ele pode estar no início da linha
ou como item de lista (`- **Status**: ...`) — os dois layouts convivem no
arquivo, e **uma varredura tem de aceitar os dois**:

- `**Status**: **corrigido**` — com os testes que o travam e o controle
  negativo executado (ver a política anti-regressão em `CLAUDE.md`).
- `**Status**: **não corrigido**` — seguido do motivo.
- `**Status**: **fechado — não corrigir**` — decisão registrada, não pendência.
- `**Status**: **parcialmente corrigido**` — com o que ficou aberto e por quê.

O formato importa, e a varredura também: em 2026-08-08 três varreduras
seguidas minhas erraram — uma leu cinco entradas como "sem status" porque o
veredito estava em texto simples, outra perdeu quatro porque o `**Status**`
era item de lista. Em todos os casos **o documento estava certo e o método
errado**, e eu quase "corrigi" entradas íntegras.

Entradas com MAIS de um `**Status**` são legítimas: o achado tem sub-itens
com desfechos diferentes (ver F29, F49, F69). O veredito que vale é o do
sub-item; não existe um status único para elas.

Seções que são **nota** e não achado — evidência nova para entradas
existentes, observação de acompanhamento — não levam status. Dê a elas um
título que diga isso ("Nota sobre…", "Evidências novas…"), para que a
varredura as distinga de um achado que esqueceu o status.

Referências entre entradas são por **título**, nunca por número de linha —
ver a F61, cuja própria referência ficou obsoleta quando estes arquivos
foram divididos.
