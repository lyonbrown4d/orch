package api

import (
	"github.com/arcgolabs/collectionx/list"

	deployv1 "github.com/lyonbrown4d/orch/internal/deploy/v1alpha1"
)

type AppRevisionItem struct {
	Generation string            `json:"generation"`
	Metadata   deployv1.Metadata `json:"metadata"`
	Workloads  int               `json:"workloads"`
	App        deployv1.App      `json:"app"`
}

type ListAppRevisionsInput struct {
	Namespace string `path:"namespace"`
	Name      string `path:"name"`
}

type ListAppRevisionsOutput struct {
	Body struct {
		Items *list.List[AppRevisionItem] `json:"items"`
	} `json:"body"`
}
