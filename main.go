// Command tempest-exporter listens for WeatherFlow Tempest UDP broadcasts on
// the local network and exposes them as Prometheus metrics.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sys/unix"
)

func main() {
	listenAddr := flag.String("listen-address", envOr("TEMPEST_LISTEN_ADDRESS", ":7070"), "HTTP address for /metrics (env TEMPEST_LISTEN_ADDRESS)")
	udpAddr := flag.String("udp-address", envOr("TEMPEST_UDP_ADDRESS", ":50222"), "UDP address to receive hub broadcasts on (env TEMPEST_UDP_ADDRESS)")
	staleDefault, err := time.ParseDuration(envOr("TEMPEST_STALE_AFTER", "5m"))
	if err != nil {
		slog.Error("invalid TEMPEST_STALE_AFTER", "err", err)
		os.Exit(2)
	}
	staleAfter := flag.Duration("stale-after", staleDefault, "drop a device's metrics after this long without packets (env TEMPEST_STALE_AFTER)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	collector := NewCollector(*staleAfter)
	prometheus.MustRegister(collector)

	conn, err := listenUDP(ctx, *udpAddr)
	if err != nil {
		slog.Error("listen udp", "addr", *udpAddr, "err", err)
		os.Exit(1)
	}
	go receive(conn, collector)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`<html><body><h1>Tempest exporter</h1><a href="/metrics">Metrics</a></body></html>`))
	})
	srv := &http.Server{Addr: *listenAddr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	go func() {
		<-ctx.Done()
		conn.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	slog.Info("listening", "http", *listenAddr, "udp", *udpAddr, "stale_after", *staleAfter)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("http server", "err", err)
		os.Exit(1)
	}
}

// listenUDP binds with SO_REUSEADDR and SO_REUSEPORT so other consumers of the
// hub broadcast (Home Assistant, WeatherFlow2MQTT) can listen on the same port.
func listenUDP(ctx context.Context, addr string) (net.PacketConn, error) {
	lc := net.ListenConfig{Control: func(_, _ string, rc syscall.RawConn) error {
		var sockErr error
		err := rc.Control(func(fd uintptr) {
			sockErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
			if sockErr == nil {
				sockErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			}
		})
		if err != nil {
			return err
		}
		return sockErr
	}}
	return lc.ListenPacket(ctx, "udp4", addr)
}

func receive(conn net.PacketConn, c *Collector) {
	buf := make([]byte, 64*1024)
	for {
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			slog.Error("read udp", "err", err)
			continue
		}
		if err := c.Handle(buf[:n]); err != nil {
			slog.Warn("bad packet", "from", from, "err", err)
		}
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
