package message

import (
	"reflect"
	"strings"
	"testing"

	"wa-api/pkg/presentation/http/contracttest"
)

// A regra de nome canónico (docs/HTTP-DTO-CONVENTIONS.md §8) vale para o corpo
// de PEDIDO tanto quanto para o de resposta, e o pedido não tem por onde ser
// afirmado pelo corpo servido — ninguém o serializa.
//
// Por isso a afirmação é sobre as ETIQUETAS dos tipos de pedido desta família,
// varridas por reflexão, recursivamente pelos tipos aninhados.
//
// Derivar por reflexão aqui NÃO é o defeito que o repositório proíbe. O que se
// proíbe é derivar a lista ESPERADA da mesma struct que se está a medir, caso
// em que renomear a etiqueta muda os dois lados. Aqui o lado esperado é uma
// EXPRESSÃO REGULAR independente: renomear `phone` para `Phone` faz este teste
// falhar, que é exactamente o que se quer.
//
// E há um motivo concreto para esta trava existir: o `encoding/json` do Go casa
// chaves SEM distinguir maiúsculas na descodificação. Um pedido com `"Phone"`
// continua a preencher um campo etiquetado `phone`, então NENHUM teste de rota
// desta família falharia se a etiqueta voltasse ao PascalCase. Só esta afirma
// o nome.

// requestTypes enumera, um por linha, os tipos de PEDIDO da família mensagens.
// Escritos à mão de propósito: uma varredura do pacote inteiro passaria a
// abençoar em silêncio um tipo novo que ninguém reviu.
func requestTypes() []any {
	return []any{
		ChatTarget{},
		ReplyContextRequest{},
		InteractiveButtonRequest{},
		TemplateButtonRequest{},
		ListRowRequest{},
		ListSectionRequest{},
		SendTextRequest{},
		SendImageRequest{},
		SendDocumentRequest{},
		SendAudioRequest{},
		SendStickerRequest{},
		SendVideoRequest{},
		SendContactRequest{},
		SendLocationRequest{},
		SendButtonsRequest{},
		SendCarouselCardRequest{},
		SendCarouselRequest{},
		SendListRequest{},
		SendPollRequest{},
		SendPollVoteRequest{},
		SendTemplateRequest{},
		SendForwardRequest{},
		DeleteMessageRequest{},
		SendEditMessageRequest{},
		ReactRequest{},
		SendPresenceRequest{},
		SubscribePresenceRequest{},
		ChatPresenceRequest{},
		MarkReadRequest{},
	}
}

func TestRequestDTOs_EtiquetasCanonicas(t *testing.T) {
	for _, exemplo := range requestTypes() {
		tipo := reflect.TypeOf(exemplo)
		t.Run(tipo.Name(), func(t *testing.T) {
			walkTags(t, tipo, tipo.Name())
		})
	}
}

func walkTags(t *testing.T, tipo reflect.Type, caminho string) {
	t.Helper()
	for tipo.Kind() == reflect.Pointer || tipo.Kind() == reflect.Slice {
		tipo = tipo.Elem()
	}
	if tipo.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < tipo.NumField(); i++ {
		campo := tipo.Field(i)
		if campo.Anonymous {
			walkTags(t, campo.Type, caminho)
			continue
		}
		etiqueta := campo.Tag.Get("json")
		nome := strings.Split(etiqueta, ",")[0]
		if nome == "" {
			t.Errorf("%s.%s não tem etiqueta json: o codificador emitiria o nome Go do campo, que nunca é canónico",
				caminho, campo.Name)
			continue
		}
		if strings.Contains(etiqueta, "omitempty") {
			t.Errorf("%s.%s tem omitempty; num DTO de pedido ele é irrelevante (o Go não o lê na descodificação) e sugere uma semântica que não existe",
				caminho, campo.Name)
		}
		if !contracttest.IsCanonicalKey(nome) {
			t.Errorf("%s.%s tem etiqueta %q, fora de ^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$",
				caminho, campo.Name, nome)
		}
		walkTags(t, campo.Type, caminho+"."+nome)
	}
}

// TestRequestDTOs_ChavesDuplicadasNaoExistem trava o que a regra de nome
// canónico RESOLVEU sem querer: quatro grafias do mesmo identificador de linha
// (RowId/RowID/rowId/rowID) e duas do corpo da lista (Body/body, Text/text)
// eram chaves DIFERENTES em PascalCase e são a MESMA em snake_case. Um tipo que
// as declarasse as duas produziria um corpo com a chave repetida, e o
// `encoding/json` escolheria a última em silêncio.
func TestRequestDTOs_ChavesDuplicadasNaoExistem(t *testing.T) {
	for _, exemplo := range requestTypes() {
		tipo := reflect.TypeOf(exemplo)
		t.Run(tipo.Name(), func(t *testing.T) {
			vistas := map[string]string{}
			collectTags(t, tipo, vistas)
		})
	}
}

func collectTags(t *testing.T, tipo reflect.Type, vistas map[string]string) {
	t.Helper()
	for i := 0; i < tipo.NumField(); i++ {
		campo := tipo.Field(i)
		if campo.Anonymous {
			collectTags(t, campo.Type, vistas)
			continue
		}
		nome := strings.Split(campo.Tag.Get("json"), ",")[0]
		if anterior, repetida := vistas[nome]; repetida {
			t.Errorf("a chave %q é declarada por DOIS campos, %s e %s: o corpo sairia com a chave repetida e o decodificador ficaria com a última",
				nome, anterior, campo.Name)
		}
		vistas[nome] = campo.Name
	}
}
