package armadillo_test

import (
	_ "wa-api/internal/wa-noise/protocol/proto/instamadilloAddMessage"
	_ "wa-api/internal/wa-noise/protocol/proto/instamadilloCoreTypeActionLog"
	_ "wa-api/internal/wa-noise/protocol/proto/instamadilloCoreTypeAdminMessage"
	_ "wa-api/internal/wa-noise/protocol/proto/instamadilloCoreTypeCollection"
	_ "wa-api/internal/wa-noise/protocol/proto/instamadilloCoreTypeLink"
	_ "wa-api/internal/wa-noise/protocol/proto/instamadilloCoreTypeMedia"
	_ "wa-api/internal/wa-noise/protocol/proto/instamadilloCoreTypeText"
	_ "wa-api/internal/wa-noise/protocol/proto/instamadilloDeleteMessage"
	_ "wa-api/internal/wa-noise/protocol/proto/instamadilloSupplementMessage"
	_ "wa-api/internal/wa-noise/protocol/proto/instamadilloTransportPayload"
	_ "wa-api/internal/wa-noise/protocol/proto/instamadilloXmaContentRef"
	_ "wa-api/internal/wa-noise/protocol/proto/waAICommon"
	_ "wa-api/internal/wa-noise/protocol/proto/waAICommonDeprecated"
	_ "wa-api/internal/wa-noise/protocol/proto/waAdv"
	_ "wa-api/internal/wa-noise/protocol/proto/waArmadilloApplication"
	_ "wa-api/internal/wa-noise/protocol/proto/waArmadilloBackupCommon"
	_ "wa-api/internal/wa-noise/protocol/proto/waArmadilloBackupMessage"
	_ "wa-api/internal/wa-noise/protocol/proto/waArmadilloICDC"
	_ "wa-api/internal/wa-noise/protocol/proto/waArmadilloMiTransportAdminMessage"
	_ "wa-api/internal/wa-noise/protocol/proto/waArmadilloTransportEvent"
	_ "wa-api/internal/wa-noise/protocol/proto/waArmadilloXMA"
	_ "wa-api/internal/wa-noise/protocol/proto/waBotMetadata"
	_ "wa-api/internal/wa-noise/protocol/proto/waCert"
	_ "wa-api/internal/wa-noise/protocol/proto/waChatLockSettings"
	_ "wa-api/internal/wa-noise/protocol/proto/waCommon"
	_ "wa-api/internal/wa-noise/protocol/proto/waCommonParameterised"
	_ "wa-api/internal/wa-noise/protocol/proto/waCompanionReg"
	_ "wa-api/internal/wa-noise/protocol/proto/waConsumerApplication"
	_ "wa-api/internal/wa-noise/protocol/proto/waConsumerApplicationParameterised"
	_ "wa-api/internal/wa-noise/protocol/proto/waDeviceCapabilities"
	_ "wa-api/internal/wa-noise/protocol/proto/waE2E"
	_ "wa-api/internal/wa-noise/protocol/proto/waE2EGuest"
	_ "wa-api/internal/wa-noise/protocol/proto/waEphemeral"
	_ "wa-api/internal/wa-noise/protocol/proto/waFingerprint"
	_ "wa-api/internal/wa-noise/protocol/proto/waGroupHistory"
	_ "wa-api/internal/wa-noise/protocol/proto/waHistorySync"
	_ "wa-api/internal/wa-noise/protocol/proto/waLidMigrationSyncPayload"
	_ "wa-api/internal/wa-noise/protocol/proto/waMediaEntryData"
	_ "wa-api/internal/wa-noise/protocol/proto/waMediaTransport"
	_ "wa-api/internal/wa-noise/protocol/proto/waMmsRetry"
	_ "wa-api/internal/wa-noise/protocol/proto/waMsgApplication"
	_ "wa-api/internal/wa-noise/protocol/proto/waMsgTransport"
	_ "wa-api/internal/wa-noise/protocol/proto/waMultiDevice"
	_ "wa-api/internal/wa-noise/protocol/proto/waQuickPromotionSurfaces"
	_ "wa-api/internal/wa-noise/protocol/proto/waReporting"
	_ "wa-api/internal/wa-noise/protocol/proto/waRoutingInfo"
	_ "wa-api/internal/wa-noise/protocol/proto/waServerSync"
	_ "wa-api/internal/wa-noise/protocol/proto/waStatusAttributions"
	_ "wa-api/internal/wa-noise/protocol/proto/waSyncAction"
	_ "wa-api/internal/wa-noise/protocol/proto/waSyncdSnapshotRecovery"
	_ "wa-api/internal/wa-noise/protocol/proto/waUserPassword"
	_ "wa-api/internal/wa-noise/protocol/proto/waVnameCert"
	_ "wa-api/internal/wa-noise/protocol/proto/waWa6"
	_ "wa-api/internal/wa-noise/protocol/proto/waWeb"
	_ "wa-api/internal/wa-noise/protocol/proto/waWinUIApi"

	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

// wantGoPackagePrefix e' o prefixo que todo descritor deste modulo deve
// declarar em option go_package.
const wantGoPackagePrefix = "wa-api/internal/wa-noise/protocol/proto/"

// TestDescritoresParseiamEDeclaramGoPackageDoModulo prova duas coisas de uma
// vez sobre os 55 pacotes gerados:
//
//  1. cada descritor serializado ainda PARSEIA — os blank imports acima fazem
//     o runtime do protobuf desserializar cada FileDescriptorProto no init do
//     pacote, e um descritor corrompido entra em panic ali, antes deste corpo
//     rodar;
//  2. o go_package de cada arquivo aponta para o path real do modulo.
//
// O ponto (1) e' o que trava a reescrita do go_package feita dentro dos bytes
// do descritor: o campo carrega prefixo de tamanho (varint) e um
// busca-e-substitui textual deixaria os tamanhos mentindo sobre o conteudo.
func TestDescritoresParseiamEDeclaramGoPackageDoModulo(t *testing.T) {
	vistos := 0
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		opts, ok := fd.Options().(*descriptorpb.FileOptions)
		if !ok || opts == nil {
			return true
		}
		gp := opts.GetGoPackage()
		if !strings.HasPrefix(gp, wantGoPackagePrefix) {
			return true // arquivo de outra dependencia (protobuf runtime etc.)
		}
		vistos++
		if strings.Contains(gp, "go.mau.fi") {
			t.Errorf("%s: go_package ainda aponta para o modulo externo: %q", fd.Path(), gp)
		}
		if fd.Messages().Len() == 0 && fd.Enums().Len() == 0 {
			t.Errorf("%s: descritor sem mensagem nem enum — parse suspeito", fd.Path())
		}
		return true
	})
	if vistos < 55 {
		t.Fatalf("esperava ao menos %d descritores do modulo registrados, vi %d", 55, vistos)
	}
}
