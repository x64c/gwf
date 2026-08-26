package security

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BuildJWKSFile loads all *_public.pem files in publicKeyDir into a JWKS and
// atomically writes the result to outputPath as JSON (via a temp file + rename).
func BuildJWKSFile(publicKeyDir, outputPath string) error {
	jwks, err := LoadPublicPEMKeysAsJWKS(publicKeyDir)
	if err != nil {
		return fmt.Errorf("failed to load public keys: %v", err)
	}
	tmpPath := outputPath + ".tmp"
	if err = jwks.CreateJSONFile(tmpPath); err != nil {
		return fmt.Errorf("failed to write %s: %v", tmpPath, err)
	}
	if err = os.Rename(tmpPath, outputPath); err != nil {
		return fmt.Errorf("failed to rename to %s: %v", outputPath, err)
	}
	return nil
}

// DeleteOldRSAKeys removes all *_private.pem files in privateKeyDir and all
// *_public.pem files in publicKeyDir whose filenames don't contain activeKid.
// Files matching the active kid are preserved.
func DeleteOldRSAKeys(privateKeyDir, publicKeyDir, activeKid string) error {
	entries, err := os.ReadDir(privateKeyDir)
	if err != nil {
		return fmt.Errorf("failed to read private key dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "_private.pem") && !strings.Contains(e.Name(), activeKid) {
			_ = os.Remove(filepath.Join(privateKeyDir, e.Name()))
		}
	}
	entries, err = os.ReadDir(publicKeyDir)
	if err != nil {
		return fmt.Errorf("failed to read public key dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "_public.pem") && !strings.Contains(e.Name(), activeKid) {
			_ = os.Remove(filepath.Join(publicKeyDir, e.Name()))
		}
	}
	return nil
}

// FindLatestKidByMtime scans privateKeyDir for *_private.pem files and returns
// the kid (filename minus the _private.pem suffix) of the file with the latest
// modification time. Returns an error if no matching files are found.
func FindLatestKidByMtime(privateKeyDir string) (string, error) {
	entries, err := os.ReadDir(privateKeyDir)
	if err != nil {
		return "", fmt.Errorf("failed to read dir %q: %v", privateKeyDir, err)
	}
	const suffix = "_private.pem"
	var (
		latestKid   string
		latestMtime time.Time
	)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, suffix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if latestKid == "" || info.ModTime().After(latestMtime) {
			latestMtime = info.ModTime()
			latestKid = strings.TrimSuffix(name, suffix)
		}
	}
	if latestKid == "" {
		return "", fmt.Errorf("no private key files found in %q", privateKeyDir)
	}
	return latestKid, nil
}

// PrivateKeyPath is the path of kid's private key under privateKeyDir:
// <privateKeyDir>/<kid>_private.pem — the naming GenerateAndSaveRSAKey writes.
func PrivateKeyPath(privateKeyDir, kid string) string {
	return filepath.Join(privateKeyDir, kid+"_private.pem")
}

// GenerateAndSaveRSAKey generates a new 2048-bit RSA key pair, computes a
// short kid from the public key, and saves both PEM files at
// <privateKeyDir>/<kid>_private.pem and <publicKeyDir>/<kid>_public.pem.
// Returns the kid and the new public key.
func GenerateAndSaveRSAKey(privateKeyDir, publicKeyDir string) (string, *rsa.PublicKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate RSA key pair: %v", err)
	}
	publicKey := &privateKey.PublicKey
	kid, err := GenerateKeyID(publicKey, 8)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate a key id: %v", err)
	}
	privPath := PrivateKeyPath(privateKeyDir, kid)
	if err = SavePrivatePEMKeyLocal(privPath, privateKey); err != nil {
		return "", nil, fmt.Errorf("failed to save private key: %v", err)
	}
	pubPath := filepath.Join(publicKeyDir, kid+"_public.pem")
	if err = SavePublicPEMKeyLocal(pubPath, publicKey); err != nil {
		return "", nil, fmt.Errorf("failed to save public key: %v", err)
	}
	return kid, publicKey, nil
}
