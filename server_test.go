// Copyright 2021 Edward McFarlane. All rights reserved.
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
	"io/ioutil"
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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/emcfarlane/larking/api"
	"github.com/emcfarlane/larking/testpb"
)

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

	ts, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	if err := ts.Mux().RegisterConn(context.Background(), conn); err != nil {
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

	cc, err := grpc.Dial(lisProxy.Addr().String(), grpc.WithInsecure())
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
		//ins    []in
		//outs   []out
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

			ctx := context.Background()
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
					out := proto.Clone(typ.msg)
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

func createCertificateAuthority() ([]byte, []byte, error) {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2021),
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore:             time.Now(),
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
	pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})
	caPrivKeyPEM := new(bytes.Buffer)
	pem.Encode(caPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
	})
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
		NotBefore:    time.Now(),
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
	pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})
	certPrivKeyPEM := new(bytes.Buffer)
	pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})
	return certPEM.Bytes(), certPrivKeyPEM.Bytes(), nil
}

func TestTLSServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
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

	s, err := NewServer(
		LarkingServerOption(map[string]string{"default": ""}),
		TLSCredsOption(tlsConfig),
		MuxOptions(
			UnaryServerInterceptorOption(
				func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
					if err := verfiyPeer(ctx); err != nil {
						return nil, err
					}
					return handler(ctx, req)
				},
			),
		),
	)

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve(l)
	defer s.Shutdown(ctx)

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
		rsp, err := client.Get("https://" + l.Addr().String() + "/v1/threads")
		if err != nil {
			t.Fatal(err)
		}
		if rsp.StatusCode != http.StatusOK {
			t.Fatal("invalid status code", rsp.StatusCode)
		}
		defer rsp.Body.Close()
		b, err := ioutil.ReadAll(rsp.Body)
		if err != nil {
			t.Fatal(err)
		}

		var threads api.ListThreadsResponse
		if err := protojson.Unmarshal(b, &threads); err != nil {
			t.Fatal(err)
		}
		t.Logf("http threads: %+v", &threads)
	})
	t.Run("grpcClient", func(t *testing.T) {
		creds := credentials.NewTLS(tlsConfig)
		cc, err := grpc.DialContext(ctx, l.Addr().String(),
			grpc.WithTransportCredentials(creds),
		)
		if err != nil {
			t.Fatal(err)
		}
		client := api.NewLarkingClient(cc)

		threads, err := client.ListThreads(ctx, &api.ListThreadsRequest{})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("grpc threads: %+v", threads)
	})
	t.Run("httpNoMTLS", func(t *testing.T) {
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsInsecure,
			},
		}
		_, err := client.Get("https://" + l.Addr().String() + "/v1/threads")
		if err == nil {
			t.Fatal("got nil error")
		}
		var nerr *net.OpError
		if errors.As(err, &nerr) {
			t.Log("nerr", nerr)
		} else {
			t.Fatal("unknown error:", err)
		}
		//for err != nil {
		//	t.Logf("%T", err)
		//	err = errors.Unwrap(err)
		//}
	})
	t.Run("grpcNoMTLS", func(t *testing.T) {
		creds := credentials.NewTLS(tlsInsecure)
		cc, err := grpc.DialContext(ctx, l.Addr().String(),
			grpc.WithTransportCredentials(creds),
		)
		if err != nil {
			t.Fatal(err)
		}
		client := api.NewLarkingClient(cc)

		// TODO: why NIL NIL!?!
		if threads, err := client.ListThreads(ctx, &api.ListThreadsRequest{}); threads != nil && err != nil {
			t.Fatal("got nil error", threads, nil)
		}
		for err != nil {
			t.Logf("%T", err)
			err = errors.Unwrap(err)
		}

	})
}

func TestAPIServer(t *testing.T) {
	s, err := NewServer(LarkingServerOption(map[string]string{"default": ""}))
	if err != nil {
		t.Fatal(err)
	}

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

	// Create the client.
	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatalf("cannot connect to server: %v", err)
	}
	defer conn.Close()

	client := api.NewLarkingClient(conn)

	tests := []struct {
		name string
		ins  []*api.Command
		outs []*api.Result
	}{{
		name: "fibonacci",
		ins: []*api.Command{{
			Name: "default",
			Exec: &api.Command_Input{
				Input: `def fibonacci(n):
	    res = list(range(n))
	    for i in res[2:]:
		res[i] = res[i-2] + res[i-1]
	    return res
`},
		}, {
			Exec: &api.Command_Input{
				Input: "fibonacci(10)\n",
			},
		}},
		outs: []*api.Result{{
			Result: &api.Result_Output{
				Output: &api.Output{
					Output: "",
				},
			},
		}, {
			Result: &api.Result_Output{
				Output: &api.Output{
					Output: "[0, 1, 1, 2, 3, 5, 8, 13, 21, 34]",
				},
			},
		}},
	}}
	ctx := context.Background()
	cmpOpts := cmp.Options{protocmp.Transform()}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if len(tt.ins) < len(tt.outs) {
				t.Fatal("invalid args")
			}

			stream, err := client.RunOnThread(ctx)
			if err != nil {
				t.Fatal(err)
			}

			for i := 0; i < len(tt.ins); i++ {
				in := tt.ins[i]
				if err := stream.Send(in); err != nil {
					t.Fatal(err)
				}

				out, err := stream.Recv()
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("out: %v", out)

				diff := cmp.Diff(out, tt.outs[i], cmpOpts...)
				if diff != "" {
					t.Error(diff)
				}
			}
		})
	}
	t.Logf("thread: %v", s.ls.threads["default"])
}
