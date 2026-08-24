package domain

import "testing"

// TestNaoConfirmadoEXIGEMotivo trava a razão de este tipo existir.
//
// A invariante 14 do stack headless proíbe SUCESSO SILENCIOSO, não incerteza. O
// campo Confirmed foi criado para que "a sessão que agiu não consegue ler a
// mudança de volta" (medido na H58 e na H65) deixe de ser silêncio e vire
// desfecho relatado.
//
// Confirmed falso com Reason vazia traria o silêncio de volta, agora disfarçado
// de estrutura — e seria pior que antes, porque teria a APARÊNCIA de rigor.
func TestNaoConfirmadoEXIGEMotivo(t *testing.T) {
	casos := []struct {
		nome  string
		u     ParticipantsUpdate
		valid bool
	}{
		{"confirmado sem motivo", ParticipantsUpdate{Confirmed: true}, true},
		{"nao confirmado com motivo", ParticipantsUpdate{Reason: "esta sessão não observa a mudança"}, true},
		{"nao confirmado SEM motivo", ParticipantsUpdate{}, false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			ok := c.u.Confirmed || c.u.Reason != ""
			if ok != c.valid {
				t.Fatalf("ParticipantsUpdate{Confirmed:%v Reason:%q} válido=%v, quero %v",
					c.u.Confirmed, c.u.Reason, ok, c.valid)
			}
		})
	}
}

// TestValidaEOQueOTipoPROMETE: a checagem vive num método para que quem escrever
// um adaptador novo a encontre, em vez de a redescobrir lendo um comentário.
func TestValidaEOQueOTipoPROMETE(t *testing.T) {
	if err := (ParticipantsUpdate{}).Valida(); err == nil {
		t.Fatal("não confirmado e sem motivo passou na validação: o sucesso " +
			"silencioso voltou, agora com aparência de rigor")
	}
	if err := (ParticipantsUpdate{Confirmed: true}).Valida(); err != nil {
		t.Fatalf("confirmado foi recusado: %v", err)
	}
	if err := (ParticipantsUpdate{Reason: "não observável desta sessão"}).Valida(); err != nil {
		t.Fatalf("não confirmado COM motivo foi recusado: %v", err)
	}
}
