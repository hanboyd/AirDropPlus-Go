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
- 进程使用 Per-Monitor DPI Awareness V2；窗口、字体、线条、图片与命中区域按当前显示器 DPI 直接重绘，避免 Windows 位图拉伸导致高缩放屏幕模糊。
- `AddClipboardFormatListener` 事件驱动监听 PC 剪贴板，不做定时轮询。
- 界面每页 3 条、最多 2 页。长文本进入原生只读滚动文本框，可查看并复制完整内容。
- 第 7 条起的旧记录同步追加到按日期划分的 Markdown；文字保留全文，图片保存为 PNG 并由文档相对引用，文件内容保存路径清单。
- 归档标题使用 RFC 3339 纳秒时间戳并带时区；归档目录为 `dist/data/clipboard-archive/`，权限按当前 Windows 用户继承且不进入 Git。
- 手机发送成功时只把托盘状态灯从灰色切为绿色，不主动展开面板；点击托盘图标视为确认最新一次发送、复位灰灯并展开面板。状态不计数、不排队。
- 手动展开的面板在 10 秒无操作后自动隐藏。
- PC → iPhone 使用现有鉴权接口由快捷指令 `Receive` 拉取。iOS 不允许本工具在后台被 Windows 主动唤醒，因此状态代表最近一次已鉴权活动，不是假造的长连接。

采用 Go 标准库 HTTP 服务，避免 GUI/托盘库与 C 编译器依赖。后台运行由可审计的 PowerShell 启停脚本负责，不写注册表和计划任务。Windows 命名互斥量负责单实例判断，PID 文件只用于精确管理进程，即使强制终止留下 PID 文件也不会误判为活动实例。
