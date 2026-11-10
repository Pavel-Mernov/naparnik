// Поговорить с агентом из терминала:
//
//	go run ./cmd/buddy -user intern.sokolov "Что такое витрина и где лежит витрина объявлений?"
//	go run ./cmd/buddy -user intern.sokolov            # диалог, выход — пустая строка
//
// Модель — из окружения (LLM_PROVIDER не задан → заглушка stub_ok, работает без ключа).
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/worker4556/naparnik/backend/internal/agent"
	"github.com/worker4556/naparnik/backend/internal/app"
)

func main() {
	app.LoadDotEnv(".env", "../.env")
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, in io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("buddy", flag.ContinueOnError)
	user := fs.String("user", "intern.sokolov", "логин тестовой учётки")
	if err := fs.Parse(args); err != nil {
		return err
	}
	a, err := app.New()
	if err != nil {
		return err
	}
	defer a.Store.Close()
	fmt.Fprintf(out, "Модель: %s/%s (%s), пользователь: %s\n", a.LLM.Provider, a.LLM.Model, a.LLM.Mode, *user)
	reader := bufio.NewReader(in)
	ask := func(text string) error {
		reply, err := a.Agent.Ask(context.Background(), *user, text)
		if err != nil {
			return err
		}
		show(out, reader, a.Agent, *user, reply)
		return nil
	}
	if fs.NArg() > 0 {
		return ask(strings.Join(fs.Args(), " "))
	}
	for {
		fmt.Fprint(out, "\nТы: ")
		line, _ := reader.ReadString('\n')
		if line = strings.TrimSpace(line); line == "" {
			return nil
		}
		if err := ask(line); err != nil {
			return err
		}
	}
}

func show(out io.Writer, in *bufio.Reader, a *agent.Agent, user string, r agent.Reply) {
	fmt.Fprintf(out, "\n🐾 %s\n", r.Text)
	for _, s := range r.Sources {
		fmt.Fprintf(out, "   📎 %s — %s, %s %s\n", s.Title, s.Author, s.UpdatedAt, s.Label)
	}
	for _, d := range r.Drafts {
		fmt.Fprintf(out, "   📝 Черновик %s: %s %v\n   Подтвердить? [y/N] ", d.ID, d.Tool, d.Args)
		line, _ := in.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(line)) == "y" {
			res := a.Confirm(user, d.ID)
			if res.Error != nil {
				fmt.Fprintf(out, "   ❌ %s\n", res.Error.Message)
			} else {
				fmt.Fprintf(out, "   ✅ %v\n", res.Receipt)
			}
		}
	}
}
