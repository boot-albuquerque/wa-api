package msgpad

import (
	"bytes"
	"sync"
	"testing"
)

func TestPadThenUnpadReturnsTheOriginalPlaintext(t *testing.T) {
	for _, plaintext := range [][]byte{nil, {}, []byte("a"), bytes.Repeat([]byte("x"), 1024)} {
		padded := Pad(append([]byte(nil), plaintext...))
		got, err := Unpad(padded, 2)
		if err != nil {
			t.Fatalf("Unpad(%d bytes): %v", len(plaintext), err)
		}
		if !bytes.Equal(got, plaintext) && !(len(got) == 0 && len(plaintext) == 0) {
			t.Errorf("= %q, esperado %q", got, plaintext)
		}
	}
}

// O padding tem que ficar entre 1 e 15 bytes: e' o que o outro lado espera ao
// ler o ultimo byte como comprimento.
func TestPadAppendsBetweenOneAndFifteenBytes(t *testing.T) {
	for i := 0; i < 200; i++ {
		padded := Pad(nil)
		if len(padded) < 1 || len(padded) > 15 {
			t.Fatalf("padding de %d bytes fora de [1,15]", len(padded))
		}
		last := padded[len(padded)-1]
		if int(last) != len(padded) {
			t.Fatalf("ultimo byte %d nao bate com o comprimento %d", last, len(padded))
		}
	}
}

// Versao 3 nao usa padding: o payload tem que voltar intacto.
func TestUnpadLeavesVersionThreeUntouched(t *testing.T) {
	plaintext := []byte{1, 2, 3}
	got, err := Unpad(plaintext, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("= %q, esperado %q", got, plaintext)
	}
}

func TestUnpadRejectsEmptyPlaintext(t *testing.T) {
	if _, err := Unpad(nil, 2); err == nil {
		t.Error("esperado erro para plaintext vazio")
	}
}

func TestUnpadRejectsWrongPadding(t *testing.T) {
	// Ultimo byte diz 4 bytes de padding, mas os anteriores nao sao 4.
	if _, err := Unpad([]byte{0, 0, 0, 4}, 2); err == nil {
		t.Error("esperado erro para padding invalido")
	}
}

// Pad tira bytes de um gerador aleatorio global e e' chamada de varias
// goroutines no pipeline de envio; com -race isto pega qualquer estado
// compartilhado que escape.
func TestPadIsSafeUnderConcurrentUse(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				plaintext := []byte("mensagem")
				got, err := Unpad(Pad(append([]byte(nil), plaintext...)), 2)
				if err != nil {
					t.Errorf("Unpad: %v", err)
					return
				}
				if !bytes.Equal(got, plaintext) {
					t.Errorf("= %q, esperado %q", got, plaintext)
					return
				}
			}
		}()
	}
	wg.Wait()
}
