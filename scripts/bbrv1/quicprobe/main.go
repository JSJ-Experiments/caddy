// quicprobe makes protocol-enforced, integrity-checked streaming downloads.
// It never falls back from HTTP/3 to HTTP/2 and does not buffer files in RAM.
package main

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/quic-go/quic-go/http3"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	target := flag.String("url", "", "HTTPS download URL")
	protocol := flag.String("protocol", "h3", "h3 or h2; response protocol is enforced")
	insecure := flag.Bool("insecure", false, "accept self-signed certificates ONLY for isolated tests")
	expected := flag.Int64("bytes", -1, "expected response size, or -1")
	digest := flag.String("sha256", "", "expected SHA-256 digest")
	byteRange := flag.String("range", "", "optional Range header")
	repeat := flag.Int("repeat", 1, "downloads on the same connection")
	timeout := flag.Duration("timeout", 2*time.Minute, "timeout for each complete download")
	flag.Parse()
	if *target == "" || *repeat < 1 {
		return fmt.Errorf("url is required and repeat must be positive")
	}
	tlsConfig := &tls.Config{InsecureSkipVerify: *insecure, MinVersion: tls.VersionTLS13} //nolint:gosec // explicit test-only flag
	var transport http.RoundTripper
	var closeTransport func() error
	switch *protocol {
	case "h3":
		tr := &http3.Transport{TLSClientConfig: tlsConfig}
		transport, closeTransport = tr, tr.Close
	case "h2":
		tr := &http.Transport{TLSClientConfig: tlsConfig, ForceAttemptHTTP2: true}
		transport = tr
		closeTransport = func() error { tr.CloseIdleConnections(); return nil }
	default:
		return fmt.Errorf("unsupported protocol %q", *protocol)
	}
	defer closeTransport()
	client := &http.Client{
		Transport: transport,
		Timeout:   *timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	encoder := json.NewEncoder(os.Stdout)
	for n := 0; n < *repeat; n++ {
		request, err := http.NewRequest(http.MethodGet, *target, nil)
		if err != nil {
			return err
		}
		request.Header.Set("Accept-Encoding", "identity")
		if *byteRange != "" {
			request.Header.Set("Range", *byteRange)
		}
		start := time.Now()
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		firstByte := time.Since(start)
		wantProto := 3
		if *protocol == "h2" {
			wantProto = 2
		}
		wantStatus := http.StatusOK
		if *byteRange != "" {
			wantStatus = http.StatusPartialContent
		}
		if response.ProtoMajor != wantProto || response.StatusCode != wantStatus {
			response.Body.Close()
			return fmt.Errorf("got %s status %d; expected HTTP/%d status %d", response.Proto, response.StatusCode, wantProto, wantStatus)
		}
		hash := sha256.New()
		count, err := io.Copy(hash, response.Body)
		closeErr := response.Body.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		elapsed := time.Since(start)
		actualDigest := hex.EncodeToString(hash.Sum(nil))
		if *expected >= 0 && count != *expected {
			return fmt.Errorf("download size %d; expected %d", count, *expected)
		}
		if *digest != "" && actualDigest != *digest {
			return fmt.Errorf("SHA-256 mismatch: got %s", actualDigest)
		}
		if err := encoder.Encode(map[string]any{
			"iteration": n, "protocol": response.Proto, "status": response.StatusCode,
			"bytes": count, "sha256": actualDigest, "first_byte_ms": firstByte.Seconds() * 1000,
			"seconds": elapsed.Seconds(), "mib_per_second": float64(count) / (1 << 20) / elapsed.Seconds(),
			"content_range": response.Header.Get("Content-Range"),
		}); err != nil {
			return err
		}
	}
	return nil
}
