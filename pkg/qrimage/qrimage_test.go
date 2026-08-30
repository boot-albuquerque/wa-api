package qrimage

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
	"testing"
)

// codigoDeParUmMedido é uma string de pareamento no formato que o WhatsApp
// Web emite: quatro campos separados por vírgula (ref, chave pública Curve25519
// em base64, chave de identidade, segredo ADV). Comprimento e alfabeto batem
// com o que `internal/headless/capabilities/qr` lê de
// `WAWebConnModel.Conn.ref` + `WAWebCompanionRegClientUtils` — não é uma
// abreviação, porque o que se está a medir aqui é justamente se o valor
// atravessa a codificação inteiro.
const codigoDeParUmMedido = "2@Ld9xK3vQpR7sT1uW5yA8bC2dE4fG6hJ0kL3mN5pQ7rS9tU1vW3xY5zA7bC9dE1f=," +
	"kR4mN6pQ8sT0uW2yA4bC6dE8fG0hJ2kL4mN6pQ8sT0uW2y=," +
	"pQ8sT0uW2yA4bC6dE8fG0hJ2kL4mN6pQ8sT0uW2yA4bC6d=," +
	"T0uW2yA4bC6dE8fG0hJ2kL4mN6pQ8sT0uW2yA4bC6dE8fG="

// TestEncodeProduzImagemEnaoOTexto é o teste do defeito da F373, na fronteira
// onde ele nasce.
//
// O defeito medido: `GET /session/pair/qr` respondia a IMAGEM para noise e
// a STRING CRUA para headless, ambas como `string`, ambas no mesmo campo
// `qr_code`. O painel, tratando as duas como crua, desenhou os 1858
// caracteres do data URI como PAYLOAD de um QR — código perfeito, conteúdo
// errado, WhatsApp recusa.
//
// A asserção não é "não é vazio": é que o resultado passa em IsDataURI e que
// o texto original NÃO está lá como conteúdo legível. Um encoder que
// devolvesse o código cru passaria em qualquer teste de "tem valor".
func TestEncodeProduzImagemEnaoOTexto(t *testing.T) {
	got, err := Encode(codigoDeParUmMedido)
	if err != nil {
		t.Fatalf("Encode() erro = %v", err)
	}
	if !IsDataURI(got) {
		t.Fatalf("Encode() = %.40q…, quero um data URI com prefixo %q — devolver a string "+
			"crua aqui é exatamente o defeito da F373: o consumidor desenha-a como payload "+
			"de QR e o WhatsApp recusa o código", got, DataURIPrefix)
	}
	if strings.Contains(got, codigoDeParUmMedido) {
		t.Errorf("Encode() carrega o código de pareamento em claro dentro do resultado; " +
			"o resultado tem de ser a IMAGEM do código, não o código")
	}
}

// TestEncodeDevolvePNGDoTamanhoDeContrato desembrulha o data URI até ao PNG e
// confere as dimensões.
//
// Não é decoração: o valor de noise que o painel serviu durante um dia era
// um data URI PERFEITAMENTE válido, e o problema estava a um nível acima. Um
// teste que só olhasse para o prefixo aceitaria `data:image/png;base64,` +
// lixo. Este exige que os bytes sejam mesmo um PNG de ImageSize×ImageSize —
// as mesmas 256×256 que a medição em campo observou (1375 bytes de PNG,
// 1858 caracteres de data URI).
func TestEncodeDevolvePNGDoTamanhoDeContrato(t *testing.T) {
	got, err := Encode(codigoDeParUmMedido)
	if err != nil {
		t.Fatalf("Encode() erro = %v", err)
	}
	bruto, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(got, DataURIPrefix))
	if err != nil {
		t.Fatalf("o corpo do data URI não é base64 válido: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(bruto))
	if err != nil {
		t.Fatalf("o corpo do data URI não é um PNG: %v", err)
	}
	if quero := (image.Rect(0, 0, ImageSize, ImageSize)); img.Bounds() != quero {
		t.Errorf("dimensões = %v, quero %v", img.Bounds(), quero)
	}
}

// TestEncodeVazioNaoDesenhaNada trava a assimetria deliberada: "" não é erro e
// não vira imagem.
//
// Sem esta regra, um poller que apanha a janela ENTRE dois códigos — estado
// normal e documentado desta rota ("vazio é informação, não defeito",
// api/openapi/schemas/sessao.yaml) — receberia um QR válido da string vazia.
// É a mesma classe de defeito da F373: uma imagem que decodifica para algo que
// ninguém pediu.
func TestEncodeVazioNaoDesenhaNada(t *testing.T) {
	got, err := Encode("")
	if err != nil {
		t.Fatalf("Encode(\"\") erro = %v; um código ausente é estado normal, não falha", err)
	}
	if got != "" {
		t.Errorf("Encode(\"\") = %.40q…, quero \"\" — desenhar a string vazia dá ao cliente "+
			"um QR legível que não é código de pareamento nenhum", got)
	}
}

// TestIsDataURIRecusaCodigoCru fecha a guarda pelo lado negativo: a função que
// distingue imagem de texto tem de recusar justamente o valor que headless
// devolvia antes da correção.
func TestIsDataURIRecusaCodigoCru(t *testing.T) {
	if IsDataURI(codigoDeParUmMedido) {
		t.Error("IsDataURI aceitou a string crua de pareamento; ela é o que a guarda existe para rejeitar")
	}
	if IsDataURI("") {
		t.Error("IsDataURI aceitou a string vazia")
	}
	if !IsDataURI(DataURIPrefix) {
		t.Error("IsDataURI recusou o próprio prefixo")
	}
}
