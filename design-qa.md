# 首页设计 QA

- source visual truth path: 本轮用户提供的四张主页截图；本地同状态基线 `/private/tmp/yuan-home-light-before.png`
- implementation screenshot path: `/private/tmp/yuan-home-light-after-final.png`、`/private/tmp/yuan-home-features-after-2.png`、`/private/tmp/yuan-home-config-after-2.png`、`/private/tmp/yuan-home-enterprise-after-2.png`、`/private/tmp/yuan-home-dark-after-clean-2.png`
- viewport: 浏览器 CSS viewport `1504 × 818`，截图像素 `3008 × 1636`；历史移动端证据 `390 × 844`
- state: 浅色与深色主题；配置模块默认“代码编辑器 / Claude Code”，并复核“命令行工具 / curl”切换
- full-view comparison evidence: `/private/tmp/yuan-home-qa-comparison.png`
- focused region comparison evidence: `/private/tmp/yuan-home-features-after-2.png`、`/private/tmp/yuan-home-config-after-2.png`、`/private/tmp/yuan-home-enterprise-after-2.png`，用于核对区块节奏、信息密度、页尾收束和锚点落位

## Findings

最终复核未发现仍需处理的 P0、P1 或 P2 问题。

- 字体与排版：沿用现有系统字体与 CJK 回退栈；首屏标题、区块标题、正文、辅助文字和等宽配置文本保持清晰层级，移动端未出现裁切或不可读换行。
- 间距与布局：保留原首屏双栏与 API 路由示意；新增价值入口、配置前准备、完整工具配置和企业服务收尾，桌面与移动端均无页面级横向溢出。
- 颜色与令牌：继续使用单一业务蓝、深海军蓝正文、浅蓝选中态和绿色成功态；深色配置代码区与深色主题均保持语义一致。
- 图像与资源：页面没有照片或插画类资源；现有 API 流程示意得到保留，新增图标继续使用同一 Lucide 图标体系，未引入占位图片或不一致的装饰资产。
- 文案与内容：首页同时清晰表达模型 Token 服务与企业级 AI 网关；首屏品牌名跟随运行时 `systemName`，不再出现导航品牌与正文品牌不一致；配置示例不写死域名、真实密钥、模型数量或固定模型 ID。
- 交互与无障碍：场景标签、工具切换、协议同步、复制反馈、主题切换均通过；不支持的工具协议被禁用，按钮保留文本标签、焦点样式与可理解的 `aria-label`。

## Comparison History

### Pass 1

- [P2] 工具协议可以切换到该工具未声明支持的协议，造成“Cherry Studio + Anthropic 配置”这种内容不一致状态。
- [P2] 移动端工具列表因动态容器层级变为纵向堆叠，增加了配置模块高度与扫描成本。
- [P2] 复制完整配置时反馈文本包含整段代码，可能在小视口形成过长提示。

修复：工具只允许使用其对应协议；移动端工具项改为横向滚动行；完整配置复制反馈改为短文本并限制提示最大宽度。

### Pass 2

Post-fix evidence: `/private/tmp/TokenAI-home-final-desktop.png`、`/private/tmp/TokenAI-home-final-mobile.png`。

- Claude Code 默认同步 Anthropic Base URL，OpenAI 兼容按钮为禁用状态。
- 命令行切换到 Anthropic SDK 后，标题、协议、Base URL 与完整配置同步更新。
- 移动端工具列表保持单行横向滚动，页面 `scrollWidth` 与 `clientWidth` 均为 `390px`。
- 复制、主题切换与场景切换均通过；浏览器控制台错误与页面脚本错误均为 0。

## Primary Interactions Tested

- 代码编辑器、命令行工具、前端应用三个场景标签切换。
- Claude Code、curl、Anthropic SDK、Cherry Studio 工具内容切换。
- 工具与协议、Base URL、完整配置同步。
- 不支持协议禁用状态。
- Base URL、API Key、模型 ID 与完整配置复制反馈。
- 深浅色主题切换。
- 移动端页面级横向溢出检查。
- 从 `/api/status` 的 `data.server_address` 读取后台 `ServerAddress`，并同步更新首屏请求、Base URL 与完整配置。
- Claude Code、Cline、Continue、curl、Python SDK、Node.js SDK 的真实配置示例与官方说明入口。

### Pass 3

根据后续反馈，将代码编辑器与命令行区域从抽象占位配置改为可执行示例；页面通过 `/api/status` 读取后台 `ServerAddress`，OpenAI 兼容示例统一追加 `/v1`，Anthropic 兼容示例使用站点根地址。

Post-fix evidence: `/private/tmp/TokenAI-dynamic-examples-desktop.png`、`/private/tmp/TokenAI-dynamic-examples-mobile.png`。

- 后台测试配置 `https://gateway.TokenAI.test` 已同步出现在 Claude Code、Cline、Continue、curl、Python SDK、Node.js SDK 示例中。
- 编辑器区只保留有官方配置依据的 Claude Code、Cline、Continue。
- 命令行区提供完整 curl、Python SDK 与 Node.js SDK 最小请求。
- 桌面和移动端浏览器检查通过，控制台错误与页面脚本错误均为 0。

### Pass 4

依据 UI/UX Pro Max 的企业网关、可访问性与触控规范完成质量优化。

Post-fix evidence: `/private/tmp/TokenAI-ui-final-desktop.png`、`/private/tmp/TokenAI-ui-final-mobile.png`、`/private/tmp/TokenAI-ui-final-dark.png`。

- 所有可见按钮与链接在 `375 × 812`、`844 × 390`、`768 × 1024`、`1440 × 1000` 下均达到至少 `44 × 44px` 的交互目标。
- 场景标签支持 `ArrowLeft`、`ArrowRight`、`Home`、`End` 键切换，并同步维护 `aria-selected` 与 `tabindex`。
- 主题选择写入本地存储，刷新后继续保持；浅色与深色状态均经过浏览器渲染检查。
- 后台地址读取增加 loading、success、fallback、error 状态；读取失败时显示“重新读取”恢复入口。
- `prefers-reduced-motion: reduce` 下过渡时长已缩短，避免不必要动态效果。
- 所有测试视口均无页面级横向溢出，浏览器控制台错误与页面脚本错误均为 0。
- 导航、首屏、快速步骤、价值区、配置区、企业区与页尾统一使用 `1200px` 桌面内容宽度；移动端统一使用 `16px` 安全边距，并已在四类视口中验证左右边界完全对齐。
- 将首屏右侧能力说明收回流程区域，避免其越出 `1200px` 统一内容网格。

### Pass 5

根据用户提供的 `docs/90-ui-ux/首页蓝色商务风格示例 copy.html` 恢复原始配色行为。

- 对比备份与当前文件的浅色、深色主题变量，共核对 37 个既有颜色、阴影与圆角令牌，差异为 0。
- 页面始终以备份中的白底、海军蓝文字和业务蓝强调色打开，不再跟随系统主题，也不再跨刷新保留深色主题。
- 深色模式仍可手动预览，但刷新页面后恢复原始浅色方案。
- 保留 `1200px` 统一网格、`16px` 移动端边距、真实配置示例、动态 ServerAddress、键盘操作与触控尺寸优化。

### Pass 6

根据首屏反馈重新组织 API 流程图与左右两栏的纵向平衡。

- 右侧流程改为“请求 → 统一 API 网关 → 路由阶段 → 响应”的纵向链路；用户标注的红框区域作为完整“路由阶段”移动到网关下方，内部保留路由决策卡片与多渠道调度、失败重试、用量记录三项能力说明。
- 左侧补充“统一 API 接入、多渠道稳定路由、Token 与计费可追溯”三项能力摘要，并调整内容区上下留白，使文字重心与增高后的流程图对齐。
- 桌面、平板、横屏手机和小屏手机复测均无横向溢出，所有区域继续保持统一内容网格，所有可见控件继续满足 `44px` 触控目标。
- 默认浅色配色及备份中的全部既有主题令牌保持不变；主题、键盘切换、失败重试与减少动态效果检查均通过，浏览器错误为 0。

### Pass 7

本轮依据用户提供的最终主页截图，对当前工作树做收尾复核与轻量优化。

Post-fix evidence: `/private/tmp/yuan-home-qa-comparison.png`、`/private/tmp/yuan-home-light-after-final.png`、`/private/tmp/yuan-home-dark-after-clean-2.png`。

- [P2] 首屏正文固定显示 `TokenAI`，而导航、流程卡与页脚使用运行时系统名；在 `元空间`、`TokenAI` 等部署配置下形成明显品牌割裂。修复为通过 i18n 插值统一使用 `systemName`，并补齐 en、zh、fr、ja、ru、vi、zh-TW。
- [P3] 业务卡片同时使用描边与投影，层次略显重复，也不符合当前“描边或投影二选一”的视觉规则。移除卡片投影，保留清晰描边；浅色与深色主题均复核通过。
- [P3] 页内导航落点未给固定页头预留空间。为快速开始、能力、配置和企业区补充统一 `scroll-margin`，锚点跳转后标题与内容保持完整可见。
- 命令行场景切换已验证，选择后面板从 Claude Code 正确更新为 curl；主题菜单浅色与深色状态均可切换并正确渲染。
- 当前仓库匹配后端使用临时 SQLite 启动后，`/api/status` 与 `/api/home_page_content` 均返回 200；旧的本地 `:8080` 进程并非本仓库服务，其 404 不计入本轮产品缺陷。
- 浏览器自动化上下文不允许写入系统剪贴板，复制按钮走到既有“复制失败”反馈分支；该限制不影响布局验收，但真实剪贴板成功路径仍应在普通浏览器会话中复核。
- `bun run typecheck`、定向 oxlint、定向 oxfmt、`bun run i18n:sync` 与 `bun run build` 全部通过。

## Follow-up Polish

没有阻塞交付的 P3 项。正式接入产品时，应由运行时配置提供 Base URL、账户可用模型和已验证工具清单。

final result: passed

---

# 登录与注册方案 2 设计 QA

- source visual truth path: `/Users/randy/.codex/generated_images/019f50dd-3a53-7f90-a84a-6685cdd41c19/exec-a27720a8-600f-4d0a-9c6f-363f48f64d20.png`
- implementation screenshot path: unavailable
- viewport: 目标桌面视口 `1440 × 1024`；移动端目标宽度 `390px`
- state: 登录页与注册页，默认浅色主题；保留现有密码、验证码、OAuth、Passkey 与协议同意状态
- full-view comparison evidence: blocked；当前浏览器扩展无法建立连接，无法捕获实现页并与方案图合并对比
- focused region comparison evidence: blocked；无法获取品牌栏、表单区和网关预览区的浏览器渲染截图

## Findings

- [P1] 缺少浏览器渲染证据
  - Location: `/sign-in`、`/sign-up`
  - Evidence: 方案图已可读取，但实现截图不可用，无法验证双栏比例、表单纵向节奏、注册页高度、移动端折叠和真实字体渲染。
  - Impact: 代码、类型、构建和静态多语言检查通过，但不能据此声明视觉还原已通过。
  - Fix: 浏览器连接恢复后，在相同视口分别捕获登录页与注册页，合并方案图与实现截图进行对比并修复 P0/P1/P2。

## Required Fidelity Surfaces

- 字体与排版：代码沿用项目字体轴和既有字号/字重契约；浏览器字体回退、换行与字面宽度待截图确认。
- 间距与布局：实现为 `46% / 54%` 双栏、移动端单栏；实际视口中的高度与溢出待截图确认。
- 颜色与令牌：仅使用 `primary`、`muted`、`border`、`foreground` 等语义令牌；实际主题对比度待截图确认。
- 图像与资源：默认品牌使用项目现有 `TokenAiMark`，自定义 Logo 仍由运行时配置接管；无新增位图、占位图片或手绘 SVG。
- 文案与内容：复用现有 i18n 键并展示运行时 `systemName`；六个项目基础语言包与 `zh-TW` 同步报告均为零缺失。

## Comparison History

### Pass 1

- 已完成方案图检查、实现代码检查、类型检查、定向 lint、格式检查、i18n 同步与生产构建。
- 浏览器截图与主交互检查因 Chrome 扩展连接失败而未执行；没有伪造视觉对比结论。

## Implementation Checklist

- [x] 登录与注册共用同一品牌化双栏壳层。
- [x] 默认 Logo 使用主页同款品牌标记，自定义 Logo 和运行时产品名继续生效。
- [x] 保留现有认证逻辑、字段、验证和替代登录方式。
- [x] 使用语义颜色、项目圆角、响应式 CTA 与项目图标体系。
- [ ] 捕获桌面与移动端登录/注册实现截图。
- [ ] 完成方案图与实现图的同视口合并对比。

## Follow-up Polish

视觉证据恢复后，再判断是否需要调整右侧网关预览的节点宽度、表单首屏位置或注册页纵向密度。

final result: blocked
