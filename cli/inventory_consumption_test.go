package cli_test

import (
	"context"
	"reflect"
	"testing"

	"kc/client"
	snapshotclient "kc/snapshot/client"
	"kc/snapshot/dataset"
)

func TestInventoryDatasetsHonorPublicDefaultsAndCurrentGrants(t *testing.T) {
	world := readExampleWorld(t)
	world.Datasets = []string{"store-oncall"}
	d := openExampleDeployment(t, world)
	d.publish(t, world, true)
	viewer, err := client.New(client.Config{BaseURL: d.url})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := viewer.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "agent:inventory-viewer"}}); err != nil {
		t.Fatal(err)
	}
	assertLists := func(want []string) {
		t.Helper()
		for _, list := range []func(context.Context, snapshotclient.RequestOptions, any) error{
			viewer.SnapshotService().Show, viewer.SnapshotService().Datasets,
		} {
			var result struct {
				Datasets []struct {
					ID       string            `json:"id"`
					Revision int               `json:"revision"`
					Metadata map[string]string `json:"metadata"`
				} `json:"datasets"`
			}
			if err := list(context.Background(), snapshotclient.RequestOptions{}, &result); err != nil {
				t.Fatal(err)
			}
			ids := make([]string, 0, len(result.Datasets))
			for _, row := range result.Datasets {
				ids = append(ids, row.ID)
			}
			if !reflect.DeepEqual(ids, want) {
				t.Fatalf("inventory did not honor current Dataset consumption: got=%v want=%v", ids, want)
			}
			for _, row := range result.Datasets {
				wantRevision := map[string]int{"qinghe-analysis": 3, "store-oncall": 3}[row.ID]
				if row.Revision != wantRevision || row.Metadata["title"] == "" {
					t.Fatalf("consumer inventory lost its current revision or purpose: %#v", row)
				}
			}
		}
	}
	assertLists([]string{"store-oncall"})
	// Source permission alone still cannot select another Dataset.
	sourceGrant := asMap(t, body(t, d.as(t, d.operator, "grant", "add", "--principal", "agent:inventory-viewer", "--repo", "sales-semantic", "--action", "file.read")))
	assertLists([]string{"store-oncall"})
	grant := asMap(t, body(t, d.as(t, d.operator, "grant", "add", "--principal", "agent:inventory-viewer", "--dataset", "qinghe-analysis", "--action", "dataset.resolve,file.read")))
	assertLists([]string{"qinghe-analysis", "store-oncall"})
	body(t, d.as(t, d.operator, "grant", "remove", "--id", grant["id"].(string)))
	assertLists([]string{"store-oncall"})
	expectCode(t, d.as(t, "agent:inventory-viewer", "files", "read", "--dataset", "qinghe-analysis", "--revision", "2", "--path", "metrics/shop.gmv/caliber.yaml"), "FORBIDDEN")
	body(t, d.as(t, d.operator, "grant", "remove", "--id", sourceGrant["id"].(string)))
	resolveGrant := asMap(t, body(t, d.as(t, d.operator, "grant", "add", "--principal", "agent:inventory-viewer", "--dataset", "qinghe-analysis", "--action", "dataset.resolve")))
	var pin dataset.ResolvedDataset
	if err := viewer.SnapshotService().ResolveDataset(context.Background(), "qinghe-analysis", snapshotclient.DatasetResolveRequest{}, snapshotclient.RequestOptions{}, &pin); err != nil {
		t.Fatal(err)
	}
	var memberGrants []string
	for repository, commit := range pin.Repositories {
		grant := asMap(t, body(t, d.as(t, d.operator, "grant", "add", "--principal", "agent:inventory-viewer", "--dataset", "qinghe-analysis", "--repo", string(repository), "--ref", string(commit), "--action", "file.read")))
		memberGrants = append(memberGrants, grant["id"].(string))
	}
	if len(memberGrants) < 2 {
		t.Fatalf("fixed-member fixture did not span multiple repositories: %#v", pin.Repositories)
	}
	assertLists([]string{"qinghe-analysis", "store-oncall"})
	control := asMap(t, body(t, d.as(t, d.operator, "files", "read", "--dataset", "qinghe-analysis", "--revision", "3", "--path", "metrics/shop.gmv/caliber.yaml")))
	read := asMap(t, body(t, d.as(t, "agent:inventory-viewer", "files", "read", "--dataset", "qinghe-analysis", "--revision", "3", "--path", "metrics/shop.gmv/caliber.yaml")))
	if control["content"] == "" || !reflect.DeepEqual(read, control) {
		t.Fatalf("listed fixed-member delivery differs from the accepted bytes and basis: read=%#v control=%#v", read, control)
	}
	for _, grant := range memberGrants {
		body(t, d.as(t, d.operator, "grant", "remove", "--id", grant))
		assertLists([]string{"store-oncall"})
	}
	body(t, d.as(t, d.operator, "grant", "remove", "--id", resolveGrant["id"].(string)))
	body(t, d.as(t, d.operator, "dataset", "retire", "store-oncall"))
	assertLists([]string{})
}
