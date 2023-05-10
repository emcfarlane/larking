package larking

import (
	"context"

	"github.com/graphql-go/graphql"
	"larking.io/api/graphqlpb"
)

func toGraphqlErrors(errs []gqlerrors.FormattedError) []*graphqlpb.Error {
	if len(errs) == 0 {
		return nil
	}
	r := make([]*graphqlpb.Error, len(errs))
	for i, err := range errs {
		r[i] = &graphqlpb.GraphQLError{
			Message:   err.Message,
			Locations: err.Locations,
			Path:      err.Path,
		}
	}
	return r
}

func (m *Mux) Query(ctx context.Context, req *graphqlpb.Request) (*graphqlpb.Response, error) {
	s := m.loadState()

	params := &graphql.Params{
		Schema:         s.graphqlSchema,
		RequestString:  req.Query,
		VariableValues: req.Variables.Fields,
		OperationName:  req.OperationName,
		Context:        ctx,
	}
	result := graphql.Do(params)
	return &graphqlpb.Response{
		Data:   result.Data,
		Errors: toGraphqlErrors(result.Errors),
	}, nil
}
