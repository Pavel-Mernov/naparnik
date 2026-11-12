// Прогон эталонного набора через агента (H7, M-14…M-17):
//
//	go run ./cmd/evals -mode stub     # заглушка модели, без ключа и без сети
//	go run ./cmd/evals -mode live     # модель из .env (DeepSeek / Claude)
//
// Состояние — в памяти, с чистыми тестовыми данными: PostgreSQL и Redis не трогаются. Отчёт — docs/evals/<mode>.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/worker4556/naparnik/backend/internal/app"
	"github.com/worker4556/naparnik/backend/internal/evals"
)

func main() {
	mode := flag.String("mode", "stub", "stub — заглушка модели, live — модель из .env")
	out := flag.String("out", "../docs/evals", "куда положить отчёт")
	only := flag.String("only", "", "через запятую: прогнать только эти id (e-17,e-21)")
	flag.Parse()
	app.LoadDotEnv(".env", "../.env")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("REDIS_URL")
	if *mode == "stub" {
		for _, k := range []string{"LLM_API_KEY", "DEEPSEEK_API_KEY", "ANTHROPIC_API_KEY", "LLM_PROVIDER"} {
			os.Unsetenv(k)
		}
	}
	a, err := app.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dir := os.Getenv("MOCK_DATA_DIR")
	if dir == "" {
		dir = "../docs/04-mock-data"
	}
	items, err := evals.Load(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *only != "" {
		keep := map[string]bool{}
		for _, id := range strings.Split(*only, ",") {
			keep[strings.TrimSpace(id)] = true
		}
		var sel []evals.Item
		for _, it := range items {
			if keep[it.ID] {
				sel = append(sel, it)
			}
		}
		items = sel
	}
	name := *mode
	if *mode == "live" {
		name = a.LLM.Provider + "-" + a.LLM.Model
	}
	rep, err := evals.Run(context.Background(), a.Agent, a.Store, items, name)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = os.MkdirAll(*out, 0o755)
	path := filepath.Join(*out, *mode+".md")
	if err := os.WriteFile(path, []byte(rep.Markdown()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	pass, total := 0, 0
	for _, c := range rep.ByCat {
		pass += c[0]
		total += c[1]
	}
	fmt.Printf("%s: %d из %d → %s\n", name, pass, total, path)
	for k, v := range rep.Metrics {
		fmt.Printf("  %s: %.0f%%\n", k, v*100)
	}
}
