package handlers

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// JPEGThumbnail e' a unica funcao pura do pacote e o ponto de entrada do
// caminho de midia que a F1b ja' registrou como fonte de nil deref — por isso
// o caso nil vem primeiro.

// hugeImage tem limites grandes demais para o codificador JPEG (>65535) sem
// alocar um pixel sequer. E' o que torna alcancavel o ramo de erro de
// jpeg.Encode: o destino e' um bytes.Buffer, que nunca falha na escrita, e a
// unica falha possivel vem do proprio codificador.
type hugeImage struct{ w, h int }

func (hugeImage) ColorModel() color.Model   { return color.GrayModel }
func (i hugeImage) Bounds() image.Rectangle { return image.Rect(0, 0, i.w, i.h) }
func (hugeImage) At(_, _ int) color.Color   { return color.Gray{} }

func TestJPEGThumbnail_NilImage(t *testing.T) {
	got, err := JPEGThumbnail(nil, 64, 64)

	if err == nil {
		t.Fatal("imagem nil devolveu erro nil — o caminho de midia perde o guarda de nil deref")
	}
	if got != nil {
		t.Fatalf("erro veio acompanhado de %d bytes de saida", len(got))
	}
}

func TestJPEGThumbnail_Encodes(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 64; x++ {
			src.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}

	cases := []struct {
		name          string
		width, height uint
		wantW, wantH  int
	}{
		{"reduz mantendo proporcao", 16, 16, 16, 8},
		{"limite maior que a origem preserva o tamanho", 256, 256, 64, 32},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := JPEGThumbnail(src, tc.width, tc.height)
			if err != nil {
				t.Fatalf("JPEGThumbnail: %v", err)
			}

			decoded, err := jpeg.Decode(bytes.NewReader(got))
			if err != nil {
				t.Fatalf("saida nao e' JPEG decodificavel: %v", err)
			}
			if b := decoded.Bounds(); b.Dx() != tc.wantW || b.Dy() != tc.wantH {
				t.Fatalf("dimensoes = %dx%d, esperado %dx%d", b.Dx(), b.Dy(), tc.wantW, tc.wantH)
			}
		})
	}
}

// TestJPEGThumbnail_EncodeFailure cobre o ramo em que o codificador recusa a
// imagem: o erro tem de subir, e nao virar buffer parcial.
func TestJPEGThumbnail_EncodeFailure(t *testing.T) {
	got, err := JPEGThumbnail(hugeImage{w: 70000, h: 1}, 100000, 100000)

	if err == nil {
		t.Fatal("imagem grande demais para o codificador nao produziu erro")
	}
	if got != nil {
		t.Fatalf("erro de codificacao devolveu %d bytes — buffer parcial vaza para o chamador", len(got))
	}
}
