/**
 * Trivy→Copa Converter
 *
 * Converts Trivy JSON vulnerability reports to Copa v1alpha1 native format.
 * This replicates the internal conversion Copa does when parsing Trivy reports,
 * but exposes it as a standalone tool for previewing/inspecting what Copa would patch.
 */

import { readFile } from "node:fs/promises";
import type { CopaManifest, CopaUpdatePackage } from "./sbom-checker";

// ─── Trivy Report Types ──────────────────────────────────────────────────────

/** Trivy vulnerability entry (subset we care about) */
export interface TrivyVulnerability {
  VulnerabilityID: string;
  PkgName: string;
  PkgID?: string;
  InstalledVersion: string;
  FixedVersion?: string;
  Severity: string;
  Status?: string;
  PrimaryURL?: string;
  Title?: string;
  Description?: string;
}

/** Trivy scan result (per target) */
export interface TrivyResult {
  Target: string;
  Class?: string;
  Type?: string;
  Vulnerabilities?: TrivyVulnerability[];
}

/** Trivy image metadata */
export interface TrivyMetadata {
  OS?: {
    Family: string;
    Name: string;
  };
  ImageID?: string;
  RepoTags?: string[];
}

/** Trivy JSON report */
export interface TrivyReport {
  SchemaVersion?: number;
  ArtifactName?: string;
  ArtifactType?: string;
  Metadata?: TrivyMetadata;
  Results?: TrivyResult[];
}

// ─── Conversion Options ──────────────────────────────────────────────────────

export interface ConvertOptions {
  /** Only include vulnerabilities with these severities (default: all with fixes) */
  severities?: string[];
  /** Include vulnerabilities without fixes (default: false) */
  includeUnfixed?: boolean;
  /** Exclude specific vulnerability IDs */
  excludeIds?: string[];
  /** Custom vulnerability ID prefix for non-CVE IDs */
  idPrefix?: string;
}

// ─── Conversion Logic ────────────────────────────────────────────────────────

/**
 * Convert a Trivy JSON report to Copa v1alpha1 manifest format.
 */
export function trivyToCopa(
  report: TrivyReport,
  options: ConvertOptions = {},
): CopaManifest {
  const { severities, includeUnfixed = false, excludeIds = [] } = options;

  // Extract OS info from metadata
  const osType = report.Metadata?.OS?.Family || "unknown";
  const osVersion = report.Metadata?.OS?.Name || "unknown";

  // Collect fixable vulnerabilities from all results
  const updates: CopaUpdatePackage[] = [];
  const seen = new Set<string>();

  for (const result of report.Results || []) {
    // Only process OS package results (skip language packages for now)
    if (result.Class && result.Class !== "os-pkgs") continue;

    for (const vuln of result.Vulnerabilities || []) {
      // Skip if no fix available and we're not including unfixed
      if (!vuln.FixedVersion && !includeUnfixed) continue;

      // Skip if severity filter doesn't match
      if (severities && !severities.includes(vuln.Severity)) continue;

      // Skip if in exclude list
      if (excludeIds.includes(vuln.VulnerabilityID)) continue;

      // Deduplicate by package name + vulnerability ID
      const key = `${vuln.PkgName}:${vuln.VulnerabilityID}`;
      if (seen.has(key)) continue;
      seen.add(key);

      updates.push({
        name: vuln.PkgName,
        installedVersion: vuln.InstalledVersion,
        fixedVersion: vuln.FixedVersion || vuln.InstalledVersion,
        vulnerabilityID: vuln.VulnerabilityID,
      });
    }
  }

  return {
    apiVersion: "v1alpha1",
    metadata: {
      os: {
        type: osType,
        version: osVersion,
      },
      config: {
        arch: "amd64",
      },
    },
    updates,
  };
}

/**
 * Load a Trivy JSON report from a file and convert it.
 */
export async function convertTrivyReport(
  path: string,
  options: ConvertOptions = {},
): Promise<CopaManifest> {
  const raw = await readFile(path, "utf-8");
  const report = JSON.parse(raw) as TrivyReport;
  return trivyToCopa(report, options);
}

// ─── Summary Formatting ──────────────────────────────────────────────────────

/**
 * Generate a human-readable summary of the conversion.
 */
export function formatConversionSummary(
  report: TrivyReport,
  manifest: CopaManifest,
): string {
  const lines: string[] = [];

  // Count total vulnerabilities found
  let totalVulns = 0;
  let fixableVulns = 0;
  for (const result of report.Results || []) {
    for (const vuln of result.Vulnerabilities || []) {
      totalVulns++;
      if (vuln.FixedVersion) fixableVulns++;
    }
  }

  lines.push("=== Trivy → Copa Conversion Summary ===");
  lines.push(`Image: ${report.ArtifactName || "unknown"}`);
  lines.push(`OS: ${manifest.metadata.os.type} ${manifest.metadata.os.version}`);
  lines.push("");
  lines.push(`Total vulnerabilities found: ${totalVulns}`);
  lines.push(`Fixable vulnerabilities: ${fixableVulns}`);
  lines.push(`Updates in manifest: ${manifest.updates.length}`);
  lines.push("");

  if (manifest.updates.length === 0) {
    lines.push("No fixable vulnerabilities found.");
  } else {
    lines.push("Updates:");
    for (const update of manifest.updates) {
      const fixed =
        update.installedVersion !== update.fixedVersion
          ? `${update.installedVersion} → ${update.fixedVersion}`
          : `${update.installedVersion} (no version change)`;
      lines.push(`  ${update.vulnerabilityID}: ${update.name} (${fixed})`);
    }
  }

  return lines.join("\n");
}

// ─── Severity Analysis ───────────────────────────────────────────────────────

/**
 * Group vulnerabilities by severity for analysis.
 */
export function analyzeSeverities(report: TrivyReport): Record<string, number> {
  const counts: Record<string, number> = {
    CRITICAL: 0,
    HIGH: 0,
    MEDIUM: 0,
    LOW: 0,
    UNKNOWN: 0,
  };

  for (const result of report.Results || []) {
    for (const vuln of result.Vulnerabilities || []) {
      const sev = vuln.Severity.toUpperCase();
      if (sev in counts) {
        counts[sev]++;
      } else {
        counts.UNKNOWN++;
      }
    }
  }

  return counts;
}

// ─── CLI ─────────────────────────────────────────────────────────────────────

/**
 * Format a Copa manifest as a formatted JSON string.
 */
export function formatManifest(manifest: CopaManifest): string {
  return JSON.stringify(manifest, null, 2);
}

export { formatConversionSummary as formatSummary };
