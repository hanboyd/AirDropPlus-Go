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

- 有共享输入且为图片：Base64 编码后向 `/api/v1/clipboard` 发 JSON，type 为 `image`。这是首选路径，避开 iOS 1.5.4 multipart 把文件名回填到文件 body 的 quirk。
- 有共享输入且为非图片文件：逐项用“获取 URL 内容”向 `/file` 发 multipart 文件。
- 无共享输入且 iPhone 剪贴板为图片：Base64 编码后向 `/api/v1/clipboard` 发 JSON。
- 其他 iPhone 剪贴板内容：向 `/clipboard` 发 form 字段 `clipboard`。**不要**把分享图片的文件名也回填到这里——服务端会把单 token 的文件名 / HEIC 标识识别为 iOS quirk 并返回 HTTP 400。

#### 发送时排查“只有文件名、没有内容”

AirDropPlus 1.5.4 官方签名快捷指令在某些 iOS 版本上会把 file 字段 body 发成空、或退化成原文件名字符串；服务端现在会直接返回 HTTP 400，错误信息形如：

```text
received file "IMG_0042.PNG" is empty; check the iOS shortcut's file field
received file "IMG_0042.PNG" contains only its name; check the iOS shortcut's file field
clipboard form value "IMG_0042.HEIC" looks like a filename; send the image as multipart to /file or Base64 JSON to /api/v1/clipboard
```

如果出现前两条提示，请确认在“获取 URL 内容”动作里：

- 请求体类型：`Form` → `Multipart/form-data`
- 字段：`file`，类型 `File`，值是直接拖入的图片/文件变量（不是变量名字符串）
- 不要把“Shortcut Input”当作文件名字符串当 body 发出

第三条 `looks like a filename` 出现时，意味着 iPhone 端把图片的标题字符串塞到了 `/clipboard` 的 urlencoded 表单里（不是二进制）。这是 1.5.4 在某些 iOS 版本上把“分享图片”错误降级到文本通道造成的。本项目自有蓝图已经把分享图片改为走 `/api/v1/clipboard` 的 JSON Base64 接口，避开这条降级路径。

收到 400 时，iPhone 端可以用“显示通知”动作把响应原文展示出来，方便判断是哪种异常。

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

- iPhone → Windows 图片使用新 JSON API，要求 PNG Base64。本项目自有蓝图已把分享图片统一改走这条路径，避免 iOS 1.5.4 把 multipart 文件 body 回填为文件名的 quirk。
- 兼容 `/clipboard` 现在也接受 multipart 图片字段，以及 `data:image/...;base64,...` / 可解码的图片 Base64；这些会记录为图片并在 UI 显示缩略图，不再降级成文件名。**只有“单 token + 媒体扩展名 / `IMG_xxxx` 形态”的值会触发 400**，普通正文 / 句子 / URL 不受影响。
- Windows → iPhone 支持 Windows 剪贴板常见的 24/32 位未压缩 DIB，服务转换为 PNG。
- “复制文件”与“复制文件里的图片”不同：前者作为文件传输，后者作为剪贴板图片传输。
- iPhone HEIC 走 `/file` multipart 时，文件会按原字节落盘到 `dist/received/`，但 Windows 剪贴板无法被 Go 的 `image.Decode` 解析（仅支持 PNG/JPEG/GIF）。此时 PC 日志会出现 `image saved but clipboard update skipped: decoder does not support this format`，剪贴板条目保持上一条；这是预期行为，不是上传失败。

## 固定地址要求

当前电脑 WLAN 地址为 `192.168.1.50`。要让它长期固定，需要在路由器中为这台电脑设置 DHCP 地址保留；本项目不自动修改路由器，也不做 mDNS/UDP 发现。如果地址日后变化，只需修改快捷指令 host。
