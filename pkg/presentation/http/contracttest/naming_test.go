package contracttest

import (
	"fmt"
	"strings"
	"testing"
)

// Um helper de asserção que nunca falha é pior que nenhum: dá confiança falsa
// às seis migrações que o vão chamar. Estes testes são o CONTROLO NEGATIVO
// permanente dele — cada um alimenta um corpo que TEM de ser recusado, e
// verifica que foi.

// captureT é um *testing.T de mentira: regista as falhas em vez de as
// propagar, para que o teste possa afirmar que a falha aconteceu.
type captureT struct {
	failures []string
	fatal    bool
}

var _ TestingT = (*captureT)(nil)

func (c *captureT) Helper() {}
func (c *captureT) Errorf(format string, args ...any) {
	c.failures = append(c.failures, fmt.Sprintf(format, args...))
}
func (c *captureT) Fatalf(format string, args ...any) {
	c.fatal = true
	c.failures = append(c.failures, fmt.Sprintf(format, args...))
}
func (c *captureT) all() string { return strings.Join(c.failures, "\n") }

func TestIsCanonicalKey(t *testing.T) {
	bons := []string{"jid", "group_jid", "is_admin", "participant_count", "sha256", "avatar_url", "x", "a1_b2"}
	for _, k := range bons {
		if !IsCanonicalKey(k) {
			t.Errorf("IsCanonicalKey(%q) = false, quero true", k)
		}
	}
	maus := []string{
		"GroupJID",   // PascalCase, o que o encoder produz sem tag
		"groupJid",   // camelCase
		"group-jid",  // kebab
		"group__jid", // duplo underscore
		"_group",     // começa por underscore
		"group_",     // termina em underscore
		"Group_JID",  // maiúscula no meio
		"1group",     // começa por dígito
		"",           // vazio
	}
	for _, k := range maus {
		if IsCanonicalKey(k) {
			t.Errorf("IsCanonicalKey(%q) = true, quero false", k)
		}
	}
}

// TestAssertPublicJSONUsesCanonicalNaming_Morde é o controlo negativo do
// caminho recursivo: a chave errada está DENTRO de um objecto dentro de um
// array, que é o lugar onde uma verificação superficial não olharia.
func TestAssertPublicJSONUsesCanonicalNaming_Morde(t *testing.T) {
	casos := map[string]string{
		"raiz":                    `{"GroupJID":"x"}`,
		"objecto aninhado":        `{"data":{"group_info":{"OwnerJID":"x"}}}`,
		"dentro de array":         `{"participants":[{"jid":"a"},{"IsAdmin":true}]}`,
		"array dentro de array":   `{"m":[[{"badKey":1}]]}`,
		"camelCase discreto":      `{"data":{"nextCursor":"x"}}`,
		"kebab que parece inócuo": `{"data":{"next-cursor":"x"}}`,
	}
	for nome, corpo := range casos {
		t.Run(nome, func(t *testing.T) {
			c := &captureT{}
			AssertPublicJSONUsesCanonicalNaming(c, []byte(corpo))
			if len(c.failures) == 0 {
				t.Fatalf("o helper NÃO recusou %s — um helper que não morde é confiança falsa", corpo)
			}
			if !strings.Contains(c.all(), "$.") {
				t.Errorf("a falha não nomeia o caminho onde a chave está:\n%s", c.all())
			}
		})
	}
}

func TestAssertPublicJSONUsesCanonicalNaming_AceitaOCorpoCerto(t *testing.T) {
	c := &captureT{}
	AssertPublicJSONUsesCanonicalNaming(c, []byte(`{"success":true,"code":200,"data":{"group_info":{"jid":"x","participants":[{"is_admin":true,"phone_number":"y"}],"created_at":null}}}`))
	if len(c.failures) > 0 {
		t.Fatalf("recusou um corpo canónico:\n%s", c.all())
	}
}

func TestAssertNoKeys_Morde(t *testing.T) {
	c := &captureT{}
	AssertNoKeys(c, []byte(`{"data":{"group_info":{"jid":"x","Participants":[]}}}`), "Participants")
	if len(c.failures) == 0 {
		t.Fatal("AssertNoKeys não recusou uma chave banida presente no corpo")
	}

	ok := &captureT{}
	AssertNoKeys(ok, []byte(`{"data":{"group_info":{"jid":"x","participants":[]}}}`), "Participants")
	if len(ok.failures) > 0 {
		t.Fatalf("AssertNoKeys recusou um corpo limpo:\n%s", ok.all())
	}
}
