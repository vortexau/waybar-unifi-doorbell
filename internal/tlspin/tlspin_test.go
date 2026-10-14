package tlspin

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

func makeCert(t *testing.T, cn string) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestConfigFromPEMAcceptsPinned(t *testing.T) {
	cert := makeCert(t, "udm")
	cfg, err := ConfigFromPEM(EncodePEM(cert), "192.168.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.VerifyConnection(tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cert},
	}); err != nil {
		t.Fatalf("pinned cert should verify: %v", err)
	}
}

func TestConfigFromPEMRejectsOther(t *testing.T) {
	cfg, err := ConfigFromPEM(EncodePEM(makeCert(t, "udm")), "192.168.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.VerifyConnection(tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{makeCert(t, "evil")},
	}); err == nil {
		t.Fatal("different cert should be rejected")
	}
}

func TestFingerprintStable(t *testing.T) {
	cert := makeCert(t, "udm")
	if Fingerprint(cert) != Fingerprint(cert) {
		t.Fatal("fingerprint must be deterministic")
	}
	if len(Fingerprint(cert)) != 95 { // 32 bytes -> 64 hex + 31 colons
		t.Fatalf("unexpected fingerprint length: %q", Fingerprint(cert))
	}
}
