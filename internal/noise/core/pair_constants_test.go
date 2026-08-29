package core

import "testing"

// F195. Os comentários de pkg/application/session/orchestrator.go e de
// pkg/presentation/http/devui/assets/sessions.js descreviam a janela do
// PRIMEIRO código de QR. Estiveram errados duas vezes: primeiro diziam 20s
// quando eram 60s (corrigido pela F69), depois diziam 60s quando a constante
// já tinha passado a 20s — e ninguém deu por isso durante meses, porque
// comentário não falha em teste.
//
// Este teste é a trava que faltava. Não defende um VALOR: defende a
// IGUALDADE, que é o que os comentários afirmam. Se alguém voltar a dar ao
// primeiro código uma janela própria, isto falha e obriga a passar pelos dois
// comentários — que é exatamente o que não aconteceu da última vez.
func TestQRPrimeiroCodigoTemAMesmaJanelaQueOsDemais(t *testing.T) {
	if qrCodeFirstTimeout != qrCodeTimeout {
		t.Fatalf("qrCodeFirstTimeout = %v, qrCodeTimeout = %v.\n"+
			"A igualdade é deliberada (um QR de pareamento é uma credencial).\n"+
			"Se a divergência for intencional, ATUALIZE também:\n"+
			"  - pkg/application/session/orchestrator.go (comentário de expiresAt)\n"+
			"  - pkg/presentation/http/devui/assets/sessions.js (iniciarTTL)\n"+
			"Ver HOUSEKEEP F195: os dois já estiveram errados por não acompanharem esta constante.",
			qrCodeFirstTimeout, qrCodeTimeout)
	}
}
