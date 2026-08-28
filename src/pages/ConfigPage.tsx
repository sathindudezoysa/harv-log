import React, { useState } from 'react';
import { Field, Input, Button, FieldSet } from '@grafana/ui';
import type { AppPluginMeta, PluginConfigPageProps, KeyValue } from '@grafana/data';
import { getBackendSrv } from '@grafana/runtime';
import { lastValueFrom } from 'rxjs';

interface JsonData {
  lokiDatasourceUid?: string;
  namespaceLabel?: string;
  nodeLabel?: string;
}

type Props = PluginConfigPageProps<AppPluginMeta<JsonData>>;

export function ConfigPage({ plugin }: Props) {
  const [lokiUid, setLokiUid] = useState(plugin.meta.jsonData?.lokiDatasourceUid ?? '');
  const [namespaceLabel, setNamespaceLabel] = useState(plugin.meta.jsonData?.namespaceLabel ?? 'namespace');
  const [nodeLabel, setNodeLabel] = useState(plugin.meta.jsonData?.nodeLabel ?? '');
  const [saving, setSaving] = useState(false);

  const onSave = async () => {
    setSaving(true);
    try {
      await lastValueFrom(
        getBackendSrv().fetch({
          url: `/api/plugins/${plugin.meta.id}/settings`,
          method: 'POST',
          data: {
            enabled: true,
            pinned: true,
            jsonData: {
              lokiDatasourceUid: lokiUid,
              namespaceLabel,
              nodeLabel,
            } as JsonData & KeyValue,
          },
        })
      );
      window.location.reload();
    } finally {
      setSaving(false);
    }
  };

  return (
    <FieldSet label="Harvester RCA settings">
      <Field label="Loki datasource UID" description="The Loki datasource that has your Harvester support-bundle logs">
        <Input value={lokiUid} onChange={(e) => setLokiUid(e.currentTarget.value)} width={60} />
      </Field>
      <Field
        label="Namespace label"
        description="Label name used to select namespace in Loki queries, e.g. 'namespace'"
      >
        <Input value={namespaceLabel} onChange={(e) => setNamespaceLabel(e.currentTarget.value)} width={60} />
      </Field>
      <Field
        label="Node label (optional)"
        description="Optional label used to select node-level logs in Loki, e.g. 'node'. Leave blank to keep the legacy namespace-only behavior. When set, node logs are matched alongside namespace logs."
      >
        <Input value={nodeLabel} onChange={(e) => setNodeLabel(e.currentTarget.value)} width={60} />
      </Field>
      <Button onClick={onSave} disabled={saving}>
        {saving ? 'Saving...' : 'Save settings'}
      </Button>
    </FieldSet>
  );
}
