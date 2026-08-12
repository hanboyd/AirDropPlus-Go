# iPhone 快捷指令配置

> 当前阶段只完成快捷指令协议和配置材料，不执行真实 iPhone 导入、权限授权或局域网联调。

## 立即可用：兼容的已签名快捷指令

1. 在 iPhone Safari 打开原项目发布的 [AirDrop Plus 1.5.4 快捷指令](https://www.icloud.com/shortcuts/c499c9a3d9b04e189cce38d9560b3e2e)。
2. 点“添加快捷指令”。这是无法由 Windows 自动完成的系统确认。
3. 打开新添加的快捷指令进行编辑。在最前面的 `userconfig` 字典中替换三个示例值：
   - host：`192.168.1.50`（不要带 `http://`）
   - port：`53317`
   - key：从 `dist/data/config.json` 复制 `token`
4. 官方分享包没有导入问题，不会在安装时询问这些值。若保留默认的
   `YourPcName.local / 53843 / 123456`，点“Send”不会连接到本服务。
5. 第一次运行时允许局域网访问。
6. 在快捷指令详情中打开“在共享表单中显示”。

从共享表单发送图片时，服务会保存原文件，并在 `image_upload_to_clipboard: true` 时把同一图片写入 Windows 图片剪贴板，因此官方已签名快捷指令也能覆盖“iPhone 图片 → Windows 剪贴板”。

服务兼容 `ShortcutVersion: 1.5.4`，配置中的 `legacy_shortcut_version` 为 `1.5`，按主次版本匹配。

## 本项目自有版本的动作结构

可机器读取的完整蓝图位于 `shortcuts/shortcut-blueprint.json`。快捷指令包含三个菜单：

### 发送

- 有共享输入：逐项用“获取 URL 内容”向 `/file` 发 multipart 文件。
- 无共享输入且剪贴板为图片：Base64 编码后向 `/api/v1/clipboard` 发 JSON，type 为 `image`。
- 其他剪贴板内容：向 `/clipboard` 发 form 字段 `clipboard`。

### 接收

- 请求 `/clipboard`。
- `text`：复制 `data.data`。
- `img`：Base64 解码 `data.data` 后复制。
- `file`：逐项请求 `/file/{id}`，再显示共享表单或保存文件。

### 测试

- 请求 `/` 并显示 `Hello World!`。

所有请求统一加入：

```text
Authorization: <token>
ShortcutVersion: 1.5.4
```

## 图片同步说明

- iPhone → Windows 图片使用新 JSON API，要求 PNG Base64。
- 兼容 `/clipboard` 现在也接受 multipart 图片字段，以及 `data:image/...;base64,...` / 可解码的图片 Base64；这些会记录为图片并在 UI 显示缩略图，不再降级成文件名。
- Windows → iPhone 支持 Windows 剪贴板常见的 24/32 位未压缩 DIB，服务转换为 PNG。
- “复制文件”与“复制文件里的图片”不同：前者作为文件传输，后者作为剪贴板图片传输。

## 固定地址要求

当前电脑 WLAN 地址为 `192.168.1.50`。要让它长期固定，需要在路由器中为这台电脑设置 DHCP 地址保留；本项目不自动修改路由器，也不做 mDNS/UDP 发现。如果地址日后变化，只需修改快捷指令 host。
