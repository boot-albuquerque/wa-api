package upgrades

import (
	"embed"

	"go.mau.fi/util/dbutil"
)

var Table dbutil.UpgradeTable

//go:embed *.sql
var upgrades embed.FS

func init() {
	Table.RegisterFS(upgrades)
}
