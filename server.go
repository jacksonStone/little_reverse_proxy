package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	// "github.com/ncruces/go-sqlite3"
	// _ "github.com/ncruces/go-sqlite3/embed"
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
		log.Println("Starting HTTP server on :8888")
		err := http.ListenAndServe(":8888", nil)
		if err != nil {
			log.Fatal("HTTP server error: ", err)
		}

	} else {
		httpOnly := false

		cert, err := tls.LoadX509KeyPair("/etc/letsencrypt/live/jacksonstone.info/fullchain.pem",
			"/etc/letsencrypt/live/jacksonstone.info/privkey.pem")

		if err != nil {
			print(err)
			// Fall back to http only, as maybe we are setting up the ssls for the first time
			httpOnly = true
		}
		cert2, err := tls.LoadX509KeyPair("/etc/letsencrypt/live/libby.cards/fullchain.pem",
			"/etc/letsencrypt/live/libby.cards/privkey.pem")

		if err != nil {
			print(err)
			// Fall back to http only, as maybe we are setting up the ssls for the first time
			httpOnly = true
		}

		cert3, err := tls.LoadX509KeyPair("/etc/letsencrypt/live/theologian.chat/fullchain.pem",
			"/etc/letsencrypt/live/theologian.chat/privkey.pem")

		if err != nil {
			print(err)
			// Fall back to http only, as maybe we are setting up the ssls for the first time
			httpOnly = true
		}

		if !httpOnly {
			go startHTTPServer()
			cfg := &tls.Config{Certificates: []tls.Certificate{cert, cert2, cert3}}
			srv := &http.Server{
				Addr:      ":443",
				TLSConfig: cfg,
			}
			fmt.Println("Starting HTTPS server on :443")
			log.Fatal(srv.ListenAndServeTLS("", ""))
		} else {
			fmt.Println("Skipping HTTPS server start as no certs were found. This is likely because the server is being set up for the first time.")
			startHTTPServer()
		}

	}
}
func startHTTPServer() {
	prodHttpMux := http.NewServeMux()
	// For DNS Challenge task required to renew SSL on the server every 60 or so days.
	// This is done on an ubuntu E2C box with certbot which has been configured to create these files here when it is attempting to renew
	// The Go server will then serve up these files over Port 80 for Let's Encrypt to hit and sign my new shiny certs with confidence.
	prodHttpMux.Handle("/.well-known/", http.StripPrefix("/.well-known/", http.FileServer(http.Dir("/home/ubuntu/.well-known"))))
	// All other requests we want to redirect to HTTPS in prod
	prodHttpMux.HandleFunc("/", redirectToTls)
	fmt.Println("Starting HTTP server on :80")
	if err := http.ListenAndServe(":80", prodHttpMux); err != nil {
		log.Fatalf("ListenAndServe error: %v", err)
	}
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	// Check if the host starts with "www.", prefer this for CDN reasons? Something like that.
	if !strings.HasPrefix(host, "www.") {
		// Prepend "www." to the host
		newHost := "www." + host
		newURL := "https://" + newHost + r.URL.Path
		if r.URL.RawQuery != "" {
			newURL += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, newURL, http.StatusMovedPermanently)
		return
	}
	if strings.Contains(r.Host, "jacksonstone.info") {
		fmt.Fprintf(w, "Hello, World! This is my site, hosted on an EC2 mirco. The only feature right now is it auto renews HTTPS certs :D\nI plan to put more things here at some point...")
	} else if strings.Contains(r.Host, "libby.cards") {
		fmt.Print("This is the Libby.cards site")
	} else if strings.Contains(r.Host, "theologian.chat") {
		fmt.Print("This is the theologian.chat site")
	}
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

// func createDbHandler(w http.ResponseWriter, r *http.Request) {

// 	db, err := sqlite3.Open("personalDb")
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	err = db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`)
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	fmt.Fprintf(w, "Tables Created")

// }
