# ADR-0009: segredo de S3 cifrado em coluna TEXT, com envelope versionado

- **Status**: **accepted (2026-08-19)** — decidido no canal de orquestração;
  implementação **não iniciada**. Divergência DELIBERADA do comportamento
  histórico, registrada aqui porque o CLAUDE.md exige que divergir seja
  decisão consciente e não acidente.
- **Data**: 2026-08-19
- **Relacionado**: F151, F157, F162 em `HOUSEKEEP.md`; ADR-0004 (fork
  intencional)
- **Por que 0009 e não 0008**: o número 0008 foi deixado VAGO de propósito.
  `cmd/logcov/main_test.go:98` e a saída do gate de log-coverage citam
  "ADR-008" como justificativa do estágio ratchet, mas esse documento nunca
  existiu em `docs/adr/`. Ocupar o 0008 faria a referência pendurada resolver
  em silêncio para ESTE documento, e o gate de cobertura passaria a parecer
  justificado por um ADR sobre cifra de S3. Ver [[F162]].

## Contexto

`configure_s3.go` é um dos dez stubs de configuração da F157: responde
`200 "S3 configuration validated"` e não grava nada. Ao recuperar o contrato
para consertá-lo, a medição do histórico produziu um resultado incômodo.

### O que o histórico faz, medido

`41bc8e2^:handlers.go:6207` (`ConfigureS3`) grava as credenciais **em claro**:

```go
_, err = s.db.Exec(`UPDATE users SET ... s3_access_key = $5, s3_secret_key = $6 ...`,
    t.Enabled, t.Endpoint, t.Region, t.Bucket, t.AccessKey, t.SecretKey, ...)
```

Nenhuma chamada de cifra. E o cache recebe os mesmos valores em claro
(`S3AccessKey`, `S3SecretKey`).

Isso contrasta com o único outro segredo do schema. O HMAC, no mesmo arquivo
histórico (`:6767`), **cifra** antes de gravar, e a coluna reflete isso:

| coluna | tipo | comentário do próprio schema |
|---|---|---|
| `hmac_key` | `BYTEA` | *"as BYTEA for encrypted data"* (`migrations.go:1047`) |
| `s3_secret_key` | `TEXT DEFAULT ''` | — (`migrations.go:421`) |

Ou seja: o histórico tratou os dois segredos com critérios diferentes, e a
diferença ficou congelada no tipo da coluna.

### Verificado nos DOIS backends, não só no Postgres

A tabela acima veio do bloco `information_schema`, que é o caminho Postgres.
O SQLite tem migração própria, e foi conferida — o tipo é o mesmo:

| coluna | Postgres | SQLite |
|---|---|---|
| `hmac_key` | `BYTEA` (`migrations.go:1047`) | `BLOB` (`:757`) |
| `s3_secret_key` | `TEXT DEFAULT ''` (`:421`) | `TEXT DEFAULT ''` (`:701`) |

Importa porque o SQLite é o padrão de instalação single-pod — é o que rodou na
validação real desta sessão. Uma decisão de envelope que só valesse no
Postgres quebraria justamente a instalação mais comum. Nenhum dos dois exige
migração nova.

## Decisão

**Cifrar o segredo de S3**, divergindo do histórico, e acomodar o ciphertext
na coluna `TEXT` existente através de um **envelope versionado**:

```
enc:v1:<base64 do ciphertext AES-GCM>
```

**Linha sem o prefixo `enc:v1:` é INVÁLIDA.** O sistema falha fechado e exige
reconfiguração da credencial. **Não há fallback para plaintext.**

## Por que assim, e não das outras formas

**Por que não manter em claro (fidelidade ao histórico).** Recuperar contrato
é o método desta missão, mas fidelidade não é obrigação de repetir um defeito.
Credencial de terceiro em claro no banco é superfície de vazamento que o
próprio repo já resolveu melhor no HMAC — divergir aqui é escolher o padrão
que a base já demonstra conhecer.

**Por que não migrar `s3_secret_key` para `BYTEA`.** Seria mais coerente com o
`hmac_key` e auto-documentado no schema. Recusado por ser migração de coluna
que pode conter dado de produção, num ponto onde o dado é justamente o que não
se pode perder nem corromper. O envelope entrega a mesma propriedade sem tocar
no schema.

**Por que o envelope, e não simplesmente base64 do ciphertext.** Esta é a
parte que decide, e não é estética. Uma instalação antiga, escrita pelo
binário histórico, **pode já ter segredo em claro nessa coluna**. Sem prefixo,
o código não tem como distinguir "texto em claro legado" de "ciphertext
corrompido": `Decrypt` falha nos dois casos, com o mesmo erro. E o que
acontece a seguir é previsível — alguém "trata" essa falha caindo de volta
para interpretar o valor como plaintext, que é **fallback silencioso em
caminho de segredo**, a classe de defeito explicitamente proibida nesta
missão.

O prefixo torna o legado **detectável em vez de ambíguo**. Com ele, "sem
`enc:v1:`" é um estado nomeado, com resposta definida, e não uma exceção que
alguém vai domesticar.

**Por que invalidar o legado em vez de cifrá-lo na migração.** Cifrar o que
está em claro exigiria a chave de cifra no momento da migração, acoplando
migração a configuração de runtime. Invalidar custa uma reconfiguração manual
por instalação afetada, e é honesto: a credencial que esteve em claro no banco
deve ser considerada exposta e rotacionada de qualquer forma.

## Consequências

- Instalação com credencial de S3 gravada pelo binário histórico precisa
  reconfigurar o S3 uma vez. Deve constar em nota de release.
- O `v1` no envelope é o mecanismo de troca de algoritmo no futuro sem repetir
  esta discussão.
- Teste obrigatório: linha sem prefixo é REJEITADA, e a rejeição **não** cai
  para plaintext. É controle negativo, não asserção de caminho feliz.
- O `GET` continua devolvendo a credencial mascarada; o envelope não pode
  vazar nem em erro nem em log.

## O que este ADR NÃO decide

A chave HMAC **global** (`F156`: gerada com `math/rand` e impressa no log em
claro) é de bootstrap, não de rota por usuário, e o canal a manteve como bloco
separado. Não é escopo daqui.
