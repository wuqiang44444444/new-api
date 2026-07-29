---
status: current
owner: Dev Team
last-reviewed: 2026-07-29
---

# 内置 API 文档中心实施计划

## 1. 计划定位

本计划负责把[内置 API 文档中心架构](../20-architecture/内置API文档中心架构.md)落地到当前 React Web。架构文档定义长期边界，本计划定义当前批次、文件、门禁和完成顺序。

`docs/99-archive/2026-07-28-内置API文档中心实施方案.md` 只保留历史背景，不再作为实现依据。若本计划与归档方案冲突，以当前架构和本计划为准。

目标交付链路：

```text
公开合同审计与 operation 白名单
  -> manifest + Markdown + 构建校验
  -> React Docs 模块
  -> bun run build
  -> web/dist
  -> Go embed
  -> /docs
```

## 2. 当前实施状态

截至 2026-07-29：

- `/api/status` 的 `docs_link`、`server_address` 和 `system_name` 已通过统一解析器供首页、顶部导航和文档页使用；
- `/docs` 与 `/docs/*` 已作为未登录公开路由落地，深层刷新由 Go 测试固定；
- `web/public/docs-content/` 已包含 manifest、16 篇首批中文 Markdown 和生成的搜索索引；
- `docs/openapi/public-operations.json` 已批准 28 个文本、图片、视频与素材 operation，后台 `docs/openapi/api.json` 不进入公开产物；
- Docs 专用 Marked token 渲染器拒绝 raw HTML、未知 token、危险链接、远程图片和未知占位符，不使用文档内容驱动的 `dangerouslySetInnerHTML`；
- `bun run build` 已强制串联内容/合同校验、搜索索引生成、Rsbuild 和最终 dist 审计；
- Vitest、React Testing Library、user-event、happy-dom 与 `bun run test` 已接入；
- manifest 使用 `no-cache, must-revalidate`，Markdown 与搜索索引按 contentVersion 加载。

剩余上线门禁是多视口、主题、纯键盘和真实部署缓存的人工验收，以及仓库既有全局 lint/format 基线问题的独立清理。

## 3. 交付原则

### 3.1 必须先满足

以下项目是 UI 和公开内容发布前的 P0 门禁：

1. 冻结公开北向合同矩阵。
2. 修正首期范围内的 `docs/openapi/relay.json`。
3. 建立 `docs/openapi/public-operations.json`。
4. 建立强制运行的公开内容构建校验。
5. 决定并接入组件测试运行器。

P0 未完成时可以开发不含真实 API 参数的布局和组件 fixture，但不得合入可被用户理解为正式合同的 API Reference。

### 3.2 首期交付

- `/docs` 和 `/docs/*` 未登录公开路由；
- 文档顶部栏、左侧导航、正文、本页目录、上一页/下一页；
- 移动端目录、页面目录和可访问焦点管理；
- manifest 白名单加载和公开目录全量构建校验；
- 受控 Markdown token 渲染、动态 Base URL、代码高亮和复制；
- 标题、摘要、关键词、`h2`/`h3` 搜索；
- 快速开始、鉴权、Base URL、模型、错误、限流、计费、幂等和排障；
- 经白名单批准的文本、图片、视频、素材与任务查询中文文档；
- 深层路由刷新、缓存一致性和 Go embed 产物验证。

### 3.3 首期不交付

- 在线 Try it；
- 真实 API Key、用户模型或凭据自动填充；
- 作者原始 HTML、MDX、远程图片和任意组件；
- 服务端搜索和正文全文搜索；
- SSR/SSG、独立文档域名和第二套构建；
- SDK 或全部参数表的无审查自动生成；
- 管理员在线 Markdown 编辑；
- 多语言正文。

## 4. P0：公开合同冻结

### 4.1 建立机器可读白名单

新增：

```text
docs/openapi/public-operations.json
```

白名单只保存：

- schema 版本；
- 已批准的 operationId；
- 发布状态；
- 可选的合同族标识。

白名单不复制请求或响应 schema，不使用路径前缀表示公开范围。构建脚本以 operationId 在审计后的 `relay.json` 中解析方法、路径和 schema。

禁止：

- 自动公开全部 `/v1` 路由；
- 自动排除全部 `/api` 路由；
- 从 Go DTO 反射字段；
- 把 `docs/openapi/api.json` 合并到 Relay 文档；
- 把退休、存量只读、未实现或未验证 operation 误标为新用户可用。

### 4.2 合同审计清单

#### 文本

- `GET /v1/models`；
- `POST /v1/chat/completions`；
- `POST /v1/responses`；
- `POST /v1/messages`。

逐项确认不同认证头是否改变响应格式，以及页面是发布一个明确协议视图还是拆分说明。

#### 图片

- `POST /v1/images/generations`；
- `POST /v1/images/edits`；
- `GET /v1/images/tasks/:task_id`。

修正 Qwen `input.messages` 南向结构、图片编辑 Content-Type、重复 operationId、显式零值、`n` 上限、同步/异步联合响应和图片 Task 错误信封。

#### 视频

- ModelArk `/api/v3/contents/generations/tasks` 创建、列表、查询和删除；
- Kling `/kling/v1/videos/text2video`、`image2video` 及各自查询；
- 即梦 `/jimeng/` 的 submit/query Action；
- `/v1/videos/:task_id/content` 只在确认其普通 API 调用边界后加入。

视频文档按官方北向合同分别组织，不抽象成一个供应商中立请求 DTO。OpenAI 视频创建和仅兼容读取的存量入口不得作为新用户能力发布。

#### 素材

- `/v1/assets` 创建、列表、读取、更新和删除；
- `/v1/assets/:asset_id/bindings`；
- `/v1/assets/:asset_id/migrations`；
- 真人授权创建、查询、撤销和重试。

逐项确认普通 Token 权限、模型访问约束、幂等、用户隔离、`asset://` 引用、状态和错误。面向同意人的匿名 `/consent/...` 页面不是 API Key 文档主体；后台 policy API 永不公开。

### 4.3 合同退出标准

- 每个首期 API Reference 绑定唯一、非退休的 operationId；
- Gin 路径、方法、鉴权、OpenAPI 和 Markdown frontmatter 一致；
- 参数表不存在渠道、Provider、`Extra`、历史别名和 `private_data`；
- 示例仅使用占位 Key、动态 Base URL 和占位模型；
- 未能确认的 operation 从白名单和首期导航移除，不用猜测内容占位。

## 5. P0：构建与发布安全门禁

### 5.1 新增脚本

```text
web/scripts/docs/validate-manifest.mjs
web/scripts/docs/validate-content.mjs
web/scripts/docs/validate-contracts.mjs
web/scripts/docs/build-search-index.mjs
web/scripts/docs/validate-dist.mjs
```

实际实现可以在 `web/scripts/docs/` 内合并共享类型和稳定领域逻辑，但不得扩写现有通用 Markdown 组件。

`web/package.json` 至少提供：

```text
docs:validate
docs:validate:dist
test
```

`bun run build` 固定执行：

```text
docs:validate
-> build-search-index
-> rsbuild build
-> docs:validate:dist
```

任何步骤失败都必须使生产构建失败。

### 5.2 Manifest 和文件清单

校验：

- schemaVersion、contentVersion、locale、group、page 和 asset 类型；
- id、slug、file 和 asset 唯一性；
- slug 规范、路径长度、字符串长度、页面数和文件大小上限；
- 一次 URL 解码后的路径仍为规范 slug；
- 禁止 `.`、`..`、反斜线、百分号、控制字符、查询参数、fragment 和协议相对形式；
- Markdown 与本地资源必须位于对应 locale 目录。

枚举 `web/public/docs-content/` 实际文件，要求精确等于：

```text
manifest.json
+ manifest 登记 Markdown
+ manifest 登记本地资源
+ 固定生成文件
```

拒绝符号链接、隐藏文件、编辑器临时文件、未登记 Markdown、OpenAPI 副本和未知生成文件。

### 5.3 内容安全

构建时拒绝：

- raw HTML、MDX、SVG、MathML、style、iframe、script、表单；
- `javascript:`、`data:`、`file:`、协议相对 URL；
- 未登记本地资源和远程图片；
- 未知动态占位符；
- 疑似真实 Key、Cookie、Bearer Token、渠道凭据或私有域名；
- `private_data`、渠道 Key、Header override、内部 Task 快照和管理 API 内容；
- 指向内部 `docs/` 或 `docs/openapi/api.json` 的链接。

密钥扫描使用明确规则和安全 fixture；允许固定的 `sk-your-key`、`<available-model-id>` 等占位形式。

### 5.4 最终产物审计

Rsbuild 完成后重新枚举 `web/dist/docs-content`，确认：

- 文件集合与源清单一致；
- 搜索索引只含公开页面；
- 不包含 `docs/openapi/api.json`、内部 `docs/` 和源脚本；
- 不包含真实凭据模式；
- manifest 与搜索索引的 contentVersion 一致。

## 6. P1：Manifest、加载器和路由

### 6.1 Manifest

采用架构文档定义的 locale 分层 schema：

```text
schemaVersion
contentVersion
defaultLocale
locales[locale].groups[].pages[]
```

Manifest 是导航、搜索元数据和运行时文件映射的权威来源。Markdown frontmatter 只保存：

```text
page-id
kind
last-verified
operations
```

构建器要求每页只有一个 `h1`，且与 manifest 标题一致。

### 6.2 路由与加载器

新增：

```text
web/src/routes/docs/route.tsx
web/src/routes/docs/index.tsx
web/src/routes/docs/$.tsx
web/src/features/docs/lib/docs-manifest.ts
web/src/features/docs/lib/docs-loader.ts
```

TanStack Router 的实际 splat 文件名以当前插件生成结果为准，`routeTree.gen.ts` 只由插件生成。

加载顺序：

```text
规范化 slug
-> 查已校验 manifest
-> 得到受控 file
-> fetch Markdown?version=contentVersion
-> 解析和渲染
```

不得执行 `fetch('/docs-content/' + userSlug + '.md')`。

状态至少包含：

```text
loading_manifest
manifest_error
resolving_page
not_found
loading_content
content_error
rendering
ready
```

页面切换取消旧请求，旧页内容不能伪装为新页。`/docs`、深层 URL、未知 slug、快速切换和前进/后退都需要集成测试。

## 7. P1：Markdown 与安全渲染

新增：

```text
web/src/features/docs/lib/docs-frontmatter.ts
web/src/features/docs/lib/docs-markdown.ts
web/src/features/docs/lib/docs-headings.ts
web/src/features/docs/lib/docs-links.ts
web/src/features/docs/lib/docs-placeholders.ts
web/src/features/docs/components/docs-content.tsx
web/src/features/docs/components/docs-code-block.tsx
```

运行顺序：

```text
frontmatter
-> Marked lexer
-> 拒绝 raw HTML/未知 token
-> 在文本和代码 token 中替换受控变量
-> heading ID
-> 链接和资源解析
-> React 组件
-> Shiki codeToTokens
```

要求：

- 不使用文档内容驱动的 `dangerouslySetInnerHTML`；
- Shiki token 由 React span 渲染，代码文本始终转义；
- 代码高亮失败时显示转义后的纯文本；
- 复制按钮只复制原始代码文本；
- 相对文档链接解析为 manifest slug，而不是磁盘路径；
- 任何不得不生成的 HTML 片段使用独立 DOMPurify 严格允许列表；
- heading ID 算法固定、去重且可测试；
- 页面加载和路由切换后正确处理 hash 与焦点。

## 8. P1：动态站点信息和链接

### 8.1 文档链接

新增稳定公共逻辑：

```text
web/src/lib/docs-link.ts
```

规则：

- 空值：内部 `/docs`；
- 合法 HTTP(S)：外部链接；
- 单个 `/` 开头且不是 `//`：内部链接；
- 其他：回退 `/docs`。

修改现有文件时只保留窄调用：

```text
web/src/hooks/use-top-nav-links.ts
web/src/features/home/components/sections/hero.tsx
```

外部 `docs_link` 配置继续优先，不在上线时自动覆盖为 `/docs`。

### 8.2 Base URL

新增稳定公共逻辑统一推导：

```text
rootBaseUrl
openAIBaseUrl
anthropicBaseUrl
```

来源优先级：

```text
合法 server_address
-> 当前 request origin
```

规则：

- 只接受 HTTP(S)；
- 保留合法部署路径前缀；
- 去除尾部斜线；
- 只在终端位置处理 `/v1`；
- OpenAI 与 Anthropic 是否包含 `/v1` 按协议分别生成；
- 不使用 localStorage、Cookie、用户 Token 或当前模型自动填充。

首页配置引导和文档示例复用同一解析结果，防止两个页面给出不同 Base URL。

## 9. P1：缓存一致性

现有静态中间件会对非根资源设置一周缓存。实施必须加入窄范围规则：

```text
/docs-content/manifest.json
  Cache-Control: no-cache, must-revalidate
```

客户端同时使用 `cache: no-store` 获取 manifest。Markdown 和搜索索引请求附加 `contentVersion`；manifest、Markdown 和索引随同一 Go embed 版本发布。

若不修改后端缓存中间件，必须提供浏览器和实际 Go 静态服务下等价的确定性验证，证明新部署不会继续读取旧 manifest；仅设置自定义 `Cache-Version` 响应头不构成证明。

## 10. P2：布局、搜索和内容

### 10.1 布局与交互

新增：

```text
web/src/features/docs/components/docs-layout.tsx
web/src/features/docs/components/docs-header.tsx
web/src/features/docs/components/docs-sidebar.tsx
web/src/features/docs/components/docs-mobile-nav.tsx
web/src/features/docs/components/docs-toc.tsx
web/src/features/docs/components/docs-pagination.tsx
web/src/features/docs/components/docs-search.tsx
```

要求：

- 桌面三栏，移动端抽屉；
- 320px 无页面级横向溢出；
- 表格和代码块局部横向滚动；
- 当前页使用 `aria-current`；
- 抽屉按钮使用 `aria-expanded`；
- 支持 Escape、焦点返回、键盘搜索和跳转；
- 路由切换后聚焦正文标题；
- 暗色与浅色满足 WCAG 2.1 AA。

### 10.2 搜索

构建期从 manifest 和 `h2`/`h3` 生成：

```text
title
description
keywords
headings
slug
group
locale
```

浏览器首次打开搜索时加载当前 locale 索引。首期不索引正文和代码。索引失败时保留目录导航并禁用搜索。

### 10.3 首批中文内容

内容按 P0 白名单决定，至少覆盖：

- 概述、快速开始、鉴权、Base URL；
- 模型发现、错误、限流、计费、幂等、重试；
- 文本最小请求；
- 图片生成、编辑、同步/异步响应和图片 Task；
- 已批准的 ModelArk、Kling、即梦视频创建与查询；
- 已批准的素材生命周期、`asset://` 引用和真人授权；
- Base URL、Key、模型、额度、限流、Content-Type、multipart 和异步任务排障。

每个 API Reference 包含方法、路径、鉴权、Content-Type、参数、字段上限、模型/SKU、示例、成功响应、错误、幂等或任务生命周期、计费提示和最后验证日期。

## 11. 测试基础设施与用例

### 11.1 测试运行器

在开始组件开发前加入并固定：

- Vitest；
- React Testing Library；
- `@testing-library/user-event`；
- happy-dom 测试环境；
- `bun run test` 的非 watch CI 入口。

测试文件放在模块专属 `__tests__/` 目录，不使用大范围 DOM 或 Tailwind class 快照。

### 11.2 纯逻辑

- manifest schema、唯一性、上限和 locale fallback；
- slug、编码路径、反斜线、协议相对 URL 和路径穿越；
- 文档链接内部/外部/非法值回退；
- Base URL 协议、路径前缀和 `/v1`；
- frontmatter、operation 绑定和未知占位符；
- heading ID、重复 heading；
- 搜索索引内容、排序、locale 和上限；
- raw HTML、危险协议和未登记资源拒绝。

### 11.3 组件与路由

- `/docs`、深层刷新和文档 404；
- manifest/Markdown/搜索失败降级；
- 快速切换不显示旧页；
- 代码复制；
- 内链、外链和 hash；
- 上一页/下一页；
- 移动抽屉打开、关闭、Escape、焦点返回和 ARIA；
- 搜索键盘选择；
- 路由切换后正文焦点；
- 320px 关键溢出合同。

### 11.4 Go 与构建

- manifest 静态响应缓存规则；
- Go embed 包含 manifest、Markdown 和搜索索引；
- `/docs/...` 深层路径返回 SPA；
- `web/dist/docs-content` 文件集合与源清单一致；
- `api.json`、内部 docs、私有字段和密钥 fixture 不在产物中。

## 12. 验证命令

实现阶段至少执行：

```bash
cd web
bun run i18n:sync
bun run docs:validate
bun run test
bun run typecheck
bun run lint
bun run format:check
bun run build
```

如果修改静态缓存规则或 Go embed 测试，再执行对应 Go 测试。文档收尾执行：

```bash
task docs:check
task ai:check
```

## 13. 建议批次

| 批次 | 范围 | 退出结果 |
| --- | --- | --- |
| P0-A | 合同矩阵、OpenAPI 修正、公开 operation 白名单 | 可证明哪些接口允许公开 |
| P0-B | manifest、公开目录和 dist 强制校验；测试运行器 | 错误内容不能进入生产构建 |
| P1-A | 路由、加载器、缓存、链接和 Base URL | `/docs` 可安全加载受控页面 |
| P1-B | token 渲染、代码块、链接、heading | Markdown 无执行面且可复制 |
| P2-A | 三栏布局、移动端、目录、分页、搜索 | 完整阅读和导航体验 |
| P2-B | 首批文本、图片、视频、素材内容 | 第三方可完成真实接入 |
| P3 | OpenAPI 结构化生成、多语言、全文搜索 | 按真实需求增强 |

各批次必须独立通过对应测试和生产构建。不得同时以“稍后审计”为前提大规模编写参数文档。

## 14. 上线验收

### 合同

- 所有 API Reference 都绑定公开白名单 operation；
- Gin、OpenAPI、Markdown 的方法、路径和 Content-Type 一致；
- 图片、视频、素材和 Task 按各自北向合同说明；
- 退休、存量只读、未实现和未验证能力未作为新能力发布。

### 安全

- 公开目录和最终产物全量清单通过；
- 无后台 OpenAPI、渠道字段、`private_data`、凭据和真实 Key；
- 无 raw HTML、危险协议、远程图片和未登记文件；
- 动态变量不读取用户 Token、Cookie 或 localStorage；
- 外部 `docs_link` 仅接受 HTTP(S)，非法配置回退 `/docs`。

### 质量

- typecheck、lint、format、test、build 全部通过；
- 未登录访问、深层刷新、缓存升级和 Go embed 通过；
- 320、768、1440 三类视口和 light/dark/system 主题通过；
- 鼠标、纯键盘、搜索、目录、复制、锚点和前进/后退通过；
- 人工公开面审查签字完成。

## 15. 回滚

文档中心不新增数据库和后端业务迁移：

1. 外部 `docs_link` 可继续把流量指向既有文档站；
2. 回滚同一个 Web/Go 应用版本即可同时回滚路由和内容；
3. 单页合同错误可先从 manifest 和白名单移除并重新发布；
4. 缓存规则必须保证回滚后的 manifest 不继续引用新版本内容；
5. 回滚不删除或替换任何受保护的项目身份、署名和归属信息。

## 16. 完成定义

```text
公开合同白名单
+ 审计后的 OpenAPI
+ 强制公开目录与 dist 校验
+ 单次 Web 构建和单个 Go 部署
+ /docs 公开路由
+ 受控 token 渲染
+ 动态部署信息
+ 构建期搜索索引
+ 文本、图片、视频、素材与任务文档
+ 自动测试和人工公开面审查
```

只完成页面外观、只渲染 Markdown、只发布 OpenAPI，或只依赖 manifest 隐藏未登记静态文件，都不能视为完成。
