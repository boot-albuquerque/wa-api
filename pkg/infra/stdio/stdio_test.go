package stdio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// O transporte stdio e' JSON-RPC sobre io.Reader/io.Writer: NewServerWithIO e'
// a costura que permite exercitar o servidor inteiro sem processo externo e
// sem tocar em os.Stdin/os.Stdout. As unicas excecoes sao os testes de
// SendNotification, que escreve em os.Stdout por contrato (webhook em modo
// stdio) e por isso troca o descritor global — todos sequenciais, sem t.Parallel.

// recordingRouter captura a requisicao HTTP que o adaptador stdio sintetiza.
type recordingRouter struct {
	calls    int
	method   string
	path     string
	rawQuery string
	body     string
	token    string
	auth     string
	ctype    string

	status   int
	respBody string
}

func (r *recordingRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.calls++
	r.method = req.Method
	r.path = req.URL.Path
	r.rawQuery = req.URL.RawQuery
	r.token = req.Header.Get("token")
	r.auth = req.Header.Get("Authorization")
	r.ctype = req.Header.Get("Content-Type")
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		r.body = string(b)
	}
	if r.status != 0 {
		w.WriteHeader(r.status)
	}
	if r.respBody != "" {
		_, _ = io.WriteString(w, r.respBody)
	}
}

// newTestServer devolve o servidor e o buffer de stdout.
func newTestServer(t *testing.T, router interface{}) (*Server, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	return NewServerWithIO(router, strings.NewReader(""), out), out
}

// decodeOne le a unica resposta escrita no buffer.
func decodeOne(t *testing.T, out *bytes.Buffer) map[string]interface{} {
	t.Helper()
	lines := splitLines(out.String())
	if len(lines) != 1 {
		t.Fatalf("esperava 1 resposta, veio %d: %q", len(lines), out.String())
	}
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("resposta nao e' JSON: %v (%q)", err, lines[0])
	}
	return got
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func rpcError(t *testing.T, resp map[string]interface{}) (int, string) {
	t.Helper()
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("esperava campo error, veio %v", resp)
	}
	code, _ := errObj["code"].(float64)
	msg, _ := errObj["message"].(string)
	return int(code), msg
}

// --- ID ---------------------------------------------------------------------

func TestID_MarshalUnmarshalRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantJSON string
		wantStr  string
	}{
		{name: "numerico", raw: `7`, wantJSON: `7`, wantStr: "7"},
		{name: "string", raw: `"abc"`, wantJSON: `"abc"`, wantStr: "abc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var id ID
			if err := json.Unmarshal([]byte(tc.raw), &id); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !id.IsSet {
				t.Fatal("IsSet deveria ser true")
			}
			if got := id.String(); got != tc.wantStr {
				t.Fatalf("String() = %q, quero %q", got, tc.wantStr)
			}
			b, err := json.Marshal(id)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(b) != tc.wantJSON {
				t.Fatalf("Marshal = %s, quero %s", b, tc.wantJSON)
			}
		})
	}
}

func TestID_UnmarshalRejectsNonScalar(t *testing.T) {
	var id ID
	if err := json.Unmarshal([]byte(`{"a":1}`), &id); err == nil {
		t.Fatal("esperava erro para id objeto")
	}
	if id.IsSet {
		t.Fatal("id nao deveria ter sido marcado como set")
	}
}

func TestID_MarshalUnsetIsNull(t *testing.T) {
	b, err := json.Marshal(ID{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(b) != "null" {
		t.Fatalf("Marshal = %s, quero null", b)
	}
}

// --- Start ------------------------------------------------------------------

func TestStart_ProcessaLinhasEIgnoraVazias(t *testing.T) {
	router := &recordingRouter{respBody: `{"data":{"ok":true}}`}
	in := strings.NewReader("\n" + `{"id":1,"method":"health"}` + "\n\n" + `{"id":"x","method":"health"}` + "\n")
	out := &bytes.Buffer{}
	ss := NewServerWithIO(router, in, out)

	if err := ss.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if router.calls != 2 {
		t.Fatalf("router chamado %d vezes, quero 2", router.calls)
	}
	if got := len(splitLines(out.String())); got != 2 {
		t.Fatalf("respostas = %d, quero 2", got)
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestStart_PropagaErroDeLeitura(t *testing.T) {
	want := errors.New("stdin quebrado")
	ss := NewServerWithIO(&recordingRouter{}, errReader{err: want}, &bytes.Buffer{})

	err := ss.Start()
	if !errors.Is(err, want) {
		t.Fatalf("Start err = %v, quero %v", err, want)
	}
}

// --- handleRequest ----------------------------------------------------------

func TestHandleRequest_Rejeicoes(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantCode int
		wantMsg  string
	}{
		{name: "json invalido", line: `{nao e json`, wantCode: 400, wantMsg: "invalid JSON request"},
		{name: "sem id", line: `{"method":"health"}`, wantCode: 400, wantMsg: "missing request id"},
		{name: "sem metodo", line: `{"id":3}`, wantCode: 400, wantMsg: "missing method"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := &recordingRouter{}
			ss, out := newTestServer(t, router)

			ss.handleRequest([]byte(tc.line))

			code, msg := rpcError(t, decodeOne(t, out))
			if code != tc.wantCode {
				t.Fatalf("code = %d, quero %d", code, tc.wantCode)
			}
			if !strings.Contains(msg, tc.wantMsg) {
				t.Fatalf("mensagem = %q, quero conter %q", msg, tc.wantMsg)
			}
			if router.calls != 0 {
				t.Fatal("router nao deveria ter sido chamado")
			}
		})
	}
}

// --- roteamento -------------------------------------------------------------

func TestRouteRequest_RotaEstaticaComTokens(t *testing.T) {
	router := &recordingRouter{respBody: `{"data":"ok"}`}
	ss, out := newTestServer(t, router)

	ss.handleRequest([]byte(`{"id":1,"method":"chat.send.text","params":{"token":"T","adminToken":"A","Phone":"55"}}`))

	if router.method != http.MethodPost || router.path != "/chat/send/text" {
		t.Fatalf("rota = %s %s", router.method, router.path)
	}
	if router.token != "T" || router.auth != "A" {
		t.Fatalf("headers token=%q auth=%q", router.token, router.auth)
	}
	if router.ctype != "application/json" {
		t.Fatalf("content-type = %q", router.ctype)
	}
	if !strings.Contains(router.body, `"Phone":"55"`) {
		t.Fatalf("body = %q", router.body)
	}
	if resp := decodeOne(t, out); resp["result"] != "ok" {
		t.Fatalf("result = %v", resp["result"])
	}
}

func TestRouteRequest_SemParamsNaoEnviaCorpo(t *testing.T) {
	router := &recordingRouter{respBody: `{"data":null}`}
	ss, _ := newTestServer(t, router)

	ss.handleRequest([]byte(`{"id":1,"method":"health"}`))

	if router.method != http.MethodGet || router.path != "/health" {
		t.Fatalf("rota = %s %s", router.method, router.path)
	}
	if router.body != "" {
		t.Fatalf("body deveria ser vazio, veio %q", router.body)
	}
}

func TestRouteRequest_RotasDinamicas(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantPath  string
		wantQuery string
		wantVerb  string
	}{
		{
			name:     "admin get",
			line:     `{"id":1,"method":"admin.users.get","params":{"userId":"u9"}}`,
			wantPath: "/admin/users/u9", wantVerb: http.MethodGet,
		},
		{
			name:     "admin delete full",
			line:     `{"id":1,"method":"admin.users.delete.full","params":{"userId":"u9"}}`,
			wantPath: "/admin/users/u9/full", wantVerb: http.MethodDelete,
		},
		{
			name:     "user lid",
			line:     `{"id":1,"method":"user.lid","params":{"jid":"55@s.whatsapp.net"}}`,
			wantPath: "/user/lid/55@s.whatsapp.net", wantVerb: http.MethodGet,
		},
		{
			name:     "chat history sem limit",
			line:     `{"id":1,"method":"chat.history","params":{"chat_jid":"c@s.whatsapp.net"}}`,
			wantPath: "/chat/history", wantQuery: "chat_jid=c@s.whatsapp.net", wantVerb: http.MethodGet,
		},
		{
			name:     "chat history com limit",
			line:     `{"id":1,"method":"chat.history","params":{"chat_jid":"c@s.whatsapp.net","limit":25}}`,
			wantPath: "/chat/history", wantQuery: "chat_jid=c@s.whatsapp.net&limit=25", wantVerb: http.MethodGet,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := &recordingRouter{respBody: `{"data":"ok"}`}
			ss, _ := newTestServer(t, router)

			ss.handleRequest([]byte(tc.line))

			if router.path != tc.wantPath {
				t.Fatalf("path = %q, quero %q", router.path, tc.wantPath)
			}
			if router.rawQuery != tc.wantQuery {
				t.Fatalf("query = %q, quero %q", router.rawQuery, tc.wantQuery)
			}
			if router.method != tc.wantVerb {
				t.Fatalf("verbo = %q, quero %q", router.method, tc.wantVerb)
			}
		})
	}
}

func TestRouteRequest_ParamObrigatorioAusente(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		param string
	}{
		{name: "admin sem userId", line: `{"id":1,"method":"admin.users.get","params":{}}`, param: "userId"},
		{name: "admin full sem userId", line: `{"id":1,"method":"admin.users.delete.full"}`, param: "userId"},
		{name: "lid sem jid", line: `{"id":1,"method":"user.lid","params":{"jid":""}}`, param: "jid"},
		{name: "history sem chat_jid", line: `{"id":1,"method":"chat.history","params":{"chat_jid":42}}`, param: "chat_jid"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := &recordingRouter{}
			ss, out := newTestServer(t, router)

			ss.handleRequest([]byte(tc.line))

			code, msg := rpcError(t, decodeOne(t, out))
			if code != 400 || !strings.Contains(msg, tc.param) {
				t.Fatalf("erro = %d %q, quero 400 citando %q", code, msg, tc.param)
			}
			if router.calls != 0 {
				t.Fatal("router nao deveria ter sido chamado")
			}
		})
	}
}

func TestRouteRequest_MetodoDesconhecido(t *testing.T) {
	router := &recordingRouter{}
	ss, out := newTestServer(t, router)

	ss.handleRequest([]byte(`{"id":"z","method":"nao.existe"}`))

	code, msg := rpcError(t, decodeOne(t, out))
	if code != 404 || !strings.Contains(msg, "nao.existe") {
		t.Fatalf("erro = %d %q, quero 404 citando o metodo", code, msg)
	}
	if router.calls != 0 {
		t.Fatal("router nao deveria ter sido chamado")
	}
}

// TestRoutesTable_TodasEstaticasDespacham prova que cada entrada da tabela
// estatica chega no handler HTTP com o verbo e o caminho declarados — o unico
// jeito de a tabela nao apodrecer quando um grupo novo entra.
func TestRoutesTable_TodasEstaticasDespacham(t *testing.T) {
	for method, route := range staticRoutes {
		t.Run(method, func(t *testing.T) {
			router := &recordingRouter{respBody: `{"data":"ok"}`}
			ss, _ := newTestServer(t, router)

			ss.handleRequest([]byte(fmt.Sprintf(`{"id":1,"method":%q}`, method)))

			if router.calls != 1 {
				t.Fatalf("router chamado %d vezes", router.calls)
			}
			if router.method != route.httpMethod || router.path != route.httpPath {
				t.Fatalf("despacho = %s %s, quero %s %s",
					router.method, router.path, route.httpMethod, route.httpPath)
			}
		})
	}
}

func TestRoutesTable_GruposNaoSeSobrepoem(t *testing.T) {
	for method := range staticRoutes {
		if _, ok := dynamicRoutes[method]; ok {
			t.Fatalf("metodo %q declarado como estatico e dinamico", method)
		}
	}
	if len(staticRoutes) == 0 || len(dynamicRoutes) == 0 {
		t.Fatal("tabelas de rota vazias")
	}
}

func TestMergeRoutes_AvisaDuplicata(t *testing.T) {
	static := mergeStaticRoutes([]map[string]staticRoute{
		{"a": {httpMethod: "GET", httpPath: "/1"}},
		{"a": {httpMethod: "POST", httpPath: "/2"}, "b": {httpMethod: "GET", httpPath: "/3"}},
	})
	if len(static) != 2 || static["a"].httpPath != "/2" {
		t.Fatalf("merge estatico = %v", static)
	}

	build := func(*Server, *JSONRpcRequest) (string, bool) { return "/x", true }
	dynamic := mergeDynamicRoutes([]map[string]dynamicRoute{
		{"a": {httpMethod: "GET", buildPath: build}},
		{"a": {httpMethod: "PUT", buildPath: build}, "b": {httpMethod: "GET", buildPath: build}},
	})
	if len(dynamic) != 2 || dynamic["a"].httpMethod != "PUT" {
		t.Fatalf("merge dinamico = %v", dynamic)
	}
}

// --- executeHTTPHandler -----------------------------------------------------

func TestExecuteHTTPHandler_ParamsNaoSerializaveis(t *testing.T) {
	ss, out := newTestServer(t, &recordingRouter{})
	req := &JSONRpcRequest{
		ID:     ID{Num: 1, IsSet: true},
		Method: "chat.send.text",
		Params: map[string]interface{}{"ch": make(chan int)},
	}

	ss.executeHTTPHandler(req, http.MethodPost, "/chat/send/text")

	code, msg := rpcError(t, decodeOne(t, out))
	if code != 400 || !strings.Contains(msg, "invalid params") {
		t.Fatalf("erro = %d %q", code, msg)
	}
}

func TestExecuteHTTPHandler_RouterNaoEHTTPHandler(t *testing.T) {
	ss, out := newTestServer(t, "isto nao e um handler")

	ss.handleRequest([]byte(`{"id":1,"method":"health"}`))

	resp := decodeOne(t, out)
	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("esperava sucesso vazio, veio %v", resp)
	}
}

// --- convertHTTPResponse ----------------------------------------------------

func TestConvertHTTPResponse_Formas(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantErr  bool
		wantCode int
		wantMsg  string
		wantJSON string
	}{
		{name: "envelope com data", status: 200, body: `{"data":{"id":"m1"}}`, wantJSON: `{"id":"m1"}`},
		{name: "envelope com error string", status: 404, body: `{"error":"nao achei"}`,
			wantErr: true, wantCode: 404, wantMsg: "nao achei"},
		{name: "envelope com error nao string vira sucesso", status: 500, body: `{"error":42}`,
			wantJSON: `{"error":42}`},
		{name: "mapa sem data nem error", status: 200, body: `{"qualquer":"coisa"}`,
			wantJSON: `{"qualquer":"coisa"}`},
		{name: "corpo nao JSON com sucesso", status: 200, body: "texto puro", wantJSON: `"texto puro"`},
		{name: "corpo nao JSON com falha", status: 503, body: "indisponivel",
			wantErr: true, wantCode: 503, wantMsg: "indisponivel"},
		{name: "corpo vazio com falha", status: 500, body: "",
			wantErr: true, wantCode: 500, wantMsg: "request failed"},
		{name: "lista JSON com sucesso", status: 200, body: `[1,2]`, wantJSON: `[1,2]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ss, out := newTestServer(t, &recordingRouter{})
			rec := httptest.NewRecorder()
			rec.Code = tc.status
			rec.Body = bytes.NewBufferString(tc.body)

			ss.convertHTTPResponse(ID{Num: 5, IsSet: true}, rec)

			resp := decodeOne(t, out)
			if tc.wantErr {
				code, msg := rpcError(t, resp)
				if code != tc.wantCode || !strings.Contains(msg, tc.wantMsg) {
					t.Fatalf("erro = %d %q, quero %d contendo %q", code, msg, tc.wantCode, tc.wantMsg)
				}
				return
			}
			assertResultJSON(t, resp, tc.wantJSON)
		})
	}
}

func assertResultJSON(t *testing.T, resp map[string]interface{}, want string) {
	t.Helper()
	if _, hasErr := resp["error"]; hasErr {
		t.Fatalf("esperava sucesso, veio %v", resp)
	}
	got, err := json.Marshal(resp["result"])
	if err != nil {
		t.Fatalf("remarshal do result: %v", err)
	}
	if string(got) != want {
		t.Fatalf("result = %s, quero %s", got, want)
	}
}

// --- writeResponse ----------------------------------------------------------

type failWriter struct{ err error }

func (f failWriter) Write([]byte) (int, error) { return 0, f.err }

func TestWriteResponse_FallbackQuandoResultNaoSerializa(t *testing.T) {
	out := &bytes.Buffer{}
	ss := NewServerWithIO(&recordingRouter{}, strings.NewReader(""), out)

	ss.sendSuccess(ID{Str: "abc", IsString: true, IsSet: true}, 200, make(chan int))

	code, msg := rpcError(t, decodeOne(t, out))
	if code != -32603 || !strings.Contains(msg, "failed to marshal response") {
		t.Fatalf("fallback = %d %q", code, msg)
	}
	if id, _ := decodeOne(t, out)["id"].(string); id != "abc" {
		t.Fatalf("fallback perdeu o id: %q", id)
	}
}

func TestWriteResponse_ErroDeEscritaNaoDerruba(t *testing.T) {
	ss := NewServerWithIO(&recordingRouter{}, strings.NewReader(""), failWriter{err: errors.New("pipe fechado")})

	ss.sendError(ID{Num: 1, IsSet: true}, 400, "qualquer") // nao pode entrar em panico
}

// --- SendNotification -------------------------------------------------------

// captureStdout troca os.Stdout por um pipe durante fn. Sequencial de proposito:
// SendNotification escreve em os.Stdout por contrato.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()

	_ = w.Close()
	os.Stdout = orig
	got := <-done
	_ = r.Close()
	return got
}

func TestSendNotification_EscreveLinhaJSONRPC(t *testing.T) {
	got := captureStdout(t, func() {
		SendNotification("webhook.event", map[string]interface{}{"tipo": "Message"})
	})

	var notif JSONRpcNotification
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &notif); err != nil {
		t.Fatalf("notificacao nao e' JSON: %v (%q)", err, got)
	}
	if notif.JSONRPC != "2.0" || notif.Method != "webhook.event" {
		t.Fatalf("notificacao = %+v", notif)
	}
	if notif.Params["tipo"] != "Message" {
		t.Fatalf("params = %v", notif.Params)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatal("notificacao precisa terminar em newline")
	}
}

func TestSendNotification_ParamsNaoSerializaveisNaoEscreve(t *testing.T) {
	got := captureStdout(t, func() {
		SendNotification("webhook.event", map[string]interface{}{"ch": make(chan int)})
	})

	if got != "" {
		t.Fatalf("nada deveria ter sido escrito, veio %q", got)
	}
}

func TestSendNotification_StdoutFechadoNaoDerruba(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	orig := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = orig }()

	SendNotification("webhook.event", map[string]interface{}{"a": 1}) // nao pode entrar em panico
}

// --- NewServer --------------------------------------------------------------

func TestNewServer_UsaDescritoresPadrao(t *testing.T) {
	ss := NewServer(&recordingRouter{})
	if ss.stdin != os.Stdin || ss.stdout != os.Stdout {
		t.Fatal("NewServer deveria ligar em os.Stdin/os.Stdout")
	}
}
