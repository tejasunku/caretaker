// Core types for the patch tracking system

export interface Patch {
  id: string;            // e.g., "CVE-2024-1234"
  severity: "critical" | "high" | "medium" | "low";
  package: string;       // e.g., "openssl"
  fixedVersion: string;  // e.g., "3.0.13"
  releasedAt: string;    // ISO timestamp
  reconciliationWindowDays: number;
}

export interface ImageRecord {
  tag: string;
  sha: string;
  builtAt: string;       // ISO timestamp
  baseImage: string;     // e.g., "ubuntu:22.04"
  baseDigest: string;
}

export interface PatchApplication {
  imageTag: string;
  appliedAt: string;     // ISO timestamp
  patchesApplied: string[]; // patch IDs
}

export interface ServiceDeployment {
  tag: string;
  sha: string;
  replicaCount?: number;
  resourceTier?: "small" | "medium" | "large";
}

export interface Snapshot {
  folder: string;
  timestamp: string;
  services: Record<string, ServiceDeployment>;
  registry: Record<string, { digest: string; pushed_at: string }>;
}

export interface PatchStatus {
  patch: Patch;
  applied: boolean;
  appliedAt?: string;
}

export interface ImagePatchReport {
  imageTag: string;
  builtAt: string;
  baseImage: string;
  totalPatchesAvailable: number;
  patchesApplied: number;
  patchesMissing: number;
  status: "compliant" | "non_compliant" | "unknown";
  details: PatchStatus[];
}

export interface TimeRangeReport {
  rangeStart: string;
  rangeEnd: string;
  patchesReleased: Patch[];
  imagesNeedingPatches: {
    imageTag: string;
    missingPatches: Patch[];
    urgency: "overdue" | "due_soon" | "ok";
  }[];
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
  previousSha: string;
  currentSha: string;
  detectedInSnapshot: string; // snapshot folder where mutation was detected
  timestamp: string;
}

export type Environment = "testing" | "staging" | "production";

export interface EnvironmentDeployment {
  environment: Environment;
  sha: string;
  tag: string;
  deployedAt: string;
  status: "deployed" | "promoted" | "rolled_back";
}

export interface TagMutationReport {
  mutation: TagMutation;
  patchStatus: ImagePatchReport | null;
  environmentDeployments: EnvironmentDeployment[];
  testingGap: boolean;  // sha exists in staging/prod but never tested
  summary: string;      // human-readable risk assessment
}
