# Fusão de `feature/wa-headless-foundation` em `feature/macbook-lucas`

**Data**: 2026-08-23.
**Âncora do estado ANTERIOR**: tag `pre-merge/macbook-lucas-2026-08-23` e branch
`backup/macbook-lucas-pre-headless`, ambas em `229e756`.

Este documento existe para uma pergunta futura: *"faltou alguma coisa, ou entrou
algum defeito nesta fusão?"* — e para que a resposta venha de comandos e não de
memória.

## Como voltar atrás

A fusão é um commit de merge explícito (`--no-ff`), então há duas formas, e a
escolha depende de já se ter construído em cima dela.

```sh
# (a) desfazer a fusão preservando a história — seguro depois de outros commits
git revert -m 1 <sha-do-merge>

# (b) voltar o ponteiro ao estado exato de antes — só se nada foi construído em cima
git branch -f feature/macbook-lucas pre-merge/macbook-lucas-2026-08-23
```

A tag NUNCA é apagada. Ela é o que permite (b) meses depois, e o que dá a linha
de base de qualquer comparação.

## O que a fusão trouxe

```sh
# commits acrescentados
git rev-list --count pre-merge/macbook-lucas-2026-08-23..feature/macbook-lucas

# arquivos tocados
git diff --stat pre-merge/macbook-lucas-2026-08-23..feature/macbook-lucas | tail -1
```

## As quatro verificações, para reexecutar

Foram estas que rodaram antes de fundir. Qualquer uma que passe a falhar no
futuro aponta o que mudou.

### 1. Nenhum commit ficou de fora

```sh
git merge-base --is-ancestor pre-merge/macbook-lucas-2026-08-23 feature/macbook-lucas && echo OK
git rev-list --count feature/macbook-lucas..pre-merge/macbook-lucas-2026-08-23   # tem de ser 0
```

### 2. Nenhum arquivo desapareceu

```sh
comm -23 <(git ls-tree -r --name-only pre-merge/macbook-lucas-2026-08-23 | sort) \
         <(git ls-tree -r --name-only feature/macbook-lucas | sort)
# saída vazia = nenhum arquivo do estado anterior sumiu
```

### 3. Nenhuma DECLARAÇÃO desapareceu

Esta é a que pega o erro que as duas primeiras não pegam: um merge que mantém o
arquivo e perde uma função dentro dele.

```sh
simb() { git show "$1:$2" 2>/dev/null | grep -oE "^(func|type|const|var) \(?[^)]*\)? ?[A-Za-z_][A-Za-z0-9_]*" | sed -E 's/.* //' | sort -u; }
for f in $(git diff --name-only pre-merge/macbook-lucas-2026-08-23..feature/macbook-lucas -- '*.go'); do
  faltam=$(simb pre-merge/macbook-lucas-2026-08-23 "$f" | comm -23 - <(simb feature/macbook-lucas "$f"))
  [ -n "$faltam" ] && { echo "### $f"; echo "$faltam" | sed 's/^/    /'; }
done
```

**Resultado no dia da fusão**: quatro arquivos sinalizados, e os quatro
explicados como RENOMEAÇÃO para EN-US (regra de idioma do `CLAUDE.md`), com o
equivalente localizado um a um:

| símbolo antigo | equivalente |
|---|---|
| `clusterModeConfigurado` | `clusterModeFromEnv` |
| `executar` | `runJob` |
| `trabalhar` | `worker` |
| `Metricas` | `Metrics` |

(`int` também apareceu, e era artefato do grep casando `tentativa int` numa
assinatura.)

### 4. A árvore compila e os testes passam

```sh
go vet ./... && go test ./pkg/... ./cmd/... -count=1
```

## O que a verificação NÃO cobre, dito em voz alta

Ela prova que nenhuma **declaração** sumiu. Não prova que nenhum
**comportamento** se perdeu DENTRO de uma função onde um lado foi escolhido.

Onde havia esse risco, a resolução foi bloco a bloco, compondo os dois lados em
vez de escolher um:

- `pkg/bootstrap/main.go` — seleção de engine (decisão 94) **e**
  `publishCapabilities`/outbox sobreviveram no mesmo arquivo.
- `Makefile` — comentário de um lado (a evidência da F99) com o comando do outro
  (F110). Os cinco alvos exclusivos dos dois lados sobreviveram.
- `pkg/application/contracts/user_ports.go` — as duas formas de estreitar a
  mesma interface gorda coexistem (`IdentityResolver`+`AvatarReader`+
  `ContactRoster` da decisão 82, e `LIDResolver`).
- `pkg/infra/wa-noise/mapping/jid/parse.go` — o mesmo panic consertado nos dois
  ramos, com as duas notas fundidas.
- `pkg/bootstrap/dispatch_retry.go` — prosa de um lado, código do outro.

## Duas coisas que ficaram por decidir

**Colisão de numeração de achados.** Doze números aparecem duas vezes, e NOVE
são colisões reais — `F94`, `F96`–`F103` nomeiam coisas diferentes nos dois
ramos (ex.: `F101` é o panic de `ParseJID("")` num lado e *"user not found
responde 500"* no outro). Não foram renumerados: isso reescreveria dezenas de
referências cruzadas em código e documento.

```sh
grep -oE "^## F[0-9]+ " HOUSEKEEP.md | sed 's/^## //;s/ $//' | sort -V | uniq -d
```

**Diluição do `log-coverage`.** `eligible` foi de 631 para 921 porque a árvore
headless inteira entrou no denominador, e as razões caíram por diluição — nenhum
sítio que logava deixou de logar. O conserto melhor tem precedente no próprio
`.log-coverage-baseline` (a F204 tirou a fachada `wa-noise/client` do
denominador por ser delegação), e não foi aplicado aqui para não misturar
decisão de arquitetura com fusão no mesmo diff.
