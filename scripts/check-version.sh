#!/usr/bin/env bash
# 发版前版本一致性校验：只读，不修改任何文件。
# 检查 VERSION 格式、CHANGELOG 条目、tag 重复、文档写死版本号。
set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
fail() { echo "版本校验失败: $*" >&2; exit 1; }

V="$(tr -d '\r\n' < VERSION)"
[ -n "$V" ] || fail "VERSION 为空"

echo "$V" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-(rc|beta|alpha)\.[0-9]+)?$' \
  || fail "VERSION 格式非法: ${V}（应为 X.Y.Z 或 X.Y.Z-rc.N）"

# 预发布版本走 Unreleased，稳定版必须有带日期的正式条目。
case "$V" in
  *-*)
    grep -q '^## \[Unreleased\]' CHANGELOG.md \
      || fail "CHANGELOG.md 缺少 Unreleased 区"
    echo "  预发布版本，CHANGELOG 条目走 Unreleased"
    ;;
  *)
    grep -Fq "## [$V]" CHANGELOG.md \
      || fail "CHANGELOG.md 缺少 [$V] 版本条目"
    grep -Eq "^## \[$V\] - [0-9]{4}-[0-9]{2}-[0-9]{2}" CHANGELOG.md \
      || fail "CHANGELOG.md 的 [$V] 缺少发布日期"
    ;;
esac

# 已发布的 tag 不允许重复发。
if git rev-parse --git-dir >/dev/null 2>&1; then
  if git rev-parse -q --verify "refs/tags/v${V}" >/dev/null; then
    fail "tag v${V} 已存在，禁止重复发布；按 docs/versioning.md 发新的 PATCH"
  fi
fi

# 版本号只应出现在 VERSION 与 CHANGELOG，避免文档随版本漂移。
if grep -En '当前版本 v[0-9]|发布说明.*v[0-9]+\.[0-9]+\.[0-9]+|（v[0-9]+\.[0-9]+\.[0-9]+）' \
    README.md docs/metrics.md docs/release.md; then
  fail "文档里写死了版本号；版本号只应出现在 VERSION 与 CHANGELOG.md"
fi

echo "=== 版本校验通过 ==="
echo "VERSION      ${V}"
echo "git tag      v${V}"
echo "Release      v${V}"
echo "资产         KeyPulse-${V}-win-x64.exe / KeyPulse-${V}-win-x64.zip / SHA256SUMS.txt"
echo "发布目录     dist/KeyPulse-${V}/"
