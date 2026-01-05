# CDPTunnel
CDPTunnel is a tool that routes HTTP traffic (redteam/c2) via the Chrome DevTools Protocol (CDP) of a web browser.

## When would CDPTunnel be useful?

CDPTunnel is useful in scenarios where the client program communicating directly with an internet destination is undersirable or restricted, whereas not so much for web-browsers.

Example scenarios:
  - EDR/AV tools are restrictive or aggressively detecting unusual processes connecting to the internet
  - Network policy restrictions enforced at the process or traffic-signature level
  - Network restrictions require users interacting with or clicking-through captive-portal pages to access some destinations (e.g.: Block-Continue type IDS/Firewall rules)
  - Traffic inspection middleware is flagging unusual traffic signatures (e.g.: JARM signatures, DPI)
  - DataExfiltration detection solutions flag unusual processes transfering data, but not approved browsers

## Running CDPTunnel

CDPTunnel by default attempts to load configuration from `settings.yaml` in the same directory it is in.
If a comand-line argument is present, the first argument is treated a settings Yaml file, and it will be loaded.

Examples:

`cdptunnel.exe`

`cdptunnel.exe settings_chromium.yaml`

`./cdptunnel settings_tunnel_server.yaml`

## Configuration and Operation

### Mode
There are three modes of operation defined under the `mode` parameter

1) `server` mode will run the tunnel-handler server using the `remotetunnel` parameter; this should be set when running cdptunnel on the c2/redirector and it is handling requests from `tunnel` mode clients.
2) `tunnel` mode is the recommended and more stable client-mode. it will start an HTTP server using the `httpserver` config parameter and encode requests before sending to the tunnel server set under the `remotetunnel` config parameter.
3) `direct` mode is much less stable, it will start an HTTP server using the `httpserver` setting, perform requests directly to destinations using the HTTP `Host` header. This can be useful for uploading/downloading content to sites you don't control, but a lot of instability is introduced as a result of browser behavior and security settings.

### HTTP requests

Clients must direct POST or GET requests using the setting defined in the `httpserver` parameter.
The `Host` header is used to determine the final IP/Domain and Port of the request.
The `scheme` header should be set to `https://` if the request is for an HTTPS destination, if unset, it defaults to `http://`.

Here is a simple POST request using Python REPL to httpbin.org to demonstrate how this works.

```
>>> header={"Host":"httpbin.org","test":"it works","scheme":"https://"}
>>> print(requests.post("http://127.0.0.1:8080/post",headers=header,data="test post data").text)
{
  "args": {},
  "data": "",
  "files": {},
  "form": {},
  "headers": {
    "Accept": "*/*",
    "Accept-Encoding": "gzip, deflate",
    "Content-Length": "0",
    "Host": "httpbin.org",
    "Test": "it works",
    "User-Agent": "python-requests/2.32.4",
    "X-Amzn-Trace-Id": "Root=1-..."
  },
  "json": null,
  "origin": "my external ip",
  "url": "https://httpbin.org/post"
}
```

### Execution allocator

There are two ways of connecting to browsers, by running the browsing using an exec allocator, or by using a remote allocator.

There are pros and cons to both approaches. 

#### Exec allocator

An exec allocator can set lots of flags and run headless browsers. users won't notice the activity, and existing browser sessions are left uninterrupted. The downside is that this is slightly more detectable, since headless browsers and unusual processes starting browsers might stand out a bit more. 

`exec_allocator` should be set to `true` to use this method. The `browser_path` parameter should point to the target browser you with to launch.

You can bring your own browser (any browser supporting CDP) or an electron application that has CDP remote debugging enabled.

#### Remote allocator

Remote allocator connects using the CDP protocol over websocket to a browser running with remote debugging turned on.

If an existing browser session for that browser is active, it might need to be terminated, this can easily be done using the `pre_launch_command` option and setting the approprite termination command there.

If the `browser_launch_command` is set and `exec_allocator` is set to `false`, it will attempt to run the command in question to start a browser; this command should ideally have parameters to configure remote debugging and IP addresses.

The `devtoolsURL` needs to point to the same address as configured in the commandline of the browser that it should connect to. Usually this is a local browser, but it can be used to control browsers across the network. This can be useful when controlling devices that can't reach the internet directly.

The major downside is that to my knowledge, there is no way to prevent the opening of tabs each time a request is made, this will cause the browser's icon to blink constantly unless the browser commandline includes `--headless`. If that is not a problem, or if users getting annoyed by the occasional tab that opens and closes really fast is also not a problem, then this is the most ideal way of connecting to the browser. This is because both cdptunnel and the client/c2 process will not have any interaction with the browser aside from the CDP remote debugging session, and this in turn can evade attempts to correlate process and network events for detections.

## Build

Building is as simple as:

`go build -o cdptunnel.exe .\cmd\CDPTunnel\`
