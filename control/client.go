// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package control

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"

	"github.com/pkg/browser"
	"golang.org/x/sync/errgroup"
)

// Client is the client that connects to a larkingcontrol server for a node.
type Client struct {
	httpc  *http.Client // HTTP client used to talk to larkcontrol
	svrURL *url.URL     // URL of the larkcontrol server
	//timeNow   func() time.Time
	//authorization string

	//lastPrintMap           time.Time
	//newDecompressor        func() (Decompressor, error)
	//keepAlive              bool
	//logf                   logger.Logf
	//linkMon                *monitor.Mon // or nil
	//discoPubKey            key.DiscoPublic
	//getMachinePrivKey      func() (key.MachinePrivate, error)
	//debugFlags             []string
	//keepSharerAndUserSplit bool
	//skipIPForwardingCheck  bool
	//pinger                 Pinger

	//mu           sync.Mutex // mutex guards the following fields
	//serverKey    key.MachinePublic
	//persist      persist.Persist
	//authKey      string
	//tryingNewKey key.NodePrivate
	//expiry       *time.Time
	//// hostinfo is mutated in-place while mu is held.
	//hostinfo      *tailcfg.Hostinfo // always non-nil
	//endpoints     []tailcfg.Endpoint
	//everEndpoints bool   // whether we've ever had non-empty endpoints
	//localPort     uint16 // or zero to mean auto
	//lastPingURL   string // last PingRequest.URL received, for dup suppression
}

func NewClient(addr string) (*Client, error) {
	svrURL, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}

	httpc := http.DefaultClient

	return &Client{
		httpc:  httpc,
		svrURL: svrURL,
	}, nil

}

type Credentials struct {
	Name       string `json:"name"`
	PublicKey  string `json:"public_key"`
	PrivateKey string `json:"private_key"`
}

func (c *Client) getCredentials(ctx context.Context) (map[string]string, error) {
	// open browser

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, err
	}

	newURL := *c.svrURL
	newURL.Path = path.Join(newURL.Path, "/client")
	values := newURL.Query()
	values.Add("redirect_uri", l.Addr().String())
	newURL.RawQuery = values.Encode()

	if err := browser.OpenURL(newURL.String()); err != nil {
		return nil, err
	}

	creds := make(chan map[string]string)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if err := func() error {
			b, err := ioutil.ReadAll(r.Body)
			if err != nil {
				return err
			}
			defer r.Body.Close()

			var m map[string]string
			if err := json.Unmarshal(b, &m); err != nil {
				return err
			}

			select {
			case creds <- m:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	})
	server := &http.Server{Handler: handler}

	var cred map[string]string
	g, _ := errgroup.WithContext(ctx)
	g.Go(func() error {
		cred = <-creds
		return server.Close()
	})
	g.Go(func() error {
		if err := server.Serve(l); err != nil {
			if err != http.ErrServerClosed {
				return err
			}
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return cred, nil
}

func (c *Client) LoadCredentials(ctx context.Context, filename string) (map[string]string, error) {
	if data, err := ioutil.ReadFile(filename); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	} else {
		var creds map[string]string
		if err := json.Unmarshal(data, &creds); err != nil {
			return nil, err
		}
		return creds, nil
	}

	creds, err := c.getCredentials(ctx)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(creds)
	if err != nil {
		return nil, err
	}

	if err := ioutil.WriteFile(filename, data, os.ModePerm); err != nil {
		return nil, err
	}
	return creds, nil
}
