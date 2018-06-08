package main

import (
	"fmt"
	"html"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hi, %q\n", html.EscapeString(r.URL.Path))
}

func main() {
	http.HandleFunc("/", handler)
	http.ListenAndServe(":3333", nil)
}
