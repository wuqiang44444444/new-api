---
status: historical
owner: Dev Team
last-reviewed: 2026-10-15
archived-at: 2026-10-16
source-path: docs/80-dev/2026-10-15-SQLite到PostgreSQL导入脚本与全量数据复验.md
superseded-by:
  - docs/40-operations/SQLite全量迁移PostgreSQL手册.md
---

# SQLite 到 PostgreSQL 导入脚本与全量数据复验

## 问题与目标

本次验收对象是独立导入脚本及数据库内容：确认本工程 `one-api.db` 中全部表、行、字段、
主键、索引与自增高水位正确导入 PostgreSQL，并核对当前工程初始化造成的数据变化。
渠道生成能力、视频结果、OSS 下载和全仓 lint 不作为数据库搬运完整性的判据。

已运行的 `oneapi_import_verified_v128` 有业务写入与后台清理，不能再承诺它与离线 SQLite
逐行相等。因此重新实际导入一个没有业务进程连接的数据库，并分别核对导入完成、工程
初始化后和已有运行库的状态。全量迁移保持独立脚本，未把搬库逻辑加入应用启动代码。

## 当前实际情况

### 原始备份与实际重新导入

- 源：工程根目录 `one-api.db`，581,726,208 字节。
- 源 SHA-256：`47ef70d648f73621daf17407f567253379a4fded9a9848b3e2359eaef556aca3`。
- 复验前后源文件 SHA-256 相同；检查时 WAL 为零字节，没有忽略已提交 WAL 数据。
- 本机 PostgreSQL 16；重新实际导入 `oneapi_migration_recheck_20261015`。
- 使用当前 `scripts/migrate-sqlite-to-postgres.py` 执行 import，再另起命令执行只读 verify。
- 99 张源表、670,809 行；事务内和提交后的所有源字段规范化 SHA-256 均一致。

### 独立比较结果

另写只读检查器，直接读取 SQLite 与 PostgreSQL，按原主键逐行逐字段比较；不调用导入脚本的
`convert`、`normalized_row` 或摘要函数，以降低同一实现同时写错与验错的风险。检查不输出源行、
凭据、连接密码或 Task 私有字段值。

| 检查对象 | 实测结果 |
| --- | --- |
| 表集合 | 99 张全部存在，无漏表或额外导入表 |
| 行与主键对应 | 670,809 行全部对应，无缺行、额外行或主键变化 |
| 全字段 | 11,565,592 个字段值比较通过 |
| 主键与普通/唯一索引 | 551 个检查通过，核对列顺序、升降序、唯一性与部分索引条件 |
| 自增序列 | 84 个检查通过，拥有关系及高水位不低于源最大 ID / sqlite_sequence |
| 列默认值 | 1,243 个列默认值检查通过；84 个序列默认值另外检查 |
| 列类型与非空约束 | 符合导入类型映射，没有意外类型或约束差异 |
| 长分组 | 11 条、最长 115 字符，VARCHAR(128) 完整保留，无裁剪 |

整数、浮点数、定点数、布尔、JSON/BLOB、Unicode、NULL 与空字符串均按源列语义比较。
JSON 文本与 SQLite BLOB 中的 UTF-8 JSON 保留正文内容；JSON 数字不经过浮点解析后重写。
主库与日志同库，历史 logs 223,140 行、tasks 3,871 行、create attempts 3,886 行均完整。

**精度例外必须保留在结论中：** PostgreSQL timestamp 只能存微秒，本次显式使用
`--allow-timestamp-truncation`，56 个软删除时间截去微秒以下尾数：models 15、tokens 39、
users 1、vendors 1。两个独立检查均确认这个数量；源 SQLite 原值保留。该结果是按已声明
类型转换规则完整一致，不能称为所有原始字节和纳秒完全无损。其余字段未发现值变化。

### 当前工程重复初始化后

从重新导入库创建独立副本 `oneapi_acceptance_recheck_20261015`，运行当前工程的
`TestImportedPostgresApplicationStartup`，实际执行两次 `InitResources()`、正式 ORM
读取和数据库重新打开；测试通过。随后再次独立核对全部 99 张源表和所有原列。

初始化后的实际变化如下，不能将应用初始化描述成所有表永久不发生变化：

1. `user_sessions` 删除 25 条在初始化前已经过期的会话，由
   `main.go` 中 `StartAuthArtifactCleanup()` 与 `model.DeleteExpiredUserSessions` 执行。
2. `casbin_rule` 的 3 条内置权限记录重建主键 ID；按全部权限字段比较，源与目标均为 7 条，
   权限内容集合完全相同。来源是 `authz.Init()` 重置与生成内置角色规则。
3. 应用新增 `task_video_result_objects` 表，并按当前模型调整 31 个既有列的声明类型；
   对原列的全字段比较没有发现伴随这些声明调整的数据值损失。

除上述两个表的行身份变化外，其余源行没有缺失、增加或字段变化。仍对应的 670,781 条
源行、11,565,168 个字段值比较通过。用户资金、Key 额度、渠道、Task、冻结执行与计费事实、
日志等全部原始字段保持一致；全部源索引和序列高水位继续有效。

### 已有运行库的全量核对与资金对账

对 `oneapi_import_verified_v128` 也逐表检查全部原列。原始 83 条渠道、153 条 Token、
85 条用户、3,871 条 Task、3,886 条 attempt 与 223,140 条日志均能按原主键找到；所有
原始 Task、attempt 与日志的原字段仍一致。运行库还有正常新增业务记录和后台维护变化，
原始整库相等检查会如实报告差异，不把这些差异隐藏为迁移通过。

已核对运行后的资金差异：

- 新增 3 条已结算 Task，合计 quota 207,174；新增 1 条 held attempt，quota 551,183。
- 钱包与 Token 剩余额度较备份减少 758,357，恰好等于 207,174 + 551,183。
- 用户 used_quota 增加 207,174、request_count 增加 3，分别对应上述新结算与新请求事实。
- 运行库的权限 ID、过期会话、渠道巡检状态、系统任务、统计和证据过期标记等变化，与
  初始化或后台维护行为对应。与迁移完成时逐行不一致，不等于导入漏数据。

### 独立脚本回归

8 项回归全部通过，其中 4 项实际使用本机 PostgreSQL：

- WAL 快照包含已提交数据，备份输出不覆盖已有文件；
- 非法布尔、未知类型、unsupported schema、溢出/舍入/超长与默认纳秒精度损失明确拒绝；
- 64 位资金、JSON/BLOB、NULL/空字符串、原 ID 与已删除高 ID 的序列高水位正确；
- 非空目标拒绝，已有目标记录不改动；
- 校验失败时 DDL、COPY 与索引创建事务全部回滚；
- 金额被修改后，只读完整校验能检测失败。

## 优化方案与交付结论

这份备份的独立导入脚本及全量数据复验通过，未发现漏表、漏行、资金误差、JSON 改写、
分组裁剪、索引遗失或序列回退。工程初始化的会话清理与内置权限 ID 重建、以及 56 个时间
精度转换已明确记录。该结论限定于本次备份与本机 PostgreSQL 16，不代表正式数据库已切换。

正式导入必须针对最终停写快照重新执行 import 和 verify，在应用开始产生业务写入之前保存
一致性报告。应用启动后再验证资金/任务等持久事实，并按应用维护规则解释会话和权限变化。
步骤沿用 [SQLite 全量迁移 PostgreSQL 手册](../40-operations/SQLite全量迁移PostgreSQL手册.md)。

脱敏证据保存在本地忽略目录 `.local-tests/sqlite-postgres-recheck-20261015/`：
`import.json`、`verify.json`、`baseline-independent.json`、`defaults.json`、
`after-startup-independent.json`、`application-validation.json`、
`live-independent.json`、`live-financial-reconciliation.json` 与对应执行日志。
独立比较脚本保存在本机 `/tmp/oneapi-independent-db-audit.py`，不读取外部 Provider 或 OSS。

`after-startup-independent.json` 与 `live-independent.json` 的原始比较结果保留为不相等；
初始化差异的逐项解释与放行结论单独记录在 `application-validation.json`，不篡改原始校验结果。
