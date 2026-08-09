# Patches locais em internal/wa-headless/

Registro do que divergimos do upstream `wa-api/internal/wa-noise` e por quê.

Desde o [ADR-0004](../../docs/adr/0004-refatorar-internal-waclient-em-fork-intencional.md)
este diretório **deixou de ser espelho drift-zero** e passou a ser um fork
ativamente mantido: divergência contra o upstream é esperada e correta, não
uma falha. Só `internal/wa-noise/proto/` (código gerado a partir de `.proto`)
continua travado byte-a-byte por `make waclient-drift`.

Este arquivo é o mecanismo de reconciliação manual contra futuras versões
upstream — cada entrada precisa dizer **quais arquivos**, **o que mudou**,
**por quê** e **se o comportamento mudou**.

---

## Fase A — raiz do pacote (`internal/wa-headless/*.go`), 2026-08-08

### Contexto
