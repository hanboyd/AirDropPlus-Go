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
       ├─ CF_DIB → PNG
       └─ CF_HDROP → 不透明临时文件引用
```

项目不包含账号、云端、中转、WebSocket、mDNS、SQLite、Windows Service 或开机启动。个人单机用途下，这些组件会增加维护面，却不改善固定地址快捷指令的核心路径。

采用 Go 标准库 HTTP 服务，避免 GUI/托盘库与 C 编译器依赖。后台运行由可审计的 PowerShell 启停脚本负责，不写注册表和计划任务。
