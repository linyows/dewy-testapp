package main

import (
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lestrrat-go/server-starter/listener"
)

const Name string = "dewy-testapp"
const Version string = "1.3.0"

func main() {
	var err error
	var listeners []net.Listener

	if len(os.Args) > 0 {
		listeners, err = listener.ListenAll()
	} else {
		var l net.Listener
		l, err = net.Listen("tcp", ":3333")
		listeners = append(listeners, l)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen: %s\n", err)
		os.Exit(1)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Yo, %q\n", html.EscapeString(r.URL.Path))
		io.Copy(w, r.Body)
	})

	for _, l := range listeners {
		http.Serve(l, handler)
	}

	loop := false
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGHUP)

	for loop {
		select {
		case <-sigCh:
			loop = false
		default:
			time.Sleep(time.Second)
		}
	}
}
