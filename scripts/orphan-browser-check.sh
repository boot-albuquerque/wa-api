#!/usr/bin/env bash
# Falha quando ha browsers de TESTE sobreviventes de execucoes anteriores.
#
# NAO limpa. Falhar e a decisao (84): limpar em silencio esconderia o vazamento,
# que foi exatamente o erro cometido antes — a serializacao da 79 removeu a
# SATURACAO e isso foi lido como se tivesse removido o VAZAMENTO. Ele voltou a
# acumular por tres horas ate derrubar o gate.
#
# O recorte e' estrito: so' processos cujo --user-data-dir esteja sob o diretorio
# temporario de testes do Go (/var/folders/.../T/Test*). Um perfil PAREADO nunca
# casa esse padrao, e nao deve — SIGKILL contra perfil pareado arrisca corrompe-lo,
# e reparear exige um humano com o telefone.
#
# F289: o recorte por --user-data-dir sozinho nao distingue "browser que
# sobreviveu a uma execucao anterior" de "browser que uma execucao EM CURSO,
# noutro worktree, acabou de abrir" — os dois casam o mesmo padrao. Por isso
# cada candidato agora tambem verifica se o PROCESSO DONO (ppid) ainda esta
# vivo: se estiver, o browser pertence a uma suite em progresso (nao e orfao),
# mesmo que o PID do browser tenha mudado entre corridas. So' o ppid morto (ou
# 1, reparented para o init) e evidencia de vazamento real. Ver HOUSEKEEP.md
# F289 para a evidencia medida do falso positivo.
set -uo pipefail

padrao='user-data-dir=/var/folders/[^ ]*/T/Test'

# Lista os processos de browser de teste candidatos a orfao (ainda sem
# filtrar por ppid vivo/morto). Isolado em funcao para poder ser
# substituido por um dublê nos testes.
list_candidatos() {
  ps -Ao pid,ppid,etime,command 2>/dev/null \
    | grep -i "Google Chrome" \
    | grep -E "$padrao" \
    | grep -v -- "--type=" \
    | grep -v grep || true
}

# Verdadeiro (exit 0) quando o processo dono (ppid) ainda esta vivo. ppid=1
# significa reparented para o init: o dono original morreu sem o parar.
# Isolado em funcao para poder ser substituido por um dublê nos testes.
ppid_alive() {
  local ppid="$1"
  [ "$ppid" != "1" ] && ps -p "$ppid" >/dev/null 2>&1
}

main() {
  local principais orfaos n linha ppid

  principais=$(list_candidatos)

  if [ -z "$principais" ]; then
    echo "orphan-browser-check: nenhum browser de teste sobrevivente."
    return 0
  fi

  orfaos=""
  while IFS= read -r linha; do
    [ -z "$linha" ] && continue
    ppid=$(printf '%s\n' "$linha" | awk '{print $2}')
    if ! ppid_alive "$ppid"; then
      orfaos="${orfaos}${linha}
"
    fi
  done <<EOF_CANDIDATOS
$principais
EOF_CANDIDATOS

  if [ -z "$orfaos" ]; then
    echo "orphan-browser-check: nenhum browser de teste orfao (dono ainda vivo noutra execucao)."
    return 0
  fi

  n=$(printf '%s\n' "$orfaos" | grep -c .)
  echo "orphan-browser-check: $n browser(s) de teste SOBREVIVERAM a execucoes anteriores." >&2
  printf '%s\n' "$orfaos" | awk '{printf "  pid=%s ppid=%s idade=%s\n", $1, $2, $3}' >&2
  cat >&2 <<'AVISO'

  Cada um segura memoria e CPU e torna o proximo arranque mais lento, ate um
  teste estourar o prazo por uma razao que nada tem a ver com o codigo (F103).

  ppid=1 significa que o processo DONO morreu antes de o parar: nesse caso nao
  houve CleanStop a correr, e a escalada nao cobre — so' esta varredura cobre.
  Um browser cujo ppid ainda esta vivo NAO aparece aqui (F289): pertence a
  uma execucao noutro worktree, e matá-lo sabotaria esse trabalho.

  Para limpar, com o recorte de perfil TEMPORARIO ja aplicado:
    make orphan-browser-clean
AVISO
  return 1
}

# Permite que o teste faça `source` do script sem disparar main() — só
# executa quando invocado diretamente.
if [ "${BASH_SOURCE[0]:-$0}" = "$0" ]; then
  main
  exit $?
fi
