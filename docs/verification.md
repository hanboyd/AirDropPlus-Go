# 验证记录

验证日期：2026-08-11（Windows 11，Go 1.26.5 windows/amd64）。

已通过：

- `go test ./...`：全部通过；覆盖配置、鉴权、文字/图片剪贴板模拟、文件上传、重名、文件引用下载、DIB/DIBV5/Alpha 转换。
- `go vet -unsafeptr=false ./...`：通过。只排除 Win32 `GlobalLock` 必需的指针转换启发式告警，其他分析器保持启用。
- `scripts/Build.ps1`：成功生成 Windows amd64 原生单文件 `dist/AirDropPlus-Go.exe`。
- EXE SHA-256：`7AD0591587DE8FE6704F36DFBA938C97645CF36BB3C6332496BC4419548B3523`。
- `scripts/Smoke-Test.ps1`：真实隐藏进程启动成功；`/healthz` 返回 `0.1.0`；无 token 请求返回 401；带 token 的 multipart 文件上传成功；第二实例被命名互斥量拒绝且未覆盖 PID；测试副本删除；测试进程按 PID 停止。
- 管理脚本：隐藏启动、状态查询、重复启动保护、精确停止、停止后非运行状态均验证通过。
- PowerShell 解析器：`scripts/*.ps1` 全部无语法错误。

未冒充完成的验证：

- 按当前阶段要求，未执行真实 iPhone 的快捷指令导入、权限授权和局域网联调；这些不阻塞 Windows 侧功能完成。
- 未修改路由器 DHCP 地址保留；当前 WLAN 地址 `192.168.1.50` 是否长期不变取决于路由器配置。
- 已提供精确、可回滚的专用网络防火墙脚本并通过语法检查，但在 iPhone 联调暂缓期间未执行任何防火墙写入。
