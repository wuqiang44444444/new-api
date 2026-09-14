---
status: current
owner: Dev Team
last-reviewed: 2026-09-14
---

# SQLite 历史日志用户名补齐手册

## 适用范围

独立脚本 [repair-log-usernames.py](../../scripts/repair-log-usernames.py) 仅使用 Python 3 标准库。
无需更新、重启或替换网关代码；支持用户与日志同库，或者位于两个 SQLite 文件中。
适用于具有 `logs.id` 单列主键的 SQLite 原生日志表；不支持 MySQL、PostgreSQL、ClickHouse。
本地临时数据库已覆盖同库、分库、WAL 备份、重跑及失败恢复；尚未执行线上验证。

脚本只更新 `logs.username`，目标限定为预览时用户名为 NULL 或空字符串、且 `user_id` 能关联到非空
用户名的记录。空白字符串用户名不归入本次补齐。当前用户表中的软删除记录仍可按稳定 ID 提供用户名；
物理删除、无效用户 ID 或用户表用户名为空的记录计入 unresolved，不猜测、不补写。
补入的是**预览时用户表中的用户名**，并非恢复请求发生时的历史名称；已有日志用户名保持原值。

旧线上代码仍可能产生新的空用户名。该脚本修复已有数据，后端写入路径修复需要另行发布。

## 前置条件

- 找到实际使用的主数据库文件。若配置独立日志 SQLite，另外确认日志文件位置。
- 在能访问数据库文件和 WAL/SHM 文件的同一宿主机或容器内执行，使用有权限的服务账号。
- 安装 Python 3。命令中的 `/data/one-api.db` 和 `/data/repair/` 均为占位路径，必须替换。
- 预览清单、完整备份和执行日志放入管理员专用目录；清单含用户名，完整日志库备份含原有私有数据，
  不上传聊天、工单或公共目录。脚本新建清单与备份权限为 `0600`。
- 备份目录需有足够空间容纳完整日志库。不要直接复制活跃数据库的 `.db` 文件作为备份，WAL 中可能
  还有已提交数据。脚本使用 SQLite backup API 生成一致性备份，并检查备份完整性。

## 1. 放置脚本并生成预览清单

只需将 `repair-log-usernames.py` 复制到执行机器，无需部署仓库其余代码。以下命令从脚本所在目录执行。

```bash
install -d -m 700 /data/repair
python3 repair-log-usernames.py \
  --db /data/one-api.db \
  --plan /data/repair/usernames-plan-01.csv
```

默认仅预览，**不修改业务数据**。清单文件必须不存在，避免覆盖前次清单。
脚本将预览开始时的当前时间作为上限（不含上限秒），并限定开始时的最大日志 ID。
成功后输出 `mode=preview` 及以下计数，不输出用户名或原始日志：

| 字段 | 含义 |
| --- | --- |
| `after` / `before` | 本次范围，Unix 秒，左闭右开 |
| `max_id` | 预览开始时的最大日志 ID |
| `missing` | 范围内缺失用户名的日志数 |
| `planned` | 本次可以补齐并写入清单的日志数 |
| `unresolved` | 无法通过用户 ID 取得有效用户名的日志数 |

清单保存日志 ID、用户 ID、创建时间、原值是否 NULL 和目标用户名，充当本次操作的字段级记录。
执行时使用清单中的值；预览后发生的用户改名不会改变本次目标名称。
清单绑定预览机器上的数据库路径及文件身份；不要手工编辑、搬到另一个数据库上执行。

数据量大时可在预览命令上增加 `--after <起始Unix秒> --before <结束Unix秒>` 分段处理。
预览按批读取；执行会将清单加载到内存。百万级记录建议缩短每次时间范围。

## 2. 备份并执行同一清单

核对预览计数后执行：

```bash
python3 repair-log-usernames.py \
  --db /data/one-api.db \
  --plan /data/repair/usernames-plan-01.csv \
  --apply \
  --backup /data/repair/before-usernames-01.db
```

执行模式必须提供新的备份路径，备份已存在时拒绝覆盖。整个日志库备份完成、完整性检查通过后才开始
写入；同库时备份也包含用户等其他表，分库时只备份日志库，因为主库不被修改。

默认每 200 条一个事务，可用 `--batch-size 50` 缩短单批写锁时间，允许范围为 1—500。
无需强制停服，但 SQLite 同时只有一个写事务，建议在低流量时执行；锁等待超过 10 秒会停止。
后台清理任务或其他日志维护操作可能造成冲突，执行时应避开。

每条写入都核对日志 ID、用户 ID、创建时间和原始空值，且只 SET `username`。发现日志表存在触发器时，
脚本会在写入前拒绝执行，防止触发器引起额外副作用。
更新不调用扣费、退款、统计累计、模型或 Provider 接口，也不修改 Redis。

## 分库用法

如果 `users` 位于 `/data/main.db`、`logs` 位于 `/data/logs.db`，预览和执行都要提供 `--log-db`：

```bash
python3 repair-log-usernames.py \
  --db /data/main.db --log-db /data/logs.db \
  --plan /data/repair/usernames-plan-01.csv

python3 repair-log-usernames.py \
  --db /data/main.db --log-db /data/logs.db \
  --plan /data/repair/usernames-plan-01.csv \
  --apply --backup /data/repair/before-usernames-01.db
```

## 3. 核验与成功信号

- `backup=complete` 表示完整备份成功。
- 每批输出 `updated`、`already`、`conflicts`，均为累计计数。
- `result=ok` 且退出码 0 表示清单已处理完，没有身份或原值冲突。
- 首次执行一般为 `updated=planned`、`already=0`；再次执行同一清单，已处理项计入 `already`。
- 用户已删除、日志已改变、日志已被清理等冲突项保持不动，计入 `conflicts`，退出码 3。
- 刷新管理端日志，检查已修复记录能显示用户名，并用用户名筛选核对结果。

脚本写入语句只涉及用户名，本地回归测试对所有其他字段和行数执行精确比较。
线上持续有请求时，全库费用、Token 和日志总数可能正常增加，不能把修复前后全库总量不等视为修复失败。
需要严格核验时，按清单中的日志 ID 与备份比较同一批记录的非用户名字段。

## 失败处置与重跑

错误退出码 1；键盘中断退出码 130。当前未提交批次回滚，之前成功批次仍保留。
修复清单和备份必须保留；排除锁竞争、磁盘空间或权限问题后，使用原清单及**新的备份路径**重跑：

```bash
python3 repair-log-usernames.py \
  --db /data/one-api.db \
  --plan /data/repair/usernames-plan-01.csv \
  --apply --backup /data/repair/before-usernames-resume-01.db
```

预览中断形成的清单缺少结束标记，执行会拒绝；改用新的清单路径重新预览。
脚本不会自动覆盖非空用户名，不会强制处理冲突项。新增空日志不在旧清单中，需重新生成新清单。

**不要把旧备份直接覆盖正在运行的数据库，也不要在线整库回滚**：备份之后可能有新的请求、扣费和
其他业务写入。确需撤销时由维护人员按清单逐条确认当前身份和目标用户名，在独立受控操作中只恢复
对应字段的原空值；完整备份作为取证和恢复依据。本工具不提供整库恢复命令。

## 本地验证命令

```bash
python3 -m unittest discover -s scripts -p test_repair_log_usernames.py -v
```
