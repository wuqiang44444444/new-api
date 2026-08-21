#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
database_path="${1:-${script_dir}/one-api.db}"
customer_model="seedance-2.0-cmcc"

if ! command -v sqlite3 >/dev/null 2>&1; then
  echo "缺少 sqlite3，无法写入渠道凭据。" >&2
  exit 1
fi
if [[ ! -f "${database_path}" ]]; then
  echo "数据库不存在：${database_path}" >&2
  exit 1
fi

IFS='|' read -r channel_id match_count <<<"$(
  sqlite3 "${database_path}" "
    SELECT COALESCE(MIN(id), 0), COUNT(*)
    FROM channels
    WHERE type = 61
      AND status = 2
      AND models = '${customer_model}'
      AND json_extract(settings, '$.video_upstream_protocol') = 'modelark_v3_cmcc'
      AND json_extract(settings, '$.asset_upstream_protocol') = 'cmcc_aicc_assets_v2'
      AND json_extract(model_mapping, '$.\"${customer_model}\"') = 'doubao-seedance-2.0';
  "
)"
if [[ "${match_count}" != "1" || "${channel_id}" == "0" ]]; then
  echo "需要且只能存在一个手动禁用、协议与模型映射完全匹配的移动云 Seedance 渠道；数据库未修改。" >&2
  exit 1
fi

echo "将为渠道 ID ${channel_id} 写入移动云视频 API Key 与素材 AK/SK。"
echo "输入内容不会回显，也不会写入命令历史或脚本文件。"
read -r -s -p "视频 API Key：" video_api_key
echo
read -r -s -p "素材 Access Key：" asset_access_key
echo
read -r -s -p "素材 Secret Key：" asset_secret_key
echo

credential_pattern='^[A-Za-z0-9._~+/=-]+$'
if [[ ! "${video_api_key}" =~ ${credential_pattern} ||
      ! "${asset_access_key}" =~ ${credential_pattern} ||
      ! "${asset_secret_key}" =~ ${credential_pattern} ]]; then
  echo "凭据为空或含有未允许字符；数据库未修改。" >&2
  unset video_api_key asset_access_key asset_secret_key
  exit 1
fi

backup_dir="${script_dir}/.local-tests/cmcc-backups"
mkdir -p "${backup_dir}"
backup_path="${backup_dir}/$(basename "${database_path}").before-cmcc-credentials-$(date +%Y%m%d%H%M%S).bak"
cp "${database_path}" "${backup_path}"

now="$(date +%s)"
sqlite3 "${database_path}" <<SQL
.bail on
BEGIN IMMEDIATE;
CREATE TEMP TABLE cmcc_channel_guard (valid INTEGER NOT NULL CHECK (valid = 1));
INSERT INTO cmcc_channel_guard (valid)
SELECT CASE WHEN COUNT(*) = 1 THEN 1 ELSE 0 END
FROM channels
WHERE id = ${channel_id} AND status = 2;
UPDATE channels
SET key = '${video_api_key}'
WHERE id = ${channel_id} AND status = 2;
INSERT INTO channel_asset_credentials (
  channel_id, access_key_id, secret_access_key, created_time, updated_time
) VALUES (
  ${channel_id}, '${asset_access_key}', '${asset_secret_key}', ${now}, ${now}
)
ON CONFLICT(channel_id) DO UPDATE SET
  access_key_id = excluded.access_key_id,
  secret_access_key = excluded.secret_access_key,
  updated_time = excluded.updated_time;
COMMIT;
SQL

unset video_api_key asset_access_key asset_secret_key

echo "凭据已写入；渠道仍保持手动禁用。"
echo "备份：${backup_path}"
echo "请重启服务，再执行视频与素材只读连通性测试；测试通过后才能启用渠道。"
