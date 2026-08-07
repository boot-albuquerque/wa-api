package group

import (
	"fmt"
	"slices"
	"testing"

	"wa-api/internal/wa-noise/protocol/types"
)

// A F53 registra que GetOrFetch devolve o *Meta VIVO do mapa: o caminho de
// envio le' Members sem lock enquanto o handler de notificacao w:gp2 o muta sob
// lock. A correcao proposta e' devolver uma copia — e a entrada pede medicao
// antes, porque o custo cai num caminho quente (todo envio para grupo com cache
// quente) e a lista de membros de um grupo grande tem milhares de entradas.
//
// Este benchmark existe para responder essa pergunta com numero, nao com
// palpite. O limite do WhatsApp e' 1024 membros por grupo; 4096 esta' aqui como
// margem.
func benchMeta(n int) *Meta {
	members := make([]types.JID, n)
	for i := range members {
		members[i] = types.NewJID(fmt.Sprintf("5511%09d", i), types.DefaultUserServer)
	}
	return &Meta{
		AddressingMode: types.AddressingModePN,
		Members:        members,
	}
}

func BenchmarkMetaPonteiroVivo(b *testing.B) {
	for _, n := range []int{2, 50, 256, 1024, 4096} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			meta := benchMeta(n)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				got := meta
				if len(got.Members) != n {
					b.Fatal("membros perdidos")
				}
			}
		})
	}
}

func BenchmarkMetaClonado(b *testing.B) {
	for _, n := range []int{2, 50, 256, 1024, 4096} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			meta := benchMeta(n)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				got := &Meta{
					AddressingMode:             meta.AddressingMode,
					CommunityAnnouncementGroup: meta.CommunityAnnouncementGroup,
					Members:                    slices.Clone(meta.Members),
				}
				if len(got.Members) != n {
					b.Fatal("membros perdidos")
				}
			}
		})
	}
}
