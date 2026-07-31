# PAIRING-SMOKE.md — Smoke Test de Pareamento disparazaap ↔ WuzAPI

> **Objetivo:** Validar que o fork `disparazaap-wuzapi` não regride o pareamento
> de contas WhatsApp consumido pelo wa-worker do disparazaap.
> O comportamento esperado é que, após pareamento via `/accounts/connect`,
> `owner_pushname` e `owner_avatar_url` estejam populados no banco disparazaap.

## Pré-requisitos

- Docker e Docker Compose instalados
- Repositório disparazaap clonado em `~/Documentos/repos/disparazaap`
- Fork disparazaap-wuzapi compilado e com imagem Docker: `docker build -t disparazaap-wuzapi:local .`
- Uma conta WhatsApp válida para pareamento (celular com WhatsApp)

## Passos

### 1. Subir stack de pairtest

```bash
cd ~/Documentos/repos/disparazaap
docker compose -f infra/compose.yaml -f infra/compose.pairtest.yaml up -d wuzapi wa-worker
```

Aguarde ~10s para os containers iniciarem.

### 2. Criar conta via API de management

```bash
# Criar conta com send_mode='single' (usa WuzAPI como provider)
curl -X POST http://localhost:3001/api/accounts \
  -H "Content-Type: application/json" \
  -d '{"msisdn": "5511999999999", "send_mode": "single"}'
```

Anote o `id` retornado (formato `wa_<uuid>`).

### 3. Parear via UI

Abra `http://localhost:3000/accounts/connect` no navegador.
Siga o wizard de 4 etapas até a exibição do QR code.
Escaneie o QR code com o WhatsApp no celular (WhatsApp → Linked Devices → Link a Device).

### 4. Aguardar retry do wa-worker

O wa-worker faz retry a cada 1s + 3×3s ao detectar uma conta pareada.
Aguarde ~10 segundos após o pareamento ser confirmado na tela.

### 5. Verificar população no banco

```bash
psql -h localhost -U postgres -d disparazaap -c "
  SELECT id, msisdn, owner_pushname, owner_avatar_url, owner_avatar_id
  FROM whatsapp_accounts
  ORDER BY created_at DESC
  LIMIT 1;
"
```

### 6. Critérios de sucesso

- [ ] `owner_pushname` NÃO está vazio (contém o nome público do WhatsApp)
- [ ] `owner_avatar_url` NÃO está vazio (contém URL da foto de perfil)
- [ ] `owner_avatar_id` presente (pode ser string vazia — depende da conta)
- [ ] Nenhum erro nos logs do wa-worker: `docker compose logs wa-worker | grep -i error`

### 7. Evidência

Copie o output do comando `psql` do passo 5 para `docs/evidence/pairing-smoke-YYYY-MM-DD.txt`.
Exemplo de output esperado:

```
                  id                  |     msisdn      | owner_pushname |           owner_avatar_url            | owner_avatar_id
--------------------------------------+-----------------+----------------+----------------------------------------+------------------
 wa_abc123-def456-...                 | 5511999999999   | John Doe       | https://pps.whatsapp.net/v/t58.1234/1 | abc123
(1 row)
```

## Troubleshooting

### QR code não aparece
- Verifique se o container wuzapi está saudável: `curl http://localhost:8080/health`
- Verifique logs: `docker compose logs wuzapi | tail -20`

### Campos owner_* vazios após pareamento
- Aguarde mais 10s (retry do wa-worker tem intervalo de até 10s)
- Verifique se o GET /session/profile responde: `curl -H "Authorization: Bearer <token>" http://localhost:8080/session/profile`
- Verifique logs do wa-worker: `docker compose logs wa-worker | grep -i profile`

### Erro de conexão com Postgres
- Verifique se o banco está acessível: `psql -h localhost -U postgres -d disparazaap -c "SELECT 1"`

## Histórico de smoke tests

| Data | Resultado | owner_pushname | owner_avatar_url | Evidência |
|---|---|---|---|---|
| (preencher) | (preencher) | (preencher) | (preencher) | (preencher) |
