---
status: current
owner: Dev Team
last-reviewed: 2026-08-05
---

# 公开 API 文档维护指南

本文说明如何在不扩大公开范围、不泄露内部实现且保持 OpenAPI 一致的前提下维护 `/docs` 内容。
交付边界见[公开 API 文档交付架构](../20-architecture/公开API文档交付架构.md)，页面交互见
[API 文档中心交互规范](../90-ui-ux/API文档中心交互规范.md)。

## 1. 内容位置与职责

```text
docs/openapi/relay.json                    # 经审计的 Relay 机器合同
docs/openapi/public-operations.json        # 已批准公开的 operationId
web/public/docs-content/manifest.json      # 导航、页面和文件映射
web/public/docs-content/{locale}/**/*.md   # 公开正文
web/public/docs-content/generated/         # 构建生成文件
web/scripts/docs/                          # 校验和索引生成脚本
web/src/features/docs/                     # 加载、渲染和交互模块
web/src/routes/docs/                       # /docs 路由
```

内部 `docs/` 不能整目录复制到公开内容。架构、运维、研究、实现草稿、渠道信息和后台 API 都需要
人工选择、改写和脱敏后才能成为公开说明。

## 2. 标准维护流程

### 2.1 确认公开资格

新增或修改 API Reference 前先确认：

1. 路由已经实现且对普通 API 调用方开放；
2. operationId 已在 `docs/openapi/public-operations.json` 批准；
3. `docs/openapi/relay.json` 的方法、路径、鉴权、Content-Type、schema 和错误信封与代码一致；
4. Link 能力已经发布，或 NEWAPI 原生合同已经过公开范围审计；
5. Provider 私有字段、兼容别名、`Extra` 和内部快照没有被误当成客户合同。

未实现、仅兼容读取、已退休、未完成真实验证或只面向管理员的 operation 不得进入公开导航。

### 2.2 更新机器合同与白名单

需要改变方法、路径、字段或错误时，先更新并验证 OpenAPI，再更新白名单和 Markdown。白名单只保存
operationId 与必要发布状态，不复制 schema，也不通过 `/v1`、`/api` 等路径前缀推断公开范围。

### 2.3 新增或修改页面

1. 在对应 locale 目录创建或修改 Markdown；
2. 在 `manifest.json` 登记稳定 page ID、slug、file、标题、摘要和关键词；
3. API Reference frontmatter 绑定 operationId；
4. 使用公开 Base URL 和固定占位 Key/模型编写示例；
5. 更新 `contentVersion`；
6. 运行文档校验、聚焦测试与完整 Web 构建。

页面移动或废弃时保留稳定迁移说明，不能复用旧 slug 表示另一份合同。

## 3. Manifest 编写规则

Manifest 的 locale、group、page、slug、file 和 asset 必须在各自作用域唯一：

- page ID 是跨语言稳定身份；
- slug 使用规范化小写 ASCII 分段；
- file 必须位于对应 locale 目录并以 `.md` 结尾；
- 页面顺序完全由 Manifest 决定；
- 本地资源必须显式登记；
- 字符串长度、关键词数量、页面数、资源数和文件大小受构建器上限约束；
- 禁止空段、`.`、`..`、反斜线、百分号、查询参数、fragment、控制字符和协议相对形式。

最小页面结构示例：

```json
{
  "id": "images-generations",
  "title": "创建图片",
  "slug": "api-reference/images/generations",
  "file": "zh/api-reference/images/generations.md",
  "description": "创建同步或异步图片任务",
  "keywords": ["images", "generation"],
  "assets": []
}
```

Manifest 负责标题、摘要和关键词。Markdown frontmatter 不重复这些字段，只保存：

```yaml
---
page-id: images-generations
kind: api-reference
last-verified: 2026-08-05
operations:
  - createImageGeneration
---
```

每页只允许一个 H1，且必须与 Manifest 标题一致。

## 4. Markdown 内容规范

### 4.1 API Reference 最低内容

每个接口页面至少说明：

- 方法、路径和鉴权；
- Content-Type；
- 必填与可选字段及上限；
- 明确的请求示例；
- 成功响应、异步状态或流式语义；
- 稳定错误码和调用方处理方式；
- 幂等、计费或生命周期边界；
- 最后验证日期和绑定 operationId。

参数表必须与 OpenAPI 一致。Markdown 可以补充场景和解释，但不能扩张 schema 或把某个 Provider
适配能力写成所有渠道的公开保证。

### 4.2 动态变量

首期只使用受控占位符：

```text
{{SYSTEM_NAME}}
{{SITE_BASE_URL}}
{{OPENAI_BASE_URL}}
{{ANTHROPIC_BASE_URL}}
{{API_KEY_PLACEHOLDER}}
{{MODEL_ID_PLACEHOLDER}}
```

未知占位符会使构建失败。正文不得读取真实用户 Token、Cookie、localStorage 或管理员设置。

### 4.3 链接与资源

- 文档内部页面链接到逻辑 `/docs/...` slug；
- 当前页锚点使用 `#heading`；
- 外部链接只允许 `http://`、`https://`；
- 本地图片和资源必须位于对应 locale 并由 Manifest 登记；
- 禁止远程图片、Data URL、协议相对 URL 和指向内部 `docs/` 的链接。

### 4.4 禁止内容

- raw HTML、MDX、SVG、MathML、style、iframe、script、表单；
- `javascript:`、`data:`、`file:` 等可执行或本地 scheme；
- 真实 Key、Cookie、Bearer Token、渠道凭据、私有域名；
- Provider 真实模型 ID、渠道 ID、Header override；
- Task `private_data`、连接快照、完整上游响应；
- `docs/openapi/api.json` 中的后台接口；
- 未验证能力、历史兼容字段和内部实施状态。

## 5. 本地校验命令

在 `web/` 目录运行：

```bash
bun run docs:validate
bun run test
bun run build
```

职责如下：

| 命令 | 验证内容 |
| --- | --- |
| `bun run docs:validate` | Manifest、目录清单、Markdown、安全、OpenAPI 和搜索索引 |
| `bun run test` | Docs 加载、解析、交互与相关前端回归 |
| `bun run build` | 文档校验、Rsbuild、最终 `dist` 审计和品牌校验 |

`bun run dev` 也会先执行 `docs:validate`。生产验收不能只运行独立校验，必须完成 `bun run build`，
因为最终产物审计只发生在构建后。

仓库级再运行：

```bash
task docs:check
task ai:check
```

## 6. 常见失败与处理

| 失败 | 原因 | 处理 |
| --- | --- | --- |
| Manifest page/file 不一致 | 文件未登记或路径不规范 | 修正 Manifest，不绕过校验直接 fetch |
| operationId 不存在 | 白名单、OpenAPI 或 frontmatter 漂移 | 先确认真实公开合同，再同步三者 |
| 出现额外公开文件 | 临时文件、未知生成物或内部副本 | 从公开目录移除或显式登记合法资源 |
| raw HTML/危险链接失败 | 内容超出受控 Markdown 能力 | 改用受控 Markdown 或 React 功能模块 |
| 凭据扫描命中 | 示例疑似真实秘密 | 替换为固定占位符并确认历史未泄露 |
| 搜索索引版本不一致 | 未更新 contentVersion 或未重新生成 | 更新版本并重新运行校验/构建 |
| 深层路由刷新失败 | SPA fallback 或静态部署配置错误 | 按当前 Go embed 路由测试和部署手册检查 |

不得通过关闭校验、放宽 URL/HTML 白名单或把构建错误改成 warning 来修复内容问题。

## 7. 评审清单

- [ ] 公开资格和 operationId 已确认；
- [ ] Gin 路由、OpenAPI、白名单和 Markdown 一致；
- [ ] 页面只描述客户合同，不暴露渠道和 Provider 私有实现；
- [ ] Manifest、frontmatter、H1、slug 和 contentVersion 已同步；
- [ ] 示例只使用动态 Base URL 与固定占位符；
- [ ] 链接、资源、HTML 和敏感内容通过校验；
- [ ] `bun run docs:validate`、相关测试和 `bun run build` 通过；
- [ ] 架构、工程、交互和发布事项分别更新到正确目录。
