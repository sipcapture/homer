# UI Parity Matrix (`homer-ui` -> `homer-core/src/ui`)

## Settings pages

| Legacy page | New UI status | Notes |
| --- | --- | --- |
| profile | missing | Add dedicated profile/settings page |
| about | done | `AboutPanel.jsx` |
| users | done | `UsersPanel.jsx` |
| user settings | done | `UserSettingsPanel.jsx` |
| alias | done | `AliasesPanel.jsx` |
| ip alias | missing | Add separate panel if backend exposes separate API |
| advanced | missing | Add panel (advanced/system key-value config) |
| mapping | done | `MappingsPanel.jsx` |
| hepsub | done | `HepsubsPanel.jsx` |
| auth token | done | `AuthTokensPanel.jsx` |
| scripts | missing | Add scripts CRUD panel |
| agentsub | done | `AgentSubsPanel.jsx` |
| system overview | partial | `SystemPanel.jsx` (needs parity fields/actions) |
| reset | missing | Add reset operations page |
| api documentation | missing | Add API docs page |

## Dashboard widgets

| Legacy widget/type | New UI status | Notes |
| --- | --- | --- |
| protosearch | done | Legacy `type`; same as `search` (hidden from picker) |
| smart-input | missing | Add query builder / smart query widget |
| result | done | `ResultsPanel.jsx` |
| result-chart | partial | `ChartPanel.jsx` basic aggregate only |
| clickhousechart | missing | Add dedicated chart widget type |
| influxdbchart | missing | Add dedicated chart widget type |
| grafana-widget | partial | only iframe-style widget currently |
| general-iframe | done | `IFramePanel.jsx` |
| alert | done | `AlertPanel.jsx` |
| pcap-uploader | done | `PcapUploaderPanel.jsx` |
| ace-editor | done | `CodeEditorPanel.jsx` |
| clock | done | `ClockPanel.jsx` |
| note | done | `NotePanel.jsx` |

## Transaction details

| Legacy tab | New UI status | Notes |
| --- | --- | --- |
| messages | done | present |
| flow | done | present |
| qos | done | RTCP, RTP, VQRTCP tabs |
| logs | done | present |
| callinfo | missing | add tab + endpoint integration |
| events | missing | add tab + endpoint integration |
| sub | n/a | not exposed in transaction modal UI |
| hepsub | done | transaction modal tab |

## Access control

| Area | New UI status | Notes |
| --- | --- | --- |
| Sidebar visibility by role | missing | add role-aware filtering |
| Action permissions (add/edit/delete/reset/import/export) | missing | bind to role matrix |
| External user restrictions | missing | apply legacy external profile |

