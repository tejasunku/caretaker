package webhook

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"testing"
)

func TestEnsureCertsReusesAndHasHostSAN(t *testing.T) {
	dir := t.TempDir()
	certs, err := EnsureCerts(dir, "10.89.2.1", false)
	if err != nil {
		t.Fatal(err)
	}

	block, _ := pem.Decode(certs.ServerCert)
	if block == nil {
		t.Fatal("server certificate is not PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if !hasIP(cert, "10.89.2.1") {
		t.Fatalf("server certificate SANs = %v, want 10.89.2.1", cert.IPAddresses)
	}
	if _, err := tls.X509KeyPair(certs.ServerCert, certs.ServerKey); err != nil {
		t.Fatalf("key pair: %v", err)
	}

	again, err := EnsureCerts(dir, "10.89.2.1", false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(certs.CACert, again.CACert) {
		t.Fatal("existing certificates should be reused")
	}
}

func hasIP(cert *x509.Certificate, want string) bool {
	target := net.ParseIP(want)
	for _, ip := range cert.IPAddresses {
		if ip.Equal(target) {
			return true
		}
	}
	return false
}
