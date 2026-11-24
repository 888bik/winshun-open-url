package main

import (
	"encoding/json"
	"fmt"
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

func TestExtractURLs(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"看看这个 https://github.com/Thysrael/Horizon 很不错", []string{"https://github.com/Thysrael/Horizon"}},
		{"参考这两个：https://a.com 和 https://b.com/x?y=1。", []string{"https://a.com", "https://b.com/x?y=1"}},
		{"muse.ai", []string{"https://muse.ai"}},
		{"今天天气不错", nil},
		{"链接是 https://example.com/path。", []string{"https://example.com/path"}},
		{"https://a.com 和 https://a.com", []string{"https://a.com"}},
		{"发我邮箱 a@b.com", nil},
		{"本地服务 http://localhost:3000/api 看一下", []string{"http://localhost:3000/api"}},
		{"文档见 [Horizon](https://github.com/Thysrael/Horizon) 这一篇", []string{"https://github.com/Thysrael/Horizon"}},
		{"www.deepseek.com 也可以", []string{"https://www.deepseek.com"}},
	}
	for _, c := range cases {
		got := extractURLs(c.input)
		if len(got) != len(c.want) {
			t.Fatalf("extractURLs(%q) = %q, want %q", c.input, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("extractURLs(%q)[%d] = %q, want %q", c.input, i, got[i], c.want[i])
			}
		}
	}
}

func TestExtractURLsIPv6AndTen(t *testing.T) {
	if got := extractURLs("本机服务 http://[::1]:8080/ 测试"); len(got) != 1 || got[0] != "http://[::1]:8080/" {
		t.Fatalf("IPv6 extract = %q", got)
	}
	var many []string
	for i := 0; i < 11; i++ {
		many = append(many, fmt.Sprintf("https://site%d.com", i))
	}
	input := strings.Join(many, " ")
	if got := extractURLs(input); len(got) != 10 || got[9] != "https://site9.com" {
		t.Fatalf("want at most 10 URLs, got %d", len(got))
	}
}

func TestBuildItemsMultiple(t *testing.T) {
	rows := buildItems("regex", "", "看看 https://a.com 和 https://b.com/x。", nil)
	if len(rows) != 2 {
		t.Fatalf("want 2 items, got %d: %#v", len(rows), rows)
	}
	if rows[0].ID != "open-url-1" || rows[1].ID != "open-url-2" {
		t.Fatalf("ids = %q, %q", rows[0].ID, rows[1].ID)
	}
	if rows[0].Score != 95 || rows[1].Score != 94 {
		t.Fatalf("scores = %d, %d", rows[0].Score, rows[1].Score)
	}
	if rows[1].Title != "https://b.com/x" || rows[1].Copy != "https://b.com/x" {
		t.Fatalf("row 1 = %#v", rows[1])
	}
	if rows := buildItems("regex", "", "今天天气不错", nil); len(rows) != 0 {
		t.Fatalf("plain text regex must return no results: %#v", rows)
	}
	if rows := buildItems("keyword", "hello world", "", nil); len(rows) != 1 || rows[0].Valid == nil || *rows[0].Valid {
		t.Fatalf("keyword mode keeps hint rows: %#v", rows)
	}
	if rows := buildItems("keyword", "看这个 https://a.com 和 muse.ai", "", nil); len(rows) != 2 || rows[0].Title != "https://a.com" || rows[1].Title != "https://muse.ai" {
		t.Fatalf("keyword mode extracts from whole query: %#v", rows)
	}
	if rows := buildItems("regex", "", "看看 deepseek.com", map[string]any{"bareDomains": false}); len(rows) != 0 {
		t.Fatalf("bareDomains off must suppress bare domains: %#v", rows)
	}
	if rows := buildItems("regex", "", "看看 www.deepseek.com", map[string]any{"bareDomains": false}); len(rows) != 1 || rows[0].Title != "https://www.deepseek.com" {
		t.Fatalf("bareDomains off must keep www. URLs: %#v", rows)
	}
}

func TestSiteIcons(t *testing.T) {
	// 网址结果的 Icon 必须是 "site:" + 该条归一化后的完整网址。
	rows := buildItems("regex", "", "muse.com", nil)
	if len(rows) != 1 || rows[0].Icon != "site:"+rows[0].Title {
		t.Fatalf("single result icon = %q, want site:%q", rows[0].Icon, rows[0].Title)
	}
	// 多网址时每条的 Icon 各对各的网址。
	rows = buildItems("regex", "", "对比 https://a.com 和 https://b.com/x 和 www.deepseek.com", nil)
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	for _, r := range rows {
		if r.Icon != "site:"+r.Title {
			t.Fatalf("row %s icon = %q, want site:%q", r.Title, r.Icon, r.Title)
		}
	}
	if rows[0].Icon == rows[1].Icon {
		t.Fatalf("distinct rows must carry distinct icons: %q", rows[0].Icon)
	}
	// keyword 模式的 hint 行保持地球 glyph 不变。
	rows = buildItems("keyword", "hello world", "", nil)
	if len(rows) != 1 || rows[0].Icon != icon {
		t.Fatalf("keyword hint icon = %q, want %q", rows[0].Icon, icon)
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
	for _, input := range []string{"https://www.deepseek.com/", "http://a.b", "HTTPS://X.com", "www.deepseek.com", "deepseek.com", "a.cn", "localhost", "localhost:3000/x", "192.168.0.1:80", "github.io/foo?bar=1#x", "https://中文.com", "www.example.zip",
		"看看这个 https://github.com/Thysrael/Horizon 很不错", "参考这两个：https://a.com 和 https://b.com/x?y=1。", "muse.ai", "链接是 https://example.com/path。", "发我邮箱 a@b.com", "本地服务 http://localhost:3000/api 看一下", "文档见 [Horizon](https://github.com/Thysrael/Horizon) 这一篇", "www.deepseek.com 也可以"} {
		if !rx.MatchString(input) {
			t.Errorf("trigger should match %q", input)
		}
	}
	for _, input := range []string{"notepad", "hello world", "deepseek com", "报告 2026", "C:\\Users", "1.5", "", "ab", "main.py", "readme.md", "run.sh", "archive.zip", "report.pdf", "今天天气不错"} {
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
