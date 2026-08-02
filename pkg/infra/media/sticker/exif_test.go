package sticker

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// webpDecoderRegistered garante que image.DecodeConfig reconheca o container
// RIFF/WEBP dentro deste binario de teste. O repositorio nao importa nenhum
// decodificador WebP em producao, entao sem este registro InjectWebPEXIF nunca
// passaria da chamada a image.DecodeConfig e o caminho feliz ficaria
// inalcancavel. O stub le apenas largura/altura do payload VP8X.
func init() {
	image.RegisterFormat("stickertest-webp", "RIFF????WEBP", decodeStubWebP, decodeStubWebPConfig)
}

// undecodableChunkTag marca uma entrada que o stub recusa a decodificar. Serve
// para exercitar o ramo de erro de image.DecodeConfig em InjectWebPEXIF sem
// quebrar a validacao RIFF/WEBP que vem antes dele.
const undecodableChunkTag = "BADC"

func decodeStubWebPConfig(r io.Reader) (image.Config, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return image.Config{}, err
	}
	if bytes.Contains(data, []byte(undecodableChunkTag)) {
		return image.Config{}, errors.New("stub webp: payload marcado como indecodificavel")
	}
	w, h := stubWebPDimensions(data)
	return image.Config{ColorModel: color.RGBAModel, Width: w, Height: h}, nil
}

func decodeStubWebP(r io.Reader) (image.Image, error) {
	cfg, err := decodeStubWebPConfig(r)
	if err != nil {
		return nil, err
	}
	return image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height)), nil
}

// stubWebPDimensions procura um chunk VP8X e le width-1/height-1 de 24 bits.
// Sem VP8X devolve 64x64, um tamanho arbitrario mas valido.
func stubWebPDimensions(data []byte) (int, int) {
	pos := RiffHeaderSize
	for pos+ChunkHeaderSize <= len(data) {
		tag := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		if tag == "VP8X" && size >= Vp8xPayloadSize && pos+Vp8xChunkSize <= len(data) {
			w := uint24LE(data[pos+Vp8xWidthOffset:]) + 1
			h := uint24LE(data[pos+Vp8xHeightOffset:]) + 1
			return w, h
		}
		pos += ChunkHeaderSize + size + (size & 1)
	}
	return 64, 64
}

func uint24LE(b []byte) int {
	return int(b[0]) | int(b[1])<<8 | int(b[2])<<16
}

// buildWebP monta um RIFF/WEBP minimo a partir de chunks brutos.
func buildWebP(chunks ...[]byte) []byte {
	var out bytes.Buffer
	out.WriteString("RIFF")
	out.Write([]byte{0, 0, 0, 0})
	out.WriteString("WEBP")
	for _, c := range chunks {
		out.Write(c)
	}
	b := out.Bytes()
	binary.LittleEndian.PutUint32(b[RiffSizeOffset:], uint32(len(b)-8))
	return b
}

// rawChunk monta um chunk tag+size+payload com o padding de paridade.
func rawChunk(tag string, payload []byte) []byte {
	var buf bytes.Buffer
	WriteChunk(&buf, tag, payload)
	return buf.Bytes()
}

func pngFixture(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatalf("encode png fixture: %v", err)
	}
	return buf.Bytes()
}

// ---------------------------------------------------------------------------
// IsValidWebP
// ---------------------------------------------------------------------------

func TestIsValidWebP(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want bool
	}{
		{"nil", nil, false},
		{"curto demais", []byte("RIFF0000WEB"), false},
		{"magic RIFF errado", append([]byte("XIFF\x00\x00\x00\x00WEBP"), 0), false},
		{"magic WEBP errado", append([]byte("RIFF\x00\x00\x00\x00WEBX"), 0), false},
		{"valido", buildWebP(), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidWebP(tc.in); got != tc.want {
				t.Fatalf("IsValidWebP() = %v, quero %v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// PutUint24LE / CreateVP8XChunk
// ---------------------------------------------------------------------------

func TestPutUint24LE(t *testing.T) {
	tests := []struct {
		name string
		v    int
		want []byte
	}{
		{"zero", 0, []byte{0x00, 0x00, 0x00}},
		{"um byte", 0x7F, []byte{0x7F, 0x00, 0x00}},
		{"dois bytes", 0x1234, []byte{0x34, 0x12, 0x00}},
		{"tres bytes", 0xABCDEF, []byte{0xEF, 0xCD, 0xAB}},
		{"trunca acima de 24 bits", 0x11223344, []byte{0x44, 0x33, 0x22}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := make([]byte, 3)
			PutUint24LE(b, tc.v)
			if !bytes.Equal(b, tc.want) {
				t.Fatalf("PutUint24LE(%#x) = % x, quero % x", tc.v, b, tc.want)
			}
		})
	}
}

func TestCreateVP8XChunk(t *testing.T) {
	chunk := CreateVP8XChunk(512, 384)

	if len(chunk) != Vp8xChunkSize {
		t.Fatalf("tamanho = %d, quero %d", len(chunk), Vp8xChunkSize)
	}
	if string(chunk[0:4]) != "VP8X" {
		t.Fatalf("tag = %q, quero VP8X", chunk[0:4])
	}
	if got := binary.LittleEndian.Uint32(chunk[4:8]); got != Vp8xPayloadSize {
		t.Fatalf("payload size = %d, quero %d", got, Vp8xPayloadSize)
	}
	if chunk[Vp8xFlagsOffset]&Vp8xFlagEXIF == 0 {
		t.Fatalf("flag EXIF ausente: %#x", chunk[Vp8xFlagsOffset])
	}
	if got := uint24LE(chunk[Vp8xWidthOffset:]); got != 511 {
		t.Fatalf("width-1 = %d, quero 511", got)
	}
	if got := uint24LE(chunk[Vp8xHeightOffset:]); got != 383 {
		t.Fatalf("height-1 = %d, quero 383", got)
	}
}

// ---------------------------------------------------------------------------
// WriteChunk / AssembleWebP
// ---------------------------------------------------------------------------

func TestWriteChunkPadding(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		want    []byte
	}{
		{"payload par sem padding", []byte{1, 2}, []byte("TAGS\x02\x00\x00\x00\x01\x02")},
		{"payload impar ganha padding", []byte{1}, []byte("TAGS\x01\x00\x00\x00\x01\x00")},
		{"payload vazio", nil, []byte("TAGS\x00\x00\x00\x00")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			WriteChunk(&buf, "TAGS", tc.payload)
			if !bytes.Equal(buf.Bytes(), tc.want) {
				t.Fatalf("WriteChunk = % x, quero % x", buf.Bytes(), tc.want)
			}
		})
	}
}

func TestAssembleWebP(t *testing.T) {
	vp8 := rawChunk("VP8 ", []byte{9, 9, 9, 9})
	exif := []byte{0xAA, 0xBB, 0xCC}

	out := AssembleWebP([][]byte{vp8}, exif)

	if !IsValidWebP(out) {
		t.Fatalf("saida nao e' um RIFF WEBP: % x", out[:12])
	}
	if got := binary.LittleEndian.Uint32(out[RiffSizeOffset:]); int(got) != len(out)-8 {
		t.Fatalf("tamanho RIFF = %d, quero %d", got, len(out)-8)
	}
	if !bytes.Contains(out, vp8) {
		t.Fatal("chunk VP8 original ausente na saida")
	}
	// EXIF impar (3 bytes) tem de ganhar o byte de padding.
	wantExif := append([]byte("EXIF\x03\x00\x00\x00"), 0xAA, 0xBB, 0xCC, 0x00)
	if !bytes.HasSuffix(out, wantExif) {
		t.Fatalf("chunk EXIF final = % x, quero sufixo % x", out, wantExif)
	}
}

// ---------------------------------------------------------------------------
// ParseWebPChunks
// ---------------------------------------------------------------------------

// le chunks e ignora EXIF preexistente
func TestParseWebPChunksIgnoraEXIFPreexistente(t *testing.T) {
	in := buildWebP(
		rawChunk("VP8 ", []byte{1, 2, 3, 4}),
		rawChunk("EXIF", []byte{7, 7}),
		rawChunk("ANIM", []byte{5, 5, 5, 5}),
	)

	chunks, vp8xIndex, err := ParseWebPChunks(in)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if vp8xIndex != -1 {
		t.Fatalf("vp8xIndex = %d, quero -1", vp8xIndex)
	}
	if len(chunks) != 2 {
		t.Fatalf("quantidade de chunks = %d, quero 2", len(chunks))
	}
	for _, c := range chunks {
		if string(c[0:4]) == "EXIF" {
			t.Fatal("chunk EXIF preexistente deveria ter sido descartado")
		}
	}
}

// localiza VP8X e preserva o indice
func TestParseWebPChunksLocalizaVP8X(t *testing.T) {
	in := buildWebP(CreateVP8XChunk(100, 200), rawChunk("VP8 ", []byte{1, 2}))

	chunks, vp8xIndex, err := ParseWebPChunks(in)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if vp8xIndex != 0 {
		t.Fatalf("vp8xIndex = %d, quero 0", vp8xIndex)
	}
	if len(chunks) != 2 {
		t.Fatalf("quantidade de chunks = %d, quero 2", len(chunks))
	}
}

// VP8X curto demais nao conta como VP8X
func TestParseWebPChunksVP8XCurtoDemais(t *testing.T) {
	in := buildWebP(rawChunk("VP8X", []byte{1, 2}))

	_, vp8xIndex, err := ParseWebPChunks(in)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if vp8xIndex != -1 {
		t.Fatalf("vp8xIndex = %d, quero -1", vp8xIndex)
	}
}

// payload impar recebe padding no chunk copiado
func TestParseWebPChunksPayloadImparRecebePadding(t *testing.T) {
	in := buildWebP(rawChunk("VP8 ", []byte{0xFF}))

	chunks, _, err := ParseWebPChunks(in)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("quantidade de chunks = %d, quero 1", len(chunks))
	}
	if len(chunks[0]) != ChunkHeaderSize+1+1 {
		t.Fatalf("tamanho do chunk = %d, quero %d", len(chunks[0]), ChunkHeaderSize+2)
	}
	if chunks[0][ChunkHeaderSize+1] != 0 {
		t.Fatalf("byte de padding = %#x, quero 0", chunks[0][ChunkHeaderSize+1])
	}
}

// chunk truncado vira erro
func TestParseWebPChunksChunkTruncado(t *testing.T) {
	in := buildWebP()
	in = append(in, []byte("VP8 ")...)
	in = append(in, 0xFF, 0xFF, 0x00, 0x00) // declara 65535 bytes que nao existem
	in = append(in, 1, 2, 3)

	chunks, vp8xIndex, err := ParseWebPChunks(in)
	if err == nil {
		t.Fatal("quero erro de chunk truncado")
	}
	if !strings.Contains(err.Error(), "truncated webp chunk: VP8 ") {
		t.Fatalf("mensagem = %q", err.Error())
	}
	if chunks != nil || vp8xIndex != -1 {
		t.Fatalf("quero (nil, -1), tenho (%v, %d)", chunks, vp8xIndex)
	}
}

// ---------------------------------------------------------------------------
// EnsureVP8XWithEXIF
// ---------------------------------------------------------------------------

func TestEnsureVP8XWithEXIF(t *testing.T) {
	t.Run("VP8X existente so ganha a flag", func(t *testing.T) {
		vp8x := CreateVP8XChunk(10, 20)
		vp8x[Vp8xFlagsOffset] = 0x00 // zera para provar que a flag e' setada aqui
		chunks := [][]byte{vp8x, rawChunk("VP8 ", []byte{1, 2})}

		got := EnsureVP8XWithEXIF(chunks, 0, 999, 999)

		if len(got) != 2 {
			t.Fatalf("quantidade de chunks = %d, quero 2", len(got))
		}
		if got[0][Vp8xFlagsOffset]&Vp8xFlagEXIF == 0 {
			t.Fatal("flag EXIF nao foi setada no VP8X existente")
		}
		// As dimensoes do VP8X existente nao sao reescritas.
		if w := uint24LE(got[0][Vp8xWidthOffset:]); w != 9 {
			t.Fatalf("width-1 = %d, quero 9 (preservado)", w)
		}
	})

	t.Run("sem VP8X um chunk novo e' prefixado", func(t *testing.T) {
		chunks := [][]byte{rawChunk("VP8 ", []byte{1, 2})}

		got := EnsureVP8XWithEXIF(chunks, -1, 512, 512)

		if len(got) != 2 {
			t.Fatalf("quantidade de chunks = %d, quero 2", len(got))
		}
		if string(got[0][0:4]) != "VP8X" {
			t.Fatalf("primeiro chunk = %q, quero VP8X", got[0][0:4])
		}
		if w := uint24LE(got[0][Vp8xWidthOffset:]); w != 511 {
			t.Fatalf("width-1 = %d, quero 511", w)
		}
	})
}

// ---------------------------------------------------------------------------
// BuildStickerMetadata
// ---------------------------------------------------------------------------

func TestBuildStickerMetadata(t *testing.T) {
	tests := []struct {
		name                            string
		packID, packName, packPublisher string
		emojis                          []string
		want                            map[string]interface{}
	}{
		{name: "tudo vazio devolve nil", want: nil},
		{
			name:   "so emojis",
			emojis: []string{"🙂"},
			want:   map[string]interface{}{"emojis": []string{"🙂"}},
		},
		{
			name:   "so packID",
			packID: "pack-1",
			want:   map[string]interface{}{"sticker-pack-id": "pack-1"},
		},
		{
			name:     "so packName",
			packName: "Meu Pack",
			want:     map[string]interface{}{"sticker-pack-name": "Meu Pack"},
		},
		{
			name:          "so publisher",
			packPublisher: "wa-api",
			want:          map[string]interface{}{"sticker-pack-publisher": "wa-api"},
		},
		{
			name:          "completo",
			packID:        "pack-1",
			packName:      "Meu Pack",
			packPublisher: "wa-api",
			emojis:        []string{"🙂", "🎉"},
			want: map[string]interface{}{
				"sticker-pack-id":        "pack-1",
				"sticker-pack-name":      "Meu Pack",
				"sticker-pack-publisher": "wa-api",
				"emojis":                 []string{"🙂", "🎉"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildStickerMetadata(tc.packID, tc.packName, tc.packPublisher, tc.emojis)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("BuildStickerMetadata = %#v, quero %#v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// BuildWhatsAppEXIF
// ---------------------------------------------------------------------------

func TestBuildWhatsAppEXIF(t *testing.T) {
	meta := map[string]interface{}{"sticker-pack-id": "pack-1"}

	out := BuildWhatsAppEXIF(meta)

	wantHeader := []byte{0x49, 0x49, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00, 0x01, 0x00, 0x41, 0x57, 0x07, 0x00}
	if !bytes.HasPrefix(out, wantHeader) {
		t.Fatalf("header = % x, quero % x", out[:len(wantHeader)], wantHeader)
	}

	jsonBytes, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal de referencia: %v", err)
	}
	gotLen := binary.LittleEndian.Uint32(out[14:18])
	if int(gotLen) != len(jsonBytes) {
		t.Fatalf("campo de tamanho = %d, quero %d", gotLen, len(jsonBytes))
	}
	if !bytes.Equal(out[18:22], []byte{0x16, 0x00, 0x00, 0x00}) {
		t.Fatalf("footer = % x, quero 16 00 00 00", out[18:22])
	}
	if !bytes.Equal(out[22:], jsonBytes) {
		t.Fatalf("payload JSON = %q, quero %q", out[22:], jsonBytes)
	}
}

func TestBuildWhatsAppEXIFMarshalError(t *testing.T) {
	// Um canal nao e' serializavel por encoding/json.
	out := BuildWhatsAppEXIF(map[string]interface{}{"bad": make(chan int)})
	if out != nil {
		t.Fatalf("quero nil quando o marshal falha, tenho % x", out)
	}
}

// ---------------------------------------------------------------------------
// InjectWebPEXIF
// ---------------------------------------------------------------------------

// exifFixture: os bytes EXIF de um pack minimo, reusados pelos casos de injecao.
func exifFixture() []byte {
	return BuildWhatsAppEXIF(map[string]interface{}{"sticker-pack-id": "pack-1"})
}

// entrada que nao e' RIFF WEBP
func TestInjectWebPEXIFEntradaNaoWebP(t *testing.T) {
	_, err := InjectWebPEXIF([]byte("nao sou webp"), exifFixture())
	if err == nil || !strings.Contains(err.Error(), "not a RIFF WEBP file") {
		t.Fatalf("erro = %v, quero 'not a RIFF WEBP file'", err)
	}
}

// config de imagem indecodificavel
func TestInjectWebPEXIFConfigIndecodificavel(t *testing.T) {
	in := buildWebP(rawChunk(undecodableChunkTag, []byte{1, 2, 3, 4}))

	_, err := InjectWebPEXIF(in, exifFixture())
	if err == nil || !strings.Contains(err.Error(), "failed to decode image config") {
		t.Fatalf("erro = %v, quero 'failed to decode image config'", err)
	}
}

// chunk truncado propaga o erro do parser
func TestInjectWebPEXIFChunkTruncado(t *testing.T) {
	in := buildWebP(CreateVP8XChunk(64, 64))
	in = append(in, []byte("VP8 ")...)
	in = append(in, 0xFF, 0xFF, 0x00, 0x00)

	_, err := InjectWebPEXIF(in, exifFixture())
	if err == nil || !strings.Contains(err.Error(), "truncated webp chunk") {
		t.Fatalf("erro = %v, quero 'truncated webp chunk'", err)
	}
}

// injeta EXIF criando VP8X quando nao existe
func TestInjectWebPEXIFCriaVP8X(t *testing.T) {
	in := buildWebP(rawChunk("VP8 ", []byte{1, 2, 3, 4}))

	out, err := InjectWebPEXIF(in, exifFixture())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !IsValidWebP(out) {
		t.Fatal("saida nao e' um RIFF WEBP")
	}

	chunks, vp8xIndex, err := ParseWebPChunks(out)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if vp8xIndex != 0 {
		t.Fatalf("vp8xIndex = %d, quero 0 (VP8X criado)", vp8xIndex)
	}
	if chunks[0][Vp8xFlagsOffset]&Vp8xFlagEXIF == 0 {
		t.Fatal("flag EXIF ausente no VP8X criado")
	}
	if !bytes.Contains(out, exifFixture()) {
		t.Fatal("payload EXIF ausente na saida")
	}
	// O stub reporta 64x64 quando nao ha VP8X na entrada.
	if w := uint24LE(chunks[0][Vp8xWidthOffset:]); w != 63 {
		t.Fatalf("width-1 = %d, quero 63", w)
	}
}

// reaproveita VP8X existente
func TestInjectWebPEXIFReaproveitaVP8X(t *testing.T) {
	in := buildWebP(CreateVP8XChunk(128, 96), rawChunk("VP8 ", []byte{1, 2, 3, 4}))

	out, err := InjectWebPEXIF(in, exifFixture())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	chunks, vp8xIndex, err := ParseWebPChunks(out)
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if vp8xIndex != 0 || len(chunks) != 2 {
		t.Fatalf("vp8xIndex = %d, chunks = %d, quero 0 e 2", vp8xIndex, len(chunks))
	}
	if w := uint24LE(chunks[0][Vp8xWidthOffset:]); w != 127 {
		t.Fatalf("width-1 = %d, quero 127 (VP8X preservado)", w)
	}
}

// descarta EXIF preexistente antes de reinjetar
func TestInjectWebPEXIFDescartaEXIFPreexistente(t *testing.T) {
	in := buildWebP(rawChunk("VP8 ", []byte{1, 2, 3, 4}), rawChunk("EXIF", []byte("velho")))

	out, err := InjectWebPEXIF(in, exifFixture())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if bytes.Contains(out, []byte("velho")) {
		t.Fatal("EXIF antigo permaneceu na saida")
	}
}

// ---------------------------------------------------------------------------
// EmbedStickerEXIF
// ---------------------------------------------------------------------------

func TestEmbedStickerEXIF(t *testing.T) {
	t.Run("sem metadados devolve a entrada intacta", func(t *testing.T) {
		in := buildWebP(rawChunk("VP8 ", []byte{1, 2, 3, 4}))

		out := EmbedStickerEXIF(in, "", "", "", nil)

		if !bytes.Equal(out, in) {
			t.Fatal("entrada deveria voltar byte a byte quando nao ha metadados")
		}
	})

	t.Run("entrada invalida devolve a entrada intacta", func(t *testing.T) {
		in := []byte("nao sou webp")

		out := EmbedStickerEXIF(in, "pack-1", "Pack", "wa-api", []string{"🙂"})

		if !bytes.Equal(out, in) {
			t.Fatalf("quero a entrada original de volta, tenho % x", out)
		}
	})

	t.Run("caminho feliz embute os metadados", func(t *testing.T) {
		in := buildWebP(rawChunk("VP8 ", []byte{1, 2, 3, 4}))

		out := EmbedStickerEXIF(in, "pack-1", "Pack", "wa-api", []string{"🙂"})

		if bytes.Equal(out, in) {
			t.Fatal("saida deveria diferir da entrada")
		}
		if !bytes.Contains(out, []byte("sticker-pack-id")) {
			t.Fatal("metadados do pack ausentes na saida")
		}
		if !bytes.Contains(out, []byte("wa-api")) {
			t.Fatal("publisher ausente na saida")
		}
	})
}

// ---------------------------------------------------------------------------
// ConvertToWebPSticker / ProcessStickerData
// ---------------------------------------------------------------------------

func TestConvertToWebPStickerPassthrough(t *testing.T) {
	// image/webp nao entra em nenhum dos ramos de conversao.
	in := buildWebP(rawChunk("VP8 ", []byte{1, 2, 3, 4}))

	out, mimeType, err := ConvertToWebPSticker(in, "image/webp")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if mimeType != "image/webp" {
		t.Fatalf("mimeType = %q, quero image/webp", mimeType)
	}
	if !bytes.Equal(out, in) {
		t.Fatal("dados deveriam passar intactos")
	}
}

func TestConvertToWebPStickerDetectaMimeQuandoNaoHaOverride(t *testing.T) {
	// text/plain nao casa com nenhum ramo: o passthrough devolve o mime detectado.
	in := []byte("apenas texto simples, sem magic number de imagem")

	out, mimeType, err := ConvertToWebPSticker(in, "")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !strings.HasPrefix(mimeType, "text/plain") {
		t.Fatalf("mimeType = %q, quero text/plain*", mimeType)
	}
	if !bytes.Equal(out, in) {
		t.Fatal("dados deveriam passar intactos")
	}
}

func TestConvertToWebPStickerErroDeConversao(t *testing.T) {
	withFailingFFmpeg(t)

	t.Run("video", func(t *testing.T) {
		_, _, err := ConvertToWebPSticker([]byte("dados"), "video/mp4")
		if err == nil || !strings.Contains(err.Error(), "failed to convert video/gif sticker to webp") {
			t.Fatalf("erro = %v", err)
		}
	})

	t.Run("gif", func(t *testing.T) {
		_, _, err := ConvertToWebPSticker([]byte("dados"), "image/gif")
		if err == nil || !strings.Contains(err.Error(), "failed to convert video/gif sticker to webp") {
			t.Fatalf("erro = %v", err)
		}
	})

	t.Run("imagem", func(t *testing.T) {
		_, _, err := ConvertToWebPSticker([]byte("dados"), "image/png")
		if err == nil || !strings.Contains(err.Error(), "failed to convert image sticker to webp") {
			t.Fatalf("erro = %v", err)
		}
	})
}

func TestConvertToWebPStickerConversaoBemSucedida(t *testing.T) {
	want := buildWebP(rawChunk("VP8 ", []byte{7, 7, 7, 7}))
	withFakeFFmpeg(t, want)

	t.Run("imagem png vira webp", func(t *testing.T) {
		out, mimeType, err := ConvertToWebPSticker(pngFixture(t, 8, 8), "")
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if mimeType != "image/webp" {
			t.Fatalf("mimeType = %q, quero image/webp", mimeType)
		}
		if !bytes.Equal(out, want) {
			t.Fatalf("saida = % x, quero % x", out, want)
		}
	})

	t.Run("video vira webp", func(t *testing.T) {
		out, mimeType, err := ConvertToWebPSticker([]byte("bytes de video"), "video/mp4")
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if mimeType != "image/webp" {
			t.Fatalf("mimeType = %q, quero image/webp", mimeType)
		}
		if !bytes.Equal(out, want) {
			t.Fatalf("saida = % x, quero % x", out, want)
		}
	})
}

// payload que nao e' data URI
func TestProcessStickerDataNaoEDataURI(t *testing.T) {
	_, _, err := ProcessStickerData("http://exemplo/x.png", "", "", "", "", nil)
	if err == nil || !strings.Contains(err.Error(), `data should start with "data:mime/type;base64,"`) {
		t.Fatalf("erro = %v", err)
	}
}

// data URI malformada
func TestProcessStickerDataDataURIMalformada(t *testing.T) {
	_, _, err := ProcessStickerData("data:isso-nao-e-uma-data-uri", "", "", "", "", nil)
	if err == nil || !strings.Contains(err.Error(), "could not decode base64 encoded data from payload") {
		t.Fatalf("erro = %v", err)
	}
}

// erro de conversao propaga
func TestProcessStickerDataErroDeConversao(t *testing.T) {
	withFailingFFmpeg(t)

	_, _, err := ProcessStickerData("data:image/png;base64,QUJD", "image/png", "", "", "", nil)
	if err == nil || !strings.Contains(err.Error(), "failed to convert image sticker to webp") {
		t.Fatalf("erro = %v", err)
	}
}

// webp ja pronto ganha EXIF sem passar por ffmpeg
func TestProcessStickerDataWebPProntoGanhaEXIF(t *testing.T) {
	webp := buildWebP(rawChunk("VP8 ", []byte{1, 2, 3, 4}))
	uri := "data:image/webp;base64," + base64Of(webp)

	out, mimeType, err := ProcessStickerData(uri, "image/webp", "pack-1", "Pack", "wa-api", []string{"🙂"})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if mimeType != "image/webp" {
		t.Fatalf("mimeType = %q, quero image/webp", mimeType)
	}
	if !bytes.Contains(out, []byte("sticker-pack-id")) {
		t.Fatal("EXIF do pack ausente na saida")
	}
}

// mime nao conversivel devolve os bytes originais sem EXIF
func TestProcessStickerDataMimeNaoConversivel(t *testing.T) {
	uri := "data:application/octet-stream;base64," + base64Of([]byte{0x01, 0x02, 0x03})

	out, mimeType, err := ProcessStickerData(uri, "application/octet-stream", "pack-1", "", "", nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if mimeType != "application/octet-stream" {
		t.Fatalf("mimeType = %q", mimeType)
	}
	if !bytes.Equal(out, []byte{0x01, 0x02, 0x03}) {
		t.Fatalf("saida = % x, quero 01 02 03", out)
	}
}
