// Package buildinfo expõe a versão do binário em execução (GET /version),
// para responder "o que está no ar" perguntando ao próprio serviço.
package buildinfo

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"
)

// Version, Commit, and BuildTime are set at image build time via
// `-ldflags "-X ..."` (see /Dockerfile). Left at these defaults for
// anything not built that way (`go run .`, `go test`) — a local run
// should honestly report "dev", never claim to be a prior real release.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// changelog.txt is overwritten with the deployed commit's own message
// right before compiling in the Docker build — go:embed instead of
// another -ldflags -X because a commit message contains spaces and
// go build's -ldflags string is whitespace-split. Checked in as a
// placeholder so `go run` / `go test` always have a file to embed.
//
//go:embed changelog.txt
var embeddedChangelog string

// Info is the JSON shape served at GET /version.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Changelog string `json:"changelog"`
	BuildTime string `json:"build_time"`
}

func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		Changelog: strings.TrimSpace(embeddedChangelog),
		BuildTime: BuildTime,
	}
}

// Handler serves Get() as JSON — unauthenticated like /healthz and
// /metrics: nothing here is user data, and a deploy check shouldn't need
// a token.
func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Get())
	}
}
