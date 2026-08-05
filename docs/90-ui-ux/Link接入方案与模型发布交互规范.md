---
status: current
owner: Dev Team
last-reviewed: 2026-08-05
---

# Link 接入方案与模型发布交互规范

## 1. 目标与边界

渠道管理端把一个精确 `LinkImplementation` 表现为“Link 接入方案”，让管理员一次选择完整南向实现，
继续通过现有 `Models` 和 `model_mapping` 配置客户模型与 Provider 模型。界面不提供第二张可编辑的
“客户模型到 Link SKU”映射，也不让管理员自由组合 Provider、视频/素材 profile、路径或 adapter。

普通 NEWAPI 渠道不进入 Link publication 流程；未选择接入方案时保持既有渠道编辑行为。

## 2. 渠道抽屉

### 2.1 操作顺序

1. 管理员先选择适用于当前渠道类型的 Link 接入方案；方案列表不依赖 `Models` 的填写顺序。
2. 界面从 implementation 注册事实展示 Provider、视频/素材合同、资源解析方式、路径、adapter 和支持
   SKU 摘要。
3. 管理员填写任意客户模型名，并在原有模型映射中选择或输入 execution binding 已登记的 Provider 模型。
4. 界面只读展示 `客户模型 -> Provider 模型 -> Link SKU`、route family、publication version 和
   当前可履约状态。
5. 后端保存继续作为权威校验；前端预览不能创建 SKU、publication 或 implementation 身份。

Provider 模型建议项可以展示 SKU 和媒体上限摘要，但不能根据客户模型名称相似度过滤方案或自动猜测
SKU。未知、重复或不完整的 execution binding 显示明确错误并阻止 Link 保存。

### 2.2 全量响应式投影

方案投影同时响应选中方案、`Models` 和 `model_mapping`：

- 先选方案再填写模型时，受管字段随映射完成重新计算；
- 切换方案时，新方案未声明的创建路径、查询路径和 Advanced Custom routes 必须清空；
- 当前模型暂时无法解析时，发布预览和 SKU 专属字段必须清空，不能展示旧结果；
- route family 直接取唯一 execution binding，不按渠道类型维护第二份前端映射；
- 清除方案时恢复选择方案前的普通渠道值；编辑已有 Link 渠道且没有恢复快照时使用普通渠道默认值。

视频和素材字段在 Link 模式中只读。浏览器层解锁或手工覆盖不构成有效配置，后端仍按注册事实失败关闭。

## 3. 发布状态与显式改绑

同一 namespace、route family 和客户模型已经发布到相同 SKU 时，界面显示版本与当前可履约状态；零
候选显示“已发布但暂不可履约”，不能显示为模型不存在。

当方案推导 SKU 与当前 publication 不一致时：

- 普通保存保持禁止；
- 只有具备渠道敏感写权限的管理员看到“重新绑定”动作；
- 对话框固定展示客户模型、route family、当前 SKU、目标 SKU 和 expected version；
- 原因必填，确认前不能提交；
- 成功后刷新 publication 列表和渠道预览，不在前端本地伪造新版本。

错误反馈遵循：无效参数或 SKU 为 400，不存在为 404，版本竞争、相同 SKU、并发改变或合同冲突为
409。409 后必须刷新并重新确认，不提供自动覆盖或修改 expected version 的捷径。

## 4. 可见性与无障碍

- 客户页面只显示客户模型名；Link SKU、implementation 和 Provider 模型只进入管理员界面与审计。
- 派生字段必须有只读状态和来源说明，不能只依靠禁用颜色表达不可编辑。
- rebind 原因输入框使用稳定 label/控件关联；确认按钮在原因为空或提交中禁用。
- 成功与错误使用可感知状态反馈；错误文案说明对象和下一步，不暴露 Provider 凭据、私有模型或上游
  响应体。
- 新文案通过 `useTranslation()` 和七种 locale 维护，不在组件中写仅中文或仅英文的用户文案。

## 5. 验收不变量

1. 未填写模型也可以选择当前渠道类型的全部 Link 接入方案。
2. 修改客户模型不会令已选方案从列表消失。
3. 方案切换、映射变化和解析失败不会残留旧执行字段。
4. Feicai、Moxing、Kling 等真实注册项的 route family 和路径投影来自注册表。
5. 普通渠道清除方案后恢复原有编辑语义。
6. 普通保存不能静默改绑；rebind 必须具备权限、expected version 和原因。
7. 客户模型、Provider 模型和 Link SKU 在界面中职责分离，不增加重复映射入口。
