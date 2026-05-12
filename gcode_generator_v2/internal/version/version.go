// Package version exposes the build-time version string and a helper for
// checking GitHub for newer releases.
//
// Version is injected at build time by the build scripts via:
//
//	go build -ldflags="-X gcodegen.local/generator/internal/version.Version=$(git describe --tags --always)"
//
// When run from `go run` (no -ldflags), Version stays "dev" and the update
// check is skipped — there's nothing meaningful to compare against.
package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Version is the build version, set at link time via -ldflags. "dev" by
// default for unversioned local builds.
var Version = "dev"

// repoLatestURL is the public GitHub Releases API endpoint. No auth needed
// for public repos. Rate limited to 60/hr per IP unauthenticated, which is
// far above any reasonable launch frequency.
const repoLatestURL = "https://api.github.com/repos/willfreyman/GCodeGen/releases/latest"

// LatestRelease fetches the latest released tag name from GitHub. Returns
// the tag (e.g. "v3.0.2") or an error. Times out after 5 seconds so an
// offline / slow network doesn't make the app feel laggy on startup.
func LatestRelease() (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, repoLatestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "GcodeGenV1/"+Version)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api returned %s", resp.Status)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return release.TagName, nil
}

// IsDev reports whether this binary was built without a version tag
// (i.e. `go run` or a build that didn't set the -X ldflag). We skip the
// update check for dev builds — there's no sensible "current" to compare.
func IsDev() bool {
	return Version == "" || Version == "dev"
}
