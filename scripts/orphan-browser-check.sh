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
set -uo pipefail

padrao='user-data-dir=/var/folders/[^ ]*/T/Test'

principais=$(ps -Ao pid,ppid,etime,command 2>/dev/null \
  | grep -i "Google Chrome" \
  | grep -E "$padrao" \
  | grep -v -- "--type=" \
  | grep -v grep || true)

if [ -z "$principais" ]; then
  echo "orphan-browser-check: nenhum browser de teste sobrevivente."
  exit 0
fi

n=$(printf '%s\n' "$principais" | grep -c . )
echo "orphan-browser-check: $n browser(s) de teste SOBREVIVERAM a execucoes anteriores." >&2
printf '%s\n' "$principais" | awk '{printf "  pid=%s ppid=%s idade=%s\n", $1, $2, $3}' >&2
cat >&2 <<'AVISO'

  Cada um segura memoria e CPU e torna o proximo arranque mais lento, ate um
  teste estourar o prazo por uma razao que nada tem a ver com o codigo (F103).

  ppid=1 significa que o processo DONO morreu antes de o parar: nesse caso nao
  houve CleanStop a correr, e a escalada nao cobre — so' esta varredura cobre.

  Para limpar, com o recorte de perfil TEMPORARIO ja aplicado:
    make orphan-browser-clean
AVISO
exit 1
