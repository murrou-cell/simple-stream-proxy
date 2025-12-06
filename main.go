package main

import (
	"bufio"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var scheme = "http"

func main() {
	http.HandleFunc("/proxy", handleProxy)
	port := "8088"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}
	if os.Getenv("USE_HTTPS") == "true" {
		scheme = "https"
	}
	log.Printf("Proxy server running on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleProxy(w http.ResponseWriter, r *http.Request) {
	// Allow CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Handle preflight requests
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	rawURL := r.URL.Query().Get("url")
	log.Printf("Proxying request for URL: %s", rawURL)
	if rawURL == "" {
		http.Error(w, "missing url parameter", http.StatusBadRequest)
		return
	}

	upstream, err := url.Parse(rawURL)
	if err != nil {
		http.Error(w, "invalid url parameter", http.StatusBadRequest)
		return
	}

	// Build upstream request
	req, err := http.NewRequestWithContext(r.Context(), "GET", upstream.String(), nil)
	if err != nil {
		http.Error(w, "cannot create request", http.StatusInternalServerError)
		return
	}

	// Custom headers support: header=Key=Value
	for _, h := range r.URL.Query()["header"] {
		parts := strings.SplitN(h, "=", 2)
		if len(parts) == 2 {
			req.Header.Set(parts[0], parts[1])
		}
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "upstream request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Check if it's an M3U8 playlist
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/vnd.apple.mpegurl") ||
		strings.Contains(contentType, "application/x-mpegURL") ||
		strings.HasSuffix(strings.ToLower(upstream.Path), ".m3u8") {

		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		rewriteM3U8(w, resp.Body, upstream, r)
		return
	}

	// Otherwise just stream normally
	for k, vals := range resp.Header {
		// Skip CORS headers from upstream to avoid duplicates
		if k == "Access-Control-Allow-Origin" ||
			k == "Access-Control-Allow-Methods" ||
			k == "Access-Control-Allow-Headers" {
			continue
		}
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func rewriteM3U8(w http.ResponseWriter, body io.Reader, base *url.URL, r *http.Request) {
	scanner := bufio.NewScanner(body)

	for scanner.Scan() {
		line := scanner.Text()

		// Not a URL
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			w.Write([]byte(line + "\n"))
			continue
		}

		// Convert relative -> absolute
		absURL := base.ResolveReference(&url.URL{Path: line})

		// Rewrite to route through your proxy
		proxyURL := url.URL{
			Scheme: scheme,
			Host:   r.Host,
			Path:   "/proxy",
		}
		values := proxyURL.Query()
		values.Set("url", absURL.String())

		// Preserve headers
		for _, h := range r.URL.Query()["header"] {
			values.Add("header", h)
		}

		proxyURL.RawQuery = values.Encode()

		w.Write([]byte(proxyURL.String() + "\n"))
	}
}
