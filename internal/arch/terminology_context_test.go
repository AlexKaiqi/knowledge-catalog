package arch_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// terminologyContext removes recognized uses of a name, never a whole file or
// directory. Split speech tokens require the brand in both the sentence and
// adjacent timing tokens; an unrelated occurrence in the same JSON stays visible.
func terminologyContext(text string) string {
	var value any
	if json.Unmarshal([]byte(text), &value) == nil {
		normalizeBrandTiming(value)
		if encoded, err := json.Marshal(value); err == nil {
			text = string(encoded)
		}
	}
	retired := "know" + "ledge"
	retiredZH := "知" + "识"
	for _, phrase := range []string{
		"ac" + retired, retired + "-catalog", retired + "_catalog", retired + " catalog",
		retiredZH + "底座", "zero-" + retired, "zero " + retired, "零" + retiredZH,
		"不能让观众误认为安装即获得全部" + retiredZH,
	} {
		text = strings.ReplaceAll(strings.ToLower(text), strings.ToLower(phrase), "")
	}
	return text
}

func normalizeBrandTiming(value any) {
	switch node := value.(type) {
	case []any:
		for _, child := range node {
			normalizeBrandTiming(child)
		}
	case map[string]any:
		retired := "know" + "ledge"
		brand := retired + " catalog"
		text, _ := node["text"].(string)
		spoken, _ := node["spoken_text"].(string)
		if strings.HasPrefix(strings.ToLower(text), brand) || strings.HasPrefix(strings.ToLower(spoken), brand) {
			for _, key := range []string{"words", "word_timestamps"} {
				words, _ := node[key].([]any)
				for i := 0; i+1 < len(words); i++ {
					first, _ := words[i].(map[string]any)
					second, _ := words[i+1].(map[string]any)
					word, _ := first["word"].(string)
					next, _ := second["word"].(string)
					if strings.EqualFold(word, retired) && strings.TrimRight(strings.ToLower(next), "。.,!！?？") == "catalog" {
						first["word"] = ""
						if i == 0 && strings.EqualFold(stringValue(node["first_word"]), retired) {
							node["first_word"] = ""
						}
					}
				}
			}
		}
		for _, child := range node {
			normalizeBrandTiming(child)
		}
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func TestTerminologyContextKeepsDomainViolationsVisible(t *testing.T) {
	retired := "know" + "ledge"
	retiredZH := "知" + "识"
	brand := retired + " Catalog"
	tests := []struct {
		name, text string
		violation  bool
	}{
		{"trial method", "零" + retiredZH + "试跑 zero-" + retired, false},
		{"brand filename", retired + "-catalog-handdrawn-pilot.mp4", false},
		{"quoted counterexample", "不能让观众误认为安装即获得全部" + retiredZH, false},
		{"actual claim", "安装即获得全部" + retiredZH, true},
		{"object type", "create " + retired + " object", true},
		{"method and domain", "zero-" + retired + " creates " + retired, true},
		{"brand and domain", brand + " stores " + retired, true},
		{"split brand", `{"text":"` + brand + `。","first_word":"` + retired + `","word_timestamps":[{"word":"` + retired + `"},{"word":"Catalog。"}]}`, false},
		{"unrelated JSON", `{"text":"` + brand + `。","object":"` + retired + `"}`, true},
		{"unproven timing", `{"text":"` + brand + `。","first_word":"` + retired + `"}`, true},
		{"nonbrand timing", `{"text":"hello","words":[{"word":"` + retired + `"},{"word":"Catalog。"}]}`, true},
		{"unpaired timing", `{"text":"` + brand + `。","words":[{"word":"` + retired + `"},{"word":"object"}]}`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			text := terminologyContext(tc.text)
			got := strings.Contains(text, retired) || strings.Contains(text, retiredZH)
			if got != tc.violation {
				t.Fatalf("violation = %t, want %t: %q", got, tc.violation, text)
			}
		})
	}
	for _, tc := range []struct {
		name, path, declaration string
		violation               bool
	}{
		{"registered wire constant", "home/managed_legacy_actions.go", "const managedLegacyActionPrefix = ", false},
		{"different file", "home/managed.go", "const managedLegacyActionPrefix = ", true},
		{"different constant", "home/managed_legacy_actions.go", "const publicActionPrefix = ", true},
		{"mutable variable", "home/managed_legacy_actions.go", "var managedLegacyActionPrefix = ", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "package home\n" + tc.declaration + `"` + retired + `."` + "\n"
			text = durableTerminologyContext(tc.path, text, retired)
			if strings.Contains(text, retired) != tc.violation {
				t.Fatalf("historical wire context changed an unregistered location: %s", text)
			}
			text += "const unrelated = " + `"` + retired + ` object"`
			if !strings.Contains(durableTerminologyContext(tc.path, text, retired), retired) {
				t.Fatal("historical wire context hid another retired term in the same file")
			}
		})
	}
}
