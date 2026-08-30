// get_qr_contract_test.go — a forma da resposta de `GET /session/pair/qr`,
// travada contra os DOIS engines que a servem.
//
// O defeito da F373 não estava em nenhum dos dois adapters isoladamente:
// cada um respondia coerentemente consigo mesmo. Estava em NINGUÉM comparar
// os dois. noise devolvia a imagem, headless devolvia a string crua, as
// duas são `string`, as duas saem no mesmo campo `qr_code`, e o consumidor
// que acreditou na uniformidade desenhou uma delas como payload de QR.
//
// Por isso o teste é uma TABELA sobre os engines, e não dois testes soltos: a
// asserção que interessa é que as duas pernas produzem a mesma FORMA, e essa
// asserção não existe se cada perna for verificada no seu próprio ficheiro.
package session_test

import (
	"context"
	"strings"
	"testing"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/session"
	"wa-api/pkg/qrimage"
)

// codigoCruDaPagina é o que o adapter headless devolve pela porta:
// a string de pareamento lida ao vivo de `WAWebConnModel.Conn.ref`
// (pkg/infra/wa-headless/pairing/qr.go → internal/wa-headless/capabilities/qr).
// Formato de quatro campos separados por vírgula, como o WhatsApp Web emite.
const codigoCruDaPagina = "2@Ld9xK3vQpR7sT1uW5yA8bC2dE4fG6hJ0kL3mN5pQ7rS9tU1vW3xY5zA7bC9dE1f=," +
	"kR4mN6pQ8sT0uW2yA4bC6dE8fG0hJ2kL4mN6pQ8sT0uW2y=," +
	"pQ8sT0uW2yA4bC6dE8fG0hJ2kL4mN6pQ8sT0uW2yA4bC6d=," +
	"T0uW2yA4bC6dE8fG0hJ2kL4mN6pQ8sT0uW2yA4bC6dE8fG="

// qrNoisePersistido é o que o adapter noise devolve pela MESMA porta: o
// conteúdo literal da coluna users.qrcode, que o listener de QR do
// orquestrador escreve já codificado em PNG
// (pkg/application/session/orchestrator.go, onPairingQR: "A coluna guarda a
// IMAGEM"). Construído com o mesmo codificador da produção em vez de ser uma
// constante escrita à mão — um literal abreviado seria um dublê mais SIMPLES
// que a produção, e abençoaria o caminho que estamos a medir.
var qrNoisePersistido = mustEncode(codigoCruDaPagina)

func mustEncode(code string) string {
	s, err := qrimage.Encode(code)
	if err != nil {
		panic("dublê de noise: " + err.Error())
	}
	return s
}

// leitorFixo é um PairingQRReader que devolve exatamente o que o engine
// correspondente devolve na produção — nem normalizado, nem abreviado. É
// esse o ponto: se o dublê normalizasse, o teste mediria o dublê.
func leitorFixo(valor string) *contractsfake.PairingQRReader {
	return &contractsfake.PairingQRReader{
		PairingQRFunc: func(context.Context, string) (string, error) { return valor, nil },
	}
}

// TestGetQR_OsDoisEnginesRespondemAImagemDocumentada é o teste do defeito.
//
// Condição medida em campo (2026-08-29, servidor vivo em :8099, sessão
// noise recém-criada): `GET /session/pair/qr` respondeu
// `{"qr_code":"data:image/png;base64,iVBORw0KG…"}`, 1858 caracteres, cujo
// corpo decodifica para um PNG de 1375 bytes. O painel desenhou esses 1858
// caracteres como PAYLOAD de um QR — cabem folgados no limite de 2953 bytes
// do nível L, então nada falhou, nada foi ao console, e o telefone leu um
// código que o WhatsApp recusou.
//
// A asserção é a FORMA, para os dois engines, e mais: para noise, que a
// imagem sai INTACTA. Recodificá-la é precisamente o defeito, e um teste que
// só exigisse "é um data URI" passaria com o data URI do data URI.
func TestGetQR_OsDoisEnginesRespondemAImagemDocumentada(t *testing.T) {
	casos := []struct {
		engine string
		// devolvido pelo adapter daquele engine, pela porta PairingQRReader
		daPorta string
		// o que a rota tem de responder
		querRota string
	}{
		{
			engine:   "noise",
			daPorta:  qrNoisePersistido,
			querRota: qrNoisePersistido, // passa intacto: já é a imagem
		},
		{
			engine:   "headless",
			daPorta:  codigoCruDaPagina,
			querRota: qrNoisePersistido, // renderizado para a MESMA imagem
		},
	}

	for _, tc := range casos {
		t.Run(tc.engine, func(t *testing.T) {
			r, err := session.NewGetQRUseCase(leitorFixo(tc.daPorta), &contractsfake.Logger{}).
				Execute(context.Background(), txtID)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if !qrimage.IsDataURI(r.QRCode) {
				t.Fatalf("%s: qr_code = %.40q…, e a rota documenta uma imagem em data URI "+
					"(api/openapi/schemas/sessao.yaml). Devolver a string crua faz o "+
					"consumidor desenhá-la como payload de QR — F373", tc.engine, r.QRCode)
			}
			if r.QRCode != tc.querRota {
				t.Errorf("%s: qr_code = %.40q… (%d chars), quero %.40q… (%d chars)",
					tc.engine, r.QRCode, len(r.QRCode), tc.querRota, len(tc.querRota))
			}
			// A trava contra a recodificação: o prefixo do data URI não pode
			// aparecer DENTRO do que foi desenhado. Se aparecesse, o QR
			// codificaria um data URI, que é o defeito exato.
			if corpo := strings.TrimPrefix(r.QRCode, qrimage.DataURIPrefix); strings.Contains(corpo, "data:image") {
				t.Errorf("%s: o resultado carrega um data URI dentro do próprio data URI — "+
					"a imagem foi codificada a partir de outra imagem (F373)", tc.engine)
			}
		})
	}
}

// TestGetQR_OsDoisEnginesConcordamNaJanelaSemCodigo trava a outra metade da
// uniformidade: "ainda não há código" tem de ser a MESMA resposta nos dois.
//
// Vale porque é o estado que um poller vê na maior parte das chamadas (o
// código roda, e a rota documenta "vazio é informação, não defeito"). Se um
// engine respondesse "" e o outro a imagem da string vazia, o painel
// mostraria um QR ilegível para metade das sessões em vez de esperar.
func TestGetQR_OsDoisEnginesConcordamNaJanelaSemCodigo(t *testing.T) {
	for _, engine := range []string{"noise", "headless"} {
		t.Run(engine, func(t *testing.T) {
			r, err := session.NewGetQRUseCase(leitorFixo(""), &contractsfake.Logger{}).
				Execute(context.Background(), txtID)
			if err != nil {
				t.Fatalf("%s: código ausente não é erro, mas veio %v", engine, err)
			}
			if r.QRCode != "" {
				t.Errorf("%s: qr_code = %.40q…, quero vazio — desenhar a string vazia dá ao "+
					"cliente um QR legível que não é código de pareamento nenhum", engine, r.QRCode)
			}
		})
	}
}
