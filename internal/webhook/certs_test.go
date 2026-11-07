package webhook

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func TestLoadCertsReadsMountedMaterial(t *testing.T) {
	dir := t.TempDir()
	written, err := EnsureCertsFor(dir, []string{"caretaker-webhook.caretaker-system.svc"}, false)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCerts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written.CACert, loaded.CACert) || !bytes.Equal(written.ServerCert, loaded.ServerCert) {
		t.Fatal("loaded certificates differ from the written ones")
	}
	if loaded.CertFile == "" || loaded.KeyFile == "" {
		t.Fatal("loaded certificate paths are empty")
	}
	if _, err := LoadCerts(t.TempDir()); err == nil {
		t.Fatal("expected an error when the certificate directory is empty")
	}
}

func TestEnsureCertsForCoversServiceSANs(t *testing.T) {
	dir := t.TempDir()
	hosts := []string{"caretaker-webhook.caretaker-system.svc", "caretaker-webhook.caretaker-system.svc.cluster.local"}
	certs, err := EnsureCertsFor(dir, hosts, false)
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
	for _, host := range hosts {
		found := false
		for _, name := range cert.DNSNames {
			if name == host {
				found = true
			}
		}
		if !found {
			t.Fatalf("DNS names = %v, want %s", cert.DNSNames, host)
		}
	}

	// A certificate that does not cover a requested host is regenerated.
	again, err := EnsureCertsFor(dir, []string{"caretaker-webhook.other.svc"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(certs.CACert, again.CACert) {
		t.Fatal("certificate should have been regenerated for a new host")
	}
}
