"""运行：python -m unittest -v test_main

测试分三层：
1. normalize() 的规整与拒绝规则；
2. plugin.json 里的触发正则（这里用 Python 的 re 检验，宿主的正则引擎要在 Win顺 里再核对一次）；
3. 像 Win顺 那样通过环境变量起一个真实进程，检查 stdout 是协议规定的 UTF-8 JSON。
"""
import json
import os
import re
import subprocess
import sys
import unittest

import main

HERE = os.path.dirname(os.path.abspath(__file__))


def run_plugin(trigger, query="", text=None, settings=None):
    env = dict(os.environ)
    for k in [k for k in env if k.startswith("WS_")]:
        del env[k]
    env.update({
        "WS_TRIGGER": trigger,
        "WS_QUERY": query,
        "WS_TEXT": query if text is None else text,
        "WS_COMMAND": "open-url",
        "WS_SETTINGS": json.dumps(settings or {}),
        "WS_LOCALE": "zh",
    })
    return subprocess.run(
        [sys.executable, "-X", "utf8", os.path.join(HERE, "main.py")],
        env=env, capture_output=True, timeout=10,
    )


class Normalize(unittest.TestCase):
    def ok(self, text, expect, added=None, **kw):
        url, was_added = main.normalize(text, **kw)
        self.assertEqual(url, expect, text)
        if added is not None:
            self.assertEqual(was_added, added, text)

    def no(self, text, **kw):
        self.assertEqual(main.normalize(text, **kw), (None, False), text)

    def test_full_urls(self):
        self.ok("https://www.deepseek.com/", "https://www.deepseek.com/", added=False)
        self.ok("http://example.com/a?b=1#c", "http://example.com/a?b=1#c", added=False)
        self.ok("HTTPS://Example.com/Path", "https://Example.com/Path")
        self.ok("  https://www.deepseek.com/  ", "https://www.deepseek.com/")  # 首尾空白（比如粘贴带来的）
        self.ok("https://a.com:8443/x", "https://a.com:8443/x")
        self.ok("http://localhost:3000", "http://localhost:3000", added=False)
        self.ok("http://192.168.1.1:8080/admin", "http://192.168.1.1:8080/admin")
        self.ok("http://[::1]:8080/", "http://[::1]:8080/")

    def test_no_scheme(self):
        self.ok("www.deepseek.com", "https://www.deepseek.com", added=True)
        self.ok("deepseek.com", "https://deepseek.com", added=True)
        self.ok("github.io/foo", "https://github.io/foo")
        self.ok("a.b.example.co.uk/x", "https://a.b.example.co.uk/x")
        self.ok("localhost:3000", "http://localhost:3000", added=True)   # 本机默认 http
        self.ok("192.168.1.1", "http://192.168.1.1", added=True)

    def test_authority_splitting(self):
        self.ok("https://a.com?x=1", "https://a.com?x=1")          # 没有路径、直接接查询
        self.ok("https://a.com#top", "https://a.com#top")
        self.ok("https://a.com:8080?x", "https://a.com:8080?x")
        self.ok("https://a.com:", "https://a.com:")                 # 空端口，浏览器也接受
        self.ok("http://[2001:db8::1]/p", "http://[2001:db8::1]/p")
        self.ok("https://a.com/%E4%B8%AD?q=a@b", "https://a.com/%E4%B8%AD?q=a@b")  # 路径和查询里的 @ 不算账号
        for bad in ["https://a.com:abc", "https://a.com:65536", "https://a.com:80:80", "https://:80",
                    "http://[::1]x", "http://[::1]:", "https://a.com\\x", "https://.com", "https://a.com."]:
            if bad == "http://[::1]:":
                self.ok(bad, bad)                                  # [::1]: 同样是空端口
            else:
                self.no(bad)

    def test_www_accepts_any_tld(self):
        self.ok("www.example.zip", "https://www.example.zip")

    def test_file_names_are_not_urls(self):
        for name in ["main.py", "readme.md", "run.sh", "archive.zip", "notes.txt",
                     "a.b.c", "v1.2.3", "3.14", "report.pdf"]:
            self.no(name)

    def test_keyword_mode_is_looser(self):
        self.ok("example.zip", "https://example.zip", explicit=True)
        self.no("main", explicit=True)     # 没有后缀，仍然不是网址
        self.no("1.2.3", explicit=True)

    def test_bare_domains_can_be_turned_off(self):
        self.no("deepseek.com", allow_bare=False)
        self.ok("www.deepseek.com", "https://www.deepseek.com", allow_bare=False)
        self.ok("https://deepseek.com", "https://deepseek.com", allow_bare=False)
        self.ok("localhost:3000", "http://localhost:3000", allow_bare=False)

    def test_empty_and_whitespace(self):
        for t in ["", "   ", None, "\t\n"]:
            self.no(t)

    def test_invalid_input(self):
        for t in [
            "hello world",                 # 中间有空格
            "https://",                    # 没有主机
            "https:// deepseek.com",
            "ftp://example.com",           # 非 http(s)
            "file:///C:/Windows",
            "javascript:alert(1)",
            "https://user:pw@example.com",  # 含账号，不当作普通网址
            "https://example.com:0",
            "https://example.com:99999",
            "https://-bad-.com",
            "https://a..com",
            "https://exa_mple.com",
            "https://999.999.999.999",
            "http://01.2.3.4",             # IPv4 不允许前导零
            "http://1.2.3.256",
            "http://1.2.3.4.5",
            "http://[12345::1]",           # 无效 IPv6
            "http://[::1",                 # 方括号没闭合
            "x" * 5000,
        ]:
            self.no(t)

    def test_internationalized_domain(self):
        url, _ = main.normalize("https://中文.com/路径")
        self.assertTrue(url.startswith("https://xn--"), url)
        self.assertTrue(url.endswith("/路径"), url)
        self.assertEqual(main.normalize("中文.com", explicit=True)[0].split("/")[2], "xn--fiq228c.com")


class TriggerRegex(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        with open(os.path.join(HERE, "plugin.json"), encoding="utf-8") as f:
            cls.manifest = json.load(f)
        trig = cls.manifest["commands"][0]["triggers"][0]
        cls.rx = re.compile(trig["regex"])
        cls.min, cls.max = trig["minLength"], trig["maxLength"]

    def gate(self, s):
        return self.min <= len(s) <= self.max and bool(self.rx.match(s))

    def test_matches_urls(self):
        for s in ["https://www.deepseek.com/", "http://a.b", "HTTPS://X.com", "www.deepseek.com",
                  "deepseek.com", "a.cn", "localhost", "localhost:3000/x", "192.168.0.1:80",
                  "github.io/foo?bar=1#x", "https://中文.com", "  deepseek.com  "]:
            self.assertTrue(self.gate(s), s)

    def test_ignores_ordinary_searches(self):
        for s in ["notepad", "hello world", "deepseek com", "报告 2026", "a b.com", "C:\\Users", "1.5",
                  "", "ab"]:
            self.assertFalse(self.gate(s), s)

    def test_everything_the_gate_passes_is_decided_by_python(self):
        # 门（正则）只负责挡掉普通搜索；像 main.py 这样的文件名能过门，但必须被 normalize 拒绝。
        self.assertTrue(self.gate("main.py"))
        self.assertEqual(main.normalize("main.py"), (None, False))

    def test_no_catastrophic_backtracking(self):
        import time
        t0 = time.time()
        self.gate("a." * 900 + "-")
        self.gate("a" * 2000 + "!")
        self.assertLess(time.time() - t0, 1.0)

    def test_manifest_basics(self):
        m = self.manifest
        self.assertRegex(m["id"], r"^[a-z0-9._-]+$")
        self.assertIn(".", m["id"])  # id 形如 作者.名字；仓库根目录名不必等于 id
        self.assertNotIn("permissions", m)          # 不申请任何权限
        self.assertEqual(m["runtime"]["mode"], "once")
        self.assertEqual(m["runtime"]["debounceMs"], 40)

    def test_rare_imports_stay_lazy(self):
        # 启动速度：ipaddress / traceback 只在用到时才导入
        code = "import sys; sys.path.insert(0, %r); import main; main.normalize('https://a.com'); " \
               "print('ipaddress' in sys.modules, 'traceback' in sys.modules)" % HERE
        out = subprocess.run([sys.executable, "-c", code], capture_output=True, text=True).stdout.strip()
        self.assertEqual(out, "False False")


class EndToEnd(unittest.TestCase):
    def parse(self, proc):
        self.assertEqual(proc.returncode, 0, proc.stderr.decode("utf-8", "replace"))
        data = json.loads(proc.stdout.decode("utf-8"))   # 必须是 UTF-8 JSON
        self.assertEqual(list(data), ["items"])
        return data["items"]

    def test_regex_trigger_url(self):
        items = self.parse(run_plugin("regex", text="https://www.deepseek.com/"))
        self.assertEqual(len(items), 1)
        row = items[0]
        self.assertEqual(row["id"], "open-url")                         # 稳定 id
        self.assertEqual(row["open"], {"url": "https://www.deepseek.com/"})
        self.assertEqual(row["copy"], "https://www.deepseek.com/")
        self.assertNotIn("args", row["open"])                           # 无 processes 权限也能打开
        self.assertNotIn("valid", row)

    def test_regex_trigger_bare_domain(self):
        row = self.parse(run_plugin("regex", text="deepseek.com"))[0]
        self.assertEqual(row["open"]["url"], "https://deepseek.com")

    def test_regex_trigger_invalid_gives_nothing(self):
        self.assertEqual(self.parse(run_plugin("regex", text="main.py")), [])
        self.assertEqual(self.parse(run_plugin("regex", text="")), [])

    def test_keyword_empty_shows_hint_row_that_does_nothing(self):
        rows = self.parse(run_plugin("keyword", query=""))
        self.assertEqual(len(rows), 1)
        self.assertIs(rows[0]["valid"], False)
        self.assertNotIn("open", rows[0])

    def test_keyword_invalid_shows_error_row(self):
        rows = self.parse(run_plugin("keyword", query="hello world"))
        self.assertEqual(rows[0]["id"], "invalid")
        self.assertIs(rows[0]["valid"], False)

    def test_keyword_valid(self):
        row = self.parse(run_plugin("keyword", query="example.zip"))[0]
        self.assertEqual(row["open"]["url"], "https://example.zip")

    def test_setting_off(self):
        self.assertEqual(self.parse(run_plugin("regex", text="deepseek.com", settings={"bareDomains": False})), [])
        row = self.parse(run_plugin("regex", text="www.deepseek.com", settings={"bareDomains": False}))[0]
        self.assertEqual(row["open"]["url"], "https://www.deepseek.com")

    def test_garbage_settings_do_not_crash(self):
        env_bad = run_plugin("regex", text="deepseek.com", settings=None)
        self.assertEqual(env_bad.returncode, 0)
        proc = subprocess.run(
            [sys.executable, "-X", "utf8", os.path.join(HERE, "main.py")],
            env={**os.environ, "WS_TRIGGER": "regex", "WS_TEXT": "deepseek.com", "WS_SETTINGS": "{not json"},
            capture_output=True, timeout=10)
        self.assertEqual(proc.returncode, 0)
        self.assertEqual(json.loads(proc.stdout)["items"][0]["open"]["url"], "https://deepseek.com")

    def test_stderr_is_quiet_on_success(self):
        self.assertEqual(run_plugin("regex", text="https://a.com").stderr, b"")


if __name__ == "__main__":
    unittest.main()
