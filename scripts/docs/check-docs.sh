#!/usr/bin/env bash
set -euo pipefail

required_files=(
  "AGENTS.md"
  "README.md"
  "docs/README.md"
  "docs/00-context/README.md"
  "docs/10-product/README.md"
  "docs/20-architecture/README.md"
  "docs/30-engineering/README.md"
  "docs/40-operations/README.md"
  "docs/50-planning/README.md"
  "docs/60-marketing/README.md"
  "docs/70-research/README.md"
  "docs/80-dev/README.md"
  "docs/90-ui-ux/README.md"
  "docs/99-archive/README.md"
  "docs/00-context/项目简报.md"
  "docs/00-context/需求清单.md"
  "docs/00-context/硬约束.md"
  "docs/10-product/业务流程.md"
  "docs/10-product/验收标准.md"
  "docs/20-architecture/架构概览.md"
  "docs/20-architecture/异步任务与计费事实架构.md"
  "docs/30-engineering/本地搭建.md"
  "docs/30-engineering/命令清单.md"
  "docs/30-engineering/人工智能编码指南.md"
  "docs/40-operations/环境配置.md"
  "docs/40-operations/运维手册.md"
  "docs/50-planning/路线图.md"
  "docs/50-planning/变更记录.md"
  "docs/60-marketing/产品定位.md"
  "docs/60-marketing/发布说明.md"
  "docs/70-research/参考资料.md"
  "docs/70-research/备选方案.md"
  "docs/90-ui-ux/页面清单.md"
  "docs/90-ui-ux/交互模式.md"
)

for file in "${required_files[@]}"; do
  test -s "$file"
done

readme_dirs=(
  "docs/00-context"
  "docs/10-product"
  "docs/20-architecture"
  "docs/30-engineering"
  "docs/40-operations"
  "docs/50-planning"
  "docs/60-marketing"
  "docs/70-research"
  "docs/80-dev"
  "docs/90-ui-ux"
  "docs/99-archive"
)

for dir in "${readme_dirs[@]}"; do
  grep -q "^## 目标" "$dir/README.md"
  grep -q "^## 放什么" "$dir/README.md"
  grep -q "^## 不放什么" "$dir/README.md"
done

grep -q "YYYY-MM-DD-" "docs/80-dev/README.md"
grep -q "docs/README.md" "AGENTS.md"

decision_dir="docs/20-architecture/decisions"
decision_index="$decision_dir/README.md"
test -s "$decision_index"

decision_count=0
while IFS= read -r decision_file; do
  decision_count=$((decision_count + 1))
  decision_name="${decision_file##*/}"
  decision_prefix="${decision_name%%-*}"
  decision_adr="$(sed -n 's/^adr: //p' "$decision_file" | head -n 1)"

  decision_status="$(sed -n 's/^status: //p' "$decision_file" | head -n 1)"
  case "$decision_status" in
    accepted)
      ;;
    superseded)
      decision_successor="$(sed -n 's/^superseded-by: //p' "$decision_file" | head -n 1 | tr -d '"')"
      test -n "$decision_successor"
      ;;
    *)
      echo "invalid ADR status in $decision_file: $decision_status" >&2
      exit 1
      ;;
  esac
  test "$decision_adr" = "$decision_prefix"
  grep -Fq "($decision_name)" "$decision_index"

  while IFS= read -r relative_target; do
    test -f "$decision_dir/$relative_target"
  done < <(grep -oE '\]\(\./[^)#[:space:]]+\.md' "$decision_file" | sed 's#^](\./##' || true)
done < <(find "$decision_dir" -maxdepth 1 -type f -name '*.md' ! -name 'README.md' | sort)

test "$decision_count" -gt 0

while IFS= read -r indexed_target; do
  test -f "$decision_dir/$indexed_target"
done < <(grep -oE '\]\([0-9]{4}-[^)#[:space:]]+\.md' "$decision_index" | sed 's#^](##' || true)

grep -q '^## ADR 判定门禁$' "docs/30-engineering/人工智能编码指南.md"
grep -q '不得为了识别或拒绝 Link SKU 而修改' "docs/00-context/硬约束.md"

echo "docs:check passed"
