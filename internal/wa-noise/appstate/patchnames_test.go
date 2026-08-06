package appstate

import "testing"

func TestAllPatchNamesCoversEveryDeclaredName(t *testing.T) {
	declared := []WAPatchName{
		WAPatchCriticalBlock,
		WAPatchCriticalUnblockLow,
		WAPatchRegularLow,
		WAPatchRegularHigh,
		WAPatchRegular,
	}
	if len(AllPatchNames) != len(declared) {
		t.Fatalf("len(AllPatchNames) = %d, esperado %d", len(AllPatchNames), len(declared))
	}
	for _, name := range declared {
		var found bool
		for _, candidate := range AllPatchNames {
			if candidate == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AllPatchNames nao contem %q", name)
		}
	}
}

func TestPatchNamesAreDistinctAndNonEmpty(t *testing.T) {
	seen := map[WAPatchName]bool{}
	for _, name := range AllPatchNames {
		if name == "" {
			t.Error("nome de patch vazio em AllPatchNames")
		}
		if seen[name] {
			t.Errorf("nome de patch duplicado: %q", name)
		}
		seen[name] = true
	}
}

// TestIndexConstantsAreDistinct trava que nenhum dos indices nomeados colide
// com outro — um indice duplicado faria duas mutacoes diferentes competirem
// pelo mesmo index MAC no estado.
func TestIndexConstantsAreDistinct(t *testing.T) {
	indexes := map[string]string{
		"IndexPin":                     IndexPin,
		"IndexArchive":                 IndexArchive,
		"IndexMarkChatAsRead":          IndexMarkChatAsRead,
		"IndexLabelAssociationMessage": IndexLabelAssociationMessage,
		"IndexLabelEdit":               IndexLabelEdit,
		"IndexLabelAssociationChat":    IndexLabelAssociationChat,
		"IndexStar":                    IndexStar,
		"IndexMute":                    IndexMute,
		"IndexDeleteChat":              IndexDeleteChat,
		"IndexContact":                 IndexContact,
		"IndexLIDContact":              IndexLIDContact,
		"IndexSettingPushName":         IndexSettingPushName,
		"IndexSettingLocale":           IndexSettingLocale,
	}
	seen := map[string]string{}
	for name, value := range indexes {
		if value == "" {
			t.Errorf("%s esta vazio", name)
		}
		if other, dup := seen[value]; dup {
			t.Errorf("%s e %s tem o mesmo valor %q", name, other, value)
		}
		seen[value] = name
	}
}
