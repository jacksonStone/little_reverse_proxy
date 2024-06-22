package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
)

func main() {
	// Example route
	http.Handle("/.well-known/", http.StripPrefix("/.well-known/", http.FileServer(http.Dir("/home/ubuntu/.well-known"))))
	http.HandleFunc("/", helloHandler)

	// Determine if we're in a local development environment
	isLocalDev := false // You might want to set this based on an environment variable

	if isLocalDev {
		// For local development, allow both HTTP and HTTPS
		go func() {
			log.Println("Starting HTTP server on :8080")
			err := http.ListenAndServe(":8080", nil)
			if err != nil {
				log.Fatal("HTTP server error: ", err)
			}
		}()

		log.Println("Starting HTTPS server on :8443")
		err := http.ListenAndServeTLS(":8443", "server.crt", "server.key", nil)
		if err != nil {
			log.Fatal("HTTPS server error: ", err)
		}
	} else {
		cert, err := tls.LoadX509KeyPair("/etc/letsencrypt/live/jacksonstone.info/fullchain.pem",
			"/etc/letsencrypt/live/jacksonstone.info/privkey.pem")
		if err != nil {
			log.Fatal(err)
		}

		go func() {
			if err := http.ListenAndServe(":80", http.HandlerFunc(redirectToTls)); err != nil {
				log.Fatalf("ListenAndServe error: %v", err)
			}
		}()

		cfg := &tls.Config{Certificates: []tls.Certificate{cert}}
		srv := &http.Server{
			Addr:      ":443",
			TLSConfig: cfg,
		}

		log.Fatal(srv.ListenAndServeTLS("", ""))
	}
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello, World! This is my site, hosted on an EC2 mirco. The only feature right now is it auto renews HTTPS certs :D\nI plan to put more things here at some point...")
}

func redirectToTls(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "https://jacksonstone.info:443"+r.RequestURI, http.StatusMovedPermanently)
}
