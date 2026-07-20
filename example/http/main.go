// Command http is a minimal example: serve QR codes over HTTP.
//
//	go run ./example/http -addr :8080
//	curl 'http://localhost:8080/qr?text=HELLO&level=H&scale=8' -o code.png
//	curl 'http://localhost:8080/qr?text=HELLO&fmt=txt'            # ASCII
package main

import (
	"flag"
	"log"
	"net/http"
	"strconv"

	"github.com/netstar-labs/qr"
)

var levels = map[string]qr.Level{
	"": qr.Medium, "L": qr.Low, "M": qr.Medium, "Q": qr.Quartile, "H": qr.High,
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	http.HandleFunc("/qr", handle)
	log.Printf("qr http: listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

// handle answers GET /qr?text=…&level=L|M|Q|H&scale=N&fmt=png|txt.
func handle(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	text := q.Get("text")
	if text == "" {
		http.Error(w, "?text= is required", http.StatusBadRequest)
		return
	}
	level, ok := levels[q.Get("level")]
	if !ok {
		http.Error(w, "level must be L, M, Q, or H", http.StatusBadRequest)
		return
	}
	code, err := qr.Encode(text, level)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if q.Get("fmt") == "txt" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(code.String()))
		return
	}
	scale, err := strconv.Atoi(q.Get("scale"))
	if err != nil || scale < 1 {
		scale = 8
	}
	w.Header().Set("Content-Type", "image/png")
	if err := code.PNG(w, scale, 4); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
