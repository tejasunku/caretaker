package webhook

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
)

// Certs holds the PEM material and the on-disk key pair paths.
type Certs struct {
	CACert     []byte
	ServerCert []byte
	ServerKey  []byte
	CertFile   string
	KeyFile    string
}

// EnsureCerts loads or creates a CA and server certificate for host. The
// server certificate includes host and localhost as SANs.
func EnsureCerts(dir, host string, force bool) (*Certs, error) {
	caPath := filepath.Join(dir, "ca.crt")
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")

	if !force {
		ca, caErr := os.ReadFile(caPath)
		cert, certErr := os.ReadFile(certPath)
		key, keyErr := os.ReadFile(keyPath)
		if caErr == nil && certErr == nil && keyErr == nil {
			return &Certs{CACert: ca, ServerCert: cert, ServerKey: key, CertFile: certPath, KeyFile: keyPath}, nil
		}
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create cert directory: %w", err)
	}
	ca, cert, key, err := generateCerts(host)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(caPath, ca, 0o644); err != nil {
		return nil, fmt.Errorf("write CA: %w", err)
	}
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		return nil, fmt.Errorf("write server certificate: %w", err)
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		return nil, fmt.Errorf("write server key: %w", err)
	}
	return &Certs{CACert: ca, ServerCert: cert, ServerKey: key, CertFile: certPath, KeyFile: keyPath}, nil
}

func generateCerts(host string) (caCert, serverCert, serverKey []byte, err error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	caSerial, err := serialNumber()
	if err != nil {
		return nil, nil, nil, err
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          caSerial,
		Subject:               pkix.Name{CommonName: "caretaker-webhook-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	serial, err := serialNumber()
	if err != nil {
		return nil, nil, nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "caretaker-webhook"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = []net.IP{ip, net.ParseIP("127.0.0.1")}
		template.DNSNames = []string{"localhost"}
	} else {
		template.DNSNames = []string{host, "localhost"}
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, template, caTemplate, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, nil, err
	}
	caCert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	serverCert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	serverKey = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if _, err := tls.X509KeyPair(serverCert, serverKey); err != nil {
		return nil, nil, nil, fmt.Errorf("validate generated key pair: %w", err)
	}
	return caCert, serverCert, serverKey, nil
}

func serialNumber() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}
