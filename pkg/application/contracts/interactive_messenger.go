package port

import (
	"context"

	"wa-api/pkg/domain"
)

// InteractiveMessenger envia uma mensagem INTERATIVA nova — a família
// `waE2E.InteractiveMessage`/`NativeFlowMessage`, os botões nativos que o
// aparelho renderiza abaixo do corpo — com um header de mídia OPCIONAL
// (CAP-21).
//
// Porta separada de SimpleMessenger e de MediaMessenger de propósito, e a
// fronteira é o UPLOAD CONDICIONAL:
//
//   - SimpleMessenger define-se por "montada inteiramente a partir de dados
//     já presentes no payload — sem upload, sem fetch externo, sem
//     conversão". Enfiar SendButtons lá tornaria esse contrato FALSO para as
//     quatro capacidades que já vivem nele (Location, Contact, Poll,
//     Template), que nunca sobem nada. É exatamente a incoerência que o
//     CAP-02 evitou ao não esticar TextMessenger num "SendAnything".
//
//   - MediaMessenger define-se por "envia uma mensagem de MÍDIA nova,
//     cobrindo o upload do anexo": lá o anexo É a mensagem, o upload é
//     obrigatório e o payload é domain.MediaPayload. Aqui a mensagem é o
//     corpo mais os botões, o anexo é decoração de header e na maioria das
//     chamadas não existe — um SendButtons em MediaMessenger seria o único
//     método da porta que pode não fazer upload nenhum.
//
// Não é porta especulativa: é o primeiro caso REAL desta forma, na mesma
// disciplina com que MediaMessenger nasceu com um método só (SendImage,
// CAP-02). E não recolhe o vizinho por antecipação: send/list monta
// `waE2E.ListMessage` sem upload nenhum (`git show 41bc8e2^:handlers.go`,
// função SendList) e portanto cabe em SimpleMessenger, não aqui.
type InteractiveMessenger interface {
	SessionGuard

	// SendButtons monta um InteractiveMessage com NativeFlowMessage a
	// partir de payload (Body/Title/Footer/Buttons/HeaderImage) e o envia
	// para target. id, quando não-vazio, é o identificador de mensagem que
	// o chamador quer usar; o resultado devolvido traz o ID e o Timestamp
	// que a sessão REALMENTE usou, nunca fabricados localmente — mesma
	// disciplina de SendLocation e SendImage.
	//
	// Quando payload.HeaderImage não é vazio, a implementação sobe esses
	// bytes ANTES do envio e os usa como header de mídia; um upload
	// bem-sucedido seguido de envio falho propaga o erro do envio, sem
	// tentativa de desfazer o upload (o protocolo não oferece essa
	// operação), igual a MediaMessenger.SendImage.
	//
	// A tradução de cada domain.InteractiveButton para o seu
	// NativeFlowButton é da IMPLEMENTAÇÃO, e com ela DOIS detalhes que são
	// puro wire: o `Name` de cada tipo (que NÃO é o tipo público — `copy`
	// vira `cta_copy`) e o fato de os parâmetros irem como STRING JSON em
	// ButtonParamsJSON, e não como campos de protobuf. QUE a ordem dos
	// botões seja preservada é contrato desta porta — é a ordem em que
	// eles aparecem no aparelho de quem recebe.
	SendButtons(ctx context.Context, txtID string, target domain.JID, payload domain.ButtonsPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error)

	// SendCarousel mounts an InteractiveMessage whose oneof is
	// CarouselMessage and sends it to target. Each card is itself a
	// full InteractiveMessage with header, body, footer and native-flow
	// buttons — the adapter handles the recursive nesting and the
	// per-card image uploads.
	//
	// payload.Cards must arrive with buttons ALREADY NORMALISED by the
	// use case, same discipline as SendButtons.
	SendCarousel(ctx context.Context, txtID string, target domain.JID, payload domain.CarouselPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error)
}
