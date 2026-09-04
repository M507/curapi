// Package tlsutil issues and loads local TLS certificates.
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/tageecc/cursor-agent-api-proxy/internal/config"
)

const validity = 365 * 24 * time.Hour
const renewIfLeft = 30 * 24 * time.Hour

func Ensure(cfg config.Config) (certFile, keyFile string, err error) {
	certFile = cfg.TLSCertFile
	keyFile = cfg.TLSKeyFile
	if !cfg.TLSAuto {
		if _, err := os.Stat(certFile); err != nil {
			return "", "", fmt.Errorf("tls cert: %w", err)
		}
		if _, err := os.Stat(keyFile); err != nil {
			return "", "", fmt.Errorf("tls key: %w", err)
		}
		return certFile, keyFile, nil
	}
	if needsGenerate(certFile, keyFile) {
		if err := Generate(certFile, keyFile, Hosts(cfg)); err != nil {
			return "", "", err
		}
	}
	return certFile, keyFile, nil
}

func TLSConfig(certFile, keyFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load tls key pair: %w", err)
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}, nil
}

func Hosts(cfg config.Config) []string {
	hosts := append([]string{}, cfg.TLSHosts...)
	hosts = append(hosts, cfg.PublicHost(), "localhost", "127.0.0.1", "::1")
	if name, err := os.Hostname(); err == nil && name != "" {
		hosts = append(hosts, name)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if h == "" || h == "0.0.0.0" || h == "::" || h == "[::]" {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	return out
}

func Generate(certFile, keyFile string, hosts []string) error {
	if err := os.MkdirAll(filepath.Dir(certFile), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(keyFile), 0o700); err != nil {
		return err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate tls key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate tls serial: %w", err)
	}

	now := time.Now().Add(-time.Minute)
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{config.AppName},
			CommonName:   config.AppName,
		},
		NotBefore:             now,
		NotAfter:              now.Add(validity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
			continue
		}
		tmpl.DNSNames = append(tmpl.DNSNames, h)
	}
	if len(tmpl.DNSNames) == 0 && len(tmpl.IPAddresses) == 0 {
		tmpl.DNSNames = []string{"localhost"}
		tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create tls cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal tls key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := writeSecret(certFile, certPEM, 0o600); err != nil {
		return err
	}
	if err := writeSecret(keyFile, keyPEM, 0o600); err != nil {
		return err
	}
	return nil
}

func needsGenerate(certFile, keyFile string) bool {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return true
	}
	if _, err := os.Stat(keyFile); err != nil {
		return true
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return true
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return true
	}
	return time.Now().Add(renewIfLeft).After(cert.NotAfter)
}

func writeSecret(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Chmod(path, mode)
}
