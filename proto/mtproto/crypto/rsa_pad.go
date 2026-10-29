package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"io"
	"math/big"
)

const (
	rsaPadDataLen   = 192
	rsaPadHashLen   = 32
	rsaPadKeyLen    = 32
	rsaPadBlockLen  = 256
	rsaPadPlaintext = rsaPadDataLen + rsaPadHashLen // 224
)

// rsaPadZeroIV is the 32-byte IV AES-256-IGE uses for RSA_PAD (two zero blocks).
var rsaPadZeroIV = make([]byte, 32)

// DecryptPQInner decrypts req_DH_params encrypted_data.
// Modern clients send RSA_PAD (exactly 256 bytes). The RSA plaintext is
// left-padded to the modulus size: big.Int.Bytes drops a leading 0x00, which
// used to fail the len==256 check ("need len(encryptedPQInnerData) < 256").
// A SHA1-padded payload from older clients is accepted when the RSA_PAD
// SHA256 check does not match.
func DecryptPQInner(key *RSACryptor, encrypted []byte) ([]byte, error) {
	if key == nil || key.key == nil {
		return nil, fmt.Errorf("rsa_pad: missing key")
	}
	size := key.modulusSize()
	if len(encrypted) != size {
		return nil, fmt.Errorf("rsa_pad: encrypted_data length %d, want %d", len(encrypted), size)
	}
	raw := key.Decrypt(encrypted)
	block := leftPad(raw, size)
	if data, err := decryptRSAPadBlock(block); err == nil {
		return data, nil
	}
	if data, err := decryptLegacySHA1(raw); err == nil {
		return data, nil
	}
	return nil, fmt.Errorf("rsa_pad: sha256 mismatch")
}

func decryptRSAPadBlock(block []byte) ([]byte, error) {
	if len(block) < rsaPadBlockLen {
		return nil, fmt.Errorf("rsa_pad: plaintext length %d", len(block))
	}
	if len(block) > rsaPadBlockLen {
		block = block[len(block)-rsaPadBlockLen:]
	}
	tempKeyXOR := block[:rsaPadKeyLen]
	aesEncrypted := block[rsaPadKeyLen:]
	sum := sha256.Sum256(aesEncrypted)
	tempKey := make([]byte, rsaPadKeyLen)
	for i := 0; i < rsaPadKeyLen; i++ {
		tempKey[i] = tempKeyXOR[i] ^ sum[i]
	}
	ige := NewAES256IGECryptor(tempKey, rsaPadZeroIV)
	if ige == nil {
		return nil, fmt.Errorf("rsa_pad: aes key")
	}
	plain, err := ige.Decrypt(aesEncrypted)
	if err != nil {
		return nil, err
	}
	if len(plain) != rsaPadPlaintext {
		return nil, fmt.Errorf("rsa_pad: aes length %d", len(plain))
	}
	dataWithPadding := reverseCopy(plain[:rsaPadDataLen])
	mac := sha256.Sum256(append(append([]byte{}, tempKey...), dataWithPadding...))
	if !bytes.Equal(mac[:], plain[rsaPadDataLen:]) {
		return nil, fmt.Errorf("rsa_pad: sha256 mismatch")
	}
	return dataWithPadding, nil
}

func decryptLegacySHA1(raw []byte) ([]byte, error) {
	const digest = 20
	if len(raw) <= digest {
		return nil, fmt.Errorf("legacy pq: short")
	}
	for pad := 0; pad < 16 && digest < len(raw)-pad; pad++ {
		body := raw[digest : len(raw)-pad]
		sum := sha1.Sum(body)
		if bytes.Equal(sum[:], raw[:digest]) {
			return append([]byte(nil), body...), nil
		}
	}
	return nil, fmt.Errorf("legacy pq: sha1 mismatch")
}

func reverseCopy(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[len(b)-1-i] = b[i]
	}
	return out
}

// EncryptRSAPad is the client-side RSA_PAD from core.telegram.org/mtproto/auth_key.
// data must be at most 144 bytes. Tests use it to build a 256-byte encrypted_data
// whose plaintext starts with 0x00 often enough to drop a byte from big.Int.Bytes.
func EncryptRSAPad(pub *rsa.PublicKey, data []byte, r io.Reader) ([]byte, error) {
	if pub == nil || pub.N == nil {
		return nil, fmt.Errorf("rsa_pad: missing public key")
	}
	if len(data) > 144 {
		return nil, fmt.Errorf("rsa_pad: data length %d > 144", len(data))
	}
	if r == nil {
		r = rand.Reader
	}
	padded := make([]byte, rsaPadDataLen)
	copy(padded, data)
	if _, err := io.ReadFull(r, padded[len(data):]); err != nil {
		return nil, err
	}
	reversed := reverseCopy(padded)
	size := (pub.N.BitLen() + 7) / 8
	for attempt := 0; attempt < 16; attempt++ {
		tempKey := make([]byte, rsaPadKeyLen)
		if _, err := io.ReadFull(r, tempKey); err != nil {
			return nil, err
		}
		mac := sha256.Sum256(append(append([]byte{}, tempKey...), padded...))
		withHash := make([]byte, 0, rsaPadPlaintext)
		withHash = append(withHash, reversed...)
		withHash = append(withHash, mac[:]...)
		ige := NewAES256IGECryptor(tempKey, rsaPadZeroIV)
		if ige == nil {
			return nil, fmt.Errorf("rsa_pad: aes key")
		}
		aesEnc, err := ige.Encrypt(withHash)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(aesEnc)
		tempKeyXOR := make([]byte, rsaPadKeyLen)
		for i := 0; i < rsaPadKeyLen; i++ {
			tempKeyXOR[i] = tempKey[i] ^ sum[i]
		}
		keyAES := make([]byte, 0, rsaPadBlockLen)
		keyAES = append(keyAES, tempKeyXOR...)
		keyAES = append(keyAES, aesEnc...)
		if new(big.Int).SetBytes(keyAES).Cmp(pub.N) >= 0 {
			continue
		}
		c := new(big.Int).Exp(new(big.Int).SetBytes(keyAES), big.NewInt(int64(pub.E)), pub.N)
		return leftPad(c.Bytes(), size), nil
	}
	return nil, fmt.Errorf("rsa_pad: modulus retries exhausted")
}
