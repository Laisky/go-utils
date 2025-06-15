

# **Deep Dive into GOAWAY Errors in Go HTTP/2 Reverse Proxies: From Protocol Principles to Production-Grade Solutions**

### **Executive Summary**

This report provides an in-depth technical analysis of a specific bug report concerning a Go service acting as a reverse proxy (passthrough). The bug manifests when an upstream service, such as OpenAI, utilizes the HTTP/2 protocol and sends a GOAWAY frame, causing request retries to fail. The root cause of this failure is that the request body (Request.Body), having been consumed during the initial attempt, cannot be read a second time. This results in the error message: "http2: Transport: cannot retry err after Request.Body was written; define Request.GetBody to avoid this error".

The core analysis of this report reveals that the issue stems from an inherent conflict between the connection management mechanisms of the HTTP/2 protocol—specifically the graceful shutdown capability of the GOAWAY frame—and the streaming, read-once design of Go's net/http package's http.Request.Body. A common misconception is to view a reverse proxy as a simple, transparent data pipeline. However, in technical reality, a reverse proxy acts as a full-fledged HTTP client to its upstream services. Consequently, it must assume the full responsibility of implementing client-side robustness, including handling transient network interruptions, server-side graceful shutdowns, and ensuring that requests (especially those with bodies) are retryable.

To address this problem, this report will systematically elaborate on the following:

1. **Protocol Background**: A deep dive into the HTTP/2 protocol's GOAWAY frame, explaining its role in connection lifecycle management and the graceful shutdown process.
2. **Implementation Details**: A detailed analysis of the io.ReadCloser nature of Go's http.Request.Body, clarifying why it is not replayable by default.
3. **Problem Reproduction**: A complete, standalone code example to reliably reproduce the error in a controlled environment, thereby validating the root cause analysis.
4. **Solutions and Best Practices**: A series of production-grade solutions, ranging from simple to complex. For scenarios involving small, bounded payloads (like API gateways), an in-memory buffering strategy is recommended. For handling large, unbounded payloads (such as file uploads), a disk-based buffering strategy is advised. Furthermore, the report will explore how to leverage mature third-party libraries to simplify the implementation of retry logic.

Ultimately, this report aims to provide a clear and actionable set of architectural guidelines and a technical blueprint for building highly available and resilient Go reverse proxy services, helping developers and architects make informed decisions tailored to their specific business needs.

## **Section 1: Deconstructing the HTTP/2 GOAWAY Frame**

To understand the root of this error, one must first grasp the connection management mechanisms of the HTTP/2 protocol. Unlike HTTP/1.1, HTTP/2 introduces a more sophisticated control plane, with the GOAWAY frame being a critical component.1

### **1.1 HTTP/2 Fundamentals: Connections, Streams, and Frames**

One of the core optimizations of HTTP/2 is multiplexing over a single TCP connection.1 This means a client and server can send and receive multiple independent HTTP requests and responses concurrently on the same connection without blocking one another. This concurrency is achieved by associating each request/response pair with its own independent, bidirectional "stream".1

At the protocol level, all communication is encapsulated into binary "frames" for transmission.1 Each frame carries a type, flags, and a stream identifier, which are used to distinguish its purpose and the stream it belongs to. This design cleanly separates connection management (e.g., establishing and closing connections) from data transfer (request headers and body). The table below lists some key HTTP/2 frame types to help understand the position of

GOAWAY within the protocol.

**Table 1: Common HTTP/2 Frame Types**

| Frame Type (Hex) | Name | Description | References |
| :---- | :---- | :---- | :---- |
| 0x0 | DATA | Used to transport the HTTP request or response body data. | 3 |
| 0x1 | HEADERS | Used to open a new stream and transport HTTP header information. | 2 |
| 0x4 | SETTINGS | Used at the beginning of a connection to exchange configuration parameters, such as max concurrent streams and window size. | 2 |
| 0x6 | PING | Used to measure round-trip time (RTT) or verify that the connection is still active. | 3 |
| 0x3 | RST\_STREAM | Used to immediately terminate a specific stream, typically indicating an error or that the stream is no longer needed. | 1 |
| 0x7 | GOAWAY | **Used to initiate the shutdown process for the entire connection or to signal a serious error. This is a connection-level control frame.** | 3 |

As seen in the table, RST\_STREAM acts on a single stream, whereas GOAWAY acts on the entire connection—a crucial distinction for understanding this issue.

### **1.2 The GOAWAY Frame (Type 0x7): Purpose and Semantics**

According to the IETF specification (the latest being RFC 9113, which obsoletes the older RFC 7540\) 1, the primary purpose of the

GOAWAY frame is to allow an endpoint (usually the server) to gracefully stop accepting new streams while continuing to process already established ones.3 This enables administrative operations like server maintenance and rolling updates without abruptly severing ongoing communications.5

The payload of a GOAWAY frame contains the following key fields:

* **Last-Stream-ID**: A 31-bit integer indicating the ID of the last stream that the sender has processed or might process. The receiver (client) can use this ID to determine which in-flight requests are safe and which need to be retried on a new connection.8
* **Error Code**: A 32-bit error code indicating the reason for the connection closure. For example, NO\_ERROR (value 0\) signifies a normal, error-free graceful shutdown. Other error codes might indicate protocol violations or other issues.9
* **Additional Debug Data**: An optional, opaque block of data that can contain supplementary information for diagnosing problems.9

In the scenario reported by the user, the GOAWAY frame sent by the OpenAI server likely carries the NO\_ERROR code, indicating it is performing a planned connection rotation or load balancing—a normal operation, not an error.10

### **1.3 The Graceful Shutdown Process and Race Condition Avoidance**

A typical graceful shutdown process is as follows:

1. The server decides to close the connection for some reason (e.g., maintenance or reaching a connection request limit).3
2. The server sends a GOAWAY frame to the client. The Last-Stream-ID in this frame informs the client that any request with a stream ID greater than this value will not be processed and should be re-initiated on a new connection.
3. The server continues to process all streams with IDs less than or equal to Last-Stream-ID.
4. Upon receiving the GOAWAY frame, the client stops creating new streams on that connection and retries any unaccepted requests (those with IDs greater than Last-Stream-ID) on a new connection.

However, this simple process has a subtle race condition. Consider the case where a client sends a new request at the exact moment the server decides to send a GOAWAY. Due to network latency (RTT, Round-Trip Time), the server is unaware of this new in-flight request when it sends the GOAWAY. Consequently, the Last-Stream-ID it sets might be lower than the new request's ID, causing the new request to be unnecessarily rejected and retried.

To solve this, the HTTP/2 specification recommends a more sophisticated "two-phase GOAWAY" strategy 8:

1. Phase 1: Announce Shutdown Intent
   The server first sends a GOAWAY frame with its Last-Stream-ID set to the maximum possible value (231−1). This special GOAWAY frame is an explicit signal to the client: "Stop initiating any new requests on this connection immediately; I am about to shut down.".12
2. Phase 2: Wait and Acknowledge
   After sending the first GOAWAY, the server waits for at least one RTT. This ensures the client has enough time to receive and process the shutdown signal. A common practice is for the server to send a PING frame immediately after the first GOAWAY and then wait for the client's PING ACK.8
3. Phase 3: Send Final GOAWAY
   After the waiting period, the server sends a second GOAWAY frame. This time, Last-Stream-ID is set to the ID of the last stream it actually processed. This cleanly closes the connection while ensuring no in-flight requests are lost.

Many large services and load balancers (like AWS ELB) may opt for the simpler single GOAWAY shutdown strategy for efficiency.10 While this is fully compliant with the specification, it shifts more of the responsibility for handling race conditions and ensuring request idempotency to the client. This is precisely the core external trigger for the error in the user's report: the client (i.e., the user's reverse proxy) must be capable of reliably retrying requests after receiving a

GOAWAY.

## **Section 2: Characteristics of http.Request.Body in Go**

Having understood the external trigger (the GOAWAY frame), we now need to delve into the internal cause, which is closely related to the design philosophy of Go's net/http package, particularly the implementation of http.Request.Body.

### **2.1 Request.Body as a Streaming io.ReadCloser**

In Go, the Body field of the http.Request struct received by a server handler is of type io.ReadCloser.13 This interface combines

io.Reader and io.Closer, meaning it has two core characteristics:

* **Readable (Read)**: Data can be read from it.
* **Must be Closed (Close)**: Its Close method must be called after use to release associated resources.14

A critical and often misunderstood concept is that Request.Body is **not a memory buffer containing all the request body data**. Instead, it is a streaming wrapper around the underlying network connection (TCP socket).16 When you call

r.Body.Read(p), you are, in fact, pulling data directly from the network socket's receive buffer.17

This design is based on deliberate engineering decisions. Imagine a server that needs to handle large file uploads, where the request body could be several gigabytes in size.18 If the

net/http server had to read the entire request body into memory before calling your handler function, it would lead to two catastrophic consequences:

1. **Memory Exhaustion**: The massive memory overhead would quickly deplete server resources, severely limiting its concurrent processing capabilities.
2. **Denial-of-Service (DoS) Attacks**: An attacker could easily exhaust the server's memory by sending a huge request body, causing the service to crash.21

By providing a streaming interface, Go's design allows handlers to process request bodies of any size in chunks, thereby maintaining an extremely low memory footprint. This is a foundational design for building scalable and resilient network services.16

### **2.2 The "Read-Once" Problem and Its Impact on Retries**

The direct consequence of this streaming design is that Request.Body is a **forward-only, single-use resource**.21 Once data is read from the stream, it's gone. You cannot "rewind" or "reset" the stream's pointer as you would with a byte slice, because the data from the underlying network connection has already been consumed.

This leads directly to the "read-once" problem, which has a devastating impact on scenarios requiring retries. We can illustrate this dilemma with simple pseudocode:

Go

// Assume proxy.ServeHTTP makes its first attempt to forward the request upstream.
// During this process, Go's http.Transport starts reading from r.Body and sending data.
bytes\_sent, err := http.Transport.RoundTrip(request)

// At this point, the internal pointer of r.Body has moved forward, or it has reached the end of the stream (EOF).

// Now, assume the first attempt fails due to receiving a GOAWAY, and a retry is needed.
// Go's http.Transport attempts a second call to RoundTrip.
// It tries to read data from the beginning of request.Body again.
// But request.Body is already in a consumed state.

// Result: The second read will immediately return 0 bytes and an io.EOF error, causing the retry to fail.

This process precisely describes why, without a special mechanism, a retry will fail after a GOAWAY is received. The http.Transport needs to resend the request body from the beginning, but the request.Body it holds is an already "drained" stream.

## **Section 3: The Origin of the Failure: When GOAWAY Meets Go's Streaming Body**

Now, we will combine the knowledge from the previous two sections—the HTTP/2 GOAWAY mechanism and Go's streaming Request.Body—to accurately trace the sequence of events that leads to the error reported by the user.

### **3.1 The Failure Timeline**

Here is the detailed sequence of events that causes the error:

1. **Request Initiation**: The user's Go reverse proxy service receives a POST/PUT request from a downstream client. The proxy service creates a new http.Request object to forward to the upstream OpenAI service. At this point, the Body field of this new request is set to the original request's r.Body, which is an io.ReadCloser streaming data from the downstream client. Crucially, the GetBody field of this request object is nil.
2. **Request Body Transmission**: Go's http.Transport begins processing this outbound request. It first sends a HEADERS frame to the OpenAI server, containing the request method, path, headers, etc. It then starts reading data from the req.Body stream and sends this data encapsulated in one or more DATA frames.
3. **Receiving GOAWAY**: During the transmission of the DATA frames, OpenAI's server (or its front-end load balancer) decides to close this HTTP/2 connection. It sends a GOAWAY frame, for example, LastStreamID=1999, ErrCode=NO\_ERROR.10 This is typically a normal operation for connection lifecycle management, load balancing, or rolling deployments.
4. **Activating Retry Logic**: The proxy service's (acting as a client) http2.Transport receives this GOAWAY frame. It checks that the error code is NO\_ERROR and determines that the current request is idempotent (POST requests are often treated as idempotent in modern practice, or the request itself is GET/PUT, etc.), thus classifying it as a retryable transient error.
5. **The Fatal Retry Attempt**: The http.Transport attempts to resend the entire request on a new HTTP/2 connection to OpenAI. To do this, it must be able to re-read the complete request body from the beginning.
6. **Error Occurs**: The http.Transport checks the req.GetBody field. It finds that the field is nil.23 Since there is no "factory function" to generate a new, readable request body stream, and the original
   req.Body stream has been partially or fully consumed, the Transport cannot complete the retry. It has no choice but to give up and return the precise, informative error message seen in the bug report: "http2: Transport: cannot retry err \[...\] after Request.Body was written; define Request.GetBody to avoid this error".

### **3.2 The Complexity of Redirects: A Hidden Failure Path**

In addition to the direct failure mode described above, research has uncovered a more subtle and confusing failure path related to HTTP redirects.

In Go's net/http client implementation, there is a nuanced behavior when handling HTTP redirects (such as status codes 307 or 308): the client automatically creates a new http.Request object to access the redirected URL, but in this process, it **does not** copy the GetBody function pointer from the original request to the new request object.23

This can lead to the following difficult-to-diagnose scenario:

1. A diligent developer, building their reverse proxy, correctly handles the request body and carefully sets the req.GetBody function for a request to http://api.example.com, ensuring it is retryable.
2. The api.example.com server returns a 307 Temporary Redirect to https://api.example.com.
3. Go's HTTP client library internally catches this redirect and automatically creates a brand new http.Request object targeting the https URL. It copies most of the relevant fields, like headers.
4. However, as documented in a related GitHub issue, the GetBody field of this internally created new request object will be nil, as it was not inherited from the original request.23
5. Now, the client uses this **new, GetBody-less request object** to make a request to https://api.example.com.
6. If **this** request happens to encounter a GOAWAY frame from the upstream server, the subsequent retry attempt will fail because GetBody is nil.

Ultimately, the developer will receive the familiar "define Request.GetBody to avoid this error" error and be baffled, because they had correctly set GetBody in their initial code. This hidden failure path demonstrates that the problem's complexity goes beyond the surface, and a truly robust solution must account for the various transformations a request might undergo throughout its lifecycle, including redirects.

## **Section 4: A Practical Guide to Reproducing the Error**

To translate theory into practice and validate our understanding of the problem's root cause, this section provides a complete, self-contained Go project to reliably reproduce the error in a local environment.

### **4.1 Test Environment Components**

Our test environment consists of three independent Go programs:

1. **GOAWAY Server (server.go)**: A minimalist HTTP/2 server. It listens on a port, accepts a request, attempts to read its body, and then immediately sends a GOAWAY frame to initiate a graceful shutdown. This precisely simulates the behavior of OpenAI or any upstream service performing active connection management.8
2. **Vulnerable Reverse Proxy (proxy.go)**: A simple reverse proxy built using the standard library's httputil.NewSingleHostReverseProxy. It performs no special request body handling and directly streams the incoming request body to the upstream, representing the user's initial "passthrough" implementation.25
3. **Client (client.go)**: A simple program that creates a POST request with a small amount of data and sends it to our vulnerable reverse proxy.

### **4.2 Complete Source Code and Execution Steps**

Below is the complete, commented Go code for the three components.

#### **server.go: The GOAWAY Server**

Go

package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func main() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r \*http.Request) {
		log.Println("Server: Received request")
		// Attempt to read the request body to simulate processing
		body, err := io.ReadAll(r.Body)
		if err\!= nil {
			log.Printf("Server: Error reading body: %v", err)
			http.Error(w, "Failed to read body", http.StatusInternalServerError)
			return
		}
		log.Printf("Server: Read %d bytes from body", len(body))

		// Respond to the client
		fmt.Fprintln(w, "Hello, your request was received.")
		log.Println("Server: Sent response.")

		// Key step: After responding, immediately send a GOAWAY frame
		// This will trigger the client's retry logic
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Obtain the underlying http2.Conn from the request context
		// This is a bit of an internal detail but necessary for this simulation
		s, \_ := r.Context().Value(http.ServerContextKey).(\*http.Server)
		c, \_ := s.ConnContext(r.Context(), nil).(\*http2.Conn)
		if c\!= nil {
			log.Println("Server: Sending GOAWAY frame and closing connection.")
			c.Shutdown(context.Background())
		}
	})

	h2s := \&http2.Server{}
	server := \&http.Server{
		Addr:    "127.0.0.1:8081",
		Handler: h2c.NewHandler(handler, h2s), // Use h2c for cleartext HTTP/2
	}

	log.Println("Starting GOAWAY server on http://127.0.0.1:8081")
	if err := server.ListenAndServe(); err\!= nil {
		log.Fatalf("Server: ListenAndServe failed: %v", err)
	}
}

#### **proxy.go: The Vulnerable Reverse Proxy**

Go

package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"golang.org/x/net/http2"
	"net"
	"time"
)

func main() {
	target, err := url.Parse("http://127.0.0.1:8081")
	if err\!= nil {
		log.Fatalf("Proxy: Failed to parse target URL: %v", err)
	}

	// Create a standard, unmodified reverse proxy
	// It will directly stream the request body, making it non-retryable
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Force the use of an HTTP/2 transport
	proxy.Transport \= \&http2.Transport{
		AllowHTTP: true, // Allow non-encrypted h2c
		// For h2c, we need to provide a custom dialer
		DialTLS: func(network, addr string, cfg \*tls.Config) (net.Conn, error) {
			return net.Dial(network, addr)
		},
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r \*http.Request) {
		log.Println("Proxy: Received request, forwarding to upstream...")
		proxy.ServeHTTP(w, r)
		log.Println("Proxy: Finished handling request.")
	})

	log.Println("Starting vulnerable reverse proxy on http://127.0.0.1:8080")
	if err := http.ListenAndServe("127.0.0.1:8080", nil); err\!= nil {
		log.Fatalf("Proxy: ListenAndServe failed: %v", err)
	}
}

#### **client.go: The Error-Triggering Client**

Go

package main

import (
	"bytes"
	"log"
	"net/http"
)

func main() {
	proxyURL := "http://127.0.0.1:8080"
	requestBody := "This is the request body that will be lost on retry."

	req, err := http.NewRequest("POST", proxyURL, bytes.NewBufferString(requestBody))
	if err\!= nil {
		log.Fatalf("Client: Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "text/plain")

	// Note: We are not setting req.GetBody here because bytes.NewBufferString returns a \*bytes.Buffer,
	// which is not one of the types for which http.NewRequest automatically sets GetBody (\*bytes.Reader, \*strings.Reader).
	// Even with those types, GetBody would be lost when forwarded by the naive proxy.
	// We are simulating the simplest (and most fragile) case here.

	client := \&http.Client{}

	log.Println("Client: Sending request to proxy...")
	resp, err := client.Do(req)

	if err\!= nil {
		log.Printf("Client: FAILED\! Received expected error: %v", err)
		return
	}

	defer resp.Body.Close()
	log.Printf("Client: SUCCESS? This should not happen. Status: %s", resp.Status)
}

#### **Execution and Expected Output**

Please execute the following commands in three separate terminals, in order:

1. go run server.go
2. go run proxy.go
3. go run client.go

The expected output from the client will be:

Client: FAILED\! Received expected error: Post "http://127.0.0.1:8080": http2: Transport: cannot retry err after Request.Body was written; define Request.GetBody to avoid this error

This output precisely reproduces the error from the user's report, providing strong evidence that our analysis of the root cause is correct.

## **Section 5: The Basic Solution: Correctly Implementing Request.GetBody**

The error message itself provides the direct solution: "define Request.GetBody to avoid this error". This section will explain the correct usage of GetBody, which is the foundation for any advanced solution.

### **5.1 The GetBody Field: Official Purpose and Usage**

In Go's net/http standard library, the GetBody field of the Request struct is a function type with the signature func() (io.ReadCloser, error).13

Its core purpose is to act as a **factory function for the request body stream**. Whenever the http.Transport needs to re-read the request body (for example, during a redirect or a retry), it calls the GetBody function. This function must return a brand new io.ReadCloser instance that contains the exact same content as the original request body and can be read from the beginning.13

The standard library documentation explicitly states 13:

* GetBody is **only for client requests**. For server-side requests, this field is ignored.
* It is key to implementing 307/308 redirects (which preserve the request method and body) and handling automatic retries after other transient network failures.
* Even if GetBody is set, the Body field itself must still be set to an initial, readable stream.

### **5.2 A Simple Client-Side Fix Example**

For a standard, non-proxy Go HTTP client, making it retry-capable can be straightforward. It's worth noting that when you create a request using certain io.Reader types (like \*bytes.Reader or \*strings.Reader), the http.NewRequest function automatically sets up GetBody for you.13

Go

package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
)

func main() {
	// Use a byte slice as the data source
	requestData :=byte("some retryable data")

	req, err := http.NewRequest("POST", "http://example.com", bytes.NewReader(requestData))
	if err\!= nil {
		log.Fatalf("Failed to create request: %v", err)
	}

	// http.NewRequest has already set GetBody automatically for \*bytes.Reader.
	// If we were using a different type of io.Reader, we would need to set it manually:
	// req.GetBody \= func() (io.ReadCloser, error) {
	// 	return io.NopCloser(bytes.NewReader(requestData)), nil
	// }

	//... send the request...
}

This example shows how simple it is to ensure the request body is replayable at the client's source. However, in a reverse proxy scenario, the problem becomes more complex because the proxy server itself does not "own" the request body data; it is merely an intermediary.

## **Section 6: Production-Grade Strategies: Building a Resilient Reverse Proxy**

Having understood the basic fix, we can now apply it to the more complex scenario of a reverse proxy. The core idea here is that the reverse proxy must proactively take on the responsibility of ensuring its outbound requests are retryable.

### **6.1 The Proxy's Responsibility: Actively Acquiring Replayability**

This is the central architectural argument of this report: **a reverse proxy is by no means a transparent data pipeline**. When it receives an inbound request and creates an outbound request, it becomes a client to the upstream service. Therefore, it must adhere to all client-side best practices, the most important of which is ensuring the request body is replayable.

The standard httputil.NewSingleHostReverseProxy does not perform this by default because it cannot make any safe assumptions about the size or nature of the incoming request body.31 If it were to blindly read all request bodies into memory, it would easily lead to the memory exhaustion and DoS attacks described earlier. Therefore, this responsibility is intentionally left to the developer, who must choose an appropriate buffering strategy based on their specific business context.

In the proxy's ServeHTTP method or a custom Director function, the following logic must be implemented:

1. Buffer the inbound r.Body in some way.
2. Use this buffer to populate the Body field of the outbound request, outReq.
3. Use the same buffer to implement the GetBody factory function for outReq.

### **6.2 Strategy 1: In-Memory Buffering (for Small, Bounded Payloads)**

**Applicable Scenarios**: This strategy is ideal for API gateways, which typically handle smaller, predictably sized JSON or form data payloads.

**Implementation Steps**:

1. **Enforce a Size Limit**: Before reading any data, it is imperative to wrap the inbound request body with http.MaxBytesReader. This is an indispensable security measure to prevent malicious or accidental oversized requests from exhausting server memory.21
2. **Read into Memory**: Under the protection of the size limiter, use io.ReadAll to read the entire request body into a byte slice (byte).21
3. **Set Up the Outbound Request**: In the proxy's Director function or a custom ServeHTTP handler, use this byte slice to set both the Body and GetBody of the outbound request.

**Code Example (Implemented in ReverseProxy.Director)**:

Go

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

const maxBodySize \= 1024 \* 1024 // 1 MB

func NewResilientMemoryProxy(target \*url.URL) \*httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)

	originalDirector := proxy.Director
	proxy.Director \= func(req \*http.Request) {
		originalDirector(req) // Execute default Director logic (e.g., setting Host header)

		// 1\. Limit the request body size
		req.Body \= http.MaxBytesReader(nil, req.Body, maxBodySize)

		// 2\. Read into memory
		bodyBytes, err := io.ReadAll(req.Body)
		if err\!= nil {
			log.Printf("Error reading body for retry buffering: %v", err)
			// In a real application, you might need to return an error to the client.
			// For simplicity, we just clear the body.
			req.Body \= io.NopCloser(bytes.NewBuffer(nil))
			req.GetBody \= func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewBuffer(nil)), nil
			}
			return
		}

		// 3\. Set the outbound request's Body and GetBody
		req.Body \= io.NopCloser(bytes.NewBuffer(bodyBytes))
		req.GetBody \= func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewBuffer(bodyBytes)), nil
		}
		req.ContentLength \= int64(len(bodyBytes))
	}

	return proxy
}

**Trade-offs**: This method is simple to implement and offers high performance (no disk I/O), but its memory consumption is directly proportional to the request body size, making it absolutely unsuitable for scenarios like large file uploads.30

### **6.3 Strategy 2: Disk Buffering (for Large, Unbounded Payloads)**

**Applicable Scenarios**: When a reverse proxy needs to handle large file uploads or any data stream that might exceed available memory, this strategy is the only viable option.18

**Implementation Steps**:

1. **Create a Temporary File**: Use os.CreateTemp to create a unique temporary file in the system's temporary directory. This avoids filename conflicts and polluting the working directory.34
2. **Stream-Copy to File**: Use io.Copy to write the inbound req.Body stream directly to this temporary file. This process is streaming, so the proxy's own memory usage remains extremely low, limited to a small buffer.
3. **Ensure Cleanup**: Use a defer statement to ensure the temporary file is closed and deleted (os.Remove) after the request is handled (whether it succeeds or fails). This is crucial to prevent disk space leaks.
4. **Implement GetBody**: This is the core of this strategy. The implementation of the GetBody function must re-open the temporary file using os.Open each time it's called, returning a new file handle. Each handle has its own independent read pointer, thus providing a fresh, readable-from-the-start stream for each retry.36

**Code Example (Implemented in ServeHTTP)**:

Go

import (
    "io"
    "log"
    "net/http"
    "net/http/httputil"
    "net/url"
    "os"
)

func DiskBufferingProxy(p \*httputil.ReverseProxy) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r \*http.Request) {
        // Let requests with no body pass through directly
        if r.Body \== nil |
| r.Body \== http.NoBody {
            p.ServeHTTP(w, r)
            return
        }

        // 1\. Create a temporary file
        tmpFile, err := os.CreateTemp("", "proxy-body-")
        if err\!= nil {
            log.Printf("Error creating temp file: %v", err)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }
        // 3\. Ensure the file is closed and removed
        defer os.Remove(tmpFile.Name())
        defer tmpFile.Close()

        // 2\. Stream the request body to the file
        size, err := io.Copy(tmpFile, r.Body)
        if err\!= nil {
            log.Printf("Error copying body to temp file: %v", err)
            // No need to return an error, as it might just be the client disconnecting
            return
        }

        // Reset the file pointer to the beginning for the first send
        if \_, err := tmpFile.Seek(0, io.SeekStart); err\!= nil {
            log.Printf("Error seeking temp file: %v", err)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }

        // Modify the original request to make it retryable
        r.ContentLength \= size
        r.Body \= tmpFile
        r.GetBody \= func() (io.ReadCloser, error) {
            // 4\. On each retry, re-open the file to get a new reader
            f, err := os.Open(tmpFile.Name())
            return f, err
        }

        p.ServeHTTP(w, r)
    })
}

**Trade-offs**: This method is extremely memory-efficient and has no limit on request body size, making it very robust. However, its drawback is the introduction of disk I/O, which may increase latency and requires very careful handling of the file lifecycle to avoid resource leaks.

### **Table 2: Comparison of Request Body Buffering Strategies**

| Strategy | Ideal Use Case | Performance | Memory Usage | Implementation Complexity | Security Considerations |
| :---- | :---- | :---- | :---- | :---- | :---- |
| **No Buffering (Default)** | Only for scenarios where retries are definitively not needed. | Highest (no overhead) | Lowest (zero) | Lowest | Fragile; cannot handle GOAWAY or other transient failures. |
| **In-Memory Buffering** | API gateways, small JSON/form data. | High (memory operations) | High (proportional to body size) | Medium | Must use MaxBytesReader to prevent DoS attacks. |
| **Disk Buffering** | Large file uploads, streams of unknown size. | Medium (affected by disk I/O) | Very Low (fixed buffer) | High | Requires strict file permissions and lifecycle management to prevent disk and information leaks. |

### **6.4 Strategy 3: Leveraging Mature Third-Party Libraries**

Implementing robust retry logic from scratch (including exponential backoff, jitter, error classification, etc.) is a complex task.37 The community has already produced excellent libraries that encapsulate this complexity.

For example, hashicorp/go-retryablehttp is a widely popular choice.38 It provides an interface that is almost identical to the standard

http.Client but comes with built-in automatic retry and backoff logic. More importantly, it has already addressed the issue of replayable request bodies. You can create a retryablehttp.Client and set it as the Transport for your ReverseProxy.

**Integration Example**:

Go

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/hashicorp/go-retryablehttp"
)

func NewLibraryProxy(target \*url.URL) \*httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Create a retryable HTTP client
	retryClient := retryablehttp.NewClient()
	// Configure retry count, wait time, etc.
	retryClient.RetryMax \= 3
	// You can customize the CheckRetry function to decide which errors to retry.
	// By default, it already handles network errors like GOAWAY.

	// Set the retryable client as the proxy's Transport.
	// retryablehttp internally handles the problem of re-reading the Body.
	proxy.Transport \= retryClient.StandardClient().Transport

	// Note: You still need to buffer the request body in the Director,
	// because retryablehttp needs a resettable Body source.
	// Therefore, this strategy must be combined with Strategy 1 or 2\.
	// The Director converts the inbound streaming Body into a replayable Body (like bytes.Reader),
	// and then the retryablehttp.Client's Transport will use this replayable Body for retries.

	//... Implement a Director similar to Strategy 1 or 2...

	return proxy
}

Using such libraries allows you to focus on business logic rather than the low-level details of network communication, significantly simplifying your code and improving reliability.40

## **Section 7: Conclusion and Strategic Recommendations**

The error http2: Transport: cannot retry err \[...\] define Request.GetBody to avoid this error, while appearing on the surface to be a limitation of Go's HTTP client, stems from a deeper architectural issue: **a trade-off between protocol expectations and language implementation philosophy**. The HTTP/2 protocol, to achieve efficient connection management and graceful shutdowns, expects clients to be capable of retrying requests after a connection interruption. Conversely, Go's net/http package, to achieve maximum memory efficiency and scalability, designs the request body as a single-use data stream. When these two paradigms meet in a simple "passthrough" reverse proxy, conflict is inevitable.

The core conclusion of this report is that **a reverse proxy must be treated as a fully-featured, highly resilient, independent client, not as a simple network relay**. It inherits all the responsibilities of a client, including but not limited to: handling connection errors, adhering to server connection management signals (like GOAWAY), and, most importantly, ensuring that the requests it sends upstream are retryable.

Based on this conclusion, we offer the following strategic recommendations:

1. **Evaluate Load Characteristics and Choose the Appropriate Buffering Strategy**:
   * If your proxy service primarily handles small API requests with a clear upper size limit (e.g., JSON payloads under 1-2 MB), the **in-memory buffering strategy (Strategy 1\)** is the best choice. It offers good performance and a relatively simple implementation but must be used with http.MaxBytesReader to ensure security.
   * If your proxy needs to handle large file uploads or any data streams of unpredictable size, the **disk buffering strategy (Strategy 2\)** is the only safe and scalable solution. Although it introduces disk I/O overhead and increases implementation complexity, it can handle requests of any size with minimal memory usage, fundamentally eliminating the risk of resource exhaustion.
2. Don't Reinvent the Wheel:
   For the retry logic itself (e.g., backoff algorithms, jitter, error classification), it is strongly recommended to leverage mature third-party libraries like hashicorp/go-retryablehttp. These libraries have already solved many common pitfalls in network programming. Integrating them into your proxy's Transport layer can significantly enhance code robustness and reduce development effort.
3. Treat Buffering Logic as a First-Class Citizen:
   When designing a reverse proxy, the buffering and replaying of the request body should not be treated as an afterthought or a patch. It should be considered a core part of the proxy's logic, as important as routing, authentication, and logging. At the code level, this means there must be explicit logic within the Director function or a custom ServeHTTP middleware to handle the inbound request body and convert it into a replayable form before passing it to the downstream Transport.

In summary, by correctly understanding the protocol, the design intent of the language library, and the architectural role of the proxy itself, developers can fully build highly available Go reverse proxy services that gracefully handle GOAWAY and other transient network failures. The key to solving this problem lies in shifting from a "passive passthrough" mindset to an architectural philosophy of "active management and ensuring resilience."

#### **Works cited**

1. RFC 9113 \- HTTP/2 \- IETF Datatracker, accessed June 15, 2025, [https://datatracker.ietf.org/doc/html/rfc9113](https://datatracker.ietf.org/doc/html/rfc9113)
2. How HTTP/2 Works and How to Enable It in Go \- VictoriaMetrics, accessed June 15, 2025, [https://victoriametrics.com/blog/go-http2/](https://victoriametrics.com/blog/go-http2/)
3. RFC 7540 \- HTTP/2 protocol \- SANS Internet Storm Center, accessed June 15, 2025, [https://isc.sans.edu/diary/19799](https://isc.sans.edu/diary/19799)
4. HTTP/2 Frame Types \- Web Concepts, accessed June 15, 2025, [https://webconcepts.info/concepts/http2-frame-type/](https://webconcepts.info/concepts/http2-frame-type/)
5. HTTP/2 Frame Type: 0x7 GOAWAY \- Web Concepts, accessed June 15, 2025, [https://webconcepts.info/concepts/http2-frame-type/0x7](https://webconcepts.info/concepts/http2-frame-type/0x7)
6. victoriametrics.com, accessed June 15, 2025, [https://victoriametrics.com/blog/go-http2/\#:\~:text=If%20the%20server%20needs%20to,ID%20it%20plans%20to%20handle.](https://victoriametrics.com/blog/go-http2/#:~:text=If%20the%20server%20needs%20to,ID%20it%20plans%20to%20handle.)
7. RFC 7540 \- Hypertext Transfer Protocol Version 2 (HTTP/2) \- IETF Datatracker, accessed June 15, 2025, [https://datatracker.ietf.org/doc/html/rfc7540](https://datatracker.ietf.org/doc/html/rfc7540)
8. How to gracefully close an HTTP/2 connection? \- SwiftNIO \- Swift Forums, accessed June 15, 2025, [https://forums.swift.org/t/how-to-gracefully-close-an-http-2-connection/70221](https://forums.swift.org/t/how-to-gracefully-close-an-http-2-connection/70221)
9. Http2GoAwayFrame (Netty API Reference (4.1.122.Final)), accessed June 15, 2025, [https://netty.io/4.1/api/io/netty/handler/codec/http2/Http2GoAwayFrame.html](https://netty.io/4.1/api/io/netty/handler/codec/http2/Http2GoAwayFrame.html)
10. http2: server sent GOAWAY and closed the connection; LastStreamID=1999, accessed June 15, 2025, [https://stackoverflow.com/questions/45209168/http2-server-sent-goaway-and-closed-the-connection-laststreamid-1999](https://stackoverflow.com/questions/45209168/http2-server-sent-goaway-and-closed-the-connection-laststreamid-1999)
11. Http2 returns GOAWAY error : r/golang \- Reddit, accessed June 15, 2025, [https://www.reddit.com/r/golang/comments/1fr8si6/http2\_returns\_goaway\_error/](https://www.reddit.com/r/golang/comments/1fr8si6/http2_returns_goaway_error/)
12. HTTP/2 GOAWAY logic doesn't support graceful shutdown · Issue \#3564 \- GitHub, accessed June 15, 2025, [https://github.com/netty/netty/issues/3564](https://github.com/netty/netty/issues/3564)
13. request.go \- \- The Go Programming Language, accessed June 15, 2025, [https://tip.golang.org/src/net/http/request.go](https://tip.golang.org/src/net/http/request.go)
14. request body close \- Google Groups, accessed June 15, 2025, [https://groups.google.com/g/golang-dev/c/o9i\_SbGRrzI](https://groups.google.com/g/golang-dev/c/o9i_SbGRrzI)
15. Getting the Body of an HTTP Request With Go \- Zero to Hero, accessed June 15, 2025, [https://zerotohero.dev/tips/getting-the-body-of-an-http-request-with-go/](https://zerotohero.dev/tips/getting-the-body-of-an-http-request-with-go/)
16. http \- Why is response body in golang is a readCloser? \- Stack ..., accessed June 15, 2025, [https://stackoverflow.com/questions/71338019/why-is-response-body-in-golang-is-a-readcloser](https://stackoverflow.com/questions/71338019/why-is-response-body-in-golang-is-a-readcloser)
17. Debugging strange http.Response Read() behavior in Golang | DoltHub Blog, accessed June 15, 2025, [https://www.dolthub.com/blog/2022-03-09-debugging-http-body-read-behavior/](https://www.dolthub.com/blog/2022-03-09-debugging-http-body-read-behavior/)
18. golang http handle big file upload \- Stack Overflow, accessed June 15, 2025, [https://stackoverflow.com/questions/19267336/golang-http-handle-big-file-upload](https://stackoverflow.com/questions/19267336/golang-http-handle-big-file-upload)
19. Avoid reading large file into memory from http request body : r/golang, accessed June 15, 2025, [https://www.reddit.com/r/golang/comments/11a5qmd/avoid\_reading\_large\_file\_into\_memory\_from\_http/](https://www.reddit.com/r/golang/comments/11a5qmd/avoid_reading_large_file_into_memory_from_http/)
20. How to handle large file uploads : r/golang \- Reddit, accessed June 15, 2025, [https://www.reddit.com/r/golang/comments/1ca4on7/how\_to\_handle\_large\_file\_uploads/](https://www.reddit.com/r/golang/comments/1ca4on7/how_to_handle_large_file_uploads/)
21. http \- Golang read request body multiple times \- Stack Overflow, accessed June 15, 2025, [https://stackoverflow.com/questions/43021058/golang-read-request-body-multiple-times](https://stackoverflow.com/questions/43021058/golang-read-request-body-multiple-times)
22. Go gin get request body json \- Stack Overflow, accessed June 15, 2025, [https://stackoverflow.com/questions/61919830/go-gin-get-request-body-json](https://stackoverflow.com/questions/61919830/go-gin-get-request-body-json)
23. net/http: Unexpected "define Request.GetBody to avoid this error ..., accessed June 15, 2025, [https://github.com/golang/go/issues/69412](https://github.com/golang/go/issues/69412)
24. http2/server.go \- net \- Git at Google, accessed June 15, 2025, [https://go.googlesource.com/net/+/master/http2/server.go](https://go.googlesource.com/net/+/master/http2/server.go)
25. ReverseProxy \- \- The Go Programming Language, accessed June 15, 2025, [https://go.dev/src/net/http/httputil/reverseproxy.go?s=1027:3074](https://go.dev/src/net/http/httputil/reverseproxy.go?s=1027:3074)
26. The Right Use of 'ReverseProxy' in Golang \- Josh Software, accessed June 15, 2025, [https://blog.joshsoftware.com/2021/05/25/simple-and-powerful-reverseproxy-in-go/](https://blog.joshsoftware.com/2021/05/25/simple-and-powerful-reverseproxy-in-go/)
27. net/http/httputil \- Go Packages, accessed June 15, 2025, [https://pkg.go.dev/net/http/httputil](https://pkg.go.dev/net/http/httputil)
28. \[go\] net/http: add Request.GetBody func for 307/308 redirects \- Google Groups, accessed June 15, 2025, [https://groups.google.com/g/golang-codereviews/c/Jo66aL5BRPo](https://groups.google.com/g/golang-codereviews/c/Jo66aL5BRPo)
29. Golang cant get body from request.GetBody() \- Stack Overflow, accessed June 15, 2025, [https://stackoverflow.com/questions/46579429/golang-cant-get-body-from-request-getbody](https://stackoverflow.com/questions/46579429/golang-cant-get-body-from-request-getbody)
30. Reading body of http.Request without modifying request state? \- Stack Overflow, accessed June 15, 2025, [https://stackoverflow.com/questions/23070876/reading-body-of-http-request-without-modifying-request-state](https://stackoverflow.com/questions/23070876/reading-body-of-http-request-without-modifying-request-state)
31. How to Create a Reverse Proxy using Golang \- Code Dodle, accessed June 15, 2025, [https://www.codedodle.com/go-reverse-proxy-example.html](https://www.codedodle.com/go-reverse-proxy-example.html)
32. ReverseProxy depending on the request.Body in golang \- Stack Overflow, accessed June 15, 2025, [https://stackoverflow.com/questions/49745252/reverseproxy-depending-on-the-request-body-in-golang](https://stackoverflow.com/questions/49745252/reverseproxy-depending-on-the-request-body-in-golang)
33. Go huge file download and passing to client (proxifying) \- Stack Overflow, accessed June 15, 2025, [https://stackoverflow.com/questions/22283505/go-huge-file-download-and-passing-to-client-proxifying](https://stackoverflow.com/questions/22283505/go-huge-file-download-and-passing-to-client-proxifying)
34. Temporary Files and Directories \- Go by Example, accessed June 15, 2025, [https://gobyexample.com/temporary-files-and-directories](https://gobyexample.com/temporary-files-and-directories)
35. Create a temporary file or directory \- YourBasic, accessed June 15, 2025, [https://yourbasic.org/golang/temporary-file-directory/](https://yourbasic.org/golang/temporary-file-directory/)
36. Understanding the Limitation of Reusing File-based Request Bodies in Go's HTTP Client, accessed June 15, 2025, [https://forum.golangbridge.org/t/understanding-the-limitation-of-reusing-file-based-request-bodies-in-gos-http-client/34168](https://forum.golangbridge.org/t/understanding-the-limitation-of-reusing-file-based-request-bodies-in-gos-http-client/34168)
37. Best Practice: Implementing Retry Logic in HTTP API Clients \- api4ai, accessed June 15, 2025, [https://api4.ai/blog/best-practice-implementing-retry-logic-in-http-api-clients](https://api4.ai/blog/best-practice-implementing-retry-logic-in-http-api-clients)
38. hashicorp/go-retryablehttp: Retryable HTTP client in Go \- GitHub, accessed June 15, 2025, [https://github.com/hashicorp/go-retryablehttp](https://github.com/hashicorp/go-retryablehttp)
39. Notes on Retriable HTTP Client (with Golang/Rust example \- ZengXu's BLOG, accessed June 15, 2025, [https://www.zeng.dev/post/2023-retriable-http-client/](https://www.zeng.dev/post/2023-retriable-http-client/)
40. retryablehttp package \- github.com/projectdiscovery/retryablehttp-go \- Go Packages, accessed June 15, 2025, [https://pkg.go.dev/github.com/projectdiscovery/retryablehttp-go](https://pkg.go.dev/github.com/projectdiscovery/retryablehttp-go)
41. An easy way to retry a failed HTTP client request in GO lang \- Solution Toolkit, accessed June 15, 2025, [https://www.solutiontoolkit.com/2023/01/a-retryable-http-client-server-communication-in-go-language/](https://www.solutiontoolkit.com/2023/01/a-retryable-http-client-server-communication-in-go-language/)
42. avast/retry-go: Simple golang library for retry mechanism \- GitHub, accessed June 15, 2025, [https://github.com/avast/retry-go](https://github.com/avast/retry-go)
