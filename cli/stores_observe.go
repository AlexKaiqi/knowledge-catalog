package cli

import (
	"net/url"
	"os"
	"strings"
)

type storeBinding struct {
	ID, Driver, DSN string
}

// verbObserveStores is the console/HTTP observation of ⓪ Snapshot and ③
// retrieval bindings. It is not store-ls: layout directories and secret
// environment names stay off the public surface (STORE_ADAPTERS secrets rule).
func verbObserveStores(cx *invocation) (any, error) {
	var catalogs, repos []storeBinding
	if cx.WS != nil {
		if cx.WS.Deployment != nil {
			for _, item := range cx.WS.Deployment.Catalogs {
				catalogs = append(catalogs, storeBinding{item.ID, item.Driver, item.DSN})
			}
			for _, item := range cx.WS.Deployment.Repositories {
				repos = append(repos, storeBinding{item.ID, item.Driver, item.DSN})
			}
		}
		return observePublicStores(cx.WS.Stores, cx.WS.File, catalogs, repos), nil
	}
	if !homeReady(cx.Home) {
		return nil, missingHome(cx.Home)
	}
	stores, err := ReadStores(cx.Home)
	if err != nil {
		return nil, err
	}
	file, err := ReadHome(cx.Home)
	if err != nil {
		return nil, err
	}
	return observePublicStores(stores, file, nil, nil), nil
}

func observePublicStores(stores StoresFile, file HomeFile, catalogs, repos []storeBinding) map[string]any {
	seen := map[string]bool{}
	authorities := make([]map[string]any, 0)
	add := func(role, id, driver, dsn string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		driver = strings.TrimSpace(driver)
		if driver == "" {
			driver = stores.Repository
		}
		item := map[string]any{"id": id, "role": role, "driver": driver}
		if origin, name := publicHTTPEntry(dsn); origin != "" {
			origin = publishedOrigin(origin, os.Getenv("KC_LAKEFS_URL"), os.Getenv("KC_LAKEFS_PUBLIC_URL"), "lakefs")
			item["origin"] = origin
			if name != "" {
				item["name"] = name
			}
		}
		authorities = append(authorities, item)
	}
	for _, item := range catalogs {
		add("catalog", item.ID, item.Driver, item.DSN)
	}
	for _, item := range file.Catalogs {
		add("catalog", item.ID, stores.Repository, "")
	}
	for _, item := range repos {
		add("repository", item.ID, item.Driver, item.DSN)
	}
	for _, item := range file.Repos {
		add("repository", item.ID, item.Driver, item.DSN)
	}
	indexDriver := NormalizeIndexDriver(stores.Index)
	retrieval := map[string]any{"driver": indexDriver}
	if indexDriver == "opensearch" {
		if origin, _ := publicHTTPEntry(stores.OpenSearch.URL); origin != "" {
			retrieval["origin"] = publishedOrigin(origin, os.Getenv("KC_OPENSEARCH_URL"), os.Getenv("KC_OPENSEARCH_PUBLIC_URL"), "opensearch")
		}
	}
	driver := strings.TrimSpace(stores.Repository)
	for _, item := range authorities {
		if item["role"] != "catalog" {
			continue
		}
		if catalogDriver, _ := item["driver"].(string); strings.TrimSpace(catalogDriver) != "" {
			driver = catalogDriver
		}
		break
	}
	snapshot := map[string]any{"driver": driver, "authorities": authorities}
	if profile := strings.TrimSpace(stores.Profile); profile != "" {
		snapshot["profile"] = profile
	}
	return map[string]any{"snapshot": snapshot, "retrieval": retrieval}
}

// publicHTTPEntry returns a browser-openable origin and optional last path
// name. Userinfo, query, and fragment are dropped; a DSN that already carries
// secrets is omitted rather than redacted in place.
func publicHTTPEntry(raw string) (origin, name string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 && parts[len(parts)-1] != "" {
		name = parts[len(parts)-1]
		prefix := parts[:len(parts)-1]
		path := ""
		if len(prefix) > 0 {
			path = "/" + strings.Join(prefix, "/")
		}
		return u.Scheme + "://" + u.Host + path, name
	}
	return u.Scheme + "://" + u.Host, ""
}

// publishedOrigin rewrites a cluster-internal HTTP origin (lakefs:8000,
// opensearch:9200) to the host-facing URL operators actually open. The server
// keeps using the internal DSN; only observation changes.
func publishedOrigin(origin, internalURL, publicURL, defaultHost string) string {
	origin = strings.TrimSpace(origin)
	publicURL = strings.TrimSpace(publicURL)
	if origin == "" || publicURL == "" {
		return origin
	}
	pub, err := url.Parse(publicURL)
	if err != nil || pub.Host == "" || pub.User != nil || pub.RawQuery != "" || pub.Fragment != "" || (pub.Scheme != "http" && pub.Scheme != "https") {
		return origin
	}
	cur, err := url.Parse(origin)
	if err != nil || cur.Hostname() == "" {
		return origin
	}
	match := hostnameOf(internalURL)
	if match == "" {
		match = strings.TrimSpace(defaultHost)
	}
	if match == "" || cur.Hostname() != match {
		return origin
	}
	return pub.Scheme + "://" + pub.Host + cur.EscapedPath()
}

func hostnameOf(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
