// Сервер AI-buddy: HTTP API для фронта. Запуск: go run ./cmd/server (порт — PORT, по умолчанию 8080).
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/worker4556/naparnik/backend/internal/app"
	"github.com/worker4556/naparnik/backend/internal/httpapi"
)

func main() {
	if f := app.LoadDotEnv(".env", "../.env"); f != "" {
		log.Printf("настройки из %s", f)
	}
	a, err := app.New()
	if err != nil {
		log.Fatal(err)
	}
	defer a.Store.Close()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	static := os.Getenv("STATIC_DIR")
	if static == "" {
		static = "../frontend/web"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           (&httpapi.Server{Agent: a.Agent, Store: a.Store, StaticDir: static, TG: a.TG, KV: a.KV, ChatLimit: a.ChatLimit}).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("AI-buddy: http://localhost:%s, модель %s/%s (%s), демо-дата %s", port, a.LLM.Provider, a.LLM.Model, a.LLM.Mode,
		a.Store.Today.Format(time.DateOnly))
	log.Printf("кэш: %s", a.KV.Kind())
	go a.TG.Run(context.Background()) // напоминания и Telegram в фоне; сбой Telegram не влияет на веб (BR-53)
	log.Fatal(srv.ListenAndServe())
}
