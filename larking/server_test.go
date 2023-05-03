// Copyright 2022 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/testing/protocmp"

	"larking.io/api/healthpb"
	"larking.io/api/testpb"
	"larking.io/health"
)

func testContext(t *testing.T) context.Context {
	ctx := context.Background()
	return ctx
}

func TestServer(t *testing.T) {
	ms := &testpb.UnimplementedMessagingServer{}

	o := &overrides{}
	gs := grpc.NewServer(o.streamOption(), o.unaryOption())
	testpb.RegisterMessagingServer(gs, ms)
	reflection.Register(gs)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	var g errgroup.Group
	defer func() {
		if err := g.Wait(); err != nil {
			t.Fatal(err)
		}
	}()

	g.Go(func() error {
		return gs.Serve(lis)
	})
	defer gs.Stop()

	// Create the client.
	creds := insecure.NewCredentials()
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatalf("cannot connect to server: %v", err)
	}
	defer conn.Close()

	mux, err := NewMux()
	if err != nil {
		t.Fatal(err)
	}
	if err := mux.RegisterConn(context.Background(), conn); err != nil {
		t.Fatal(err)
	}

	ts, err := NewServer(mux, InsecureServerOption())
	if err != nil {
		t.Fatal(err)
	}

	lisProxy, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lisProxy.Close()

	g.Go(func() error {
		if err := ts.Serve(lisProxy); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
	defer func() {
		if err := ts.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	}()

	cc, err := grpc.Dial(
		lisProxy.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatal(err)
	}

	cmpOpts := cmp.Options{protocmp.Transform()}

	var unaryStreamDesc = &grpc.StreamDesc{
		ClientStreams: false,
		ServerStreams: false,
	}

	tests := []struct {
		name   string
		desc   *grpc.StreamDesc
		method string
		inouts []interface{}
	}{{
		name:   "unary_message",
		desc:   unaryStreamDesc,
		method: "/larking.testpb.Messaging/GetMessageOne",
		inouts: []interface{}{
			in{msg: &testpb.GetMessageRequestOne{Name: "proxy"}},
			out{msg: &testpb.Message{Text: "success"}},
		},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o.reset(t, "test", tt.inouts)

			ctx := testContext(t)
			ctx = metadata.AppendToOutgoingContext(ctx, "test", tt.method)

			s, err := cc.NewStream(ctx, tt.desc, tt.method)
			if err != nil {
				t.Fatal(err)
			}

			for i := 0; i < len(tt.inouts); i++ {
				switch typ := tt.inouts[i].(type) {
				case in:
					if err := s.SendMsg(typ.msg); err != nil {
						t.Fatal(err)
					}
				case out:
					out := typ.msg.ProtoReflect().New().Interface()
					if err := s.RecvMsg(out); err != nil {
						t.Fatal(err)
					}
					diff := cmp.Diff(out, typ.msg, cmpOpts...)
					if diff != "" {
						t.Fatal(diff)
					}
				}
			}
		})
	}
}

func TestMuxHandleOption(t *testing.T) {
	mux, err := NewMux()
	if err != nil {
		t.Fatal(err)
	}

	hs := health.NewServer()
	defer hs.Shutdown()
	mux.RegisterService(&healthpb.Health_ServiceDesc, hs)

	s, err := NewServer(
		mux,
		InsecureServerOption(),
		MuxHandleOption("/", "/api/", "/pfx"),
	)
	if err != nil {
		t.Fatal(err)
	}

	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	var g errgroup.Group
	defer func() {
		if err := g.Wait(); err != nil {
			t.Fatal(err)
		}
	}()

	g.Go(func() (err error) {
		if err := s.Serve(lis); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
	defer func() {
		if err := s.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	}()

	for _, tt := range []struct {
		path string
		okay bool
	}{
		{"/v1/health", true},
		{"/api/v1/health", true},
		{"/pfx/v1/health", true},
		{"/bad/v1/health", false},
		{"/v1/health/bad", false},
	} {
		t.Run(tt.path, func(t *testing.T) {
			rsp, err := http.Get("http://" + lis.Addr().String() + tt.path)
			if err != nil {
				t.Fatal(err)
			}
			okay := rsp.StatusCode == 200
			if okay != tt.okay {
				t.Errorf("request got %t for %s", okay, tt.path)
			}
		})
	}
}

func createCertificateAuthority() ([]byte, []byte, error) {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2021),
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore:             time.Now().AddDate(-1, 0, 0),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}
	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, nil, err
	}

	caPEM := new(bytes.Buffer)
	if err := pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	}); err != nil {
		return nil, nil, err
	}
	caPrivKeyPEM := new(bytes.Buffer)
	if err := pem.Encode(caPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
	}); err != nil {
		return nil, nil, err
	}
	return caPEM.Bytes(), caPrivKeyPEM.Bytes(), nil
}

func createCertificate(caCertPEM, caKeyPEM []byte, commonName string) ([]byte, []byte, error) {
	keyPEMBlock, _ := pem.Decode(caKeyPEM)
	privateKey, err := x509.ParsePKCS1PrivateKey(keyPEMBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	certPEMBlock, _ := pem.Decode(caCertPEM)
	parent, err := x509.ParseCertificate(certPEMBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1658),
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
			CommonName:   commonName,
		},
		IPAddresses: []net.IP{
			net.IPv4(127, 0, 0, 1),
			net.IPv6loopback,
			net.IPv4(0, 0, 0, 0),
			net.IPv6zero,
		},
		NotBefore:    time.Now().AddDate(-1, 0, 0),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, cert, parent, &certPrivKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM := new(bytes.Buffer)
	if err := pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	}); err != nil {
		return nil, nil, err
	}
	certPrivKeyPEM := new(bytes.Buffer)
	if err := pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	}); err != nil {
		return nil, nil, err
	}
	return certPEM.Bytes(), certPrivKeyPEM.Bytes(), nil
}

func TestTLSServer(t *testing.T) {
	ctx, cancel := context.WithCancel(testContext(t))
	defer cancel()

	// certPool
	certPool := x509.NewCertPool()
	caCertPEM, caKeyPEM, err := createCertificateAuthority()
	if err != nil {
		t.Fatal(err)
	}
	if ok := certPool.AppendCertsFromPEM(caCertPEM); !ok {
		t.Fatal("failed to append client certs")
	}

	certPEM, keyPEM, err := createCertificate(caCertPEM, caKeyPEM, "Server")
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig := &tls.Config{
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{certificate},
		ClientCAs:    certPool,
	}

	// TODO!
	verfiyPeer := func(ctx context.Context) error {
		//	p, ok := peer.FromContext(ctx)
		//	if !ok {
		//		return status.Error(codes.Unauthenticated, "no peer found")
		//	}
		//	tlsAuth, ok := p.AuthInfo.(credentials.TLSInfo)
		//	if !ok {
		//		return status.Error(codes.Unauthenticated, "unexpected peer transport credentials")
		//	}
		//	if len(tlsAuth.State.VerifiedChains) == 0 || len(tlsAuth.State.VerifiedChains[0]) == 0 {
		//		return status.Error(codes.Unauthenticated, "could not verify peer certificate")
		//	}
		//	fmt.Println(
		//		"tlsAuth.State.VerifiedChains[0][0].Subject.CommonName",
		//		tlsAuth.State.VerifiedChains[0][0].Subject.CommonName,
		//	)
		//	// Check subject common name against configured username
		//	if tlsAuth.State.VerifiedChains[0][0].Subject.CommonName != "Client" {
		//		return status.Error(codes.Unauthenticated, "invalid subject common name")
		//	}
		return nil
	}

	mux, err := NewMux(
		UnaryServerInterceptorOption(
			func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
				if err := verfiyPeer(ctx); err != nil {
					return nil, err
				}
				return handler(ctx, req)
			},
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	healthServer := health.NewServer()
	defer healthServer.Shutdown()
	mux.RegisterService(&healthpb.Health_ServiceDesc, healthServer)

	s, err := NewServer(mux,
		TLSCredsOption(tlsConfig),
	)
	if err != nil {
		t.Fatal(err)
	}

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}

	g := errgroup.Group{}
	g.Go(func() error { return s.Serve(l) })
	defer func() {
		if err := s.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		if err := g.Wait(); err != nil && err != http.ErrServerClosed {
			t.Error(err)
		}
	}()

	certPEM, keyPEM, err = createCertificate(caCertPEM, caKeyPEM, "Client")
	if err != nil {
		t.Fatal(err)
	}
	certificate, err = tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{certificate},
		RootCAs:      certPool,
	}
	tlsInsecure := &tls.Config{
		InsecureSkipVerify: true,
	}

	t.Run("httpClient", func(t *testing.T) {
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		}
		rsp, err := client.Get("https://" + l.Addr().String() + "/v1/health")
		if err != nil {
			t.Fatal(err)
		}
		if rsp.StatusCode != http.StatusOK {
			t.Fatal("invalid status code", rsp.StatusCode)
		}
		defer rsp.Body.Close()
		b, err := io.ReadAll(rsp.Body)
		if err != nil {
			t.Fatal(err)
		}

		var check healthpb.HealthCheckResponse
		if err := protojson.Unmarshal(b, &check); err != nil {
			t.Fatal(err)
		}
		t.Logf("http threads: %+v", &check)
	})
	t.Run("grpcClient", func(t *testing.T) {
		creds := credentials.NewTLS(tlsConfig)
		cc, err := grpc.DialContext(ctx, l.Addr().String(),
			grpc.WithTransportCredentials(creds),
		)
		if err != nil {
			t.Fatal(err)
		}
		client := healthpb.NewHealthClient(cc)

		check, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("grpc threads: %+v", check)
	})
	t.Run("httpNoMTLS", func(t *testing.T) {
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsInsecure,
			},
		}
		_, err := client.Get("https://" + l.Addr().String() + "/v1/health")
		if err == nil {
			t.Fatal("got nil error")
		}
		var nerr *net.OpError
		if errors.As(err, &nerr) {
			t.Log("nerr", nerr)
		} else {
			for err := errors.Unwrap(err); err != nil; err = errors.Unwrap(err) {
				t.Logf("%T: %v", err, err)
			}
		}
	})
	t.Run("grpcNoMTLS", func(t *testing.T) {
		creds := credentials.NewTLS(tlsInsecure)
		cc, err := grpc.DialContext(ctx, l.Addr().String(),
			grpc.WithTransportCredentials(creds),
		)
		if err != nil {
			t.Fatal(err)
		}
		client := healthpb.NewHealthClient(cc)

		// TODO: why NIL NIL!?!
		check, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
		if check != nil && err != nil {
			t.Fatal("got nil error", check, err)
		}
		for err != nil {
			t.Logf("%T", err)
			err = errors.Unwrap(err)
		}

	})
}
