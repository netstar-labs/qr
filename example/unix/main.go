// Command unix is a minimal example: serve QR codes over a Unix-domain socket —
// a local sidecar reachable only from the same host, with filesystem
// permissions as the access control. It is the HTTP example over an AF_UNIX
// listener instead of TCP; the request is identical.
//
//	go run ./example/unix -sock /tmp/qr.sock
//	curl --unix-socket /tmp/qr.sock 'http://qr/qr?text=HELLO&fmt=txt'
//	curl --unix-socket /tmp/qr.sock 'http://qr/qr?text=HELLO&level=H' -o code.png
package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/netstar-labs/qr"
)

var levels = map[string]qr.Level{
	"": qr.Medium, "L": qr.Low, "M": qr.Medium, "Q": qr.Quartile, "H": qr.High,
}

func main() {
	sock := flag.String("sock", "/tmp/qr.sock", "unix socket path")
	flag.Parse()

	os.Remove(*sock) // clear a stale socket from a previous run
	ln, err := net.Listen("unix", *sock)
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(*sock)

	http.HandleFunc("/qr", handle)
	log.Printf("qr unix: listening on %s", *sock)
	log.Fatal(http.Serve(ln, nil))
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
