package fixture_test

import (
	"reflect"
	"testing"

	common "kc/discovery/plugin"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/liveaccess/fixture"
	"kc/snapshot"
)

func TestExplicitAdoptionPreservesIndexAndUnrelatedMetadata(t *testing.T) {
	fake := testkit.NewLakeFSFake(t)
	store, err := snapshot.OpenLakeFS("kr://fixture/liveaccess", fake.DSN(fake.NewRepo()), fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	metadata := store.(fixture.MetadataSource)
	if err := metadata.WriteRepositoryMetadata(map[string]string{common.RepositoryPluginsKey: `{"version":1,"plugins":[{"name":"declarative-index","instance":"selected"}]}`, "owner": "source"}); err != nil {
		t.Fatal(err)
	}
	if err := fixture.AdoptMetadata(metadata, "main"); err != nil {
		t.Fatal(err)
	}
	if err := fixture.AdoptMetadata(metadata, "main"); err != nil {
		t.Fatal(err)
	}
	properties, err := metadata.ReadRepositoryMetadata()
	if err != nil {
		t.Fatal(err)
	}
	config, err := common.ParseRepositoryPlugins(properties)
	if err != nil || properties["owner"] != "source" || !reflect.DeepEqual(config.Plugins, []common.PluginSelection{{Name: "declarative-index", Instance: "selected"}, {Name: "liveaccess", Instance: "main"}}) {
		t.Fatalf("explicit adoption changed prior choices: %#v %#v %v", properties, config, err)
	}
	if err := fixture.AdoptMetadata(metadata, "replacement"); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("helper overwrote another explicit selection: %v", err)
	}
}
