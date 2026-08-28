export type Severity = 'low' | 'medium' | 'high';

export interface TimeRange {
  from: string; // RFC3339
  to: string; // RFC3339
}

export interface SpikeWindow {
  namespace: string;
  from: string;
  to: string;
  errorCount: number;
  baseline: number;
  zScore: number;
}

export interface MatchedEvent {
  patternId: string;
  namespace: string;
  category: string;
  severity: Severity;
  ruleDescription?: string;
  count: number;
  firstSeen: string;
  lastSeen: string;
  sample: string;
  logLine?: string;
}

export interface Timeline {
  window: TimeRange;
  events: MatchedEvent[];
  warnings?: string[];
}

export interface RcaRequest {
  window: TimeRange;
  namespaces?: string[]; // defaults to correlation_order from rca-rules.yaml
  lokiDatasourceUid: string;
  nodeLabel?: string;
}

export interface DetectSpikesRequest {
  around?: TimeRange; // optional wider search window, defaults to last 6h
  lokiDatasourceUid: string;
}

export interface DetectSpikesResponse {
  spikes: SpikeWindow[];
  suggestedWindow?: TimeRange;
}

export interface RcaAnalyzeResponse {
  timeline: Timeline;
  // report text arrives via a separate streaming call (see api/backend.ts streamReport)
}
