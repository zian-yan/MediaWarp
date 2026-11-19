package logging

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AkimioJR/MediaWarp/internal/config"
)

func dayName(offset int) string {
	return time.Now().AddDate(0, 0, offset).Format("2006-01-02")
}

func writeDay(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("创建测试目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "service.log"), []byte("line\n"), 0o640); err != nil {
		t.Fatalf("写入测试日志失败: %v", err)
	}
}

func TestPruneExpiredLogsKeepsRetentionWindow(t *testing.T) {
	root := t.TempDir()
	writeDay(t, root, dayName(0))
	writeDay(t, root, dayName(-1))
	writeDay(t, root, dayName(-KeepDays))
	writeDay(t, root, dayName(-KeepDays-1))
	writeDay(t, root, dayName(-KeepDays-30))

	pruneExpiredLogs(root, KeepDays)

	for _, name := range []string{dayName(0), dayName(-1), dayName(-KeepDays)} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("保留期内的目录 %s 不应被删除: %v", name, err)
		}
	}
	for _, name := range []string{dayName(-KeepDays - 1), dayName(-KeepDays - 30)} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("过期目录 %s 应被删除", name)
		}
	}
}

func TestPruneExpiredLogsIgnoresNonDateEntries(t *testing.T) {
	root := t.TempDir()
	old := dayName(-KeepDays - 5)
	writeDay(t, root, old)

	// 非日期命名的目录与文件（手工放进去的说明、脚本）必须原样保留。
	if err := os.MkdirAll(filepath.Join(root, "keep-me"), 0o750); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o640); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}

	pruneExpiredLogs(root, KeepDays)

	if _, err := os.Stat(filepath.Join(root, "keep-me")); err != nil {
		t.Fatalf("非日期目录不应被删除: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "notes.txt")); err != nil {
		t.Fatalf("非日期文件不应被删除: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, old)); !os.IsNotExist(err) {
		t.Fatal("过期日期目录应被删除")
	}
}

func TestPruneExpiredLogsHandlesLegacyUnpaddedFolders(t *testing.T) {
	root := t.TempDir()
	expired := time.Now().AddDate(0, 0, -KeepDays-3).Format("2006-1-2")
	writeDay(t, root, expired)

	pruneExpiredLogs(root, KeepDays)

	if _, err := os.Stat(filepath.Join(root, expired)); !os.IsNotExist(err) {
		t.Fatalf("旧版无前导零的过期目录 %s 也应被删除", expired)
	}
}

func TestPruneExpiredLogsMissingRootIsNoop(t *testing.T) {
	// 目录还不存在（首次启动、日志未开启）时不能 panic，也不能报错。
	pruneExpiredLogs(filepath.Join(t.TempDir(), "not-created"), KeepDays)
}

func TestLogDirIsNamespacedAndDated(t *testing.T) {
	if got := filepath.ToSlash(config.LogDir()); got != "logs/mediawarp" {
		t.Fatalf("日志目录应按来源分类，实际 %q", got)
	}
	if _, ok := parseLogDay(dayName(0)); !ok {
		t.Fatal("标准日期目录名必须能被解析")
	}
}
