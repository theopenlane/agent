package constants

import (
	"runtime/debug"
	"strings"
	"text/template"
)

var (
	// AgentVersion is the version of the application. Note that this is
	// set at compile time using ldflags.
	AgentVersion = "no-info"
	// VerboseAgentVersion is the verbose version of the application.
	// Note that this is set up at init time.
	VerboseAgentVersion = ""
)

type versionStruct struct {
	Version   string
	GoVersion string
	Time      string
	Commit    string
	OS        string
	Arch      string
	Modified  bool
}

const (
	verboseTemplate = `Version: {{.Version}}
Go Version: {{.GoVersion}}
Git Commit: {{.Commit}}
Commit Date: {{.Time}}
OS/Arch: {{.OS}}/{{.Arch}}
Dirty: {{.Modified}}`
)

func init() {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}

	var vvs versionStruct

	vvs.Version = AgentVersion
	vvs.GoVersion = bi.GoVersion

	for _, kv := range bi.Settings {
		switch kv.Key {
		case "vcs.time":
			vvs.Time = kv.Value
		case "vcs.revision":
			vvs.Commit = kv.Value
		case "vcs.modified":
			vvs.Modified = kv.Value == "true"
		case "GOOS":
			vvs.OS = kv.Value
		case "GOARCH":
			vvs.Arch = kv.Value
		}
	}

	VerboseAgentVersion = vvs.String()
}

func (vvs *versionStruct) String() string {
	stringBuilder := &strings.Builder{}
	tmpl := template.Must(template.New("version").Parse(verboseTemplate))

	err := tmpl.Execute(stringBuilder, vvs)
	if err != nil {
		panic(err)
	}

	return stringBuilder.String()
}

// FullVersion returns the agent version string
func FullVersion() string {
	return AgentVersion
}

// UserAgent returns a properly formatted user-agent string
func UserAgent() string {
	return "openlane-agent/" + AgentVersion
}
