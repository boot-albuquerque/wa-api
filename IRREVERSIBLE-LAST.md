# Operações irreversíveis — lote da sessão descartável

**Conclusão em uma linha: nenhuma operação irreversível deste lote ficou por
fazer por falta de fixture descartável.** Todas as destrutivas foram
executadas, contra sessões criadas para o efeito, e a recuperação de todas era
a mesma: `POST /admin/users` outra vez.

Este documento existe para tornar isso auditável, e não para registar um
bloqueio — o único bloqueio real do lote é de outro tipo (falta de bucket S3) e
está em `HUMAN-LAST.md`.

## As irreversíveis que FORAM executadas

| Endpoint | Efeito | Fixture descartável disponível? | Recuperação | Autorizado? |
|---|---|---|---|---|
| `DELETE /admin/users/{id}` | apaga a linha em `users`; deixa a sessão viva em memória e a mídia em disco | **sim** — `descartavel-2`, criada e apagada na mesma bateria | recriar com `POST /admin/users`; o token é novo, o antigo não é recuperável | sim — sessão criada por mim, nunca emparelhada |
| `DELETE /admin/users/{id}/full` | apaga a linha, faz `logout`+`disconnect` e apaga `<datadir>/files/<id>` | **sim** — `descartavel-4` no teste, e `descartavel-1` e `descartavel-3` na limpeza | idem; a mídia apagada não volta, mas não havia nenhuma | sim — idem |
| `DELETE /webhook` | apaga URL e subscrições da sessão | **sim** — `descartavel-1` | reconfigurar com `POST /webhook` | sim |
| `DELETE /hmac/config` · `DELETE /session/hmac/config` | apaga a chave HMAC gravada, que **nunca é devolvida** por rota nenhuma | **sim** | regravar; a chave antiga não é recuperável, mas era minha e descartável | sim |
| `DELETE /s3/config` · `DELETE /session/s3/config` | apaga credenciais de S3 da sessão | **sim** | regravar | sim |
| `POST /session/logout` | desemparelha o aparelho | **sim**, mas **inócuo** — a sessão nunca tinha sido emparelhada, logo não havia nada para desemparelhar. Foi isso que produziu o `500` da F275 | n/a | sim |

## As irreversíveis que NÃO foram executadas

| Endpoint | Efeito | Fixture descartável disponível? | Recuperação | Autorizado? |
|---|---|---|---|---|
| `POST /session/pairphone` | inicia emparelhamento de um número real | **não** — o número é de uma pessoa, não da sessão | cancelar no telemóvel | **não** — fora do escopo deste lote por enunciado |
| `POST /users/avatar` | substitui o avatar da conta | **não** — o avatar pertence à conta, não à sessão | repor o anterior, se alguém o tiver guardado antes | **não** — fora do escopo |
| `POST /users/privacy` | altera definições de privacidade da conta | **não** — idem | repor manualmente, se as anteriores tiverem sido anotadas | **não** — fora do escopo |
| `POST /users/status` | substitui o recado da conta | **não** — idem | repor manualmente | **não** — fora do escopo |

**O padrão que separa as duas tabelas** vale a pena enunciar, porque é ele que
diz quando a desculpa "é irreversível" é verdadeira:

> É descartável o que pertence à **sessão**. Não é descartável o que pertence à
> **conta de WhatsApp** por trás dela.

Uma sessão nova custa uma chamada a `POST /admin/users`. Um avatar, um recado
ou uma definição de privacidade são de uma pessoa, e apagá-los não se desfaz
criando outra sessão. As quatro linhas da segunda tabela têm todas a mesma
forma: **o alvo do efeito está do lado de lá da fronteira da sessão.**

Foi por não fazer essa distinção que 25 rotas ficaram ⬜ com o motivo "mexeria
na sessão em uso" — a palavra que faltava era **"em uso"**, e a resposta a isso
não é não testar: é **não usar a sessão em uso**.
