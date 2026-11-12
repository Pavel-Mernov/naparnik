// MCP-сервер Напарника (stdio): инструменты агента для любого MCP-клиента.
//
//	cd backend && NAPARNIK_USER=intern.sokolov go run ./cmd/mcp
//
// Состояние — то же, что у сервера (DATABASE_URL из .env), поэтому черновик, созданный через MCP,
// человек подтверждает в Напарнике. Без DATABASE_URL — состояние в памяти, для проверки.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/worker4556/naparnik/backend/internal/app"
	"github.com/worker4556/naparnik/backend/internal/mcp"
)

func main() {
	log.SetOutput(os.Stderr) // stdout — только для протокола
	app.LoadDotEnv(".env", "../.env")
	login := os.Getenv("NAPARNIK_USER")
	if login == "" {
		login = "intern.sokolov"
	}
	a, err := app.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer a.Store.Close()
	u, ok := a.Store.UserByLogin(login)
	if !ok {
		fmt.Fprintln(os.Stderr, "нет учётки", login)
		os.Exit(1)
	}
	web := os.Getenv("WEB_URL")
	if web == "" {
		web = "http://localhost:3000"
	}
	if err := (&mcp.Server{Store: a.Store, User: u, WebURL: web}).Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
