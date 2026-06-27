package orch

import (
	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/plano/compiler"
	"github.com/arcgolabs/plano/schema"
)

func orchActionSpecs() list.List[compiler.ActionSpec] {
	return compiler.ActionSpecs(
		compiler.ActionSpec{
			Name:     "http",
			MinArgs:  1,
			MaxArgs:  4,
			ArgTypes: schema.Types(schema.TypeInt, schema.TypeString, schema.TypeInt, schema.TypeString),
			Docs:     `Declare an HTTP endpoint: http(8080), http(8080, "admin"), or http(8080, "admin", 18080).`,
		},
		compiler.ActionSpec{
			Name:     "tcp",
			MinArgs:  1,
			MaxArgs:  4,
			ArgTypes: schema.Types(schema.TypeInt, schema.TypeString, schema.TypeInt, schema.TypeString),
			Docs:     `Declare a TCP endpoint: tcp(5432), tcp(5432, "postgres"), or tcp(5432, "postgres", 5432).`,
		},
		compiler.ActionSpec{
			Name:     "udp",
			MinArgs:  1,
			MaxArgs:  4,
			ArgTypes: schema.Types(schema.TypeInt, schema.TypeString, schema.TypeInt, schema.TypeString),
			Docs:     `Declare a UDP endpoint: udp(8125), udp(8125, "statsd"), or udp(8125, "statsd", 8125).`,
		},
		compiler.ActionSpec{
			Name:     "port",
			MinArgs:  2,
			MaxArgs:  5,
			ArgTypes: schema.Types(schema.TypeInt, schema.TypeString, schema.TypeString, schema.TypeInt, schema.TypeString),
			Docs:     `Declare an endpoint with protocol: port(5432, "tcp"), port(5432, "tcp", "postgres"), or port(5432, "tcp", "postgres", 5432).`,
		},
	)
}
