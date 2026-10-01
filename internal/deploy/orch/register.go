package orch

import (
	"github.com/arcgolabs/plano/compiler"
	"github.com/arcgolabs/plano/schema"

	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

// Register attaches the orch deploy DSL form graph to the compiler.
func Register(c *compiler.Compiler) error {
	if c == nil {
		return nil
	}
	if err := c.RegisterAll(orchFormSpecs()...); err != nil {
		return oopsx.B("deploy", "orch").Wrapf(err, "register orch forms")
	}
	if err := c.RegisterAll(orchActionSpecs()...); err != nil {
		return oopsx.B("deploy", "orch").Wrapf(err, "register orch actions")
	}
	return nil
}

func orchFormSpecs() []schema.FormSpec {
	forms := make([]schema.FormSpec, 0, 32)
	forms = append(forms, appFormSpecs()...)
	forms = append(forms, workloadFormSpecs()...)
	forms = append(forms, runtimeOptionFormSpecs()...)
	forms = append(forms, objectFormSpecs()...)
	return forms
}
