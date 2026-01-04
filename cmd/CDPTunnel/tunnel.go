package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type RequestData struct {
	URL     string `json:"URL"`
	Method  string `json:"Method"`
	Headers string `json:"Headers"`
	Body    string `json:"Body"`
}
type ResponseData struct {
	Data string `json:"responseData"`
}

func TunnelRequest(tReq RequestData) (string, string, string, string) {
	/*
		     Takes structured request data and encodes it as a tunnel request.

			 Args:
			   tReq: The client request data
			Returns:
			  URL: The url of the tunnel
			  Method: always 'POST'
			  Headers: The HTTP headers (none needed for now)
			  Body: The POST request body - base64 encoded request
	*/
	//maybe encrypt here if needed?
	encodedB64 := ""
	jsonData, err := json.Marshal(tReq)
	if err != nil {
		log.Printf("TunnelRequest:Unable to Marshal request:%s\n", err)
	} else {
		if Debug {
			log.Printf("TunnelRequest:JSON Request:%s\n", string(jsonData))
		}
		encodedB64 = base64.StdEncoding.EncodeToString(jsonData)
	}
	return "http://" + Config.RemoteTunnel, "POST", "", encodedB64
}

func TunnelRequestDecode(body string) *RequestData {
	/*
		    Takes the base64 encoded POST request body string from TunnelRequest()
			and returns an unpacked RequestData.

			Args:
			  body: The request body string
			Returns:
			  An unpacked structed RequestData pointer
	*/
	var reqData RequestData
	if len(body) < 1 {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		log.Printf("TunnelRequestDecode:Error base64 decoding body [%v]:%s", err, body)
		return nil
	}
	if len(decoded) < 1 {
		return nil
	}
	err = json.Unmarshal(decoded, &reqData)
	if err != nil {
		log.Printf("TunnelRequestDecode:Error JSON decoding body [%v]:%s", err, body)
		return nil
	}
	return &reqData
}

func TunnelHandler(w http.ResponseWriter, r *http.Request) {
	/*
		 An HTTP request handler for tunnel requests coming from browsers.

		Args:
		   w: The HTTP response writer handle to write back to the browser
		   r: The HTTP request reader handle, to get the details of the browser request
	*/
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("TunnelHandler: Failed to read body:%v\n", err)
		http.Error(w, "Failed to read body", http.StatusInternalServerError)
		return
	}
	if Debug {
		log.Printf("TunnelHandler: BYTES:%s\n", string(bodyBytes))
		log.Printf("TunnelHandler: URL:%s\nHeaders:%v\n", getURL(r), r.Header)
	}
	reqData := TunnelRequestDecode(string(bodyBytes))
	if reqData != nil {
		tunnelResponse := HttpRequest(reqData)
		encodedB64 := base64.StdEncoding.EncodeToString(tunnelResponse)
		count, err := fmt.Fprintf(w, `{"responseData":"%s"}`, encodedB64)
		if err != nil {
			log.Printf("TunnelHandler: tunnelHandler encountered an error responding to request:%v\n", err)
		} else if Debug {
			log.Printf("TunnelHandler: Wrote %d bytes through the tunnel\n", count)
		}
	} else if Debug {
		log.Println("TunnelHandler: Ignoring request")
	}
	if Debug {
		log.Println("------------------------------------------------")
	}
}
