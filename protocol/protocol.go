package protocol

type ExecutionMetadata struct {
	Cmd        []string `json:"cmd,omitempty"`
	Entrypoint []string `json:"entrypoint,omitempty"`
	Workdir    string   `json:"workdir,omitempty"`
}

type DockerImageMetadata struct {
	ExecutionMetadata ExecutionMetadata
	DockerImage       string
}
