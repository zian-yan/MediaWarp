#!/usr/bin/env bash
set -euo pipefail

VERSION_TAG_GLOB='v[0-9]*.[0-9]*.[0-9]*'

log() { printf '%s\n' "$*" >&2; }

TAG="${1:-${GITHUB_REF_NAME:-}}"
if [ -z "${TAG}" ]; then
	log "缺少 tag 参数"
	exit 2
fi

if ! git rev-parse --verify -q "${TAG}^{commit}" >/dev/null; then
	log "无法解析 tag/ref: ${TAG}（CI 中请确认 checkout 使用了 fetch-depth: 0）"
	exit 2
fi

REPO_SLUG="${GITHUB_REPOSITORY:-}"
if [ -z "${REPO_SLUG}" ]; then
	REPO_SLUG="$(git config --get remote.origin.url || true)"
	REPO_SLUG="${REPO_SLUG%.git}"
	REPO_SLUG="$(printf '%s' "${REPO_SLUG}" | sed -E 's#^.*[:/]([^/]+/[^/]+)$#\1#')"
fi

nearest_ancestor_tag() {
	local tag="$1" ref
	if ref=$(git describe --tags --abbrev=0 --match "${VERSION_TAG_GLOB}" "${tag}^" 2>/dev/null); then
		printf '%s' "${ref}"
		return 0
	fi
	return 1
}

latest_ancestor_tag() {
	local tag="$1" best="" best_ct=-1 t ct
	while IFS= read -r t; do
		if [ "${t}" = "${tag}" ]; then
			continue
		fi
		if git merge-base --is-ancestor "${t}" "${tag}" 2>/dev/null; then
			ct=$(git log -1 --format=%ct "${t}")
			if [ "${ct}" -gt "${best_ct}" ]; then
				best="${t}"
				best_ct="${ct}"
			fi
		fi
	done < <(git tag -l "${VERSION_TAG_GLOB}")
	printf '%s' "${best}"
}

BASE="${2:-${CHANGELOG_BASE:-}}"
if [ -z "${BASE}" ]; then
	BASE=$(nearest_ancestor_tag "${TAG}" || true)
fi
if [ -z "${BASE}" ]; then
	BASE=$(latest_ancestor_tag "${TAG}")
fi

if [ -n "${BASE}" ]; then
	log "changelog 区间: ${BASE}..${TAG}"
	COMMITS=$(git log --no-merges --pretty=format:"%s%x1f%an" "${BASE}..${TAG}")
else
	log "changelog 区间: 未找到更早的版本 tag，按首个版本处理"
	COMMITS=""
fi

declare -A SEEN
FEATURES=""
FIXES=""
OTHERS=""

while IFS=$'\x1f' read -r msg author; do
	if [ -z "${msg:-}" ]; then
		continue
	fi

	case "${msg}" in
	"chore(release):"* | "更新 version"* | "MediaWarp v"* | "Merge pull request"* | "Merge branch"*)
		continue
		;;
	esac

	if [[ "${msg}" =~ ^(feat|perfect)(\(.+\))?:[[:space:]]*(.*)$ ]]; then
		category="FEATURES"
		desc="${BASH_REMATCH[3]}"
	elif [[ "${msg}" =~ ^fix(\(.+\))?:[[:space:]]*(.*)$ ]]; then
		category="FIXES"
		desc="${BASH_REMATCH[2]}"
	elif [[ "${msg}" =~ ^(docs|style|refactor|perf|test|build|ci|chore|revert|deps)(\(.+\))?:[[:space:]]*(.*)$ ]]; then
		category="OTHERS"
		desc="${BASH_REMATCH[3]}"
	else
		category="OTHERS"
		desc="${msg}"
	fi

	dedup_key="${category}::${desc}"
	if [ -n "${SEEN[${dedup_key}]+x}" ]; then
		continue
	fi
	SEEN[${dedup_key}]=1

	if [[ "${author}" =~ ^[A-Za-z0-9-]+$ ]]; then
		entry="- ${desc} by @${author}"
	else
		entry="- ${desc} by ${author}"
	fi

	case "${category}" in
	FEATURES) FEATURES="${FEATURES}${entry}\n" ;;
	FIXES) FIXES="${FIXES}${entry}\n" ;;
	OTHERS) OTHERS="${OTHERS}${entry}\n" ;;
	esac
done <<<"${COMMITS}"

CHANGELOG=""

if [ -z "${BASE}" ]; then
	CHANGELOG="### 🎉 首个版本\n\n"
	CHANGELOG="${CHANGELOG}本版本没有可对比的历史版本，完整记录见下方链接。\n\n"
fi

if [ -n "${FEATURES}" ]; then
	CHANGELOG="${CHANGELOG}### ✨ 新功能\n\n${FEATURES}\n"
fi

if [ -n "${FIXES}" ]; then
	CHANGELOG="${CHANGELOG}### 🐛 修复\n\n${FIXES}\n"
fi

if [ -n "${OTHERS}" ]; then
	CHANGELOG="${CHANGELOG}### 🔧 其他\n\n${OTHERS}\n"
fi

if [ -n "${BASE}" ] && [ -z "${FEATURES}${FIXES}${OTHERS}" ]; then
	CHANGELOG="${CHANGELOG}本版本没有需要列出的提交变更。\n\n"
fi

if [ -n "${BASE}" ]; then
	CHANGELOG="${CHANGELOG}**完整更新记录**: https://github.com/${REPO_SLUG}/compare/${BASE}...${TAG}\n"
else
	CHANGELOG="${CHANGELOG}**完整更新记录**: https://github.com/${REPO_SLUG}/commits/${TAG}\n"
fi

printf '%b' "${CHANGELOG}"
