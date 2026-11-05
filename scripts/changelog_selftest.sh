#!/usr/bin/env bash
set -euo pipefail

SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/changelog.sh"
export GITHUB_REPOSITORY="test/repo"

WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

fail() {
	printf 'FAIL: %s\n' "$*" >&2
	exit 1
}
pass() { printf 'PASS: %s\n' "$*"; }

assert_contains() {
	case "$1" in
	*"$2"*) return 0 ;;
	*) fail "$3：期望包含「$2」，实际输出：$(printf '%s' "$1" | tr '\n' ' ')" ;;
	esac
}

assert_not_contains() {
	case "$1" in
	*"$2"*) fail "$3：不应包含「$2」，实际输出：$(printf '%s' "$1" | tr '\n' ' ')" ;;
	*) return 0 ;;
	esac
}

REPO="${WORK}/repo"
mkdir -p "${REPO}"
cd "${REPO}"
git init -q -b main
git config user.name tester
git config user.email tester@example.com

commit() {
	echo "$1" >>log.txt
	git add -A
	git commit -qm "$1"
}

commit "feat: 初始功能"
ROOT="$(git rev-parse HEAD)"
git tag v1.0.0

commit "feat: 新增 A"
commit "fix: 修复 B"
commit "docs: 更新说明"
git tag v1.1.0

git checkout -q -b side v1.0.0
commit "feat: 旁支功能"
git tag v9.9.9
git checkout -q main

commit "feat: 新增 C"
git tag v1.2.0

out="$(bash "${SCRIPT}" v1.1.0 2>/dev/null)"
assert_contains "${out}" "新增 A" "v1.1.0 应包含上版本之后的 feat"
assert_contains "${out}" "修复 B" "v1.1.0 应包含上版本之后的 fix"
assert_contains "${out}" "更新说明" "v1.1.0 应包含上版本之后的 docs"
assert_not_contains "${out}" "初始功能" "v1.1.0 不应包含上一个版本已有的提交"
assert_not_contains "${out}" "旁支功能" "v1.1.0 不应包含旁支提交"
assert_contains "${out}" "compare/v1.0.0...v1.1.0" "v1.1.0 的对比链接应指向 v1.0.0"
pass "常规区间：只统计 v1.0.0..v1.1.0"

out="$(bash "${SCRIPT}" v1.2.0 2>/dev/null)"
assert_contains "${out}" "新增 C" "v1.2.0 应包含自身提交"
assert_not_contains "${out}" "新增 A" "v1.2.0 不应包含 v1.1.0 已发布的提交"
assert_not_contains "${out}" "修复 B" "v1.2.0 不应包含 v1.1.0 已发布的提交"
assert_contains "${out}" "compare/v1.1.0...v1.2.0" "v1.2.0 的基线应为最近的祖先 tag v1.1.0"
pass "旁支干扰：基线取最近的祖先 tag，而不是版本号最大的 tag"

out="$(bash "${SCRIPT}" v1.0.0 2>/dev/null)"
assert_contains "${out}" "首个版本" "首个版本应有明确说明"
assert_not_contains "${out}" "初始功能" "首个版本不应把全量历史当成本版变更"
assert_contains "${out}" "commits/v1.0.0" "首个版本的链接应指向提交列表"
pass "首个版本：不回退成全量历史"

out="$(CHANGELOG_BASE=v1.0.0 bash "${SCRIPT}" v1.2.0 2>/dev/null)"
assert_contains "${out}" "新增 A" "显式基线应把区间起点前推到 v1.0.0"
assert_contains "${out}" "新增 C" "显式基线不应丢掉本版本提交"
assert_contains "${out}" "compare/v1.0.0...v1.2.0" "显式基线应体现在对比链接里"
pass "显式基线：CHANGELOG_BASE 覆盖自动推导"

old_base="$(git tag -l 'v*' --sort=-v:refname | grep -v '^v1.2.0$' | head -n1)"
old_commits="$(git log --no-merges --pretty=format:'%s' "${old_base}..v1.2.0")"
printf '修复前逻辑选出的基线：%s\n' "${old_base}"
assert_contains "${old_commits}" "新增 A" "旧逻辑应复现「基线取版本号最大 tag」导致的历史混入"
pass "已复现修复前缺陷（基线 = ${old_base}，混入已发布提交）"

out="$(AUTHOR_MAP="tester=test-user" bash "${SCRIPT}" v1.1.0 2>/dev/null)"
assert_contains "${out}" "by @test-user" "AUTHOR_MAP 命中时应使用映射后的 GitHub 账号"
assert_not_contains "${out}" "by @tester" "映射生效后不应再输出未映射的作者名提及"

out="$(bash "${SCRIPT}" v1.1.0 2>/dev/null)"
assert_contains "${out}" "by tester" "未映射且非 noreply 邮箱时应只写作者名"
assert_not_contains "${out}" "by @" "未解析出 GitHub 账号时不应输出 @ 提及"

printf '\n全部用例通过\n'
