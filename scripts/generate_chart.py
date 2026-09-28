#!/usr/bin/env python3
"""
Helm Chart Generator for the patch tracking prototype.

Generates standard Helm chart directory structures with values files
that capture both application state (image tags) and infrastructure
state (replicas, resource tier).

Usage:
    python3 generate_chart.py --service myapp --tag "4.1.1726912345" --output ./charts
    python3 generate_chart.py --service myapp --tag "4.1.1726990000" --replicas 3 --resource-tier large --output ./charts
"""

import argparse
import json
import os
import sys
from pathlib import Path

import yaml


# Resource tier presets — maps tier name to CPU/memory limits
RESOURCE_TIERS = {
    "small":  {"cpu": "250m",  "memory": "256Mi"},
    "medium": {"cpu": "500m",  "memory": "512Mi"},
    "large":  {"cpu": "1000m", "memory": "1Gi"},
}


def generate_chart_yaml(service_name: str) -> dict:
    return {
        "apiVersion": "v2",
        "name": service_name,
        "description": f"A Helm chart for {service_name}",
        "type": "application",
        "version": "0.1.0",
        "appVersion": "1.0.0",
    }


def generate_values(
    service_name: str,
    image_tag: str,
    registry: str = "registry.example.com",
    replica_count: int = 2,
    resource_tier: str = "medium",
) -> dict:
    tier = RESOURCE_TIERS.get(resource_tier, RESOURCE_TIERS["medium"])
    return {
        "image": {
            "repository": f"{registry}/{service_name}",
            "tag": image_tag,
            "pullPolicy": "IfNotPresent",
        },
        "replicaCount": replica_count,
        "resources": {
            "limits": {
                "cpu": tier["cpu"],
                "memory": tier["memory"],
            },
            "requests": {
                "cpu": tier["cpu"],
                "memory": tier["memory"],
            },
        },
    }


def generate_deployment_template() -> str:
    return '''apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "{{ .Chart.Name }}.fullname" . }}
  labels:
    {{- include "{{ .Chart.Name }}.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "{{ .Chart.Name }}.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "{{ .Chart.Name }}.selectorLabels" . | nindent 8 }}
    spec:
      containers:
        - name: {{ .Chart.Name }}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
'''


def generate_helpers_template() -> str:
    return '''{{/*
Expand the name of the chart.
*/}}
{{- define "{{ .Chart.Name }}.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "{{ .Chart.Name }}.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "{{ .Chart.Name }}.labels" -}}
helm.sh/chart: {{ include "{{ .Chart.Name }}.name" . }}-{{ .Chart.Version | replace "+" "_" }}
{{ include "{{ .Chart.Name }}.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "{{ .Chart.Name }}.selectorLabels" -}}
app.kubernetes.io/name: {{ include "{{ .Chart.Name }}.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
'''


def write_chart(service_name: str, output_dir: Path, values: dict, write_templates: bool = False):
    chart_dir = output_dir / service_name
    chart_dir.mkdir(parents=True, exist_ok=True)

    # Chart.yaml
    with open(chart_dir / "Chart.yaml", "w") as f:
        yaml.dump(generate_chart_yaml(service_name), f, default_flow_style=False)

    # values.yaml
    with open(chart_dir / "values.yaml", "w") as f:
        yaml.dump(values, f, default_flow_style=False)

    # Templates (only for new charts)
    if write_templates:
        templates_dir = chart_dir / "templates"
        templates_dir.mkdir(exist_ok=True)
        with open(templates_dir / "deployment.yaml", "w") as f:
            f.write(generate_deployment_template())
        with open(templates_dir / "_helpers.tpl", "w") as f:
            f.write(generate_helpers_template())

    return chart_dir


def main():
    parser = argparse.ArgumentParser(description="Generate Helm chart values for a service")
    parser.add_argument("--service", required=True, help="Service name")
    parser.add_argument("--tag", required=True, help="Image tag (e.g. 4.1.1726912345)")
    parser.add_argument("--registry", default="registry.example.com", help="Image registry")
    parser.add_argument("--replicas", type=int, default=2, help="Replica count")
    parser.add_argument("--resource-tier", default="medium", choices=RESOURCE_TIERS.keys(),
                        help="Resource tier (small/medium/large)")
    parser.add_argument("--output", default="./charts", help="Output directory")
    parser.add_argument("--write-templates", action="store_true",
                        help="Write Chart.yaml and templates/ (for new charts)")
    args = parser.parse_args()

    output_dir = Path(args.output)
    values = generate_values(
        service_name=args.service,
        image_tag=args.tag,
        registry=args.registry,
        replica_count=args.replicas,
        resource_tier=args.resource_tier,
    )

    chart_dir = write_chart(args.service, output_dir, values, args.write_templates)
    print(f"Generated: {chart_dir / 'values.yaml'}")

    # Also write as JSON for snapshot services.json consumption
    snapshot_path = chart_dir / "values.json"
    with open(snapshot_path, "w") as f:
        json.dump(values, f, indent=2)
    print(f"Generated: {snapshot_path}")


if __name__ == "__main__":
    main()
