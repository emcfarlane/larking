// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package control

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sync"

	"github.com/pkg/browser"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/encoding/protojson"
	"larking.io/api/controlpb"
)

// TODO: use OAuth2 libraries directly.

// Client is the client that connects to a larkingcontrol server for a node.
type Client struct {
	httpc    *http.Client // HTTP client used to talk to larkcontrol
	svrURL   *url.URL     // URL of the larkcontrol server
	cacheDir string       // Cache directory

	mu     sync.Mutex
	perRPC *PerRPCCredentials
}

func NewClient(addr, cacheDir string) (*Client, error) {
	svrURL, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}

	httpc := http.DefaultClient

	return &Client{
		httpc:    httpc,
		svrURL:   svrURL,
		cacheDir: cacheDir,
	}, nil

}

// OpenPerRPCCredentials at the control server address.
func (c *Client) OpenRPCCredentials(ctx context.Context) (*PerRPCCredentials, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.perRPC != nil {
		return c.perRPC, nil
	}

	// Login, save creds to file.
	credFile := path.Join(c.cacheDir, "credentials.json")

	creds, err := c.doLogin(ctx)
	if err != nil {
		return nil, fmt.Errorf("doLogin: %w", err)
	}

	b, err := protojson.Marshal(creds)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(credFile, b, 0644); err != nil {
		return nil, err
	}

	u := "file://" + credFile
	perRPC, err := OpenRPCCredentials(ctx, u)
	if err != nil {
		return nil, err
	}
	c.perRPC = perRPC
	return perRPC, nil
}

func (c *Client) doLogin(ctx context.Context) (*controlpb.Credentials, error) {
	// open browser

	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}

	newURL := *c.svrURL
	newURL.Path = path.Join(newURL.Path, "/login")
	values := newURL.Query()
	values.Add("mode", "select")
	values.Add("signInSuccessUrl", "http://"+l.Addr().String())
	newURL.RawQuery = values.Encode()

	fmt.Println("Opening URL to complete login flow: ", newURL.String())
	if err := browser.OpenURL(newURL.String()); err != nil {
		return nil, err
	}

	var (
		cred  *controlpb.Credentials
		creds = make(chan *controlpb.Credentials)
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if err := func() error {
			//body, err := httputil.DumpRequest(r, true)
			//if err != nil {
			//	return err
			//}
			//fmt.Println("--- body ---")
			//fmt.Println(string(body))
			//fmt.Println()

			if r.Method != http.MethodGet || r.URL.Path != "/" {
				okURL := *c.svrURL
				okURL.Path = r.URL.Path
				http.Redirect(w, r, okURL.String(), http.StatusFound)
				return nil
			}

			q := r.URL.Query()
			name := q.Get("name")
			accessToken := q.Get("accessToken")

			v := controlpb.Credentials{
				Name: name,
				Type: &controlpb.Credentials_Bearer{
					Bearer: &controlpb.Credentials_BearerToken{
						AccessToken: accessToken,
					},
				},
			}

			select {
			case creds <- &v:
			case <-ctx.Done():
				return ctx.Err()
			}

			okURL := *c.svrURL
			okURL.Path = path.Join(okURL.Path, "/success")
			http.Redirect(w, r, okURL.String(), http.StatusFound)
			return nil
		}(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	})
	server := &http.Server{Handler: handler}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if err := server.Serve(l); err != nil {
			if err != http.ErrServerClosed {
				return err
			}
		}
		return nil
	})
	g.Go(func() error {
		select {
		case v := <-creds:
			cred = v
		case <-gctx.Done():
			return gctx.Err()
		}
		return server.Close()
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return cred, nil
}
