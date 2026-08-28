package plugin

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

type timeRangeDTO struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (t timeRangeDTO) parse() (time.Time, time.Time, error) {
	from, err := time.Parse(time.RFC3339, t.From)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	to, err := time.Parse(time.RFC3339, t.To)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return from, to, nil
}

// ---- POST /rca/detect-spikes ----

type detectSpikesRequest struct {
	Around            *timeRangeDTO `json:"around"`
	LokiDatasourceUID string        `json:"lokiDatasourceUid"`
}

func (a *App) handleDetectSpikes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req detectSpikesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	to := time.Now()
	from := to.Add(-6 * time.Hour)
	if req.Around != nil {
		if f, t, err := req.Around.parse(); err == nil {
			from, to = f, t
		}
	}

	uid := req.LokiDatasourceUID
	if uid == "" {
		uid = a.settings.LokiDatasourceUID
	}

	spikes, err := a.loki.DetectSpikes(ctx, uid, a.settings.NamespaceLabel, a.settings.NodeLabel, a.rules.Namespaces(), from, to, a.rules.SpikeDetection)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}

	resp := map[string]interface{}{"spikes": spikes}
	if len(spikes) > 0 {
		// Suggest the single highest z-score spike, padded by a few minutes
		// on each side so we don't clip the leading/trailing signal.
		best := spikes[0]
		pad := 5 * time.Minute
		resp["suggestedWindow"] = timeRangeDTO{
			From: best.From.Add(-pad).Format(time.RFC3339),
			To:   best.To.Add(pad).Format(time.RFC3339),
		}
	}
	writeJSON(w, resp)
}

// ---- POST /rca/analyze ----

type analyzeRequest struct {
	Window            timeRangeDTO `json:"window"`
	Namespaces        []string     `json:"namespaces"`
	LokiDatasourceUID string       `json:"lokiDatasourceUid"`
	NodeLabel         string       `json:"nodeLabel"`
}

func (a *App) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req analyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	from, to, err := req.Window.parse()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	uid := req.LokiDatasourceUID
	if uid == "" {
		uid = a.settings.LokiDatasourceUID
	}

	namespaces := req.Namespaces
	if len(namespaces) == 0 {
		namespaces = a.rules.CorrelationOrder
	}

	nodeLabel := req.NodeLabel
	if nodeLabel == "" {
		nodeLabel = a.settings.NodeLabel
	}
	timeline, err := BuildTimeline(ctx, a.loki, a.rules, uid, a.settings.NamespaceLabel, nodeLabel, namespaces, from, to)
	if err != nil {
		log.DefaultLogger.Error("analyze failed", "error", err)
		writeErr(w, http.StatusBadGateway, err)
		return
	}

	writeJSON(w, map[string]interface{}{"timeline": timeline})
}

// ---- POST /rca/report/stream ----

type streamReportRequest struct {
	Timeline Timeline `json:"timeline"`
}

func (a *App) handleStreamReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req streamReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, errStreamingUnsupported)
		return
	}

	err := a.llm.StreamRCAReport(ctx, a.rules, req.Timeline, func(delta string) {
		chunk, _ := json.Marshal(map[string]string{"delta": delta})
		w.Write(chunk)
		w.Write([]byte("\n"))
		flusher.Flush()
	})
	if err != nil {
		chunk, _ := json.Marshal(map[string]string{"error": err.Error()})
		w.Write(chunk)
		w.Write([]byte("\n"))
		flusher.Flush()
		return
	}
	doneChunk, _ := json.Marshal(map[string]bool{"done": true})
	w.Write(doneChunk)
	w.Write([]byte("\n"))
	flusher.Flush()
}

type streamChatRequest struct {
	Event    MatchedEvent  `json:"event"`
	Messages []ChatMessage `json:"messages"`
}

func (a *App) handleStreamChat(w http.ResponseWriter, r *http.Request) {
	var req streamChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Event.Sample == "" || req.Event.Namespace == "" {
		writeErr(w, http.StatusBadRequest, errors.New("event namespace and log line are required"))
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, errStreamingUnsupported)
		return
	}
	err := a.llm.StreamEventChat(r.Context(), req.Event, req.Messages, func(delta string) {
		chunk, _ := json.Marshal(map[string]string{"delta": delta})
		w.Write(chunk)
		w.Write([]byte("\n"))
		flusher.Flush()
	})
	if err != nil {
		chunk, _ := json.Marshal(map[string]string{"error": err.Error()})
		w.Write(chunk)
		w.Write([]byte("\n"))
		flusher.Flush()
		return
	}
	doneChunk, _ := json.Marshal(map[string]bool{"done": true})
	w.Write(doneChunk)
	w.Write([]byte("\n"))
	flusher.Flush()
}

// ---- POST /rca/reload-rules ----

func (a *App) handleReloadRules(w http.ResponseWriter, r *http.Request) {
	rules, err := LoadRuleset(rulesFilePath())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	a.rules = rules
	writeJSON(w, map[string]bool{"ok": true})
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.DefaultLogger.Error("failed to write JSON response", "error", err)
	}
}

func writeErr(w http.ResponseWriter, status int, err error) {
	w.WriteHeader(status)
	writeJSON(w, map[string]string{"error": err.Error()})
}

var errStreamingUnsupported = errors.New("streaming not supported by response writer")
