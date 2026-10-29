// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package crypto

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
)

type RSACryptor struct {
	key *rsa.PrivateKey
}

func NewRSACryptor(keyFile string) (*RSACryptor, error) {
	pkcs1PemPrivateKey, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}
	return NewRSACryptorByKeyData(pkcs1PemPrivateKey)
}

// NewRSACryptorFromKey wraps an already-parsed private key. Tests use it.
func NewRSACryptorFromKey(key *rsa.PrivateKey) *RSACryptor {
	return &RSACryptor{key: key}
}

func NewRSACryptorByKeyData(pkcs1PemPrivateKey []byte) (*RSACryptor, error) {
	block, _ := pem.Decode(pkcs1PemPrivateKey)
	if block == nil {
		return nil, fmt.Errorf("invalid pemsKeyData")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return &RSACryptor{key}, nil
}

func (m *RSACryptor) modulusSize() int {
	if m == nil || m.key == nil || m.key.N == nil {
		return 256
	}
	n := (m.key.N.BitLen() + 7) / 8
	if n <= 0 {
		return 256
	}
	return n
}

func leftPad(b []byte, size int) []byte {
	if len(b) >= size {
		return append([]byte(nil), b...)
	}
	out := make([]byte, size)
	copy(out[size-len(b):], b)
	return out
}

func (m *RSACryptor) Encrypt(b []byte) []byte {
	c := new(big.Int)
	c.Exp(new(big.Int).SetBytes(b), big.NewInt(int64(m.key.E)), m.key.N)
	// Big-endian, left-padded so a 2048-bit ciphertext is exactly 256 bytes.
	// copy-to-front dropped the leading zeros and shifted the integer.
	return leftPad(c.Bytes(), m.modulusSize())
}

func (m *RSACryptor) Decrypt(b []byte) []byte {
	c := new(big.Int)
	c.Exp(new(big.Int).SetBytes(b), m.key.D, m.key.N)
	// big.Int.Bytes strips leading zeros. RSA_PAD plaintext is a 256-byte
	// integer that often starts with 0x00; callers that require len==256
	// must use DecryptBlock.
	return c.Bytes()
}

// DecryptBlock is Decrypt left-padded to the modulus size (256 for a 2048-bit key).
func (m *RSACryptor) DecryptBlock(b []byte) []byte {
	return leftPad(m.Decrypt(b), m.modulusSize())
}
