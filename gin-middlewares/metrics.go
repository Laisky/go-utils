package middlewares

import (
	"context"
	"net/http"
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
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
type MetricsOptFunc func(*MetricOption) error

// WithMetricAddr set option addr
func WithMetricAddr(addr string) MetricsOptFunc {
	return func(opt *MetricOption) error {
		opt.addr = addr
		return nil
	}
}

// WithMetricGraceWait set wating time after graceful shutdown
func WithMetricGraceWait(wait time.Duration) MetricsOptFunc {
	return func(opt *MetricOption) error {
		opt.graceWait = wait
		return nil
	}
}

// WithPprofPath set option pprofPath
func WithPprofPath(path string) MetricsOptFunc {
	return func(opt *MetricOption) error {
		opt.pprofPath = path
		return nil
	}
}

// EnableMetric enable metrics for exsits gin server
func EnableMetric(srv *gin.Engine, options ...MetricsOptFunc) (err error) {
	opt := NewMetricOption()
	for _, optf := range options {
		if err = optf(opt); err != nil {
			return errors.Wrap(err, "set option")
		}
	}

	pprof.Register(srv, opt.pprofPath)
	BindPrometheus(srv)
	return nil
}

// GetHTTPMetricSrv start new gin server with metrics api
func GetHTTPMetricSrv(ctx context.Context, options ...MetricsOptFunc) (srv *http.Server, err error) {
	opt := NewMetricOption()
	for _, optf := range options {
		if err = optf(opt); err != nil {
			return nil, errors.Wrap(err, "set option")
		}
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	srv = &http.Server{
		Addr:    opt.addr,
		Handler: router,
	}

	go func() {
		<-ctx.Done()
		utils.Logger.Info("got signal to shutdown metric server")
		timingCtx, cancel := context.WithTimeout(context.Background(), opt.graceWait)
		defer cancel()
		if err := srv.Shutdown(timingCtx); err != nil {
			utils.Logger.Error("shutdown metrics server", zap.Error(err), zap.String("addr", opt.addr))
		}
	}()

	if err = EnableMetric(router, options...); err != nil {
		return nil, errors.Wrap(err, "enable metric")
	}

	return
}

// BindPrometheus bind prometheus endpoint.
func BindPrometheus(s *gin.Engine) {
	p := ginprometheus.NewPrometheus("gin")
	p.Use(s)
}
