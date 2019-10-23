package middlewares

import (
	"context"
	"net/http"
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	ginprometheus "github.com/zsais/go-gin-prometheus"
)

var (
	defaultMetricAddr = "localhost:8080"
	// defaultMetricPath      = "/metrics"
	defaultPProfPath       = "/pprof"
	defaultMetricGraceWait = 1 * time.Second
)

// MetricOption metric option argument
type MetricOption struct {
	addr, pprofPath string
	graceWait       time.Duration
}

// NewMetricOption create new default option
func NewMetricOption() *MetricOption {
	return &MetricOption{
		addr:      defaultMetricAddr,
		pprofPath: defaultPProfPath,
		graceWait: defaultMetricGraceWait,
	}
}

// MetricsOptFunc option of metrics
type MetricsOptFunc func(*MetricOption)

// WithMetricAddr set option addr
func WithMetricAddr(addr string) MetricsOptFunc {
	return func(opt *MetricOption) {
		opt.addr = addr
	}
}

// WithMetricGraceWait set wating time after graceful shutdown
func WithMetricGraceWait(wait time.Duration) MetricsOptFunc {
	return func(opt *MetricOption) {
		opt.graceWait = wait
	}
}

// WithPprofPath set option pprofPath
func WithPprofPath(path string) MetricsOptFunc {
	return func(opt *MetricOption) {
		opt.pprofPath = path
	}
}

// EnableMetric enable metrics for exsits gin server
func EnableMetric(srv *gin.Engine, options ...MetricsOptFunc) {
	opt := NewMetricOption()
	for _, optf := range options {
		optf(opt)
	}
	pprof.Register(srv, opt.pprofPath)
	BindPrometheus(srv)
}

// StartHTTPMetricSrv start new gin server with metrics api
func StartHTTPMetricSrv(ctx context.Context, options ...MetricsOptFunc) {
	opt := NewMetricOption()
	for _, optf := range options {
		optf(opt)
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	srv := &http.Server{
		Addr:    opt.addr,
		Handler: router,
	}

	go func() {
		<-ctx.Done()
		timingCtx, cancel := context.WithTimeout(context.Background(), opt.graceWait)
		defer cancel()
		if err := srv.Shutdown(timingCtx); err != nil {
			utils.Logger.Error("shutdown metrics server", zap.Error(err), zap.String("addr", opt.addr))
		}
	}()

	EnableMetric(router, options...)
	utils.Logger.Info("listening on http", zap.String("http-addr", opt.addr))
	utils.Logger.Info("server exit", zap.Error(srv.ListenAndServe()))
}

// BindPrometheus bind prometheus endpoint.
func BindPrometheus(s *gin.Engine) {
	p := ginprometheus.NewPrometheus("gin")
	p.Use(s)
}
