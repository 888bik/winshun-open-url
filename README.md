# 打开网址（me.open-url）1.1.0

**独立 Windows x64 EXE 版。用户不需要安装 Python、Node、Go 或 Qt。**

在 Win顺 搜索框里直接输入网址，选择结果并按 Enter，即可在默认浏览器中打开。也可以输入 `url ` 后跟网址，明确使用网址模式。

## 相比 1.0.1 的变化

- 去掉 Python 运行时依赖：查询由单文件 `open-url.exe` 处理。
- 保留 Win顺 `once` 插件协议、原插件 ID `me.open-url` 和原有搜索/设置体验。
- 默认防抖从 40ms 调整为 120ms，减少连续输入时不必要的进程启动。
- 收紧自动触发正则：普通文件名如 `main.py`、`readme.md`、`run.sh`、`archive.zip`、`report.pdf` 不再触发 EXE。
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
- 源码测试覆盖网址规范化、IP/端口、危险协议、账号密码拒绝、IDN Punycode、设置开关、触发正则和结果格式。
- 本次已完成 Windows x64 PE 交叉编译，并在 Linux 同逻辑构建产物上完成协议冒烟测试。当前构建环境不能运行 Windows 程序，最终宿主真实往返、默认浏览器打开及原生插件确认流程仍需在 Windows + Win顺 中实测。

## 许可证

MIT，见 `LICENSE`。
