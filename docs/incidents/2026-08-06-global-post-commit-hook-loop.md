# Incidente: loop no hook global `post-commit` (fora do repo `wa-api`, mas o disparou)

- **Data**: 2026-08-06
- **Onde o bug mora**: `~/.config/git/hooks/post-commit` (hook **global**,
  `core.hooksPath` aponta pra lá — afeta **todo repositório da máquina**,
  não é nada dentro de `wa-api`).
- **Onde o dano apareceu**: worktree `wa-api-lid-resolution`
  (`feature/contacts-last-activity-lid-resolution`), durante um `git commit`
  normal.
- **Impacto real**: 213 commits de lixo (`activity: [GitHub] ...`) na branch
  local + arquivo `log.md` commitado dentro do repo `wa-api` (nunca deveria
  estar lá). **Nenhum push malicioso chegou ao GitHub** — confirmado com
  `git fetch origin main` logo depois: `origin/main` ficou exatamente onde
  estava antes do incidente.
- **Ação tomada**: `git reset --hard` na branch local pro último commit
  legítimo (o hook não toca em `main`, só na branch corrente do worktree
  onde ele disparou — nada precisou ser revertido em `develop`/`main`). O
  hook em si **não foi alterado** — está fora do escopo deste repo, fica
  registrado aqui pra quem for mexer nele.

## O que o hook faz (comportamento pretendido)

Depois de qualquer commit em qualquer repo da máquina, ele registra uma
linha de log num repositório "espelho" central:

```bash
REPO_DIR="$HOME/.git-mirror/activity-log"
LOG_FILE="$REPO_DIR/log.md"
...
# Ignorar commits dentro do próprio activity-log para evitar loop
if echo "$REMOTE_URL" | grep -qi "activity-log"; then
  exit 0
fi

git -C "$REPO_DIR" pull --quiet --rebase 2>/dev/null || true

echo "| $DATE | $HOSTNAME | $PLATFORM | \`$REPO_NAME\` | \`$BRANCH\` | $MSG |" >> "$LOG_FILE"

cd "$REPO_DIR"
git add log.md
git commit --quiet -m "activity: [$PLATFORM] $REPO_NAME/$BRANCH — $MSG"
git push --quiet origin main 2>/dev/null || true
```

## A causa raiz

### Hipótese inicial (parcialmente errada)

A primeira leitura do incidente apontou o `cd "$REPO_DIR"` **sem checagem de
falha** (sem `set -e`, sem `|| exit`, sem verificar `$?`) como a causa: se o
`cd` falhasse, o script continuaria rodando no diretório do commit original.
Isso levou a uma primeira correção — trocar `cd` + comandos relativos por
`git -C "$REPO_DIR" add/commit/push` — que **não resolveu o problema**: o
loop reproduziu de novo, idêntico, mesmo com `git -C` em todo lugar.

### Causa raiz real: vazamento de variáveis de ambiente do git

Hooks (principalmente em **worktrees**) rodam com `GIT_DIR`, `GIT_WORK_TREE`,
`GIT_INDEX_FILE` (e às vezes `GIT_COMMON_DIR`, `GIT_OBJECT_DIRECTORY`,
`GIT_PREFIX`) **exportados no ambiente** pelo processo `git commit` que
disparou o hook, apontando pro repositório/worktree onde o commit aconteceu.

Essas variáveis têm **prioridade sobre `git -C <dir>`** — `-C` só troca o
diretório de trabalho do processo `git`, mas se `GIT_DIR` já está no
ambiente, o git usa esse `GIT_DIR` independente do `-C`. Ou seja:

```bash
git -C "$REPO_DIR" add log.md          # GIT_DIR ainda aponta pro repo original
git -C "$REPO_DIR" commit --quiet ...   # commita no repo ERRADO mesmo com -C
git -C "$REPO_DIR" push --quiet ...     # push também mira o remote errado
```

continua secretamente operando no repo onde o commit original rodou — o
mesmo bug de sempre, só que sobrevivendo à correção ingênua de trocar `cd`
por `-C`.

Esse commit "errado" **também dispara o `post-commit`** (todo commit
dispara), e o guard de "evitar loop"
(`if echo "$REMOTE_URL" | grep -qi "activity-log"`) **não protege esse
caso**: ele checa o remote `origin` do repo atual, que continua sendo o do
repo original (ex.: `wa-api`), não o do mirror. Como a condição de saída
nunca fica verdadeira, o loop continua até alguma interrupção externa.

### Fator agravante encontrado durante o teste

No momento do teste (2026-08-06), `~/.git-mirror/activity-log` **não tinha
mais `.git`** — o repositório mirror tinha sido perdido/corrompido em algum
momento anterior (causa não determinada). Isso não é a causa raiz do loop,
mas confirma que o hook precisa validar a saúde do mirror antes de operar
nele, e não assumir que ele existe e está íntegro.

## Por que não foi pior

- O `git push --quiet origin main` dentro do loop tentava empurrar a branch
  local `main` do `wa-api` (não a branch que estava de fato commitando o
  lixo) pro `origin` do `wa-api`. Como essa `main` local nunca foi tocada
  pelo loop (só a branch corrente do worktree recebeu os commits de lixo), o
  push não tinha nada de novo pra mandar, ou falhava por non-fast-forward —
  de qualquer forma, o `2>/dev/null || true` engoliu o resultado. Confirmado
  via `git fetch origin main` que `origin/main` no GitHub ficou intacto.
- O dano ficou inteiramente contido na branch/worktree local onde o commit
  original rodou — nada foi publicado, nenhuma outra branch/repo foi
  afetado.

## Correção aplicada e testada (2026-08-06)

Fora do escopo original do repo `wa-api`, mas aplicada e verificada ao vivo
porque o bug estava ativo e bloqueando qualquer `git commit` na máquina.
Alterações em `~/.config/git/hooks/post-commit`:

1. **Isolar o ambiente do git antes de tocar no mirror** — a correção que
   efetivamente resolve o loop:
   ```bash
   unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_COMMON_DIR \
         GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX
   ```
2. **Guarda dura pós-unset**: valida que `$REPO_DIR` é de fato a raiz de um
   repositório git válido antes de prosseguir, ao invés de assumir que
   existe e está íntegro:
   ```bash
   MIRROR_TOPLEVEL=$(git -C "$REPO_DIR" rev-parse --show-toplevel 2>/dev/null)
   if [ -z "$MIRROR_TOPLEVEL" ] || [ "$MIRROR_TOPLEVEL" != "$REPO_DIR" ]; then
     echo "post-commit: $REPO_DIR não é um repositório git válido — pulando log de atividade (sem afetar o commit atual)" >&2
     exit 0
   fi
   ```
3. Todos os comandos no mirror usam `git -C "$REPO_DIR" ...` (sem `cd`) —
   agora funciona de verdade porque o ambiente já não está vazando `GIT_DIR`.

### Verificação

Testado ao vivo, duas vezes, com watchdog matando o processo se passasse de
~10s: ambas terminaram em **2 segundos**, sem loop, sem processos git
residuais, com o aviso correto no stderr (mirror sem `.git`, hook pulou o
log sem afetar o commit). `ps aux` confirmou zero processos `git
commit`/`add`/`push` remanescentes após cada teste.

**Pendência separada, não bloqueante**: `~/.git-mirror/activity-log` segue
sem `.git` — o log de atividade central está desativado até alguém recriar
esse repositório lá (`git init` + remote). Isso não afeta mais a segurança
de `git commit` em nenhum repo da máquina.

## Como reproduzir / verificar se ainda está quebrado

```bash
cd /caminho/de/qualquer/repo/com/hooksPath/global
git commit --allow-empty -m "teste"
# Verifique imediatamente:
git log --oneline -5   # se aparecer "activity: [...] ... log.md" AQUI, o bug persiste
```
