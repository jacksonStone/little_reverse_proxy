package main

import (
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
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
	fmt.Println("SQL URL: ", os.Getenv("SQLITE_URL"))
	for _, site := range sites {
		fmt.Printf("Domain: %s, Port: %s\n", site.domain, site.port)
	}
	http.HandleFunc("/", rootHandler)

	// Determine if we're in a local development environment
	isLocalDev, exists := os.LookupEnv("LOCAL_DEV")
	if !exists {
		isLocalDev = "false"
	}

	exitListener()

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

func rootHandler(w http.ResponseWriter, r *http.Request) {
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
	if strings.HasSuffix(host, "jacksonstone.info") {
		newURL := "https://" + "www.jacksonst.one" + r.URL.Path
		if r.URL.RawQuery != "" {
			newURL += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, newURL, http.StatusMovedPermanently)
		return
	}
	reverseProxyRequest(w, r)
}

/**
 * Write the visit record to the SQLite database
 * A return value of 0 means no record was written
 */
func writeVisitRecord(url string, remoteAddr string, duration int64) {
	// Skip requests that are likely not indicative of a page visit
	// (Imperfect)
	excludeList := []string{"/favicon.ico"}
	for _, exclude := range excludeList {
		if strings.Contains(url, exclude) {
			return
		}
	}
	sqliteUrl := os.Getenv("SQLITE_URL")
	if sqliteUrl == "" {
		fmt.Println("SQLITE_URL environment variable must be set, skipping persisting visit record")
		return
	}
	hasedIp := produceHashFromRemoteAddr(remoteAddr)
	urlWithoutWww := strings.TrimPrefix(url, "www.")
	jsonRequest := fmt.Sprintf(`{
		"query":"INSERT INTO reverse_proxy_visits (url_without_params, vistor_hash, duration) VALUES (?, ?, ?)",
		"parameters":["%s", "%s", %d],
		"database": "visits"
	}`, urlWithoutWww, hasedIp, duration)
	fmt.Println("Sending request to SQLite: ", jsonRequest)
	// send the SQL query to the locally running SQLite wrapper
	_, err := http.Post(sqliteUrl+"/execute", "application/json",
		strings.NewReader(jsonRequest))
	if err != nil {
		fmt.Println("Failed to write visit record to SQLite: ", err)
		return
	}

}

/**
 * Write the visit record to the SQLite database
 * A return value of 0 means no record was written
 */
func writeWebsiteVisitRecord(url string, remoteAddr string) {
	sqliteUrl := os.Getenv("SQLITE_URL")
	if sqliteUrl == "" {
		fmt.Println("SQLITE_URL environment variable must be set, skipping persisting visit record")
		return
	}
	hasedIp := produceHashFromRemoteAddr(remoteAddr)
	urlWithoutWww := strings.TrimPrefix(url, "www.")
	jsonRequest := fmt.Sprintf(`{
		"query":"INSERT INTO reverse_proxy_website_visits (url_without_params, vistor_hash) VALUES (?, ?)",
		"parameters":["%s", "%s"],
		"database": "visits"
	}`, urlWithoutWww, hasedIp)
	// send the SQL query to the locally running SQLite wrapper
	_, err := http.Post(sqliteUrl+"/execute", "application/json",
		strings.NewReader(jsonRequest))
	if err != nil {
		fmt.Println("Failed to write website visit record to SQLite: ", err)
		return
	}
}
func produceHashFromRemoteAddr(remoteAddr string) string {
	ipPlusHex := strings.Split(remoteAddr, ":")[0]
	var hasedIp string
	// get last15 characters
	// 111.111.111.111
	if len(ipPlusHex) <= 15 {
		hasedIp = ipPlusHex
	} else {
		hasedIp = ipPlusHex[len(ipPlusHex)-15:]
	}
	h := sha256.New()

	h.Write([]byte(hasedIp))
	return fmt.Sprintf("%x", h.Sum(nil))[0:10]
}
func reverseProxyRequest(w http.ResponseWriter, r *http.Request) {
	// reverse proxy request to localhost on the port
	proxy, ok := reverseProxyMap[strings.TrimPrefix(r.Host, "www.")]
	if !ok {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	} else {
		// if this is a request to a /____reserved/_ping endpoint, we want to return a 200
		// This is fired from the web-browser in javascript to help me distinguish between a real visit and a bot
		// The bots can of course still make requests to /____reserved/_ping, or execute the JS code, but this is a good start
		// to filter out most trivial bots.
		if strings.HasSuffix(r.URL.Path, "/____reserved/_ping") {
			w.WriteHeader(http.StatusOK)
			writeWebsiteVisitRecord(r.Host+strings.TrimSuffix(r.URL.Path, "/____reserved/_ping"), r.RemoteAddr)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/robots.txt") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("User-agent: *\nDisallow:"))
			return
		}
		startTime := time.Now().UnixMilli()
		proxy.ServeHTTP(w, r)
		endTime := time.Now().UnixMilli()
		writeVisitRecord(r.Host+r.URL.Path, r.RemoteAddr, endTime-startTime)
	}
}

func redirectToTls(w http.ResponseWriter, r *http.Request) {
	// Construct the new URL with HTTPS and the original host
	newURL := "https://" + r.Host + r.RequestURI
	http.Redirect(w, r, newURL, http.StatusMovedPermanently)
}

func exitListener() {
	// Set up channel to receive OS signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	// Start a goroutine to handle shutdown signals
	go func() {
		sig := <-sigs
		fmt.Printf("Received signal: %v\n", sig)
		cleanupProcedure()
		os.Exit(0)
	}()
}

func cleanupProcedure() {
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
