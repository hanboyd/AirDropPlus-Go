# AirDropPlus-Go

个人专用的 Windows 11 ↔ iPhone 局域网文件与剪贴板工具。Windows 端使用 Go 标准库实现，固定地址访问，不依赖云服务、不做设备发现、不做开机启动。

## 已实现

- iPhone → Windows：上传任意文件，自动规避重名和危险文件名；上传图片同时写入 Windows 图片剪贴板
- iPhone → Windows 剪贴板：文字、PNG 图片
- Windows → iPhone 剪贴板：文字、图片、资源管理器中复制的文件
- 兼容 AirDropPlus 1.5 已签名 iOS 快捷指令的 `/file`、`/clipboard` 接口
- 独立的新 `/api/v1` 接口
- 随机 192 位令牌鉴权、固定端口、上传大小限制、不向客户端泄露 Windows 文件路径
- 后台启动/精确停止脚本（不注册服务，不设置开机启动）

## 目录原则

此目录是项目的唯一源码位置。运行期私密配置、接收文件与构建产物也位于此目录下，但已被 `.gitignore` 排除：

```text
AirDropPlus-Go/
├─ cmd/                 程序入口
├─ internal/            服务、配置、Windows 剪贴板实现
├─ shortcuts/           自有快捷指令蓝图
├─ scripts/             构建、后台启动、精确停止
└─ dist/                Windows 可执行文件与运行期内容
   ├─ data/             首次运行生成的私密配置
   └─ received/         默认接收目录
```

## 构建与启动

要求：Windows 11、Go 1.24 或更高版本。

```powershell
.\scripts\Build.ps1
.\dist\AirDropPlus-Go.exe
```

第一次启动会在可执行文件旁生成 `data/config.json`（构建产物即 `dist/data/config.json`），其中包含随机 token。当前配置示例使用：

```text
电脑固定 Wi-Fi 地址：192.168.1.50
端口：53317
手机访问地址：http://192.168.1.50:53317
```

要隐藏窗口运行：

```powershell
.\scripts\Start-Hidden.ps1
```

要停止：

```powershell
.\scripts\Stop.ps1
```

停止脚本会核对 PID 与可执行文件完整路径，不匹配时保持现状。

## iPhone 快捷指令

Windows 无法替 Apple 签名新的 `.shortcut` 包；未签名文件在 iPhone 上不能直接导入。项目因此提供两条可验证路径：

1. 直接导入原 AirDropPlus 官方已签名快捷指令，并填入本项目地址和 token。本服务保留了它需要的公开 API，能立即使用。
2. 按 [快捷指令蓝图](shortcuts/shortcut-blueprint.json) 创建完全属于本项目的版本；动作清单见 [iPhone 配置](docs/iPhone-shortcut.md)。创建后可由 iPhone 自己生成 iCloud 分享链接。

无论哪种方式，iOS 都会要求本人确认“添加快捷指令”和第一次局域网/剪贴板/文件权限，这是系统安全机制，电脑端不能代按。

## API

除 `/` 和 `/healthz` 外，所有请求都需要以下任一请求头：

```http
Authorization: <token>
Authorization: Bearer <token>
X-AirDropPlus-Token: <token>
```

新接口：

- `GET /api/v1/clipboard`
- `POST /api/v1/clipboard`，JSON：`{"type":"text","data":"..."}` 或 `{"type":"image","data":"<PNG base64>"}`
- `POST /api/v1/files`，multipart，可包含一个或多个文件字段
- `GET /api/v1/files/clipboard`
- `GET /api/v1/files/{opaque-id}`

兼容接口：`GET/POST /clipboard`、`POST /file`、`GET /file/{opaque-id}`。

## 安全边界

- 这是可信家庭/个人局域网工具，不应直接映射到公网。
- token 只存在于被忽略的运行期配置与 iPhone 快捷指令中，不提交到 GitHub。
- Windows 文件下载只接受服务端刚为当前剪贴板文件签发的随机引用，不接受任意本机路径；引用 10 分钟后过期。
- HTTP 在局域网内未加密；如果网络中存在不可信设备，应改用可信热点或在系统层使用 VPN。
- Windows 防火墙第一次启动时可能请求允许访问；只需允许“专用网络”，不要允许公用网络。

## 验证

```powershell
go test ./...
go vet -unsafeptr=false ./...
.\scripts\Build.ps1
```

测试覆盖鉴权、文字与图片模拟剪贴板、文件上传清洗/重名、Windows 路径不泄露、临时文件引用下载，以及 DIB ↔ PNG 像素方向转换。

`unsafeptr` 是唯一关闭的 vet 分析器：Win32 `GlobalLock` 通过系统调用返回原始指针值，Go 必须在这个很小的 FFI 边界把它转换成字节视图。其他 vet 分析器保持启用。

## 来源与许可

本项目是独立 Go 重写，受 `yeyt97/AirDropPlus` 启发，并按其公开 API 文档提供兼容层。两者均使用 MIT License；详情见 [NOTICE](NOTICE)。
