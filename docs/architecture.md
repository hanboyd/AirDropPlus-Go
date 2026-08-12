# 架构与取舍

```text
iPhone 快捷指令
  │ HTTP + token（固定 192.168.1.50:53317）
  ▼
Go HTTP 服务
  ├─ /api/v1：文字、PNG、文件
  ├─ 兼容层：AirDropPlus 1.5 /clipboard、/file
  ├─ received/：手机上传文件
  └─ Win32 Clipboard
       ├─ CF_UNICODETEXT
       ├─ CF_DIB / CF_DIBV5 → PNG
       └─ CF_HDROP → 不透明临时文件引用
```

项目不包含账号、云端、中转、WebSocket、mDNS、SQLite、Windows Service 或开机启动。个人单机用途下，这些组件会增加维护面，却不改善固定地址快捷指令的核心路径。

## 原生托盘 UI

- Win32 托盘图标作为空闲状态；提示文字显示 iPhone 最近连接状态。
- 弹窗使用 Win32/GDI 和 Windows 11 原生圆角，不嵌入 Chromium、Electron 或 WebView。
- `AddClipboardFormatListener` 事件驱动监听 PC 剪贴板，不做定时轮询。
- 最近 10 条文字、图片和文件仅保存在进程内存；退出后清空，避免默认落盘私人内容。
- 手机发送成功时通过进程内事件展开面板；10 秒无操作后自动隐藏。
- PC → iPhone 使用现有鉴权接口由快捷指令 `Receive` 拉取。iOS 不允许本工具在后台被 Windows 主动唤醒，因此状态代表最近一次已鉴权活动，不是假造的长连接。

采用 Go 标准库 HTTP 服务，避免 GUI/托盘库与 C 编译器依赖。后台运行由可审计的 PowerShell 启停脚本负责，不写注册表和计划任务。Windows 命名互斥量负责单实例判断，PID 文件只用于精确管理进程，即使强制终止留下 PID 文件也不会误判为活动实例。
