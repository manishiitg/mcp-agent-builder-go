package server

import (
	"reflect"
	"testing"

	"github.com/manishiitg/mcpagent/mcpclient"
)

func TestRuntimeMCPServersDropsLegacyCustomToolCategories(t *testing.T) {
	got := runtimeMCPServers([]string{"workspace_advanced", "gmail", "workspace_browser"})
	if want := []string{"gmail"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runtimeMCPServers() = %#v, want %#v", got, want)
	}
}

func TestRuntimeMCPServersUsesNoServersWhenOnlyLegacyCategoriesRemain(t *testing.T) {
	got := runtimeMCPServers([]string{"workspace_advanced", "workspace_browser"})
	if want := []string{mcpclient.NoServers}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runtimeMCPServers() = %#v, want %#v", got, want)
	}
}
