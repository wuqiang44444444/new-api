# 2026-04-09 bugfix: 前端构建错误修复

## 分析

本地 `bun install` 后 `bun run build` 连续出现三个构建错误，逐一排查修复。

### 1. Semi UI CSS 导入失败

**错误：** `Missing "./dist/css/semi.css" specifier in "@douyinfe/semi-ui" package`

**原因：** `@douyinfe/semi-ui` v2.94.0 的 `exports` 字段不再包含 `./dist/css/semi.css` 路径，Vite 严格校验 `exports` 导致导入失败。项目已通过 `@douyinfe/vite-plugin-semi`（vite.config.js:54）自动注入 Semi Design CSS，手动导入 `semi.css` 属于冗余代码。

**决策：** 删除 `web/src/index.jsx` 中第 23 行的 `import '@douyinfe/semi-ui/dist/css/semi.css'`，依赖 `vitePluginSemi` 插件处理样式注入。

### 2. SiLinkedin 图标不存在

**错误：** `"SiLinkedin" is not exported by "react-icons/si/index.mjs"`

**原因：** `react-icons` 新版本中 `si`（Simple Icons）包已完全移除 LinkedIn 图标。

**决策：** 改用 `react-icons/fa6`（Font Awesome 6）中的 `FaLinkedin` 替代。修改 `web/src/helpers/render.jsx`：
- 移除 `SiLinkedin` 从 `react-icons/si` 的导入
- 新增 `import { FaLinkedin } from 'react-icons/fa6'`
- 更新 `oauthProviderIconMap` 中 `linkedin` 映射

### 3. Air 监控范围过大

**错误：** `too many open files` — Air 默认监控整个项目目录，包括 `web/node_modules/`

**决策：** 创建 `.air.toml` 配置文件，通过 `exclude_dir` 排除 `web/node_modules`、`web/dist` 等非 Go 目录，`include_ext` 仅监控 Go/模板相关文件。

## 修改文件

| 文件 | 变更 |
|------|------|
| `web/src/index.jsx` | 删除冗余 semi.css 导入 |
| `web/src/helpers/render.jsx` | SiLinkedin → FaLinkedin (fa6) |
| `.air.toml` | 新增 Air 配置，排除前端依赖目录 |

## 总结

三个问题均为依赖升级导致的兼容性断裂。Semi UI 新版改变了 CSS 导出策略，react-icons 移除了部分 Simple Icons，Air 无配置时默认全量监控。修复原则：优先利用已有插件能力消除冗余代码，选择同库替代方案保持一致性。
