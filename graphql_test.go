// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emcfarlane/larking/testpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type starWarsServer struct {
	factions     map[string]*testpb.Faction
	ships        map[string]*testpb.Ship
	factionShips map[string][]*testpb.Ship
}

func newStarWarsServer(t *testing.T) *starWarsServer {
	var (
		xwing = &testpb.Ship{
			Name:        "factions/1/ships/1",
			DisplayName: "X-Wing",
		}
		ywing = &testpb.Ship{
			Name:        "factions/1/ships/2",
			DisplayName: "Y-Wing",
		}
		awing = &testpb.Ship{
			Name:        "factions/1/ships/3",
			DisplayName: "A-Wing",
		}
		falcon = &testpb.Ship{
			Name:        "factions/1/ships/4",
			DisplayName: "Millenium Falcon",
		}
		homeOne = &testpb.Ship{
			Name:        "factions/1/ships/5",
			DisplayName: "Home One",
		}
		tieFighter = &testpb.Ship{
			Name:        "factions/2/ships/6",
			DisplayName: "TIE Fighter",
		}
		tieInterceptor = &testpb.Ship{
			Name:        "factions/2/ships/7",
			DisplayName: "TIE Interceptor",
		}
		executor = &testpb.Ship{
			Name:        "factions/2/ships/8",
			DisplayName: "Executor",
		}

		rebels = &testpb.Faction{
			Name:        "factions/1",
			DisplayName: "Alliance to Restore the Republic",
		}
		empire = &testpb.Faction{
			Name:        "factions/2",
			DisplayName: "Galactic Empire",
		}
	)

	return &starWarsServer{
		factions: map[string]*testpb.Faction{
			"factions/1": rebels,
			"factions/2": empire,
		},
		ships: map[string]*testpb.Ship{
			"factions/1/ships/1": xwing,
			"factions/1/ships/2": ywing,
			"factions/1/ships/3": awing,
			"factions/1/ships/4": falcon,
			"factions/1/ships/5": homeOne,
			"factions/2/ships/6": tieFighter,
			"factions/2/ships/7": tieInterceptor,
			"factions/2/ships/8": executor,
		},
		factionShips: map[string][]*testpb.Ship{
			"factions/1": {xwing, ywing, awing, falcon, homeOne},
			"factions/2": {tieFighter, tieInterceptor, executor},
		},
	}
}

func (s *starWarsServer) GetFaction(ctx context.Context, req *testpb.GetFactionRequest) (*testpb.Faction, error) {
	//id := strings.TrimPrefix("factions/", req.Name)
	v, ok := s.factions[req.Name]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "missing faction: %s", req.Name)
	}
	return v, nil
}

func (s *starWarsServer) GetShip(ctx context.Context, req *testpb.GetShipRequest) (*testpb.Ship, error) {
	//id := strings.TrimPrefix("ships/", req.Name)
	v, ok := s.ships[req.Name]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "missing ship: %s", req.Name)
	}
	return v, nil
}

func (s *starWarsServer) ListShips(ctx context.Context, req *testpb.ListShipsRequest) (*testpb.ListShipsResponse, error) {
	//pid := strings.TrimPrefix(req.Parent, "factions/")
	vs, ok := s.factionShips[req.Parent]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "missing factions ships: %s", req.Parent)
	}
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 10
	}
	pageToken := req.PageToken

	if pageToken != "" {
		for i, v := range vs {
			if v.Name == pageToken {
				vs = vs[i+1:]
				break
			}
		}
	}

	if len(vs) > pageSize {
		vs = vs[:pageSize]
	}

	var nextPageToken string
	if len(vs) > 0 {
		nextPageToken = vs[len(vs)-1].Name
	}

	return &testpb.ListShipsResponse{
		Ships:         vs,
		NextPageToken: nextPageToken,
	}, nil
}

//func (s *starWarsServer) GetRelayNode(ctx context.Context, req *api.GetRelayNodeRequest) (*anypb.Any, error) {
//	ps := strings.Split(req.Id, "/")
//	if len(ps) != 2 {
//		return nil, status.Errorf(codes.InvalidArgument, "invalid name: %v", req.Id)
//	}
//	switch pfx := ps[0]; pfx {
//	case "factions":
//		v, err := s.GetFaction(ctx, &testpb.GetFactionRequest{Name: req.Id})
//		if err != nil {
//			return nil, err
//		}
//		return anypb.New(v)
//	case "ships":
//		v, err := s.GetShip(ctx, &testpb.GetShipRequest{Name: req.Id})
//		if err != nil {
//			return nil, err
//		}
//		return anypb.New(v)
//	default:
//		return nil, status.Errorf(codes.NotFound, "missing node: %s", req.Id)
//	}
//}

func TestGraphQLStarWars(t *testing.T) {
	ss := newStarWarsServer(t)

	m, err := NewMux()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterServiceByName("larking.testpb.StarWars", ss); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		query   string
		want    string
		wantErr error
	}{{
		name: "getRebels",
		query: `query RebelsQuery {
  rebels {
    id
    name
  }
}`,
		want: `{
  "rebels": {
    "id": "RmFjdGlvbjox",
    "name": "Alliance to Restore the Republic"
  }
}`,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(
				http.MethodPost, "/graphql", strings.NewReader(tt.query),
			)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			m.ServeHTTP(w, r)
			t.Log(w)
			res := w.Result()
			if res.StatusCode != 200 {
				t.Fatal(res.Status)
			}

			data, err := ioutil.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			t.Log(string(data))

			if rsp := string(data); rsp != tt.want {
				t.Fatalf("response mismatch: %s != %s", rsp, tt.want)
			}

		})
	}
}
