package larking

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"larking.io/api/graphqlpb"
	"larking.io/api/starwarspb"
)

type starWarsServer struct {
	factions     map[string]*starwarspb.Faction
	ships        map[string]*starwarspb.Ship
	factionShips map[string][]*starwarspb.Ship
}

func newStarWarsServer(t *testing.T) *starWarsServer {
	var (
		xwing = &starwarspb.Ship{
			Name:        "factions/rebels/ships/1",
			DisplayName: "X-Wing",
		}
		ywing = &starwarspb.Ship{
			Name:        "factions/rebels/ships/2",
			DisplayName: "Y-Wing",
		}
		awing = &starwarspb.Ship{
			Name:        "factions/rebels/ships/3",
			DisplayName: "A-Wing",
		}
		falcon = &starwarspb.Ship{
			Name:        "factions/rebels/ships/4",
			DisplayName: "Millenium Falcon",
		}
		homeOne = &starwarspb.Ship{
			Name:        "factions/rebels/ships/5",
			DisplayName: "Home One",
		}
		tieFighter = &starwarspb.Ship{
			Name:        "factions/empire/ships/6",
			DisplayName: "TIE Fighter",
		}
		tieInterceptor = &starwarspb.Ship{
			Name:        "factions/empire/ships/7",
			DisplayName: "TIE Interceptor",
		}
		executor = &starwarspb.Ship{
			Name:        "factions/empire/ships/8",
			DisplayName: "Executor",
		}

		rebels = &starwarspb.Faction{
			Name:        "factions/rebels",
			DisplayName: "Alliance to Restore the Republic",
		}
		empire = &starwarspb.Faction{
			Name:        "factions/empire",
			DisplayName: "Galactic Empire",
		}
	)

	return &starWarsServer{
		factions: map[string]*starwarspb.Faction{
			"factions/rebels": rebels,
			"factions/empire": empire,
		},
		ships: map[string]*starwarspb.Ship{
			"factions/rebels/ships/1": xwing,
			"factions/rebels/ships/2": ywing,
			"factions/rebels/ships/3": awing,
			"factions/rebels/ships/4": falcon,
			"factions/rebels/ships/5": homeOne,
			"factions/empire/ships/6": tieFighter,
			"factions/empire/ships/7": tieInterceptor,
			"factions/empire/ships/8": executor,
		},
		factionShips: map[string][]*starwarspb.Ship{
			"factions/1": {xwing, ywing, awing, falcon, homeOne},
			"factions/2": {tieFighter, tieInterceptor, executor},
		},
	}
}

func (s *starWarsServer) GetFaction(ctx context.Context, req *starwarspb.GetFactionRequest) (*starwarspb.Faction, error) {
	//id := strings.TrimPrefix("factions/", req.Name)
	v, ok := s.factions[req.Name]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "missing faction: %s", req.Name)
	}
	return v, nil
}

func (s *starWarsServer) GetShip(ctx context.Context, req *starwarspb.GetShipRequest) (*starwarspb.Ship, error) {
	//id := strings.TrimPrefix("ships/", req.Name)
	v, ok := s.ships[req.Name]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "missing ship: %s", req.Name)
	}
	return v, nil
}

func (s *starWarsServer) ListShips(ctx context.Context, req *starwarspb.ListShipsRequest) (*starwarspb.ListShipsResponse, error) {
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

	return &starwarspb.ListShipsResponse{
		Ships:         vs,
		NextPageToken: nextPageToken,
	}, nil
}

func TestGraphQL(t *testing.T) {
	m, err := NewMux()
	if err != nil {
		t.Fatal(err)
	}

	//starwarspb.RegisterStarWarsServer(m, &starwarsServer{})

	toStruct := func(str string) *structpb.Struct {
		var s structpb.Struct
		if err := protojson.Unmarshal([]byte(str), &s); err != nil {
			t.Fatal(err)
		}
		return &s
	}

	//t.Skip("TODO")
	tests := []struct {
		name string
		req  *graphqlpb.GraphQLRequest
		rsp  *graphqlpb.GraphQLResponse
		err  error
	}{{
		name: "hero",
		req: &graphqlpb.GraphQLRequest{
			Query: `{
				rebels {
					id
					name
				}
			}`,
		},
		rsp: &graphqlpb.GraphQLResponse{
			Data: toStruct(`{
				"hero": {
					"name": "R2-D2"
				}
			}`),
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			rsp, err := m.Query(ctx, tt.req)
			if err != nil {
				if errors.Is(err, tt.err) {
					t.Skip(err)
				}
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.rsp, rsp); diff != "" {
				t.Errorf("diff: %s", diff)
			}
		})
	}
}
