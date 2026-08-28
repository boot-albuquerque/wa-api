#!/usr/bin/env bash
# Testes do orphan-browser-check.sh (F289).
#
# Exercita os DOIS lados exigidos pelo HOUSEKEEP.md F289: pai morto (acusa
# orfao, exit 1) e pai vivo (nao acusa, exit 0) -- e o segundo lado e o que
# faltava antes da correcao, porque o script so tinha sido validado no caso
# em que deve falhar.
#
# Uso:
#   ./scripts/orphan-browser-check_test.sh
#
# Sai com 0 se todos os casos passarem, 1 caso contrario.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ALVO="$SCRIPT_DIR/orphan-browser-check.sh"

falhas=0

assert_contains() {
  local rotulo="$1" agulha="$2" palheiro="$3"
  if ! printf '%s' "$palheiro" | grep -qF -- "$agulha"; then
    echo "FALHA [$rotulo]: esperava conter '$agulha', saida foi:"
    echo "$palheiro" | sed 's/^/    /'
    falhas=$((falhas + 1))
    return 1
  fi
  return 0
}

assert_eq() {
  local rotulo="$1" esperado="$2" obtido="$3"
  if [ "$esperado" != "$obtido" ]; then
    echo "FALHA [$rotulo]: esperava '$esperado', obteve '$obtido'"
    falhas=$((falhas + 1))
    return 1
  fi
  return 0
}

linha_candidata='32481 32190 00:16 /Applications/Google Chrome.app/Contents/MacOS/Google Chrome --user-data-dir=/var/folders/xx/T/Test123/profile'

# --- Caso 1: ppid MORTO -> deve acusar orfao (exit 1) -----------------------
teste_ppid_morto() {
  # shellcheck source=orphan-browser-check.sh
  source "$ALVO"

  list_candidatos() { printf '%s\n' "$linha_candidata"; }
  ppid_alive() { return 1; } # dono morto

  local saida status
  saida=$(main 2>&1)
  status=$?

  assert_eq "ppid-morto/exit" "1" "$status"
  assert_contains "ppid-morto/mensagem" "SOBREVIVERAM" "$saida"
  assert_contains "ppid-morto/pid-reportado" "pid=32481 ppid=32190" "$saida"
}

# --- Caso 2: ppid VIVO -> NAO deve acusar orfao (exit 0) ---------------------
# Este e o caso que a F289 mediu como falso positivo: outro worktree com um
# `go test` em curso, mesmo ppid, PID de browser novo a cada corrida.
teste_ppid_vivo() {
  # shellcheck source=orphan-browser-check.sh
  source "$ALVO"

  list_candidatos() { printf '%s\n' "$linha_candidata"; }
  ppid_alive() { return 0; } # dono vivo (suite de outro worktree em curso)

  local saida status
  saida=$(main 2>&1)
  status=$?

  assert_eq "ppid-vivo/exit" "0" "$status"
  assert_contains "ppid-vivo/mensagem" "dono ainda vivo" "$saida"
}

# --- Caso 3: nenhum candidato -> exit 0, mensagem de "nenhum sobrevivente" --
teste_sem_candidatos() {
  # shellcheck source=orphan-browser-check.sh
  source "$ALVO"

  list_candidatos() { printf ''; }

  local saida status
  saida=$(main 2>&1)
  status=$?

  assert_eq "sem-candidatos/exit" "0" "$status"
  assert_contains "sem-candidatos/mensagem" "nenhum browser de teste sobrevivente" "$saida"
}

teste_ppid_morto
teste_ppid_vivo
teste_sem_candidatos

if [ "$falhas" -eq 0 ]; then
  echo "orphan-browser-check_test.sh: todos os casos passaram."
  exit 0
fi

echo "orphan-browser-check_test.sh: $falhas caso(s) falharam."
exit 1
