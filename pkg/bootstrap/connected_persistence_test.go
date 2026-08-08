package bootstrap

import (
	"testing"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/persistence/store"
)

// F82: uma sessão pareada por QR não sobrevivia a um restart.
//
// handleConnected saía cedo quando o pushname estava vazio — guarda legítima,
// existe para não anunciar presença sem nome —, mas levava junto o
// `UPDATE users SET connected=1`, que nada tem a ver com pushname. Num
// pareamento novo o evento Connected chega ANTES de o pushname existir, então
// a coluna ficava em 0 numa sessão viva e autenticada, e connectOnStartup
// (que itera WHERE connected=1) ignorava a sessão no start seguinte.
//
// Medido: TesteQR pareado às 07:59, vivo, sem logout; restart às 08:16 não
// produziu nenhum "Connect to Whatsapp on startup" e /admin/users reportava
// connected=False. A credencial estava intacta — faltava só a coluna.

func conectado(t *testing.T, pushName string) (*UserEventHandler, func() int) {
	t.Helper()
	sqlDB := schemaDB(t)
	seedUser(t, sqlDB, "u-f82", "tok-f82", "")

	evh := &UserEventHandler{
		UserID:   "u-f82",
		Token:    "tok-f82",
		DB:       sqlDB,
		WAClient: wanoise.NewClient(&store.Device{PushName: pushName}, nil),
	}
	lerColuna := func() int {
		var conectado int
		if err := sqlDB.Get(&conectado, `SELECT connected FROM users WHERE id=$1`, "u-f82"); err != nil {
			t.Fatalf("ler users.connected: %v", err)
		}
		return conectado
	}
	return evh, lerColuna
}

// TestHandleConnected_PersisteMesmoSemPushName é o teste da F82. O pushname
// vazio é exatamente o estado de um pareamento recém-concluído.
func TestHandleConnected_PersisteMesmoSemPushName(t *testing.T) {
	evh, lerColuna := conectado(t, "")

	if ok := evh.handleConnected(&eventState{postmap: map[string]interface{}{}}); !ok {
		t.Fatal("handleConnected devolveu false no caminho feliz")
	}

	if got := lerColuna(); got != 1 {
		t.Errorf("users.connected = %d, quero 1 — a sessao nao sobreviveria a um restart", got)
	}
}

// TestHandleConnected_PersisteComPushName: o caminho que já funcionava tem de
// continuar funcionando. Sem ele, mover a escrita poderia ter quebrado o caso
// que ninguém suspeitava.
func TestHandleConnected_PersisteComPushName(t *testing.T) {
	evh, lerColuna := conectado(t, "Alice")

	if ok := evh.handleConnected(&eventState{postmap: map[string]interface{}{}}); !ok {
		t.Fatal("handleConnected devolveu false no caminho feliz")
	}

	if got := lerColuna(); got != 1 {
		t.Errorf("users.connected = %d, quero 1", got)
	}
}

// TestHandleConnected_MarcaWebhookNosDoisCasos: dowebhook=1 é anterior a tudo
// e vale com ou sem pushname. O comentário histórico da função registra que
// trocar a saída antecipada por `return false` silenciaria o evento Connected
// de toda sessão sem pushname — esta asserção impede que isso volte.
func TestHandleConnected_MarcaWebhookNosDoisCasos(t *testing.T) {
	for _, pushName := range []string{"", "Alice"} {
		t.Run("pushname="+pushName, func(t *testing.T) {
			evh, _ := conectado(t, pushName)
			st := &eventState{postmap: map[string]interface{}{}}

			evh.handleConnected(st)

			if st.dowebhook != 1 {
				t.Errorf("dowebhook = %d, quero 1", st.dowebhook)
			}
			if st.postmap["type"] != "Connected" {
				t.Errorf(`postmap["type"] = %v, quero "Connected"`, st.postmap["type"])
			}
		})
	}
}
