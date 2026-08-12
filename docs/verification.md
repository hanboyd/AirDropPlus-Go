# 验证记录

验证日期：2026-08-12（Windows 11，Go 1.26.5 windows/amd64）。

已通过：

- `go test ./...`：全部通过；覆盖配置、鉴权、双向文字/图片剪贴板模拟、历史去重和 64 MiB 内存上限、文件上传、重名、文件引用下载、DIB/DIBV5/Alpha 转换。
- `go vet -unsafeptr=false ./...`：通过。只排除 Win32 `GlobalLock` 必需的指针转换启发式告警，其他分析器保持启用。
- `scripts/Build.ps1 -Version 0.2.1` 成功生成原生 Windows GUI 构建，`/healthz` 返回 `0.2.1`；EXE SHA-256：`26D6C54A3183E45A0AEE1E168EDD46154B2D144A5F4AE176FF113517C4F08D73`。
- 真实隐藏进程启动成功；无 token 请求返回 401；带 token 的 iPhone → PC 文字与 PNG 写入成功；PC → iPhone 鉴权读取返回相同文字与有效 PNG。
- 托盘 UI 使用 Win32/GDI、系统剪贴板事件和进程内通知；不包含 Electron、Chromium 或 WebView。手机侧最新一次写入只将托盘状态灯切为绿色，不展开面板；点击图标后复位灰灯并展开，面板 10 秒空闲后隐藏。
- 文字和图片往返测试后 5 秒采样：CPU 时间增加 0.031 秒，工作集 25.2 MiB，私有内存 52.4 MiB。重启清空历史后的纯空闲 5 秒采样：CPU 时间增加 0 秒，工作集 15.6 MiB，私有内存 46.8 MiB，11 个线程。
- Charter 字体从 `dist/fonts` 以 `FR_PRIVATE` 方式加载，退出时移除，不注册或修改 Windows 系统字体。
- 管理脚本：隐藏启动、状态查询、重复启动保护、精确停止、停止后非运行状态均验证通过。
- PowerShell 解析器：`scripts/*.ps1` 全部无语法错误。

未冒充完成的验证：

- 按当前阶段要求，未执行真实 iPhone 的快捷指令导入、权限授权和局域网联调；这些不阻塞 Windows 侧功能完成。
- 未修改路由器 DHCP 地址保留；当前 WLAN 地址 `192.168.1.50` 是否长期不变取决于路由器配置。
- 已提供精确、可回滚的专用网络防火墙脚本并通过语法检查，但在 iPhone 联调暂缓期间未执行任何防火墙写入。
