package main

import (
	"fmt"
	"os"

	"github.com/sinuxlee/giny/internal/app"
)

var version = "No Build Info"

func main() {
	srv, err := app.New(
		app.Version(version),
		app.NodeID(),
		app.Logger(),
		app.KVStore(),
		app.Redis(),
		app.Handler(),
	)

	defer func() {
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err.Error())
		}
	}()

	if err != nil {
		return
	}

	if err = srv.Run(); err != nil {
		return
	}

	err = srv.Stop()
}
