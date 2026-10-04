// quicprobe makes protocol-enforced, integrity-checked streaming downloads.
// It never falls back from HTTP/3 to HTTP/2 and does not buffer files in RAM.
package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	quicproxy "github.com/quic-go/quic-go/integrationtests/tools/proxy"
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
	label := flag.String("label", "", "label included in JSON output for A/B comparisons")
	insecure := flag.Bool("insecure", false, "accept self-signed certificates ONLY for isolated tests")
	expected := flag.Int64("bytes", -1, "expected response size, or -1")
	digest := flag.String("sha256", "", "expected SHA-256 digest")
	byteRange := flag.String("range", "", "optional Range header")
	repeat := flag.Int("repeat", 1, "downloads on the same connection")
	timeout := flag.Duration("timeout", 2*time.Minute, "timeout for each complete download")
	delay := flag.Duration("delay", 0, "test-only UDP delay in each direction (loopback targets only)")
	jitter := flag.Duration("jitter", 0, "uniform +/- UDP delay jitter, permitting reordering")
	loss := flag.Float64("loss", 0, "test-only UDP packet loss probability, e.g. 0.001")
	seed := flag.Uint64("seed", 1, "seed for reproducible impairment parameters")
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
		if *delay != 0 || *jitter != 0 || *loss != 0 {
			if *delay < 0 || *jitter < 0 || *jitter > *delay || *loss < 0 || *loss >= 1 {
				return fmt.Errorf("invalid impairment parameters")
			}
			u, err := url.Parse(*target)
			if err != nil {
				return err
			}
			ip := net.ParseIP(u.Hostname())
			if ip == nil || !ip.IsLoopback() {
				return fmt.Errorf("impairment is restricted to numeric loopback targets")
			}
			serverAddr, err := net.ResolveUDPAddr("udp", u.Host)
			if err != nil {
				return err
			}
			socket, err := net.ListenUDP("udp", &net.UDPAddr{IP: ip})
			if err != nil {
				return err
			}
			defer socket.Close()
			var mu sync.Mutex
			rng := rand.New(rand.NewPCG(*seed, *seed+1)) //nolint:gosec // reproducible test impairment, not cryptography
			proxy := &quicproxy.Proxy{
				Conn: socket, ServerAddr: serverAddr,
				DropPacket: func(quicproxy.Direction, net.Addr, net.Addr, []byte) bool {
					mu.Lock()
					defer mu.Unlock()
					return rng.Float64() < *loss
				},
				DelayPacket: func(quicproxy.Direction, net.Addr, net.Addr, []byte) time.Duration {
					mu.Lock()
					defer mu.Unlock()
					if *jitter == 0 {
						return *delay
					}
					return *delay - *jitter + time.Duration(rng.Int64N(int64(2**jitter)))
				},
			}
			if err := proxy.Start(); err != nil {
				socket.Close()
				return err
			}
			defer proxy.Close()
			tr.Dial = func(ctx context.Context, _ string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
				return quic.DialAddr(ctx, socket.LocalAddr().String(), tlsCfg, cfg)
			}
		}
		transport, closeTransport = tr, tr.Close
	case "h2":
		if *delay != 0 || *jitter != 0 || *loss != 0 {
			return fmt.Errorf("UDP impairment applies to HTTP/3 only; refusing an unshaped HTTP/2 comparison")
		}
		tr := &http.Transport{TLSClientConfig: tlsConfig, ForceAttemptHTTP2: true}
		transport = tr
		closeTransport = func() error { tr.CloseIdleConnections(); return nil }
	default:
		return fmt.Errorf("unsupported protocol %q", *protocol)
	}
	defer func() { _ = closeTransport() }()
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
			"label": *label, "iteration": n, "protocol": response.Proto, "status": response.StatusCode,
			"bytes": count, "sha256": actualDigest, "first_byte_ms": firstByte.Seconds() * 1000,
			"seconds": elapsed.Seconds(), "mib_per_second": float64(count) / (1 << 20) / elapsed.Seconds(),
			"content_range":       response.Header.Get("Content-Range"),
			"delay_per_direction": delay.String(), "jitter": jitter.String(), "loss": *loss, "seed": *seed,
		}); err != nil {
			return err
		}
	}
	return nil
}
