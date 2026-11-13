package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func assertURL(t *testing.T, input, want string, explicit, allowBare bool) {
	t.Helper()
	got, added := normalize(input, explicit, allowBare)
	if got != want {
		t.Fatalf("normalize(%q) = %q, want %q", input, got, want)
	}
	_ = added
}

func assertInvalid(t *testing.T, input string, explicit, allowBare bool) {
	t.Helper()
	if got, _ := normalize(input, explicit, allowBare); got != "" {
		t.Fatalf("normalize(%q) = %q, want invalid", input, got)
	}
}

func TestNormalizeExplicitSchemes(t *testing.T) {
	assertURL(t, "https://www.deepseek.com/", "https://www.deepseek.com/", false, true)
	assertURL(t, "HTTPS://EXAMPLE.ZIP/a?q=1", "https://EXAMPLE.ZIP/a?q=1", false, true)
	assertURL(t, "http://localhost:3000", "http://localhost:3000", false, true)
	assertURL(t, "http://192.168.1.1:8080/admin", "http://192.168.1.1:8080/admin", false, true)
	assertURL(t, "http://[::1]:8080/", "http://[::1]:8080/", false, true)
}

func TestNormalizeSchemeLess(t *testing.T) {
	assertURL(t, "www.deepseek.com", "https://www.deepseek.com", false, true)
	assertURL(t, "deepseek.com", "https://deepseek.com", false, true)
	assertURL(t, "github.io/foo", "https://github.io/foo", false, true)
	assertURL(t, "a.b.example.co.uk/x", "https://a.b.example.co.uk/x", false, true)
	assertURL(t, "localhost:3000", "http://localhost:3000", false, true)
	assertURL(t, "192.168.1.1", "http://192.168.1.1", false, true)
	assertURL(t, "example.zip", "", false, true)
	assertURL(t, "example.zip", "https://example.zip", true, true)
}

func TestNormalizeAuthorityAndPorts(t *testing.T) {
	assertURL(t, "https://a.com?x=1", "https://a.com?x=1", false, true)
	assertURL(t, "https://a.com#top", "https://a.com#top", false, true)
	assertURL(t, "https://a.com:8080?x", "https://a.com:8080?x", false, true)
	assertURL(t, "https://a.com:", "https://a.com:", false, true)
	for _, bad := range []string{"https://a.com:abc", "https://a.com:65536", "https://a.com:80:80", "https://:80", "http://[::1]x", "http://[::1]:abc", "https://.com", "https://a.com."} {
		assertInvalid(t, bad, false, true)
	}
}

func TestNormalizeRejectsUnsafeOrInvalid(t *testing.T) {
	for _, bad := range []string{
		"hello world", "https://", "https:// deepseek.com", "ftp://example.com", "file:///C:/Windows",
		"javascript:alert(1)", "https://user:pw@example.com", "https://example.com:0", "https://example.com:99999",
		"https://-bad-.com", "https://a..com", "https://exa_mple.com", "https://999.999.999.999",
		"http://01.2.3.4", "http://1.2.3.256", "http://1.2.3.4.5", "http://[12345::1]", "http://[::1", strings.Repeat("x", 5000),
	} {
		assertInvalid(t, bad, false, true)
	}
}

func TestNormalizeBareDomainSetting(t *testing.T) {
	assertInvalid(t, "deepseek.com", false, false)
	assertURL(t, "www.deepseek.com", "https://www.deepseek.com", false, false)
	assertURL(t, "https://deepseek.com", "https://deepseek.com", false, false)
	assertURL(t, "localhost:3000", "http://localhost:3000", false, false)
}

func TestNormalizeIDN(t *testing.T) {
	got, _ := normalize("https://中文.com/路径", false, true)
	if got != "https://xn--fiq228c.com/路径" {
		t.Fatalf("IDN normalize = %q", got)
	}
	got, _ = normalize("中文.com", true, true)
	if got != "https://xn--fiq228c.com" {
		t.Fatalf("bare IDN normalize = %q", got)
	}
}

func TestNormalizeIDNExtraVectors(t *testing.T) {
	assertURL(t, "https://münchen.de/", "https://xn--mnchen-3ya.de/", false, true)
	assertURL(t, "https://bücher.de/path?q=1", "https://xn--bcher-kva.de/path?q=1", false, true)
	assertURL(t, "https://例え.jp/", "https://xn--r8jz45g.jp/", false, true)
	assertURL(t, "HTTPS://MÜNCHEN.DE/", "https://xn--mnchen-3ya.de/", false, true)
}

func TestBuildItems(t *testing.T) {
	rows := buildItems("regex", "", "deepseek.com", nil)
	if len(rows) != 1 || rows[0].Open["url"] != "https://deepseek.com" || rows[0].Score != 95 || rows[0].Copy != "https://deepseek.com" {
		t.Fatalf("unexpected regex result: %#v", rows)
	}
	if rows := buildItems("regex", "", "main.py", nil); len(rows) != 0 {
		t.Fatalf("invalid regex input must return no results: %#v", rows)
	}
	rows = buildItems("keyword", "hello world", "", nil)
	if len(rows) != 1 || rows[0].Valid == nil || *rows[0].Valid {
		t.Fatalf("keyword invalid should return hint: %#v", rows)
	}
	rows = buildItems("keyword", "example.zip", "", nil)
	if len(rows) != 1 || rows[0].Open["url"] != "https://example.zip" {
		t.Fatalf("explicit keyword should accept uncommon TLD: %#v", rows)
	}
	rows = buildItems("regex", "", "deepseek.com", map[string]any{"bareDomains": false})
	if len(rows) != 0 {
		t.Fatalf("setting should disable bare domain trigger: %#v", rows)
	}
}

func TestManifestRegexGate(t *testing.T) {
	b, err := os.ReadFile("plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Commands []struct {
			Triggers []struct {
				Regex string `json:"regex"`
			} `json:"triggers"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Commands) == 0 || len(m.Commands[0].Triggers) == 0 {
		t.Fatal("missing trigger regex")
	}
	rx, err := regexp.Compile(m.Commands[0].Triggers[0].Regex)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"https://www.deepseek.com/", "http://a.b", "HTTPS://X.com", "www.deepseek.com", "deepseek.com", "a.cn", "localhost", "localhost:3000/x", "192.168.0.1:80", "github.io/foo?bar=1#x", "https://中文.com", "www.example.zip"} {
		if !rx.MatchString(input) {
			t.Errorf("trigger should match %q", input)
		}
	}
	for _, input := range []string{"notepad", "hello world", "deepseek com", "报告 2026", "C:\\Users", "1.5", "", "ab", "main.py", "readme.md", "run.sh", "archive.zip", "report.pdf"} {
		if rx.MatchString(input) {
			t.Errorf("trigger should ignore %q", input)
		}
	}
}

func TestProtocolSmokeWithBuiltBinary(t *testing.T) {
	// The test is opt-in because it builds a helper binary. CI/build.ps1 also
	// runs the same protocol smoke checks after the final Windows build.
	if os.Getenv("ME_OPEN_URL_SMOKE") != "1" {
		t.Skip("set ME_OPEN_URL_SMOKE=1 to run process smoke test")
	}
	exe := os.Getenv("ME_OPEN_URL_BINARY")
	if exe == "" {
		t.Fatal("ME_OPEN_URL_BINARY is required")
	}
	cmd := exec.Command(filepath.Clean(exe))
	cmd.Env = append(os.Environ(), "WS_TRIGGER=regex", "WS_TEXT=deepseek.com", "WS_SETTINGS={\"bareDomains\":true}")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var got reply
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("invalid JSON: %v; output=%s", err, out)
	}
	if len(got.Items) != 1 || got.Items[0].Title != "https://deepseek.com" {
		t.Fatalf("unexpected protocol output: %#v", got.Items)
	}
}
