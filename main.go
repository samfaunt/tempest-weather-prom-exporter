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
	"strconv"
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
	var elevation *float64
	setElevation := func(s string) error {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		elevation = &v
		return nil
	}
	if v := os.Getenv("TEMPEST_ELEVATION"); v != "" {
		if err := setElevation(v); err != nil {
			slog.Error("invalid TEMPEST_ELEVATION", "err", err)
			os.Exit(2)
		}
	}
	flag.Func("elevation", "station elevation in meters; enables sea-level pressure (env TEMPEST_ELEVATION)", setElevation)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	collector := NewCollector(*staleAfter, elevation)
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
		_, _ = w.Write([]byte(`<html><body><h1>Tempest exporter</h1><a href="/metrics">Metrics</a></body></html>`))
	})
	srv := &http.Server{Addr: *listenAddr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	go func() {
		<-ctx.Done()
		_ = conn.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("http shutdown", "err", err)
		}
	}()

	slog.Info("listening", "http", *listenAddr, "udp", *udpAddr, "stale_after", *staleAfter)
	if elevation != nil {
		slog.Info("sea-level pressure enabled", "elevation_m", *elevation)
	}
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
			s := int(fd) // #nosec G115 -- file descriptors always fit in int
			sockErr = unix.SetsockoptInt(s, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
			if sockErr == nil {
				sockErr = unix.SetsockoptInt(s, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
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
