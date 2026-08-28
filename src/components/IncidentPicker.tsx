import React, { useState } from 'react';
import { Button, Stack, Text, TimeRangePicker } from '@grafana/ui';
import { dateTime, TimeRange as GrafanaTimeRange } from '@grafana/data';
import { detectSpikes } from '../api/backend';
import type { SpikeWindow, TimeRange } from '../types/rca';

interface Props {
  lokiDatasourceUid: string;
  onConfirm: (range: TimeRange) => void;
}

export function IncidentPicker({ lokiDatasourceUid, onConfirm }: Props) {
  const [range, setRange] = useState<GrafanaTimeRange>(() => {
    const to = dateTime();
    const from = dateTime().subtract(6, 'h');
    return { from, to, raw: { from: 'now-6h', to: 'now' } };
  });
  const [spikes, setSpikes] = useState<SpikeWindow[]>([]);
  const [detecting, setDetecting] = useState(false);

  const onDetect = async () => {
    setDetecting(true);
    try {
      const resp = await detectSpikes({
        lokiDatasourceUid,
        around: { from: range.from.toISOString(), to: range.to.toISOString() },
      });
      setSpikes(resp.spikes ?? []);
      if (resp.suggestedWindow) {
        onConfirm(resp.suggestedWindow);
      }
    } finally {
      setDetecting(false);
    }
  };

  return (
    <Stack direction="column" gap={2}>
      <Stack direction="row" gap={2} alignItems="center" justifyContent="flex-end">
        <TimeRangePicker
          value={range}
          onChange={setRange}
          onChangeTimeZone={() => {}}
          onMoveBackward={() => {}}
          onMoveForward={() => {}}
          onZoom={() => {}}
        />
        <Button onClick={onDetect} disabled={detecting} icon={detecting ? 'fa fa-spinner' : 'search'}>
          {detecting ? 'Scanning for error spikes...' : 'Auto-detect incident window'}
        </Button>
        <Button
          variant="secondary"
          onClick={() => onConfirm({ from: range.from.toISOString(), to: range.to.toISOString() })}
        >
          Use selected range
        </Button>
      </Stack>

      {spikes.length > 0 && (
        <Stack direction="column" gap={1}>
          <Text variant="bodySmall" color="secondary">
            Detected error spikes (highest first):
          </Text>
          {spikes.map((s) => (
            <Stack key={`${s.namespace}-${s.from}`} direction="row" gap={2} alignItems="center">
              <Text weight="medium">{s.namespace}</Text>
              <Text variant="bodySmall">
                {s.from} → {s.to} · {s.errorCount} errors (baseline {s.baseline}, z={s.zScore.toFixed(1)})
              </Text>
              <Button size="sm" variant="secondary" onClick={() => onConfirm({ from: s.from, to: s.to })}>
                Use this window
              </Button>
            </Stack>
          ))}
        </Stack>
      )}
    </Stack>
  );
}
