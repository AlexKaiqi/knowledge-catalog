package service

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"kc/snapshot/access"
)

type nativeRoute struct {
	method  string
	path    string
	action  string
	refs    []string
	pattern *regexp.Regexp
}

func nativeOperation(method, path, action string, refs ...string) nativeRoute {
	pattern := regexp.MustCompile(`\{([^}]+)\}`).ReplaceAllString(path, `(?P<$1>[^/]+)`)
	return nativeRoute{method, path, action, refs, regexp.MustCompile("^" + pattern + "$")}
}

// Every retained native repository operation has an explicit product action.
// A newly introduced native endpoint cannot fall through to an unguarded proxy.
var nativeRepositoryRoutes = []nativeRoute{
	nativeOperation("GET", "/repositories", access.RepositoryMetadataRead),
	nativeOperation("POST", "/repositories", access.RepositoryCreate),
	nativeOperation("GET", "/repositories/{repository}", access.RepositoryMetadataRead),
	nativeOperation("DELETE", "/repositories/{repository}", access.RepositoryDelete),
	nativeOperation("GET", "/repositories/{repository}/metadata", access.RepositoryMetadataRead),
	nativeOperation("POST", "/repositories/{repository}/metadata", access.RepositoryMetadataWrite),
	nativeOperation("DELETE", "/repositories/{repository}/metadata", access.RepositoryMetadataWrite),
	nativeOperation("GET", "/repositories/{repository}/branches", access.RepositoryHistoryRead),
	nativeOperation("POST", "/repositories/{repository}/branches", access.RepositoryRefsManage),
	nativeOperation("GET", "/repositories/{repository}/branches/{branch}", access.RepositoryHistoryRead, "branch"),
	nativeOperation("DELETE", "/repositories/{repository}/branches/{branch}", access.RepositoryRefsManage, "branch"),
	nativeOperation("PUT", "/repositories/{repository}/branches/{branch}", access.RepositoryRefsManage, "branch"),
	nativeOperation("POST", "/repositories/{repository}/branches/{branch}/revert", access.RepositoryMerge, "branch"),
	nativeOperation("GET", "/repositories/{repository}/tags", access.RepositoryHistoryRead),
	nativeOperation("POST", "/repositories/{repository}/tags", access.RepositoryRefsManage),
	nativeOperation("GET", "/repositories/{repository}/tags/{tag}", access.RepositoryHistoryRead, "tag"),
	nativeOperation("DELETE", "/repositories/{repository}/tags/{tag}", access.RepositoryRefsManage, "tag"),
	nativeOperation("GET", "/repositories/{repository}/refs/{ref}/objects/ls", access.RepositoryFileRead, "ref"),
	nativeOperation("GET", "/repositories/{repository}/refs/{ref}/objects/stat", access.RepositoryFileRead, "ref"),
	nativeOperation("HEAD", "/repositories/{repository}/refs/{ref}/objects", access.RepositoryFileRead, "ref"),
	nativeOperation("GET", "/repositories/{repository}/refs/{ref}/objects", access.RepositoryFileRead, "ref"),
	nativeOperation("POST", "/repositories/{repository}/branches/{branch}/objects", access.RepositoryFilesWrite, "branch"),
	nativeOperation("DELETE", "/repositories/{repository}/branches/{branch}/objects", access.RepositoryFilesWrite, "branch"),
	nativeOperation("POST", "/repositories/{repository}/branches/{branch}/objects/delete", access.RepositoryFilesWrite, "branch"),
	nativeOperation("POST", "/repositories/{repository}/branches/{branch}/objects/copy", access.RepositoryFilesWrite, "branch"),
	nativeOperation("GET", "/repositories/{repository}/branches/{branch}/objects/stage_allowed", access.RepositoryFilesWrite, "branch"),
	nativeOperation("GET", "/repositories/{repository}/refs/{ref}/commits", access.RepositoryHistoryRead, "ref"),
	nativeOperation("GET", "/repositories/{repository}/commits/{commit}", access.RepositoryHistoryRead, "commit"),
	nativeOperation("POST", "/repositories/{repository}/branches/{branch}/commits", access.RepositoryCommit, "branch"),
	nativeOperation("GET", "/repositories/{repository}/branches/{branch}/diff", access.RepositoryHistoryRead, "branch"),
	nativeOperation("GET", "/repositories/{repository}/refs/{left}/diff/{right}", access.RepositoryHistoryRead, "left", "right"),
	nativeOperation("POST", "/repositories/{repository}/refs/{source}/merge/{destination}", access.RepositoryMerge, "source", "destination"),
	nativeOperation("GET", "/repositories/{repository}/actions/runs", access.RepositoryActionsRead),
	nativeOperation("GET", "/repositories/{repository}/actions/runs/{run}", access.RepositoryActionsRead),
	nativeOperation("GET", "/repositories/{repository}/actions/runs/{run}/hooks", access.RepositoryActionsRead),
	nativeOperation("GET", "/repositories/{repository}/actions/runs/{run}/hooks/{hook}/output", access.RepositoryActionsRead),
	nativeOperation("GET", "/repositories/{repository}/pulls", access.RepositoryPullsRead),
	nativeOperation("GET", "/repositories/{repository}/pulls/{pull}", access.RepositoryPullsRead),
	nativeOperation("POST", "/repositories/{repository}/pulls", access.RepositoryPullsManage),
	nativeOperation("PATCH", "/repositories/{repository}/pulls/{pull}", access.RepositoryPullsManage),
	nativeOperation("PUT", "/repositories/{repository}/pulls/{pull}/merge", access.RepositoryMerge),
	nativeOperation("GET", "/repositories/{repository}/settings/gc_rules", access.RepositorySettingsRead),
	nativeOperation("PUT", "/repositories/{repository}/settings/gc_rules", access.RepositorySettingsManage),
	nativeOperation("DELETE", "/repositories/{repository}/settings/gc_rules", access.RepositorySettingsManage),
	nativeOperation("GET", "/repositories/{repository}/gc/rules/set_allowed", access.RepositorySettingsManage),
	nativeOperation("GET", "/repositories/{repository}/settings/branch_protection", access.RepositorySettingsRead),
	nativeOperation("PUT", "/repositories/{repository}/settings/branch_protection", access.RepositorySettingsManage),
	nativeOperation("GET", "/repositories/{repository}/branch_protection/set_allowed", access.RepositorySettingsManage),
	nativeOperation("GET", "/repositories/{repository}/branches/{branch}/import", access.RepositoryImport, "branch"),
	nativeOperation("POST", "/repositories/{repository}/branches/{branch}/import", access.RepositoryImport, "branch"),
	nativeOperation("DELETE", "/repositories/{repository}/branches/{branch}/import", access.RepositoryImport, "branch"),
	nativeOperation("GET", "/repositories/{repository}/branches/{branch}/staging/backing", access.RepositoryImport, "branch"),
	nativeOperation("PUT", "/repositories/{repository}/branches/{branch}/staging/backing", access.RepositoryImport, "branch"),
}

func matchNativeRoute(r *http.Request) (nativeRoute, map[string]string, bool) {
	path := strings.TrimPrefix(r.URL.EscapedPath(), "/api/v1")
	for _, route := range nativeRepositoryRoutes {
		method := r.Method
		if method == "HEAD" && route.method == "GET" {
			method = "GET"
		}
		if route.method != method {
			continue
		}
		match := route.pattern.FindStringSubmatch(path)
		if match == nil {
			continue
		}
		values := map[string]string{}
		for i, name := range route.pattern.SubexpNames() {
			if i > 0 && name != "" {
				value, err := url.PathUnescape(match[i])
				if err != nil {
					return nativeRoute{}, nil, false
				}
				values[name] = value
			}
		}
		return route, values, true
	}
	return nativeRoute{}, nil, false
}
