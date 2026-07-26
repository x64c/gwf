package security

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
)

func SavePrivatePEMKeyLocal(filePath string, privateKey *rsa.PrivateKey) error {
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	return os.WriteFile(filePath, pem.EncodeToMemory(pemBlock), 0600)
}

func SavePublicPEMKeyLocal(filePath string, publicKey *rsa.PublicKey) error {
	bytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return err
	}
	pemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: bytes,
	}
	return os.WriteFile(filePath, pem.EncodeToMemory(pemBlock), 0644)
}

func LoadLocalPrivatePEMKey(filePath string) (*rsa.PrivateKey, error) {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	pemBlock, _ := pem.Decode(bytes)
	if pemBlock == nil {
		return nil, fmt.Errorf("no PEM block found in %q", filePath)
	}
	return x509.ParsePKCS1PrivateKey(pemBlock.Bytes)
}

func LoadLocalPublicPEMKey(filePath string) (*rsa.PublicKey, error) {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	pemBlock, _ := pem.Decode(bytes)
	if pemBlock == nil {
		return nil, fmt.Errorf("no PEM block found in %q", filePath)
	}
	publicKey, err := x509.ParsePKIXPublicKey(pemBlock.Bytes)
	if err != nil {
		return nil, err
	}
	rsaPub, ok := publicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("PEM in %q is not an RSA public key (got %T)", filePath, publicKey)
	}
	return rsaPub, nil
}

func GenerateKeyID(pub *rsa.PublicKey, length int) (string, error) {
	if length < 8 || length > 32 {
		return "", errors.New("8 <= length <= 32")
	}
	n := pub.N.Bytes()
	e := big.NewInt(int64(pub.E)).Bytes()
	h := sha256.Sum256(append(n, e...))
	return hex.EncodeToString(h[:length]), nil
}
