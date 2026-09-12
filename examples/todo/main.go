// Command todo is a small in-memory JSON API that shows bodhiApi routing,
// request context, and graceful shutdown.
//
// Run it from the module root:
//
//	go run ./examples/todo
//
// Then:
//
//	curl -s http://127.0.0.1:8080/health
//	curl -s http://127.0.0.1:8080/users/42
//	curl -s http://127.0.0.1:8080/todos
//
// Press Ctrl-C to drain in-flight requests and exit.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	bodhiApi "github.com/anti-gravity-bit/bodhiApi"
)

const listenAddress = ":8080"
const shutdownTimeout = 10 * time.Second

type todoItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

type todoStore struct {
	mutex sync.RWMutex
	items []todoItem
}

func main() {
	store := &todoStore{
		items: []todoItem{
			{ID: "1", Title: "read the README", Done: true},
			{ID: "2", Title: "run go test ./...", Done: false},
		},
	}

	application := bodhiApi.New(bodhiApi.Info{
		Title:   "Todo API",
		Version: "0.0.0",
	})

	application.GET("/users/:id", func(requestContext bodhiApi.Context) error {
		return requestContext.JSON(http.StatusOK, map[string]string{
			"id": requestContext.Param("id"),
		})
	})
	application.GET("/todos", func(requestContext bodhiApi.Context) error {
		return requestContext.JSON(http.StatusOK, store.list())
	})
	application.POST("/todos", func(requestContext bodhiApi.Context) error {
		createdItem := store.add("new todo")
		return requestContext.JSON(http.StatusCreated, createdItem)
	})

	listenErrorChannel := make(chan error, 1)
	go func() {
		listenErrorChannel <- application.ListenAndServe(listenAddress)
	}()

	interruptChannel := make(chan os.Signal, 1)
	signal.Notify(interruptChannel, os.Interrupt, syscall.SIGTERM)

	select {
	case listenError := <-listenErrorChannel:
		if listenError != nil {
			slog.Error("server stopped", slog.String("err", listenError.Error()))
			os.Exit(1)
		}
	case <-interruptChannel:
		slog.Info("shutting down")
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancelShutdown()
		if shutdownError := application.Shutdown(shutdownContext); shutdownError != nil {
			slog.Error("shutdown failed", slog.String("err", shutdownError.Error()))
			os.Exit(1)
		}
		slog.Info("drained")
	}
}

func (store *todoStore) list() []todoItem {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	copiedItems := make([]todoItem, len(store.items))
	copy(copiedItems, store.items)
	return copiedItems
}

func (store *todoStore) add(title string) todoItem {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	createdItem := todoItem{ID: time.Now().UTC().Format("20060102150405.000"), Title: title, Done: false}
	store.items = append(store.items, createdItem)
	return createdItem
}
