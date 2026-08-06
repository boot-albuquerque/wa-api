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

`cd "$REPO_DIR"` **não tem checagem de falha** (sem `set -e`, sem `|| exit`,
sem verificar `$?`). Se esse `cd` falhar por qualquer motivo (o mirror não
existir nessa máquina, permissão, o worktree confundir a resolução de
caminho — não determinei a causa exata da falha do `cd` em si, só que ela
aconteceu), o script **continua rodando no diretório onde o commit original
aconteceu** — ou seja, dentro do próprio `wa-api` — e faz:

```bash
git add log.md          # cria log.md DENTRO do wa-api
git commit --quiet -m "activity: ..."   # commita no repo errado
git push --quiet origin main            # tenta empurrar pro remote ERRADO (o do wa-api, não o do mirror)
```

Esse commit **também dispara o `post-commit`** (todo commit dispara), e o
guard de "evitar loop" (`if echo "$REMOTE_URL" | grep -qi "activity-log"`)
**não protege esse caso**: ele checa se o remote `origin` do repo atual tem
"activity-log" no nome — mas o repo atual continua sendo `wa-api`
(`git@github.com:.../wa-api.git`), não o mirror. O guard foi desenhado pra
impedir o hook de disparar recursivamente **dentro do próprio mirror**, não
pra detectar "estou no repo errado por causa de um `cd` que falhou". Como a
condição de saída nunca fica verdadeira nesse cenário, o loop continua até
alguma interrupção externa (no nosso caso, um timeout de 2 minutos do
processo que disparou o commit original).

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

## Sugestão de correção (não aplicada — fora do escopo deste repo)

No script do hook (`~/.config/git/hooks/post-commit`):

1. Adicionar `set -e` no topo, OU checar explicitamente o resultado do `cd`:
   ```bash
   cd "$REPO_DIR" || { echo "post-commit: não foi possível entrar em $REPO_DIR, abortando" >&2; exit 1; }
   ```
2. Tornar o guard de loop robusto a esse cenário: checar `$(pwd)` (ou
   `git rev-parse --show-toplevel`) contra `$REPO_DIR` **depois** do `cd`,
   não confiar só no `remote.origin.url` do repo original.
3. Considerar `git -C "$REPO_DIR" add/commit/push` (com `-C`, sem depender
   de `cd` ter funcionado) em vez de `cd` + comandos relativos — mais barato
   de auditar e não tem esse modo de falha silencioso.

## Como reproduzir / verificar se ainda está quebrado

```bash
cd /caminho/de/qualquer/repo/com/hooksPath/global
git commit --allow-empty -m "teste"
# Verifique imediatamente:
git log --oneline -5   # se aparecer "activity: [...] ... log.md" AQUI, o bug persiste
```
