# mini-api.md — 图片区域（九宫格画廊）接口文档

Compose 页新增的独立图片区域：拖拽/多选上传 → 服务端自动压缩 + 正方形缩略图 → frontmatter `images` → 首页/详情页九宫格展示 → 点击 Lightbox 查看。

## 1. 上传图片

| 项 | 值 |
|---|---|
| 名称 | 上传画廊图片 |
| 地址 | `POST /compose/image/:slug` |
| 认证 | 会话认证 + CSRF（同 `/compose/upload/:slug`） |
| 限流 | 复用 `RateLimit.Upload` 配置 |
| 实现点 | [internal/handlers/image_upload.go](internal/handlers/image_upload.go) `ComposeHandler.UploadImage`；路由注册于 [internal/commands/serve/command.go](internal/commands/serve/command.go) |

**请求**（multipart/form-data，单文件）：

| 参数 | 位置 | 说明 |
|---|---|---|
| `file` | form-file | 图片文件，必填。支持 JPEG/PNG/GIF(动图)/WebP，仅接受图片，非图片返回 400 |
| `:slug` | path | 文章 slug，须已保存草稿 |

大小限制：`Upload.MaxSize`（默认 10MB），超限返回 413。

**响应 200**：

```json
{
  "url": "/uploads/<slug>/1730000000-image-ab12cd34.jpg",
  "thumb": "/uploads/<slug>/1730000000-image-ab12cd34-thumb.jpg",
  "filename": "1730000000-image-ab12cd34.jpg",
  "width": 2048,
  "height": 1536
}
```

**处理规则**（[internal/imaging/imaging.go](internal/imaging/imaging.go)）：
- 原图：最长边 >2048px 按比例缩小后重编码（JPEG q85）；透明 PNG 保留 PNG；动图 GIF 原样透传
- 缩略图：短边中心裁剪正方形，缩放至 ≤480×480（不放大小图）；透明 PNG 输出 PNG，其余 JPEG
- 落盘：`uploads/<slug>/`，原图 + `-thumb` 配对文件，0o644
- 解码超过 40MP 拒绝（防 decompression bomb）

**错误**：400 slug 非法 / 非图片 / 损坏文件；413 超限；500 落盘失败。

## 2. 删除图片（上传后、提交前）

| 项 | 值 |
|---|---|
| 名称 | 删除画廊图片 |
| 地址 | `DELETE /compose/image/:slug/:filename` |
| 实现点 | [internal/handlers/image_upload.go](internal/handlers/image_upload.go) `ComposeHandler.DeleteImage` |

同时删除原图与 `-thumb` 配对文件。路径穿越 filename 返回 400；两文件均不存在返回 404。

## 3. 提交文章（images 字段）

`POST /compose` 与 `POST /compose/edit/:slug` 的表单新增隐藏字段 `images`：换行分隔的 `/uploads/<slug>/<filename>` URL 列表。

- 解析校验：[internal/services/compose/images.go](internal/services/compose/images.go) `ParseImagesInput` — 仅接受本 slug 名下、单段安全文件名（拒绝穿越/查询串/外部 URL/跨 slug），非法项静默丢弃并去重
- 持久化：frontmatter `images: []`（[internal/models/article.go](internal/models/article.go) `Article.Images`）
- 编辑清理：`HandleEdit` 在更新前读取 `PrevImages`，更新后 `UnlinkRemovedImages` 删除从列表移除的文件对（实现点：[internal/handlers/compose.go](internal/handlers/compose.go) `HandleEdit`）
- 孤儿清理：[internal/services/article/sweep.go](internal/services/article/sweep.go) `addImageRefs` 已把 images 及其缩略图纳入引用集合，定时清理不会误删

## 4. 渲染

- 首页卡片（thought/article）与详情页（thought/article）输出九宫格：模板 define `image-gallery`（[web/templates/feed.html](web/templates/feed.html)），最多显示 9 块，第 9 块叠加 `+N`
- 每块 `data-gallery-index` + 容器 `data-full-urls`（逗号分隔的全尺寸 URL，含未显示项）
- 点击打开 Lightbox：[web/static/js/modules/lightbox.js](web/static/js/modules/lightbox.js)（左右切换 / Esc / 遮罩关闭）
- Compose 图片区交互：[web/static/js/modules/compose-images.js](web/static/js/modules/compose-images.js)；样式 [web/static/css/gallery.css](web/static/css/gallery.css)
