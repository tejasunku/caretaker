// Core types for the patch tracking system

export interface Patch {
  id: string;            // e.g., "CVE-2024-1234"
  severity: "critical" | "high" | "medium" | "low";
  package: string;       // e.g., "openssl"
  fixedVersion: string;  // e.g., "3.0.13"
  releasedAt: string;    // ISO timestamp
}

export interface ImageRecord {
  tag: string;              // Simple feature tag, e.g., "api:4.1"
  digest: string;           // Immutable content-addressable identity
  builtAt: string;          // ISO timestamp
  baseImage: string;        // e.g., "node:20-slim"
  baseDigest: string;       // Digest of the base image
  originatorDigest: string; // Digest of the original feature build (lineage anchor)
  parentDigest: string;     // Immediate parent image digest
  isPatched: boolean;       // True if produced by Copa patching
  patchesApplied: string[]; // Patch IDs applied to this image
}

export interface ServiceDeployment {
  tag: string;
  digest: string;          // Image digest (immutable identity)
  replicaCount?: number;
  resourceTier?: "small" | "medium" | "large";
}

export interface Snapshot {
  folder: string;
  timestamp: string;
  services: Record<string, ServiceDeployment>;
  registry: Record<string, { digest: string; pushed_at: string }>;
}

export interface SnapshotDiff {
  added: Record<string, ServiceDeployment>;
  removed: Record<string, ServiceDeployment>;
  changed: Record<string, {
    prev: ServiceDeployment;
    curr: ServiceDeployment;
    diffType: string;
  }>;
  unchanged: Record<string, ServiceDeployment>;
}

// Tag mutation detection

export interface TagMutation {
  serviceName: string;
  tag: string;              // The tag that was reused
  previousDigest: string;
  currentDigest: string;
  detectedInSnapshot: string; // snapshot folder where mutation was detected
  timestamp: string;
}

export type Environment = "testing" | "staging" | "production";

export interface EnvironmentDeployment {
  environment: Environment;
  digest: string;
  tag: string;
  deployedAt: string;
  status: "deployed" | "promoted" | "rolled_back";
}
