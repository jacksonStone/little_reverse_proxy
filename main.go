package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	http.HandleFunc("/", helloHandler)

	// Determine if we're in a local development environment
	isLocalDev, exists := os.LookupEnv("PERSONAL_SITE_DEV")
	if !exists {
		isLocalDev = "false"
	}
	// Set up channel to receive OS signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	// Start a goroutine to handle shutdown signals
	go func() {
		sig := <-sigs
		fmt.Printf("Received signal: %v\n", sig)
		cleanup()
		os.Exit(0)
	}()

	if isLocalDev == "true" {
		// For local development, just use http
		log.Println("Starting HTTP server on :8080")
		err := http.ListenAndServe(":8080", nil)
		if err != nil {
			log.Fatal("HTTP server error: ", err)
		}

	} else {
		cert, err := tls.LoadX509KeyPair("/etc/letsencrypt/live/jacksonstone.info/fullchain.pem",
			"/etc/letsencrypt/live/jacksonstone.info/privkey.pem")

		if err != nil {
			log.Fatal(err)
		}

		go func() {
			prodHttpMux := http.NewServeMux()
			// For DNS Challenge task required to renew SSL on the server every 60 or so days.
			// This is done on an ubuntu E2C box with certbot which has been configured to create these files here when it is attempting to renew
			// The Go server will then serve up these files over Port 80 for Let's Encrypt to hit and sign my new shiny certs with confidence.
			prodHttpMux.Handle("/.well-known/", http.StripPrefix("/.well-known/", http.FileServer(http.Dir("/home/ubuntu/.well-known"))))
			// All other requests we want to redirect to HTTPS in prod
			prodHttpMux.HandleFunc("/", redirectToTls)
			if err := http.ListenAndServe(":80", prodHttpMux); err != nil {
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
	// Construct the new URL with HTTPS and the original host
	newURL := "https://" + r.Host + r.RequestURI
	http.Redirect(w, r, newURL, http.StatusMovedPermanently)
}

func cleanup() {
	// Create a file
	file, err := os.Create("cleanup.txt")
	if err != nil {
		fmt.Println("An error occurred while creating the file:", err)
		return
	}
	defer file.Close()

	// Write to the file
	_, err = file.WriteString("This File was created at: " + time.Now().String())
	if err != nil {
		fmt.Println("An error occurred while writing to the file:", err)
		return
	}

	fmt.Println("File created and written successfully")

}
