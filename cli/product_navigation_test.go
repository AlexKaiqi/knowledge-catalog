package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestProductBookNavigationMatchesSections(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "doc", "product.html"))
	if err != nil {
		t.Fatal(err)
	}
	navigation := regexp.MustCompile(`<a href="#([^"]+)"><span>([0-9]+)</span>`).FindAllSubmatch(raw, -1)
	sections := regexp.MustCompile(`<section id="([^"]+)">\s*<div class="heading"><span class="no">([0-9]+)</span>`).FindAllSubmatch(raw, -1)
	if len(sections) == 0 || len(navigation) != len(sections) {
		t.Fatalf("numbered navigation must cover every product section: links=%d sections=%d", len(navigation), len(sections))
	}
	for index, section := range sections {
		number := fmt.Sprintf("%02d", index+1)
		if string(section[2]) != number || string(navigation[index][1]) != string(section[1]) || string(navigation[index][2]) != number {
			t.Errorf("section %q must be numbered %s in both navigation and heading: navigation=%q heading=%q", section[1], number, navigation[index][0], section[0])
		}
	}
	anchors := make(map[string]bool)
	for _, match := range regexp.MustCompile(`\bid="([^"]+)"`).FindAllSubmatch(raw, -1) {
		id := string(match[1])
		if anchors[id] {
			t.Errorf("product anchor %q is repeated", id)
		}
		anchors[id] = true
	}
	for _, match := range regexp.MustCompile(`href="#([^"]+)"`).FindAllSubmatch(raw, -1) {
		if !anchors[string(match[1])] {
			t.Errorf("product link %q has no target", match[0])
		}
	}
}
