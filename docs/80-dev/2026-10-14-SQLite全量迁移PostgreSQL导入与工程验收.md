---
status: implemented-pending-acceptance
owner: Dev Team
last-reviewed: 2026-10-14
---

# SQLite 全量迁移 PostgreSQL 导入与工程验收

## 范围与结论

渠道分组容量已按用户确认改为 VARCHAR(128)，前后端拒绝超过 128 个 Unicode 字符的 CSV
列表，逗号计入长度。工作区线上备份已实际导入本地 PostgreSQL 16，完整校验及当前工程启动、
HTTP、鉴权和用户隔离验收通过。全量搬库仍由独立 Python 脚本完成，不增加应用搬库入口。

未连接生产 PostgreSQL、未部署、未修改线上配置或原 SQLite 备份。扩大检查发现当前工作区
Router/service 测试与前端全仓 lint 仍有失败，不能宣称全仓测试通过或整体生产环境没有问题。
真实 Provider、OSS、生产负载和备份恢复尚需部署验收。

正式步骤见 [SQLite 全量迁移 PostgreSQL 手册](../40-operations/SQLite全量迁移PostgreSQL手册.md)。

## 存储与保存规则

- `Channel.Group` 声明为 `type:varchar(128);size:128;default:'default'`。
- 渠道新增/编辑及标签批量修改共用后端字符长度校验，在写库前拒绝超长输入。
- 渠道表单与两个标签编辑界面显示错误并阻止保存，中英文提示已补齐。
- 前端按 Unicode code point、后端按 `utf8.RuneCountInString` 计数，包含分隔逗号。
- 导入脚本将历史 VARCHAR(64) 扩为 VARCHAR(128)，不裁剪；未来超过 128 的源值使预检查失败。

三库回归发现 GORM v1.25.12 的等价默认值比较会覆盖此前识别出的列长度变化。仅改 `type`
或增加 `size` 仍不能升级旧 VARCHAR(64) 列。已有本地 MySQL/PostgreSQL schema migrator 增加
列比较接线，仅对 `Channel.Group` 隐去已经等价的默认值比较；长度、其他字段和实际 ALTER
继续由 GORM 及原模型处理，默认分组保持不变。没有新增全量迁移或数据回填入口。

校验和 schema 比较主体分别在新增 `model/channel_group_validation.go` 与
`model/channel_group_schema_comparison.go`。NEWAPI 原生修改的必要性与冲突影响：
`model/channel.go` 仅修改容量声明及标签保存窄校验；`controller/channel.go` 仅增加统一保存
校验的三行接线。这些点表达正式管理输入与长期存储合同，只改界面或一次性改数据库不能保证
重启安全。请求热路径未修改；未来上游同步仅需核对这些声明/接线。

## 实际导入与完整性

源为根目录 `one-api.db`，581,726,208 字节，源 SHA-256：

```text
47ef70d648f73621daf17407f567253379a4fded9a9848b3e2359eaef556aca3
```

实际导入库为本机 Docker PostgreSQL 16 的 `oneapi_import_verified_v128`。
主节点初始化使用独立副本 `oneapi_acceptance_v128`；HTTP 使用副本 `oneapi_acceptance_http`。
随机测试凭据保存在本机私有文件中，不在本文记录连接串、密码、源行或 Task 私有数据。

| 对象 | 结果 |
| --- | --- |
| 全量导入 | 99 张源表、670,809 行，事务提交 |
| 校验 | 事务内校验与独立提交后 verify，所有源字段规范化 SHA-256 一致 |
| 用户／Key／渠道 | 85／153／83 行，原 ID 与关联保留 |
| logs／Task／attempt | 223,140／3,871／3,886 行 |
| 长渠道分组 | 11 行，最长 115 字符，导入及重复启动后完整保留，尾项仍可查询 |
| 原始 SQLite | 执行前后 SHA-256 相同 |
| 纳秒转换 | 56 个软删除时间显式截去微秒以下尾数，写入报告 |

56 个时间分别属于 `models.deleted_at` 15 行、`tokens.deleted_at` 39 行、`users.deleted_at`
1 行、`vendors.deleted_at` 1 行。整数时间戳、资金和 JSON 不因此改写；原纳秒时间保留在
SQLite 备份中。不能将规范化校验称为原始字节完全无损。

退役合同表保留历史数据，没有恢复其 API 或建立历史兼容运行路径。序列使用最大 ID 与
SQLite AUTOINCREMENT 高水位，回归覆盖删除高 ID 后不复用旧 ID。

## 当前工程验收

### 正式主节点初始化

根包 `TestImportedPostgresApplicationStartup` 只接受明确命名的隔离副本，在没有 `.env` 的
目录调用实际 `InitResources()` 两次，再重新打开正式 DB。未启动 Provider 轮询/重发 worker，
没有复制另一套迁移实现。每次启动前后核对以下原始字段/资金字段的哈希：

- 用户钱包、邀请额度、请求次数、密码哈希、角色、状态、分组和软删除标记；
- Token、Channel、Task、create attempt、幂等、计费日志交付、Provider exposure；
- 托管素材及组、合同及规则、历史 logs。

全部一致。实际分组列保持 VARCHAR(128)，11 条长分组的尾项仍可被正式过滤器查到。
全部渠道和 Task 经真实 ORM 解码，价格投影可加载，路由注册不冲突。
既有累计用量启动修复两次均为 `corrected=0 unchanged=85 blocked=0`，不存在借搬库重算资金。

### 二进制与 HTTP

前端生产构建后编译当前工作区 Linux arm64 二进制，在只可访问隔离 PostgreSQL 的 Docker
内部网络运行。HTTP 进程为 slave 角色，防止触发 Provider 后台 worker；主节点迁移已由上节
验证。只在 HTTP 副本中新建测试用户与 Key，不使用真实客户凭据：

| 请求 | 结果 |
| --- | --- |
| `GET /api/status` | 200 |
| 未鉴权 `GET /api/channel/` | 401 |
| 未鉴权 `GET /v1/models` | 401 |
| 测试 Key `GET /v1/models` | 200，89 个可见模型 |
| 测试 Key 查询其他用户的视频任务 | 404，不返回任务内容 |

这些结果覆盖启动、鉴权、模型发现与用户隔离，不代替真实推理/扣款或历史 OSS 正文读取验收。
测试使用私有临时 secrets；生产必须保留原实际 secrets 和外部对象存储。

### 回归与构建

- SQLite、MySQL 5.7.44、PostgreSQL 16：旧列升级、长分组、默认分组、重复启动通过。
- 同一三库上的既有 `TestMigrationSchemaStability` 通过。
- Python 8 项回归含真实 PostgreSQL：WAL、禁止覆盖、64 位资金、JSON/BLOB、NULL/空值、
  主键/序列、非空目标拒绝、校验失败整事务回滚、资金改变后校验失败，通过。
- 前端 6 项表单与标签界面测试、TypeScript、涉及文件 lint、生产构建通过。
- 扩大 Go model/controller/common/middleware 包检查通过；`go vet ./model ./controller .` 通过。
- 当前工程完整二进制编译与隔离运行通过。

## 扩大检查仍失败的项目

1. `TestAccessTokenRouteRulesCoverEveryDashboardRoute`：新增
   `POST /api/system-task/evidence-cleanup` 未在 dashboard access-token 路由清单中声明。
2. service 中 `TestRunVideoResultTransferPassTransfersAndSigns`、
   `TestRunVideoResultTransferPassFeicaiCredentialAndSameOrigin`、
   `TestRunVideoResultTransferPassSynlinkLastFrame` 及失败分类测试：新视频结果托管模块的
   ready/失败数量不符合预期。这些测试使用自身 fixture，并非本次导入库。
3. 前端全仓 `bun run lint` 有既有规则错误，本次涉及文件已单独通过。

保留并如实记录失败，不删除断言、不屏蔽测试、不改变其它正在实施的功能来换取迁移通过。
正式放行前需要解决相关工作区失败，并使用最终停写快照验收真实部署环境、负载与外部链路。

## 本机证据

证据目录 `/tmp/oneapi-pg-acceptance.3pPQxB/`，报告限制当前操作者访问，未写入 Git：

- `import-v128.json`、`verify-v128.json`：全量数据校验；
- `application-v128.log`：真实主节点初始化与不变量；
- `group-three-databases-final.log`：三库分组及既有 schema 回归；
- `http-acceptance.json`：隔离二进制 HTTP 验收；
- `additional-core-tests.jsonl`：未通过的扩大检查。

目录也保存本机临时连接配置，不能整体上传、粘贴到工单或提交。数据以私有 Docker volume
保留；正式迁移仍需重新取得最终快照，不能以本机演练库替代线上最新事实。
