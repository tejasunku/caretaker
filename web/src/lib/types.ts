// Core types for the patch tracking system

export interface Patch {
  id: string;            // e.g., "CVE-2024-1234"
  severity: "critical" | "high" | "medium" | "low";
  package: string;       // e.g., "openssl"
  fixedVersion: string;  // e.g., "3.0.13"
  releasedAt: string;    // ISO timestamp
}
