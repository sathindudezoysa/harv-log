package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"time"
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

type LogLine struct {
	Timestamp time.Time
	Line      string
	Labels    map[string]string
}

// FetchLogs pulls raw log lines for one namespace within [from,to]. Callers
// (timeline.go) are expected to match/collapse these against the ruleset
// rather than forwarding raw lines further downstream, to keep volume and
// LLM token usage bounded.
func (c *LokiClient) FetchLogs(ctx context.Context, datasourceUID, namespaceLabel, namespace string, from, to time.Time) ([]LogLine, error) {
	query, err := buildLabelQuery(namespaceLabel, namespace)
	if err != nil {
		return nil, err
	}
	return c.queryLogRange(ctx, datasourceUID, query, from, to)
}

func (c *LokiClient) FetchNodeLogs(ctx context.Context, datasourceUID, nodeLabel string, from, to time.Time) ([]LogLine, error) {
	if nodeLabel == "" {
		return nil, nil
	}
	if !labelNamePattern.MatchString(nodeLabel) {
		return nil, fmt.Errorf("invalid node label %q", nodeLabel)
	}
	query := fmt.Sprintf(`{%s=~".+"}`, nodeLabel)
	return c.queryLogRange(ctx, datasourceUID, query, from, to)
}

var labelNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func buildLabelQuery(label, value string) (string, error) {
	if !labelNamePattern.MatchString(label) {
		return "", fmt.Errorf("invalid namespace label %q", label)
	}
	return fmt.Sprintf("{%s=%s}", label, strconv.Quote(value)), nil
}

// ---- internals ----

type point struct {
	ts    time.Time
	value float64
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
			lines = append(lines, LogLine{
				Timestamp: time.Unix(0, nanos),
				Line:      entry[1],
				Labels:    stream.Stream,
			})
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
			Stream map[string]string `json:"stream"`
			Values [][2]string       `json:"values"`
		} `json:"result"`
	} `json:"data"`
}
