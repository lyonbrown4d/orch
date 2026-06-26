package api

type DeployOperationInput struct {
	Namespace string `path:"namespace"`
	Name      string `path:"name"`
	Body      struct {
		TargetNode string   `json:"targetNode,omitempty"`
		Workloads  []string `json:"workloads,omitempty"`
	} `json:"body"`
}

type DeployOperationOutput struct {
	Body struct {
		Accepted   bool   `json:"accepted"`
		Operation  string `json:"operation"`
		App        string `json:"app"`
		Namespace  string `json:"namespace"`
		TargetNode string `json:"targetNode,omitempty"`
		Workloads  int    `json:"workloads"`
		Moved      int    `json:"moved"`
		Status     string `json:"status"`
	} `json:"body"`
}
