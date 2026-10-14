package tlspin

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net"
	"time"
)

// FetchLeafCert connects to host:443 and returns its leaf certificate. It
// deliberately skips verification: its ONLY purpose is retrieving a self-signed
// cert for trust-on-first-use pinning. The caller must show the fingerprint to
// the user for out-of-band confirmation, and no secrets are sent here.
func FetchLeafCert(host string) (*x509.Certificate, error) {
	d := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := tls.DialWithDialer(d, "tcp", net.JoinHostPort(host, "443"),
		&tls.Config{InsecureSkipVerify: true}) //nolint:gosec // TOFU retrieval only
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no peer certificate from %s", host)
	}
	return certs[0], nil
}

// Fingerprint returns the SHA-256 of the cert as colon-separated hex.
func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	h := hex.EncodeToString(sum[:])
	out := make([]byte, 0, len(h)+len(h)/2-1)
	for i := 0; i < len(h); i += 2 {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, h[i], h[i+1])
	}
	return string(out)
}

// EncodePEM returns the PEM encoding of a certificate.
func EncodePEM(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}

// ConfigFromPEM builds a *tls.Config trusting ONLY the exact certificate in
// pemBytes. It pins by comparing the server leaf's SHA-256 to the pinned cert
// in VerifyConnection. The default CA chain check is disabled because the pin
// is strictly stronger for a known device and is robust to self-signed certs
// whose SAN omits the IP address.
func ConfigFromPEM(pemBytes []byte, serverName string) (*tls.Config, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("no certificate in pinned PEM")
	}
	pinned, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse pinned cert: %w", err)
	}
	want := sha256.Sum256(pinned.Raw)
	return &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // replaced by exact pin below
		ServerName:         serverName,
		MinVersion:         tls.VersionTLS12,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return fmt.Errorf("no peer certificate")
			}
			if got := sha256.Sum256(cs.PeerCertificates[0].Raw); got != want {
				return fmt.Errorf("certificate pin mismatch for %s", serverName)
			}
			return nil
		},
	}, nil
}
