---
status: current
owner: Dev Team
last-reviewed: 2026-10-14
---

# SQLite 全量迁移 PostgreSQL 手册

## 适用范围与结论

适用于本仓库的 SQLite `one-api.db`，主库与日志同库，迁入专用、空的 PostgreSQL 数据库。
使用独立脚本 [migrate-sqlite-to-postgres.py](../../scripts/migrate-sqlite-to-postgres.py) 手动执行，
全量导入不接入应用启动代码，不修改应用数据库选择逻辑；脚本不启动应用、Redis、后台任务或 Provider 调用。
渠道分组容量修复由正式模型、schema 比较及前后端保存校验表达，不把全量搬库逻辑写入应用。
脚本不适用于把另一个版本的原始 One API 项目直接升级为本项目，也不提供在线增量同步。

推荐流程：完整副本演练 → 排除应用兼容阻断 → 停止受理与排空写入 → 最终快照 → 全量导入
与校验 → 单实例启动验收 → 开放流量。数据库转换和应用版本升级分开实施，演练使用实际部署
的镜像 digest／二进制，不能根据当前工作区可编译就认定与线上版本相同。

数据校验通过只证明迁移脚本完成了数据转换。当前工程已在导入副本上通过重复主节点初始化、
资金与冻结事实校验及 HTTP 路由验证；外部存储、真实 Provider 与生产负载仍须部署验收。
历史 64 字符版本不得用来启动新库；工程证据见
[导入与工程验收记录](../80-dev/2026-10-14-SQLite全量迁移PostgreSQL导入与工程验收.md)。

## 备份中已核实的风险

2026-10-14 对工作区的线上备份进行了只读检查，并在临时 PostgreSQL 16 中演练全量导入：

| 项目 | 检查结果 | 处理要求 |
| --- | --- | --- |
| 文件 | 581,726,208 字节，约 555 MiB | 保留原始文件；正式切换重新取得最终快照 |
| 数据 | 99 张业务表，670,809 行 | 保留所有表、所有列和原始主键；不能只导入当前 ORM 表清单 |
| SQLite 完整性 | `integrity_check=ok`，无外键检查错误 | 最终快照再次检查 |
| 用户／Key／渠道 | 85／153／83 行 | 登录、鉴权、权限、模型与分组行为需应用验收 |
| 日志／视频 Task | 223,140／3,871 行 | 不能丢历史日志或从日志重新构造资金状态 |
| 视频待核实 | 21 条 `RECONCILIATION_REQUIRED`，计费 `pending` | 保留现状；不能批量置失败或结算 |
| 创建结果未知 | 200 条 `unknown` | 这批记录的 hold 均已 released；unknown 不代表仍在占款，也不允许重发 |
| 系统任务 | 180,163 行，其中 1 条 running；1 行租约 | 保留任务与租约，旧实例全部退出后按既有过期恢复逻辑处理 |
| 渠道分组长度 | 11 行超过 64 字符，最长 115 | 导入与当前应用均使用 VARCHAR(128)，完整保留；超过 128 在导入及保存时拒绝 |
| 时间精度 | 56 个非空软删除时间存在非零纳秒尾数 | 默认拒绝；显式允许后截到微秒，原始时间保留在原备份 |

56 个时间分别来自 `models.deleted_at` 15 行、`tokens.deleted_at` 39 行、`users.deleted_at`
1 行和 `vendors.deleted_at` 1 行。原有整数时间戳、资金、任务状态和 JSON 不因此改写。
PostgreSQL timestamp 的存储精度为微秒，不能把本次转换称为全部原始字节无损。
依据：[PostgreSQL 日期时间类型](https://www.postgresql.org/docs/16/datatype-datetime.html)。

**渠道分组容量已修复：** [Channel.Group](../../model/channel.go) 的 ORM 声明为
`type:varchar(128);size:128`，整个 CSV 列表最多 128 个 Unicode 字符（包含逗号）。新增、编辑及
标签批量修改均在后端校验；渠道表单和两个标签编辑界面在超长时显示错误并阻止保存。
界面校验不能代替后端校验，因为直接调用 API 可以绕过界面。

项目使用的 GORM 版本会在等价默认值比较时覆盖此前识别出的列长度变化，因此已有 MySQL／
PostgreSQL VARCHAR(64) 列不能只依赖修改声明。现有两个 schema migrator 的列比较入口仅对
Channel.Group 排除已经等价的默认值比较，保留长度比较与原模型默认值；没有新增全量迁移入口。
SQLite、MySQL 5.7.44 与 PostgreSQL 16 已验证旧列升级、长分组保存、默认分组和再次启动行为。

旧版本 `varchar(64)` 声明仍可能通过显式类型转换截断超长分组，因此新库必须运行已修复版本，
不能将数据库已扩容理解为旧镜像也安全，也不能只以“启动没有报错”证明分组未损坏。
依据：[PostgreSQL 字符类型](https://www.postgresql.org/docs/16/datatype-character.html)。
导入始终不截短源分组、不删除渠道、不跳过资金检查。未来源库若出现超过 128 的值，预检查
明确拒绝，必须先审核容量合同与存量事实，不能静默删除分组来通过导入。

旧 Asset、publication 等退役表在备份中仍含历史记录。脚本保留这些表供全量核对，不重新开放
退役 API，不建立历史兼容 decoder，也不为旧素材构建新的运行时资源事实。

## 脚本保障与边界

- `snapshot` 使用 SQLite Backup API，包含已提交 WAL，不直接复制在线 `.db` 文件。
  参见 [SQLite Backup API](https://www.sqlite.org/backup.html)。
- `preflight`、`import`、`verify` 以只读事务访问源库，禁止带非空 WAL 的源文件，核对源文件
  SHA-256；执行期间源文件或 WAL 变化即拒绝。
- 所有 SQLite 整数转为 PostgreSQL BIGINT，实数转为 DOUBLE PRECISION，避免钱包整数缩水
  与 real 浮点降为 32 位。布尔列按明确清单转为 BOOLEAN，不根据某列当前只有 0/1 猜测类型。
- JSON 列中 GORM 写入的 SQLite BLOB 先严格按 UTF-8 解码，存为 PostgreSQL JSON，保留 JSON
  文本内容；NULL 与空字符串保持区别。JSON 之外的 BLOB 存为 BYTEA。
- 渠道分组列由历史 VARCHAR(64) 扩为 VARCHAR(128)，匹配当前工程合同；其他小数不能四舍五入，
  普通字符串不能裁剪，未知列类型、触发器、视图、生成列、自定义排序、
  外键、CHECK 或未支持的索引表达式会拒绝，不能跳过后声称全量成功。
- 保留主键、普通索引、唯一索引和已支持的软删除条件唯一索引。SQLite 内部自动索引改为稳定
  的 PostgreSQL 名称；整型 `id` 主键获得独立序列，起点考虑最大 ID 和 `sqlite_sequence`
  历史高水位，不复用已删除的历史 ID。
- 目标必须是空的专用 UTF-8 数据库，并明确传入实际数据库名称。存在任何用户表、视图或序列
  就拒绝；不执行 DROP/TRUNCATE、不覆盖、不合并现有数据。目标禁止应用同时访问。
- 建表、COPY、建索引、序列设置和全表校验在一个事务内完成，任一失败回滚。全表哈希按主键
  顺序和明确定义的类型转换计算，覆盖所有字段，不使用随机抽样代替资金与 Task 校验。
- 连接串只从环境变量读取；推荐 PostgreSQL service + 密码文件，不把密码放进命令参数。
  错误仅显示阶段、异常类别或 SQLSTATE，不显示失败行、Provider 响应或连接串。
- 报告以 `0600` 权限排他创建，不覆盖旧报告，不包含源行、凭据或 options 值。失败可能留下
  空／未完成报告；只有 `result=complete` 才是完成报告。

当前完整副本演练通过；不等于已经验证生产 PostgreSQL 的存储、负载、网络、TLS、权限、
HA 与备份恢复。当前工程的本地启动验证已通过；大库的单事务会消耗 WAL 和索引临时空间，容量与耗时以目标环境
的完整演练为依据，不根据 SQLite 文件大小承诺停机时间。

## 手动演练步骤

以下路径和数据库名均为示例。先复制脚本到迁移机器，使用独立工作目录；新备份、报告与 dump
不要进入 Git。所有命令在应用尚未连接目标数据库时执行。

### 1. 准备 Python 与连接

```bash
mkdir -p /data/oneapi-migration
chmod 700 /data/oneapi-migration
python3 -m venv /data/oneapi-migration/venv
/data/oneapi-migration/venv/bin/pip install 'psycopg[binary]==3.3.6'
```

`snapshot` 与 `preflight` 只用 Python 标准库；`import` 与 `verify` 需要 psycopg 3。
管理员先建立专用、空的 UTF-8 演练数据库，并授权迁移账号建表、建索引与建序列。
通过本机私有 PostgreSQL service 配置提供 host、port、user、dbname、TLS 验证参数，通过
密码文件提供密码，文件权限均限制为当前操作者；不要把真实配置内容发到聊天或日志。

```bash
export ONEAPI_MIGRATION_PG_DSN='service=oneapi-migration-rehearsal'
```

service 对应数据库名须与后续 `--expect-database` 完全相同。迁移优先使用 PostgreSQL 直连
端点，不通过未经完整演练的事务池代理。
依据：[PostgreSQL 连接 service 文件](https://www.postgresql.org/docs/16/libpq-pgservice.html)。

### 2. 生成独立快照

```bash
python3 scripts/migrate-sqlite-to-postgres.py snapshot \
  --source /path/to/one-api.db \
  --output /data/oneapi-migration/rehearsal.db
```

脚本不覆盖已经存在的快照。线上仍在写入时这只是演练快照，不是正式切换基线；正式切换必须
停止全部写入后重新生成快照。源库保持原位，不手工移动或删除 WAL 文件。

### 3. 只读检查

默认禁止时间精度损失，本次备份会在发现带纳秒尾数的软删除时间时拒绝。审核该转换后，显式
传入以下选项；后续 import／verify 使用相同选项。新快照可能出现不同列，必须重新阅读报告
的 `timestamp_truncations`，不能把此次 56 行视为永久固定清单。

```bash
python3 scripts/migrate-sqlite-to-postgres.py preflight \
  --source /data/oneapi-migration/rehearsal.db \
  --allow-timestamp-truncation \
  --report /data/oneapi-migration/preflight.json
```

检查 `result`、`source_sha256`、逐表行数、schema、`states`、`timestamp_truncations` 与
`application_blockers`。当前快照的 11 条长分组在 128 字符以内，已通过当前工程验收；preflight
不替代实际部署验收。源文件不同，SHA-256 也不同，不能复制本手册日期下某次演练的摘要。

### 4. 导入空的演练库

把 `--expected-source-sha256` 替换为刚才 preflight 报告中的实际摘要。数据库名称必须与
service 对应数据库一致，报告使用新路径。

```bash
/data/oneapi-migration/venv/bin/python scripts/migrate-sqlite-to-postgres.py import \
  --source /data/oneapi-migration/rehearsal.db \
  --allow-timestamp-truncation \
  --expected-source-sha256 <preflight中的source_sha256> \
  --expect-database oneapi_migration_rehearsal \
  --report /data/oneapi-migration/import.json
```

成功输出同时包含 `import=committed` 与 `data_verification=all_tables_match`。导入包含全表
校验，再执行一次独立只读校验可以确认提交后的目标库：

```bash
/data/oneapi-migration/venv/bin/python scripts/migrate-sqlite-to-postgres.py verify \
  --source /data/oneapi-migration/rehearsal.db \
  --allow-timestamp-truncation \
  --expected-source-sha256 <preflight中的source_sha256> \
  --expect-database oneapi_migration_rehearsal \
  --report /data/oneapi-migration/verify.json
```

独立 verify 校验所有源表的所有源列，不要求目标后来新增的表／列为零，不把它当作完整 ORM
schema 验收。应用开始写入后，源/目标哈希自然可能不同，不能再用它推断全部变化都是迁移损坏。

### 5. 验证应用启动与外部依赖

使用已修复分组容量的版本，先备份刚导入的演练库，再用真实部署版本做启动验收。演练应用必须
与线上 Redis、Provider、邮件、支付和生产对象清理能力隔离；只关闭 HTTP 流量不足以隔离
主节点后台任务。禁止对历史 unknown／在途记录进行 Provider 重放。

启动会运行 AutoMigrate 和显式数据迁移，不仅是“连接 PostgreSQL”。核对迁移前后的渠道类型、
options migration marker、权限、合同、Provider 折扣、历史用量统计与持款期限。应用中存在
用户历史累计用量修正入口，预期变化应另行核实，不能把所有变化都归为数据库格式转换。
对存量数据仍要证明钱包、Key 额度、Task/attempt 资金事实和冻结连接没有无依据变化。

特别核对 `channels.group` 完整内容与该列实际类型，不能只核对应用成功启动。
保留 `SESSION_SECRET` 与实际生效的 `CRYPTO_SECRET`；后者未显式配置时由前者派生。
保持证据加密密钥、Task plugin 历史版本、私有 OSS 对象位置、账号权限及对象内容可用。
数据库只保存对象引用，导入数据库不会复制托管素材、批处理文件、结果或音视频证据正文。

至少验收登录与原 Key 调用、权限与用户隔离、渠道分组/模型路由、历史日志/账单查询、新增
用户/Key/日志的主键生成、一次受控收费与结算、异步结果查询和托管对象读取。对 Provider
操作使用专门测试账号与新测试任务；不能以重发历史任务代替验收。

## 正式切换与回滚

1. 排除应用兼容阻断，在实际目标环境完成完整演练、备份恢复和应用验收，记录导入与索引耗时。
2. 进入维护窗口，停止新请求、管理写入、充值回调与自动化写入，等待已经受理的同步请求完成。
   无法等到全部异步任务终态时保留完整持久状态，不能重置 unknown 或人工批量退款。
3. 若启用 `BATCH_UPDATE_ENABLED`，先排空业务并核实最后一批内存差额已成功落库，再退出所有
   实例。当前关闭数据库的函数不等于显式刷新批量更新，不能把强制退出视为资金已保存。
4. 确认全部实例、后台进程与直接数据库写入方已经停止，再通过 snapshot 取最终快照；同时
   保留旧配置/镜像、加密密钥和必要外部存储备份。最终快照重新 preflight，不能使用旧演练报告。
5. 在新的、空的正式数据库重复 import／verify。此前准备的演练库不能承接正式最终数据，脚本
   不提供清空或增量覆盖。导入后、应用启动前再做一份 PostgreSQL 可恢复备份。
6. 保持 `LOG_SQL_DSN` 未设置，使主库和日志库仍同库，仅手动把部署配置中的 `SQL_DSN`
   改为经过验证的 PostgreSQL URL；保留原有 secrets、对象存储与业务配置。先启动一个实例，
   完成 schema 与资金核验，再开放流量并恢复其余实例。
7. 为新实例使用经核实的缓存隔离/重建方案，避免读到旧余额。不要对共享 Redis 执行 FLUSHALL。
   按实际 PostgreSQL 连接预算配置连接池；应用默认每进程最大连接数 1000，不宜原样照搬。

脚本不修改旧 SQLite，所以**首次任何新写入发生前**可以停止目标实例、恢复旧配置并用最终
SQLite 快照回退。后台任务、租约、统计与启动数据迁移也属于写入，不只客户流量算写入。
PostgreSQL 已发生新扣款、退款、充值或任务推进后，禁止直接切回旧 SQLite，否则会丢失新事实；
优先在 PostgreSQL 上向前修复。确需回退时必须先停写并制定覆盖所有新增资金与业务事实的
回迁/对账方案，不能用“再导入一遍旧备份”替代。

## 失败处置与完成信号

- preflight／COPY／索引／哈希失败：保持旧部署配置，审阅脱敏错误阶段；不跳过失败行。
  事务未提交时 PostgreSQL 会回滚全部目标 DDL 与数据，再次确认目标为空后使用新报告重跑。
- COMMIT 附近网络中断或报告写入失败：提交结果可能需要核实。目标为空才重跑 import；目标
  存在表时先运行 verify，不能自动删库。脚本也不会在非空目标重跑导入。
- 只接受同时满足“导入提交、所有源表字段哈希匹配、明确转换已审核、应用兼容阻断排除、启动
  与资金/对象验收完成”的切换结果；没有真实验收时不宣称生产可用。

回归测试位于 [test_migrate_sqlite_to_postgres.py](../../scripts/test_migrate_sqlite_to_postgres.py)：
覆盖 WAL 快照、禁止覆盖、无私有值错误输出、64 位资金、JSON BLOB、NULL/空值、复合主键、
软删除唯一索引、历史序列高水位、非空目标拒绝、校验失败整事务回滚与修改资金后的哈希失败。
真实 PostgreSQL 测试只应对有 CREATE DATABASE 权限的临时测试服务器运行。

```bash
python3 -m unittest discover -s scripts -p 'test_migrate_sqlite_to_postgres.py'
```

设置 `ONEAPI_MIGRATION_TEST_DSN` 后同一命令运行 PostgreSQL 集成用例；未设置时集成用例
会明确跳过，不能将此情况表述为真实 PostgreSQL 验证通过。
