package sqlstore

import (
	"context"
	"fmt"

	"go.mau.fi/util/dbutil"
)

// legacyTablePrefix e legacyIndexes descrevem o schema como ele se chamava
// antes da renomeacao do modulo. Um banco criado por qualquer versao anterior
// carrega esses nomes; um banco novo nunca os teve.
const (
	legacyTablePrefix  = "whatsmeow_"
	currentTablePrefix = "wanoise_"
)

// legacyTables lista os sufixos de tabela que mudam de prefixo. A tabela de
// versao (`version`) esta' aqui de proposito e e' o motivo de esta rotina
// existir em Go em vez de virar mais um arquivo em upgrades/: o dbutil LE a
// tabela de versao antes de rodar qualquer migracao, entao uma migracao nunca
// conseguiria renomear a propria tabela que a seleciona. Se o rename da
// tabela de versao nao acontecesse antes, todo banco existente pareceria estar
// na versao 0 e o dbutil tentaria recriar o schema inteiro por cima dos dados.
//
// Renomear aqui, antes do upgrade, tambem e' o que permite que os arquivos de
// upgrades/ (00 ate' 14) usem os nomes novos: quando qualquer um deles roda,
// as tabelas ja' foram renomeadas, seja qual for a versao de origem do banco.
var legacyTables = []string{
	"version",
	"device",
	"identity_keys",
	"pre_keys",
	"sessions",
	"sender_keys",
	"app_state_sync_keys",
	"app_state_version",
	"app_state_mutation_macs",
	"contacts",
	"chat_settings",
	"message_secrets",
	"privacy_tokens",
	"nct_salt",
	"lid_map",
	"event_buffer",
	"retry_buffer",
}

// legacyIndex e' um indice que precisa ser recriado sob o nome novo. Renomear
// a tabela NAO renomeia seus indices, e `ALTER INDEX ... RENAME TO` so' existe
// no Postgres — por isso a forma dropa-e-recria, que vale nos dois dialetos.
//
// O campo table existe porque o indice so' pode ser recriado se a tabela dele
// estiver presente: um banco parado numa versao anterior a' que criou a tabela
// chega aqui sem ela, e um CREATE INDEX incondicional derrubaria o upgrade
// antes de a primeira migracao rodar.
type legacyIndex struct {
	oldName string
	newName string
	table   string
	create  string
}

var legacyIndexes = []legacyIndex{
	{
		oldName: "idx_whatsmeow_privacy_tokens_our_jid_timestamp",
		newName: "idx_wanoise_privacy_tokens_our_jid_timestamp",
		table:   "wanoise_privacy_tokens",
		create:  "CREATE INDEX IF NOT EXISTS idx_wanoise_privacy_tokens_our_jid_timestamp ON wanoise_privacy_tokens (our_jid, timestamp)",
	},
	{
		oldName: "whatsmeow_retry_buffer_timestamp_idx",
		newName: "wanoise_retry_buffer_timestamp_idx",
		table:   "wanoise_retry_buffer",
		create:  "CREATE INDEX IF NOT EXISTS wanoise_retry_buffer_timestamp_idx ON wanoise_retry_buffer (our_jid, timestamp)",
	},
}

// tableExistsQuery devolve a consulta de existencia de tabela do dialeto. Nao
// ha' forma portatil: o SQLite expoe sqlite_master, o Postgres expoe
// information_schema.
func tableExistsQuery(dialect dbutil.Dialect) string {
	if dialect == dbutil.SQLite {
		return "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=$1"
	}
	return "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=$1"
}

// renameLegacyTables normaliza o schema para o prefixo atual, antes de
// qualquer leitura de versao ou migracao.
//
// E' idempotente por construcao: cada tabela so' e' renomeada se a antiga
// existir E a nova nao existir. Num banco novo nada acontece; num banco ja'
// normalizado, tambem nao. Se as duas existirem — situacao que so' aparece com
// intervencao manual — a rotina nao mexe e nao falha, porque adivinhar qual
// das duas tem os dados bons e' pior do que deixar o operador decidir.
func renameLegacyTables(ctx context.Context, db *dbutil.Database) error {
	existsQuery := tableExistsQuery(db.Dialect)
	exists := func(name string) (bool, error) {
		var n int
		if err := db.QueryRow(ctx, existsQuery, name).Scan(&n); err != nil {
			return false, fmt.Errorf("falha ao checar existencia da tabela %s: %w", name, err)
		}
		return n > 0, nil
	}

	renamedAny := false
	for _, suffix := range legacyTables {
		oldName, newName := legacyTablePrefix+suffix, currentTablePrefix+suffix
		hasOld, err := exists(oldName)
		if err != nil {
			return err
		}
		if !hasOld {
			continue
		}
		hasNew, err := exists(newName)
		if err != nil {
			return err
		}
		if hasNew {
			continue
		}
		if _, err := db.Exec(ctx, fmt.Sprintf("ALTER TABLE %s RENAME TO %s", oldName, newName)); err != nil {
			return fmt.Errorf("falha ao renomear %s para %s: %w", oldName, newName, err)
		}
		renamedAny = true
	}

	if !renamedAny {
		return nil
	}

	// So' chega aqui num banco que acabou de ser migrado. Os indices seguem a
	// tabela renomeada carregando o nome antigo, entao dropar e recriar e' o
	// que sobra — e e' barato, porque acontece uma unica vez na vida do banco.
	for _, idx := range legacyIndexes {
		hasTable, err := exists(idx.table)
		if err != nil {
			return err
		}
		if !hasTable {
			// Banco parado antes da migracao que criou esta tabela. O indice
			// vira junto com ela quando o upgrade chegar la'.
			continue
		}
		if _, err := db.Exec(ctx, "DROP INDEX IF EXISTS "+idx.oldName); err != nil {
			return fmt.Errorf("falha ao dropar o indice legado %s: %w", idx.oldName, err)
		}
		if _, err := db.Exec(ctx, idx.create); err != nil {
			return fmt.Errorf("falha ao recriar o indice %s: %w", idx.newName, err)
		}
	}
	return nil
}
