#!/usr/bin/env bash
# Trava do carimbo `route` dos handlers HTTP (F146).
#
# Cada handler carimba no log uma constante `route` que serve para FILTRAR o
# log por rota durante investigacao. Ela nao muda comportamento, e por isso
# nenhum teste a olhava: tres das quinze constantes apontavam para caminhos
# que NUNCA existiram ("/chat/rejectcall" no lugar de "/call/reject",
# "/chat/requestunavailablemessage" no lugar de "/chat/request-unavailable-message",
# "/admin/users/{id}/complete" no lugar de "/admin/users/{id}/full").
# Quem investigasse chamada rejeitada ou mensagem perdida filtrando por rota
# nao acharia nada, e concluiria que o handler nunca foi chamado.
#
# Esta checagem falha se qualquer `const route = "..."` de handler nao
# corresponder EXATAMENTE a um caminho registrado por cmd/listroutes.
#
# FALHA FECHADO: se listroutes nao rodar, ou devolver lista vazia, o gate
# FALHA. Um gate que passa porque a comparacao abortou em silencio e' pior
# que nenhum (foi o que aconteceu na F129).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

HANDLER_DIR="pkg/presentation/http/handlers"
ROUTES_FILE="$(mktemp)"
CONSTS_FILE="$(mktemp)"
trap 'rm -f "$ROUTES_FILE" "$CONSTS_FILE"' EXIT

# 2>/dev/null e' OBRIGATORIO: sem ele as linhas de log do bootstrap entram na
# lista e viram "rotas" fantasma (foi assim que o inventario contou 110 em vez
# de 107). O redirecionamento so' apaga stderr; a falha do comando continua
# visivel pelo status de saida, capturado abaixo.
if ! go run ./cmd/listroutes 2>/dev/null | sed -E 's/^[A-Z]+[[:space:]]+//' | sort -u > "$ROUTES_FILE"; then
  echo "FALHA: 'go run ./cmd/listroutes' nao executou — impossivel validar os carimbos de rota."
  echo "O gate FALHA FECHADO: sem a lista de rotas registradas nao ha' com o que comparar."
  exit 1
fi

route_count="$(wc -l < "$ROUTES_FILE" | tr -d ' ')"
if [ "$route_count" -eq 0 ]; then
  echo "FALHA: cmd/listroutes devolveu lista VAZIA de rotas registradas."
  echo "O gate FALHA FECHADO: com lista vazia toda constante 'passaria' por"
  echo "ausencia de comparacao, exatamente o modo de falha da F129."
  exit 1
fi

# Constantes de rota dos handlers, em ordem estavel (arquivo:linha ordenados),
# para que a saida do gate nao dependa da ordem de expansao do glob.
grep -rnE '^[[:space:]]*const[[:space:]]+route[[:space:]]*=[[:space:]]*"[^"]+"' "$HANDLER_DIR" \
  --include '*.go' \
  | grep -v '_test\.go:' \
  | sed -E 's/^([^:]+):([0-9]+):.*"([^"]+)".*/\3|\1|\2/' \
  | LC_ALL=C sort -t'|' -k2,2 -k3,3n > "$CONSTS_FILE" || true

const_count="$(wc -l < "$CONSTS_FILE" | tr -d ' ')"
if [ "$const_count" -eq 0 ]; then
  echo "FALHA: nenhuma constante 'const route = \"...\"' encontrada em $HANDLER_DIR."
  echo "O gate FALHA FECHADO: ou os handlers pararam de carimbar rota no log,"
  echo "ou o padrao mudou e esta checagem deixou de enxergar o que deveria travar."
  exit 1
fi

failed=0
while IFS='|' read -r path file line; do
  if grep -qxF "$path" "$ROUTES_FILE"; then
    continue
  fi
  failed=1
  echo "FALHA: constante route aponta para caminho NAO REGISTRADO"
  echo "  valor:  \"$path\""
  echo "  onde:   $file:$line"
  echo "  rotas registradas mais parecidas:"
  # Vizinhanca por prefixo comum mais longo; empate resolvido pela ordem
  # lexicografica da propria rota, entao a saida e' deterministica.
  awk -v want="$path" '
    {
      n = 0
      m = (length($0) < length(want)) ? length($0) : length(want)
      while (n < m && substr($0, n+1, 1) == substr(want, n+1, 1)) n++
      printf "%03d\t%s\n", n, $0
    }
  ' "$ROUTES_FILE" | LC_ALL=C sort -k1,1nr -k2,2 | head -3 | cut -f2 | sed 's/^/    /'
  echo
done < "$CONSTS_FILE"

if [ "$failed" -ne 0 ]; then
  echo "O carimbo 'route' do log tem de ser o padrao da rota REGISTRADA em"
  echo "pkg/bootstrap/wiring_routes.go — com o parametro na forma {id}, e nao"
  echo "o caminho concreto de uma requisicao. CORRIJA a constante para o"
  echo "caminho registrado; nao registre uma rota nova para casar com o log."
  exit 1
fi

echo "handler-route-check: $const_count constantes route conferidas contra $route_count rotas registradas; todas existem."
