# Product preview

## Dashboard

![StePanel dashboard preview](assets/dashboard-preview.png)

The preview is rendered at 2× resolution for crisp display on GitHub and
retina screens. The editable [SVG source](assets/dashboard-preview.svg) remains
available for presentations and product materials.

## What the preview covers

<table>
<tr>
<td width="50%"><strong>Live operations</strong><br>Service inventory, health counts, load, uptime, and attention states come from the host rather than placeholder activity.</td>
<td width="50%"><strong>Safe migrations</strong><br>The migration center makes cpmove and WordPress restore workflows visible, reviewable, and asynchronous.</td>
</tr>
<tr>
<td><strong>Persistent jobs</strong><br>Restore, backup, certificate, and application work is represented by durable job state with status links.</td>
<td><strong>Security posture</strong><br>Operator checks, readiness, request correlation, and capability-aware controls are surfaced before changes are made.</td>
</tr>
</table>

This illustration mirrors the current dashboard structure using representative
values, including the selected webserver and direct operator-diagnostics link.
At runtime, all service counts, health states, load, security checks, and jobs
come from the current server; the application does not ship simulated activity.

For release reviews, capture a screenshot from the tagged build as a supplement
to this deterministic preview. Real screenshots are useful for verifying theme,
responsive layout, and capability-specific controls on a target host.
