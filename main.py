"""Win顺 插件：输入网址，回车在默认浏览器中打开。

一次性（once）脚本，不依赖 SDK：从环境变量读输入，向 stdout 打印一个 JSON 对象。
只做查询，不启动程序、不写文件、不联网；打开动作由 Win顺 按结果里的 open.url 执行。
"""
import json
import os
import re
import sys

MAX_LEN = 2048

# 不带协议、也不带 www. 的“裸域名”只认这些常见后缀，
# 免得把 main.py、readme.md、run.sh 这类文件名当成网址（.py/.md/.sh 恰好也是国家域名）。
COMMON_TLDS = frozenset(
    """
    com net org edu gov mil int info biz name pro mobi
    io ai co me tv cc app dev xyz top site online tech store shop blog cloud
    cn hk tw mo jp kr sg my th vn in id ph
    uk de fr es it nl se no fi dk ch at be pl ru ua tr
    us ca au nz br mx ar za eu
    """.split()
)

ICON = "glyph:U+E774"  # Segoe Fluent Icons 的地球图标；不用 site:，免得为每个输入向对方站点发请求


def _label_ok(label):
    if not 1 <= len(label) <= 63:
        return False
    if label[0] == "-" or label[-1] == "-":
        return False
    return all(c.isalnum() or c == "-" for c in label)


def _is_ipv4(host):
    parts = host.split(".")
    return len(parts) == 4 and all(
        p.isascii() and p.isdigit() and len(p) <= 3 and int(p) <= 255 and (p == "0" or p[0] != "0")
        for p in parts
    )


def _is_ip(host):
    if _is_ipv4(host):
        return True
    if ":" in host:  # IPv6 很少见，用到时才导入 ipaddress（导入它要几毫秒）
        import ipaddress
        try:
            ipaddress.IPv6Address(host)
            return True
        except ValueError:
            return False
    return False


def _is_domain(host):
    """host 是不是形如 a.b.tld 的域名（允许中文等国际化域名）。"""
    labels = host.split(".")
    if len(labels) < 2 or not all(_label_ok(x) for x in labels):
        return False
    tld = labels[-1]
    return tld.startswith("xn--") or (tld.isalpha() and len(tld) >= 2)


def _split_authority(rest):
    """把 `主机[:端口][/路径?查询#片段]` 拆成 (netloc, 主机小写, 端口, 其余部分)；格式不对返回 None。

    不用 urllib.parse：它在 Python 3.12 里会连带导入 ipaddress，单次启动白白多花好几毫秒，
    而且各 Python 版本对怪输入的处理并不一致。
    """
    ends = [i for i in (rest.find(c) for c in "/?#") if i != -1]
    end = min(ends) if ends else len(rest)
    netloc, tail = rest[:end], rest[end:]
    if not netloc or "@" in netloc:
        return None
    if netloc.startswith("["):  # [IPv6] 或 [IPv6]:端口
        close = netloc.find("]")
        if close == -1:
            return None
        host, after = netloc[1:close], netloc[close + 1:]
        if after and not after.startswith(":"):
            return None
        port_text = after[1:]
    else:
        host, _, port_text = netloc.partition(":")
    if port_text:
        if not (port_text.isascii() and port_text.isdigit()) or not 1 <= int(port_text) <= 65535:
            return None
        port = int(port_text)
    else:
        port = None
    if not host or (":" in host and not netloc.startswith("[")):
        return None
    return netloc, host.lower(), port, tail


def normalize(text, explicit=False, allow_bare=True):
    """把输入规整成可打开的 http(s) 网址；认不出来返回 (None, False)。

    explicit：用户用了 `url ` 关键字，意图明确，裸域名不限后缀。
    allow_bare：自动识别时，是否认不带协议的常见域名（如 deepseek.com）。
    返回 (url, 是否替用户补了协议)。
    """
    text = (text or "").strip()
    if not text or len(text) > MAX_LEN or re.search(r"\s", text):
        return None, False

    scheme_match = re.match(r"^(https?)://", text, re.I)
    if not scheme_match and "://" in text:
        return None, False  # ftp://、file:// 等不在 http/https 之内，Win顺 不会替无 processes 权限的插件打开

    scheme = "https"
    if scheme_match:
        scheme = scheme_match.group(1).lower()
        rest = text[scheme_match.end():]
    else:
        rest = text
    parts = _split_authority(rest)
    if parts is None:
        return None, False
    netloc, host, port, tail = parts

    is_ip = _is_ip(host)
    is_local = host == "localhost"
    if not (is_ip or is_local or _is_domain(host)):
        return None, False

    added = not scheme_match
    if added:
        if is_ip or is_local:
            scheme = "http"  # 本机和内网地址通常只有 http
        elif host.startswith("www.") or explicit:
            pass
        elif not allow_bare or host.rsplit(".", 1)[-1] not in COMMON_TLDS:
            return None, False

    if not host.isascii():  # 中文域名转成浏览器认的 punycode
        try:
            ascii_host = host.encode("idna").decode("ascii")
        except UnicodeError:
            return None, False
        netloc = ascii_host + (":%d" % port if port else "")

    return "%s://%s%s" % (scheme, netloc, tail), added


def _hint_rows(text):
    if text.strip():
        return [{
            "id": "invalid",
            "title": "这不是可识别的网址",
            "subtitle": "例如 https://www.deepseek.com/ 、 www.deepseek.com 、 localhost:3000",
            "icon": ICON,
            "valid": False,
        }]
    return [{
        "id": "hint",
        "title": "输入网址，回车在浏览器中打开",
        "subtitle": "例如 https://www.deepseek.com/ 或 deepseek.com",
        "icon": ICON,
        "valid": False,
    }]


def build_items(trigger, query, whole, settings):
    explicit = trigger == "keyword"
    text = query if explicit else whole
    allow_bare = settings.get("bareDomains", True)
    if not isinstance(allow_bare, bool):
        allow_bare = True

    url, added = normalize(text, explicit=explicit, allow_bare=allow_bare)
    if url is None:
        # 自动触发时什么都不出，避免干扰别的搜索；用户用 url 关键字时才给提示。
        return _hint_rows(text) if explicit else []

    return [{
        "id": "open-url",
        "title": url,
        "subtitle": ("未写协议，按 %s 打开 · Enter 在浏览器中打开" % url.split(":", 1)[0].upper()
                     if added else "Enter 在默认浏览器中打开 · Ctrl+C 复制网址"),
        "icon": ICON,
        "score": 95,
        "open": {"url": url},
        "copy": url,
    }]


def main():
    env = os.environ
    try:
        settings = json.loads(env.get("WS_SETTINGS") or "{}")
        if not isinstance(settings, dict):
            settings = {}
    except ValueError:
        settings = {}

    items = build_items(
        trigger=env.get("WS_TRIGGER", "keyword"),
        query=env.get("WS_QUERY", ""),
        whole=env.get("WS_TEXT", ""),
        settings=settings,
    )
    out = json.dumps({"items": items}, ensure_ascii=False)
    try:
        sys.stdout.reconfigure(encoding="utf-8")
    except (AttributeError, ValueError):
        pass
    sys.stdout.write(out + "\n")
    sys.stdout.flush()


if __name__ == "__main__":
    try:
        main()
    except Exception:  # 诊断写 stderr（进 plugins.log），非 0 退出让底栏提示失败，不返回伪造结果
        import traceback  # 只在出错时才导入
        traceback.print_exc(file=sys.stderr)
        sys.exit(1)
