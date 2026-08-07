package keys

import (
	"bytes"
	"testing"

	"go.mau.fi/libsignal/ecc"
	"golang.org/x/crypto/curve25519"
)

// O clamping de Curve25519 nao e' cosmetico: uma chave privada sem ele cai
// fora da faixa que a curva exige e produz um segredo compartilhado que o
// outro lado nao reproduz. Este teste vale para QUALQUER chave gerada.
func TestNewKeyPairClampsThePrivateKey(t *testing.T) {
	for i := 0; i < 200; i++ {
		kp := NewKeyPair()
		if got := kp.Priv[clampFirstByte] & 0b0000_0111; got != 0 {
			t.Fatalf("os 3 bits baixos nao foram zerados: %#b", kp.Priv[clampFirstByte])
		}
		if got := kp.Priv[clampLastByte] & 0b1000_0000; got != 0 {
			t.Fatalf("o bit mais alto nao foi zerado: %#b", kp.Priv[clampLastByte])
		}
		if got := kp.Priv[clampLastByte] & 0b0100_0000; got == 0 {
			t.Fatalf("o penultimo bit nao foi ligado: %#b", kp.Priv[clampLastByte])
		}
	}
}

// A publica tem que ser a base multiplicada pela privada. Se a derivacao
// mudasse, o handshake Noise falharia de um jeito dificil de diagnosticar.
func TestPublicKeyIsDerivedFromPrivate(t *testing.T) {
	kp := NewKeyPair()
	var want [KeyLength]byte
	curve25519.ScalarBaseMult(&want, kp.Priv)
	if !bytes.Equal(kp.Pub[:], want[:]) {
		t.Errorf("publica = %x, esperado %x", kp.Pub[:], want[:])
	}
}

func TestNewKeyPairFromPrivateKeyIsDeterministic(t *testing.T) {
	seed := NewKeyPair().Priv
	first := NewKeyPairFromPrivateKey(*seed)
	second := NewKeyPairFromPrivateKey(*seed)
	if !bytes.Equal(first.Pub[:], second.Pub[:]) {
		t.Error("a mesma privada produziu publicas diferentes")
	}
	if !bytes.Equal(first.Priv[:], seed[:]) {
		t.Error("a privada foi alterada")
	}
}

// NewKeyPairFromPrivateKey copia a chave: guarda um ponteiro para o parametro,
// que e' uma copia por valor. Sem isso, dois keypairs criados a partir do mesmo
// array compartilhariam a memoria da chave privada.
func TestNewKeyPairFromPrivateKeyCopiesTheKey(t *testing.T) {
	var seed [KeyLength]byte
	seed[0] = 1
	kp := NewKeyPairFromPrivateKey(seed)
	seed[0] = 2
	if kp.Priv[0] != 1 {
		t.Error("alterar o array original mudou a chave do keypair")
	}
}

func TestNewKeyPairProducesDistinctKeys(t *testing.T) {
	seen := make(map[[KeyLength]byte]struct{}, 100)
	for i := 0; i < 100; i++ {
		kp := NewKeyPair()
		if _, dup := seen[*kp.Priv]; dup {
			t.Fatal("NewKeyPair repetiu uma chave privada")
		}
		seen[*kp.Priv] = struct{}{}
	}
}

// A assinatura e' sobre a chave publica PRECEDIDA do byte de tipo de curva.
// Assinar a chave crua produziria uma assinatura que nenhum outro cliente
// valida, e o erro so' apareceria no servidor recusando a prekey.
func TestSignPrefixesTheKeyWithItsCurveType(t *testing.T) {
	identity := NewKeyPair()
	target := NewKeyPair()

	signature := identity.Sign(target)

	signed := make([]byte, signedKeyLength)
	signed[0] = ecc.DjbType
	copy(signed[keyTypePrefixLength:], target.Pub[:])

	if !ecc.VerifySignature(ecc.NewDjbECPublicKey(*identity.Pub), signed, *signature) {
		t.Error("a assinatura nao valida sobre a chave com prefixo de tipo")
	}
	// E nao valida sobre a chave crua, sem o prefixo.
	if ecc.VerifySignature(ecc.NewDjbECPublicKey(*identity.Pub), target.Pub[:], *signature) {
		t.Error("a assinatura validou sobre a chave sem prefixo — o prefixo deixou de ser assinado")
	}
}

func TestSignatureDoesNotValidateForAnotherKey(t *testing.T) {
	identity := NewKeyPair()
	target := NewKeyPair()
	other := NewKeyPair()

	signature := identity.Sign(target)

	signed := make([]byte, signedKeyLength)
	signed[0] = ecc.DjbType
	copy(signed[keyTypePrefixLength:], other.Pub[:])

	if ecc.VerifySignature(ecc.NewDjbECPublicKey(*identity.Pub), signed, *signature) {
		t.Error("a assinatura de uma chave validou para outra")
	}
}

func TestNewPreKeyCarriesItsID(t *testing.T) {
	pk := NewPreKey(42)
	if pk.KeyID != 42 {
		t.Errorf("KeyID = %d, esperado 42", pk.KeyID)
	}
	if pk.Pub == nil || pk.Priv == nil {
		t.Error("a prekey veio sem par de chaves")
	}
	if pk.Signature != nil {
		t.Error("NewPreKey nao deveria assinar — quem assina e' CreateSignedPreKey")
	}
}

// CreateSignedPreKey e' o que o device faz no registro. A assinatura tem que
// ser da chave de identidade sobre a prekey nova.
func TestCreateSignedPreKeySignsWithTheIdentityKey(t *testing.T) {
	identity := NewKeyPair()
	signed := identity.CreateSignedPreKey(7)

	if signed.KeyID != 7 {
		t.Errorf("KeyID = %d, esperado 7", signed.KeyID)
	}
	if signed.Signature == nil {
		t.Fatal("a prekey assinada veio sem assinatura")
	}
	if len(signed.Signature) != SignatureLength {
		t.Errorf("assinatura de %d bytes, esperado %d", len(signed.Signature), SignatureLength)
	}
	if bytes.Equal(signed.Priv[:], identity.Priv[:]) {
		t.Error("a prekey reusou a chave de identidade")
	}

	toVerify := make([]byte, signedKeyLength)
	toVerify[0] = ecc.DjbType
	copy(toVerify[keyTypePrefixLength:], signed.Pub[:])
	if !ecc.VerifySignature(ecc.NewDjbECPublicKey(*identity.Pub), toVerify, *signed.Signature) {
		t.Error("a assinatura da prekey nao valida contra a chave de identidade")
	}
}

// Dois lados derivando o mesmo segredo e' o que o handshake Noise depende.
func TestKeyPairsAgreeOnASharedSecret(t *testing.T) {
	alice, bob := NewKeyPair(), NewKeyPair()

	fromAlice, err := curve25519.X25519(alice.Priv[:], bob.Pub[:])
	if err != nil {
		t.Fatal(err)
	}
	fromBob, err := curve25519.X25519(bob.Priv[:], alice.Pub[:])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fromAlice, fromBob) {
		t.Error("os dois lados derivaram segredos diferentes")
	}
}
