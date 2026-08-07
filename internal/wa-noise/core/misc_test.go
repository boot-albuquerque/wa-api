package whatsmeow

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// --- disappearing_timer.go ---

func TestParseDisappearingTimerString(t *testing.T) {
	for name, tc := range map[string]struct {
		input string
		want  time.Duration
		ok    bool
	}{
		"off":                    {"off", DisappearingTimerOff, true},
		"zero":                   {"0", DisappearingTimerOff, true},
		"24h":                    {"24h", DisappearingTimer24Hours, true},
		"1 day com espaco":       {"1 day", DisappearingTimer24Hours, true},
		"maiusculas":             {"1WEEK", DisappearingTimer7Days, true},
		"7d":                     {"7d", DisappearingTimer7Days, true},
		"segundos de uma semana": {"604800s", DisappearingTimer7Days, true},
		"90d":                    {"90d", DisappearingTimer90Days, true},
		"3 months":               {"3 months", DisappearingTimer90Days, true},
		"invalido":               {"5d", 0, false},
		"vazio":                  {"", 0, false},
		"texto":                  {"para sempre", 0, false},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := ParseDisappearingTimerString(tc.input)
			if ok != tc.ok {
				t.Fatalf("ok = %v, esperado %v", ok, tc.ok)
			}
			if got != tc.want {
				t.Errorf("= %v, esperado %v", got, tc.want)
			}
		})
	}
}

// As constantes precisam bater com os segundos que os aliases numericos
// prometem, senao o parser aceita "604800s" e devolve outra coisa.
func TestDisappearingTimerConstantsMatchTheirSecondAliases(t *testing.T) {
	for alias, want := range map[string]time.Duration{
		"86400s":   DisappearingTimer24Hours,
		"604800s":  DisappearingTimer7Days,
		"7776000s": DisappearingTimer90Days,
	} {
		got, ok := ParseDisappearingTimerString(alias)
		if !ok {
			t.Fatalf("%s nao foi reconhecido", alias)
		}
		if got != want {
			t.Errorf("%s = %v, esperado %v", alias, got, want)
		}
		if seconds := strings.TrimSuffix(alias, "s"); got.Seconds() != mustParseFloat(t, seconds) {
			t.Errorf("%s: constante vale %v segundos, o alias promete %s", alias, got.Seconds(), seconds)
		}
	}
}

func mustParseFloat(t *testing.T, s string) float64 {
	t.Helper()
	var out float64
	for _, c := range s {
		if c < '0' || c > '9' {
			t.Fatalf("nao numerico: %q", s)
		}
		out = out*10 + float64(c-'0')
	}
	return out
}

func TestSetDisappearingTimerRejectsUnsupportedChatTypes(t *testing.T) {
	cli := &Client{Log: waLog.Noop}

	err := cli.SetDisappearingTimer(t.Context(), types.NewJID("123", types.NewsletterServer), DisappearingTimer24Hours, time.Time{})

	if err == nil || !strings.Contains(err.Error(), "can't set disappearing time") {
		t.Errorf("erro = %v, esperado recusa por tipo de chat", err)
	}
}

// --- update.go ---

func TestGetLatestVersionParsesClientRevision(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != latestVersionUserAgent {
			t.Errorf("User-Agent = %q, esperado o de navegador", got)
		}
		_, _ = w.Write([]byte(`{"foo":1,"client_revision":1023456,"bar":2}`))
	}))
	defer srv.Close()

	ver, err := getLatestVersionFrom(t, srv.URL)
	if err != nil {
		t.Fatalf("GetLatestVersion: %v", err)
	}
	if ver[0] != latestVersionMajor || ver[1] != latestVersionMinor || ver[2] != 1023456 {
		t.Errorf("= %v, esperado [%d %d 1023456]", ver, latestVersionMajor, latestVersionMinor)
	}
}

func TestGetLatestVersionErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		status  int
		body    string
		wantErr string
	}{
		"status inesperado":   {http.StatusInternalServerError, "boom", "unexpected response with status 500"},
		"sem client_revision": {http.StatusOK, `{"foo":1}`, "version number not found"},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			if _, err := getLatestVersionFrom(t, srv.URL); err == nil {
				t.Fatal("aceitou resposta invalida")
			} else if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("erro = %q, esperado conter %q", err, tc.wantErr)
			}
		})
	}
}

// getLatestVersionFrom roda GetLatestVersion contra um servidor local,
// redirecionando socket.Origin pelo transporte em vez de mexer na constante.
func getLatestVersionFrom(t *testing.T, target string) ([3]uint32, error) {
	t.Helper()
	client := &http.Client{Transport: rewriteHostTransport{target: target}}
	ver, err := GetLatestVersion(t.Context(), client)
	if err != nil {
		return [3]uint32{}, err
	}
	return [3]uint32{ver[0], ver[1], ver[2]}, nil
}

type rewriteHostTransport struct{ target string }

func (r rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	redirected := req.Clone(req.Context())
	parsed, err := http.NewRequest(req.Method, r.target, nil)
	if err != nil {
		return nil, err
	}
	redirected.URL = parsed.URL
	redirected.Host = parsed.Host
	return http.DefaultTransport.RoundTrip(redirected)
}

// --- request.go ---

func TestIsDisconnectNode(t *testing.T) {
	if !isDisconnectNode(xmlStreamEndNode) {
		t.Error("xmlStreamEndNode deveria ser no de desconexao")
	}
	if !isDisconnectNode(&waBinary.Node{Tag: "stream:error"}) {
		t.Error("stream:error deveria ser no de desconexao")
	}
	if isDisconnectNode(&waBinary.Node{Tag: "iq"}) {
		t.Error("iq nao deveria ser no de desconexao")
	}
	// O sentinela de fim de stream e reconhecido por *ponteiro*, nao pela tag:
	// um no com a mesma tag vindo do wire nao conta. Travado aqui porque e
	// facil "consertar" isso por engano ao mexer em isDisconnectNode.
	if isDisconnectNode(&waBinary.Node{Tag: "xmlstreamend"}) {
		t.Error("xmlstreamend por valor passou a ser reconhecido; o sentinela era comparado por ponteiro")
	}
}

// Erros de autenticacao nao devem disparar retry: reenviar o frame depois de
// reconectar so' repete a falha e atrasa o logout.
func TestIsAuthErrorDisconnect(t *testing.T) {
	streamErr := func(code string, conflictType string) *waBinary.Node {
		node := &waBinary.Node{Tag: "stream:error", Attrs: waBinary.Attrs{}}
		if code != "" {
			node.Attrs["code"] = code
		}
		if conflictType != "" {
			node.Content = []waBinary.Node{{Tag: "conflict", Attrs: waBinary.Attrs{"type": conflictType}}}
		}
		return node
	}

	for name, tc := range map[string]struct {
		node *waBinary.Node
		want bool
	}{
		"401":                     {streamErr(streamErrorAuthCode, ""), true},
		"conflito replaced":       {streamErr("", conflictTypeReplaced), true},
		"device_removed":          {streamErr("", conflictTypeDeviceRemoved), true},
		"outro codigo":            {streamErr("503", ""), false},
		"outro conflito":          {streamErr("", "unknown"), false},
		"sem codigo nem conflito": {streamErr("", ""), false},
		"nao e stream:error":      {&waBinary.Node{Tag: "iq", Attrs: waBinary.Attrs{"code": "401"}}, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := isAuthErrorDisconnect(tc.node); got != tc.want {
				t.Errorf("= %v, esperado %v", got, tc.want)
			}
		})
	}
}

func TestGenerateRequestIDIncrements(t *testing.T) {
	cli := &Client{uniqueID: "abc."}

	first, second := cli.generateRequestID(), cli.generateRequestID()

	if first == second {
		t.Fatalf("IDs repetidos: %q", first)
	}
	for _, id := range []string{first, second} {
		if !strings.HasPrefix(id, "abc.") {
			t.Errorf("%q nao comeca com o uniqueID do cliente", id)
		}
	}
	if first != "abc.1" || second != "abc.2" {
		t.Errorf("= %q/%q, esperado abc.1/abc.2", first, second)
	}
}

// --- privacysettings.go ---

func TestParsePrivacySettings(t *testing.T) {
	cli := &Client{Log: waLog.Noop}
	node := &waBinary.Node{Tag: "privacy", Content: []waBinary.Node{
		{Tag: "category", Attrs: waBinary.Attrs{"name": "groupadd", "value": "contacts"}},
		{Tag: "category", Attrs: waBinary.Attrs{"name": "last", "value": "none"}},
		// Filho que nao e' <category> deve ser ignorado sem erro.
		{Tag: "outro", Attrs: waBinary.Attrs{"name": "x"}},
	}}

	var settings types.PrivacySettings
	evt := cli.parsePrivacySettings(node, &settings)

	if settings.GroupAdd != types.PrivacySettingContacts {
		t.Errorf("GroupAdd = %q, esperado contacts", settings.GroupAdd)
	}
	if settings.LastSeen != types.PrivacySettingNone {
		t.Errorf("LastSeen = %q, esperado none", settings.LastSeen)
	}
	if !evt.GroupAddChanged || !evt.LastSeenChanged {
		t.Error("o evento deveria marcar as duas categorias como alteradas")
	}
	if evt.StatusChanged || evt.ProfileChanged {
		t.Error("categorias ausentes nao deveriam ser marcadas como alteradas")
	}
}

// Uma categoria desconhecida nao pode zerar o que ja estava preenchido nem
// marcar mudanca — o servidor introduz categorias novas antes do fork saber.
func TestParsePrivacySettingsIgnoresUnknownCategory(t *testing.T) {
	cli := &Client{Log: waLog.Noop}
	settings := types.PrivacySettings{GroupAdd: types.PrivacySettingContacts}
	node := &waBinary.Node{Tag: "privacy", Content: []waBinary.Node{
		{Tag: "category", Attrs: waBinary.Attrs{"name": "categoria_do_futuro", "value": "algo"}},
	}}

	evt := cli.parsePrivacySettings(node, &settings)

	if settings.GroupAdd != types.PrivacySettingContacts {
		t.Errorf("GroupAdd foi sobrescrito para %q", settings.GroupAdd)
	}
	if *evt != (events.PrivacySettings{}) {
		t.Errorf("categoria desconhecida marcou flags de mudanca: %+v", *evt)
	}
}

// --- broadcast.go ---

func TestDefaultStatusPrivacyIsContactsAndDefault(t *testing.T) {
	if len(DefaultStatusPrivacy) != 1 {
		t.Fatalf("len = %d, esperado 1", len(DefaultStatusPrivacy))
	}
	if DefaultStatusPrivacy[0].Type != types.StatusPrivacyTypeContacts {
		t.Errorf("Type = %q, esperado contacts", DefaultStatusPrivacy[0].Type)
	}
	if !DefaultStatusPrivacy[0].IsDefault {
		t.Error("IsDefault deveria ser true")
	}
}
