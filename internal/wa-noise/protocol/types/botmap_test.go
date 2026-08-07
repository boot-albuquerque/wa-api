package types

import "testing"

// BotJIDMap traduz o JID novo dos bots (no servidor `bot`) para o numero
// legado em s.whatsapp.net. E' uma tabela de dados escrita a mao, e as duas
// invariantes que uma tabela assim quebra sao chave duplicada (impossivel num
// literal de map em Go, o compilador barra) e VALOR duplicado — dois bots
// diferentes apontando para o mesmo numero legado, que fundiria as conversas.
func TestBotJIDMapHasNoDuplicateTargets(t *testing.T) {
	seen := make(map[JID]JID, len(BotJIDMap))
	for bot, legacy := range BotJIDMap {
		if other, dup := seen[legacy]; dup {
			t.Errorf("%s e %s apontam ambos para %s", bot, other, legacy)
		}
		seen[legacy] = bot
	}
}

// Toda chave tem que estar no servidor `bot` e todo valor no servidor de
// usuario: a tabela existe para atravessar essa fronteira, e uma entrada no
// servidor errado nunca seria encontrada.
func TestBotJIDMapServersAreConsistent(t *testing.T) {
	if len(BotJIDMap) == 0 {
		t.Fatal("BotJIDMap esta vazio")
	}
	for bot, legacy := range BotJIDMap {
		if bot.Server != BotServer {
			t.Errorf("chave %s nao esta em %q", bot, BotServer)
		}
		if legacy.Server != DefaultUserServer {
			t.Errorf("valor %s nao esta em %q", legacy, DefaultUserServer)
		}
		if bot.Device != 0 || legacy.Device != 0 {
			t.Errorf("entrada %s -> %s tem device preenchido", bot, legacy)
		}
	}
}

// Todo numero legado da tabela tem que ser reconhecido por IsBot. Se a regex de
// IsBot e a tabela divergirem, um bot conhecido deixa de ser tratado como bot.
func TestEveryLegacyBotNumberIsRecognisedAsBot(t *testing.T) {
	for bot, legacy := range BotJIDMap {
		if !legacy.IsBot() {
			t.Errorf("%s (de %s) nao e' reconhecido por IsBot", legacy, bot)
		}
		if !bot.IsBot() {
			t.Errorf("%s nao e' reconhecido por IsBot", bot)
		}
	}
}

// Os dois JIDs pre-declarados da Meta AI tem que estar na tabela e apontar um
// para o outro.
func TestMetaAIJIDsAreLinkedInTheMap(t *testing.T) {
	got, ok := BotJIDMap[NewMetaAIJID]
	if !ok {
		t.Fatalf("%s nao esta em BotJIDMap", NewMetaAIJID)
	}
	if got != MetaAIJID {
		t.Errorf("%s aponta para %s, esperado %s", NewMetaAIJID, got, MetaAIJID)
	}
}
