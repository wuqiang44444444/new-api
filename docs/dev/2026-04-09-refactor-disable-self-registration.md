# 开发日志：禁用用户自助注册功能

> **日期**: 2026-04-09
> **类型**: refactor
> **名称**: disable-self-registration

---

## 分析

项目要求彻底隐藏所有用户自助注册界面，系统将不再支持任何形式的用户自主注册。需要定位并隐藏所有注册入口，同时保留代码以便未来恢复。

### 注册入口梳理

通过全局搜索 `register`、`注册`、`/register` 等关键词，定位到以下注册入口点：

1. **路由层** — `App.jsx` 中 `/register` 路由定义
2. **登录页** — `LoginForm.jsx` 中 OAuth 登录视图和邮箱登录视图各一处注册链接
3. **顶栏** — `UserArea.jsx` 中未登录状态的注册按钮
4. **邀请系统** — `InvitationCard.jsx` 中的邀请奖励说明 + `index.jsx` 中的邀请链接生成逻辑

## 决策

- **采用注释而非删除**：所有改动以注释形式保留代码，确保可随时恢复
- **不修改 RegisterForm.jsx 本身**：仅通过路由注释使其不可达，保持组件完整性
- **后端 API 未修改**：仅隐藏前端界面，后端注册接口保留（管理员可能通过其他方式创建用户）

## 过程

### 1. 注释 `/register` 路由 (`App.jsx`)

将路由定义包裹在 `{/* ... */}` 注释中，使整个注册页面不可访问。

### 2. 注释登录页注册链接 (`LoginForm.jsx`)

两处注册入口分别位于 `renderOAuthLoginForm()` 和 `renderEmailLoginForm()` 中的底部"没有账户？注册"链接，均替换为注释标记。

### 3. 简化顶栏未登录状态 (`UserArea.jsx`)

移除注册按钮相关变量和条件逻辑，仅保留登录按钮并始终使用 `rounded-full` 样式。注册按钮部分以注释标记保留。

### 4. 注释邀请系统注册相关内容

- `InvitationCard.jsx`：注释奖励说明卡片（含"邀请好友注册"文案）
- `index.jsx`：注释 `getAffLink()` 函数定义及其在 `useEffect` 中的调用

### 修改文件清单

| 文件 | 改动点 |
|------|--------|
| `web/src/App.jsx` | 注释 `/register` 路由 (L192-202) |
| `web/src/components/auth/LoginForm.jsx` | 注释 2 处注册链接 (L699, L852) |
| `web/src/components/layout/headerbar/UserArea.jsx` | 注释注册按钮，简化样式 (L145-196) |
| `web/src/components/topup/InvitationCard.jsx` | 注释奖励说明卡片 (L197-223) |
| `web/src/components/topup/index.jsx` | 注释 `getAffLink()` 及其调用 (L537-546, L588-592) |

## 总结

所有用户自助注册入口已通过注释方式隐藏，涵盖路由、登录页、顶栏和邀请系统共 5 个文件 6 处位置。代码完整保留，可随时恢复。
