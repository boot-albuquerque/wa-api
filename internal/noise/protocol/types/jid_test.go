package types

import (
	"database/sql/driver"
	"encoding/json"
	"testing"
)

// ParseJID e String tem que ser inversas. E' o invariante mais importante do
// pacote: o JID e' a chave primaria de tudo (sessao, store, roteamento de
// mensagem), e um round trip que perde um campo manda a mensagem para outro
// destinatario.
func TestParseJIDIsInverseOfString(t *testing.T) {
	tests := []struct {
		name string
		jid  JID
	}{
		{"usuario", NewJID("5511999999999", DefaultUserServer)},
		{"grupo", NewJID("120363000000000000", GroupServer)},
		{"broadcast", NewJID("status", BroadcastServer)},
		{"newsletter", NewJID("123", NewsletterServer)},
		{"lid", NewJID("123", HiddenUserServer)},
		{"bot", NewJID("867051314767696", BotServer)},
		{"so servidor", NewJID("", DefaultUserServer)},
		{"com device", JID{User: "5511999999999", Device: 12, Server: DefaultUserServer}},
		{"com agente e device", JID{User: "5511999999999", RawAgent: 3, Device: 12, Server: DefaultUserServer}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ParseJID(tc.jid.String())
			if err != nil {
				t.Fatalf("ParseJID(%q): %v", tc.jid.String(), err)
			}
			if parsed != tc.jid {
				t.Errorf("round trip de %q: %+v != %+v", tc.jid.String(), parsed, tc.jid)
			}
		})
	}
}

// String tem quatro formatos, escolhidos por quais campos estao preenchidos.
// Emitir o formato errado produz uma string que ParseJID le' como outro JID.
func TestStringPicksFormatByPopulatedFields(t *testing.T) {
	tests := []struct {
		name string
		jid  JID
		want string
	}{
		{"so servidor", JID{Server: GroupServer}, "g.us"},
		{"user e servidor", JID{User: "123", Server: GroupServer}, "123@g.us"},
		{"com device", JID{User: "123", Device: 5, Server: DefaultUserServer}, "123:5@s.whatsapp.net"},
		{"com agente", JID{User: "123", RawAgent: 2, Device: 5, Server: DefaultUserServer}, "123.2:5@s.whatsapp.net"},
		{"agente sem device", JID{User: "123", RawAgent: 2, Server: DefaultUserServer}, "123.2:0@s.whatsapp.net"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.jid.String(); got != tc.want {
				t.Errorf("= %q, esperado %q", got, tc.want)
			}
		})
	}
}

// ADString sempre emite o formato completo, mesmo com agente e device zerados.
// String faria o contrario — e' a diferenca entre os dois metodos.
func TestADStringAlwaysEmitsAgentAndDevice(t *testing.T) {
	jid := NewJID("123", DefaultUserServer)
	if got := jid.ADString(); got != "123.0:0@s.whatsapp.net" {
		t.Errorf("ADString = %q", got)
	}
	if got := jid.String(); got != "123@s.whatsapp.net" {
		t.Errorf("String = %q, deveria omitir agente e device zerados", got)
	}
}

func TestParseJIDAcceptsBothSeparatorForms(t *testing.T) {
	tests := []struct {
		input string
		want  JID
	}{
		{"5511999999999@s.whatsapp.net", JID{User: "5511999999999", Server: DefaultUserServer}},
		{"5511999999999:3@s.whatsapp.net", JID{User: "5511999999999", Device: 3, Server: DefaultUserServer}},
		{"5511999999999.2:3@s.whatsapp.net", JID{User: "5511999999999", RawAgent: 2, Device: 3, Server: DefaultUserServer}},
		{"5511999999999.2@s.whatsapp.net", JID{User: "5511999999999", RawAgent: 2, Server: DefaultUserServer}},
		{"s.whatsapp.net", JID{Server: DefaultUserServer}},
		{"@s.whatsapp.net", JID{Server: DefaultUserServer}},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseJID(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("= %+v, esperado %+v", got, tc.want)
			}
		})
	}
}

func TestParseJIDRejectsMalformedInput(t *testing.T) {
	for _, input := range []string{
		"a.b.c@s.whatsapp.net", // pontos demais
		"a:1:2@s.whatsapp.net", // dois-pontos demais
		"a.x:1@s.whatsapp.net", // agente nao numerico
		"a.1:x@s.whatsapp.net", // device nao numerico depois de ponto
		"a:x@s.whatsapp.net",   // device nao numerico
	} {
		if _, err := ParseJID(input); err == nil {
			t.Errorf("ParseJID(%q) nao falhou", input)
		}
	}
}

// ActualAgent traduz o servidor para o codigo de dominio que vai no fio. E' a
// tabela que o encoder binario consulta, e um mapeamento errado manda a
// mensagem para o dominio errado.
func TestActualAgentMapsServerToDomain(t *testing.T) {
	tests := []struct {
		jid  JID
		want uint8
	}{
		{JID{Server: DefaultUserServer}, WhatsAppDomain},
		{JID{Server: HiddenUserServer}, LIDDomain},
		{JID{Server: HostedServer}, HostedDomain},
		{JID{Server: HostedLIDServer}, HostedLIDDomain},
		// Servidor sem dominio proprio devolve o agente cru.
		{JID{Server: GroupServer, RawAgent: 7}, 7},
		{JID{Server: BroadcastServer, RawAgent: 0}, 0},
	}
	for _, tc := range tests {
		if got := tc.jid.ActualAgent(); got != tc.want {
			t.Errorf("%q com RawAgent %d: ActualAgent = %d, esperado %d", tc.jid.Server, tc.jid.RawAgent, got, tc.want)
		}
	}
}

// NewADJID e ActualAgent sao as duas metades da mesma traducao. Quando o
// dominio tem servidor proprio, o agente e' ZERADO — guardar os dois faria o
// JID emitir o agente na string e virar outro JID.
func TestNewADJIDTranslatesDomainToServerAndClearsAgent(t *testing.T) {
	tests := []struct {
		domain uint8
		server string
	}{
		{WhatsAppDomain, DefaultUserServer},
		{LIDDomain, HiddenUserServer},
		{HostedDomain, HostedServer},
		{HostedLIDDomain, HostedLIDServer},
	}
	for _, tc := range tests {
		jid := NewADJID("123", tc.domain, 5)
		if jid.Server != tc.server {
			t.Errorf("dominio %d: servidor = %q, esperado %q", tc.domain, jid.Server, tc.server)
		}
		if jid.RawAgent != 0 {
			t.Errorf("dominio %d: RawAgent = %d, esperado 0", tc.domain, jid.RawAgent)
		}
		if jid.Device != 5 {
			t.Errorf("dominio %d: Device = %d", tc.domain, jid.Device)
		}
		// E a volta reproduz o dominio de origem.
		if got := jid.ActualAgent(); got != tc.domain {
			t.Errorf("dominio %d nao sobreviveu ao ida e volta: %d", tc.domain, got)
		}
	}
}

// Um agente que NAO e' um dominio conhecido e' preservado como agente, e o
// servidor fica vazio. E' o ramo default do switch.
func TestNewADJIDKeepsUnknownAgentAsAgent(t *testing.T) {
	jid := NewADJID("123", 42, 1)
	if jid.RawAgent != 42 {
		t.Errorf("RawAgent = %d, esperado 42", jid.RawAgent)
	}
	if jid.Server != "" {
		t.Errorf("Server = %q, esperado vazio", jid.Server)
	}
	if got := jid.ActualAgent(); got != 42 {
		t.Errorf("ActualAgent = %d, esperado 42", got)
	}
}

// SignalAddressUser sufixa o agente quando ele nao e' zero. E' a chave de
// sessao do libsignal: dois JIDs que colidissem aqui compartilhariam sessao
// cripto, que e' a pior consequencia possivel neste pacote.
func TestSignalAddressUserSeparatesDomains(t *testing.T) {
	pn := NewJID("5511999999999", DefaultUserServer)
	lid := NewJID("5511999999999", HiddenUserServer)

	if got := pn.SignalAddressUser(); got != "5511999999999" {
		t.Errorf("PN = %q, esperado sem sufixo", got)
	}
	if got := lid.SignalAddressUser(); got != "5511999999999_1" {
		t.Errorf("LID = %q, esperado sufixo _1", got)
	}
	if pn.SignalAddressUser() == lid.SignalAddressUser() {
		t.Error("o mesmo numero em PN e LID colidiu no endereco Signal")
	}
}

func TestSignalAddressCarriesTheDevice(t *testing.T) {
	jid := JID{User: "123", Device: 7, Server: DefaultUserServer}
	addr := jid.SignalAddress()
	if addr.Name() != "123" {
		t.Errorf("nome = %q", addr.Name())
	}
	if addr.DeviceID() != 7 {
		t.Errorf("device = %d", addr.DeviceID())
	}
}

func TestToNonADDropsAgentAndDevice(t *testing.T) {
	jid := JID{User: "123", RawAgent: 2, Device: 5, Integrator: 9, Server: DefaultUserServer}
	got := jid.ToNonAD()
	if got.RawAgent != 0 || got.Device != 0 {
		t.Errorf("agente/device nao foram zerados: %+v", got)
	}
	// Integrator e' preservado: identifica o parceiro interop, nao o
	// dispositivo, e derruba-lo perderia a origem da mensagem.
	if got.Integrator != 9 {
		t.Errorf("Integrator = %d, esperado 9", got.Integrator)
	}
	if got.User != "123" || got.Server != DefaultUserServer {
		t.Errorf("= %+v", got)
	}
}

// IsBroadcastList tem que excluir o broadcast de STATUS. Sao coisas
// diferentes: um e' uma lista de transmissao do usuario, o outro e' o feed de
// status, e trata-los igual mandaria status para a lista errada.
func TestIsBroadcastListExcludesStatus(t *testing.T) {
	if !NewJID("123", BroadcastServer).IsBroadcastList() {
		t.Error("uma lista de transmissao deveria ser reconhecida")
	}
	if StatusBroadcastJID.IsBroadcastList() {
		t.Error("o broadcast de status nao e' lista de transmissao")
	}
	if NewJID("123", GroupServer).IsBroadcastList() {
		t.Error("grupo nao e' lista de transmissao")
	}
}

func TestIsBotRecognisesBothForms(t *testing.T) {
	if !NewJID("867051314767696", BotServer).IsBot() {
		t.Error("JID no servidor bot deveria ser bot")
	}
	if !MetaAIJID.IsBot() {
		t.Errorf("%s deveria ser bot pelo padrao de numero", MetaAIJID)
	}
	if !NewJID("13165550012", DefaultUserServer).IsBot() {
		t.Error("a segunda faixa de numeros de bot nao foi reconhecida")
	}
	// Um numero da faixa mas com device nao e' bot: bots nao tem
	// dispositivos.
	if (JID{User: "13135550002", Device: 1, Server: DefaultUserServer}).IsBot() {
		t.Error("bot com device deveria ser rejeitado")
	}
	if NewJID("5511999999999", DefaultUserServer).IsBot() {
		t.Error("numero comum foi classificado como bot")
	}
	if NewJID("1313555000", DefaultUserServer).IsBot() {
		t.Error("numero curto demais casou com o padrao")
	}
}

func TestUserIntParsesOnlyNumericUsers(t *testing.T) {
	if got := NewJID("5511999999999", DefaultUserServer).UserInt(); got != 5511999999999 {
		t.Errorf("= %d", got)
	}
	// Usuario nao numerico (grupo, status) devolve 0 em vez de falhar.
	if got := StatusBroadcastJID.UserInt(); got != 0 {
		t.Errorf("usuario nao numerico = %d, esperado 0", got)
	}
}

func TestIsEmptyChecksTheServer(t *testing.T) {
	if !EmptyJID.IsEmpty() {
		t.Error("EmptyJID deveria ser vazio")
	}
	// Um JID com user mas sem servidor tambem e' vazio: servidor e'
	// obrigatorio, user nao.
	if !(JID{User: "123"}).IsEmpty() {
		t.Error("JID sem servidor deveria ser vazio")
	}
	if NewJID("", GroupServer).IsEmpty() {
		t.Error("JID so' com servidor nao e' vazio")
	}
}

func TestMarshalAndUnmarshalText(t *testing.T) {
	jid := JID{User: "5511999999999", Device: 3, Server: DefaultUserServer}

	text, err := jid.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	if string(text) != jid.String() {
		t.Errorf("= %q, esperado %q", text, jid.String())
	}

	var back JID
	if err := back.UnmarshalText(text); err != nil {
		t.Fatal(err)
	}
	if back != jid {
		t.Errorf("= %+v, esperado %+v", back, jid)
	}
	if err := back.UnmarshalText([]byte("a.b.c@s.whatsapp.net")); err == nil {
		t.Error("texto malformado deveria falhar")
	}
}

func TestJIDRoundTripsThroughJSON(t *testing.T) {
	type wrapper struct {
		JID JID `json:"jid"`
	}
	original := wrapper{JID: JID{User: "123", Device: 4, Server: HiddenUserServer}}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var back wrapper
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.JID != original.JID {
		t.Errorf("= %+v, esperado %+v", back.JID, original.JID)
	}
}

// Scan e Value sao o par que persiste o JID no banco. Value de um JID vazio
// tem que dar NULL, e Scan de NULL tem que deixar o JID intocado — senao uma
// coluna opcional vazia viraria um JID invalido em vez de ausente.
func TestScanAndValueRoundTripThroughSQL(t *testing.T) {
	jid := JID{User: "5511999999999", Device: 2, Server: DefaultUserServer}

	value, err := jid.Value()
	if err != nil {
		t.Fatal(err)
	}
	if value != driver.Value(jid.String()) {
		t.Errorf("Value = %v", value)
	}

	var fromString JID
	if err := fromString.Scan(jid.String()); err != nil || fromString != jid {
		t.Errorf("Scan de string = (%+v, %v)", fromString, err)
	}
	var fromBytes JID
	if err := fromBytes.Scan([]byte(jid.String())); err != nil || fromBytes != jid {
		t.Errorf("Scan de []byte = (%+v, %v)", fromBytes, err)
	}
}

func TestEmptyJIDValueIsNull(t *testing.T) {
	value, err := EmptyJID.Value()
	if err != nil {
		t.Fatal(err)
	}
	if value != nil {
		t.Errorf("Value de JID vazio = %v, esperado nil", value)
	}
}

func TestScanOfNullLeavesTheJIDUntouched(t *testing.T) {
	jid := NewJID("123", GroupServer)
	if err := jid.Scan(nil); err != nil {
		t.Fatal(err)
	}
	if jid != NewJID("123", GroupServer) {
		t.Errorf("Scan(nil) alterou o JID: %+v", jid)
	}
}

func TestScanRejectsUnsupportedTypesAndMalformedText(t *testing.T) {
	var jid JID
	if err := jid.Scan(42); err == nil {
		t.Error("Scan de int deveria falhar")
	}
	if err := jid.Scan("a.b.c@s.whatsapp.net"); err == nil {
		t.Error("Scan de texto malformado deveria falhar")
	}
}

// Os JIDs pre-construidos do pacote sao usados como constantes pelo resto do
// codigo. Um erro de digitacao aqui nao quebra compilacao, so' roteia errado.
func TestWellKnownJIDsAreWellFormed(t *testing.T) {
	tests := map[string]struct {
		jid  JID
		want string
	}{
		"ServerJID":           {ServerJID, "s.whatsapp.net"},
		"GroupServerJID":      {GroupServerJID, "g.us"},
		"BroadcastServerJID":  {BroadcastServerJID, "broadcast"},
		"StatusBroadcastJID":  {StatusBroadcastJID, "status@broadcast"},
		"PSAJID":              {PSAJID, "0@s.whatsapp.net"},
		"LegacyPSAJID":        {LegacyPSAJID, "0@c.us"},
		"OfficialBusinessJID": {OfficialBusinessJID, "16505361212@c.us"},
		"MetaAIJID":           {MetaAIJID, "13135550002@s.whatsapp.net"},
		"NewMetaAIJID":        {NewMetaAIJID, "867051314767696@bot"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tc.jid.String(); got != tc.want {
				t.Errorf("= %q, esperado %q", got, tc.want)
			}
			parsed, err := ParseJID(tc.want)
			if err != nil || parsed != tc.jid {
				t.Errorf("nao releu: (%+v, %v)", parsed, err)
			}
		})
	}
}

// Os quatro dominios tem que ser distintos entre si: sao o discriminador do
// formato AD no fio, e dois iguais fundiriam duas populacoes de usuario.
func TestDomainCodesAreDistinct(t *testing.T) {
	seen := map[uint8]string{}
	for name, code := range map[string]uint8{
		"WhatsAppDomain":  WhatsAppDomain,
		"LIDDomain":       LIDDomain,
		"HostedDomain":    HostedDomain,
		"HostedLIDDomain": HostedLIDDomain,
	} {
		if other, dup := seen[code]; dup {
			t.Errorf("%s e %s tem o mesmo codigo %d", name, other, code)
		}
		seen[code] = name
	}
}
