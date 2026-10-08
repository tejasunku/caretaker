#!/usr/bin/env node
/**
 * CLI wrapper for the SBOM checker.
 *
 * Usage:
 *   node --experimental-strip-types scripts/check-sbom.ts --image nginx:1.27 --manifest patch-manifests/mock-patches.json
 */

import { checkImage, loadManifest, formatResult } from "../web/src/lib/sbom-checker.ts";

// ─── Argument Parsing ────────────────────────────────────────────────────────

interface Args {
  image: string;
  manifest: string;
  trivyPath: string;
  json: boolean;
  help: boolean;
}

function parseArgs(): Args {
  const args: Args = {
    image: "",
    manifest: "",
    trivyPath: "trivy",
    json: false,
    help: false,
  };

  const argv = process.argv.slice(2);
  for (let i = 0; i < argv.length; i++) {
    switch (argv[i]) {
      case "--image":
      case "-i":
        args.image = argv[++i] || "";
        break;
      case "--manifest":
      case "-m":
        args.manifest = argv[++i] || "";
        break;
      case "--trivy":
        args.trivyPath = argv[++i] || "trivy";
        break;
      case "--json":
        args.json = true;
        break;
      case "--help":
      case "-h":
        args.help = true;
        break;
    }
  }

  return args;
}

function printHelp(): void {
  console.log(`
SBOM Checker — verify image patch state against Copa-native patch manifests.

Usage:
  node --experimental-strip-types scripts/check-sbom.ts [options]

Options:
  -i, --image <ref>      Container image reference (e.g., nginx:1.27)
  -m, --manifest <path>  Path to Copa v1alpha1 patch manifest JSON
  --trivy <path>         Path to trivy binary (default: trivy)
  --json                 Output as JSON instead of formatted text
  -h, --help             Show this help

Examples:
  # Check image against mock patches
  node --experimental-strip-types scripts/check-sbom.ts \\
    --image nginx:1.27 \\
    --manifest patch-manifests/mock-patches.json

  # Check with JSON output
  node --experimental-strip-types scripts/check-sbom.ts \\
    -i nginx:1.27 -m patch-manifests/cve-only.json --json

  # Use specific trivy binary
  node --experimental-strip-types scripts/check-sbom.ts \\
    -i nginx:1.27 -m patch-manifests/mock-patches.json \\
    --trivy ./bin/trivy

Exit codes:
  0  All patches satisfied
  1  Some patches missing or unknown
  2  Error (invalid args, trivy failed, etc.)
`);
}

// ─── Main ────────────────────────────────────────────────────────────────────

async function main(): Promise<void> {
  const args = parseArgs();

  if (args.help || !args.image || !args.manifest) {
    printHelp();
    process.exit(args.help ? 0 : 2);
  }

  try {
    // Load manifest
    const manifest = await loadManifest(args.manifest);

    // Run check
    const result = await checkImage(args.image, manifest, {
      trivyPath: args.trivyPath,
    });
    result.manifestPath = args.manifest;

    // Output
    if (args.json) {
      console.log(JSON.stringify(result, null, 2));
    } else {
      console.log(formatResult(result));
    }

    // Exit code based on results
    if (result.summary.missing > 0 || result.summary.unknown > 0) {
      process.exit(1);
    }
    process.exit(0);
  } catch (error) {
    console.error("Error:", error instanceof Error ? error.message : error);
    process.exit(2);
  }
}

main();
