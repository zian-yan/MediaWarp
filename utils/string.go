package utils

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var removeColorCodesRegexp = regexp.MustCompile(`\033\[[0-9]*m`)

func RemoveColorCodes(line string) string {
	return removeColorCodesRegexp.ReplaceAllString(line, "")
}

func Center(s string, width int, fill string) string {
	if len(s) >= width {
		return s
	}
	padding := width - len(s)
	leftPadding := padding / 2
	rightPadding := padding - leftPadding
	return strings.Repeat(fill, leftPadding) + s + strings.Repeat(fill, rightPadding)
}

func SplitHostPort(hostPort string) (host string, port string) {
	host = hostPort

	colon := strings.LastIndexByte(host, ':')
	if colon != -1 && validOptionalPort(host[colon:]) {
		host, port = host[:colon], host[colon+1:]
	}

	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}

	return
}

func validOptionalPort(port string) bool {
	if port == "" {
		return true
	}
	if port[0] != ':' {
		return false
	}
	for _, b := range port[1:] {
		if b < '0' || b > '9' {
			return false
		}
	}
	return true
}

func MD5Hash(raw string) string {
	hash := md5.New()
	hash.Write([]byte(raw))
	return hex.EncodeToString(hash.Sum(nil))
}

func GetEndpoint(addr string) string {
	if !strings.HasPrefix(addr, "http") {
		addr = "http://" + addr
	}
	return strings.TrimSuffix(addr, "/")
}

var embyAPIKeys = []string{"api_key", "ApiKey", "X-Emby-Token"}

func ResolveEmbyAPIKVPairs(urlString *string) (string, error) {
	if urlString == nil {
		return "", fmt.Errorf("urlString is nil")
	}
	url, err := url.Parse(*urlString)
	if err != nil {
		return "", err
	}
	for quryKey, queryValue := range url.Query() {
		for _, key := range embyAPIKeys {
			if strings.EqualFold(quryKey, key) {
				return fmt.Sprintf("%s=%s", quryKey, queryValue[0]), nil
			}
		}
	}
	return "", nil
}

func isInt[T ~[]byte | ~[]rune | ~string](s T) bool {
	_, err := strconv.Atoi(string(s))
	return err == nil
}

var sensitiveQueryKeys = []string{"api_key", "apikey", "x-emby-token", "token", "access_token"}

func isSensitiveQueryKey(key string) bool {
	lower := strings.ToLower(key)
	for _, k := range sensitiveQueryKeys {
		if lower == k {
			return true
		}
	}
	return false
}

func MaskSensitiveQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	parts := strings.Split(rawQuery, "&")
	for i, part := range parts {
		j := strings.IndexByte(part, '=')
		if j < 0 {
			continue
		}
		if isSensitiveQueryKey(part[:j]) {
			parts[i] = part[:j] + "=***"
		}
	}
	return strings.Join(parts, "&")
}

func IsSafeRedirectURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func MaskSensitiveURL(rawURL string) string {
	i := strings.Index(rawURL, "?")
	if i < 0 {
		return rawURL
	}
	base, query := rawURL[:i], rawURL[i+1:]
	fragment := ""
	if j := strings.Index(query, "#"); j >= 0 {
		fragment = query[j:]
		query = query[:j]
	}
	return base + "?" + MaskSensitiveQuery(query) + fragment
}

func FindStringIndex(slice []string, target string, caseInsensitive bool, trim bool) int {
	if trim {
		target = strings.TrimSpace(target)
	}
	for i, str := range slice {
		if trim {
			str = strings.TrimSpace(str)
		}
		if caseInsensitive {
			if strings.EqualFold(str, target) {
				return i
			}
		} else {
			if str == target {
				return i
			}
		}
	}
	return -1
}
