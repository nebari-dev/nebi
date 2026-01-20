package desktop

import (
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ProxyHandler proxies requests from the Wails webview to the embedded HTTP server
type ProxyHandler struct {
	targetURL *url.URL
	client    *http.Client
}

// NewProxyHandler creates a new proxy handler that forwards requests to the target URL
func NewProxyHandler(target string) *ProxyHandler {
	targetURL, _ := url.Parse(target)
	return &ProxyHandler{
		targetURL: targetURL,
		client: &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// ServeHTTP implements http.Handler
func (p *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Build the target URL
	targetURL := *p.targetURL
	targetURL.Path = r.URL.Path
	targetURL.RawQuery = r.URL.RawQuery

	// Create the proxy request
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), r.Body)
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for key, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// Remove hop-by-hop headers
	proxyReq.Header.Del("Connection")
	proxyReq.Header.Del("Keep-Alive")
	proxyReq.Header.Del("Proxy-Authenticate")
	proxyReq.Header.Del("Proxy-Authorization")
	proxyReq.Header.Del("Te")
	proxyReq.Header.Del("Trailers")
	proxyReq.Header.Del("Transfer-Encoding")
	proxyReq.Header.Del("Upgrade")

	// Make the request
	resp, err := p.client.Do(proxyReq)
	if err != nil {
		// If server is not ready, return a loading page
		if strings.Contains(err.Error(), "connection refused") {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(loadingPage))
			return
		}
		http.Error(w, "Failed to proxy request: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Set status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	io.Copy(w, resp.Body)
}

// loadingPage is shown while the server is starting up
const loadingPage = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Darb - Loading</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
            color: #fff;
        }
        .loader-container {
            text-align: center;
        }
        .loader {
            width: 50px;
            height: 50px;
            border: 3px solid rgba(255, 255, 255, 0.1);
            border-radius: 50%;
            border-top-color: #00d4ff;
            animation: spin 1s ease-in-out infinite;
            margin: 0 auto 20px;
        }
        @keyframes spin {
            to { transform: rotate(360deg); }
        }
        h1 {
            font-size: 2rem;
            margin-bottom: 10px;
            font-weight: 300;
        }
        p {
            font-size: 0.9rem;
            opacity: 0.7;
        }
        .retry-script {
            margin-top: 30px;
        }
    </style>
</head>
<body>
    <div class="loader-container">
        <div class="loader"></div>
        <h1>Darb</h1>
        <p>Starting server...</p>
    </div>
    <script>
        // Retry loading the page every second
        setTimeout(function() {
            window.location.reload();
        }, 1000);
    </script>
</body>
</html>`
