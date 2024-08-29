package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

// Create a struct to hold domain and port number
type domainPort struct {
	domain string
	port   string
}

// mo/link example
// Use DOMAINS_TO_PORTS which is a string formatted like so: "domain1:port1,domain2:port2,domain3:port3"
var sites = []domainPort{}

var reverseProxyMap = make(map[string]*httputil.ReverseProxy)

func initializeSiteList() {
	domainsToPorts := os.Getenv("DOMAINS_TO_PORTS")
	if domainsToPorts == "" {
		log.Fatal("DOMAINS_TO_PORTS environment variable must be set and formatted like so: \"domain1:port1,domain2:port2,domain3:port3\"")
	}
	domainsToPortsSplit := strings.Split(domainsToPorts, ",")
	for _, domainToPort := range domainsToPortsSplit {
		domainPortSplit := strings.Split(domainToPort, ":")
		if len(domainPortSplit) != 2 {
			log.Fatalf("Invalid domain to port mapping: %s", domainToPort)
		}
		sites = append(sites, domainPort{domain: domainPortSplit[0], port: domainPortSplit[1]})
	}
}
func initializeReverseProxies() {
	for _, site := range sites {
		target, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%s", site.port))
		if err != nil {
			log.Fatalf("Failed to parse URL: %v", err)
		}
		reverseProxyMap[site.domain] = httputil.NewSingleHostReverseProxy(target)
	}
}
func main() {
	initializeSiteList()
	initializeReverseProxies()
	for _, site := range sites {
		fmt.Printf("Domain: %s, Port: %s\n", site.domain, site.port)
	}
	http.HandleFunc("/", reverseProxyRequest)

	// Determine if we're in a local development environment
	isLocalDev, exists := os.LookupEnv("LOCAL_DEV")
	if !exists {
		isLocalDev = "false"
	}

	if isLocalDev == "true" {
		// For local development, just use http
		log.Println("Starting HTTP server on :8888")
		err := http.ListenAndServe(":8888", nil)
		if err != nil {
			log.Fatal("HTTP server error: ", err)
		}

	} else {
		httpOnly, certs := getSSLCerts()
		if !httpOnly {
			go startHTTPServer()
			cfg := &tls.Config{Certificates: certs}
			srv := &http.Server{
				Addr:      ":443",
				TLSConfig: cfg,
			}
			fmt.Println("Starting HTTPS server on :443")
			log.Fatal(srv.ListenAndServeTLS("", ""))
		} else {
			fmt.Println("Skipping HTTPS server start as no certs were found. This is likely because the server is being set up for the first time. or you are actually doing local development, in which case provide LOCAL_DEV=true when building to skip https related functionality.")
			startHTTPServer()
		}

	}
}
func getSSLCerts() (bool, []tls.Certificate) {
	domains := make([]string, len(sites))
	for i, site := range sites {
		domains[i] = site.domain
	}
	certs := make([]tls.Certificate, 0)
	httpOnly := false
	for _, domain := range domains {
		httpOnlyForDomain, cert := getSSLCert(domain)
		if httpOnlyForDomain {
			httpOnly = true
			continue
		}
		certs = append(certs, cert)
	}
	return httpOnly, certs
}

// bool is httpOnly
func getSSLCert(domain string) (bool, tls.Certificate) {
	cert, err := tls.LoadX509KeyPair("/etc/letsencrypt/live/"+domain+"/fullchain.pem",
		"/etc/letsencrypt/live/"+domain+"/privkey.pem")
	httpOnly := err != nil
	if httpOnly {
		fmt.Println("Failed to fetch SSL certs for domain: " + domain)
	}
	return httpOnly, cert
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

func reverseProxyRequest(w http.ResponseWriter, r *http.Request) {
	// reverse proxy request to localhost on the port
	proxy, ok := reverseProxyMap[strings.TrimPrefix(r.Host, "www.")]
	if !ok {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	} else {
		proxy.ServeHTTP(w, r)
	}
}

func redirectToTls(w http.ResponseWriter, r *http.Request) {
	// Construct the new URL with HTTPS and the original host
	newURL := "https://" + r.Host + r.RequestURI
	http.Redirect(w, r, newURL, http.StatusMovedPermanently)
}
