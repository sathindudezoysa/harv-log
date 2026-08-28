package plugin

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
)

// App holds everything the resource handlers need: the loaded ruleset,
// a Loki client factory, and an LLM client. One App instance is created
// per Grafana plugin "app instance" (roughly: per org).
type App struct {
	backend.CallResourceHandler

	settings AppSettings
	rules    *Ruleset
	loki     *LokiClient
	llm      *LLMClient
}

type AppSettings struct {
	LokiDatasourceUID string `json:"lokiDatasourceUid"`
	NamespaceLabel    string `json:"namespaceLabel"`
	NodeLabel         string `json:"nodeLabel"`
}

// NewApp is called by app.Manage in main.go whenever Grafana needs a new
// instance (e.g. on settings change).
func NewApp(_ context.Context, settings backend.AppInstanceSettings) (instancemgmt.Instance, error) {
	var jsonData AppSettings
	if len(settings.JSONData) > 0 {
		if err := json.Unmarshal(settings.JSONData, &jsonData); err != nil {
			return nil, err
		}
	}
	if jsonData.NamespaceLabel == "" {
		jsonData.NamespaceLabel = "namespace"
	}
	// Keep legacy namespace-only behavior as the default. Node log matching is
	// opt-in so existing installations continue to work without being forced to
	// query a node label that may not exist in their Loki setup.

	rules, err := LoadRuleset(rulesFilePath())
	if err != nil {
		log.DefaultLogger.Error("failed to load rca-rules.yaml", "error", err)
		// Degrade gracefully with an empty ruleset rather than failing plugin startup.
		rules = &Ruleset{}
	}

	a := &App{
		settings: jsonData,
		rules:    rules,
		loki:     NewLokiClient(),
		llm:      NewLLMClient(),
	}

	mux := http.NewServeMux()
	a.registerRoutes(mux)
	a.CallResourceHandler = httpadapter.New(mux)

	return a, nil
}

func (a *App) Dispose() {}

func (a *App) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/rca/detect-spikes", a.handleDetectSpikes)
	mux.HandleFunc("/rca/analyze", a.handleAnalyze)
	mux.HandleFunc("/rca/report/stream", a.handleStreamReport)
	mux.HandleFunc("/rca/chat/stream", a.handleStreamChat)
	mux.HandleFunc("/rca/reload-rules", a.handleReloadRules)
}
