# 打开网址（me.open-url）1.2.1

**独立 Windows x64 EXE 版。用户不需要安装 Python、Node、Go 或 Qt。**

在 Win顺 搜索框里直接输入网址，选择结果并按 Enter，即可在默认浏览器中打开。也可以输入 `url ` 后跟网址，明确使用网址模式。

## 1.2.1 新增：结果图标显示网站 favicon

每条网址结果左边的图标不再是固定的地球，而是对应网站自己的图标：插件把结果的 Icon 写为 `site:` + 该条归一化后的完整网址，由 Win顺 向网站首页取一次 favicon、存本地缓存并每月更新；没取到之前（以及 localhost、IP、内网地址这类取不到的情况）回落显示默认图标。插件本身仍不联网、不下载 favicon、不写缓存文件、不申请权限，只负责把 Icon 字段写对；`url` 关键字模式的提示行保持地球 glyph。

## 1.2.0 新增：从整段文字中提取网址

从微信、网页等复制来的整段话，直接粘贴进 Win顺 输入框即可——插件会从整段文字里找出所有网址：

- 每个网址一条结果：单个网址回车直接打开；多个网址会列出来，用方向键挑一个再回车。
- 识别三类候选：`http://` / `https://` 开头的完整网址、`www.` 开头的域名、以及 `deepseek.com` 这类常见后缀的裸域名（`localhost`、IP 地址、`[::1]` 也算）。
- 按出现顺序排列并去重，一次最多列出 10 个。
- 自动剥掉网址末尾粘上的标点（中英文句号、逗号、括号等）；`a@b.com` 这样的邮箱不会误提取。
- 未写协议的网址按老规则提示将要使用的协议（`www.` 与常见域名按 HTTPS、`localhost` 与 IP 按 HTTP）。
- 「识别不带 http 的域名」设置关闭后，裸域名不再提取，带协议和 `www.` 的网址照常识别。

例子：

| 输入 | 结果 |
|---|---|
| `看看这个 https://github.com/Thysrael/Horizon 很不错` | 提取 1 条，回车打开 |
| `参考这两个：https://a.com 和 https://b.com/x?y=1。` | 提取 2 条（句号被剥掉） |
| `文档见 [Horizon](https://github.com/Thysrael/Horizon) 这一篇` | 提取 Markdown 链接里的网址 |
| `本地服务 http://localhost:3000/api 看一下` | 提取 localhost 带端口 |
| `发我邮箱 a@b.com` | 无结果（邮箱不算网址） |
| `今天天气不错` | 无结果，不干扰其他搜索 |

## 相比 1.1.0 的变化

- 新增整段文字网址提取（见上节）；输入本身就是单个网址时行为与 1.1.0 完全一致。
- 结果图标换为网站 favicon（`site:` 图标，由 Win顺 取图与缓存）。
- 自动触发正则从"整段输入恰好是一个网址"放宽为"包含网址即可触发"，夹在文字里的网址也能唤起插件。最终认不认仍由 EXE 里的网址校验逻辑决定。
- 触发正则只是决定插件跑不跑；协议、域名、端口、IDN 的校验逻辑不变。

## 相比 1.0.1 的变化

- 去掉 Python 运行时依赖：查询由单文件 `open-url.exe` 处理。
- 保留 Win顺 `once` 插件协议、原插件 ID `me.open-url` 和原有搜索/设置体验。
- 默认防抖从 40ms 调整为 120ms，减少连续输入时不必要的进程启动。
- 将 EXE 内存预算设为 64 MiB、查询超时设为 1200ms。
- 继续支持裸域名开关、常见 TLD 白名单、localhost/IP 使用 HTTP、URL 安全校验和中文域名转 Punycode。
- 不依赖 Qt；也不启动浏览器或其他子进程。浏览器打开动作由 Win顺 根据结果里的 `open.url` 执行。

## 用法

| 输入 | 结果 |
|---|---|
| `https://www.deepseek.com/` | 打开原始 HTTPS 地址 |
| `www.deepseek.com` | 自动补成 `https://www.deepseek.com` |
| `deepseek.com` | 自动补成 `https://deepseek.com`（受裸域名设置控制） |
| `localhost:3000` | 自动补成 `http://localhost:3000` |
| `192.168.1.1:8080` | 自动补成 `http://192.168.1.1:8080` |
| `https://中文.com/路径` | 域名转成 Punycode，路径保留 |
| `url example.zip` | 明确指定网址模式，可使用常见列表以外的字母 TLD |

- 单独输入 `url ` 会显示提示行。
- 通过自动识别触发时，无效输入不返回结果，避免干扰普通搜索。
- 只允许 `http` / `https`；拒绝 `ftp://`、`file://`、`javascript:`、带账号密码的网址和无效端口。
- 域名自动触发白名单与 1.0.1 保持一致；`www.` 和显式 `url ` 模式允许其他有效字母 TLD。

## 安装

在 Win顺「设置 → 插件 → 已安装 → 从压缩包安装」导入 `.wsplugin`，确认原生 EXE 插件的安装提示，然后启用插件。

插件 ID 保持 `me.open-url`，可作为现有插件更新。由于从脚本切换成原生 EXE，Win顺 可能要求对原生程序单独确认；这是宿主的安全流程，不要跳过自己不信任的提示。

## 运行依赖

用户侧无额外运行时依赖。仅需要 Win顺 和 Windows x64。EXE 使用 Go 标准库构建，运行时已包含在单文件里，不需要安装 Go，也不需要 Qt DLL。

开发者从源码重新构建时需要 Go 1.22 或更高版本：

```powershell
.\build.ps1
```

脚本会先执行单元测试、交叉构建 Windows x64 EXE，再启动构建产物进行协议冒烟测试。也可以手动执行：

```powershell
go test ./...
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags="-s -w -buildid=" -o open-url.exe .
```

## 权限、网络和数据

- 不声明任何插件权限。
- 不联网，不读取或写入文件，不访问剪贴板。
- 不自行启动浏览器；返回 `open.url` 由 Win顺 在用户按 Enter 后执行。
- 不保存用户输入，不使用 `WS_DATA_DIR`。

## 实现与验证说明

- EXE 是 Windows x64 独立程序，读取 `WS_TRIGGER`、`WS_QUERY`、`WS_TEXT`、`WS_SETTINGS`，向 stdout 输出 UTF-8 JSON。
- 源码测试覆盖网址规范化、IP/端口、危险协议、账号密码拒绝、IDN Punycode、设置开关、触发正则和结果格式，1.2.0 起另覆盖整段文字提取（10 条固定向量 + 去重 + 截断上限），1.2.1 起另覆盖结果图标字段（单条、多条各对各、hint 行保持 glyph）与触发正则的纯文字不触发断言。
- 1.2.1 已在 Windows 上完成交叉构建：`go test` 全过，`go vet`、`gofmt` 干净，官方 check.py 原生模式 0 错误；用 `WS_TRIGGER`/`WS_QUERY`/`WS_TEXT` 环境变量对构建产物做了闭环验证（网址图标为 `site:` + 该条网址、多条目各对各、hint 行保持 glyph、纯文字无结果），发来的 EXE 与同源码同参数重编的 SHA-256 完全一致；并在 Windows + WinShun 真机上确认 GitHub 结果显示为网站自己的 favicon。

## 许可证

MIT，见 `LICENSE`。
