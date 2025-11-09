package ui

// KubectlVerb identifies the parsed kubectl-compatible command.
type KubectlVerb string

const (
	// KubectlVerbNone indicates no CLI redirection was requested.
	KubectlVerbNone KubectlVerb = ""
	// KubectlVerbGet tracks kubectl get invocations.
	KubectlVerbGet KubectlVerb = "get"
	// KubectlVerbLogs tracks kubectl logs invocations.
	KubectlVerbLogs KubectlVerb = "logs"
)

// StartupIntent captures CLI-driven startup requests (kubectl parity flows).
type StartupIntent struct {
	Verb      KubectlVerb
	Namespace string
	Get       *GetIntent
	Logs      *LogsIntent
}

// GetIntent stores the tokenized request for kubectl get.
type GetIntent struct {
	OutputFormat string
	Tokens       []GetToken
}

// GetToken preserves the raw sequence of get arguments for later resolution.
type GetToken struct {
	Value            string
	Original         string
	Resource         string
	Name             string
	ExplicitResource bool
	ExplicitName     bool
	FromComma        bool
	FromSlash        bool
}

// LogsIntent holds kubectl logs parameters.
type LogsIntent struct {
	Pod       string
	Container string
	Follow    bool
}
