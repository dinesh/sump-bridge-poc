package bridge

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

const (
	defaultInterval       = 15 * time.Second
	millisecondsPerSecond = 1000
)

type HandlerErrorHandling int

const (
	ContinueOnError HandlerErrorHandling = iota

	AbortOnError
)

type Config struct {
	URL               string
	Interval, Timeout time.Duration

	Gatherer      prometheus.Gatherer
	ErrorHandling HandlerErrorHandling
	Logger        Logger
}

type Bridge struct {
	interval      time.Duration
	logger        Logger
	errorHandling HandlerErrorHandling
	g             prometheus.Gatherer
	smClient      *sumoAPIClient
}

func NewBridge(c *Config) (*Bridge, error) {
	var z time.Duration
	if c.Timeout == z {
		c.Timeout = defaultInterval
	}

	sm, err := newSumoClient(c.URL, c.Timeout)
	if err != nil {
		return nil, err
	}

	b := &Bridge{
		smClient: sm,
		interval: c.Interval,
	}

	if c.Gatherer == nil {
		b.g = prometheus.DefaultGatherer
	} else {
		b.g = c.Gatherer
	}

	if c.Interval == z {
		b.interval = defaultInterval
	} else {
		b.interval = c.Interval
	}

	b.errorHandling = c.ErrorHandling
	b.logger = c.Logger

	return b, nil
}

func (b *Bridge) Run(ctx context.Context) {
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := b.Push(); err != nil && b.logger != nil {
				b.logger.Println("error pushing to Sumo:", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (b *Bridge) Push() error {
	mfs, err := b.g.Gather()
	if err != nil || len(mfs) == 0 {
		switch b.errorHandling {
		case AbortOnError:
			return err
		case ContinueOnError:
			if b.logger != nil {
				b.logger.Println("continue on error:", err)
			}
		default:
			panic("unrecognized error handling value")
		}
	}

	return b.writeMetrics(mfs, model.Now())
}

func (b *Bridge) writeMetrics(mfs []*dto.MetricFamily, now model.Time) error {
	b.logger.Println("collected metrics: ", len(mfs))

	var buf bytes.Buffer
	enc := expfmt.NewEncoder(&buf, expfmt.FmtText)
	for _, mf := range mfs {
		if err := enc.Encode(mf); err != nil {
			b.logger.Println("[WARN] failed to encode: %v\n", mf)
		}
	}

	if closer, ok := enc.(expfmt.Closer); ok {
		closer.Close()
	}

	b.logger.Println("metrics payload:")
	b.logger.Println(buf.String())

	return b.smClient.submit(&buf)
}

type Logger interface {
	Println(v ...interface{})
}

func newSumoClient(url string, timeout time.Duration) (*sumoAPIClient, error) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: timeout,
	}

	return &sumoAPIClient{
		url: url,
		hc:  client,
	}, nil
}

type sumoAPIClient struct {
	url string
	hc  *http.Client

	category, sourceName, sourceHost string
	sourceClient                     string
}

func (sc *sumoAPIClient) submit(payload io.Reader) error {
	req, err := http.NewRequest("POST", sc.url, payload)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/vnd.sumologic.prometheus")

	if len(sc.category) > 0 {
		req.Header.Set("X-Sumo-Category", sc.category)
	}

	if len(sc.sourceName) > 0 {
		req.Header.Set("X-Sumo-Name", sc.sourceName)
	}
	if len(sc.sourceHost) > 0 {
		req.Header.Set("X-Sumo-Host", sc.sourceHost)
	}

	if len(sc.sourceClient) > 0 {
		req.Header.Set("X-Sumo-Client", sc.sourceClient)
	}

	resp, err := sc.hc.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	// fmt.Printf("Response Code: %v\n", resp.StatusCode)

	if resp.StatusCode >= http.StatusMultipleChoices || resp.StatusCode < http.StatusOK {
		return fmt.Errorf("non-2xx response code: %v", resp.StatusCode)
	}

	return nil
}
