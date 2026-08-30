package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/notification"
	customhttp "wa-api/pkg/presentation/http"
	"wa-api/pkg/presentation/http/contracttest"

	"wa-api/pkg/domain"

	"github.com/gorilla/mux"
)

// O teste de contrato da família de CANAIS, no padrão de
// handler_group_info_contract_test.go. O que ele afirma, e que um teste do
// handler cru NÃO afirma:
//
//  1. o pedido passa pela ROTA REGISTADA, no mesmo gorilla/mux que a produção
//     usa (ARMADILHAS #2);
//  2. TODA chave do corpo, recursivamente, é snake_case minúsculo — afirmado
//     pelo helper partilhado;
//  3. as chaves ANTIGAS desapareceram. "a chave nova existe" não prova
//     migração: um struct pode carregar as duas;
//  4. os VALORES foram mapeados, e não só as chaves;
//  5. tempo desconhecido é `null` e colecção vazia é `[]`.

// canalDeReferencia é o domain.NewsletterMetadata que o adaptador noise
// produz para um canal real e completo: dono, nome e descrição datados, imagem
// e pré-visualização, relação do chamador. Todos os campos preenchidos de
// propósito — um dublê com metade dos campos no zero deixaria metade do
// mapeamento por medir.
func canalDeReferencia() *domain.NewsletterMetadata {
	criado := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	return &domain.NewsletterMetadata{
		JID:        "120363411025775186@newsletter",
		State:      "active",
		CreatedAt:  criado,
		InviteCode: "0029Vb85MPpKLaHoEUxRt01E",
		Name: domain.NewsletterText{
			Text: "Avisos da Fila Rápida", ID: "1787745654587070",
			UpdatedAt: criado.Add(time.Hour),
		},
		Description: domain.NewsletterText{
			Text: "Novidades e horários.", ID: "1787745654587071",
			UpdatedAt: criado.Add(2 * time.Hour),
		},
		SubscriberCount:   42,
		VerificationState: "unverified",
		ReactionsMode:     "ALL",
		Picture: &domain.NewsletterPicture{
			URL: "https://cdn/x.jpg", ID: "1787750687544878",
			Type: "IMAGE", DirectPath: "/v/t61/x",
		},
		Preview: &domain.NewsletterPicture{
			ID: "1787750687544880", Type: "PREVIEW",
		},
		Viewer: &domain.NewsletterViewer{MuteState: "on", Role: "owner"},
	}
}

// canalRouter monta as rotas de canal EXACTAMENTE como wiring_routes.go as
// monta: registry.Register(caminho, handler, método) aplicado a um mux.Router.
func canalRouter(t *testing.T, nr *contractsfake.NewsletterReader) *mux.Router {
	t.Helper()
	h := NewNewsletterHandlers(notification.NewNewsletterOpsUseCase(nr, &contractsfake.Logger{}))
	lista := NewListNewsletterHandler(notification.NewListNewsletterUseCase(nr, &contractsfake.Logger{}))

	registry := customhttp.NewHandlerRegistry()
	registry.Register("/newsletter/list", withContractUser(lista), http.MethodGet)
	registry.Register("/newsletter/info", withContractUser(h.Info), http.MethodPost)
	registry.Register("/newsletter/create", withContractUser(h.Create), http.MethodPost)
	registry.Register("/newsletter/messages", withContractUser(h.Messages), http.MethodPost)
	registry.Register("/newsletter/follow", withContractUser(h.Follow), http.MethodPost)
	registry.Register("/newsletter/subscribe", withContractUser(h.Subscribe), http.MethodPost)
	registry.Register("/newsletter/mark-viewed", withContractUser(h.MarkViewed), http.MethodPost)
	registry.Register("/newsletter/react", withContractUser(h.React), http.MethodPost)
	registry.Register("/newsletter/delete", withContractUser(h.Delete), http.MethodDelete)

	router := mux.NewRouter()
	registry.Apply(router)
	return router
}

// serveCanal faz o pedido pela rota registada e devolve o gravador.
func serveCanal(t *testing.T, nr *contractsfake.NewsletterReader, metodo, caminho, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(metodo, caminho, strings.NewReader(corpo))
	canalRouter(t, nr).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s: status = %d, quero 200; corpo: %s", metodo, caminho, rec.Code, rec.Body.String())
	}
	return rec
}

// dataDe extrai `data` do envelope canónico, provando de passagem que o
// envelope continua a ser `{success, code, data}`.
func dataDe(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Success bool           `json:"success"`
		Code    int            `json:"code"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("corpo não é o envelope canónico: %v\n%s", err, rec.Body.String())
	}
	if !envelope.Success || envelope.Code != 200 {
		t.Fatalf("envelope = %+v, quero success=true code=200", envelope)
	}
	return envelope.Data
}

// ---------------------------------------------------------------------------
// (1)+(2) nomes canónicos, em todas as formas que esta família serve
// ---------------------------------------------------------------------------

// TestCanal_ContratoPublico_NomesCanonicos percorre as QUATRO formas de
// resposta da família de uma vez: o canal, a listagem, as publicações e o
// reconhecimento. Uma só delas não bastaria — as chaves em falta viviam em
// sítios diferentes de cada uma.
func TestCanal_ContratoPublico_NomesCanonicos(t *testing.T) {
	casos := []struct {
		nome    string
		metodo  string
		caminho string
		corpo   string
	}{
		{"info", http.MethodPost, "/newsletter/info", `{"jid":"120363411025775186@newsletter"}`},
		{"create", http.MethodPost, "/newsletter/create", `{"name":"Canal X"}`},
		{"list", http.MethodGet, "/newsletter/list", ""},
		{"messages", http.MethodPost, "/newsletter/messages", `{"jid":"120363411025775186@newsletter","count":5}`},
		{"follow", http.MethodPost, "/newsletter/follow", `{"jid":"120363411025775186@newsletter"}`},
		{"subscribe", http.MethodPost, "/newsletter/subscribe", `{"jid":"120363411025775186@newsletter"}`},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			rec := serveCanal(t, canalFakeCompleto(), c.metodo, c.caminho, c.corpo)
			contracttest.AssertPublicJSONUsesCanonicalNaming(t, rec.Body.Bytes())
		})
	}
}

// canalFakeCompleto devolve um dublê que responde a TODAS as leituras com o
// canal de referência e com uma publicação real.
//
// O dublê imita a produção nos pontos em que a produção tem regra: a listagem
// e a página de publicações são fatias não-nil (os dois adaptadores constroem
// com `make`), e a subscrição devolve um arrendamento em segundos, que é o que
// `NewsletterSubscribeLiveUpdates` devolve.
func canalFakeCompleto() *contractsfake.NewsletterReader {
	canal := canalDeReferencia()
	return &contractsfake.NewsletterReader{
		ListSubscribedFunc: func(context.Context, string) ([]domain.NewsletterMetadata, error) {
			return []domain.NewsletterMetadata{*canal}, nil
		},
		NewsletterInfoFunc: func(context.Context, string, domain.JID) (*domain.NewsletterMetadata, error) {
			return canal, nil
		},
		CreateNewsletterFunc: func(context.Context, string, string, string, []byte) (*domain.NewsletterMetadata, error) {
			return canal, nil
		},
		MessagesFunc: func(context.Context, string, domain.JID, int, string) ([]domain.NewsletterMessage, error) {
			return []domain.NewsletterMessage{publicacaoDeReferencia()}, nil
		},
		SubscribeLiveFunc: func(context.Context, string, domain.JID) (time.Duration, error) {
			return 90 * time.Second, nil
		},
	}
}

// publicacaoDeReferencia é a publicação medida em campo a 2026-08-26: id
// hexadecimal, tipo `media` num texto puro (o protocolo mente, e o teste
// preserva a mentira), contagens de reação com o MESMO emoji em duas formas
// Unicode.
func publicacaoDeReferencia() domain.NewsletterMessage {
	return domain.NewsletterMessage{
		ServerID:  4379,
		ID:        "3EB0A4B2AFD45E625C0917",
		Type:      "media",
		Timestamp: time.Date(2026, 8, 25, 23, 22, 23, 0, time.UTC),
		ViewCount: 7,
		// As duas formas do coração são chaves SEPARADAS e é assim que têm de
		// chegar: normalizá-las somaria uma reação que o protocolo conta duas
		// vezes.
		ReactionCounts: map[string]int{"❤": 1, "❤️": 6},
		Text:           "Bom dia",
	}
}

// ---------------------------------------------------------------------------
// (3) as chaves antigas desapareceram
// ---------------------------------------------------------------------------

// TestCanal_ContratoPublico_ChavesAntigasSumiram é a afirmação (3).
//
// As chaves listadas são as que a struct do vendor emitia — `thread_metadata`,
// `viewer_metadata`, `creation_time`, `subscribers_count`, `update_time`,
// `invite`, `verification` — e as que `channel.DirectoryEntry` emitia por não
// ter etiqueta nenhuma: `JID`, `Subscribers`, `Membership`, `Verified`. Mais as
// sete de `types.NewsletterMessage`, pela mesma razão.
func TestCanal_ContratoPublico_ChavesAntigasSumiram(t *testing.T) {
	antigas := []string{
		// vendor: types.NewsletterMetadata
		"thread_metadata", "viewer_metadata", "creation_time", "subscribers_count",
		"update_time", "invite", "verification", "reaction_codes", "settings",
		// vendor: channel.DirectoryEntry, serializado sem etiquetas
		"JID", "Name", "Description", "Subscribers", "Verified", "Membership", "CreatedAt",
		// vendor: types.NewsletterMessage, serializado sem etiquetas
		"MessageServerID", "MessageID", "Type", "Timestamp", "ViewsCount",
		"ReactionCounts", "Message",
	}
	// `newsletter` NÃO entra na lista, e a ausência é deliberada: a chave
	// sobrevive à migração com outro significado. Era a LISTA de
	// GET /newsletter/list (`data.newsletter[]`) e é agora o canal ÚNICO de
	// /newsletter/info (`data.newsletter{}`). Bani-la globalmente proibiria a
	// forma nova; o desaparecimento da antiga é afirmado abaixo, na rota onde
	// ela vivia.
	casos := []struct {
		nome    string
		metodo  string
		caminho string
		corpo   string
	}{
		{"info", http.MethodPost, "/newsletter/info", `{"jid":"120363411025775186@newsletter"}`},
		{"list", http.MethodGet, "/newsletter/list", ""},
		{"messages", http.MethodPost, "/newsletter/messages", `{"jid":"120363411025775186@newsletter"}`},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			rec := serveCanal(t, canalFakeCompleto(), c.metodo, c.caminho, c.corpo)
			contracttest.AssertNoKeys(t, rec.Body.Bytes(), antigas...)
		})
	}
}

// ---------------------------------------------------------------------------
// (4) os valores foram mapeados
// ---------------------------------------------------------------------------

// TestCanal_ContratoPublico_ValoresMapeados prova que o apresentador não só
// produziu as chaves certas como pôs os VALORES certos nelas — e que as quatro
// conversões de tipo aconteceram: o instante deixou de ser um inteiro Unix em
// aspas, e a contagem de seguidores deixou de ser texto.
func TestCanal_ContratoPublico_ValoresMapeados(t *testing.T) {
	data := dataDe(t, serveCanal(t, canalFakeCompleto(), http.MethodPost,
		"/newsletter/info", `{"jid":"120363411025775186@newsletter"}`))

	canal, ok := data["newsletter"].(map[string]any)
	if !ok {
		t.Fatalf("data.newsletter = %#v, quero objecto", data["newsletter"])
	}

	quero := map[string]any{
		"jid":                "120363411025775186@newsletter",
		"state":              "active",
		"invite_code":        "0029Vb85MPpKLaHoEUxRt01E",
		"verification_state": "unverified",
		"reactions_mode":     "ALL",
		// Inteiro, e não a string "42" que o protocolo manda.
		"subscriber_count": float64(42),
		// RFC 3339 em UTC, e não "1772366400".
		"created_at": "2026-03-01T12:00:00Z",
	}
	for chave, esperado := range quero {
		if got := canal[chave]; got != esperado {
			t.Errorf("newsletter.%s = %#v, quero %#v", chave, got, esperado)
		}
	}

	nome, _ := canal["name"].(map[string]any)
	if nome["text"] != "Avisos da Fila Rápida" || nome["id"] != "1787745654587070" {
		t.Errorf("newsletter.name = %#v", nome)
	}
	if nome["updated_at"] != "2026-03-01T13:00:00Z" {
		t.Errorf("newsletter.name.updated_at = %#v", nome["updated_at"])
	}
	desc, _ := canal["description"].(map[string]any)
	if desc["text"] != "Novidades e horários." {
		t.Errorf("newsletter.description = %#v", desc)
	}

	pic, _ := canal["picture"].(map[string]any)
	if pic["url"] != "https://cdn/x.jpg" || pic["type"] != "IMAGE" || pic["direct_path"] != "/v/t61/x" {
		t.Errorf("newsletter.picture = %#v", pic)
	}
	prev, _ := canal["preview"].(map[string]any)
	if prev["type"] != "PREVIEW" || prev["url"] != "" {
		t.Errorf("newsletter.preview = %#v", prev)
	}

	// O viewer é o par que mais custa se trocar: `mute_state` e `role` são
	// ambos texto, vizinhos, e uma troca passaria em qualquer teste de chaves.
	viewer, _ := canal["viewer"].(map[string]any)
	if viewer["mute_state"] != "on" || viewer["role"] != "owner" {
		t.Errorf("newsletter.viewer = %#v, quero mute_state=on role=owner", viewer)
	}
}

// TestCanal_ContratoPublico_PublicacaoMapeada faz o mesmo para a publicação,
// incluindo o que a migração DELIBERADAMENTE deixou cair: a árvore protobuf.
func TestCanal_ContratoPublico_PublicacaoMapeada(t *testing.T) {
	data := dataDe(t, serveCanal(t, canalFakeCompleto(), http.MethodPost,
		"/newsletter/messages", `{"jid":"120363411025775186@newsletter","count":5}`))

	lista, ok := data["messages"].([]any)
	if !ok || len(lista) != 1 {
		t.Fatalf("data.messages = %#v, quero 1 elemento", data["messages"])
	}
	pub, _ := lista[0].(map[string]any)

	quero := map[string]any{
		"server_id":  float64(4379),
		"message_id": "3EB0A4B2AFD45E625C0917",
		"type":       "media",
		"view_count": float64(7),
		"timestamp":  "2026-08-25T23:22:23Z",
		"text":       "Bom dia",
	}
	for chave, esperado := range quero {
		if got := pub[chave]; got != esperado {
			t.Errorf("messages[0].%s = %#v, quero %#v", chave, got, esperado)
		}
	}

	// As duas formas Unicode do coração chegam SEPARADAS. Somá-las aqui daria
	// 7 e o protocolo nunca disse 7.
	//
	// E chegam como ARRAY, não como objecto de chaves emoji: nenhum emoji passa
	// `^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`, e é a única forma desta família em que
	// a regra de nomes obrigou a mudar a FORMA e não só o nome.
	reacoes, ok := pub["reactions"].([]any)
	if !ok || len(reacoes) != 2 {
		t.Fatalf("messages[0].reactions = %#v, quero 2 elementos", pub["reactions"])
	}
	// Ordenado por contagem decrescente, e o determinismo é o ponto: a ordem de
	// mapa em Go é aleatória por desenho, então uma fatia não ordenada faria a
	// mesma publicação serializar diferente a cada pedido.
	primeira, _ := reacoes[0].(map[string]any)
	segunda, _ := reacoes[1].(map[string]any)
	if primeira["emoji"] != "❤️" || primeira["count"] != float64(6) {
		t.Errorf("reactions[0] = %#v, quero ❤️ com 6", primeira)
	}
	if segunda["emoji"] != "❤" || segunda["count"] != float64(1) {
		t.Errorf("reactions[1] = %#v, quero ❤ com 1", segunda)
	}
}

// TestCanal_ContratoPublico_ReacoesSaoDeterministas roda a mesma asserção
// várias vezes porque o defeito que ela trava só aparece por sorte: a ordem de
// iteração de mapa em Go é aleatória, e uma única passagem passa com uma fatia
// não ordenada em metade das execuções.
func TestCanal_ContratoPublico_ReacoesSaoDeterministas(t *testing.T) {
	var primeira string
	for i := 0; i < 20; i++ {
		rec := serveCanal(t, canalFakeCompleto(), http.MethodPost,
			"/newsletter/messages", `{"jid":"120363411025775186@newsletter"}`)
		corpo := rec.Body.String()
		if i == 0 {
			primeira = corpo
			continue
		}
		if corpo != primeira {
			t.Fatalf("a rota serializou dois corpos diferentes para a mesma publicação:\n%s\n%s", primeira, corpo)
		}
	}
}

// TestCanal_ContratoPublico_SubscribeEntregaADuracao trava a F231 do lado que
// importa: a duração chega ao CLIENTE, em segundos.
//
// Antes desta migração ela não chegava de todo — o handler servia `rsp.Data`,
// que a subscrição deixa nil, e a rota respondia `data: null`. O cliente não
// tinha como saber quando renovar, que é a única coisa que esta rota existe
// para dizer.
func TestCanal_ContratoPublico_SubscribeEntregaADuracao(t *testing.T) {
	data := dataDe(t, serveCanal(t, canalFakeCompleto(), http.MethodPost,
		"/newsletter/subscribe", `{"jid":"120363411025775186@newsletter"}`))

	if data["duration_seconds"] != float64(90) {
		t.Errorf("duration_seconds = %#v, quero 90 (nanossegundos?)", data["duration_seconds"])
	}
	if data["status"] != domain.StatusSent {
		t.Errorf("status = %#v, quero %q", data["status"], domain.StatusSent)
	}
}

// TestCanal_ContratoPublico_OperacaoSemCargaDizStatus: as rotas que só mudam
// alguma coisa respondiam `data: null`, indistinguível de uma rota que falhou
// a encher a carga.
func TestCanal_ContratoPublico_OperacaoSemCargaDizStatus(t *testing.T) {
	data := dataDe(t, serveCanal(t, canalFakeCompleto(), http.MethodPost,
		"/newsletter/follow", `{"jid":"120363411025775186@newsletter"}`))
	if data["status"] != domain.StatusSent {
		t.Errorf("status = %#v, quero %q", data["status"], domain.StatusSent)
	}
	// E não traz as chaves das outras formas: `newsletter: null` numa
	// subscrição diria ao cliente que existe um canal a ler.
	for _, chave := range []string{"newsletter", "messages", "duration_seconds"} {
		if _, presente := data[chave]; presente {
			t.Errorf("data.%s presente numa operação sem carga: %#v", chave, data)
		}
	}
}

// ---------------------------------------------------------------------------
// (5) zero e vazio
// ---------------------------------------------------------------------------

// TestCanal_ContratoPublico_ZeroNaoViraDataFalsa trava a decisão do
// apresentador sobre tempo ausente: null, e não "0001-01-01T00:00:00Z" nem o
// "0" do protocolo.
//
// Não é hipotético: um canal APAGADO vem com `creation_time: "0"` e com os dois
// `update_time` a "0" — medido a 2026-08-26 —, e o motor headless não reporta
// instante nenhum para os textos.
func TestCanal_ContratoPublico_ZeroNaoViraDataFalsa(t *testing.T) {
	apagado := &domain.NewsletterMetadata{
		JID:   "120363411025775186@newsletter",
		State: "deleted",
	}
	nr := &contractsfake.NewsletterReader{
		NewsletterInfoFunc: func(context.Context, string, domain.JID) (*domain.NewsletterMetadata, error) {
			return apagado, nil
		},
	}
	data := dataDe(t, serveCanal(t, nr, http.MethodPost,
		"/newsletter/info", `{"jid":"120363411025775186@newsletter"}`))
	canal, _ := data["newsletter"].(map[string]any)

	valor, presente := canal["created_at"]
	if !presente {
		t.Error("created_at ausente: a chave tem de existir mesmo sem valor, senão o cliente não distingue ausente de desconhecido")
	}
	if valor != nil {
		t.Errorf("created_at = %#v, quero null para tempo zero", valor)
	}
	for _, campo := range []string{"name", "description"} {
		texto, _ := canal[campo].(map[string]any)
		v, p := texto["updated_at"]
		if !p {
			t.Errorf("%s.updated_at ausente", campo)
		}
		if v != nil {
			t.Errorf("%s.updated_at = %#v, quero null", campo, v)
		}
	}
	// Sem imagem e sem relação, os três vêm `null` — o que é diferente de um
	// objecto de campos vazios, que diria "há imagem, sem url".
	for _, campo := range []string{"picture", "preview", "viewer"} {
		v, p := canal[campo]
		if !p {
			t.Errorf("%s ausente", campo)
		}
		if v != nil {
			t.Errorf("%s = %#v, quero null", campo, v)
		}
	}
}

// TestCanal_ContratoPublico_ColeccaoVaziaEListaVazia: `[]` e `null` são valores
// diferentes para qualquer cliente, e só um dos dois se percorre sem
// verificação. Vale para as três colecções desta família.
func TestCanal_ContratoPublico_ColeccaoVaziaEListaVazia(t *testing.T) {
	nr := &contractsfake.NewsletterReader{} // zero-value: fatias vazias não-nil

	lista := dataDe(t, serveCanal(t, nr, http.MethodGet, "/newsletter/list", ""))
	if c, ok := lista["newsletters"].([]any); !ok || len(c) != 0 {
		t.Errorf("newsletters = %#v, quero []", lista["newsletters"])
	}
	// E a chave ANTIGA da listagem — `newsletter`, no singular — desapareceu
	// desta rota. É a metade que a lista de chaves banidas não pode afirmar,
	// porque a mesma palavra passou a nomear o canal único de /newsletter/info.
	if _, presente := lista["newsletter"]; presente {
		t.Errorf("GET /newsletter/list ainda serve a chave `newsletter`: %#v", lista)
	}

	msgs := dataDe(t, serveCanal(t, nr, http.MethodPost,
		"/newsletter/messages", `{"jid":"120363411025775186@newsletter"}`))
	if c, ok := msgs["messages"].([]any); !ok || len(c) != 0 {
		t.Errorf("messages = %#v, quero []", msgs["messages"])
	}

	// E a publicação sem reação nenhuma responde `{}`, não `null`.
	semReacoes := &contractsfake.NewsletterReader{
		MessagesFunc: func(context.Context, string, domain.JID, int, string) ([]domain.NewsletterMessage, error) {
			return []domain.NewsletterMessage{{ServerID: 1, ID: "m1"}}, nil
		},
	}
	uma := dataDe(t, serveCanal(t, semReacoes, http.MethodPost,
		"/newsletter/messages", `{"jid":"120363411025775186@newsletter"}`))
	pub, _ := uma["messages"].([]any)[0].(map[string]any)
	if c, ok := pub["reactions"].([]any); !ok || len(c) != 0 {
		t.Errorf("reactions = %#v, quero []", pub["reactions"])
	}
	// O instante zero de uma publicação também é null.
	if pub["timestamp"] != nil {
		t.Errorf("timestamp = %#v, quero null", pub["timestamp"])
	}
}

// ---------------------------------------------------------------------------
// Pedido: as cinco chaves em camelCase
// ---------------------------------------------------------------------------

// TestCanal_ContratoPublico_PedidoUsaSnakeCase é a metade do contrato que os
// outros testes não cobrem: a regra de nomes vale para o CORPO DE PEDIDO, e
// esta família tinha cinco chaves fora dela.
//
// O teste é escrito pelo EFEITO e não pela decodificação: manda o corpo em
// snake_case e afirma que o valor CHEGOU à porta. Um teste que só afirmasse
// "o campo decodificou" passaria com as duas grafias aceites, que é exactamente
// o corte a seco que não se quer.
func TestCanal_ContratoPublico_PedidoUsaSnakeCase(t *testing.T) {
	const canal = "120363411025775186@newsletter"

	nr := &contractsfake.NewsletterReader{}
	serveCanal(t, nr, http.MethodPost, "/newsletter/mark-viewed",
		`{"jid":"`+canal+`","server_ids":[1,2]}`)
	if len(nr.NewsletterCalls) != 1 || nr.NewsletterCalls[0].Extra != "[1 2]" {
		t.Fatalf("server_ids não chegou à porta: %+v", nr.NewsletterCalls)
	}

	nr = &contractsfake.NewsletterReader{}
	serveCanal(t, nr, http.MethodPost, "/newsletter/react",
		`{"jid":"`+canal+`","server_id":3,"reaction":"👍","message_id":"m1"}`)
	if len(nr.NewsletterCalls) != 1 || nr.NewsletterCalls[0].Extra != "👍" {
		t.Fatalf("react não chegou à porta: %+v", nr.NewsletterCalls)
	}

	// `confirm_jid` é a que mais importa: a validação exige que ele seja IGUAL
	// ao jid, então uma chave que não decodifica faz o apagamento ser recusado
	// — e uma que decodificasse pela grafia antiga faria o oposto, apagar um
	// canal com uma confirmação que já não é a documentada.
	nr = &contractsfake.NewsletterReader{}
	serveCanal(t, nr, http.MethodDelete, "/newsletter/delete",
		`{"jid":"`+canal+`","confirm_jid":"`+canal+`"}`)
	if len(nr.NewsletterCalls) != 1 || nr.NewsletterCalls[0].Method != "DeleteNewsletter" {
		t.Fatalf("confirm_jid não chegou à porta: %+v", nr.NewsletterCalls)
	}
}

// TestCanal_ContratoPublico_GrafiaAntigaDoPedidoNaoEAceite é o outro lado do
// corte a seco: a chave velha tem de deixar de funcionar. Se as duas fossem
// aceites, a antiga nunca morreria e o contrato teria duas verdades.
func TestCanal_ContratoPublico_GrafiaAntigaDoPedidoNaoEAceite(t *testing.T) {
	const canal = "120363411025775186@newsletter"

	nr := &contractsfake.NewsletterReader{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/newsletter/delete",
		strings.NewReader(`{"jid":"`+canal+`","confirmJID":"`+canal+`"}`))
	canalRouter(t, nr).ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("`confirmJID` ainda é aceite: o corte a seco não aconteceu (corpo: %s)", rec.Body.String())
	}
	if len(nr.NewsletterCalls) != 0 {
		t.Fatalf("o apagamento chegou à porta com a confirmação na grafia antiga: %+v", nr.NewsletterCalls)
	}
}
