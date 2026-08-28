package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// LokiClient talks to Loki through Grafana's own datasource proxy
// (/api/datasources/proxy/uid/<uid>/loki/...) so it inherits Grafana's
// existing auth/TLS/routing to Loki rather than needing its own Loki
// credentials. It forwards the caller's auth headers (cookie/Authorization)
// from the original resource request.
type LokiClient struct {
	grafanaBaseURL string
	httpClient     *http.Client
}

func NewLokiClient() *LokiClient {
	base := os.Getenv("GF_APP_URL")
	if base == "" {
		base = "http://localhost:3000"
	}
	return &LokiClient{
		grafanaBaseURL: base,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
	}
}

type SpikeWindow struct {
	Namespace  string    `json:"namespace"`
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
	ErrorCount int       `json:"errorCount"`
	Baseline   float64   `json:"baseline"`
	ZScore     float64   `json:"zScore"`
}

// DetectSpikes runs a count_over_time query per namespace across [from,to],
// computes a rolling baseline (mean/stddev over BaselineLookbackMinutes) and
// returns buckets whose error count exceeds ZScoreThreshold standard
// deviations above baseline, sorted by z-score descending.
func (c *LokiClient) DetectSpikes(
	ctx context.Context,
	datasourceUID, namespaceLabel, nodeLabel string,
	namespaces []NamespaceRules,
	from, to time.Time,
	cfg SpikeDetectionConfig,
) ([]SpikeWindow, error) {
	bucket := time.Duration(cfg.BucketMinutes) * time.Minute
	if bucket <= 0 {
		bucket = time.Minute
	}

	spikes := make([]SpikeWindow, 0)
	for _, ns := range namespaces {
		query := fmt.Sprintf(
			`sum(count_over_time({%s="%s"} |~ "(?i)(error|failed|fatal)" [%s]))`,
			namespaceLabel, ns.Name, bucket.String(),
		)
		series, err := c.queryRange(ctx, datasourceUID, query, from, to, bucket)
		if err != nil {
			log.DefaultLogger.Warn("spike detection query failed", "namespace", ns.Name, "error", err)
			continue
		}

		mean, stddev := meanStddev(series)
		for _, pt := range series {
			if stddev == 0 {
				continue
			}
			z := (pt.value - mean) / stddev
			if z >= cfg.ZScoreThreshold {
				spikes = append(spikes, SpikeWindow{
					Namespace:  ns.Name,
					From:       pt.ts,
					To:         pt.ts.Add(bucket),
					ErrorCount: int(pt.value),
					Baseline:   mean,
					ZScore:     z,
				})
			}
		}
	}
	if nodeLabel != "" {
		query := fmt.Sprintf(`sum(count_over_time({%s=~".+"} |~ "(?i)(error|failed|fatal)" [%s]))`, nodeLabel, bucket.String())
		series, err := c.queryRange(ctx, datasourceUID, query, from, to, bucket)
		if err != nil {
			log.DefaultLogger.Warn("spike detection query failed", "nodeLabel", nodeLabel, "error", err)
		} else {
			mean, stddev := meanStddev(series)
			for _, pt := range series {
				if stddev == 0 {
					continue
				}
				z := (pt.value - mean) / stddev
				if z >= cfg.ZScoreThreshold {
					spikes = append(spikes, SpikeWindow{
						Namespace:  "node",
						From:       pt.ts,
						To:         pt.ts.Add(bucket),
						ErrorCount: int(pt.value),
						Baseline:   mean,
						ZScore:     z,
					})
				}
			}
		}
	}

	// Highest z-score first.
	for i := 1; i < len(spikes); i++ {
		for j := i; j > 0 && spikes[j].ZScore > spikes[j-1].ZScore; j-- {
			spikes[j], spikes[j-1] = spikes[j-1], spikes[j]
		}
	}
	return spikes, nil
}

type LogLine struct {
	Timestamp time.Time
	Line      string
}

// FetchLogs pulls raw log lines for one namespace within [from,to]. Callers
// (timeline.go) are expected to match/collapse these against the ruleset
// rather than forwarding raw lines further downstream, to keep volume and
// LLM token usage bounded.
func (c *LokiClient) FetchLogs(ctx context.Context, datasourceUID, namespaceLabel, namespace string, from, to time.Time) ([]LogLine, error) {
	query := fmt.Sprintf(`{%s="%s"}`, namespaceLabel, namespace)
	return c.queryLogRange(ctx, datasourceUID, query, from, to)
}

func (c *LokiClient) FetchNodeLogs(ctx context.Context, datasourceUID, nodeLabel string, from, to time.Time) ([]LogLine, error) {
	if nodeLabel == "" {
		return nil, nil
	}
	query := fmt.Sprintf(`{%s=~".+"}`, nodeLabel)
	return c.queryLogRange(ctx, datasourceUID, query, from, to)
}

// ---- internals ----

type point struct {
	ts    time.Time
	value float64
}

func meanStddev(points []point) (float64, float64) {
	if len(points) == 0 {
		return 0, 0
	}
	var sum float64
	for _, p := range points {
		sum += p.value
	}
	mean := sum / float64(len(points))

	var variance float64
	for _, p := range points {
		variance += (p.value - mean) * (p.value - mean)
	}
	variance /= float64(len(points))
	return mean, math.Sqrt(variance)
}

// queryRange calls Loki's /loki/api/v1/query_range with a metric query and
// returns the decoded matrix as a flat list of points.
func (c *LokiClient) queryRange(ctx context.Context, datasourceUID, query string, from, to time.Time, step time.Duration) ([]point, error) {
	u := fmt.Sprintf(
		"%s/api/datasources/proxy/uid/%s/loki/api/v1/query_range?%s",
		c.grafanaBaseURL, url.PathEscape(datasourceUID),
		url.Values{
			"query": {query},
			"start": {strconv.FormatInt(from.UnixNano(), 10)},
			"end":   {strconv.FormatInt(to.UnixNano(), 10)},
			"step":  {step.String()},
		}.Encode(),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	// TODO: forward the caller's Authorization/Cookie headers here so this
	// request is executed with the requesting user's Grafana permissions.
	// See resources.go handlers for where to thread the incoming
	// *http.Request through, and httpclient.go for a service-account-based
	// alternative if you'd rather run queries with a fixed identity.

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("loki query_range returned %d", resp.StatusCode)
	}

	var parsed lokiMatrixResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	var points []point
	for _, result := range parsed.Data.Result {
		for _, sample := range result.Values {
			if len(sample) != 2 {
				continue
			}
			tsFloat, ok := sample[0].(float64)
			if !ok {
				continue
			}
			valStr, ok := sample[1].(string)
			if !ok {
				continue
			}
			val, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				continue
			}
			points = append(points, point{ts: time.Unix(int64(tsFloat), 0), value: val})
		}
	}
	return points, nil
}

// queryLogRange calls Loki's /loki/api/v1/query_range in "streams" mode
// (log query, not metric query) and returns raw lines.
func (c *LokiClient) queryLogRange(ctx context.Context, datasourceUID, query string, from, to time.Time) ([]LogLine, error) {
	u := fmt.Sprintf(
		"%s/api/datasources/proxy/uid/%s/loki/api/v1/query_range?%s",
		c.grafanaBaseURL, url.PathEscape(datasourceUID),
		url.Values{
			"query":     {query},
			"start":     {strconv.FormatInt(from.UnixNano(), 10)},
			"end":       {strconv.FormatInt(to.UnixNano(), 10)},
			"limit":     {"5000"},
			"direction": {"forward"},
		}.Encode(),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("loki query_range (logs) returned %d", resp.StatusCode)
	}

	var parsed lokiStreamsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	var lines []LogLine
	for _, stream := range parsed.Data.Result {
		for _, entry := range stream.Values {
			if len(entry) != 2 {
				continue
			}
			nanos, err := strconv.ParseInt(entry[0], 10, 64)
			if err != nil {
				continue
			}
			lines = append(lines, LogLine{Timestamp: time.Unix(0, nanos), Line: entry[1]})
		}
	}
	return lines, nil
}

type lokiMatrixResponse struct {
	Data struct {
		Result []struct {
			Values [][2]interface{} `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

type lokiStreamsResponse struct {
	Data struct {
		Result []struct {
			Values [][2]string `json:"values"`
		} `json:"result"`
	} `json:"data"`
}
