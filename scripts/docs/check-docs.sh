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
  "docs/20-architecture/数据模型.md"
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

echo "docs:check passed"
