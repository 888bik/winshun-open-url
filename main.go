// me.open-url — standalone Windows x64 once-plugin for WinShun.
// No Python, Node, Qt, third-party packages, network access, or child processes.
// The launcher supplies WS_TRIGGER / WS_QUERY / WS_TEXT / WS_SETTINGS and
// executes the URL itself from the returned WinShun item.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxLen = 2048
	icon   = "glyph:U+E774"
)

var commonTLDs = map[string]struct{}{}

func init() {
	for _, tld := range strings.Fields(`
		com net org edu gov mil int info biz name pro mobi
		io ai co me tv cc app dev xyz top site online tech store shop blog cloud
		cn hk tw mo jp kr sg my th vn in id ph
		uk de fr es it nl se no fi dk ch at be pl ru ua tr
		us ca au nz br mx ar za eu
	`) {
		commonTLDs[tld] = struct{}{}
	}
}

type item struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Subtitle string         `json:"subtitle,omitempty"`
	Icon     string         `json:"icon,omitempty"`
	Score    int            `json:"score,omitempty"`
	Open     map[string]any `json:"open,omitempty"`
	Copy     string         `json:"copy,omitempty"`
	Valid    *bool          `json:"valid,omitempty"`
}

type reply struct {
	Items []item `json:"items"`
}

func boolPtr(v bool) *bool { return &v }

func isLabelRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-'
}

func labelOK(label string) bool {
	runes := []rune(label)
	if len(runes) < 1 || len(runes) > 63 || runes[0] == '-' || runes[len(runes)-1] == '-' {
		return false
	}
	for _, r := range runes {
		if !isLabelRune(r) {
			return false
		}
	}
	return true
}

func isIPv4(host string) bool {
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 3 {
			return false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
		}
		if len(p) > 1 && p[0] == '0' {
			return false
		}
		n, err := strconv.Atoi(p)
		if err != nil || n > 255 {
			return false
		}
	}
	return true
}

func isIPv6(host string) bool {
	return strings.Contains(host, ":") && net.ParseIP(host) != nil
}

func isIP(host string) bool { return isIPv4(host) || isIPv6(host) }

func isDomain(host string) bool {
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !labelOK(label) {
			return false
		}
	}
	tld := labels[len(labels)-1]
	if strings.HasPrefix(tld, "xn--") {
		return true
	}
	letters := true
	for _, r := range tld {
		if !unicode.IsLetter(r) {
			letters = false
			break
		}
	}
	return letters && utf8.RuneCountInString(tld) >= 2
}

// RFC 3492 Punycode encoder. This is sufficient for the IDN hostname handling
// required here; path/query text remains untouched for the browser.
func punycodeLabel(input string) (string, error) {
	const (
		base        int64 = 36
		tMin        int64 = 1
		tMax        int64 = 26
		skew        int64 = 38
		damp        int64 = 700
		initialBias int64 = 72
		initialN    int64 = 128
	)
	adapt := func(delta, numPoints int64, first bool) int64 {
		if first {
			delta /= damp
		} else {
			delta /= 2
		}
		delta += delta / numPoints
		k := int64(0)
		for delta > ((base-tMin)*tMax)/2 {
			delta /= base - tMin
			k += base
		}
		return k + (base-tMin+1)*delta/(delta+skew)
	}
	digit := func(d int64) byte {
		if d < 26 {
			return byte('a' + d)
		}
		return byte('0' + (d - 26))
	}

	runes := []rune(input)
	var out strings.Builder
	basic := int64(0)
	for _, r := range runes {
		if r < 0x80 {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return "", errors.New("invalid ASCII character in IDN label")
			}
			out.WriteByte(byte(r))
			basic++
		}
	}
	b := basic
	h := basic
	if basic > 0 && basic < int64(len(runes)) {
		out.WriteByte('-')
	}
	n := initialN
	delta := int64(0)
	bias := initialBias
	for h < int64(len(runes)) {
		m := int64(0x10ffff)
		for _, r := range runes {
			rv := int64(r)
			if rv >= n && rv < m {
				m = rv
			}
		}
		if m == 0x10ffff || m-n > (1<<60)/(h+1) {
			return "", errors.New("punycode overflow")
		}
		delta += (m - n) * (h + 1)
		n = m
		for _, r := range runes {
			rv := int64(r)
			if rv < n {
				delta++
			}
			if rv != n {
				continue
			}
			q := delta
			for k := base; ; k += base {
				t := tMin
				if k > bias+tMin {
					t = k - bias
					if t > tMax {
						t = tMax
					}
				}
				if q < t {
					break
				}
				code := t + (q-t)%(base-t)
				out.WriteByte(digit(code))
				q = (q - t) / (base - t)
			}
			out.WriteByte(digit(q))
			bias = adapt(delta, h+1, h == b)
			delta = 0
			h++
		}
		delta++
		n++
	}
	encoded := "xn--" + out.String()
	if len(encoded) > 63 {
		return "", errors.New("IDN label is too long")
	}
	return encoded, nil
}

func asciiHost(host string) (string, error) {
	labels := strings.Split(host, ".")
	for i, label := range labels {
		if !utf8.ValidString(label) {
			return "", errors.New("invalid UTF-8 hostname")
		}
		ascii := true
		for _, r := range label {
			if r >= 0x80 {
				ascii = false
				break
			}
		}
		if !ascii {
			encoded, err := punycodeLabel(label)
			if err != nil {
				return "", err
			}
			labels[i] = encoded
		} else if len(label) > 63 {
			return "", errors.New("hostname label is too long")
		}
	}
	return strings.Join(labels, "."), nil
}

// splitAuthority returns the original netloc, lower-case host, optional port,
// and the path/query/fragment tail. An empty explicit port is preserved for
// compatibility with the original Python plugin.
func splitAuthority(rest string) (netloc, host string, port *int, tail string, ok bool) {
	end := len(rest)
	for _, marker := range []byte{'/', '?', '#'} {
		if i := strings.IndexByte(rest, marker); i >= 0 && i < end {
			end = i
		}
	}
	netloc, tail = rest[:end], rest[end:]
	if netloc == "" || strings.Contains(netloc, "@") {
		return "", "", nil, "", false
	}
	var hostPart, portText string
	if strings.HasPrefix(netloc, "[") {
		close := strings.IndexByte(netloc, ']')
		if close < 0 {
			return "", "", nil, "", false
		}
		hostPart = netloc[1:close]
		after := netloc[close+1:]
		if after != "" && !strings.HasPrefix(after, ":") {
			return "", "", nil, "", false
		}
		if after != "" {
			portText = after[1:]
		}
		if !isIPv6(hostPart) {
			return "", "", nil, "", false
		}
	} else {
		hostPart = netloc
		if i := strings.IndexByte(netloc, ':'); i >= 0 {
			hostPart = netloc[:i]
			portText = netloc[i+1:]
		}
		if strings.Contains(hostPart, ":") {
			return "", "", nil, "", false
		}
	}
	if portText != "" {
		for _, r := range portText {
			if r < '0' || r > '9' {
				return "", "", nil, "", false
			}
		}
		n, err := strconv.Atoi(portText)
		if err != nil || n < 1 || n > 65535 {
			return "", "", nil, "", false
		}
		port = &n
	}
	if hostPart == "" {
		return "", "", nil, "", false
	}
	return netloc, strings.ToLower(hostPart), port, tail, true
}

func normalize(input string, explicit, allowBare bool) (string, bool) {
	text := strings.TrimSpace(input)
	if text == "" || utf8.RuneCountInString(text) > maxLen {
		return "", false
	}
	for _, r := range text {
		if unicode.IsSpace(r) {
			return "", false
		}
	}

	scheme := "https"
	rest := text
	lower := strings.ToLower(text)
	matchedScheme := false
	if strings.HasPrefix(lower, "https://") {
		scheme, rest, matchedScheme = "https", text[len("https://"):], true
	} else if strings.HasPrefix(lower, "http://") {
		scheme, rest, matchedScheme = "http", text[len("http://"):], true
	} else if strings.Contains(text, "://") {
		return "", false
	}

	netloc, host, port, tail, ok := splitAuthority(rest)
	if !ok {
		return "", false
	}
	ip := isIP(host)
	local := host == "localhost"
	if !ip && !local && !isDomain(host) {
		return "", false
	}

	added := !matchedScheme
	if added {
		if ip || local {
			scheme = "http"
		} else if strings.HasPrefix(host, "www.") || explicit {
			// Explicit www/keyword input may use any valid alphabetic TLD.
		} else {
			if !allowBare {
				return "", false
			}
			parts := strings.Split(host, ".")
			if _, exists := commonTLDs[parts[len(parts)-1]]; !exists {
				return "", false
			}
		}
	}

	if !isASCII(host) {
		ascii, err := asciiHost(host)
		if err != nil {
			return "", false
		}
		portSuffix := ""
		if port != nil {
			portSuffix = ":" + strconv.Itoa(*port)
		}
		netloc = ascii + portSuffix
	}
	return scheme + "://" + netloc + tail, added
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func hintRows(text string) []item {
	row := item{
		ID:       "hint",
		Title:    "输入网址，回车在浏览器中打开",
		Subtitle: "例如 https://www.deepseek.com/ 或 deepseek.com",
		Icon:     icon,
		Valid:    boolPtr(false),
	}
	if strings.TrimSpace(text) != "" {
		row.ID = "invalid"
		row.Title = "这不是可识别的网址"
		row.Subtitle = "例如 https://www.deepseek.com/ 、 www.deepseek.com 、 localhost:3000"
	}
	return []item{row}
}

// extractURLs 扫描整段文本，按出现顺序返回其中所有网址（归一化后去重，最多 10 个）。
// 自动识别模式用：裸域名按现有白名单校验（explicit=false）。
func extractURLs(text string) []string {
	found := extractAll(text, false, true)
	urls := make([]string, 0, len(found))
	for _, f := range found {
		urls = append(urls, f.url)
	}
	return urls
}

const maxURLs = 10

type extracted struct {
	url   string
	added bool // normalize 替用户补了协议
}

func isTrailingPunct(r rune) bool {
	switch r {
	case '.', ',', ';', ':', '!', '?', ')', ']', '}', '\'', '"',
		'。', '，', '、', '；', '：', '！', '？', '）', '】', '」', '』', '》':
		return true
	}
	return false
}

// trimTrailingPunct 去掉候选网址尾部粘上的标点（连续剥离，末尾的 ... 也一并去掉）。
func trimTrailingPunct(s string) string {
	for s != "" {
		r, size := utf8.DecodeLastRuneInString(s)
		if !isTrailingPunct(r) {
			return s
		}
		s = s[:len(s)-size]
	}
	return s
}

func isHostStart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '['
}

// schemeIndex 返回 "http://" 或 "https://"（不分大小写）在词元里的起点，找不到返回 -1。
func schemeIndex(lower string) int {
	hi := strings.Index(lower, "https://")
	lo := strings.Index(lower, "http://")
	switch {
	case hi >= 0 && (lo < 0 || hi < lo):
		return hi
	case lo >= 0:
		return lo
	}
	return -1
}

// tokenCandidates 从一个词元（以空白或汉字为界的一段 ASCII 文本）里提取网址候选。
func tokenCandidates(token string, explicit, allowBare bool) []extracted {
	accept := func(cand string) []extracted {
		u, added := normalize(cand, explicit, allowBare)
		if u == "" {
			return nil
		}
		return []extracted{{u, added}}
	}
	lower := strings.ToLower(token)
	if k := schemeIndex(lower); k >= 0 {
		// 带协议：从协议处截到词元尾，去掉尾部标点后交给 normalize。
		return accept(trimTrailingPunct(token[k:]))
	}
	// 裸域名 / www. / localhost / IPv4 / [IPv6]：去掉词元头部不可能起头的字符和尾部标点。
	runes := []rune(token)
	start := 0
	for start < len(runes) && !isHostStart(runes[start]) {
		start++
	}
	cand := trimTrailingPunct(string(runes[start:]))
	if cand == "" || strings.Contains(cand, "@") {
		// 邮箱整体跳过：a@b.com 不把 b.com 提取出来。
		return nil
	}
	return accept(cand)
}

// extractAll 按词元扫描整段文本。空白和汉字都是分隔符，网址遇到汉字即断。
func extractAll(text string, explicit, allowBare bool) []extracted {
	var out []extracted
	seen := map[string]struct{}{}
	runes := []rune(text)
	for i := 0; i < len(runes); {
		if runes[i] >= 0x80 || unicode.IsSpace(runes[i]) {
			i++
			continue
		}
		j := i
		for j < len(runes) && runes[j] < 0x80 && !unicode.IsSpace(runes[j]) {
			j++
		}
		for _, u := range tokenCandidates(string(runes[i:j]), explicit, allowBare) {
			if _, dup := seen[u.url]; dup {
				continue
			}
			seen[u.url] = struct{}{}
			out = append(out, u)
			if len(out) >= maxURLs {
				return out
			}
		}
		i = j
	}
	return out
}

// siteIcon 返回网址结果的 site: 图标。Win顺 自己向该网站取一次 favicon 并缓存一个月；
// 插件不联网、不下载、不写缓存，只负责写对字段。取不到时 Win顺 回落显示默认图标。
func siteIcon(url string) string {
	return "site:" + url
}

func buildItems(trigger, query, whole string, settings map[string]any) []item {
	explicit := trigger == "keyword"
	text := whole
	if explicit {
		text = query
	}
	allowBare := true
	if v, ok := settings["bareDomains"].(bool); ok {
		allowBare = v
	}
	if utf8.RuneCountInString(text) > maxLen {
		if explicit {
			return hintRows(text)
		}
		return []item{}
	}
	found := extractAll(text, explicit, allowBare)
	if len(found) == 0 {
		if explicit {
			return hintRows(text)
		}
		return []item{}
	}
	items := make([]item, 0, len(found))
	for i, f := range found {
		subtitle := "Enter 在默认浏览器中打开 · Ctrl+C 复制网址"
		if f.added {
			scheme := "HTTPS"
			if k := strings.IndexByte(f.url, ':'); k >= 0 {
				scheme = strings.ToUpper(f.url[:k])
			}
			subtitle = fmt.Sprintf("未写协议，按 %s 打开 · Enter 在浏览器中打开", scheme)
		}
		score := 95 - i
		if score < 85 {
			score = 85
		}
		items = append(items, item{
			ID:       fmt.Sprintf("open-url-%d", i+1),
			Title:    f.url,
			Subtitle: subtitle,
			Icon:     siteIcon(f.url),
			Score:    score,
			Open:     map[string]any{"url": f.url},
			Copy:     f.url,
		})
	}
	return items
}

func readSettings(raw string) map[string]any {
	settings := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return settings
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil || settings == nil {
		return map[string]any{}
	}
	return settings
}

func run() error {
	trigger := os.Getenv("WS_TRIGGER")
	if trigger == "" {
		trigger = "keyword"
	}
	items := buildItems(trigger, os.Getenv("WS_QUERY"), os.Getenv("WS_TEXT"), readSettings(os.Getenv("WS_SETTINGS")))
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(reply{Items: items}); err != nil {
		return fmt.Errorf("write protocol response: %w", err)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
