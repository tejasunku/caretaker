package prefs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// SigningConfig is a cosign v3 signing configuration with no transparency log
// or timestamping services, which keeps the prototype fully offline. cosign v3
// refuses --tlog-upload=false alongside its default config, so this document is
// always passed explicitly with --signing-config.
const SigningConfig = `{"mediaType":"application/vnd.dev.sigstore.signingconfig.v0.2+json","rekorTlogConfig":{},"tsaConfig":{}}`

// Sign cosign-signs ref (repo@digest) with the given private key. The signature
// lands as a referrer of the version artifact itself.
func Sign(ctx context.Context, cosign, ref, keyPath string) error {
	if err := checkKey(keyPath, "signing key"); err != nil {
		return err
	}
	cfg, err := os.CreateTemp("", "caretaker-signing-config-*.json")
	if err != nil {
		return fmt.Errorf("create signing config: %w", err)
	}
	defer os.Remove(cfg.Name())
	if _, err := cfg.WriteString(SigningConfig); err != nil {
		cfg.Close()
		return fmt.Errorf("write signing config: %w", err)
	}
	if err := cfg.Close(); err != nil {
		return fmt.Errorf("write signing config: %w", err)
	}
	return run(ctx, cosign, "sign", "--yes", "--signing-config", cfg.Name(), "--key", keyPath, ref)
}

// Verify checks the signature on ref (repo@digest) with the given public key.
func Verify(ctx context.Context, cosign, ref, pubKeyPath string) error {
	if err := checkKey(pubKeyPath, "public key"); err != nil {
		return err
	}
	return run(ctx, cosign, "verify", "--key", pubKeyPath,
		"--insecure-ignore-tlog=true", "--insecure-ignore-sct=true", ref)
}

func checkKey(path, what string) error {
	if path == "" {
		return fmt.Errorf("%s path is required", what)
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%s %s: %w (generate one with: cosign generate-key-pair --output-key-prefix .caretaker/cosign)", what, path, err)
	}
	return nil
}

func run(ctx context.Context, cosign string, args ...string) error {
	if cosign == "" {
		cosign = "cosign"
	}
	bin, err := exec.LookPath(cosign)
	if err != nil {
		return fmt.Errorf("cosign not found (./scripts/setup.sh installs it): %w", err)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	// Keys generated with an empty COSIGN_PASSWORD must be used with the same
	// empty password, and none of these commands may prompt.
	cmd.Env = append(os.Environ(), "COSIGN_PASSWORD=")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cosign %s: %w\n%s", args[0], err, out)
	}
	return nil
}
