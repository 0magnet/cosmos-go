// Command serve hosts a directory over HTTP so the wasm demo can be opened in
// a browser — Go serves .wasm with the right MIME type, and this needs only
// the Go toolchain this project already uses.
//
//	go run ./serve                      # http://localhost:8080, current dir
//	go run ./serve --addr :9000 --dir x # a different port and directory
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/0magnet/calvin/clihelp"
	"github.com/spf13/cobra"
)

func main() {
	var addr, dir string
	cmd := &cobra.Command{
		Use:          "serve",
		Short:        "serve a directory over HTTP for the wasm demo",
		SilenceUsage: true,
		Run: func(*cobra.Command, []string) {
			log.Printf("serving %s on http://localhost%s", dir, addr)
			// An http.Server rather than ListenAndServe: the bare helper sets no
			// timeouts at all, so a stalled client holds a connection forever. The
			// write timeout is generous because a .wasm binary is large and a slow
			// link should not have its download cut off mid-file.
			srv := &http.Server{
				Addr:              addr,
				Handler:           http.FileServer(http.Dir(dir)),
				ReadHeaderTimeout: 10 * time.Second,
				ReadTimeout:       30 * time.Second,
				WriteTimeout:      5 * time.Minute,
				IdleTimeout:       2 * time.Minute,
			}
			log.Fatal(srv.ListenAndServe())
		},
	}
	cmd.Flags().StringVar(&addr, "addr", ":8080", "listen address")
	cmd.Flags().StringVar(&dir, "dir", ".", "directory to serve")
	clihelp.Init(cmd, "serve")
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
