# 验证记录

验证日期：2026-08-11（Windows 11，Go 1.26.5 windows/amd64）。

已通过：

- `go test ./...`：4 个包通过；覆盖配置、鉴权、文字/图片剪贴板模拟、文件上传、重名、文件引用下载与 DIB/PNG 转换。
- `go vet -unsafeptr=false ./...`：通过。只排除 Win32 `GlobalLock` 必需的指针转换启发式告警，其他分析器保持启用。
- `scripts/Build.ps1`：成功生成 Windows amd64 原生单文件 `dist/AirDropPlus-Go.exe`。
- EXE SHA-256：`748999DDDAA2EB30FEEB526809B749C7123651C235CB3FA9996F6173362E9557`。
- `scripts/Smoke-Test.ps1`：真实隐藏进程启动成功；`/healthz` 返回 `0.1.0`；无 token 请求返回 401；带 token 的 multipart 文件上传成功；测试副本删除；测试进程按 PID 停止。

未冒充完成的验证：

- 未在真实 iPhone 上点击导入和授权。iOS 要求设备所有者确认，Windows 自动测试无法越过。
- 未修改路由器 DHCP 地址保留；当前 WLAN 地址 `192.168.1.50` 是否长期不变取决于路由器配置。
- 未弹出 Windows 防火墙授权或写入防火墙规则。首次从 iPhone 连接时只应允许专用网络。
