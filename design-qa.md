# LinkMetaX 首页设计 QA

- source visual truth path: `/private/tmp/linkmetax-home-before-desktop.png`、`/private/tmp/linkmetax-home-before-mobile.png`
- implementation screenshot path: `/private/tmp/linkmetax-ui-final-desktop.png`、`/private/tmp/linkmetax-ui-final-mobile.png`、`/private/tmp/linkmetax-ui-final-dark.png`
- viewport: desktop `1440 × 1000`，mobile `390 × 844`
- state: 默认浅色主题；配置模块默认选择“代码编辑器 / Claude Code / Anthropic 兼容”
- full-view comparison evidence: `/private/tmp/linkmetax-home-qa-full-desktop.png`、`/private/tmp/linkmetax-home-qa-full-mobile.png`
- focused region comparison evidence: `/private/tmp/linkmetax-home-qa-config-desktop.png`，用于核对配置模块的信息密度、协议状态、代码区与复制操作

## Findings

最终复核未发现仍需处理的 P0、P1 或 P2 问题。

- 字体与排版：沿用现有系统字体与 CJK 回退栈；首屏标题、区块标题、正文、辅助文字和等宽配置文本保持清晰层级，移动端未出现裁切或不可读换行。
- 间距与布局：保留原首屏双栏与 API 路由示意；新增价值入口、配置前准备、完整工具配置和企业服务收尾，桌面与移动端均无页面级横向溢出。
- 颜色与令牌：继续使用单一业务蓝、深海军蓝正文、浅蓝选中态和绿色成功态；深色配置代码区与深色主题均保持语义一致。
- 图像与资源：页面没有照片或插画类资源；现有 API 流程示意得到保留，新增图标继续使用同一 Lucide 图标体系，未引入占位图片或不一致的装饰资产。
- 文案与内容：首页同时清晰表达模型 Token 服务与企业级 AI 网关；配置示例不写死域名、真实密钥、模型数量或固定模型 ID，并补齐配置位置、保存方式、验证方法与常见错误。
- 交互与无障碍：场景标签、工具切换、协议同步、复制反馈、主题切换均通过；不支持的工具协议被禁用，按钮保留文本标签、焦点样式与可理解的 `aria-label`。

## Comparison History

### Pass 1

- [P2] 工具协议可以切换到该工具未声明支持的协议，造成“Cherry Studio + Anthropic 配置”这种内容不一致状态。
- [P2] 移动端工具列表因动态容器层级变为纵向堆叠，增加了配置模块高度与扫描成本。
- [P2] 复制完整配置时反馈文本包含整段代码，可能在小视口形成过长提示。

修复：工具只允许使用其对应协议；移动端工具项改为横向滚动行；完整配置复制反馈改为短文本并限制提示最大宽度。

### Pass 2

Post-fix evidence: `/private/tmp/linkmetax-home-final-desktop.png`、`/private/tmp/linkmetax-home-final-mobile.png`。

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

Post-fix evidence: `/private/tmp/linkmetax-dynamic-examples-desktop.png`、`/private/tmp/linkmetax-dynamic-examples-mobile.png`。

- 后台测试配置 `https://gateway.linkmetax.test` 已同步出现在 Claude Code、Cline、Continue、curl、Python SDK、Node.js SDK 示例中。
- 编辑器区只保留有官方配置依据的 Claude Code、Cline、Continue。
- 命令行区提供完整 curl、Python SDK 与 Node.js SDK 最小请求。
- 桌面和移动端浏览器检查通过，控制台错误与页面脚本错误均为 0。

### Pass 4

依据 UI/UX Pro Max 的企业网关、可访问性与触控规范完成质量优化。

Post-fix evidence: `/private/tmp/linkmetax-ui-final-desktop.png`、`/private/tmp/linkmetax-ui-final-mobile.png`、`/private/tmp/linkmetax-ui-final-dark.png`。

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

## Follow-up Polish

没有阻塞交付的 P3 项。正式接入产品时，应由运行时配置提供 Base URL、账户可用模型和已验证工具清单。

final result: passed
