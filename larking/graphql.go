package larking

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/gqlerrors"
	"github.com/graphql-go/graphql/language/location"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"
	"larking.io/api/graphqlpb"
)

// getExtensionGraphQL returns the graphql extension of the proto message.
func getExtensionGraphQL(m proto.Message) *graphqlpb.Rule {
	return proto.GetExtension(m, graphqlpb.E_Graphql).(*graphqlpb.Rule)
}

type stateKeyT struct{}

var stateKey stateKeyT

func ctxWithState(ctx context.Context, s *state) context.Context {
	return context.WithValue(ctx, stateKey, s)
}
func stateFromCtx(ctx context.Context) *state {
	return ctx.Value(stateKey).(*state)
}

type streamCall struct {
	ctx    context.Context
	name   string
	params params
	send   proto.Message
}

func (s *streamCall) SetHeader(md metadata.MD) error  { return nil }
func (s *streamCall) SendHeader(md metadata.MD) error { return nil }
func (s *streamCall) SetTrailer(md metadata.MD)       {}

func (s *streamCall) Context() context.Context {
	sts := &serverTransportStream{s, s.name}
	return grpc.NewContextWithServerTransportStream(s.ctx, sts)
}

func (s *streamCall) SendMsg(v any) error {
	s.send = v.(proto.Message)
	return nil
}

func (s *streamCall) RecvMsg(m interface{}) error {
	args := m.(proto.Message)

	if err := s.params.set(args); err != nil {
		return err
	}
	return nil
}

func parseGraphQLParam(fds []protoreflect.FieldDescriptor, val any) (param, error) {
	if len(fds) == 0 {
		return param{}, fmt.Errorf("zero field")
	}
	fd := fds[len(fds)-1]

	switch kind := fd.Kind(); kind {
	case protoreflect.StringKind:
		return param{fds: fds, val: protoreflect.ValueOfString(val.(string))}, nil
	default:
		return param{}, fmt.Errorf("unsupported kind %v", kind)
	}
}

func (s *state) addGraphQLRule(
	opts muxOptions,
	rule *graphqlpb.Rule,
	desc protoreflect.MethodDescriptor,
	name string,
) error {
	fmt.Println("addGraphQLRule", rule)
	fmt.Println("desc", desc)

	// TODO

	// Schema

	factionType := graphql.NewObject(graphql.ObjectConfig{
		Name:        "Faction",
		Description: "Faction type",
		Fields: graphql.Fields{
			"name": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.String),
				Description: "The name of the faction.",
				Resolve: func(p graphql.ResolveParams) (any, error) {
					if msg, ok := p.Source.(proto.Message); ok {
						mr := msg.ProtoReflect()
						desc := mr.Descriptor()
						fd := desc.Fields().ByJSONName("name")
						return mr.Get(fd).Interface(), nil
					}
					return nil, nil
				},
			},
			"displayName": &graphql.Field{
				Type:        graphql.String,
				Description: "The displayName of the faction.",
				Resolve: func(p graphql.ResolveParams) (any, error) {
					if msg, ok := p.Source.(proto.Message); ok {
						mr := msg.ProtoReflect()
						desc := mr.Descriptor()
						fd := desc.Fields().ByJSONName("displayName")
						return mr.Get(fd).Interface(), nil
					}
					return nil, nil
				},
			},
		},
	})

	msgDesc := desc.Input()
	fieldDescs := msgDesc.Fields()

	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"faction": &graphql.Field{
				Type: factionType,
				Args: graphql.FieldConfigArgument{
					"name": &graphql.ArgumentConfig{
						Description: "faction description",
						Type:        graphql.String,
					},
				},
				Resolve: func(p graphql.ResolveParams) (any, error) {
					// fetch data from handler
					// convert to structpb.pb
					ctx := p.Context
					s := stateFromCtx(ctx)
					fmt.Println("state", s)
					fmt.Println("p", p.Args)

					var params params
					for k, v := range p.Args {
						fd := fieldDescs.ByJSONName(k)
						p, err := parseGraphQLParam([]protoreflect.FieldDescriptor{fd}, v)
						if err != nil {
							return nil, err
						}
						params = append(params, p)
					}

					hd, err := s.pickMethodHandler(name)
					if err != nil {
						return nil, err
					}

					stream := &streamCall{
						ctx:    ctx,
						name:   name,
						params: params,
					}
					if err := hd.handler(&opts, stream); err != nil {
						return nil, err
					}
					fmt.Println("stream.send", stream.send)
					return stream.send, nil
				},
			},
		},
	})

	schemaConfig := graphql.SchemaConfig{
		Query: queryType,
	}

	var err error
	s.gqlSchema, err = graphql.NewSchema(schemaConfig)
	if err != nil {
		return err
	}

	return nil
}

func toGraphQLPath(path []any) []string {
	if len(path) == 0 {
		return nil
	}
	r := make([]string, len(path))
	for i, p := range path {
		r[i] = p.(string)
	}
	return r
}

func toGraphQLLocations(locs []location.SourceLocation) []*graphqlpb.SourceLocation {
	if len(locs) == 0 {
		return nil
	}
	r := make([]*graphqlpb.SourceLocation, len(locs))
	for i, loc := range locs {
		r[i] = &graphqlpb.SourceLocation{
			Line:   int32(loc.Line),
			Column: int32(loc.Column),
		}
	}
	return r
}

func toGraphQLErrors(errs []gqlerrors.FormattedError) []*graphqlpb.Error {
	if len(errs) == 0 {
		return nil
	}
	r := make([]*graphqlpb.Error, len(errs))
	for i, err := range errs {
		r[i] = &graphqlpb.Error{
			Message:   err.Message,
			Locations: toGraphQLLocations(err.Locations),
			Path:      toGraphQLPath(err.Path),
		}
	}
	return r
}

// TODO: figure out how to keep structpb.Values
func toGraphQLData(data any) *structpb.Struct {
	fmt.Println("toGraphqlData", data)
	for k, v := range data.(map[string]interface{}) {
		fmt.Println("k", k)
		fmt.Println("v", v)
		fmt.Printf("%T\n", v)

		//s.Fields[k] = &structpb.Value{
		//	Kind: &structpb.Value_StringValue{
		//		StringValue: v.(string),
		//	},
		//}
	}
	b, _ := json.Marshal(data)
	var s structpb.Struct
	protojson.Unmarshal(b, &s)
	return &s
}

func (m *Mux) Query(ctx context.Context, req *graphqlpb.Request) (*graphqlpb.Response, error) {
	s := m.loadState()
	if s == nil {
		return nil, fmt.Errorf("schema not loaded")
	}
	ctx = ctxWithState(ctx, s)

	params := graphql.Params{
		Schema:         s.gqlSchema,
		RequestString:  req.Query,
		VariableValues: nil, // TODO: req.Variables.Fields,
		OperationName:  req.OperationName,
		Context:        ctx,
	}
	result := graphql.Do(params)
	fmt.Println(result)
	return &graphqlpb.Response{
		Data:   toGraphQLData(result.Data),
		Errors: toGraphQLErrors(result.Errors),
	}, nil
}
