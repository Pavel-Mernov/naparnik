package app

import (
	"bufio"
	"os"
	"strings"
)

// LoadDotEnv читает KEY=VALUE из первого найденного файла и выставляет переменные,
// которые ещё не заданы в окружении (окружение важнее файла). Нужно для запуска без Docker:
// ключ, вписанный в .env в корне репозитория, подхватывается и при `go run`.
func LoadDotEnv(paths ...string) string {
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, val, ok := strings.Cut(line, "=")
			key, val = strings.TrimSpace(key), strings.Trim(strings.TrimSpace(val), `"'`)
			if _, set := os.LookupEnv(key); ok && key != "" && !set {
				_ = os.Setenv(key, val)
			}
		}
		_ = f.Close()
		return p
	}
	return ""
}
