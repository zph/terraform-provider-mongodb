//go:build integration

package mongodb

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// testPKI is a throwaway CA for the TLS integration test.
type testPKI struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func newTestPKI(t *testing.T) *testPKI {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test Root CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	return &testPKI{cert: cert, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

// issue signs a leaf certificate and returns it as a combined PEM bundle,
// certificate first and PKCS#8 private key second: the layout mongod reads
// from tlsCertificateKeyFile and the provider reads from client_certificate_key.
func (p *testPKI) issue(t *testing.T, template *x509.Certificate) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	template.NotBefore = time.Now().Add(-time.Hour)
	template.NotAfter = time.Now().Add(24 * time.Hour)
	template.KeyUsage = x509.KeyUsageDigitalSignature
	der, err := x509.CreateCertificate(rand.Reader, template, p.cert, &key.PublicKey, p.key)
	if err != nil {
		t.Fatalf("create leaf certificate: %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal leaf key: %v", err)
	}
	var buf bytes.Buffer
	_ = pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	_ = pem.Encode(&buf, &pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return buf.Bytes()
}

// mongodTLSArgs returns the mongod flags that require TLS and, through the
// default allowConnectionsWithoutCertificates=false, a client certificate.
// Images before 4.2 only know the ssl spelling.
func mongodTLSArgs(image, caPath, serverPath string) []string {
	tag := image[strings.LastIndex(image, ":")+1:]
	if strings.HasPrefix(tag, "3.") || strings.HasPrefix(tag, "4.0") {
		return []string{"--sslMode", "requireSSL", "--sslPEMKeyFile", serverPath, "--sslCAFile", caPath}
	}
	return []string{"--tlsMode", "requireTLS", "--tlsCertificateKeyFile", serverPath, "--tlsCAFile", caPath}
}

// A mongod started with a CA file rejects every handshake that presents no
// client certificate. The provider has to get through with
// client_certificate_key and be turned away without it.
func TestIntegration_ClientCertificate(t *testing.T) {
	ctx := context.Background()
	image := testMongoImage
	if env := os.Getenv("MONGO_TEST_IMAGE"); env != "" {
		image = env
	}

	dockerProvider, err := testcontainers.NewDockerProvider()
	if err != nil {
		t.Fatalf("docker provider: %v", err)
	}
	daemonHost, err := dockerProvider.DaemonHost(ctx)
	if err != nil {
		t.Fatalf("daemon host: %v", err)
	}

	pki := newTestPKI(t)
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{Organization: []string{"Test Server"}, CommonName: "mongod"},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	if ip := net.ParseIP(daemonHost); ip != nil {
		serverTemplate.IPAddresses = append(serverTemplate.IPAddresses, ip)
	} else {
		serverTemplate.DNSNames = append(serverTemplate.DNSNames, daemonHost)
	}
	serverBundle := pki.issue(t, serverTemplate)
	clientBundle := pki.issue(t, &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{Organization: []string{"Test Client"}, CommonName: "terraform"},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})

	const caPath, serverPath = "/tls/ca.pem", "/tls/server.pem"
	natPort := nat.Port("27017/tcp")
	cmd := append([]string{"mongod", "--port", "27017", "--bind_ip_all"}, mongodTLSArgs(image, caPath, serverPath)...)
	container, err := testcontainers.Run(ctx, image,
		testcontainers.WithCmd(cmd...),
		testcontainers.WithFiles(
			testcontainers.ContainerFile{Reader: bytes.NewReader(pki.pem), ContainerFilePath: caPath, FileMode: 0o644},
			testcontainers.ContainerFile{Reader: bytes.NewReader(serverBundle), ContainerFilePath: serverPath, FileMode: 0o644},
		),
		testcontainers.WithExposedPorts(string(natPort)),
		testcontainers.WithWaitStrategy(wait.ForListeningPort(natPort).WithStartupTimeout(120*time.Second)),
	)
	if err != nil {
		t.Fatalf("start TLS mongod: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	mappedPort, err := container.MappedPort(ctx, natPort)
	if err != nil {
		t.Fatalf("container port: %v", err)
	}
	base := ClientConfig{Host: host, Port: mappedPort.Port(), Ssl: true, Direct: true, Certificate: string(pki.pem)}

	cases := []struct {
		name                 string
		clientCertificateKey string
		wantErr              bool
	}{
		{"client certificate is accepted", string(clientBundle), false},
		{"missing client certificate is rejected", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			cfg.ClientCertificateKey = tc.clientCertificateKey
			client, err := MongoClientInit(ctx, &MongoDatabaseConfiguration{Config: &cfg, MaxConnLifetime: 10})
			if tc.wantErr {
				if err == nil {
					_ = client.Disconnect(ctx)
					t.Fatal("expected the handshake to be rejected without a client certificate")
				}
				t.Logf("rejected as expected: %v", err)
				return
			}
			if err != nil {
				t.Fatalf("connect with client certificate: %v", err)
			}
			_ = client.Disconnect(ctx)
		})
	}
}
