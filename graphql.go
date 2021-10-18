// Copyright 2021 Edward McFarlane. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package larking

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/graphql-go/graphql"
)

func (m *Mux) serveGraphQL(w http.ResponseWriter, r *http.Request) error {
	fmt.Println("serve graphQL")

	// Schema
	fields := graphql.Fields{
		"hello": &graphql.Field{
			Type: graphql.String,
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				return "world", nil
			},
		},
	}
	rootQuery := graphql.ObjectConfig{Name: "RootQuery", Fields: fields}
	schemaConfig := graphql.SchemaConfig{Query: graphql.NewObject(rootQuery)}
	schema, err := graphql.NewSchema(schemaConfig)
	if err != nil {
		log.Fatalf("failed to create new schema, error: %v", err)
	}

	// Query
	query := `
		{
			hello
		}
	`
	params := graphql.Params{Schema: schema, RequestString: query}
	res := graphql.Do(params)
	if len(res.Errors) > 0 {
		log.Printf("failed to execute graphql operation, errors: %v", res.Errors)
		return fmt.Errorf("%v", res.Errors)
	}
	rJSON, err := json.Marshal(r)
	fmt.Printf("%s \n", rJSON) // {"data":{"hello":"world"}}
	return err
}
