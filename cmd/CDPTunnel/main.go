package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/security"
	"github.com/chromedp/chromedp"
	"gopkg.in/yaml.v3"
)

type Settings struct {
	Mode                 string   `yaml:"mode"`
	ExecAllocator        bool     `yaml:"exec_allocator,omitempty"`
	DevtoolsURL          string   `yaml:"devtoolsURL"`
	HttpServer           string   `yaml:"httpserver"`
	Debug                bool     `yaml:"debug,omitempty"`
	RemoteTunnel         string   `yaml:"remotetunnel"`
	PreLaunchCommand     []string `yaml:"pre_launch_command,omitempty"`
	BrowserLaunchCommand []string `yaml:"browser_launch_command,omitempty"`
	BrowserPath          string   `yaml:"browser_path"`
}

var Config Settings
var Debug bool

func devToolsRequest(targetURL, method, headers, reqBody string) string {
	/*
		     Performs HTTP POST and GET requests using Javascript XHR requests via CDP
			 If exec_allocator is set to true in settings, it uses ExecAllocator to run
			 the browser directly in headless mode. Otherwise, it will attempt to use the
			 remote allocator to connect to CDP via the websocket URL from the devtoolURL config.

			 Args:
			   targetURL: The request URL (the remote tunnel URL for tunnel mode)
		       method: The HTTP method
			   headers: new-line separated HTTP headers
			   reqBody: The request body for POST requests
			Returns:
			   The response body as a string (base64(json) encoded response for tunnel mode)
	*/
	var responseBody string
	var ctx, allocCtx context.Context
	var cancel context.CancelFunc
	if Config.ExecAllocator {
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.ExecPath(Config.BrowserPath),
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("disable-blink-features", "AutomationControlled"),
			chromedp.Flag("disk-cache-dir", "nul"),
			chromedp.Flag("disk-cache-size", "1"),
			chromedp.Flag("disable-background-timer-throttling", true),
			chromedp.Flag("disable-backgrounding-occluded-windows", true),
			chromedp.Flag("disable-breakpad", true),
			chromedp.Flag("disable-client-side-phishing-detection", true),
			chromedp.Flag("disable-default-apps", true),
			chromedp.Flag("disable-extensions", true),
			chromedp.Flag("disable-features", "site-per-process,TranslateUI,BlinkGenPropertyTrees"),
			chromedp.Flag("disable-hang-monitor", true),
			chromedp.Flag("disable-ipc-flooding-protection", true),
			chromedp.Flag("disable-popup-blocking", true),
			chromedp.Flag("disable-prompt-on-repost", true),
			chromedp.Flag("disable-renderer-backgrounding", true),
			chromedp.Flag("disable-sync", true),
			chromedp.Flag("metrics-recording-only", true),
			chromedp.Flag("safebrowsing-disable-auto-update", true),
			chromedp.Flag("enable-automation", true),
			chromedp.Flag("password-store", "basic"),
			chromedp.Flag("use-mock-keychain", true),
			chromedp.NoDefaultBrowserCheck,
			chromedp.NoFirstRun,
			chromedp.NoSandbox,
		)

		allocCtx, cancel = chromedp.NewExecAllocator(context.Background(), opts...)

		defer cancel()
		ctx, cancel = chromedp.NewContext(allocCtx)
		defer cancel()
	} else {
		allocCtx, cancel := chromedp.NewRemoteAllocator(context.Background(), Config.DevtoolsURL)
		defer cancel()
		ctx, cancel = chromedp.NewContext(allocCtx)
		defer cancel()
	}

	evalString := ""
	switch method {
	case "POST":
		evalString = fmt.Sprintf(`(function() {
		var xhr = new XMLHttpRequest();
		xhr.open('%s', '%s', false);
		var rawHeaders = %q; 
		rawHeaders.split('\n').forEach(function(line) {
			var parts = line.split(':');
			if (parts.length >= 2) {
				var key = parts[0].trim();
				var value = parts.slice(1).join(':').trim();
				xhr.setRequestHeader(key, value);
			}
		});
		xhr.send(%q);
		return xhr.responseText
	})()`, method, targetURL, headers, reqBody)
	case "GET":
		evalString = fmt.Sprintf(`(function() {
		var xhr = new XMLHttpRequest();
		xhr.open('%s', '%s', false);
		var rawHeaders = %q; 
		rawHeaders.split('\n').forEach(function(line) {
			var parts = line.split(':');
			if (parts.length >= 2) {
				var key = parts[0].trim();
				var value = parts.slice(1).join(':').trim();
				xhr.setRequestHeader(key, value);
			}
		});
		xhr.send();
		return xhr.responseText
	})()`, method, targetURL, headers)
	}
	if Debug {
		log.Printf("devToolsRequest:evalString:\n%s\n---\n", evalString)
	}
	title := "Automatic Software Update"
	if err := chromedp.Run(ctx,
		network.Enable(),
		security.SetIgnoreCertificateErrors(true),
		network.SetCacheDisabled(true),
		browser.SetDownloadBehavior(browser.SetDownloadBehaviorBehaviorDeny),
		chromedp.Title(&title),
		chromedp.Navigate(targetURL),
		chromedp.Evaluate(evalString, &responseBody),
	); err != nil {
		log.Printf("devToolsRequest:chromedp.Run Error:%v\n", err)
	}

	return responseBody
}

func main() {
	yamlSettings := "settings.yaml"
	if len(os.Args) >= 2 {
		yamlSettings = os.Args[1]
	}
	yamlData, err := os.ReadFile(yamlSettings)
	if err != nil {
		log.Fatalf("Unable to open settings from %s:%v\n", yamlSettings, err)
	}

	err = yaml.Unmarshal([]byte(yamlData), &Config)
	if err != nil {
		log.Fatalf("Yaml Unmarshal error for %s:%v\n", yamlSettings, err)
	}
	if !strings.EqualFold(Config.Mode, "tunnel") && !strings.EqualFold(Config.Mode, "direct") && !strings.EqualFold(Config.Mode, "server") {
		log.Fatalf("Unsupported mode:%s\n", Config.Mode)
	}
	if strings.EqualFold(Config.Mode, "direct") {
		log.Printf("Warning: Direct mode is unstable since each request/response behavior depends on how the browser decides to handle it.")
	}
	Debug = Config.Debug
	var args []string
	if len(Config.PreLaunchCommand) > 0 {
		if len(Config.PreLaunchCommand) >= 1 {
			args = Config.PreLaunchCommand[1:]
		}
		out, err := exec.Command(Config.PreLaunchCommand[0], args...).Output()
		if err != nil {
			log.Printf("Error reported when running PreLaunchCommand '%v':%v\n", Config.PreLaunchCommand, err)
		} else {
			log.Println(string(out))
		}
	}
	if !Config.ExecAllocator && len(Config.BrowserLaunchCommand) > 0 {

		if len(Config.BrowserLaunchCommand) >= 1 {
			args = Config.BrowserLaunchCommand[1:]
		}
		out, err := exec.Command(Config.BrowserLaunchCommand[0], args...).Output()
		if err != nil {
			log.Printf("Error reported when running BrowserLaunchCommand '%v':%v\n", Config.BrowserLaunchCommand, err)
		} else {
			log.Println(string(out))
		}
	}

	HttpServer()
}
