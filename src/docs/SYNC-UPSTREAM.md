# SYNC-UPSTREAM.md — Runbook de sincronização com asternic/wuzapi

> **Objetivo:** Manter o fork `disparazaap-wuzapi` sincronizado com o upstream
> `asternic/wuzapi` sem perder código custom nem quebrar contratos com o disparazaap.

## Pré-requisitos

- Git remote `origin` apontando para `asternic/wuzapi` (upstream)
- `.gitattributes` configurado com `merge=ours` para arquivos custom
- `go` >= 1.25 instalado e funcional (`go version`)

## Fluxo normal de sync

### 1. Buscar mudanças do upstream

```bash
git fetch origin
```

### 2. Simular merge (dry-run)

```bash
git merge --no-commit --no-ff origin/main
```

### 3. Verificar conflitos

```bash
git status
```

**Arquivos esperados SEM conflito:**
- `custom_routes.go` — protegido por `merge=ours`
- `internal/**` — protegido por `merge=ours`
- `docs/SYNC-UPSTREAM.md` — protegido por `merge=ours`

**Arquivos que PODEM conflitar:**
- `routes.go` — se upstream adicionou/removeu rotas na mesma região do hook
- `handlers.go` — se upstream modificou a função `GetProfile()` (improvável)
- `go.mod` / `go.sum` — se upstream atualizou dependências

### 4. Resolver conflitos comuns

#### Conflito em `routes.go`

Se o upstream moveu ou removeu a linha onde inserimos o hook `s.registerCustomRoutes()`:

1. Abra `routes.go`
2. Localize o bloco de rotas `/session/...` (linhas ~80-88)
3. Re-insira o hook **antes** de `s.router.PathPrefix("/")`:
   ```go
   // Hook disparazaap: registra rotas custom sem modificar arquivos upstream.
   s.registerCustomRoutes()
   ```
4. Salve e continue: `git add routes.go`

#### Conflito em `handlers.go`

Se o upstream reescreveu `GetProfile()` (improvável, mas possível):

1. Inspecione o diff: `git diff origin/main -- handlers.go | grep -A10 "GetProfile"`
2. Se a função wrapper sumiu, recoloque o wrapper de delegação:
   ```go
   func (s *server) GetProfile() http.HandlerFunc {
       return s.customHandlers.GetProfile()
   }
   ```
3. Continue: `git add handlers.go`

#### Conflito em `go.mod` / `go.sum`

1. Aceite a versão do upstream para dependências core
2. Adicione de volta dependências nossas (ex.: `testcontainers-go` para integration tests)
3. Rode `go mod tidy` para regenerar `go.sum`

### 5. Validar pós-merge

```bash
go build ./...                              # compilação limpa
go test ./internal/...                      # testes unitários + contract
go test -tags=integration ./internal/...     # testes de integração (se disponível)
golangci-lint run ./internal/...            # lint sem novos warnings
```

### 6. Commit do merge

```bash
git commit -m "chore: sync upstream asternic/wuzapi (origin/main)"
```

### 7. Push para o fork

```bash
git push origin main
```

## Checklist pré-push

- [ ] `go build ./...` passa (exit 0)
- [ ] `go test ./internal/...` passa (todos verdes)
- [ ] `golangci-lint run ./internal/...` retorna 0 issues
- [ ] `git diff HEAD~1 -- routes.go` mostra apenas o hook `s.registerCustomRoutes()`
- [ ] Nenhum arquivo em `custom_routes.go` ou `internal/` foi sobrescrito acidentalmente
- [ ] Se houve mudança em `handlers.go`, `GetProfile()` ainda é wrapper ≤5 linhas

## Troubleshooting

### `.gitattributes` não está sendo respeitado

Verifique:
```bash
git config --global core.attributesFile
```

Se retornar um path, o git está usando um atributo global que pode sobrescrever
o `.gitattributes` do repositório. Para debugging:
```bash
git check-attr -a custom_routes.go
# Deve mostrar: custom_routes.go: merge: ours
```

### Merge abortado mas arquivos estão modificados

```bash
git merge --abort           # desfaz o merge
git reset --hard HEAD       # volta ao estado limpo (CUIDADO: perde mudanças não commitadas)
```

### Dúvidas

Abra uma issue no repositório disparazaap com a tag `wuzapi-sync`.

## Security Patch Handling

> **IMPORTANTE:** Arquivos marcados com `merge=ours` (`.gitattributes`) **não** recebem
> patches de segurança do upstream automaticamente. Siga este procedimento:

### Após cada `git pull upstream main`

1. **Verificar CVEs nas dependências:**
   ```bash
   go run golang.org/x/vuln/cmd/govulncheck@latest ./...
   ```
   - Se CRITICAL: **bloquear PR** até resolver.
   - Se HIGH: documentar waiver com JIRA ticket e prazo de correção.
   - Se LOW/MEDIUM: informativo, não bloqueia.

2. **Revisar CHANGELOG do upstream por security notes:**
   ```bash
   git log origin/main --oneline --grep="security\|CVE\|fix(vuln)\|vulnerability" | head -20
   ```

3. **Inspecionar patches de segurança em arquivos `merge=ours`:**
   ```bash
   git diff origin/main HEAD -- custom_routes.go internal/
   ```
   Se houver patches de segurança nesses arquivos, aplicar manualmente:
   - `custom_routes.go` — nossa versão NÃO recebe patches upstream
   - `internal/` — nosso código, mas se dependência interna mudar assinatura (ex.: whatsmeow), atualizar adapter

4. **Re-rodar suíte completa de testes:**
   ```bash
   go build ./... && go test -race -count=1 ./internal/... && golangci-lint run ./internal/...
   ```

### Merge=ours e CVEs — regra de ouro

Arquivos com `merge=ours` NUNCA recebem mudanças do upstream. Se um CVE patch
tocar um desses arquivos, você DEVE aplicar manualmente. Verifique isso em
**todo** pull do upstream.
