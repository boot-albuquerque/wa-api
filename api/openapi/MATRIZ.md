# Matriz de distribuição da documentação OpenAPI

Levantamento: 2026-08-26, contra `bootstrap.Routes(Deps{})` — a mesma função
que serve o processo, e a mesma que `cmd/listroutes` usa.

**234 entradas de rota** (método + caminho) sobre **212 caminhos distintos**.

A matriz abaixo é do levantamento original, quando havia 141 entradas. Desde a
padronização de 26/08 cada família de colecção tem também a forma canónica, que
não muda a distribuição por autor — a canónica é gerada da mesma fonte que a
antiga, e não foi escrita por ninguém. Ver `CAMINHOS-CANONICOS.md`.

| Grupo | Tag(s) OpenAPI | Rotas | Worktree | Ficheiros | Depende de |
|---|---|---:|---|---|---|
| Saúde | `Saúde` | 4 | — (fundação) | `paths/saude.yaml`, `schemas/saude.yaml` | — |
| Sessões | `Sessões` | 21 | `openapi/sessao` | `paths/sessao.yaml`, `schemas/sessao.yaml` | base |
| Envio | `Envio de mensagens` | 16 | `openapi/envio` | `paths/envio.yaml`, `schemas/envio.yaml` | base |
| Conversas | `Conversas`, `Descarga de mídia` | 18 | `openapi/conversa` | `paths/conversa.yaml`, `schemas/conversa.yaml` | base |
| Grupos | `Grupos` | 18 | `openapi/grupo` | `paths/grupo.yaml`, `schemas/grupo.yaml` | base |
| Canais | `Canais`, `Comunidades` | 22 | `openapi/canal` | `paths/canal.yaml`, `schemas/canal.yaml` | base |
| Contactos | `Contactos e utilizadores`, `Status` | 17 | `openapi/contacto` | `paths/contacto.yaml`, `schemas/contacto.yaml` | base |
| Infraestrutura | `Administração`, `Integrações e configuração` | 25 | `openapi/infra` | `paths/infra.yaml`, `schemas/infra.yaml` | base |
| | | **141** | | | |

## Por que os ficheiros são disjuntos

Cada autor escreve em dois ficheiros que mais ninguém toca. Não é organização:
é a única forma de sete ramos concorrentes não colidirem. O documento servido
(`pkg/presentation/http/apidocs/openapi.yaml`) é GERADO, e por isso os workers
não o commitam — sete versões dele seriam sete conflitos por um ficheiro que
ninguém escreveu à mão.

`cmd/openapidoc` recusa dois fragmentos que declarem o mesmo caminho. Não é
último-a-escrever-ganha: dois autores no mesmo caminho significa que alguém
documentou uma rota que não era sua, e escolher um em silêncio é como metade
da documentação desaparece num merge.

## Fronteiras que atravessam grupos

Estas foram ditas a cada worker, porque documentar uma sem mencionar a outra
deixa quem lê sem saída:

| a rota | e a sua irmã noutro grupo |
|---|---|
| `POST /session/history` (sessões) | `POST /webhook/history` (infra) — **o mesmo manipulador**, e o nome do segundo engana (F252) |
| `POST /s3/config` (infra) | `POST /s3/configure` (infra) — aliases (F251); idem HMAC |
| `GET /session/s3/config` (sessões) | `GET /s3/config` (infra) — mesmo manipulador, caminho novo e antigo |
| `POST /group/create` com `is_parent` (grupos) | é o que CRIA uma comunidade, cujas rotas são de canais |
| `POST /chat/downloadimage` (conversas) | consome o descritor que vem no `data_json` de `GET /chat/history` |
| `POST /user/status` (sessões) | NÃO é status de 24 h; esse é `/status/set/*` (contactos) |

## Autenticação por grupo

| esquema | onde | cabeçalho |
|---|---|---|
| `TokenSessao` | 118 rotas | `token` |
| `TokenAdmin` | as 6 de `/admin` | `Authorization` |
| nenhum | `/livez`, `/health/live`, `/health/ready` | — |

`GET /health` é a excepção que confunde: está sob token de sessão, ao contrário
das outras três sondas, porque expõe contagens da instalação.
