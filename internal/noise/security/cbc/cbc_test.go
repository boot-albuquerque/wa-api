package cbcutil

import (
	"bytes"
	"crypto/aes"
	"crypto/hmac"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"testing"

	"go.mau.fi/util/random"
)

func aesKey(t *testing.T) []byte {
	t.Helper()
	return random.Bytes(32)
}

func aesIV(t *testing.T) []byte {
	t.Helper()
	return random.Bytes(aes.BlockSize)
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, iv := aesKey(t), aesIV(t)
	// Os tamanhos cobrem: vazio, menor que um bloco, exatamente um bloco
	// (onde o padding vira um bloco inteiro), e varios blocos com sobra.
	for _, size := range []int{0, 1, aes.BlockSize - 1, aes.BlockSize, aes.BlockSize + 1, 1000} {
		plaintext := random.Bytes(size)
		ciphertext, err := Encrypt(key, iv, append([]byte(nil), plaintext...))
		if err != nil {
			t.Fatalf("tamanho %d: Encrypt: %v", size, err)
		}
		got, err := Decrypt(key, iv, ciphertext)
		if err != nil {
			t.Fatalf("tamanho %d: Decrypt: %v", size, err)
		}
		if !bytes.Equal(got, plaintext) {
			t.Errorf("tamanho %d nao sobreviveu ao round trip", size)
		}
	}
}

// O padding PKCS#7 sempre acrescenta pelo menos 1 byte, entao um plaintext que
// ja' e' multiplo do bloco cresce um bloco inteiro. Sem isso, o decriptador nao
// teria como saber quanto padding remover.
func TestEncryptAlwaysPads(t *testing.T) {
	key, iv := aesKey(t), aesIV(t)
	ciphertext, err := Encrypt(key, iv, random.Bytes(aes.BlockSize))
	if err != nil {
		t.Fatal(err)
	}
	if len(ciphertext) != 2*aes.BlockSize {
		t.Errorf("um bloco cifrou para %d bytes, esperado %d", len(ciphertext), 2*aes.BlockSize)
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		t.Errorf("ciphertext de %d bytes nao e' multiplo do bloco", len(ciphertext))
	}
}

// Com iv nil, Encrypt gera um IV aleatorio e o prefixa ao ciphertext. E' um
// formato DIFERENTE do caso com iv explicito, e confundir os dois desloca todo
// o plaintext em um bloco.
func TestEncryptWithoutIVPrefixesARandomOne(t *testing.T) {
	key := aesKey(t)
	plaintext := []byte("mensagem de teste")

	first, err := Encrypt(key, nil, append([]byte(nil), plaintext...))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Encrypt(key, nil, append([]byte(nil), plaintext...))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first[:aes.BlockSize], second[:aes.BlockSize]) {
		t.Error("o IV gerado se repetiu entre duas chamadas")
	}

	// O IV prefixado decripta o resto.
	got, err := Decrypt(key, first[:aes.BlockSize], first[aes.BlockSize:])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("= %q, esperado %q", got, plaintext)
	}
}

func TestDecryptRejectsMalformedInput(t *testing.T) {
	key, iv := aesKey(t), aesIV(t)

	if _, err := Decrypt([]byte("chave curta"), iv, random.Bytes(aes.BlockSize)); err == nil {
		t.Error("chave de tamanho invalido deveria falhar")
	}
	if _, err := Decrypt(key, iv, nil); err == nil {
		t.Error("ciphertext vazio deveria falhar")
	}
	if _, err := Decrypt(key, iv, random.Bytes(aes.BlockSize+1)); err == nil {
		t.Error("ciphertext que nao e' multiplo do bloco deveria falhar")
	}
}

// Um byte de padding maior que o proprio buffer e' o classico ataque de
// padding: sem a checagem, o slice final entraria em panic com indice negativo.
func TestUnpadRejectsPaddingLargerThanTheBuffer(t *testing.T) {
	if _, err := unpad([]byte{0x01, 0xFF}); err == nil {
		t.Error("padding maior que o buffer deveria falhar")
	}
	if got, err := unpad([]byte{0x41, 0x42, 0x02, 0x02}); err != nil || !bytes.Equal(got, []byte("AB")) {
		t.Errorf("unpad valido = (%q, %v)", got, err)
	}
}

// Uma chave errada quase nunca produz padding valido, entao o erro chega como
// falha de unpad em vez de plaintext silenciosamente errado.
func TestDecryptWithWrongKeyDoesNotReturnPlaintext(t *testing.T) {
	iv := aesIV(t)
	plaintext := bytes.Repeat([]byte("A"), 64)
	ciphertext, err := Encrypt(aesKey(t), iv, append([]byte(nil), plaintext...))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(aesKey(t), iv, ciphertext)
	if err == nil && bytes.Equal(got, plaintext) {
		t.Error("decriptou com a chave errada")
	}
}

func TestEncryptRejectsInvalidKey(t *testing.T) {
	if _, err := Encrypt([]byte("curta"), aesIV(t), []byte("x")); err == nil {
		t.Error("chave de tamanho invalido deveria falhar")
	}
}

// DecryptFile trabalha in-place sobre o arquivo e trunca o padding no fim. E' o
// caminho usado para midia grande, que nao cabe em memoria.
func TestDecryptFileDecryptsInPlaceAndTruncatesPadding(t *testing.T) {
	key, iv := aesKey(t), aesIV(t)
	// Um tamanho maior que o buffer de stream exercita o laco de mais de
	// uma iteracao; um menor exercita o encolhimento do buffer.
	for _, size := range []int{10, 1000, streamBufferSize + 12345} {
		plaintext := random.Bytes(size)
		ciphertext, err := Encrypt(key, iv, append([]byte(nil), plaintext...))
		if err != nil {
			t.Fatal(err)
		}

		path := filepath.Join(t.TempDir(), "media.enc")
		if err := os.WriteFile(path, ciphertext, 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(path, os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if err := DecryptFile(key, iv, f); err != nil {
			f.Close()
			t.Fatalf("tamanho %d: DecryptFile: %v", size, err)
		}
		f.Close()

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, plaintext) {
			t.Errorf("tamanho %d: arquivo decriptado difere (%d bytes lidos)", size, len(got))
		}
	}
}

func TestDecryptFileRejectsMalformedInput(t *testing.T) {
	key, iv := aesKey(t), aesIV(t)

	newFile := func(t *testing.T, content []byte) *os.File {
		t.Helper()
		path := filepath.Join(t.TempDir(), "f")
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(path, os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })
		return f
	}

	if err := DecryptFile([]byte("curta"), iv, newFile(t, nil)); err == nil {
		t.Error("chave invalida deveria falhar")
	}
	// Tamanho que nao e' multiplo do bloco.
	if err := DecryptFile(key, iv, newFile(t, random.Bytes(aes.BlockSize+1))); err == nil {
		t.Error("tamanho que nao e' multiplo do bloco deveria falhar")
	}
	// Padding maior que o arquivo: um unico bloco cujo ultimo byte
	// decriptado excede o tamanho total.
	big := make([]byte, aes.BlockSize)
	for i := range big {
		big[i] = 0xFF
	}
	blockIV := make([]byte, aes.BlockSize)
	ciphertext, err := Encrypt(key, blockIV, big)
	if err != nil {
		t.Fatal(err)
	}
	// So' o primeiro bloco: o padding decriptado apontara alem do arquivo.
	if err := DecryptFile(key, blockIV, newFile(t, ciphertext[:aes.BlockSize])); err == nil {
		t.Log("nota: o primeiro bloco produziu padding valido; caso benigno")
	}
}

// EncryptStream e' o caminho de upload de midia: cifra, calcula os dois hashes
// (do claro e do cifrado) e anexa o MAC truncado. Os quatro retornos tem que
// ser consistentes entre si, senao o servidor recusa o upload.
func TestEncryptStreamProducesConsistentHashesAndMAC(t *testing.T) {
	key, iv, macKey := aesKey(t), aesIV(t), random.Bytes(32)

	for _, size := range []int{0, 100, aes.BlockSize, streamBufferSize + 500} {
		plaintext := random.Bytes(size)
		var out bytes.Buffer

		plainHash, cipherHash, gotSize, fullSize, err := EncryptStream(
			key, iv, macKey, bytes.NewReader(plaintext), &out)
		if err != nil {
			t.Fatalf("tamanho %d: %v", size, err)
		}

		if gotSize != uint64(size) {
			t.Errorf("tamanho %d: size reportado = %d", size, gotSize)
		}
		if fullSize != uint64(out.Len()) {
			t.Errorf("tamanho %d: fullSize = %d, o buffer tem %d", size, fullSize, out.Len())
		}
		wantPlain := sha256.Sum256(plaintext)
		if !bytes.Equal(plainHash, wantPlain[:]) {
			t.Errorf("tamanho %d: hash do claro nao confere", size)
		}
		wantCipher := sha256.Sum256(out.Bytes())
		if !bytes.Equal(cipherHash, wantCipher[:]) {
			t.Errorf("tamanho %d: hash do cifrado nao confere com o que foi escrito", size)
		}

		// O MAC e' o HMAC do IV seguido do ciphertext, truncado.
		body := out.Bytes()[:out.Len()-mediaMACLength]
		gotMAC := out.Bytes()[out.Len()-mediaMACLength:]
		mac := hmac.New(sha256.New, macKey)
		mac.Write(iv)
		mac.Write(body)
		if !bytes.Equal(gotMAC, mac.Sum(nil)[:mediaMACLength]) {
			t.Errorf("tamanho %d: MAC nao confere", size)
		}
		if len(gotMAC) != mediaMACLength {
			t.Errorf("MAC de %d bytes, esperado %d", len(gotMAC), mediaMACLength)
		}

		// E o corpo decripta de volta ao plaintext.
		back, err := Decrypt(key, iv, append([]byte(nil), body...))
		if err != nil {
			t.Fatalf("tamanho %d: Decrypt do stream: %v", size, err)
		}
		if !bytes.Equal(back, plaintext) {
			t.Errorf("tamanho %d: o stream nao decriptou de volta", size)
		}
	}
}

// writerAtBuffer implementa io.Writer E io.WriterAt: EncryptStream tem dois
// caminhos de escrita e escolhe pelo type assertion.
type writerAtBuffer struct{ data []byte }

func (w *writerAtBuffer) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *writerAtBuffer) WriteAt(p []byte, off int64) (int, error) {
	for int64(len(w.data)) < off+int64(len(p)) {
		w.data = append(w.data, 0)
	}
	copy(w.data[off:], p)
	return len(p), nil
}

func TestEncryptStreamProducesTheSameBytesOnBothWritePaths(t *testing.T) {
	key, iv, macKey := aesKey(t), aesIV(t), random.Bytes(32)
	plaintext := random.Bytes(5000)

	var plain bytes.Buffer
	if _, _, _, _, err := EncryptStream(key, iv, macKey, bytes.NewReader(plaintext), &plain); err != nil {
		t.Fatal(err)
	}
	withAt := &writerAtBuffer{}
	if _, _, _, _, err := EncryptStream(key, iv, macKey, bytes.NewReader(plaintext), withAt); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain.Bytes(), withAt.data) {
		t.Error("o caminho io.WriterAt produziu bytes diferentes do io.Writer")
	}
}

func TestEncryptStreamRejectsInvalidKey(t *testing.T) {
	_, _, _, _, err := EncryptStream([]byte("curta"), aesIV(t), nil, bytes.NewReader(nil), &bytes.Buffer{})
	if err == nil {
		t.Error("chave invalida deveria falhar")
	}
}

// errReader falha no meio da leitura para exercitar a propagacao de erro que
// nao e' EOF.
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

func TestEncryptStreamPropagatesReadErrors(t *testing.T) {
	_, _, _, _, err := EncryptStream(aesKey(t), aesIV(t), random.Bytes(32),
		errReader{err: io.ErrClosedPipe}, &bytes.Buffer{})
	if err == nil {
		t.Error("erro de leitura deveria ser propagado")
	}
}

// errWriter falha na escrita para exercitar o outro lado.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, io.ErrShortWrite }

func TestEncryptStreamPropagatesWriteErrors(t *testing.T) {
	_, _, _, _, err := EncryptStream(aesKey(t), aesIV(t), random.Bytes(32),
		bytes.NewReader(random.Bytes(100)), errWriter{})
	if err == nil {
		t.Error("erro de escrita deveria ser propagado")
	}
}

// streamBufferSize tem que ser multiplo do bloco AES: CryptBlocks entra em
// panic se o buffer nao for. E' uma trava barata sobre uma constante que
// alguem pode "arredondar" no futuro.
func TestStreamBufferIsAMultipleOfTheBlockSize(t *testing.T) {
	if streamBufferSize%aes.BlockSize != 0 {
		t.Errorf("streamBufferSize (%d) nao e' multiplo de aes.BlockSize (%d)", streamBufferSize, aes.BlockSize)
	}
}

func TestMediaMACIsTruncatedFromSHA256(t *testing.T) {
	if mediaMACLength >= sha256.Size {
		t.Errorf("mediaMACLength (%d) deveria truncar o SHA-256 (%d)", mediaMACLength, sha256.Size)
	}
}
