// mute_duration_pointer_test.go — a regra 4.5 do CONTRATO-ARQUITETURAL sobre
// `mute_duration`: ser ponteiro não basta, é preciso ramificar sobre `nil`
// ANTES de desreferenciar.
//
// O comportamento observável NÃO muda com a correção: ausente e `0` explícito
// continuam a significar "para sempre". O que estes testes travam é que os dois
// ESTADOS chegam à porta com o valor que o contrato lhes atribui, e que um
// terceiro estado — a duração explícita — não é confundido com nenhum deles.
// Sem isto, a próxima reescrita volta a colapsar os ramos sem que nada morda.
package chat_test

import (
	"context"
	"testing"
	"time"

	"wa-api/pkg/application/contracts/contractsfake"
	"wa-api/pkg/application/usecase/chat"
	"wa-api/pkg/domain"
)

const muteJID = "5511999999999@s.whatsapp.net"

// muteForeverAtPort is what "forever" looks like on the wire to BuildMute.
const muteForeverAtPort = time.Duration(0)

func TestMuteChat_DurationStates(t *testing.T) {
	explicitZero := time.Duration(0)
	eightHours := 8 * time.Hour

	cases := []struct {
		name string
		// duration is the *time.Duration the payload decoded into.
		duration *time.Duration
		want     time.Duration
		why      string
	}{
		{
			name:     "absent_is_forever",
			duration: nil,
			want:     muteForeverAtPort,
			why: "campo ausente: o chamador não exprimiu preferência, e o " +
				"contrato diz que isso é para sempre. É o ramo que a F268 " +
				"atingia por acidente, com o nome do campo mal escrito",
		},
		{
			name:     "explicit_zero_is_forever",
			duration: &explicitZero,
			want:     muteForeverAtPort,
			why: "`mute_duration:0` explícito: o chamador ESCOLHEU para sempre. " +
				"Mesmo resultado do ramo acima, entrada diferente — é " +
				"precisamente por serem indistinguíveis no resultado que os " +
				"ramos têm de estar separados no código",
		},
		{
			name:     "explicit_duration_is_carried",
			duration: &eightHours,
			want:     8 * time.Hour,
			why: "o caminho de SUCESSO com valor: se este passasse a 0, os dois " +
				"testes acima continuariam verdes e a rota silenciaria tudo " +
				"para sempre",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm := &contractsfake.ChatMuter{}
			_, err := chat.NewMuteChatUseCase(cm, &contractsfake.JIDResolver{}, &contractsfake.Logger{}).
				Execute(context.Background(), userID, domain.MuteChatRequest{
					Jid:          muteJID,
					Mute:         true,
					MuteDuration: tc.duration,
				})
			if err != nil {
				t.Fatalf("Execute = %v, want nil (%s)", err, tc.why)
			}
			if len(cm.MuteChatCalls) != 1 {
				t.Fatalf("MuteChatCalls = %d, want 1", len(cm.MuteChatCalls))
			}
			if got := cm.MuteChatCalls[0].MuteDuration; got != tc.want {
				t.Fatalf("MuteDuration na porta = %v, want %v — %s", got, tc.want, tc.why)
			}
		})
	}
}

// Com `mute:false` a duração é ignorada: nem sequer é validada. Trava a ordem —
// a validação de duração está DENTRO do ramo `req.Mute`, e tirá-la de lá faria
// um unmute com duração inválida ser recusado sem razão.
func TestMuteChat_UnmuteIgnoresDuration(t *testing.T) {
	oneHour := time.Hour // não pertence ao conjunto permitido

	cm := &contractsfake.ChatMuter{}
	_, err := chat.NewMuteChatUseCase(cm, &contractsfake.JIDResolver{}, &contractsfake.Logger{}).
		Execute(context.Background(), userID, domain.MuteChatRequest{
			Jid:          muteJID,
			Mute:         false,
			MuteDuration: &oneHour,
		})
	if err != nil {
		t.Fatalf("Execute = %v, want nil: desligar o silêncio não depende de duração", err)
	}
	if len(cm.MuteChatCalls) != 1 {
		t.Fatalf("MuteChatCalls = %d, want 1", len(cm.MuteChatCalls))
	}
	if cm.MuteChatCalls[0].Mute {
		t.Fatal("Mute = true na porta, want false")
	}
}
