#!/usr/bin/env node
/**
 * CLI wrapper for Trivy→Copa converter.
 *
 * Usage:
 *   node --experimental-strip-types scripts/trivy-to-copa.ts <trivy-report.json> [options]
 *
 * Examples:
 *   # Convert Trivy report to Copa native format
 *   node --experimental-strip-types scripts/trivy-to-copa.ts report.json
 *
 *   # Save to file
 *   node --experimental-strip-types scripts/trivy-to-copa.ts report.json > manifest.json
 *
 *   # Only critical/high severity
 *   node --experimental-strip-types scripts/trivy-to-copa.ts report.json --severity CRITICAL,HIGH
 *
 *   # Show summary instead of full manifest
 *   node --experimental-strip-types scripts/trivy-to-copa.ts report.json --summary
 */

import { readFile } from "node:fs/promises";
import {
  trivyToCopa,
  formatManifest,
  formatConversionSummary,
  analyzeSeverities,
  type TrivyReport,
  type ConvertOptions,
} from "../web/src/lib/trivy-to-copa.ts";

// ─── Argument Parsing ────────────────────────────────────────────────────────

interface Args {
  report: string;
  severity: string[];
  includeUnfixed: boolean;
  excludeIds: string[];
  summary: boolean;
  json: boolean;
  help: boolean;
}

function parseArgs(): Args {
  const args: Args = {
    report: "",
    severity: [],
    includeUnfixed: false,
    excludeIds: [],
    summary: false,
    json: false,
    help: false,
  };

  const argv = process.argv.slice(2);
  for (let i = 0; i < argv.length; i++) {
    switch (argv[i]) {
      case "--severity":
      case "-s":
        const sev = argv[++i] || "";
        args.severity = sev.split(",").map((s) => s.trim().toUpperCase());
        break;
      case "--include-unfixed":
        args.includeUnfixed = true;
        break;
      case "--exclude":
        args.excludeIds = argv[++i]?.split(",").map((s) => s.trim()) || [];
        break;
      case "--summary":
        args.summary = true;
        break;
      case "--json":
        args.json = true;
        break;
      case "--help":
      case "-h":
        args.help = true;
        break;
      default:
        if (!args.report && !argv[i].startsWith("-")) {
          args.report = argv[i];
        }
    }
  }

  return args;
}

function printHelp(): void {
  console.log(`
Trivy→Copa Converter — convert Trivy JSON reports to Copa v1alpha1 native format.

Usage:
  node --experimental-strip-types scripts/trivy-to-copa.ts <trivy-report.json> [options]

Arguments:
  <trivy-report.json>   Path to Trivy JSON report

Options:
  -s, --severity <list>     Only include these severities (comma-separated)
                            Example: CRITICAL,HIGH
  --include-unfixed         Include vulnerabilities without fixes
  --exclude <ids>           Exclude specific vulnerability IDs (comma-separated)
  --summary                 Show conversion summary instead of full manifest
  --json                    Output as JSON (default: formatted text)
  -h, --help                Show this help

Examples:
  # Basic conversion
  trivy image --pkg-types os --ignore-unfixed -f json -o report.json nginx:1.27
  node --experimental-strip-types scripts/trivy-to-copa.ts report.json

  # Only critical/high
  node --experimental-strip-types scripts/trivy-to-copa.ts report.json -s CRITICAL,HIGH

  # Save to file for Copa
  node --experimental-strip-types scripts/trivy-to-copa.ts report.json > manifest.json
  copa patch --scanner native -r manifest.json -i nginx:1.27 -t nginx:1.27-patched

  # Show summary
  node --experimental-strip-types scripts/trivy-to-copa.ts report.json --summary
`);
}

// ─── Main ────────────────────────────────────────────────────────────────────

async function main(): Promise<void> {
  const args = parseArgs();

  if (args.help || !args.report) {
    printHelp();
    process.exit(args.help ? 0 : 2);
  }

  try {
    // Load Trivy report
    const raw = await readFile(args.report, "utf-8");
    const report = JSON.parse(raw) as TrivyReport;

    // Build conversion options
    const options: ConvertOptions = {
      severities: args.severity.length > 0 ? args.severity : undefined,
      includeUnfixed: args.includeUnfixed,
      excludeIds: args.excludeIds,
    };

    // Convert
    const manifest = trivyToCopa(report, options);

    // Output
    if (args.summary) {
      console.log(formatConversionSummary(report, manifest));
      console.log("");
      console.log("Severity breakdown:");
      const severities = analyzeSeverities(report);
      for (const [sev, count] of Object.entries(severities)) {
        if (count > 0) {
          console.log(`  ${sev}: ${count}`);
        }
      }
    } else if (args.json) {
      console.log(JSON.stringify(manifest, null, 2));
    } else {
      console.log(formatManifest(manifest));
    }

    // Exit code: 0 if updates found, 1 if empty
    process.exit(manifest.updates.length > 0 ? 0 : 1);
  } catch (error) {
    console.error("Error:", error instanceof Error ? error.message : error);
    process.exit(2);
  }
}

main();
