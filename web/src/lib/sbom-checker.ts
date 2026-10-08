/**
 * SBOM Checker — verifies image patch state against Copa-native patch manifests.
 *
 * Uses Trivy to generate CycloneDX SBOMs, then checks package versions
 * against a patch manifest in Copa's v1alpha1 format.
 */

import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { readFile } from "node:fs/promises";

const execFileAsync = promisify(execFile);

// ─── Types ───────────────────────────────────────────────────────────────────

/** Copa v1alpha1 update package */
export interface CopaUpdatePackage {
  name: string;
  installedVersion: string;
  fixedVersion: string;
  vulnerabilityID: string;
}

/** Copa v1alpha1 manifest */
export interface CopaManifest {
  apiVersion: "v1alpha1";
  metadata: {
    os: { type: string; version: string };
    config: { arch: string };
  };
  updates: CopaUpdatePackage[];
}

/** CycloneDX SBOM component (subset we care about) */
interface CycloneDXComponent {
  "bom-ref"?: string;
  name: string;
  version?: string;
  purl?: string;
  type?: string;
  "package-manager"?: string;
  hashes?: Array<{ alg: string; content: string }>;
  properties?: Array<{ name: string; value: string }>;
}

/** CycloneDX SBOM document (subset) */
interface CycloneDXBom {
  bomFormat?: string;
  specVersion?: string;
  components?: CycloneDXComponent[];
}

/** Result of checking a single patch against an SBOM */
export interface PatchCheckResult {
  vulnerabilityID: string;
  packageName: string;
  requiredVersion: string;
  installedVersion: string | null;
  status: "satisfied" | "missing" | "unknown" | "different";
  /** True if installed version is newer than required (>= check passes) */
  versionOk: boolean;
  /** Human-readable explanation */
  detail: string;
}

/** Full check result for a manifest against an image */
export interface SBOMCheckResult {
  image: string;
  manifestPath: string;
  osType: string;
  osVersion: string;
  totalPackages: number;
  results: PatchCheckResult[];
  summary: {
    satisfied: number;
    missing: number;
    unknown: number;
    different: number;
  };
}

// ─── Version Comparison ──────────────────────────────────────────────────────

/**
 * Compare two package versions using a Debian-style comparison.
 * Returns: -1 (a < b), 0 (a == b), 1 (a > b)
 *
 * Handles common Debian version format: [epoch:]upstream[-revision]
 */
export function compareVersions(a: string, b: string): number {
  // Normalize: strip leading 'v' if present
  const normA = a.replace(/^v/, "");
  const normB = b.replace(/^v/, "");

  // Try simple semver-like comparison first
  const semverResult = trySemverCompare(normA, normB);
  if (semverResult !== null) return semverResult;

  // Fall back to Debian-style comparison
  return debianCompare(normA, normB);
}

function trySemverCompare(a: string, b: string): number | null {
  // Match: major.minor.patch with optional suffixes
  const semverRegex = /^(\d+)\.(\d+)\.(\d+)(?:[-+](.+))?$/;
  const matchA = a.match(semverRegex);
  const matchB = b.match(semverRegex);

  if (!matchA || !matchB) return null;

  const majorA = parseInt(matchA[1], 10);
  const majorB = parseInt(matchB[1], 10);
  if (majorA !== majorB) return majorA < majorB ? -1 : 1;

  const minorA = parseInt(matchA[2], 10);
  const minorB = parseInt(matchB[2], 10);
  if (minorA !== minorB) return minorA < minorB ? -1 : 1;

  const patchA = parseInt(matchA[3], 10);
  const patchB = parseInt(matchB[3], 10);
  if (patchA !== patchB) return patchA < patchB ? -1 : 1;

  // Same base version — compare suffixes (prerelease < release)
  const suffixA = matchA[4] || "";
  const suffixB = matchB[4] || "";
  if (suffixA === suffixB) return 0;
  if (!suffixA) return 1; // no suffix > has suffix
  if (!suffixB) return -1;
  return suffixA < suffixB ? -1 : 1;
}

/**
 * Debian-style version comparison.
 * Format: [epoch:]upstream_version[-debian_revision]
 */
function debianCompare(a: string, b: string): number {
  // Split into components
  const parseDebian = (v: string) => {
    let epoch = 0;
    let upstream = v;
    let revision = "";

    // Check for epoch
    const epochMatch = v.match(/^(\d+):(.+)$/);
    if (epochMatch) {
      epoch = parseInt(epochMatch[1], 10);
      upstream = epochMatch[2];
    }

    // Split upstream and revision
    const revSplit = upstream.lastIndexOf("-");
    if (revSplit >= 0) {
      revision = upstream.slice(revSplit + 1);
      upstream = upstream.slice(0, revSplit);
    }

    return { epoch, upstream, revision };
  };

  const aParsed = parseDebian(a);
  const bParsed = parseDebian(b);

  // Compare epochs
  if (aParsed.epoch !== bParsed.epoch) {
    return aParsed.epoch < bParsed.epoch ? -1 : 1;
  }

  // Compare upstream versions
  const upstreamResult = debianStringCompare(aParsed.upstream, bParsed.upstream);
  if (upstreamResult !== 0) return upstreamResult;

  // Compare revisions
  return debianStringCompare(aParsed.revision, bParsed.revision);
}

/**
 * Debian string comparison: splits into digit and non-digit parts.
 */
function debianStringCompare(a: string, b: string): number {
  // Empty string is less than non-empty
  if (a === b) return 0;
  if (a === "") return -1;
  if (b === "") return 1;

  let i = 0;
  let j = 0;

  while (i < a.length && j < b.length) {
    const charA = a[i];
    const charB = b[j];

    // Both digits — compare as numbers
    if (charA >= "0" && charA <= "9" && charB >= "0" && charB <= "9") {
      // Skip leading zeros
      while (i < a.length && a[i] === "0") i++;
      while (j < b.length && b[j] === "0") j++;

      // Find end of number
      let endI = i;
      let endJ = j;
      while (endI < a.length && a[endI] >= "0" && a[endI] <= "9") endI++;
      while (endJ < b.length && b[endJ] >= "0" && b[endJ] <= "9") endJ++;

      const lenI = endI - i;
      const lenJ = endJ - j;

      // Longer number is greater (after skipping leading zeros)
      if (lenI !== lenJ) return lenI < lenJ ? -1 : 1;

      // Same length — compare digit by digit
      for (let k = 0; k < lenI; k++) {
        if (a[i + k] !== b[j + k]) {
          return a[i + k] < b[j + k] ? -1 : 1;
        }
      }

      i = endI;
      j = endJ;
    } else {
      // Non-digit characters — compare directly
      // Debian: letters sort before non-letters (except ~)
      const isLetterA = (charA >= "a" && charA <= "z") || (charA >= "A" && charA <= "Z");
      const isLetterB = (charB >= "a" && charB <= "z") || (charB >= "A" && charB <= "Z");

      if (charA === charB) {
        i++;
        j++;
        continue;
      }

      // Tilde sorts before everything
      if (charA === "~" && charB !== "~") return -1;
      if (charB === "~" && charA !== "~") return 1;

      // Letters sort before non-letters
      if (isLetterA && !isLetterB) return -1;
      if (isLetterB && !isLetterA) return 1;

      // Both letters or both non-letters — compare directly
      return charA < charB ? -1 : 1;
    }
  }

  // One string exhausted
  if (i < a.length) return 1; // a has more
  if (j < b.length) return -1; // b has more
  return 0;
}

// ─── SBOM Generation ─────────────────────────────────────────────────────────

/**
 * Generate a CycloneDX SBOM for an image using Trivy.
 * Returns the parsed SBOM.
 */
export async function generateSBOM(
  image: string,
  trivyPath: string = "trivy",
): Promise<CycloneDXBom> {
  const { stdout } = await execFileAsync(trivyPath, [
    "image",
    "--format",
    "cyclonedx",
    "--quiet",
    "--no-progress",
    image,
  ], {
    maxBuffer: 50 * 1024 * 1024, // 50MB
  });

  return JSON.parse(stdout) as CycloneDXBom;
}

/**
 * Extract package name → version map from a CycloneDX SBOM.
 */
export function extractPackages(bom: CycloneDXBom): Map<string, string> {
  const packages = new Map<string, string>();

  for (const component of bom.components || []) {
    if (!component.name) continue;

    // Prefer version field, fall back to parsing from purl
    let version = component.version || "";

    if (!version && component.purl) {
      // PURL format: pkg:type/name@version
      const purlMatch = component.purl.match(/@([^?]+)/);
      if (purlMatch) {
        version = purlMatch[1];
      }
    }

    if (version) {
      packages.set(component.name, version);
    }
  }

  return packages;
}

// ─── Patch Checking ──────────────────────────────────────────────────────────

/**
 * Check a single patch against the package map.
 */
export function checkPatch(
  patch: CopaUpdatePackage,
  packages: Map<string, string>,
): PatchCheckResult {
  const installed = packages.get(patch.name);

  if (!installed) {
    return {
      vulnerabilityID: patch.vulnerabilityID,
      packageName: patch.name,
      requiredVersion: patch.fixedVersion,
      installedVersion: null,
      status: "unknown",
      versionOk: false,
      detail: `Package "${patch.name}" not found in SBOM`,
    };
  }

  const comparison = compareVersions(installed, patch.fixedVersion);

  if (comparison >= 0) {
    // Installed >= fixed — satisfied
    const isExact = comparison === 0;
    return {
      vulnerabilityID: patch.vulnerabilityID,
      packageName: patch.name,
      requiredVersion: patch.fixedVersion,
      installedVersion: installed,
      status: "satisfied",
      versionOk: true,
      detail: isExact
        ? `Installed version ${installed} matches required ${patch.fixedVersion}`
        : `Installed version ${installed} is newer than required ${patch.fixedVersion}`,
    };
  }

  // Installed < fixed — missing patch
  return {
    vulnerabilityID: patch.vulnerabilityID,
    packageName: patch.name,
    requiredVersion: patch.fixedVersion,
    installedVersion: installed,
    status: "missing",
    versionOk: false,
    detail: `Installed version ${installed} is older than required ${patch.fixedVersion}`,
  };
}

/**
 * Check all patches in a manifest against an image's SBOM.
 */
export async function checkImage(
  image: string,
  manifest: CopaManifest,
  options: {
    trivyPath?: string;
    sbom?: CycloneDXBom;
  } = {},
): Promise<SBOMCheckResult> {
  // Generate or reuse SBOM
  const bom = options.sbom || await generateSBOM(image, options.trivyPath);
  const packages = extractPackages(bom);

  // Check each patch
  const results = manifest.updates.map((patch) => checkPatch(patch, packages));

  // Compute summary
  const summary = {
    satisfied: results.filter((r) => r.status === "satisfied").length,
    missing: results.filter((r) => r.status === "missing").length,
    unknown: results.filter((r) => r.status === "unknown").length,
    different: results.filter((r) => r.status === "different").length,
  };

  return {
    image,
    manifestPath: "",
    osType: manifest.metadata.os.type,
    osVersion: manifest.metadata.os.version,
    totalPackages: packages.size,
    results,
    summary,
  };
}

/**
 * Load a Copa manifest from a JSON file.
 */
export async function loadManifest(path: string): Promise<CopaManifest> {
  const raw = await readFile(path, "utf-8");
  return JSON.parse(raw) as CopaManifest;
}

// ─── CLI ─────────────────────────────────────────────────────────────────────

/**
 * Format a check result as a human-readable string.
 */
export function formatResult(result: SBOMCheckResult): string {
  const lines: string[] = [];

  lines.push(`=== SBOM Check: ${result.image} ===`);
  lines.push(`OS: ${result.osType} ${result.osVersion}`);
  lines.push(`Total packages in SBOM: ${result.totalPackages}`);
  lines.push("");

  for (const r of result.results) {
    const icon =
      r.status === "satisfied" ? "✓" :
      r.status === "missing" ? "✗" :
      r.status === "unknown" ? "?" : "≠";

    const versionInfo = r.installedVersion
      ? `${r.installedVersion} (need ${r.requiredVersion})`
      : `(need ${r.requiredVersion})`;

    lines.push(`${icon} ${r.vulnerabilityID}: ${r.packageName} ${versionInfo}`);
    lines.push(`  ${r.detail}`);
  }

  lines.push("");
  lines.push(
    `Summary: ${result.summary.satisfied} satisfied, ` +
    `${result.summary.missing} missing, ` +
    `${result.summary.unknown} unknown` +
    (result.summary.different > 0 ? `, ${result.summary.different} different` : "")
  );

  return lines.join("\n");
}

// ─── Exports for CLI usage ───────────────────────────────────────────────────

export { compareVersions as comparePackageVersions };
