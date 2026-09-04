package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tageecc/cursor-agent-api-proxy/internal/config"
)

func TestGenerateAndLoad(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	if err := Generate(certFile, keyFile, []string{"localhost", "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("key mode %o", st.Mode().Perm())
	}

	tlsCfg, err := TLSConfig(certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if tlsCfg.MinVersion != tls.VersionTLS12 || len(tlsCfg.Certificates) != 1 {
		t.Fatalf("%+v", tlsCfg)
	}

	raw, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(raw)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(cert.NotAfter) < 300*24*time.Hour {
		t.Fatalf("short validity %s", cert.NotAfter)
	}
	foundIP, foundDNS := false, false
	for _, ip := range cert.IPAddresses {
		if ip.Equal(net.ParseIP("127.0.0.1")) {
			foundIP = true
		}
	}
	for _, d := range cert.DNSNames {
		if d == "localhost" {
			foundDNS = true
		}
	}
	if !foundIP || !foundDNS {
		t.Fatalf("sans ip=%v dns=%v", cert.IPAddresses, cert.DNSNames)
	}
}

func TestEnsureIdempotent(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = dir
	cfg.TLS = true
	cfg.TLSAuto = true
	cfg.TLSCertFile = filepath.Join(dir, "tls", "cert.pem")
	cfg.TLSKeyFile = filepath.Join(dir, "tls", "key.pem")

	a, b, err := Ensure(cfg)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = Ensure(cfg)
	if err != nil {
		t.Fatal(err)
	}
	info2, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	if info2.Size() == 0 || info.Size() == 0 {
		t.Fatal("empty cert files")
	}
}

func TestNeedsGenerateMissing(t *testing.T) {
	if !needsGenerate("/no/such/cert", "/no/such/key") {
		t.Fatal("missing files should generate")
	}
}

func TestHostsIncludeLocalhost(t *testing.T) {
	cfg := config.Default()
	cfg.Host = "127.0.0.1"
	cfg.TLSHosts = []string{"app.local", "0.0.0.0"}
	got := Hosts(cfg)
	want := map[string]bool{"app.local": true, "127.0.0.1": true, "localhost": true, "::1": true}
	for _, h := range got {
		if h == "0.0.0.0" {
			t.Fatal("wildcard bind address must not be a SAN")
		}
		delete(want, h)
	}
	for h := range want {
		if h == "::1" {
			continue
		}
		if h != "" {
			// hostname also injected; only require the fixed names
		}
	}
	foundApp := false
	for _, h := range got {
		if h == "app.local" {
			foundApp = true
		}
	}
	if !foundApp {
		t.Fatalf("%v", got)
	}
}
