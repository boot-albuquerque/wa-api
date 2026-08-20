// PENDENTE DE AUTORIZAÇÃO HUMANA — NÃO EXECUTE.
//
// Este arquivo mexe em dado REAL: lê cada linha de users com s3_secret_key
// não-vazia e sem o prefixo enc:v1:, cifra com a chave global e grava o
// envelope no lugar. Só o humano autoriza a execução.
//
// NÃO é chamado de lugar nenhum — nenhuma rota, nenhum arranque, nenhum
// teste o importa. É um programa standalone (main), escrito para ser
// compilado e rodado manualmente:
//
//	go run scripts/migrate_s3_secret_to_envelope.go \
//	  -db "path/to/wa.db" \
//	  -key "$GLOBAL_ENCRYPTION_KEY" \
//	  -dry-run
//
// A flag -dry-run (default: true) lista as linhas que seriam migradas sem
// alterar nada. Roda sem -dry-run=false para aplicar.
//
// Contexto: F163 (HOUSEKEEP.md) / ADR-0009. A decisão do canal é:
// "migração das credenciais existentes explicitamente pendente de decisão
// humana". Este arquivo materializa o código para quando a decisão for
// tomada.

//go:build ignore

package main

import (
	"database/sql"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"

	"wa-api/pkg/infra/auth"
)

func main() {
	dbPath := flag.String("db", "", "path to the SQLite database file")
	key := flag.String("key", "", "global encryption key (hex, 32 bytes)")
	dryRun := flag.Bool("dry-run", true, "list candidates without writing")
	flag.Parse()

	if *dbPath == "" || *key == "" {
		fmt.Fprintln(os.Stderr, "usage: migrate_s3_secret_to_envelope -db PATH -key KEY [-dry-run=false]")
		os.Exit(1)
	}

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT id, s3_secret_key FROM users WHERE s3_secret_key != '' AND s3_secret_key NOT LIKE ?`,
		auth.S3SecretEnvelopePrefix+"%")
	if err != nil {
		log.Fatalf("query: %v", err)
	}
	defer rows.Close()

	type candidate struct {
		id     string
		secret string
	}
	var candidates []candidate

	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.secret); err != nil {
			log.Fatalf("scan: %v", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		log.Fatalf("rows: %v", err)
	}

	fmt.Printf("found %d rows with plaintext s3_secret_key\n", len(candidates))

	if *dryRun {
		for _, c := range candidates {
			fmt.Printf("  [dry-run] user %s: %d bytes, not enveloped\n", c.id, len(c.secret))
		}
		if len(candidates) > 0 {
			fmt.Println("\nre-run with -dry-run=false to apply")
		}
		return
	}

	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("begin tx: %v", err)
	}

	stmt, err := tx.Prepare(`UPDATE users SET s3_secret_key = ? WHERE id = ?`)
	if err != nil {
		log.Fatalf("prepare: %v", err)
	}
	defer stmt.Close()

	migrated := 0
	for _, c := range candidates {
		envelope, err := auth.EncryptS3Secret(c.secret, *key)
		if err != nil {
			log.Printf("SKIP user %s: encrypt failed: %v", c.id, err)
			continue
		}
		if !strings.HasPrefix(envelope, auth.S3SecretEnvelopePrefix) {
			log.Printf("SKIP user %s: envelope missing prefix (bug?)", c.id)
			continue
		}

		// Round-trip check: decrypt the envelope and compare.
		decrypted, err := auth.DecryptS3Secret(envelope, []byte(*key))
		if err != nil {
			log.Printf("SKIP user %s: round-trip decrypt failed: %v", c.id, err)
			continue
		}
		if decrypted != c.secret {
			log.Printf("SKIP user %s: round-trip mismatch", c.id)
			continue
		}

		if _, err := stmt.Exec(envelope, c.id); err != nil {
			log.Printf("SKIP user %s: update failed: %v", c.id, err)
			continue
		}
		migrated++
		// Only the length — never the value — to avoid leaking the secret.
		_ = base64.StdEncoding
		fmt.Printf("  migrated user %s (%d bytes)\n", c.id, len(c.secret))
	}

	if err := tx.Commit(); err != nil {
		log.Fatalf("commit: %v", err)
	}
	fmt.Printf("\nmigrated %d of %d rows\n", migrated, len(candidates))
}
